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
	"math/rand"
	"net"
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
	tokenBytes := make([]byte, TOKEN_LENGTH)
	_, err := cryptorand.Read(tokenBytes)

	if err != nil {
		LogError("Token generation failed, this should not happen!")
		return ""
	}

	return base64.URLEncoding.EncodeToString(tokenBytes)
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

func (users *Users) findById(userId uint64) int {
	for i, user := range users.slice {
		if user.Id == userId {
			return i
		}
	}

	return -1
}

func makeConnections() *Connections {
	conns := new(Connections)
	conns.slice = make([]Connection, 0)
	conns.idCounter = 1
	conns.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}
	return conns
}

func (conns *Connections) add(userId uint64) Connection {
	id := conns.idCounter
	conns.idCounter += 1

	conn := Connection{
		id:     id,
		userId: userId,
		events: make(chan []byte, 100),
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

// This method rotates the small array of recent actions, keeping the most recent one at the end and the oldest one at the start.
func (server *Server) addRecentAction(actionType string, userId uint64, data any) {
	action := Action{
		Action: actionType,
		UserId: userId,
		Data:   data,
		Date:   time.Now(),
	}

	if len(server.state.actions) < cap(server.state.actions) {
		server.state.actions = append(server.state.actions, action)
		return
	}

	for i := 0; i < len(server.state.actions)-1; i++ {
		server.state.actions[i] = server.state.actions[i+1]
	}

	last := len(server.state.actions) - 1
	server.state.actions[last] = action
}

func StartServer(config ServerConfig, db *sql.DB) {
	users, ok := DatabaseLoadUsers(db)
	if !ok {
		return
	}

	maxEntryId := DatabaseMaxEntryId(db)
	maxSubId := DatabaseMaxSubtitleId(db)
	maxMsgId := DatabaseMaxMessageId(db)

	history, _ := DatabaseHistoryGet(db)
	playlist, _ := DatabasePlaylistGet(db)
	messages, _ := DatabaseMessageGet(db, 10000, 0)

	autoplay := DatabaseGetAutoplay(db)
	looping := DatabaseGetLooping(db)

	server := Server{
		config: config,
		state: ServerState{
			entry:    Entry{},
			playlist: playlist,
			history:  history,
			messages: messages,
			player: PlayerState{
				Autoplay: autoplay,
				Looping:  looping,
			},

			actions: make([]Action, 0, 4),
		},

		users: users,
		conns: makeConnections(),
		db:    db,
	}

	server.state.subsId.Store(maxSubId)
	server.state.entryId.Store(maxEntryId)
	server.state.messageId = maxMsgId

	server.state.lastUpdate = time.Now()
	handler := registerEndpoints(&server)

	behindProxy = config.BehindProxy
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

func handleUnknownEndpoint(w http.ResponseWriter, r *http.Request) {
	LogWarn("User %v requested unknown endpoint: %v", getIp(r), r.RequestURI)
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
		LogDebug("%v exited black hole after waiting period of %v.", getIp(r), BLACK_HOLE_PERIOD)
	case <-context.Done():
		LogDebug("%v exited black hole due to cancellation client side after %v.", getIp(r), time.Since(start))
		return
	}
}

func serveFavicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, WEB_ROOT+"img/favicon.ico")
}

type GatewayHandler struct {
	fsHandler           http.Handler
	ipToLimiters        map[string]*RateLimiter
	mapMutex            *sync.Mutex
	blacklistedIpRanges []IpV4Range
	hits, perSecond     int
}

func NewGatewayHandler(fsHandler http.Handler, hits, perSecond int, ipv4Ranges []IpV4Range) GatewayHandler {
	return GatewayHandler{
		fsHandler:           fsHandler,
		ipToLimiters:        make(map[string]*RateLimiter),
		mapMutex:            &sync.Mutex{},
		blacklistedIpRanges: ipv4Ranges,
		hits:                hits,
		perSecond:           perSecond,
	}
}

func (cache GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := stripPort(getIp(r))
	resource := strings.TrimPrefix(r.RequestURI, PAGE_ROOT)

	for _, blacklistedRange := range cache.blacklistedIpRanges {
		if blacklistedRange.Contains(ip) {
			LogWarn("Blacklisted address %v attempted to access %v", ip, resource)
			blackholeRequest(r)
			http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
			return
		}
	}

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
		cache.ipToLimiters[ip] = NewLimiter(cache.hits, cache.perSecond)
		cache.mapMutex.Unlock()
	}

	LogDebug("Connection %s requested resource %v", getIp(r), resource)

	// The no-cache directive does not prevent the storing of responses
	// but instead prevents the reuse of responses without revalidation.
	w.Header().Add("Cache-Control", "no-cache")
	cache.fsHandler.ServeHTTP(w, r)
}

func registerEndpoints(server *Server) *http.ServeMux {
	mux := http.NewServeMux()
	server.registerRedirects(mux)

	ipv4Ranges := compileIpRanges(server.config.BlacklistedIpRanges)
	contentRootFs := http.FileServer(http.Dir(CONTENT_ROOT))
	contentFs := http.StripPrefix(CONTENT_ROUTE, contentRootFs)
	contentHandler := NewGatewayHandler(contentFs, CONTENT_LIMITER_HITS, CONTENT_LIMITER_PER_SECOND, ipv4Ranges)
	mux.Handle(CONTENT_ROUTE, contentHandler)

	webFs := http.StripPrefix(PAGE_ROOT, http.FileServer(http.Dir(WEB_ROOT)))
	staticHandler := NewGatewayHandler(webFs, STATIC_LIMITER_HITS, STATIC_LIMITER_PER_SECOND, ipv4Ranges)
	mux.Handle(PAGE_ROOT, staticHandler)

	mux.HandleFunc("/", handleUnknownEndpoint)
	mux.HandleFunc("/favicon.ico", serveFavicon)

	// Unrelated API calls.
	server.handleEndpoint(mux, "/api/version", server.apiVersion, "GET")
	server.handleEndpoint(mux, "/api/uptime", server.apiUptime, "GET")
	server.handleEndpoint(mux, "/api/login", server.apiLogin, "GET")
	server.handleEndpointAuthorized(mux, "/api/uploadmedia", server.apiUploadMedia, "POST")

	// User related API calls.
	server.handleEndpoint(mux, "/api/user/create", server.apiUserCreate, "GET")
	server.handleEndpoint(mux, "/api/user/verify", server.apiUserVerify, "POST")
	server.handleEndpoint(mux, "/api/user/delete", server.apiUserDelete, "POST")
	server.handleEndpointAuthorized(mux, "/api/user/getall", server.apiUserGetAll, "GET")
	server.handleEndpointAuthorized(mux, "/api/user/updatename", server.apiUserUpdateName, "POST")
	server.handleEndpointAuthorized(mux, "/api/user/updateavatar", server.apiUserUpdateAvatar, "POST")

	// API calls that change state of the player.
	server.handleEndpointAuthorized(mux, "/api/player/get", server.apiPlayerGet, "GET")
	server.handleEndpointAuthorized(mux, "/api/player/set", server.apiPlayerSet, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/next", server.apiPlayerNext, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/play", server.apiPlayerPlay, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/pause", server.apiPlayerPause, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/seek", server.apiPlayerSeek, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/autoplay", server.apiPlayerAutoplay, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/looping", server.apiPlayerLooping, "POST")
	server.handleEndpointAuthorized(mux, "/api/player/updatetitle", server.apiPlayerUpdateTitle, "POST")

	// Subtitle API calls.
	server.handleEndpointAuthorized(mux, "/api/subtitle/fetch", server.apiSubtitleFetch, "GET")
	server.handleEndpointAuthorized(mux, "/api/subtitle/delete", server.apiSubtitleDelete, "POST")
	server.handleEndpointAuthorized(mux, "/api/subtitle/update", server.apiSubtitleUpdate, "POST")
	server.handleEndpointAuthorized(mux, "/api/subtitle/attach", server.apiSubtitleAttach, "POST")
	server.handleEndpointAuthorized(mux, "/api/subtitle/shift", server.apiSubtitleShift, "POST")
	server.handleEndpointAuthorized(mux, "/api/subtitle/upload", server.apiSubtitleUpload, "POST")
	server.handleEndpointAuthorized(mux, "/api/subtitle/download", server.apiSubtitleDownload, "POST")
	server.handleEndpointAuthorized(mux, "/api/subtitle/search", server.apiSubtitleSearch, "POST")

	// API calls that change state of the playlist.
	server.handleEndpointAuthorized(mux, "/api/playlist/get", server.apiPlaylistGet, "GET")
	server.handleEndpointAuthorized(mux, "/api/playlist/play", server.apiPlaylistPlay, "POST")
	server.handleEndpointAuthorized(mux, "/api/playlist/add", server.apiPlaylistAdd, "POST")
	server.handleEndpointAuthorized(mux, "/api/playlist/clear", server.apiPlaylistClear, "POST")
	server.handleEndpointAuthorized(mux, "/api/playlist/delete", server.apiPlaylistDelete, "POST")
	server.handleEndpointAuthorized(mux, "/api/playlist/shuffle", server.apiPlaylistShuffle, "POST")
	server.handleEndpointAuthorized(mux, "/api/playlist/move", server.apiPlaylistMove, "POST")
	server.handleEndpointAuthorized(mux, "/api/playlist/update", server.apiPlaylistUpdate, "POST")

	// API calls that change state of the history.
	server.handleEndpointAuthorized(mux, "/api/history/get", server.apiHistoryGet, "GET")
	server.handleEndpointAuthorized(mux, "/api/history/clear", server.apiHistoryClear, "POST")
	server.handleEndpointAuthorized(mux, "/api/history/play", server.apiHistoryPlay, "POST")
	server.handleEndpointAuthorized(mux, "/api/history/delete", server.apiHistoryDelete, "POST")
	server.handleEndpointAuthorized(mux, "/api/history/playlistadd", server.apiHistoryPlaylistAdd, "POST")

	server.handleEndpointAuthorized(mux, "/api/chat/send", server.apiChatSend, "POST")
	server.handleEndpointAuthorized(mux, "/api/chat/edit", server.apiChatEdit, "POST")
	server.handleEndpointAuthorized(mux, "/api/chat/get", server.apiChatGet, "POST")
	server.handleEndpointAuthorized(mux, "/api/chat/delete", server.apiChatDelete, "POST")

	server.handleEndpointAuthorized(mux, "/api/stream/start", server.apiStreamStart, "POST")
	server.handleEndpointAuthorized(mux, "/api/stream/upload/{filename}", server.apiStreamUpload, "POST")
	// Server events and proxy.
	server.handleEndpoint(mux, "/api/events", server.apiEvents, "GET")

	server.handleEndpoint(mux, PROXY_ROUTE, server.watchProxy, "GET")
	server.handleEndpoint(mux, STREAM_ROUTE, server.watchStream, "GET")

	// Voice chat
	server.handleEndpoint(mux, "/vc", voiceChat, "GET")

	return mux
}

func compileIpRanges(ranges [][]string) []IpV4Range {
	v4Ranges := make([]IpV4Range, 0, len(ranges))
	for i := range ranges {
		twoIps := ranges[i]
		if len(twoIps) != 2 {
			LogError("An IPv4 range must contain exactly two IP addresses but %v were given", len(twoIps))
			os.Exit(1)
		}

		ipv4Range := newIpV4Range(twoIps[0], twoIps[1])
		if ipv4Range == nil {
			LogWarn("Ignoring invalid IPv4 range %v -> %v", twoIps[0], twoIps[1])
			continue
		}

		v4Ranges = append(v4Ranges, *ipv4Range)
	}
	LogDebug("Compiled %v IPv4 range(s)", len(v4Ranges))
	return v4Ranges
}

func (server *Server) registerRedirects(primaryMux *http.ServeMux) {
	config := server.config
	redirects := config.Redirects

	primaryPatterns := NewSet[string](len(redirects))
	var muxerList []MuxPortPatterns
	for _, redirect := range redirects {
		_, err := net_url.Parse(redirect.Location)
		if err != nil {
			LogError("Failed to parse redirect location %v", err)
			continue
		}

		isPrimary := redirect.Port == 0 || config.Port == redirect.Port

		newMux := false
		// Select the primary mux, an existing one or create a new one. Repeat for patterns
		var mux *http.ServeMux
		var patterns *Set[string]
		if isPrimary {
			mux = primaryMux
			patterns = primaryPatterns
		} else {
			index := slices.IndexFunc(muxerList, func(muxer MuxPortPatterns) bool {
				return muxer.Port == redirect.Port
			})
			if index == -1 {
				mux = http.NewServeMux()
				patterns = NewSet[string](8)
				newMux = true
			} else {
				mux = muxerList[index].Mux
				patterns = muxerList[index].Patterns
			}
		}

		if redirect.Location == "" {
			LogWarn("Redirect location is empty. It'll be treated as '/'")
			redirect.Location = "/"
		}

		pathPattern := redirect.Path
		if pathPattern == "" {
			LogWarn("Path pattern cannot be empty. It'll be treated as '/'")
			pathPattern = "/"
		}
		if patterns.Contains(pathPattern) {
			LogError("Pattern conflict! Redirect for %v is already registered. Skipping duplicate!", pathPattern)
			continue
		}
		patterns.Add(pathPattern)
		mux.HandleFunc(pathPattern, func(w http.ResponseWriter, r *http.Request) {
			LogInfo("Redirecting from %v -> %v", pathPattern, redirect.Location)
			http.Redirect(w, r, redirect.Location, http.StatusMovedPermanently)
		})
		if newMux {
			muxerList = append(muxerList, MuxPortPatterns{Mux: mux, Port: redirect.Port, Patterns: patterns})
		}
		LogInfo("Configured redirect %v -> %v", pathPattern, redirect.Location)
	}
	for _, muxer := range muxerList {
		var address = config.Address + ":" + toString(int(muxer.Port))
		redirectServer := http.Server{Addr: address, Handler: muxer.Mux}

		go func() {
			err := redirectServer.ListenAndServe()
			if err != nil {
				LogError("Failed to start the redirect server: %v", err)
			}
		}()
	}
}

// This method will fetch based on behindProxy
func getIp(req *http.Request) string {
	if !behindProxy {
		return req.RemoteAddr
	}

	if ip := req.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}

	if ip := req.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}

	return req.RemoteAddr
}

func stripPort(ip string) string {
	if len(ip) < 2 {
		return ip
	}
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		return ip
	}
	return host
}

func (server *Server) handleEndpointAuthorized(mux *http.ServeMux, endpoint string, endpointHandler func(w http.ResponseWriter, r *http.Request, userId uint64), method string) {
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				LogFatalUp(2, "Panic in endpoint handler for %v serving %v: %v", endpoint, getIp(r), err)
			}
		}()

		if r.Method != method {
			errMsg := fmt.Sprintf("Method not allowed. %v was expected.", method)
			http.Error(w, errMsg, http.StatusMethodNotAllowed)
			return
		}

		user := server.getAuthorized(w, r)
		if user == nil {
			return
		}

		endpointTrim := strings.TrimPrefix(endpoint, "/api/")
		requested := strings.ReplaceAll(endpointTrim, "/", " ")
		LogInfo("Connection %s requested %v.", getIp(r), requested)

		endpointHandler(w, r, user.Id)
	}

	mux.HandleFunc(endpoint, genericHandler)
}

func (server *Server) handleEndpoint(mux *http.ServeMux, endpoint string, endpointHandler func(w http.ResponseWriter, r *http.Request), method string) {
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				LogFatalUp(2, "Panic in endpoint handler for %v serving %v: %v", endpoint, getIp(r), err)
			}
		}()

		if r.Method != method {
			errMsg := fmt.Sprintf("Method not allowed. %v was expected.", method)
			http.Error(w, errMsg, http.StatusMethodNotAllowed)
			return
		}

		// NOTE(kihau): Hack to prevent console spam on proxy.
		if PROXY_ROUTE != endpoint && STREAM_ROUTE != endpoint {
			endpointTrim := strings.TrimPrefix(endpoint, "/api/")
			requested := strings.ReplaceAll(endpointTrim, "/", " ")
			LogInfo("Connection %s requested %v.", getIp(r), requested)
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

		if playing {
			event = server.createSyncEvent("play", 0)
		} else {
			event = server.createSyncEvent("pause", 0)
		}
		server.state.mutex.Unlock()

		server.writeEventToAllConnections("sync", event, SERVER_ID)
	}
}

func (server *Server) periodicInactiveUserCleanup() {
	for {
		server.users.mutex.Lock()

		toDelete := make([]User, 0)
		for _, user := range server.users.slice {
			if user.LastUpdate.Equal(user.CreatedAt) && time.Since(user.LastOnline) > time.Hour*24 && !user.Online {
				// Remove users that are not active and that have not updated their user profile.
				LogInfo("Removing dummy temp user with id = %v due to +24h of inactivity.", user.Id)

				DatabaseDeleteUser(server.db, user)
				toDelete = append(toDelete, user)
			}
		}

		for _, user := range toDelete {
			user := server.users.removeByToken(user.token)
			server.writeEventToAllConnections("userdelete", user, SERVER_ID)
		}

		server.users.mutex.Unlock()
		time.Sleep(time.Hour * 24)
	}
}

func (server *Server) setNewEntry(entry Entry, requested RequestEntry) {
	server.state.isLoading.Store(true)
	defer server.state.isLoading.Store(false)

	entry = server.constructEntry(entry)

	if isYoutubeEntry(entry, requested) {
		server.writeEventToAllConnections("playerwaiting", "Youtube video is loading. Please stand by!", SERVER_ID)

		err := loadYoutubeEntry(&entry, requested)
		if err != nil {
			server.writeEventToAllConnections("playererror", err.Error(), SERVER_ID)
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
	} else if isTwitchEntry(entry) {
		server.writeEventToAllConnections("playerwaiting", "Twitch stream is loading. Please stand by!", SERVER_ID)

		err := loadTwitchEntry(&entry)
		if err != nil {
			server.writeEventToAllConnections("playererror", err.Error(), SERVER_ID)
			return
		}
	}

	err := server.setupProxy(&entry)
	if err != nil {
		LogWarn("%v", err)
		server.writeEventToAllConnections("playererror", err.Error(), SERVER_ID)
		return
	}

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if server.state.player.Looping {
		server.playlistAddOne(server.state.entry, false)
	}

	server.historyAdd(server.state.entry)

	server.state.entry = entry
	DatabaseCurrentEntrySet(server.db, entry)

	server.state.player.Timestamp = 0
	server.state.lastUpdate = time.Now()
	server.state.player.Playing = server.state.player.Autoplay

	LogInfo("New entry URL is now: '%s'.", entry.Url)
	server.writeEventToAllConnections("playerset", entry, SERVER_ID)

	go server.preloadYoutubeSourceOnNextEntry()
}

func isPathM3U(p string) bool {
	return strings.HasSuffix(p, ".m3u8") || strings.HasSuffix(p, ".m3u") || strings.HasSuffix(p, ".txt")
}

// isAuthorized checks if the user is authorized, if not responds with an error code
func (server *Server) isAuthorized(w http.ResponseWriter, r *http.Request) bool {
	server.users.mutex.Lock()
	defer server.users.mutex.Unlock()

	index := server.getAuthorizedIndex(w, r)
	return index != -1
}

// getAuthorized returns the authorized user or responds with an error code if user wasn't found
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
		respondUnauthorized(w, "Failed to authorize user, no token provided")
		return -1
	}

	for i, user := range server.users.slice {
		if user.token == token {
			return i
		}
	}

	respondUnauthorized(w, "User with the specified token is not in the user list")
	return -1
}

func (server *Server) findUser(token string) *User {
	index := server.users.findByToken(token)
	if index == -1 {
		return nil
	}

	return &server.users.slice[index]
}

func (server *Server) findUserById(userId uint64) *User {
	index := server.users.findById(userId)
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
		respondBadRequest(w, "Failed to deserialize request body data: %v", err)
		return false
	}

	return true
}

func (server *Server) writeEventToOneConnection(eventType string, eventData any, conn Connection) {
	data := WebsocketEventResponse{
		Type:   eventType,
		Data:   eventData,
		UserId: 0,
	}

	event, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventType, err)
		return
	}

	select {
	case conn.events <- event:
	default:
		select {
		case conn.close <- true:
			LogWarn("Event queue for connection %v is full. Sending channel close...", conn.id)
		default:
		}
	}
}

func (server *Server) writeDataToOneConnection(eventType string, eventData []byte, conn Connection) {
	data := WebsocketEventResponse{
		Type:   eventType,
		Data:   eventData,
		UserId: 0,
	}

	event, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventType, err)
		return
	}

	select {
	case conn.events <- event:
	default:
		select {
		case conn.close <- true:
			LogWarn("Event queue for connection %v is full. Sending channel close...", conn.id)
		default:
		}
	}
}

func (server *Server) writeEventToAllConnections(eventType string, eventData any, userId uint64) {
	data := WebsocketEventResponse{
		Type:   eventType,
		Data:   eventData,
		UserId: userId,
	}

	event, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventType, err)
		return
	}

	server.conns.mutex.Lock()
	for _, conn := range server.conns.slice {
		select {
		case conn.events <- event:
		default:
			select {
			case conn.close <- true:
				LogWarn("Event queue for connection %v is full. Sending channel close...", conn.id)
			default:
			}
		}
	}
	server.conns.mutex.Unlock()
}

// It should be possible to use this list in a dropdown and attach to entry
func (server *Server) getSubtitles() []string {
	subtitles := make([]string, 0)
	subsFolder := CONTENT_MEDIA + "subs"
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
				subtitlePath := CONTENT_MEDIA + "subs/" + filename
				subtitles = append(subtitles, subtitlePath)
				break
			}
		}
	}

	LogInfo("Served subtitles: %v", subtitles)
	return subtitles
}

func (server *Server) setupGenericFileProxy(url string, referer string) bool {
	_ = os.RemoveAll(CONTENT_PROXY)
	_ = os.MkdirAll(CONTENT_PROXY, os.ModePerm)
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
	proxyFilename := CONTENT_PROXY + "proxy" + proxy.extensionWithDot
	proxyFile, err := os.OpenFile(proxyFilename, os.O_RDWR|os.O_CREATE, 0666)

	if err != nil {
		LogError("Failed to open proxy file for writing: %v", err)
		return false
	}

	proxy.file = proxyFile
	proxy.diskRanges = make([]Range, 0)
	LogInfo("Successfully setup proxy for file of size %v MB", formatMegabytes(size, 2))
	return true
}

func (server *Server) isTrustedUrl(url string, parsedUrl *net_url.URL) bool {
	if strings.HasPrefix(url, CONTENT_MEDIA) || strings.HasPrefix(url, serverRootAddress) {
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

	videoM3U, err := downloadM3U(bestTrack.url, CONTENT_PROXY+VIDEO_M3U8, referer)
	if err != nil {
		LogError("Failed to download m3u8 video track: %v", err.Error())
		return false, nil, nil
	}
	if !validateOrRepointPlaylist(videoM3U, referer) {
		LogError("Chunk 0 was not available in video m3u!")
		return false, nil, nil
	}
	if videoM3U.removeTrailingSegment(MIN_SEGMENT_LENGTH) {
		LogDebug("Removed trailing playlist segment in video track.")
	}

	audioM3U, err := downloadM3U(audioUrl, CONTENT_PROXY+AUDIO_M3U8, referer)
	if err != nil {
		LogError("Failed to download m3u8 audio track: %v", err.Error())
		return false, nil, nil
	}
	if audioM3U.removeTrailingSegment(MIN_SEGMENT_LENGTH) {
		LogDebug("Removed trailing playlist segment in audio track.")
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

	vidProxy := setupVodProxy(videoM3U, CONTENT_PROXY+VIDEO_M3U8, referer, VIDEO_PREFIX)
	audioProxy := setupVodProxy(audioM3U, CONTENT_PROXY+AUDIO_M3U8, referer, AUDIO_PREFIX)
	// Craft proxied master playlist for the client
	originalM3U.tracks = originalM3U.tracks[:0]
	originalM3U.audioRenditions = originalM3U.audioRenditions[:0]
	uriParam := getParam("URI", audioRendition)
	uriParam.value = AUDIO_M3U8
	bestTrack.url = VIDEO_M3U8
	originalM3U.tracks = append(originalM3U.tracks, *bestTrack)
	originalM3U.audioRenditions = append(originalM3U.audioRenditions, audioRendition)

	originalM3U.serialize(CONTENT_PROXY + PROXY_M3U8)
	return true, vidProxy, audioProxy
}

func ytAudioFilter(track *Track) bool {
	urlStruct, err := net_url.Parse(track.url)
	if err != nil {
		return false
	}
	return strings.Contains(urlStruct.Path, "acont=original")
}

func prepareMediaPlaylistFromMasterPlaylist(m3u *M3U, referer string, depth int) *M3U {
	if len(m3u.tracks) == 0 {
		LogError("Master playlist contains 0 tracks!")
		return nil
	}

	masterUrl, _ := net_url.Parse(m3u.url)
	prefix := stripLastSegment(masterUrl)
	LogInfo("A master playlist was provided. The best track will be chosen based on quality.")
	var bestTrack *Track
	if masterUrl.Host == "manifest.googlevideo.com" {
		bestTrack = m3u.getBestTrack(ytAudioFilter)
	} else {
		bestTrack = m3u.getBestTrack(nil)
	}

	bestUrl := bestTrack.url
	isRelative := !isAbsolute(bestUrl)
	if isRelative {
		bestUrl = prefixUrl(prefix, bestUrl)
	}

	var err error = nil
	m3u, err = downloadM3U(bestUrl, CONTENT_PROXY+ORIGINAL_M3U8, referer)

	if isRelative && isErrorStatus(err, 404) {
		// Sometimes non-compliant playlists contain URLs which are relative to the root domain
		domain := getRootDomain(masterUrl)
		bestUrl = prefixUrl(domain, bestTrack.url)
		m3u, err = downloadM3U(bestUrl, CONTENT_PROXY+ORIGINAL_M3U8, referer)
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
	// This should be moved to some destructEntry() / unloadEntry() method
	proxy := &server.state.genericProxy
	server.state.setupLock.Lock()
	if proxy.file != nil {
		proxy.fileMutex.Lock()
		_ = proxy.file.Close()
		proxy.fileMutex.Unlock()
	}
	server.state.setupLock.Unlock()

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
	_ = os.RemoveAll(CONTENT_PROXY)
	_ = os.MkdirAll(CONTENT_PROXY, os.ModePerm)
	var m3u *M3U
	if server.isTrustedUrl(url, urlStruct) {
		osPath := Conditional(isAbsolute(url), stripPathPrefix(urlStruct.Path, PAGE_ROOT), url)
		m3u, err = parseM3U(CONTENT_ROOT + osPath)
		if err != nil {
			LogError("Failed to parse m3u8: %v", err)
			return false
		}
		m3u.url = url
	} else {
		m3u, err = downloadM3U(url, CONTENT_PROXY+ORIGINAL_M3U8, referer)
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

	if m3u.removeTrailingSegment(MIN_SEGMENT_LENGTH) {
		LogDebug("Removed trailing playlist segment.")
	}

	if segmentCount == 0 {
		LogWarn("No segments found")
		return false
	}
	if duration > MAX_PLAYLIST_DURATION_SECONDS {
		LogWarn("Playlist exceeds max duration")
		return false
	}

	// Test if the first chunk is available and the source operates as intended (prevents broadcasting broken entries)
	if !validateOrRepointPlaylist(m3u, referer) {
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
		newProxy = setupVodProxy(m3u, CONTENT_PROXY+PROXY_M3U8, referer, VIDEO_PREFIX)
	}
	server.state.proxy = newProxy
	setupDuration := time.Since(start)
	LogDebug("Time taken to setup proxy: %v", setupDuration)
	return true
}

func setupMapUri(segment *Segment, referer, fileName string) error {
	if segment.mapUri != "" {
		err := downloadFile(segment.mapUri, CONTENT_PROXY+fileName, &DownloadOptions{referer: referer, hasty: true})
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
func validateOrRepointPlaylist(m3u *M3U, referer string) bool {
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
			http.ServeFile(writer, request, CONTENT_PROXY+PROXY_M3U8)
			return
		}

		liveM3U, err := downloadM3U(proxy.liveUrl, CONTENT_PROXY+ORIGINAL_M3U8, proxy.referer)
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

		liveM3U.serialize(CONTENT_PROXY + PROXY_M3U8)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, CONTENT_PROXY+PROXY_M3U8)
		return
	}

	if !server.isAuthorized(writer, request) {
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
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	}

	options := &DownloadOptions{
		referer:   proxy.referer,
		hasty:     false,
		bodyLimit: MAX_CHUNK_SIZE,
	}
	fetchErr := downloadFile(fetchedChunk.realUrl, CONTENT_PROXY+chunk, options)
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
	http.ServeFile(writer, request, CONTENT_PROXY+chunk)
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
	initKeyPath := CONTENT_PROXY + init
	if liveSegment.obtainedMapUri {
		mutex.Unlock()
		http.ServeFile(writer, request, initKeyPath)
		return
	}
	options := &DownloadOptions{
		referer:   referer,
		hasty:     true,
		bodyLimit: MAX_CHUNK_SIZE,
	}
	fetchErr := downloadFile(liveSegment.realMapUri, initKeyPath, options)
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

	if chunk == STREAM_M3U8 {
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, CONTENT_STREAM+STREAM_M3U8)
		return
	}

	if !server.isAuthorized(writer, request) {
		return
	}

	if len(chunk) > MAX_CHUNK_NAME_LENGTH {
		http.Error(writer, "Not found", 404)
		return
	}

	http.ServeFile(writer, request, CONTENT_STREAM+chunk)
}

func (server *Server) serveHlsVod(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")

	switch chunk {
	case PROXY_M3U8, VIDEO_M3U8, AUDIO_M3U8:
		LogDebug("Serving %v", chunk)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	case MEDIA_INIT_SECTION:
		LogDebug("Serving %v", chunk)
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	}

	if !server.isAuthorized(writer, request) {
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
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	}

	destinationUrl := proxy.originalChunks[chunkId]
	options := &DownloadOptions{
		referer:   proxy.referer,
		hasty:     false,
		bodyLimit: MAX_CHUNK_SIZE,
	}
	fetchErr := downloadFile(destinationUrl, CONTENT_PROXY+chunk, options)
	if fetchErr != nil {
		mutex.Unlock()
		if chunkLogsite.atMostEvery(time.Second) {
			LogError("Failed to fetch chunk #%v due to %v from %v", chunkId, fetchErr, destinationUrl)
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

	http.ServeFile(writer, request, CONTENT_PROXY+chunk)
}

func (server *Server) serveGenericFileNaive(writer http.ResponseWriter, request *http.Request, pathFile string) {
	proxy := &server.state.genericProxy
	if path.Ext(pathFile) != proxy.extensionWithDot {
		http.Error(writer, "Extension is different from the proxied file", 404)
		return
	}

	byteRange, ok := ensureRangeHeader(writer, request, proxy.contentLength)
	if !ok {
		return
	}

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
			megabytes := formatMegabytes(totalWritten, 2)
			LogInfo("Connection %v terminated having downloaded %v MB", getIp(request), megabytes)
			return
		}
	}
}

func (server *Server) serveGenericFile(writer http.ResponseWriter, request *http.Request, pathFile string) {
	proxy := &server.state.genericProxy
	if path.Ext(pathFile) != proxy.extensionWithDot {
		http.Error(writer, "Extension is different from the proxied file", 404)
		return
	}

	byteRange, ok := ensureRangeHeader(writer, request, proxy.contentLength)
	if !ok {
		return
	}

	LogDebug("Connection %v requested proxied file at range %v", getIp(request), &byteRange)

	writer.Header().Set("Accept-Ranges", "bytes")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("Content-Length", int64ToString(byteRange.length()))
	writer.Header().Set("Content-Range", byteRange.toContentRange(proxy.contentLength))
	writer.Header().Set("Content-Type", "video/"+proxy.extensionWithDot[1:])

	if request.Method == "HEAD" {
		writer.WriteHeader(http.StatusOK)
		return
	}

	writer.WriteHeader(http.StatusPartialContent)

	// Debug VIEW --- start
	view := strings.Builder{}
	proxy.rangeMutex.Lock()
	for _, diskRange := range proxy.diskRanges {
		view.WriteString(diskRange.String())
		view.WriteString(", ")
	}
	LogDebug("Disk ranges: %v", view.String())
	proxy.rangeMutex.Unlock()
	// Debug VIEW --- end

	var totalWritten int64
	nextRange := byteRange
	nextRange.end = byteRange.start + GENERIC_CHUNK_SIZE - 1

	for {
		if nextRange.end >= proxy.contentLength {
			nextRange.end = proxy.contentLength - 1
		}

		proxy.rangeMutex.Lock()
		actionableRanges := proxy.determineRangeActions(nextRange)
		proxy.rangeMutex.Unlock()

		actionView := strings.Builder{}
		for _, ar := range actionableRanges {
			actionView.WriteString(ar.actionName() + "=" + ar.r.String())
			actionView.WriteString(", ")
		}
		LogDebug("Connection %v has actions: %v", getIp(request), actionView.String())

		for _, aRange := range actionableRanges {
			if isRequestDone(request) {
				LogDebug("Connection %v closed", getIp(request))
				return
			}
			switch aRange.action {
			case READ:
				if !proxy.serveRangeFromDisk(writer, request, aRange, &totalWritten) {
					return
				}
			case AWAIT:
				<-aRange.future.ready
				if !aRange.future.success {
					LogWarn("Connection %v awaited range %v but it ended in failure", getIp(request), &aRange.r)
					return
				}
				LogDebug("Connection %v is being served at %v from disk after await", getIp(request), &aRange.r)
				if !proxy.serveRangeFromDisk(writer, request, aRange, &totalWritten) {
					return
				}
			case FETCH:
				length := aRange.r.length()
				proxy.rangeMutex.Lock()
				future := proxy.newFutureRange(aRange.r)
				proxy.futureRanges = append(proxy.futureRanges, &future)
				proxy.rangeMutex.Unlock()

				response, err := openFileDownload(proxy.fileUrl, aRange.r.start, proxy.referer)
				if err != nil {
					close(future.ready)
					LogError("Unable to open file download: %v", err)
					return
				}
				chunkBytes, err := pullBytesFromResponse(response, int(length))
				if err != nil {
					close(future.ready)
					LogError("An error occurred while pulling from source: %v", err)
					return
				}
				response.Body.Close()

				proxy.rangeMutex.Lock()
				proxy.removeFutureRange(future)
				proxy.fileMutex.Lock()
				_, err = writeAtOffset(proxy.file, aRange.r.start, chunkBytes)
				if err != nil {
					proxy.fileMutex.Unlock()
					proxy.rangeMutex.Unlock()
					close(future.ready)
					LogError("An error occurred while writing to destination: %v", err)
					return
				}
				proxy.fileMutex.Unlock()
				future.success = true
				close(future.ready)
				proxy.diskRanges = incorporateRange(&nextRange, proxy.diskRanges)
				proxy.rangeMutex.Unlock()

				written, err := io.CopyN(writer, bytes.NewReader(chunkBytes), length)
				totalWritten += written
				if err != nil {
					megabytes := formatMegabytes(totalWritten, 2)
					LogInfo("Connection %v terminated having downloaded %v MB", getIp(request), megabytes)
					return
				}
			}
		}

		if nextRange.end == proxy.contentLength-1 {
			megabytes := formatMegabytes(totalWritten, 2)
			LogInfo("Connection %v fully served having downloaded %v MB", getIp(request), megabytes)
			return
		}
		nextRange.shift(GENERIC_CHUNK_SIZE)
	}
}

func (proxy *GenericProxy) determineRangeActions(nextRange Range) []*ActionableRange {
	var ranges []*ActionableRange

	fullFetch := true
	for _, diskRange := range proxy.diskRanges {
		commonPart, intersect := diskRange.intersection(&nextRange)
		if !intersect {
			continue
		}

		difference := nextRange.difference(&diskRange)
		if len(difference) == 0 {
			return []*ActionableRange{{action: READ, r: nextRange}}
		}
		if len(difference) == 1 {
			diff := difference[0]
			if diff.start == nextRange.start {
				ranges = append(ranges,
					&ActionableRange{action: FETCH, r: diff},
					&ActionableRange{action: READ, r: commonPart},
				)
			} else {
				ranges = append(ranges,
					&ActionableRange{action: READ, r: commonPart},
					&ActionableRange{action: FETCH, r: diff},
				)
			}
			fullFetch = false
			break
		}
		if len(difference) == 2 {
			// Assume ranges cannot be requested in larger parts so the difference is always singular or none
			LogFatal("Difference of two ranges shouldn't be possible with the current setup!")
			break
		}
	}
	if fullFetch {
		ranges = append(ranges, &ActionableRange{action: FETCH, r: nextRange})
	}

	for i := 0; i < len(ranges); i++ {
		aRange := ranges[i]
		if aRange.action != FETCH {
			continue
		}
		for _, futureRange := range proxy.futureRanges {
			if !futureRange.r.overlaps(&aRange.r) {
				continue
			}
			missingDiff := aRange.r.difference(&futureRange.r)
			if len(missingDiff) == 0 {
				LogDebug("Converting FETCH to AWAIT for range %v", &aRange.r)
				aRange.action = AWAIT
				aRange.future = futureRange
				break
			}

			if len(missingDiff) == 1 {
				diff := missingDiff[0]
				inProgress := &ActionableRange{action: AWAIT, r: futureRange.r, future: futureRange}
				if diff.start == aRange.r.start {
					LogDebug("Missing diff %v is at start", &diff)
					ranges = slices.Insert(ranges, i+1, inProgress)
				} else {
					LogDebug("Missing diff %v is at end", &diff)
					ranges = slices.Insert(ranges, i, inProgress)
					i++
				}
				aRange.r = diff
				break
			}
			if len(missingDiff) == 2 {
				LogWarn("Missing diff is %v & %v", &missingDiff[0], &missingDiff[1])
				// It'd be possible to request separate ranges from the source at once
				// For now leave as is
			}
		}
	}
	return ranges
}

func (proxy *GenericProxy) serveRangeFromDisk(writer http.ResponseWriter, request *http.Request, aRange *ActionableRange, totalWritten *int64) bool {
	length := aRange.r.length()
	proxy.fileMutex.Lock()
	rangeBytes, err := readAtOffset(proxy.file, aRange.r.start, int(length))
	if err != nil {
		proxy.fileMutex.Unlock()
		LogInfo("Unable to serve bytes at %v: %v", aRange.r.start, err)
		return false
	}
	proxy.fileMutex.Unlock()

	written, err := io.CopyN(writer, bytes.NewReader(rangeBytes), length)
	*totalWritten += written
	if err != nil {
		megabytes := formatMegabytes(*totalWritten, 2)
		LogInfo("Connection %v terminated having downloaded %v MB", getIp(request), megabytes)
		return false
	}
	return true
}

func (proxy *GenericProxy) removeFutureRange(future FutureRange) {
	index := -1
	for i, r := range proxy.futureRanges {
		if r.id == future.id {
			index = i
			break
		}
	}
	if index == -1 {
		LogInfo("FutureRange %v was not found", future)
		return
	}
	proxy.futureRanges = slices.Delete(proxy.futureRanges, index, index+1)
}

func (proxy *GenericProxy) newFutureRange(r Range) FutureRange {
	return FutureRange{
		id:    proxy.rangeSeed.Add(1),
		ready: make(chan struct{}),
		r:     r,
	}
}

func (a *ActionableRange) actionName() string {
	switch a.action {
	case READ:
		return "READ"
	case AWAIT:
		return "AWAIT"
	case FETCH:
		return "FETCH"
	}
	return ""
}

func ensureRangeHeader(writer http.ResponseWriter, request *http.Request, contentLength int64) (Range, bool) {
	rangeHeader := request.Header.Get("Range")
	if rangeHeader == "" {
		http.Error(writer, "Expected 'Range' header. No range was specified.", 400)
		return NO_RANGE, false
	}
	byteRange, err := parseRangeHeader(rangeHeader, contentLength)
	if err != nil {
		LogInfo("Bad request: %v", err)
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return NO_RANGE, false
	}

	if byteRange.exceedsSize(contentLength) {
		http.Error(writer, "Range out of bounds", http.StatusRequestedRangeNotSatisfiable)
		return NO_RANGE, false
	}
	return *byteRange, true
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

func (server *Server) playerPlay(timestamp float64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	return server.playerUpdateState(true, timestamp, userId)
}

func (server *Server) playerPause(timestamp float64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	return server.playerUpdateState(false, timestamp, userId)
}

func (server *Server) playerUpdateState(isPlaying bool, newTimestamp float64, userId uint64) error {
	server.state.player.Playing = isPlaying
	server.state.player.Timestamp = newTimestamp
	server.state.lastUpdate = time.Now()

	var event SyncEvent
	if isPlaying {
		event = server.createSyncEvent("play", userId)
	} else {
		event = server.createSyncEvent("pause", userId)
	}

	server.addRecentAction(event.Action, event.UserId, event.Timestamp)
	server.writeEventToAllConnections("sync", event, userId)

	return nil
}

func (server *Server) getCurrentTimestamp() float64 {
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

	urlPath := parsedUrl.Path

	if after, ok := strings.CutPrefix(urlPath, PAGE_ROOT); ok {
		urlPath = after
	}

	if after, ok := strings.CutPrefix(urlPath, "/"); ok {
		urlPath = after
	}

	if !filepath.IsLocal(urlPath) {
		return false, ""
	}

	dir := path.Join(CONTENT_ROOT, urlPath)
	stat, err := os.Stat(dir)
	if err != nil {
		return false, ""
	}

	if !stat.IsDir() {
		return false, ""
	}

	urlPath = filepath.Clean(urlPath)
	LogDebug("PATH %v", urlPath)

	return true, urlPath
}

func (server *Server) getEntriesFromDirectory(dir string, userId uint64) []Entry {
	entries := make([]Entry, 0)

	contentPath := path.Join(CONTENT_ROOT, dir)
	items, _ := os.ReadDir(contentPath)

	now := time.Now()
	for _, item := range items {
		if !item.IsDir() {
			webpath := dir + "/" + item.Name()
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
				CreatedAt: now,
				LastSetAt: now,
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
		if user.LastUpdate.Equal(user.CreatedAt) && !user.Online {
			removed = append(removed, user)
		}
	}

	for _, user := range removed {
		server.users.removeByToken(user.token)
		DatabaseDeleteUser(server.db, user)
		server.writeEventToAllConnections("userdelete", user, SERVER_ID)
	}

	return removed
}

func (server *Server) constructEntry(entry Entry) Entry {
	entry.Url = strings.TrimSpace(entry.Url)

	now := time.Now()
	if entry.Url == "" {
		entry := Entry{
			UserId:    entry.UserId,
			CreatedAt: now,
			LastSetAt: now,
		}

		return entry
	}

	entry.Id = server.state.entryId.Add(1)
	entry.Title = constructTitleWhenMissing(&entry)
	entry.LastSetAt = now

	for i := range entry.Subtitles {
		entry.Subtitles[i].Id = server.state.subsId.Add(1)
	}

	return entry
}

func (server *Server) playerGet() PlayerGetResponse {
	server.state.mutex.Lock()
	player := server.state.player
	entry := server.state.entry
	server.state.mutex.Unlock()

	player.Timestamp = server.getCurrentTimestamp()

	server.conns.mutex.Lock()
	actions := make([]Action, len(server.state.actions))
	copy(actions, server.state.actions)
	server.conns.mutex.Unlock()

	data := PlayerGetResponse{
		Player:  player,
		Entry:   entry,
		Actions: actions,
	}

	return data
}

func (server *Server) playerSet(requested RequestEntry, userId uint64) error {
	entry := Entry{
		Url:        requested.Url,
		UserId:     userId,
		Title:      requested.Title,
		UseProxy:   requested.UseProxy,
		RefererUrl: requested.RefererUrl,
		Subtitles:  requested.Subtitles,
		CreatedAt:  time.Now(),
	}

	go server.setNewEntry(entry, requested)

	return nil
}

func (server *Server) playerSeek(timestamp float64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	return server.playerSeekLockless(timestamp, userId)
}

func (server *Server) playerSeekLockless(timestamp float64, userId uint64) error {
	server.state.player.Timestamp = timestamp
	server.state.lastUpdate = time.Now()

	event := server.createSyncEvent("seek", userId)
	server.addRecentAction(event.Action, event.UserId, event.Timestamp)

	server.writeEventToAllConnections("sync", event, userId)
	return nil
}

func (server *Server) playerNext(entryId uint64, userId uint64) error {
	if server.state.isLoading.Load() {
		return nil
	}

	// NOTE(kihau):
	//     Checking whether currently set entry ID on the client side matches current entry ID on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if server.state.entry.Id != entryId {
		return fmt.Errorf("Entry ID provided in the request is not equal to the current entry ID on the server")
	}

	if len(server.state.playlist) == 0 {
		if server.state.player.Looping {
			server.playerSeekLockless(0, 0)
		}

		return nil
	}

	entry := server.playlistDeleteAt(0)
	go server.setNewEntry(entry, RequestEntry{})
	return nil
}

func (server *Server) playerAutoplay(autoplay bool, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	server.state.player.Autoplay = autoplay
	DatabaseSetAutoplay(server.db, autoplay)
	server.writeEventToAllConnections("playerautoplay", autoplay, userId)

	return nil
}

func (server *Server) playerLooping(looping bool, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	server.state.player.Looping = looping
	DatabaseSetLooping(server.db, looping)
	server.writeEventToAllConnections("playerlooping", looping, userId)

	return nil
}

func (server *Server) playerUpdateTitle(title string, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	server.state.entry.Title = title
	DatabaseCurrentEntryUpdateTitle(server.db, title)

	server.addRecentAction("updatetitle", userId, title)
	server.writeEventToAllConnections("playerupdatetitle", title, userId)

	return nil
}

// NOTE(kihau):
//
//	This could become a more generalized "cloneEntry" function that takes source (history, playlist, currentEntry)
//	and clones entry with a given ID to a provided destination (history, playlist).
func (server *Server) historyPlaylistAdd(entryId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.history, entryId)
	if index == -1 {
		return fmt.Errorf("Failed to clone history element. Entry with ID %v is not in the history.", entryId)
	}

	oldEntry := server.state.history[index]
	newEntry := server.constructEntry(oldEntry)

	server.playlistAddOne(newEntry, false)
	return nil
}

func (server *Server) playlistAdd(requested RequestEntry, userId uint64) error {
	entry := Entry{
		Url:        requested.Url,
		UserId:     userId,
		Title:      requested.Title,
		UseProxy:   requested.UseProxy,
		RefererUrl: requested.RefererUrl,
		Subtitles:  requested.Subtitles,
		CreatedAt:  time.Now(),
	}

	localDirectory, path := server.isLocalDirectory(requested.Url)
	if localDirectory {
		LogInfo("Adding directory '%s' to the playlist.", path)
		localEntries := server.getEntriesFromDirectory(path, userId)
		server.state.mutex.Lock()
		server.playlistAddMany(localEntries, requested.AddToTop)
		server.state.mutex.Unlock()
		return nil
	}

	if isYoutubeEntry(entry, requested) {
		err := loadYoutubeEntry(&entry, requested)
		if err != nil {
			LogWarn("Failed to load entry in playlist add: %v", err)
			return nil
		}
	} else if isTwitchEntry(entry) {
		err := loadTwitchEntry(&entry)
		if err != nil {
			LogWarn("Failed to load entry in playlist add: %v", err)
			return nil
		}
	}

	LogInfo("Adding '%s' url to the playlist.", requested.Url)
	server.state.mutex.Lock()
	server.playlistAddOne(entry, requested.AddToTop)
	server.state.mutex.Unlock()

	if requested.IsPlaylist {
		requested.Url = entry.Url
		entries, err := server.loadYoutubePlaylist(requested, entry.UserId)

		if err != nil {
			return nil
		}

		if len(entries) > 0 {
			entries = entries[1:]
		}

		server.state.mutex.Lock()
		server.playlistAddMany(entries, requested.AddToTop)
		server.state.mutex.Unlock()
	}

	return nil
}

func (server *Server) playlistAddOne(entry Entry, toTop bool) {
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
	server.writeEventToAllConnections("playlist", event, newEntry.UserId)
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
	server.writeEventToAllConnections("playlist", event, 0)
}

func (server *Server) playlistPlay(entryId uint64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.playlist, entryId)
	if index == -1 {
		return fmt.Errorf("Failed to play playlist element. Entry with ID %v is not in the playlist.", entryId)
	}

	entry := server.playlistDeleteAt(index)
	go server.setNewEntry(entry, RequestEntry{})

	return nil
}

func (server *Server) playlistMove(data PlaylistMoveRequest, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.playlist, data.EntryId)
	if index == -1 {
		return fmt.Errorf("Failed to move playlist element. Entry with ID %v is not in the playlist.", data.EntryId)
	}

	if data.DestIndex < 0 || data.DestIndex >= len(server.state.playlist) {
		return fmt.Errorf("Failed to move playlist element id:%v. Dest index %v out of bounds.", data.EntryId, data.DestIndex)
	}

	entry := server.state.playlist[index]

	// Remove element from the slice:
	server.state.playlist = slices.Delete(server.state.playlist, index, index+1)

	list := make([]Entry, 0)

	// Appned removed element to a new list:
	list = append(list, server.state.playlist[:data.DestIndex]...)
	list = append(list, entry)
	list = append(list, server.state.playlist[data.DestIndex:]...)

	server.state.playlist = list

	eventData := PlaylistMoveEvent{
		EntryId:   data.EntryId,
		DestIndex: data.DestIndex,
	}

	event := createPlaylistEvent("move", eventData)
	server.writeEventToAllConnections("playlist", event, userId)

	go server.preloadYoutubeSourceOnNextEntry()

	return nil
}

func (server *Server) playlistDelete(entryId uint64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.playlist, entryId)
	if index == -1 {
		return fmt.Errorf("Failed to remove playlist element. Entry with ID %v is not in the playlist.", entryId)
	}

	server.playlistDeleteAt(index)

	go server.preloadYoutubeSourceOnNextEntry()

	return nil
}

func (server *Server) playlistDeleteAt(index int) Entry {
	entry := server.state.playlist[index]

	server.state.playlist = slices.Delete(server.state.playlist, index, index+1)
	DatabasePlaylistDelete(server.db, entry.Id)

	event := createPlaylistEvent("delete", entry.Id)
	server.writeEventToAllConnections("playlist", event, 0)

	return entry
}

func (server *Server) playlistClear() {
	server.state.mutex.Lock()
	server.state.playlist = server.state.playlist[:0]
	DatabasePlaylistClear(server.db)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("clear", nil)
	server.writeEventToAllConnections("playlist", event, 0)
}

func (server *Server) playlistShuffle() {
	server.state.mutex.Lock()
	for i := range server.state.playlist {
		j := rand.Intn(i + 1)
		server.state.playlist[i], server.state.playlist[j] = server.state.playlist[j], server.state.playlist[i]
	}
	server.state.mutex.Unlock()

	event := createPlaylistEvent("shuffle", server.state.playlist)
	server.writeEventToAllConnections("playlist", event, 0)

	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) playlistUpdate(entry Entry, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.playlist, entry.Id)
	if index == -1 {
		return fmt.Errorf("Entry with id:%v is not in the playlist.", entry.Id)
	}

	updated := server.state.playlist[index]
	updated.Title = entry.Title
	updated.Url = entry.Url
	DatabasePlaylistUpdate(server.db, updated.Id, updated.Title, updated.Url)
	server.state.playlist[index] = updated

	event := createPlaylistEvent("update", updated)
	server.writeEventToAllConnections("playlist", event, userId)

	return nil
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
		DatabaseHistoryDelete(server.db, removed.Id)
		server.writeEventToAllConnections("historydelete", removed.Id, 0)

		// Preserve subtitles if new entry has none
		if len(newEntry.Subtitles) == 0 && len(removed.Subtitles) > 0 {
			newEntry.Subtitles = removed.Subtitles
		}
	}

	server.state.history = append(server.state.history, newEntry)
	DatabaseHistoryAdd(server.db, newEntry)

	if len(server.state.history) > MAX_HISTORY_SIZE {
		removed := server.state.history[0]
		server.state.history = slices.Delete(server.state.history, 0, 1)
		DatabaseHistoryDelete(server.db, removed.Id)
	}

	server.writeEventToAllConnections("historyadd", newEntry, 0)
}

func (server *Server) historyDelete(index int) Entry {
	entry := server.state.history[index]

	server.state.history = slices.Delete(server.state.history, index, index+1)
	DatabasePlaylistDelete(server.db, entry.Id)
	server.writeEventToAllConnections("historydelete", entry.Id, entry.UserId)

	return entry
}

func (server *Server) chatGet(data MessageHistoryRequest, userId uint64) ([]ChatMessage, error) {
	if MAX_CHAT_LOAD < data.Count {
		return nil, fmt.Errorf("Too many messages were requested.")
	}

	backwardOffset := int(data.BackwardOffset)
	availableCount := len(server.state.messages) - backwardOffset

	// NOTE(kihau): Messages loaded from DB are never stored in-memory and because of that, they are read-only to the user.
	if availableCount < int(data.Count) && server.db != nil {
		dbMessages, _ := DatabaseMessageGet(server.db, int(data.Count), int(data.BackwardOffset))
		return dbMessages, nil
	}

	servedCount := minOf(availableCount, int(data.Count))
	if servedCount <= 0 {
		return []ChatMessage{}, nil
	}

	endOffset := len(server.state.messages) - backwardOffset
	startOffset := endOffset - servedCount
	messages := server.state.messages[startOffset:endOffset]

	return messages, nil
}

func (server *Server) chatCreate(messageContent string, userId uint64) error {
	if len([]rune(messageContent)) > MAX_MESSAGE_CHARACTERS {
		return fmt.Errorf("Message exceeds 1000 chars")
	}

	createdAt := time.Now().UnixMilli()

	server.state.mutex.Lock()
	server.state.messageId++
	message := ChatMessage{
		Id:        server.state.messageId,
		Content:   messageContent,
		CreatedAt: createdAt,
		EditedAt:  createdAt,
		UserId:    userId,
	}
	server.state.messages = append(server.state.messages, message)
	server.state.mutex.Unlock()

	DatabaseMessageAdd(server.db, message)

	server.writeEventToAllConnections("messagecreate", message, userId)
	return nil
}

func (server *Server) chatEdit(data ChatMessageEdit, userId uint64) error {
	if len([]rune(data.Content)) > MAX_MESSAGE_CHARACTERS {
		return fmt.Errorf("Message edit exceeds 1000 chars")
	}

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	messages := server.state.messages

	index := indexOfMessageById(messages, data.MessageId)
	if index == -1 {
		return fmt.Errorf("No message found of id %v", data.MessageId)
	}

	message := &messages[index]
	if message.UserId != userId {
		user := server.findUserById(userId)
		LogWarn("User %v (id:%v) tried to edit a stranger's message", user.Username, userId)
		return fmt.Errorf("You're not the author of this message")
	}

	message.Content = data.Content
	message.EditedAt = time.Now().UnixMilli()

	DatabaseMessageEdit(server.db, *message)

	server.writeEventToAllConnections("messageedit", data, userId)

	return nil
}

func (server *Server) chatDelete(messageId uint64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	messages := server.state.messages

	index := indexOfMessageById(messages, messageId)
	if index == -1 {
		return fmt.Errorf("No message found of id %v", messageId)
	}

	message := messages[index]
	if message.UserId != userId {
		user := server.findUserById(userId)
		LogWarn("User '%v' (id:%v) tried to remove a stranger's message", user.Username, userId)
		return fmt.Errorf("You're not the author of this message")
	}

	DatabaseMessageDelete(server.db, message.Id)

	server.state.messages = append(messages[:index], messages[index+1:]...)
	server.writeEventToAllConnections("messagedelete", message.Id, userId)

	return nil
}
