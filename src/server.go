package main

import (
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
	"runtime/debug"
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

func createPlaylistEvent(action string, data any) PlaylistEventData {
	event := PlaylistEventData{
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

	server := Server{
		config: config,
		state: ServerState{
			playlist: make([]Entry, 0),
			history:  make([]Entry, 0),
			messages: make([]ChatMessage, 0),
		},
		users: users,
		conns: makeConnections(),
		db:    db,
	}

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

	var server_start_error error
	if !config.EnableSsl || missing_ssl_keys {
		serverRootAddress = "http://" + address
		LogWarn("Server is running in unencrypted http mode.")
		server_start_error = httpServer.ListenAndServe()
	} else {
		serverRootAddress = "https://" + address
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
	http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
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
	resource := strings.TrimPrefix(r.RequestURI, "/watch")
	LogDebug("Connection %s requested resource %v", r.RemoteAddr, resource)

	ip := strings.Split(r.RemoteAddr, ":")[0]

	cache.mapMutex.Lock()
	rateLimiter, exists := cache.ipToLimiters[ip]
	if exists {
		cache.mapMutex.Unlock()
		rateLimiter.mutex.Lock()
		if rateLimiter.block() {
			rateLimiter.update()
			rateLimiter.mutex.Unlock()
			respondTooManyRequests(w, ip, rateLimiter.getRetryAfter())
			return
		}
		rateLimiter.mutex.Unlock()
	} else {
		cache.ipToLimiters[ip] = NewLimiter(LIMITER_HITS, LIMITER_PER_SECOND)
		cache.mapMutex.Unlock()
	}

	// The no-cache directive does not prevent the storing of responses
	// but instead prevents the reuse of responses without revalidation.
	w.Header().Add("Cache-Control", "no-cache")
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

	server.HandleEndpoint(mux, "/watch/api/chat/send", server.apiChatSend, "POST", true)
	server.HandleEndpoint(mux, "/watch/api/chat/get", server.apiChatGet, "GET", true)

	// Server events and proxy.
	server.HandleEndpoint(mux, "/watch/api/events", server.apiEvents, "GET", false)

	server.HandleEndpoint(mux, PROXY_ROUTE, server.watchProxy, "GET", false)

	// Voice chat
	server.HandleEndpoint(mux, "/watch/vc", voiceChat, "GET", false)

	return mux
}

func (server *Server) HandleEndpoint(mux *http.ServeMux, endpoint string, endpointHandler func(w http.ResponseWriter, r *http.Request), method string, requireAuth bool) {
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// NOTE(kihau): A custom stack trace format could be interesting.
				stack := strings.TrimSpace(string(debug.Stack()))
				LogFatal("Panic in endpoint handler for %v serving %v: %v\n%v", endpoint, r.RemoteAddr, err, stack)
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

		endpointTrim := strings.TrimPrefix(endpoint, "/watch/api/")
		requested := strings.ReplaceAll(endpointTrim, "/", " ")
		LogInfo("Connection %s requested %v.", r.RemoteAddr, requested)
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

		var event SyncEventData
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

		server.writeEventToAllConnections(nil, "sync", event)
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

			// TODO(kihau): Instead of deleting, notify everyone that user is inactive?
			// else if time.Since(user.lastOnline) > time.Hour*24*14 && !user.Online {
			// 	// Archive users that were not active for more than two weeks.
			// 	LogInfo("Archiving user with id = %v due to 2 weeks of inactivity.", user.Id)
			//
			// 	server.inactiveUsers.add(user)
			// 	DatabaseArchiveUser(server.db, user)
			// 	toDelete = append(toDelete, user)
			// }
		}

		for _, user := range toDelete {
			user := server.users.removeByToken(user.token)
			server.writeEventToAllConnections(nil, "userdelete", user)
		}

		server.users.mutex.Unlock()
		time.Sleep(time.Hour * 24)
	}
}

func (server *Server) setNewEntry(newEntry *Entry) Entry {
	prevEntry := server.state.entry

	if prevEntry.Url != "" {
		server.state.history = append(server.state.history, prevEntry)
	}

	if isYoutubeUrl(newEntry.Url) {
		success := server.setupHlsProxy(newEntry.SourceUrl, "")
		if success {
			newEntry.SourceUrl = PROXY_ROUTE + PROXY_M3U8
			LogInfo("HLS proxy setup for youtube was successful.")
		} else {
			LogWarn("HLS proxy setup for youtube failed!")
		}
	} else if newEntry.UseProxy {
		paramUrl := ""
		if SCAN_QUERY_PARAMS {
			paramUrl = getParamUrl(newEntry.Url)
			if paramUrl != "" {
				LogDebug("Extracted param url: %v", paramUrl)
			}
		}

		lastSegment := strings.ToLower(lastUrlSegment(newEntry.Url))
		lastSegmentOfParam := strings.ToLower(lastUrlSegment(paramUrl))
		if isAnyUrlM3U(lastSegment, lastSegmentOfParam) {
			setup := server.setupHlsProxy(newEntry.Url, newEntry.RefererUrl)
			if setup {
				newEntry.SourceUrl = PROXY_ROUTE + PROXY_M3U8
				LogInfo("HLS proxy setup was successful.")
			} else {
				LogWarn("HLS proxy setup failed!")
			}
		} else {
			//setup := setupGenericFileProxy(newEntry.Url, newEntry.RefererUrl)
			setup := false
			if setup {
				newEntry.SourceUrl = PROXY_ROUTE + "proxy" + server.state.genericProxy.extensionWithDot
				LogInfo("Generic file proxy setup was successful.")
			} else {
				LogWarn("Generic file proxy setup failed!")
			}
		}
	}

	server.state.entry = *newEntry

	server.state.player.Timestamp = 0
	server.state.lastUpdate = time.Now()
	server.state.player.Playing = server.state.player.Autoplay

	return prevEntry
}

func isAnyUrlM3U(urls ...string) bool {
	for _, url := range urls {
		if strings.HasSuffix(url, ".m3u8") || strings.HasSuffix(url, ".m3u") {
			return true
		}
	}
	return false
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

func (server *Server) writeEventToAllConnections(origin http.ResponseWriter, eventName string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)
		if origin != nil {
			http.Error(origin, err.Error(), http.StatusInternalServerError)
		}
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

func (server *Server) writeEventToAllConnectionsExceptSelf(origin http.ResponseWriter, eventName string, data any, userId uint64, connectionId uint64) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)
		http.Error(origin, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonString := string(jsonData)

	eventId := server.state.eventId.Add(1)
	event := fmt.Sprintf("id: %v\nevent: %v\ndata: %v\nretry: %v\n\n", eventId, eventName, jsonString, RETRY)

	server.conns.mutex.Lock()
	for _, conn := range server.conns.slice {
		if userId == conn.userId && conn.id == connectionId {
			continue
		}

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

	size, err := getContentRange(url, referer)
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

type MasterPlaylistSetup struct {
	m3u    *M3U
	url    string
	prefix string
}

func (server *Server) setupMasterPlaylist(m3u *M3U, prefix string, referer string, parsedUrl *net_url.URL) *MasterPlaylistSetup {
	if len(m3u.tracks) == 0 {
		LogError("Master playlist contains 0 tracks!")
		return nil
	}

	// Rarely tracks are not fully qualified
	originalMasterPlaylist := m3u.copy()
	m3u.prefixRelativeTracks(prefix)
	LogInfo("A master playlist was provided. The best track will be chosen based on quality.")
	bestTrack := m3u.getBestTrack()

	resolution := getParamValue("RESOLUTION", bestTrack.streamInfo)
	if resolution != "" {
		LogDebug("The best track's resolution is %v", resolution)
	}

	var err error = nil
	m3u, err = downloadM3U(bestTrack.url, WEB_PROXY+ORIGINAL_M3U8, referer)

	if isErrorStatus(err, 404) {
		// Hacky trick for domain relative (non-compliant m3u8's 💩)
		domain := getRootDomain(parsedUrl)
		originalMasterPlaylist.prefixRelativeTracks(domain)
		bestTrack = originalMasterPlaylist.getBestTrack()
		m3u, err = downloadM3U(bestTrack.url, WEB_PROXY+ORIGINAL_M3U8, referer)
		if err != nil {
			LogError("Fallback failed: %v", err.Error())
			return nil
		}
	} else if err != nil {
		LogError("Failed to fetch track from master playlist: %v", err.Error())
		return nil
	}
	// Refreshing the prefix in case the newly assembled track consists of 2 or more components
	parsedUrl, err = net_url.Parse(bestTrack.url)
	if err != nil {
		LogError("Failed to parse URL from the best track, likely the segment is invalid: %v", err.Error())
		return nil
	}
	prefix = stripLastSegment(parsedUrl)
	return &MasterPlaylistSetup{m3u, bestTrack.url, prefix}
}

func (server *Server) setupHlsProxy(url string, referer string) bool {
	parsedUrl, err := net_url.Parse(url)
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
	if server.isTrustedUrl(url, parsedUrl) {
		lastSegment := lastUrlSegment(url)
		m3u, err = parseM3U(WEB_MEDIA + lastSegment)
	} else {
		m3u, err = downloadM3U(url, WEB_PROXY+ORIGINAL_M3U8, referer)
	}

	if err != nil {
		LogError("Failed to fetch m3u8: %v", err)
		return false
	}

	prefix := stripLastSegment(parsedUrl)
	if m3u.isMasterPlaylist {
		setupResult := server.setupMasterPlaylist(m3u, prefix, referer, parsedUrl)
		if setupResult == nil {
			return false
		}
		m3u = setupResult.m3u
		url = setupResult.url
		prefix = setupResult.prefix
	}

	segmentCount := len(m3u.segments)
	if segmentCount == 0 {
		LogWarn("No segments found")
		return false
	}

	// state.entry.Url = url
	// state.entry.RefererUrl = referer

	LogDebug("Playlist type: %v", m3u.getAttribute(EXT_X_PLAYLIST_TYPE))
	LogDebug("Max segment length: %vs", m3u.getAttribute(EXT_X_TARGETDURATION))
	LogDebug("isLive: %v", m3u.isLive)
	LogDebug("segments: %v", segmentCount)
	LogDebug("total duration: %vs", int(m3u.totalDuration()))

	// Test if the first chunk is available and the source operates as intended (prevents broadcasting broken entries)
	if !server.confirmSegment0Available(m3u, prefix, referer) {
		LogError("Chunk 0 was not available!")
		return false
	}
	// Sometimes m3u8 chunks are not fully qualified
	m3u.prefixRelativeSegments(prefix)

	server.state.isHls = true
	server.state.proxy = &HlsProxy{}
	var result bool
	if m3u.isLive {
		server.state.isLive = true
		result = server.setupLiveProxy(url, referer)
	} else {
		server.state.isLive = false
		result = server.setupVodProxy(m3u, referer)
	}
	duration := time.Since(start)
	LogDebug("Time taken to setup proxy: %v", duration)
	return result
}

func (server *Server) setupLiveProxy(liveUrl string, referer string) bool {
	proxy := server.state.proxy
	proxy.referer = referer
	proxy.liveUrl = liveUrl
	proxy.liveSegments.Clear()
	proxy.randomizer.Store(0)
	return true
}

func (server *Server) setupVodProxy(m3u *M3U, referer string) bool {
	proxy := server.state.proxy
	segmentCount := len(m3u.segments)

	proxy.referer = referer
	proxy.chunkLocks = make([]sync.Mutex, segmentCount)
	proxy.fetchedChunks = make([]bool, segmentCount)
	proxy.originalChunks = make([]string, 0, segmentCount)
	for i := range segmentCount {
		segment := &m3u.segments[i]
		proxy.originalChunks = append(proxy.originalChunks, segment.url)

		chunkName := "ch-" + toString(i)
		segment.url = chunkName
	}

	m3u.serialize(WEB_PROXY + PROXY_M3U8)
	LogDebug("Prepared VOD proxy file.")
	return true
}

func (server *Server) confirmSegment0Available(m3u *M3U, prefix, referer string) bool {
	// VODs are expected
	if m3u.isMasterPlaylist {
		return false
	}
	segment0 := m3u.segments[0]
	if !isAbsolute(segment0.url) {
		segment0.url = prefixUrl(prefix, segment0.url)
	}
	if segment0.mapUri != "" && !isAbsolute(segment0.mapUri) {
		segment0.url = prefixUrl(prefix, segment0.url)
	}
	return testGetResponse(segment0.url, referer)
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

// This endpoints should serve HLS chunks
// If the chunk is out of range or has no id, then 404 should be returned
// 1. Download m3u8 provided by a user
// 2. Serve a modified m3u8 to every user that wants to use a proxy
// 3. In memory use:
//   - 0-indexed string[] for original chunk URLs
//   - 0-indexed mutex[] to ensure the same chunk is not requested while it's being fetched
func (server *Server) watchProxy(writer http.ResponseWriter, request *http.Request) {
	urlPath := request.URL.Path
	chunk := path.Base(urlPath)

	if server.state.isHls {
		if server.state.isLive {
			server.serveHlsLive(writer, request, chunk)
		} else {
			server.serveHlsVod(writer, request, chunk)
		}
	} else {
		server.serveGenericFile(writer, request, chunk)
	}
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
		// Optimized to refresh only every 1.5 seconds
		if refreshedAgo.Seconds() < 1.5 {
			LogDebug("Serving unmodified %v", PROXY_M3U8)
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

		parsedUrl, err := net_url.Parse(proxy.liveUrl)
		if err != nil {
			LogError("This shouldn't have happened - failed to parse live url: %v", err.Error())
			http.Error(writer, err.Error(), 500)
			return
		}
		prefix := stripLastSegment(parsedUrl)
		liveM3U.prefixRelativeSegments(prefix)
		segmentCount := len(liveM3U.segments)
		for i := range segmentCount {
			segment := &liveM3U.segments[i]

			realUrl := segment.url
			fetched := FetchedSegment{realUrl, false, sync.Mutex{}, time.Now()}
			seed := proxy.randomizer.Add(1)
			segName := "live-" + int64ToString(seed)
			segmentMap.Store(segName, &fetched)

			segment.url = segName
		}

		liveM3U.serialize(WEB_PROXY + PROXY_M3U8)
		// LogDebug("Serving refreshed %v", PROXY_M3U8)
		http.ServeFile(writer, request, WEB_PROXY+PROXY_M3U8)
		return
	}

	if len(chunk) < 6 {
		http.Error(writer, "Not found", 404)
		return
	}

	maybeChunk, found := segmentMap.Load(chunk)
	if !found {
		http.Error(writer, "Not found", 404)
		return
	}

	fetchedChunk := maybeChunk.(*FetchedSegment)
	mutex := &fetchedChunk.mutex
	mutex.Lock()
	if fetchedChunk.obtained {
		mutex.Unlock()
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}
	fetchErr := downloadFile(fetchedChunk.realUrl, WEB_PROXY+chunk, proxy.referer)
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
	fetchedChunk.obtained = true
	mutex.Unlock()

	/*if len(keysToRemove) > 0 {
		LogDebug("Removed %v keys. Current map size: %v", len(keysToRemove), size)
	}*/

	http.ServeFile(writer, request, WEB_PROXY+chunk)
	return
}

func cleanupSegmentMap(segmentMap *sync.Map) {
	// Cleanup map - remove old entries to avoid memory leaks
	var keysToRemove []string
	now := time.Now()
	size := 0
	segmentMap.Range(func(key, value any) bool {
		fSegment := value.(*FetchedSegment)
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

func (server *Server) serveHlsVod(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")
	if chunk == PROXY_M3U8 {
		LogDebug("Serving %v", PROXY_M3U8)
		http.ServeFile(writer, request, WEB_PROXY+PROXY_M3U8)
		return
	}

	if len(chunk) < 4 {
		http.Error(writer, "Not found", 404)
		return
	}
	// Otherwise it's likely a proxy chunk which is 0-indexed
	chunkId, err := strconv.Atoi(chunk[3:])
	if err != nil {
		http.Error(writer, "Incorrect chunk ID", 404)
		return
	}

	server.state.setupLock.Lock()
	proxy := server.state.proxy
	server.state.setupLock.Unlock()
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

	fetchErr := downloadFile(proxy.originalChunks[chunkId], WEB_PROXY+chunk, proxy.referer)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch chunk %v", fetchErr)
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
		LogError("400 after parsing header")
		http.Error(writer, err.Error(), 400)
		return
	}
	if byteRange == nil || byteRange.start < 0 || byteRange.end < 0 {
		http.Error(writer, "Bad range", 400)
		return
	}

	if byteRange.start >= proxy.contentLength || byteRange.end >= proxy.contentLength {
		http.Error(writer, "Range out of bounds", 400)
		return
	}

	LogDebug("serveGenericFile() called at offset: %v", byteRange.start)
	// If download offset is different from requested it's likely due to a seek and since everyone
	// should be in sync anyway we can terminate the existing download and create a new one.
	proxy.downloadMutex.Lock()
	if proxy.downloadBeginOffset != byteRange.start {
		proxy.downloadBeginOffset = byteRange.start
		response, err := openFileDownload(proxy.fileUrl, byteRange.start, proxy.referer)
		if err != nil {
			http.Error(writer, "Unable to open file download", 500)
			return
		}
		proxy.download = response
		go server.downloadProxyFilePeriodically()
	}
	proxy.downloadMutex.Unlock()

	wroteHeaders := false
	offset := byteRange.start
	i := 0

	for {
		proxy.rangesMutex.Lock()
		if i >= len(proxy.contentRanges) {
			proxy.rangesMutex.Unlock()
			// Sleeps until offset becomes available
			i = 0
			time.Sleep(1 * time.Second)
			continue
		}
		contentRange := &proxy.contentRanges[i]
		proxy.rangesMutex.Unlock()
		i++

		if contentRange.includes(offset) && contentRange.length() >= GENERIC_CHUNK_SIZE {
			payload := make([]byte, GENERIC_CHUNK_SIZE)
			_, err := proxy.file.ReadAt(payload, offset)
			if err != nil {
				LogError("An error occurred while reading file from memory, %v", err)
				return
			}

			if !wroteHeaders {
				currentContentLength := proxy.contentLength - offset
				writer.Header().Set("Accept-Ranges", "bytes")
				writer.Header().Set("Content-Length", strconv.FormatInt(currentContentLength, 10))
				writer.Header().Set(
					"Content-Range",
					fmt.Sprintf("bytes=%v-%v/%v", offset, proxy.contentLength-1, proxy.contentLength))
				writer.WriteHeader(http.StatusPartialContent)
				wroteHeaders = true
			}

			_, err = writer.Write(payload)
			if err != nil {
				LogError("An error occurred while writing payload to user %v", err)
				return
			}
			LogInfo("Successfully wrote payload, range: %v - %v", offset, offset+GENERIC_CHUNK_SIZE)

			offset += GENERIC_CHUNK_SIZE
		}
	}
}

func (server *Server) downloadProxyFilePeriodically() {
	proxy := &server.state.genericProxy
	download := proxy.download
	offset := proxy.downloadBeginOffset
	id := generateUniqueId()
	LogInfo("Starting download (id: %v)", id)
	for {
		// Download periodically until the download is replaced pointing to a different object
		if proxy.download != download {
			_ = download.Body.Close()
			LogInfo("Terminating download (id: %v)", id)
			return
		}

		bytes, err := pullBytesFromResponse(download, GENERIC_CHUNK_SIZE)
		if err != nil {
			LogError("An error occurred while pulling from source: %v", err)
			return
		}

		_, err = proxy.file.WriteAt(bytes, offset)
		if err != nil {
			LogError("Error writing to file: %v", err)
			return
		}

		// The ranges should be checked before the download to avoid re-downloading
		// Probably open a new download so the entire coroutine should be controlling opening and closing
		proxy.rangesMutex.Lock()
		server.insertContentRangeSequentially(newRange(offset, offset+GENERIC_CHUNK_SIZE-1))
		server.mergeContentRanges()
		LogInfo("RANGES %v:", len(proxy.contentRanges))
		for i := range proxy.contentRanges {
			rang := &proxy.contentRanges[i]
			LogInfo("[%v] %v - %v", i, rang.start, rang.end)
		}
		proxy.rangesMutex.Unlock()

		offset += GENERIC_CHUNK_SIZE

		time.Sleep(5 * time.Second)
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

func (server *Server) createSyncEvent(action string, userId uint64) SyncEventData {
	server.state.mutex.Lock()
	var timestamp = server.state.player.Timestamp
	if server.state.player.Playing {
		now := time.Now()
		diff := now.Sub(server.state.lastUpdate)
		timestamp = server.state.player.Timestamp + diff.Seconds()
	}
	server.state.mutex.Unlock()

	event := SyncEventData{
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

			server.state.mutex.Lock()
			server.state.entryId += 1
			id := server.state.entryId
			server.state.mutex.Unlock()

			entry := Entry{
				Id:         id,
				Url:        url.String(),
				UserId:     userId,
				Title:      "",
				UseProxy:   false,
				RefererUrl: "",
				SourceUrl:  "",
				Subtitles:  []Subtitle{},
				Created:    time.Now(),
			}

			entry.Title = constructTitleWhenMissing(&entry)
			entries = append(entries, entry)
		}
	}

	return entries
}
