package main

import (
	"bytes"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	net_url "net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func makeUsers() *Users {
	users := new(Users)
	users.slice = make([]User, 0)
	users.idCounter = 1
	return users
}

func generateToken() string {
	bytes := make([]byte, TOKEN_LENGTH)
	_, err := cryptorand.Read(bytes)

	if err != nil {
		LogError("Token generation failed, this should not happen!")
		return ""
	}

	return base64.URLEncoding.EncodeToString(bytes)
}

func (users *Users) create() User {
	id := users.idCounter
	users.idCounter += 1

	now := time.Now()
	new_user := User{
		Id:         id,
		Username:   generateRandomNickname(),
		Avatar:     "img/default_avatar.png",
		Online:     false,
		CreatedAt:  now,
		LastUpdate: now,
		LastOnline: now,
		token:      generateToken(),
	}

	users.slice = append(users.slice, new_user)
	return new_user
}

func (users *Users) removeByToken(token string) *User {
	for i, user := range users.slice {
		if user.token == token {
			users.slice[i] = users.slice[len(users.slice)-1]
			users.slice = users.slice[:len(users.slice)-1]
			LogDebug("%v", user)
			return &user
		}
	}

	return nil
}

func (users *Users) findByToken(token string) int {
	for i, user := range users.slice {
		if user.token == token {
			return i
		}
	}

	return -1
}

func makeConnections() *Connections {
	conns := new(Connections)
	conns.slice = make([]Connection, 0)
	conns.idCounter = 1
	return conns
}

func (conns *Connections) add(userId uint64) Connection {
	id := conns.idCounter
	conns.idCounter += 1

	conn := Connection{
		id:     id,
		userId: userId,
		events: make(chan string, 100),
		close:  make(chan bool, 1),
	}
	conns.slice = append(conns.slice, conn)

	return conn
}

func (conns *Connections) removeById(id uint64) {
	for i, conn := range conns.slice {
		if conn.id != id {
			continue
		}

		length := len(conns.slice)
		conns.slice[i], conns.slice[length-1] = conns.slice[length-1], conns.slice[i]
		conns.slice = conns.slice[:length-1]
		break
	}
}

func createPlaylistEvent(action string, data any) PlaylistEvent {
	event := PlaylistEvent{
		Action: action,
		Data:   data,
	}

	return event
}

func StartServer(config ServerConfig, db *sql.DB) {
	users, ok := DatabaseLoadUsers(db)
	if !ok {
		return
	}

	maxEntryId := DatabaseMaxEntryId(db)
	maxSubId := DatabaseMaxSubtitleId(db)

	history, _ := DatabaseHistoryGet(db)
	playlist, _ := DatabasePlaylistGet(db)

	autoplay := DatabaseGetAutoplay(db)
	looping := DatabaseGetLooping(db)

	server := Server{
		config: config,
		state: ServerState{
			entry:    Entry{},
			playlist: playlist,
			history:  history,
			messages: make([]ChatMessage, 0),
			player: PlayerState{
				Autoplay: autoplay,
				Looping:  looping,
			},
		},

		users: users,
		conns: makeConnections(),
		db:    db,
	}

	server.state.subsId.Store(maxSubId)
	server.state.entryId.Store(maxEntryId)

	server.state.lastUpdate = time.Now()
	handler := registerEndpoints(&server)

	var address = config.Address + ":" + strconv.Itoa(int(config.Port))
	LogInfo("Starting server on address: %s", address)

	const CERT = "./secret/certificate.pem"
	const PRIV_KEY = "./secret/privatekey.pem"

	_, err_cert := os.Stat(CERT)
	_, err_priv := os.Stat(PRIV_KEY)

	missing_ssl_keys := errors.Is(err_priv, os.ErrNotExist) || errors.Is(err_cert, os.ErrNotExist)

	if config.EnableSsl && missing_ssl_keys {
		LogError("Failed to find either SSL certificate or the private key.")
	}

	go server.periodicResync()
	go server.periodicInactiveUserCleanup()

	internalLogger := CreateInternalLoggerForHttpServer()

	httpServer := http.Server{
		Addr:     address,
		ErrorLog: internalLogger,
		Handler:  handler,
	}

	if config.RedirectPort != 0 {
		go createRedirectServer(config)
	}

	if !config.EnableSsl || missing_ssl_keys {
		serverRootAddress = "http://" + address
	} else {
		serverRootAddress = "https://" + address
	}

	CaptureCtrlC(&server)

	go func() {
		currentEntry, _ := DatabaseCurrentEntryGet(db)
		server.setNewEntry(currentEntry, RequestEntry{})

		timestamp := DatabaseGetTimestamp(server.db)
		server.playerSeek(timestamp, 0)
	}()

	var server_start_error error
	if !config.EnableSsl || missing_ssl_keys {
		LogWarn("Server is running in unencrypted http mode.")
		server_start_error = httpServer.ListenAndServe()
	} else {
		LogInfo("Server is running with TLS on.")
		server_start_error = httpServer.ListenAndServeTLS(CERT, PRIV_KEY)
	}

	if server_start_error != nil {
		LogError("Error starting the server: %v", server_start_error)
	}
}

func createRedirectServer(config ServerConfig) {
	LogInfo("Creating a redirect server to '%v'.", config.RedirectTo)

	redirectFunc := func(w http.ResponseWriter, r *http.Request) {
		var redirect = config.RedirectTo + r.RequestURI
		http.Redirect(w, r, redirect, http.StatusMovedPermanently)
	}

	var address = config.Address + ":" + strconv.Itoa(int(config.RedirectPort))
	redirectServer := http.Server{
		Addr:    address,
		Handler: http.HandlerFunc(redirectFunc),
	}

	err := redirectServer.ListenAndServe()
	if err != nil {
		LogError("Failed to start the redirect server: %v", err)
	}
}

func handleUnknownEndpoint(w http.ResponseWriter, r *http.Request) {
	LogWarn("User %v requested unknown endpoint: %v", r.RemoteAddr, r.RequestURI)
	if len(r.RequestURI) > MAX_UNKNOWN_PATH_LENGTH {
		blackholeRequest(r)
		http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
		return
	}
	endpoint := stripParams(r.RequestURI)
	file := path.Base(endpoint)
	if endsWithAny(file, ".php", ".cgi", ".jsp", ".aspx", "wordpress", "owa") {
		blackholeRequest(r)
	}
	http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
}

func blackholeRequest(r *http.Request) {
	start := time.Now()
	context := r.Context()
	select {
	case <-time.After(BLACK_HOLE_PERIOD):
		LogDebug("%v exited black hole after waiting period of %v.", r.RemoteAddr, BLACK_HOLE_PERIOD)
	case <-context.Done():
		LogDebug("%v exited black hole due to cancellation client side after %v.", r.RemoteAddr, time.Since(start))
		return
	}
}

func serveFavicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/img/favicon.ico")
}

type CacheHandler struct {
	fsHandler    http.Handler
	ipToLimiters map[string]*RateLimiter
	mapMutex     *sync.Mutex
}

func (cache CacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := strings.Split(r.RemoteAddr, ":")[0]

	cache.mapMutex.Lock()
	rateLimiter, exists := cache.ipToLimiters[ip]
	if exists {
		cache.mapMutex.Unlock()
		rateLimiter.mutex.Lock()
		if rateLimiter.block() {
			retryAfter := rateLimiter.getRetryAfter()
			rateLimiter.mutex.Unlock()
			respondTooManyRequests(w, ip, retryAfter)
			return
		}
		rateLimiter.mutex.Unlock()
	} else {
		cache.ipToLimiters[ip] = NewLimiter(LIMITER_HITS, LIMITER_PER_SECOND)
		cache.mapMutex.Unlock()
	}

	resource := strings.TrimPrefix(r.RequestURI, "/watch")
	LogDebug("Connection %s requested resource %v", r.RemoteAddr, resource)

	// The no-cache directive does not prevent the storing of responses
	// but instead prevents the reuse of responses without revalidation.
	w.Header().Add("Cache-Control", "no-cache")

	/*if resource == "/media/video/Cats.webm" {
		LogDebug("Serving using shareFile() function")
		shareFile(w, r, "web"+resource)
		return
	}*/
	cache.fsHandler.ServeHTTP(w, r)
}

func registerEndpoints(server *Server) *http.ServeMux {
	mux := http.NewServeMux()

	fileserver := http.FileServer(http.Dir("./web"))
	fsHandler := http.StripPrefix("/watch/", fileserver)
	cacheableFs := CacheHandler{fsHandler, make(map[string]*RateLimiter), &sync.Mutex{}}
	mux.Handle("/watch/", cacheableFs)

	mux.HandleFunc("/", handleUnknownEndpoint)

	mux.HandleFunc("/favicon.ico", serveFavicon)

	// Unrelated API calls.
	server.HandleEndpoint(mux, "/watch/api/version", server.apiVersion, "GET", false)
	server.HandleEndpoint(mux, "/watch/api/uptime", server.apiUptime, "GET", false)
	server.HandleEndpoint(mux, "/watch/api/login", server.apiLogin, "GET", false)
	server.HandleEndpoint(mux, "/watch/api/uploadmedia", server.apiUploadMedia, "POST", true)

	// User related API calls.
	server.HandleEndpoint(mux, "/watch/api/user/create", server.apiUserCreate, "GET", false)
	server.HandleEndpoint(mux, "/watch/api/user/verify", server.apiUserVerify, "POST", false)
	server.HandleEndpoint(mux, "/watch/api/user/delete", server.apiUserDelete, "POST", false)
	server.HandleEndpoint(mux, "/watch/api/user/getall", server.apiUserGetAll, "GET", true)
	server.HandleEndpoint(mux, "/watch/api/user/updatename", server.apiUserUpdateName, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/user/updateavatar", server.apiUserUpdateAvatar, "POST", true)

	// API calls that change state of the player.
	server.HandleEndpoint(mux, "/watch/api/player/get", server.apiPlayerGet, "GET", true)
	server.HandleEndpoint(mux, "/watch/api/player/set", server.apiPlayerSet, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/end", server.apiPlayerEnd, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/next", server.apiPlayerNext, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/play", server.apiPlayerPlay, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/pause", server.apiPlayerPause, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/seek", server.apiPlayerSeek, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/autoplay", server.apiPlayerAutoplay, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/looping", server.apiPlayerLooping, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/player/updatetitle", server.apiPlayerUpdateTitle, "POST", true)

	// Subtitle API calls.
	server.HandleEndpoint(mux, "/watch/api/subtitle/delete", server.apiSubtitleDelete, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/subtitle/update", server.apiSubtitleUpdate, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/subtitle/attach", server.apiSubtitleAttach, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/subtitle/shift", server.apiSubtitleShift, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/subtitle/upload", server.apiSubtitleUpload, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/subtitle/download", server.apiSubtitleDownload, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/subtitle/search", server.apiSubtitleSearch, "POST", true)

	// API calls that change state of the playlist.
	server.HandleEndpoint(mux, "/watch/api/playlist/get", server.apiPlaylistGet, "GET", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/play", server.apiPlaylistPlay, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/add", server.apiPlaylistAdd, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/clear", server.apiPlaylistClear, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/remove", server.apiPlaylistRemove, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/shuffle", server.apiPlaylistShuffle, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/move", server.apiPlaylistMove, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/playlist/update", server.apiPlaylistUpdate, "POST", true)

	// API calls that change state of the history.
	server.HandleEndpoint(mux, "/watch/api/history/get", server.apiHistoryGet, "GET", true)
	server.HandleEndpoint(mux, "/watch/api/history/clear", server.apiHistoryClear, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/history/play", server.apiHistoryPlay, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/history/remove", server.apiHistoryRemove, "POST", true)

	server.HandleEndpoint(mux, "/watch/api/chat/send", server.apiChatSend, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/chat/get", server.apiChatGet, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/chat/delete", server.apiChatDelete, "POST", true)

	server.HandleEndpoint(mux, "/watch/api/stream/start", server.apiStreamStart, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/stream/upload/{filename}", server.apiStreamUpload, "POST", true)
	// Server events and proxy.
	server.HandleEndpoint(mux, "/watch/api/events", server.apiEvents, "GET", false)

	server.HandleEndpoint(mux, PROXY_ROUTE, server.watchProxy, "GET", false)
	server.HandleEndpoint(mux, STREAM_ROUTE, server.watchStream, "GET", false)

	// Voice chat
	server.HandleEndpoint(mux, "/watch/vc", voiceChat, "GET", false)

	return mux
}

func (server *Server) HandleEndpoint(mux *http.ServeMux, endpoint string, endpointHandler func(w http.ResponseWriter, r *http.Request), method string, requireAuth bool) {
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				LogFatalUp(2, "Panic in endpoint handler for %v serving %v: %v", endpoint, r.RemoteAddr, err)
				// stack := strings.TrimSpace(string(debug.Stack()))
				// LogFatalUp(2, "Panic in endpoint handler for %v serving %v: %v\n%v", endpoint, r.RemoteAddr, err, stack)
			}
		}()

		if r.Method != method {
			errMsg := fmt.Sprintf("Method not allowed. %v was expected.", method)
			http.Error(w, errMsg, http.StatusMethodNotAllowed)
			return
		}

		if requireAuth && !server.isAuthorized(w, r) {
			return
		}

		// NOTE(kihau): Hack to prevent console spam on proxy.
		if PROXY_ROUTE != endpoint {
			endpointTrim := strings.TrimPrefix(endpoint, "/watch/api/")
			requested := strings.ReplaceAll(endpointTrim, "/", " ")
			LogInfo("Connection %s requested %v.", r.RemoteAddr, requested)
		}

		endpointHandler(w, r)
	}

	mux.HandleFunc(endpoint, genericHandler)
}

func (server *Server) periodicResync() {
	for {
		server.smartSleep()

		server.conns.mutex.Lock()
		connectionCount := len(server.conns.slice)
		server.conns.mutex.Unlock()

		if connectionCount <= 1 {
			continue
		}

		var event SyncEvent
		server.state.mutex.Lock()
		playing := server.state.player.Playing

		if server.state.entry.Url == "" {
			server.state.mutex.Unlock()
			continue
		}

		server.state.mutex.Unlock()

		if playing {
			event = server.createSyncEvent("play", 0)
		} else {
			event = server.createSyncEvent("pause", 0)
		}

		server.writeEventToAllConnections("sync", event)
	}
}

func (server *Server) periodicInactiveUserCleanup() {
	for {
		server.users.mutex.Lock()

		toDelete := make([]User, 0)
		for _, user := range server.users.slice {
			if user.LastUpdate == user.CreatedAt && time.Since(user.LastOnline) > time.Hour*24 && !user.Online {
				// Remove users that are not active and that have not updated their user profile.
				LogInfo("Removing dummy temp user with id = %v due to 24h of inactivity.", user.Id)

				DatabaseDeleteUser(server.db, user)
				toDelete = append(toDelete, user)
			}
		}

		for _, user := range toDelete {
			user := server.users.removeByToken(user.token)
			server.writeEventToAllConnections("userdelete", user)
		}

		server.users.mutex.Unlock()
		time.Sleep(time.Hour * 24)
	}
}

func (server *Server) setNewEntry(entry Entry, requested RequestEntry) {
	server.state.isLoading.Store(true)
	defer server.state.isLoading.Store(false)

	entry = server.constructEntry(entry)

	if isYoutube(entry, requested) {
		server.writeEventToAllConnections("playerwaiting", "Youtube video is loading. Please stand by!")

		err := loadYoutubeEntry(&entry, requested)
		if err != nil {
			server.writeEventToAllConnections("playererror", err.Error())
			return
		}

		if requested.IsPlaylist {
			requested.Url = entry.Url

			go func() {
				entries, err := server.loadYoutubePlaylist(requested, entry.UserId)
				if err == nil {
					server.state.mutex.Lock()
					server.playlistAddMany(entries, requested.AddToTop)
					server.state.mutex.Unlock()
				}
			}()
		}
	} else if isTwitch(entry) {
		server.writeEventToAllConnections("playerwaiting", "Twitch stream is loading. Please stand by!")

		err := loadTwitchEntry(&entry, requested)
		if err != nil {
			server.writeEventToAllConnections("playererror", err.Error())
			return
		}
	}

	err := server.setupProxy(&entry)
	if err != nil {
		LogWarn("%v", err)
		server.writeEventToAllConnections("playererror", err.Error())
		return
	}

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if server.state.player.Looping {
		server.playlistAdd(server.state.entry, false)
	}

	server.historyAdd(server.state.entry)

	server.state.entry = entry
	DatabaseCurrentEntrySet(server.db, entry)

	server.state.player.Timestamp = 0
	server.state.lastUpdate = time.Now()
	server.state.player.Playing = server.state.player.Autoplay

	LogInfo("New entry URL is now: '%s'.", entry.Url)
	server.writeEventToAllConnections("playerset", entry)

	go server.preloadYoutubeSourceOnNextEntry()
}

func isPathM3U(p string) bool {
	return strings.HasSuffix(p, ".m3u8") || strings.HasSuffix(p, ".m3u") || strings.HasSuffix(p, ".txt")
}

func (server *Server) isAuthorized(w http.ResponseWriter, r *http.Request) bool {
	server.users.mutex.Lock()
	defer server.users.mutex.Unlock()

	index := server.getAuthorizedIndex(w, r)
	return index != -1
}

func (server *Server) getAuthorized(w http.ResponseWriter, r *http.Request) *User {
	server.users.mutex.Lock()
	defer server.users.mutex.Unlock()

	index := server.getAuthorizedIndex(w, r)
	if index == -1 {
		return nil
	}

	user := server.users.slice[index]
	return &user
}

func (server *Server) getAuthorizedIndex(w http.ResponseWriter, r *http.Request) int {
	token := r.Header.Get("Authorization")
	if token == "" {
		respondBadRequest(w, "Failed to authorize user, specified token is empty")
		return -1
	}

	for i, user := range server.users.slice {
		if user.token == token {
			return i
		}
	}

	respondBadRequest(w, "User with the specified token is not in the user list")
	return -1
}

func (server *Server) findUser(token string) *User {
	index := server.users.findByToken(token)
	if index == -1 {
		return nil
	}

	return &server.users.slice[index]
}

func (server *Server) readJsonDataFromRequest(w http.ResponseWriter, r *http.Request, data any) bool {
	if r.ContentLength > BODY_LIMIT {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		LogWarnUp(1, "Request body too large")
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, BODY_LIMIT)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondBadRequest(w, "Failed to read request body: %v", err)
		return false
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		respondBadRequest(w, "Failed to deserialise request body data: %v", err)
		return false
	}

	return true
}

func (server *Server) writeEvent(w http.ResponseWriter, eventName string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)

		http.Error(w, "Failed to serialize welcome message", http.StatusInternalServerError)
		return err
	}

	jsonString := string(jsonData)
	eventId := server.state.eventId.Add(1)

	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\nretry: %d\n\n", eventId, eventName, jsonString, RETRY)
	if err != nil {
		return err
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func (server *Server) writeEventToAllConnections(eventName string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)
		return
	}

	jsonString := string(jsonData)

	eventId := server.state.eventId.Add(1)
	event := fmt.Sprintf("id: %v\nevent: %v\ndata: %v\nretry: %v\n\n", eventId, eventName, jsonString, RETRY)

	server.conns.mutex.Lock()
	for _, conn := range server.conns.slice {
		select {
		case conn.events <- event:
		default:
			LogWarn("Channel event write failed for connection: %v", conn.id)
		}
	}
	server.conns.mutex.Unlock()
}

// It should be possible to use this list in a dropdown and attach to entry
func (server *Server) getSubtitles() []string {
	subtitles := make([]string, 0)
	subsFolder := WEB_MEDIA + "subs"
	files, err := os.ReadDir(subsFolder)
	if err != nil {
		LogError("Failed to read directory %v", subsFolder)
		return subtitles
	}

	for _, file := range files {
		filename := file.Name()
		if !file.Type().IsRegular() {
			continue
		}
		for _, ext := range SUBTITLE_EXTENSIONS {
			info, err := file.Info()
			if err != nil {
				break
			}
			if strings.HasSuffix(filename, ext) && info.Size() < SUBTITLE_SIZE_LIMIT {
				subtitlePath := MEDIA + "subs/" + filename
				subtitles = append(subtitles, subtitlePath)
				break
			}
		}
	}
	LogInfo("Served subtitles: %v", subtitles)
	return subtitles
}

func (server *Server) setupGenericFileProxy(url string, referer string) bool {
	_ = os.RemoveAll(WEB_PROXY)
	_ = os.Mkdir(WEB_PROXY, os.ModePerm)
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		LogError("The provided URL is invalid: %v", err)
		return false
	}

	size, err := getContentLength(url, referer)
	if err != nil {
		LogError("Couldn't read resource metadata: %v", err)
		return false
	}
	if size > PROXY_FILE_SIZE_LIMIT {
		LogError("The file exceeds the specified limit of 4 GBs.")
		return false
	}
	server.state.setupLock.Lock()
	defer server.state.setupLock.Unlock()
	server.state.isHls = false
	server.state.isLive = false

	proxy := &server.state.genericProxy
	proxy.referer = referer
	proxy.fileUrl = url
	proxy.contentLength = size
	proxy.extensionWithDot = path.Ext(parsedUrl.Path)
	proxyFilename := WEB_PROXY + "proxy" + proxy.extensionWithDot
	proxyFile, err := os.OpenFile(proxyFilename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		LogError("Failed to open proxy file for writing: %v", err)
		return false
	}
	proxy.file = proxyFile
	proxy.downloadBeginOffset = -1
	proxy.contentRanges = make([]Range, 0)
	return true
}

func (server *Server) isTrustedUrl(url string, parsedUrl *net_url.URL) bool {
	if strings.HasPrefix(url, MEDIA) || strings.HasPrefix(url, serverRootAddress) {
		return true
	}

	if parsedUrl != nil && parsedUrl.Host == server.config.Domain {
		return true
	}

	return false
}

// setupDualTrackProxy for the time being will handle only 0-depth master playlists.
// returns (success, video proxy, audio proxy)
func setupDualTrackProxy(originalM3U *M3U, referer string) (bool, *HlsProxy, *HlsProxy) {
	prefix := stripLastSegmentStr(originalM3U.url)
	originalM3U.prefixRelativeTracks(*prefix)
	bestTrack := originalM3U.getTrackByVideoHeight(1080)
	if bestTrack == nil {
		return false, nil, nil
	}
	audioId := getParamValue("AUDIO", bestTrack.streamInfo)
	if audioId == "" {
		LogError("Best track's AUDIO param is empty, unable to match to audio.")
		return false, nil, nil
	}

	matchedAudio := false
	audioUrl := ""
	var audioRendition []Param
	for i := range originalM3U.audioRenditions {
		rendition := originalM3U.audioRenditions[i]
		groupId := getParamValue("GROUP-ID", rendition)
		if groupId != audioId {
			continue
		}
		matchedAudio = true
		audioUrl = getParamValue("URI", rendition)
		audioRendition = rendition

		audioDefault := getParamValue("DEFAULT", rendition)
		if audioDefault == "YES" {
			break
		}
		// YT hack: Look for original in audio track name
		audioName := getParamValue("NAME", rendition)
		if strings.Contains(audioName, "original") {
			break
		}
	}

	if !matchedAudio {
		LogError("No corresponding audio track found for audio id: %v", audioId)
		return false, nil, nil
	}

	LogInfo("Video URL: %v", bestTrack.url)
	LogInfo("Audio URL: %v", audioUrl)
	videoPrefix := stripLastSegmentStr(bestTrack.url)
	audioPrefix := stripLastSegmentStr(audioUrl)
	if videoPrefix == nil || audioPrefix == nil {
		LogError("Invalid segment prefix of either video or audio URL")
		return false, nil, nil
	}

	videoM3U, err := downloadM3U(bestTrack.url, WEB_PROXY+VIDEO_M3U8, referer)
	if err != nil {
		LogError("Failed to download m3u8 video track: %v", err.Error())
		return false, nil, nil
	}

	audioM3U, err := downloadM3U(audioUrl, WEB_PROXY+AUDIO_M3U8, referer)
	if err != nil {
		LogError("Failed to download m3u8 audio track: %v", err.Error())
		return false, nil, nil
	}

	if videoM3U.isMasterPlaylist || audioM3U.isMasterPlaylist {
		LogWarn("Unimplemented: Either video or audio is a master playlist")
		return false, nil, nil
	}

	if len(videoM3U.segments) == 0 || len(audioM3U.segments) == 0 {
		LogWarn("One of the playlists contains 0 segments")
		return false, nil, nil
	}

	videoM3U.prefixRelativeSegments(*videoPrefix)
	audioM3U.prefixRelativeSegments(*audioPrefix)

	// Check video encryption map uri
	if err = setupMapUri(&videoM3U.segments[0], referer, MEDIA_INIT_SECTION); err != nil {
		return false, nil, nil
	}

	vidProxy := setupVodProxy(videoM3U, WEB_PROXY+VIDEO_M3U8, referer, VIDEO_PREFIX)
	audioProxy := setupVodProxy(audioM3U, WEB_PROXY+AUDIO_M3U8, referer, AUDIO_PREFIX)
	// Craft proxied master playlist for the client
	originalM3U.tracks = originalM3U.tracks[:0]
	originalM3U.audioRenditions = originalM3U.audioRenditions[:0]
	uriParam := getParam("URI", audioRendition)
	uriParam.value = AUDIO_M3U8
	bestTrack.url = VIDEO_M3U8
	originalM3U.tracks = append(originalM3U.tracks, *bestTrack)
	originalM3U.audioRenditions = append(originalM3U.audioRenditions, audioRendition)

	originalM3U.serialize(WEB_PROXY + PROXY_M3U8)
	return true, vidProxy, audioProxy
}

func prepareMediaPlaylistFromMasterPlaylist(m3u *M3U, referer string, depth int) *M3U {
	if len(m3u.tracks) == 0 {
		LogError("Master playlist contains 0 tracks!")
		return nil
	}

	masterUrl, _ := net_url.Parse(m3u.url)
	prefix := stripLastSegment(masterUrl)
	LogInfo("A master playlist was provided. The best track will be chosen based on quality.")
	bestTrack := m3u.getBestTrack()
	bestUrl := bestTrack.url
	isRelative := !isAbsolute(bestUrl)
	if isRelative {
		bestUrl = prefixUrl(prefix, bestUrl)
	}

	var err error = nil
	m3u, err = downloadM3U(bestUrl, WEB_PROXY+ORIGINAL_M3U8, referer)

	if isRelative && isErrorStatus(err, 404) {
		// Sometimes non-compliant playlists contain URLs which are relative to the root domain
		domain := getRootDomain(masterUrl)
		bestUrl = prefixUrl(domain, bestTrack.url)
		m3u, err = downloadM3U(bestUrl, WEB_PROXY+ORIGINAL_M3U8, referer)
		if err != nil {
			LogError("Root domain fallback failed: %v", err.Error())
			return nil
		}
	} else if err != nil {
		LogError("Failed to fetch track from master playlist: %v", err.Error())
		return nil
	}

	// Master playlists can point to other master playlists
	if m3u.isMasterPlaylist {
		if depth < MAX_PLAYLIST_DEPTH {
			return prepareMediaPlaylistFromMasterPlaylist(m3u, referer, depth+1)
		} else {
			LogError("Exceeded maximum playlist depth of %v. Failed to get media playlist.", MAX_PLAYLIST_DEPTH)
			return nil
		}
	}

	return m3u
}

// TODO(kihau): More explicit error output messages.
func (server *Server) setupProxy(entry *Entry) error {
	urlStruct, err := net_url.Parse(entry.Url)
	if err != nil {
		return err
	}
	if isYoutubeUrl(entry.Url) || isTwitchUrl(entry.Url) {
		success := server.setupHlsProxy(entry.SourceUrl, "")
		if success {
			entry.ProxyUrl = PROXY_ROUTE + PROXY_M3U8
			LogInfo("HLS proxy setup for youtube was successful.")
		} else {
			return fmt.Errorf("HLS proxy setup for youtube failed!")
		}
	} else if entry.UseProxy {
		file := getBaseNoParams(urlStruct.Path)
		paramUrl := getParamUrl(urlStruct)
		if isPathM3U(file) || (paramUrl != nil && isPathM3U(getBaseNoParams(paramUrl.Path))) {
			setup := server.setupHlsProxy(entry.Url, entry.RefererUrl)
			if setup {
				entry.ProxyUrl = PROXY_ROUTE + PROXY_M3U8
				LogInfo("HLS proxy setup was successful.")
			} else {
				return fmt.Errorf("HLS proxy setup failed!")
			}
		} else {
			setup := server.setupGenericFileProxy(entry.Url, entry.RefererUrl)
			if setup {
				entry.ProxyUrl = PROXY_ROUTE + "proxy" + server.state.genericProxy.extensionWithDot
				LogInfo("Generic file proxy setup was successful.")
			} else {
				return fmt.Errorf("Generic file proxy setup failed!")
			}
		}
	}

	return nil
}

func (server *Server) setupHlsProxy(url string, referer string) bool {
	urlStruct, err := net_url.Parse(url)
	if err != nil {
		LogError("The provided URL is invalid: %v", err)
		return false
	}

	server.state.setupLock.Lock()
	defer server.state.setupLock.Unlock()
	start := time.Now()
	_ = os.RemoveAll(WEB_PROXY)
	_ = os.Mkdir(WEB_PROXY, os.ModePerm)
	var m3u *M3U
	if server.isTrustedUrl(url, urlStruct) {
		osPath := Conditional(isAbsolute(url), stripPathPrefix(urlStruct.Path, "watch"), url)
		m3u, err = parseM3U("web/" + osPath)
		m3u.url = url
	} else {
		m3u, err = downloadM3U(url, WEB_PROXY+ORIGINAL_M3U8, referer)
	}

	if err != nil {
		LogError("Failed to fetch m3u8: %v", err)
		return false
	}

	if m3u.isMasterPlaylist {
		if len(m3u.audioRenditions) > 0 {
			success, videoProxy, audioProxy := setupDualTrackProxy(m3u, referer)
			if success {
				server.state.isHls = true
				server.state.isLive = false
				server.state.proxy = videoProxy
				server.state.audioProxy = audioProxy
				duration := time.Since(start)
				LogDebug("Time taken to setup proxy: %v", duration)
			}
			return success
		}
		if m3u = prepareMediaPlaylistFromMasterPlaylist(m3u, referer, 0); m3u == nil {
			return false
		}
	}

	segmentCount := len(m3u.segments)
	duration := int(m3u.totalDuration())
	LogDebug("[Playlist info]")
	LogDebug("Type:     %v", m3u.getAttribute(EXT_X_PLAYLIST_TYPE))
	LogDebug("LIVE:     %v", m3u.isLive)
	LogDebug("Segments: %v", segmentCount)
	LogDebug("Duration: %vs", duration)

	if segmentCount == 0 {
		LogWarn("No segments found")
		return false
	}
	if duration > MAX_PLAYLIST_DURATION_SECONDS {
		LogWarn("Playlist exceeds max duration")
		return false
	}

	// Test if the first chunk is available and the source operates as intended (prevents broadcasting broken entries)
	if !server.validateOrRepointPlaylist(m3u, referer) {
		LogError("Chunk 0 was not available!")
		return false
	}

	// Check encryption map uri
	if err = setupMapUri(&m3u.segments[0], referer, MEDIA_INIT_SECTION); err != nil {
		return false
	}

	server.state.isHls = true
	server.state.isLive = m3u.isLive

	var newProxy *HlsProxy
	if m3u.isLive {
		newProxy = setupLiveProxy(url, referer)
	} else {
		newProxy = setupVodProxy(m3u, WEB_PROXY+PROXY_M3U8, referer, VIDEO_PREFIX)
	}
	server.state.proxy = newProxy
	setupDuration := time.Since(start)
	LogDebug("Time taken to setup proxy: %v", setupDuration)
	return true
}

func setupMapUri(segment *Segment, referer, fileName string) error {
	if segment.mapUri != "" {
		err := downloadFile(segment.mapUri, WEB_PROXY+fileName, referer, true)
		if err != nil {
			LogWarn("Failed to obtain map uri key: %v", err.Error())
			return err
		}
		segment.mapUri = fileName
	}
	return nil
}

func setupLiveProxy(liveUrl string, referer string) *HlsProxy {
	proxy := HlsProxy{}
	proxy.referer = referer
	proxy.liveUrl = liveUrl
	proxy.liveSegments.Clear()
	return &proxy
}

func setupVodProxy(m3u *M3U, osPath, referer, chunkPrefix string) *HlsProxy {
	proxy := HlsProxy{}
	segmentCount := len(m3u.segments)

	proxy.referer = referer
	proxy.chunkLocks = make([]sync.Mutex, segmentCount)
	proxy.fetchedChunks = make([]bool, segmentCount)
	proxy.originalChunks = make([]string, segmentCount)
	for i := range segmentCount {
		segment := &m3u.segments[i]
		proxy.originalChunks[i] = segment.url

		chunkName := chunkPrefix + toString(i)
		segment.url = chunkName
	}

	m3u.serialize(osPath)
	LogDebug("Prepared VOD proxy file.")
	return &proxy
}

var EXTM3U_BYTES = []byte("#EXTM3U")

// validateOrRepointPlaylist will also prefix segments appropriately or fail
func (server *Server) validateOrRepointPlaylist(m3u *M3U, referer string) bool {
	// VODs or LIVEs are expected
	if m3u.isMasterPlaylist {
		return false
	}

	url := m3u.segments[0].url
	if isAbsolute(url) {
		success, buffer := testGetResponse(url, referer)
		if !success {
			return false
		}
		if bytes.HasPrefix(buffer.Bytes(), EXTM3U_BYTES) {
			LogError("[RARE] Segment 0 is another media playlist")
			return false
		}
		return true
	}
	// From here segment URL is relative
	urlStruct, _ := net_url.Parse(m3u.url)
	prefix := stripLastSegment(urlStruct)
	url = prefixUrl(prefix, url)

	success, buffer := testGetResponse(url, referer)
	if success && !bytes.HasPrefix(buffer.Bytes(), EXTM3U_BYTES) {
		m3u.prefixRelativeSegments(prefix)
		return true
	}
	// If that has failed, the root domain can be tried
	root := getRootDomain(urlStruct)
	url = prefixUrl(root, m3u.segments[0].url)

	success, buffer = testGetResponse(url, referer)
	if success && !bytes.HasPrefix(buffer.Bytes(), EXTM3U_BYTES) {
		m3u.prefixRelativeSegments(root)
		LogDebug("Repointing segments to root=%v", root)
		return true
	}
	return false
}

var voiceClients = make(map[*websocket.Conn]bool)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for this example
	},
}

func voiceChat(writer http.ResponseWriter, request *http.Request) {
	LogInfo("Received request to voice chat")
	conn, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		LogError("Error on connection upgrade: %v", err)
		return
	}
	defer conn.Close()
	voiceClients[conn] = true
	LogInfo("Client %v connected!", conn.RemoteAddr())

	for {
		_, bytes, err := conn.ReadMessage()
		if err != nil {
			LogError("Error on ReadMessage(): %v", err)
			delete(voiceClients, conn)
			break
		}

		// Broadcast the received message to all clients
		for client := range voiceClients {
			if client.RemoteAddr() == conn.RemoteAddr() {
				continue
			}
			// Exclude the broadcasting client
			LogDebug("Writing bytes of len: %v to %v clients", len(bytes), len(voiceClients))
			if err := client.WriteMessage(websocket.BinaryMessage, bytes); err != nil {
				LogError("Error while writing message: %v", err)
				client.Close()
				delete(voiceClients, client)
			}
		}
	}
}

func (server *Server) watchProxy(writer http.ResponseWriter, request *http.Request) {
	urlPath := request.URL.Path
	chunk := path.Base(urlPath)

	server.state.setupLock.Lock()
	isHls := server.state.isHls
	isLive := server.state.isLive
	server.state.setupLock.Unlock()

	if isHls {
		if isLive {
			server.serveHlsLive(writer, request, chunk)
		} else {
			server.serveHlsVod(writer, request, chunk)
		}
	} else {
		server.serveGenericFile(writer, request, chunk)
	}
}

func (server *Server) watchStream(writer http.ResponseWriter, request *http.Request) {
	urlPath := request.URL.Path
	chunk := path.Base(urlPath)

	server.serveStream(writer, request, chunk)
}

func (server *Server) serveHlsLive(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")
	server.state.setupLock.Lock()
	proxy := server.state.proxy
	server.state.setupLock.Unlock()

	segmentMap := &proxy.liveSegments
	lastRefresh := &proxy.lastRefresh

	now := time.Now()
	if chunk == PROXY_M3U8 {
		cleanupSegmentMap(segmentMap)
		refreshedAgo := now.Sub(*lastRefresh)
		// Optimized to refresh at most once every 1.5 seconds
		if refreshedAgo.Seconds() < 1.5 {
			LogDebug("Serving unmodified %v", PROXY_M3U8)
			writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
			http.ServeFile(writer, request, WEB_PROXY+PROXY_M3U8)
			return
		}

		liveM3U, err := downloadM3U(proxy.liveUrl, WEB_PROXY+ORIGINAL_M3U8, proxy.referer)
		var downloadErr *DownloadError
		if errors.As(err, &downloadErr) {
			LogError("Download error of the live url [%v] %v", proxy.liveUrl, err.Error())
			http.Error(writer, downloadErr.Message, downloadErr.Code)
			return
		} else if err != nil {
			LogError("Failed to fetch live url: %v", err.Error())
			http.Error(writer, err.Error(), 500)
			return
		}

		if len(liveM3U.segments) == 0 {
			respondInternalError(writer, "No live segments received!")
			return
		}

		parsedUrl, _ := net_url.Parse(proxy.liveUrl)
		id := 0
		if mediaSequence := liveM3U.getAttribute(EXT_X_MEDIA_SEQUENCE); mediaSequence != "" {
			if sequenceId, err := parseInt(mediaSequence); err == nil {
				id = sequenceId
			}
		}

		// LogDebug("mediaSequence: %v", id)
		prefix := stripLastSegment(parsedUrl)
		liveM3U.prefixRelativeSegments(prefix)

		segmentCount := len(liveM3U.segments)
		for i := range segmentCount {
			segment := &liveM3U.segments[i]

			realUrl := segment.url
			segName := LIVE_PREFIX + toString(id)

			if _, exists := segmentMap.Load(segName); !exists {
				liveSegment := LiveSegment{realUrl: realUrl, realMapUri: segment.mapUri, created: time.Now()}
				segmentMap.Store(segName, &liveSegment)
				if segment.mapUri != "" {
					segment.mapUri = MIS_PREFIX + toString(id)
				}
			}

			segment.url = segName
			id++
		}

		liveM3U.serialize(WEB_PROXY + PROXY_M3U8)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, WEB_PROXY+PROXY_M3U8)
		return
	}

	if len(chunk) > MAX_CHUNK_NAME_LENGTH {
		http.Error(writer, "Not found", 404)
		return
	}

	if strings.HasPrefix(chunk, MIS_PREFIX) {
		fetchOrServeMediaInitSection(writer, request, chunk, segmentMap, proxy.referer)
		return
	}

	maybeChunk, found := segmentMap.Load(chunk)
	if !found {
		http.Error(writer, "Not found", 404)
		return
	}

	fetchedChunk := maybeChunk.(*LiveSegment)
	mutex := &fetchedChunk.mutex
	mutex.Lock()
	if fetchedChunk.obtainedUrl {
		mutex.Unlock()
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}
	fetchErr := downloadFile(fetchedChunk.realUrl, WEB_PROXY+chunk, proxy.referer, false)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch live chunk %v", fetchErr)
		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		}
		http.Error(writer, "Failed to fetch live chunk", code)
		return
	}
	fetchedChunk.obtainedUrl = true
	mutex.Unlock()

	http.ServeFile(writer, request, WEB_PROXY+chunk)
	return
}

func fetchOrServeMediaInitSection(writer http.ResponseWriter, request *http.Request, init string, segmentMap *sync.Map, referer string) {
	_, after, ok := strings.Cut(init, "-")
	if !ok || after == "" {
		http.Error(writer, "Bad media section", http.StatusBadRequest)
		return
	}
	id, err := parseInt64(after)
	if err != nil {
		http.Error(writer, "Bad media section", http.StatusBadRequest)
		return
	}
	segName := LIVE_PREFIX + int64ToString(id)
	maybeChunk, found := segmentMap.Load(segName)
	if !found {
		http.Error(writer, "Corresponding live segment not found", http.StatusNotFound)
		return
	}
	liveSegment := maybeChunk.(*LiveSegment)
	mutex := &liveSegment.mutex
	mutex.Lock()
	initKeyPath := WEB_PROXY + init
	if liveSegment.obtainedMapUri {
		mutex.Unlock()
		http.ServeFile(writer, request, initKeyPath)
		return
	}
	fetchErr := downloadFile(liveSegment.realMapUri, initKeyPath, referer, true)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch media init section %v", fetchErr)
		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		}
		http.Error(writer, "Failed to fetch media init section", code)
		return
	}
	liveSegment.obtainedMapUri = true
	mutex.Unlock()

	http.ServeFile(writer, request, initKeyPath)
}

func cleanupSegmentMap(segmentMap *sync.Map) {
	// Cleanup map - remove old entries to avoid memory leaks
	var keysToRemove []string
	now := time.Now()
	size := 0
	segmentMap.Range(func(key, value any) bool {
		fSegment := value.(*LiveSegment)
		age := now.Sub(fSegment.created)
		if age.Seconds() > 30 {
			keysToRemove = append(keysToRemove, key.(string))
		}
		size++
		// true continues iteration
		return true
	})

	// Remove the collected keys
	for _, key := range keysToRemove {
		segmentMap.Delete(key)
	}
}

func (server *Server) serveStream(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")

	if len(chunk) > MAX_CHUNK_NAME_LENGTH {
		http.Error(writer, "Not found", 404)
		return
	}

	if chunk == STREAM_M3U8 {
		LogDebug("Serving %v", STREAM_M3U8)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, WEB_STREAM+STREAM_M3U8)
		return
	}

	http.ServeFile(writer, request, WEB_STREAM+chunk)
}

func (server *Server) serveHlsVod(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")

	switch chunk {
	case PROXY_M3U8, VIDEO_M3U8, AUDIO_M3U8:
		LogDebug("Serving %v", chunk)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	case MEDIA_INIT_SECTION:
		LogDebug("Serving %v", chunk)
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}

	if len(chunk) > MAX_CHUNK_NAME_LENGTH || len(chunk) < len(VIDEO_PREFIX) {
		http.Error(writer, "Not found", 404)
		return
	}

	chunkId, err := strconv.Atoi(chunk[3:])
	if err != nil {
		http.Error(writer, "Chunk ID is not a number", 404)
		return
	}

	var proxy *HlsProxy
	server.state.setupLock.Lock()
	if strings.HasPrefix(chunk, VIDEO_PREFIX) {
		proxy = server.state.proxy
	} else if strings.HasPrefix(chunk, AUDIO_PREFIX) {
		proxy = server.state.audioProxy
	}
	server.state.setupLock.Unlock()
	if proxy != nil {
		serveHlsChunk(writer, request, proxy, chunk, chunkId)
	}
}

var chunkLogsite = Logsite{}

func serveHlsChunk(writer http.ResponseWriter, request *http.Request, proxy *HlsProxy, chunk string, chunkId int) {
	if chunkId < 0 || chunkId >= len(proxy.fetchedChunks) {
		http.Error(writer, "Chunk ID out of range", 404)
		return
	}

	mutex := &proxy.chunkLocks[chunkId]
	mutex.Lock()

	if proxy.fetchedChunks[chunkId] {
		mutex.Unlock()
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}

	destinationUrl := proxy.originalChunks[chunkId]
	fetchErr := downloadFile(destinationUrl, WEB_PROXY+chunk, proxy.referer, false)
	if fetchErr != nil {
		mutex.Unlock()
		if chunkLogsite.atMostEvery(time.Second) {
			LogError("Failed to fetch chunk %v from %v", fetchErr, destinationUrl)
		}

		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		} else {
			if downloadCode := getDownloadErrorCode(fetchErr); downloadCode != -1 {
				code = downloadCode
			}
		}
		http.Error(writer, "Failed to fetch vod chunk", code)
		return
	}
	proxy.fetchedChunks[chunkId] = true
	mutex.Unlock()

	http.ServeFile(writer, request, WEB_PROXY+chunk)
}

func (server *Server) serveGenericFile(writer http.ResponseWriter, request *http.Request, pathFile string) {
	proxy := &server.state.genericProxy
	if path.Ext(pathFile) != proxy.extensionWithDot {
		http.Error(writer, "Failed to fetch generic chunk", 404)
		return
	}

	rangeHeader := request.Header.Get("Range")
	if rangeHeader == "" {
		http.Error(writer, "Expected 'Range' header. No range was specified.", 400)
		return
	}
	byteRange, err := parseRangeHeader(rangeHeader, proxy.contentLength)
	if err != nil {
		LogInfo("Bad request: %v", err)
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	if byteRange.exceedsSize(proxy.contentLength) {
		http.Error(writer, "Range out of bounds", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	LogDebug("Serving proxied file at range [%v-%v] to %v", byteRange.start, byteRange.end, request.RemoteAddr)
	// If download offset is different from requested it's likely due to a seek and since everyone
	// should be in sync anyway we can terminate the existing download and create a new one.

	writer.Header().Set("Accept-Ranges", "bytes")
	writer.Header().Set("Content-Length", int64ToString(byteRange.length()))
	writer.Header().Set("Content-Range", byteRange.toContentRange(proxy.contentLength))

	response, err := openFileDownload(proxy.fileUrl, byteRange.start, proxy.referer)
	if err != nil {
		http.Error(writer, "Unable to open file download", 500)
		return
	}

	if request.Method == "HEAD" {
		writer.WriteHeader(http.StatusOK)
		return
	}

	writer.WriteHeader(http.StatusPartialContent)
	var totalWritten int64
	for {
		chunkBytes, err := pullBytesFromResponse(response, GENERIC_CHUNK_SIZE)
		if err != nil {
			LogError("An error occurred while pulling from source: %v", err)
			return
		}
		written, err := io.CopyN(writer, bytes.NewReader(chunkBytes), GENERIC_CHUNK_SIZE)
		totalWritten += written
		if err != nil {
			LogInfo("Connection %v terminated download having written %v bytes", request.RemoteAddr, totalWritten)
			return
		}
	}
}

// Could make it insert with merge
func (server *Server) insertContentRangeSequentially(newRange *Range) {
	proxy := &server.state.genericProxy
	spot := 0
	for i := range proxy.contentRanges {
		r := &proxy.contentRanges[i]
		if newRange.start <= r.start {
			break
		}
		spot++
	}
	proxy.contentRanges = slices.Insert(proxy.contentRanges, spot, *newRange)
}

func (server *Server) mergeContentRanges() {
	proxy := &server.state.genericProxy
	for i := range len(proxy.contentRanges) - 1 {
		leftRange := &proxy.contentRanges[i]
		rightRange := &proxy.contentRanges[i+1]
		exclusiveRange := newRange(0, leftRange.end+1)
		if !exclusiveRange.overlaps(rightRange) {
			continue
		}
		merge := leftRange.mergeWith(rightRange)
		// This removes the leftRange and rightRange and inserts merged range
		proxy.contentRanges = slices.Replace(proxy.contentRanges, i, i+2, merge)
	}
}

// this will prevent LAZY broadcasts when users make frequent updates
func (server *Server) smartSleep() {
	time.Sleep(BROADCAST_INTERVAL)
	for {
		now := time.Now()
		server.state.mutex.Lock()
		diff := now.Sub(server.state.lastUpdate)
		server.state.mutex.Unlock()

		if diff > BROADCAST_INTERVAL {
			break
		}
		time.Sleep(BROADCAST_INTERVAL - diff)
	}
}

func (server *Server) updatePlayerState(isPlaying bool, newTimestamp float64) {
	server.state.mutex.Lock()

	server.state.player.Playing = isPlaying
	server.state.player.Timestamp = newTimestamp
	server.state.lastUpdate = time.Now()

	server.state.mutex.Unlock()
}

func (server *Server) getCurrentTimestamp() float64 {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	var timestamp = server.state.player.Timestamp
	if server.state.player.Playing {
		now := time.Now()
		diff := now.Sub(server.state.lastUpdate)
		timestamp = server.state.player.Timestamp + diff.Seconds()
	}

	return timestamp
}

func (server *Server) createSyncEvent(action string, userId uint64) SyncEvent {
	timestamp := server.getCurrentTimestamp()

	event := SyncEvent{
		Timestamp: timestamp,
		Action:    action,
		UserId:    userId,
	}

	return event
}

func (server *Server) isLocalDirectory(url string) (bool, string) {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		return false, ""
	}

	if !server.isTrustedUrl(url, parsedUrl) {
		return false, ""
	}

	path := parsedUrl.Path

	if strings.HasPrefix(path, "/watch") {
		path = strings.TrimPrefix(path, "/watch")
	}

	if strings.HasPrefix(path, "/") {
		path = strings.TrimPrefix(path, "/")
	}

	if !filepath.IsLocal(path) {
		return false, ""
	}

	stat, err := os.Stat("./web/" + path)
	if err != nil {
		return false, ""
	}

	if !stat.IsDir() {
		return false, ""
	}

	path = filepath.Clean(path)
	LogDebug("PATH %v", path)

	return true, path
}

func (server *Server) getEntriesFromDirectory(path string, userId uint64) []Entry {
	entries := make([]Entry, 0)

	items, _ := os.ReadDir("./web/" + path)
	for _, item := range items {
		if !item.IsDir() {
			webpath := path + "/" + item.Name()
			url := net_url.URL{
				Path: webpath,
			}

			LogDebug("File URL: %v", url.String())

			entry := Entry{
				Id:        server.state.entryId.Add(1),
				Url:       url.String(),
				UserId:    userId,
				UseProxy:  false,
				Subtitles: []Subtitle{},
				Created:   time.Now(),
			}

			entry.Title = constructTitleWhenMissing(&entry)
			entries = append(entries, entry)
		}
	}

	return entries
}

func (server *Server) cleanupDummyUsers() []User {
	removed := []User{}

	server.users.mutex.Lock()
	defer server.users.mutex.Unlock()

	for _, user := range server.users.slice {
		if user.LastUpdate == user.CreatedAt && !user.Online {
			removed = append(removed, user)
		}
	}

	for _, user := range removed {
		server.users.removeByToken(user.token)
		DatabaseDeleteUser(server.db, user)
		server.writeEventToAllConnections("userdelete", user)
	}

	return removed
}

func (server *Server) constructEntry(entry Entry) Entry {
	if entry.Url == "" {
		return Entry{}
	}

	entry.Id = server.state.entryId.Add(1)
	entry.Title = constructTitleWhenMissing(&entry)
	entry.Created = time.Now()

	for i := range entry.Subtitles {
		entry.Subtitles[i].Id = server.state.subsId.Add(1)
	}

	return entry
}

func (server *Server) playerSeek(timestamp float64, userId uint64) {
	server.state.mutex.Lock()
	server.state.player.Timestamp = timestamp
	server.state.lastUpdate = time.Now()
	server.state.mutex.Unlock()

	event := server.createSyncEvent("seek", userId)
	server.writeEventToAllConnections("sync", event)
}

func (server *Server) playlistAdd(entry Entry, toTop bool) {
	newEntry := server.constructEntry(entry)
	if newEntry.Id == 0 {
		return
	}

	var event PlaylistEvent

	if toTop {
		playlist := make([]Entry, 0)
		playlist = append(playlist, newEntry)
		server.state.playlist = append(playlist, server.state.playlist...)
		event = createPlaylistEvent("addtop", newEntry)
	} else {
		server.state.playlist = append(server.state.playlist, newEntry)
		event = createPlaylistEvent("add", newEntry)
	}

	DatabasePlaylistAdd(server.db, newEntry)
	server.writeEventToAllConnections("playlist", event)
}

func (server *Server) playlistAddMany(entries []Entry, toTop bool) {
	if len(entries) == 0 {
		return
	}

	var event PlaylistEvent

	if toTop {
		server.state.playlist = append(entries, server.state.playlist...)
		event = createPlaylistEvent("addmanytop", entries)
	} else {
		server.state.playlist = append(server.state.playlist, entries...)
		event = createPlaylistEvent("addmany", entries)
	}

	DatabasePlaylistAddMany(server.db, entries)
	server.writeEventToAllConnections("playlist", event)
}

func (server *Server) playlistRemove(index int) Entry {
	entry := server.state.playlist[index]

	server.state.playlist = slices.Delete(server.state.playlist, index, index+1)
	DatabasePlaylistRemove(server.db, entry.Id)

	event := createPlaylistEvent("remove", entry.Id)
	server.writeEventToAllConnections("playlist", event)

	return entry
}

func compareEntries(entry1 Entry, entry2 Entry) bool {
	if entry1.Url != entry2.Url {
		return false
	}

	if entry1.Title != entry2.Title {
		return false
	}

	if entry1.UseProxy != entry2.UseProxy {
		return false
	}

	if entry1.RefererUrl != entry2.RefererUrl {
		return false
	}

	return true
}

func (server *Server) historyAdd(entry Entry) {
	newEntry := server.constructEntry(entry)
	if newEntry.Id == 0 {
		return
	}

	compareFunc := func(entry Entry) bool {
		return compareEntries(newEntry, entry)
	}

	index := slices.IndexFunc(server.state.history, compareFunc)
	if index != -1 {
		// De-duplicate history entries.
		removed := server.state.history[index]
		server.state.history = slices.Delete(server.state.history, index, index+1)
		DatabaseHistoryRemove(server.db, removed.Id)
		server.writeEventToAllConnections("historyremove", removed.Id)

		// Select newer subtitles from the new entry
		removed.Subtitles = newEntry.Subtitles
		newEntry = removed
	}

	server.state.history = append(server.state.history, newEntry)
	DatabaseHistoryAdd(server.db, newEntry)

	if len(server.state.history) > MAX_HISTORY_SIZE {
		removed := server.state.history[0]
		server.state.history = slices.Delete(server.state.history, 0, 1)
		DatabaseHistoryRemove(server.db, removed.Id)
	}

	server.writeEventToAllConnections("historyadd", newEntry)
}

func (server *Server) historyRemove(index int) Entry {
	entry := server.state.history[index]

	server.state.history = slices.Delete(server.state.history, index, index+1)
	DatabasePlaylistRemove(server.db, entry.Id)
	server.writeEventToAllConnections("historyremove", entry.Id)

	return entry
}
