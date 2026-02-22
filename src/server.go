package main

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	rand2 "math/rand/v2"
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
	return users
}

func generateToken() string {
	tokenBytes := make([]byte, TOKEN_LENGTH)
	_, err := cryptorand.Read(tokenBytes)

	if err != nil {
		LogError("Token generation failed, this should not happen! %v", err)
		return ""
	}

	return base64.URLEncoding.EncodeToString(tokenBytes)
}

func generateSubName() string {
	buffer := make([]byte, 16)
	_, err := cryptorand.Read(buffer)

	if err != nil {
		LogError("Failed to generate subtitle name, this should not happen! %v", err)
		return "subtitle"
	}

	return base64.URLEncoding.EncodeToString(buffer)
}

func randomBase64(length int) string {
	buffer := make([]byte, length)
	_, err := cryptorand.Read(buffer)

	if err != nil {
		LogError("Random Base64 generation failed, this should not happen! %v", err)
		return ""
	}

	return base64.URLEncoding.EncodeToString(buffer)
}

func (server *Server) createUser() (User, error) {
	now := time.Now()
	user := User{
		Username:   generateRandomNickname(),
		Avatar:     "img/default_avatar.png",
		Online:     false,
		CreatedAt:  now,
		LastUpdate: now,
		LastOnline: now,
		token:      generateToken(),
	}

	server.users.mutex.Lock()
	defer server.users.mutex.Unlock()

	if err := DatabaseAddUser(server.db, &user); err != nil {
		return user, err
	}

	server.users.slice = append(server.users.slice, user)
	return user, nil
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
	if userId == SERVER_ID {
		return
	}

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
				Speed:    1.0,
			},
			actions:   make([]Action, 0, 4),
			resources: make(map[string]SharedResource),
		},

		users: users,
		conns: makeConnections(),
		db:    db,
	}

	configureRoutes()

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
	go server.periodicCacheCleanup()

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
		server.setNewEntry(currentEntry, SERVER_ID)

		timestamp := DatabaseGetTimestamp(server.db)

		server.state.mutex.Lock()
		defer server.state.mutex.Unlock()

		sync := PlayerSyncRequest{
			Timestamp:      timestamp,
			Programmatic:   true,
			CurrentEntryId: server.state.entry.Id,
		}
		server.playerUpdateState(PLAYER_SYNC_SEEK, sync, SERVER_ID)
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
	blackholeRequest(r)
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

func serveRoot(w http.ResponseWriter, r *http.Request) {
	LogDebug("User %v requested root endpoint: %v", getIp(r), r.RequestURI)

	if len(r.RequestURI) > MAX_UNKNOWN_PATH_LENGTH {
		blackholeRequest(r)
		http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
		return
	}

	endpoint := stripParams(r.RequestURI)
	file := path.Base(endpoint)
	if endsWithAny(file, ".php", ".cgi", ".jsp", ".aspx", "wordpress", "owa") {
		blackholeRequest(r)
		http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
	} else {
		http.ServeFile(w, r, WEB_ROOT+"static/welcome.html")
	}
}

func serveFavicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, WEB_ROOT+"img/favicon.ico")
}

func serveIcon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, WEB_ROOT+"img/icon.png")
}

func serveRobots(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, WEB_ROOT+"static/robots.txt")
}

func serveManifest(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, WEB_ROOT+"static/manifest.json")
}

func (server *Server) SharedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LogDebug("Connection %s requested shared resource %v", getIp(r), r.RequestURI)
		sharedPath := strings.TrimPrefix(r.RequestURI, "/share/")
		resources := server.state.resources

		server.state.resourceLock.RLock()
		resource, exists := resources[sharedPath]
		server.state.resourceLock.RUnlock()
		if !exists {
			http.NotFound(w, r)
			return
		}
		if resource.isExpired() {
			server.state.resourceLock.Lock()
			delete(resources, sharedPath)
			server.state.resourceLock.Unlock()
			http.Error(w, "Resource expired", 410)
			return
		}
		w.Header().Add("Cache-Control", "no-cache")
		http.ServeFile(w, r, resource.path)
	})
}

func (res *SharedResource) isExpired() bool {
	return time.Now().After(res.expires)
}

// NewFsHandler creates a new file system handler which includes Cache-Control headers
func NewFsHandler(strippedPrefix, dir string) http.Handler {
	dirHandler := http.FileServer(http.Dir(dir))
	fsHandler := http.StripPrefix(strippedPrefix, dirHandler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resource := strings.TrimPrefix(r.RequestURI, PAGE_ROOT)
		LogDebug("Connection %s requested resource %v", getIp(r), resource)

		// The no-cache directive does not prevent the storing of responses
		// but instead prevents the reuse of responses without revalidation.
		w.Header().Add("Cache-Control", "no-cache")
		fsHandler.ServeHTTP(w, r)
	})
}

func NewGatewayHandler(handler http.Handler, hits, perSecond int, ipv4Ranges []IpV4Range) GatewayHandler {
	return GatewayHandler{
		handler:             handler,
		ipToLimiters:        make(map[string]*RateLimiter),
		mapMutex:            new(sync.Mutex),
		blacklistedIpRanges: ipv4Ranges,
		hits:                hits,
		perSecond:           perSecond,
	}
}

func (gate GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := stripPort(getIp(r))

	for _, blacklistedRange := range gate.blacklistedIpRanges {
		if blacklistedRange.Contains(ip) {
			resource := strings.TrimPrefix(r.RequestURI, PAGE_ROOT)
			LogWarn("Blacklisted address %v attempted to access %v", ip, resource)
			blackholeRequest(r)
			http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
			return
		}
	}

	gate.mapMutex.Lock()
	rateLimiter, exists := gate.ipToLimiters[ip]
	if exists {
		gate.mapMutex.Unlock()
		rateLimiter.mutex.Lock()
		if rateLimiter.block() {
			retryAfter := rateLimiter.getRetryAfter()
			rateLimiter.mutex.Unlock()
			respondTooManyRequests(w, ip, retryAfter)
			return
		}
		rateLimiter.mutex.Unlock()
	} else {
		gate.ipToLimiters[ip] = NewLimiter(gate.hits, gate.perSecond)
		gate.mapMutex.Unlock()
	}

	gate.handler.ServeHTTP(w, r)
}

func registerEndpoints(server *Server) *http.ServeMux {
	mux := http.NewServeMux()
	server.registerRedirects(mux)

	ipv4Ranges := compileIpRanges(server.config.BlacklistedIpRanges)
	contentFs := NewFsHandler(CONTENT_ROUTE, CONTENT_ROOT)
	contentHandler := NewGatewayHandler(contentFs, CONTENT_LIMITER_HITS, CONTENT_LIMITER_PER_SECOND, ipv4Ranges)
	mux.Handle(CONTENT_ROUTE, contentHandler)

	shareHandler := NewGatewayHandler(server.SharedHandler(), 10, 1, nil)
	mux.Handle("/share/", shareHandler)

	webFs := NewCachedFsHandler(PAGE_ROOT, WEB_ROOT, true)
	staticHandler := NewGatewayHandler(webFs, STATIC_LIMITER_HITS, STATIC_LIMITER_PER_SECOND, ipv4Ranges)
	mux.Handle(PAGE_ROOT, staticHandler)

	mux.HandleFunc("/", serveRoot)
	mux.HandleFunc("/robots.txt", serveRobots)
	mux.HandleFunc("/favicon.ico", serveFavicon)
	mux.HandleFunc("/icon.png", serveIcon)
	mux.HandleFunc("/manifest.json", serveManifest)

	mux.HandleFunc("/api/", handleUnknownEndpoint)

	// Unrelated API calls.
	server.handleEndpoint(mux, "/api/version", server.apiVersion, "GET")
	server.handleEndpoint(mux, "/api/uptime", server.apiUptime, "GET")
	server.handleEndpoint(mux, "/api/login", server.apiLogin, "GET")
	server.handleEndpointAuthorized(mux, "/api/uploadmedia", server.apiUploadMedia, "POST")
	server.handleEndpointAuthorized(mux, "/api/invite/create", server.apiInviteCreate, "GET")
	server.handleEndpointAuthorized(mux, "/api/share", server.apiShare, "POST")

	// User related API calls.
	server.handleEndpoint(mux, "/api/user/create", server.apiUserCreate, "POST")
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

func (server *Server) periodicCacheCleanup() {
	for {
		time.Sleep(time.Second * 10)
		server.state.resourceLock.Lock()
		resources := server.state.resources
		var toRemove []string
		for k, v := range resources {
			if v.isExpired() {
				toRemove = append(toRemove, k)
			}
		}
		for _, k := range toRemove {
			delete(resources, k)
		}
		server.state.resourceLock.Unlock()
	}
}

func (server *Server) detectYtdlpSource(url string) QuerySource {
	if url == "" {
		return ENTRY_SOURCE_NONE
	}

	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		return ENTRY_SOURCE_NONE
	}

	host := parsedUrl.Host

	if strings.HasSuffix(host, "youtube.com") || strings.HasSuffix(host, "youtu.be") {
		return ENTRY_SOURCE_YOUTUBE
	} else if strings.HasSuffix(host, "twitch.tv") {
		return ENTRY_SOURCE_TWITCH
	} else if strings.HasSuffix(host, "twitter.com") || strings.HasSuffix(host, "x.com") {
		return ENTRY_SOURCE_TWITCH
	} else if strings.HasSuffix(host, "bandcamp.com") {
		return ENTRY_SOURCE_BANDCAMP
	} else if strings.HasSuffix(host, "tiktok.com") {
		return ENTRY_SOURCE_TIKTOK
	} else {
		return ENTRY_SOURCE_NONE
	}
}

func (server *Server) loadYtdlpSource(newEntry *Entry, source QuerySource) error {
	var err error

	switch source {
	case ENTRY_SOURCE_YOUTUBE:
		err = loadYoutubeEntry(newEntry)
	case ENTRY_SOURCE_TIKTOK:
		err = loadTikTokEntry(newEntry)
	case ENTRY_SOURCE_TWITCH:
		err = loadTwitchEntry(newEntry)
	case ENTRY_SOURCE_TWITTER:
		err = loadTwitterEntry(newEntry)
	case ENTRY_SOURCE_BANDCAMP:
		// TODO(kihau)
	default:
		LogError("Unsuppored ytdlp source host detected: %v", source)
	}

	newEntry.cacheThumbnail()
	if err != nil {
		server.writeEventToAllConnections("playererror", err.Error(), SERVER_ID)
		return err
	}

	return nil
}

func (server *Server) setNewEntry(newEntry Entry, setById uint64) {
	server.state.isLoadingEntry.Store(true)
	defer server.state.isLoadingEntry.Store(false)

	source := server.detectYtdlpSource(newEntry.Url)
	if source != ENTRY_SOURCE_NONE {
		server.writeEventToAllConnections("playerwaiting", "Video is loading. Please stand by!", SERVER_ID)
		server.loadYtdlpSource(&newEntry, source)
	}

	err := server.setupProxy(&newEntry)
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

	now := time.Now()

	newEntry.SetById = setById
	newEntry.LastSetAt = now
	DatabaseCurrentEntrySet(server.db, &newEntry)
	server.state.entry = newEntry

	server.state.player.Timestamp = 0
	server.state.player.Speed = 1
	server.state.lastUpdate = now
	server.state.player.Playing = server.state.player.Autoplay
	server.state.isLyricsFetched.Store(false)

	LogInfo("New entry URL is now: '%s'.", newEntry.Url)
	server.writeEventToAllConnections("playerset", newEntry, SERVER_ID)

	go server.preloadYoutubeSourceOnNextEntry()
}

func isPathM3U(p string) bool {
	return strings.HasSuffix(p, ".m3u8") || strings.HasSuffix(p, ".m3u") || strings.HasSuffix(p, ".txt")
}

func isContentM3U(url, referer string) bool {
	success, buffer, contentType := testGetResponse(url, referer)
	return success && (strings.HasPrefix(contentType, M3U8_CONTENT_TYPE) || bufferStartsWith(buffer, EXTM3U_BYTES))
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

// isTrustedUrl checks if url points to server address or if url's domain is trusted
func (server *Server) isTrustedUrl(parsedUrl *net_url.URL) bool {
	if parsedUrl == nil {
		return false
	}
	schemeHost := parsedUrl.Scheme + "://" + parsedUrl.Host
	if strings.HasPrefix(schemeHost, serverRootAddress) {
		return true
	}

	if parsedUrl.Hostname() == server.config.Domain {
		return true
	}

	return false
}

// TODO(kihau): More explicit error output messages.
func (server *Server) setupProxy(entry *Entry) error {
	// This should be moved to some destructEntry() / unloadEntry() method
	server.state.setupLock.Lock()
	server.state.fileProxy.destruct()
	server.state.audioFileProxy.destruct()
	server.state.setupLock.Unlock()

	urlStruct, err := net_url.Parse(entry.Url)
	if err != nil {
		return err
	}

	if isYtdlpProxy(entry.Url) {
		success := server.setupHlsProxy(entry.SourceUrl, "")
		if success {
			entry.ProxyUrl = PROXY_ROUTE + PROXY_M3U8
			LogInfo("HLS proxy setup for youtube was successful.")
			return nil
		} else {
			return fmt.Errorf("HLS proxy setup for youtube failed!")
		}
	}

	if !entry.UseProxy {
		return nil
	}

	err = nil
	file := getBaseNoParams(urlStruct.Path)
	url, referer := entry.Url, entry.RefererUrl
	if isPathM3U(file) || isContentM3U(url, referer) {
		setup := server.setupHlsProxy(url, referer)
		if setup {
			entry.ProxyUrl = PROXY_ROUTE + PROXY_M3U8
			LogInfo("HLS proxy setup was successful.")
		} else {
			err = fmt.Errorf("HLS proxy setup failed!")
		}
	} else {
		_ = os.RemoveAll(CONTENT_PROXY)
		_ = os.MkdirAll(CONTENT_PROXY, os.ModePerm)
		fileProxy := &server.state.fileProxy
		setupVideo := server.setupFileProxy(fileProxy, url, referer, "proxy-vid")
		if setupVideo {
			entry.ProxyUrl = PROXY_ROUTE + fileProxy.filename
			LogInfo("Generic file proxy setup was successful.")
		} else {
			err = fmt.Errorf("Generic file proxy setup failed!")
		}
	}
	return err
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
		_, msgBytes, err := conn.ReadMessage()
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
			LogDebug("Writing bytes of len: %v to %v clients", len(msgBytes), len(voiceClients))
			if err := client.WriteMessage(websocket.BinaryMessage, msgBytes); err != nil {
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
		server.serveFileProxy(writer, request, chunk)
	}
}

func (server *Server) watchStream(writer http.ResponseWriter, request *http.Request) {
	urlPath := request.URL.Path
	chunk := path.Base(urlPath)

	server.serveStream(writer, request, chunk)
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

func (server *Server) playerPlay(data PlayerSyncRequest, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()
	return server.playerUpdateState(PLAYER_SYNC_PLAY, data, userId)
}

func (server *Server) playerPause(data PlayerSyncRequest, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()
	return server.playerUpdateState(PLAYER_SYNC_PAUSE, data, userId)
}

func (server *Server) playerUpdateState(syncType PlayerSyncType, data PlayerSyncRequest, userId uint64) error {
	if server.state.entry.Id != data.CurrentEntryId {
		return nil
	}

	if data.Programmatic {
		userId = SERVER_ID
	}

	server.state.player.Timestamp = data.Timestamp
	server.state.lastUpdate = time.Now()

	var event SyncEvent
	switch syncType {
	case PLAYER_SYNC_PAUSE:
		event = server.createSyncEvent("pause", userId)
		server.state.player.Playing = false

	case PLAYER_SYNC_PLAY:
		event = server.createSyncEvent("play", userId)
		server.state.player.Playing = true

	case PLAYER_SYNC_SEEK:
		event = server.createSyncEvent("seek", userId)

	default:
		return fmt.Errorf("Unexpected main.PlayerSyncType: %#v", syncType)
	}

	server.addRecentAction(event.Action, event.UserId, event.Timestamp)
	server.writeEventToAllConnections("sync", event, userId)
	return nil
}

func (server *Server) getCurrentTimestamp() float64 {
	var timestamp = server.state.player.Timestamp
	var speed = server.state.player.Speed
	if server.state.player.Playing {
		now := time.Now()
		diff := now.Sub(server.state.lastUpdate)
		timestamp = server.state.player.Timestamp + diff.Seconds()*speed
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
	urlStruct, err := net_url.Parse(url)
	if err != nil {
		return false, ""
	}

	if urlStruct == nil {
		return false, ""
	}
	urlPath := urlStruct.Path

	if !filepath.IsLocal(urlPath) {
		return false, ""
	}

	stat, err := os.Stat(urlPath)
	if err != nil {
		return false, ""
	}

	if !stat.IsDir() {
		return false, ""
	}

	return true, urlPath
}

func (server *Server) loadLocalPlaylist(directoryPath string, addToTop bool, userId uint64) error {
	LogInfo("Adding directory '%s' to the playlist.", directoryPath)
	entries := make([]Entry, 0)

	items, err := os.ReadDir(directoryPath)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, item := range items {
		if item.IsDir() {
			continue
		}
		// path.Join will always use forward slashes independent of the OS
		webPath := path.Join(directoryPath, item.Name())

		LogDebug("File URL: %v", webPath)

		entry := Entry{
			Url:       webPath,
			UserId:    userId,
			UseProxy:  false,
			Subtitles: []Subtitle{},
			CreatedAt: now,
			LastSetAt: now,
		}

		entry.Title = constructTitleWhenMissing(&entry)
		entries = append(entries, entry)
	}

	server.state.mutex.Lock()
	server.playlistAddMany(entries, addToTop)
	server.state.mutex.Unlock()
	return nil
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

func (server *Server) playerGet() PlayerGetResponse {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	player := server.state.player
	entry := server.state.entry

	player.Timestamp = server.getCurrentTimestamp()

	actions := make([]Action, len(server.state.actions))
	copy(actions, server.state.actions)

	data := PlayerGetResponse{
		Player:  player,
		Entry:   entry,
		Actions: actions,
	}

	return data
}

func (server *Server) playerSet(requested RequestEntry, userId uint64) error {
	url := requested.Url
	if url != "" {
		relativeUrl, err := server.relativizeUrl(url)
		if err != nil {
			LogWarn("Not setting URL, issue: %v", err)
			return err
		}
		url = relativeUrl
	}

	if requested.QuerySource == ENTRY_SOURCE_YOUTUBE {
		url = searchYoutubeForUrl(requested.Query)
	}

	entry := Entry{
		Url:        url,
		UserId:     userId,
		Title:      requested.Title,
		UseProxy:   requested.UseProxy,
		RefererUrl: requested.Referer,
		Subtitles:  requested.Subtitles,
		CreatedAt:  time.Now(),
	}

	entry.Title = constructTitleWhenMissing(&entry)
	server.setNewEntry(entry, userId)

	if requested.LyricsFetch {
		server.fetchLyricsForCurrentEntry(userId)
	}

	source := server.detectYtdlpSource(entry.Url)
	if requested.PlaylistFetch && source == ENTRY_SOURCE_YOUTUBE {
		return server.loadYoutubePlaylist(entry.Url, requested.PlaylistSkipCount, requested.PlaylistMaxSize, requested.PlaylistToTop, userId)
	}

	return nil
}

func (server *Server) playerSeek(data PlayerSyncRequest, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()
	return server.playerUpdateState(PLAYER_SYNC_SEEK, data, userId)
}

func (server *Server) playerNext(data PlayerNextRequest, userId uint64) error {
	// TODO(kihau): Not fully thread safe. Fix it.
	if server.state.isLoadingEntry.Load() {
		return nil
	}

	// NOTE(kihau):
	//     Checking whether currently set entry ID on the client side matches current entry ID on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if server.state.entry.Id != data.CurrentEntryId {
		return nil
	}

	if len(server.state.playlist) == 0 {
		if server.state.player.Looping && server.state.player.Autoplay {
			sync := PlayerSyncRequest{
				Timestamp:      0.0,
				Programmatic:   true,
				CurrentEntryId: server.state.entry.Id,
			}
			server.playerUpdateState(PLAYER_SYNC_PLAY, sync, SERVER_ID)
		}

		return nil
	}

	if data.Programmatic {
		userId = SERVER_ID
	}

	entry := server.playlistDeleteAt(0)
	go server.setNewEntry(entry, userId)
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

func (server *Server) playerSpeedChange(speed float64, userId uint64) error {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if speed > MAX_SPEED {
		LogInfo("Reducing playback speed %v -> %v", speed, MAX_SPEED)
		speed = MAX_SPEED
	}
	if speed < MIN_SPEED {
		speed = MIN_SPEED
	}
	server.state.player.Speed = speed

	server.addRecentAction("speedchange", userId, speed)
	server.writeEventToAllConnections("playerspeedchange", speed, userId)

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

	entry := server.state.history[index]
	server.playlistAddOne(entry, false)
	return nil
}

func (server *Server) playlistAdd(requested RequestEntry, userId uint64) error {
	url := requested.Url
	if url != "" {
		relativeUrl, err := server.relativizeUrl(requested.Url)
		if err != nil {
			LogWarn("Not setting URL, issue: %v", err)
			return err
		}
		url = relativeUrl
	}

	if requested.QuerySource == ENTRY_SOURCE_YOUTUBE {
		url = searchYoutubeForUrl(requested.Query)
	}

	isLocalDir, path := server.isLocalDirectory(url)
	if isLocalDir {
		return server.loadLocalPlaylist(path, requested.PlaylistToTop, userId)
	} 

	source := server.detectYtdlpSource(url)
	if requested.PlaylistFetch && source == ENTRY_SOURCE_YOUTUBE {
		return server.loadYoutubePlaylist(url, requested.PlaylistSkipCount, requested.PlaylistMaxSize, requested.PlaylistToTop, userId)
	} else {
		now := time.Now()
		entry := Entry{
			Url:        url,
			UserId:     userId,
			Title:      requested.Title,
			UseProxy:   requested.UseProxy,
			RefererUrl: requested.Referer,
			Subtitles:  requested.Subtitles,
			CreatedAt:  now,
			LastSetAt:  now,
		}

		entry.Title = constructTitleWhenMissing(&entry)

		source := server.detectYtdlpSource(entry.Url)
		if source != ENTRY_SOURCE_NONE {
			if err := server.loadYtdlpSource(&entry, source); err != nil {
				return err
			}
		}

		if requested.LyricsFetch {
			subtitle, err := server.state.fetchLyrics(entry.Title, entry.Metadata)
			if err != nil {
				entry.Subtitles = append(entry.Subtitles, subtitle)
			}
		}

		LogInfo("Adding '%s' url to the playlist.", entry.Url)
		server.state.mutex.Lock()
		server.playlistAddOne(entry, requested.PlaylistToTop)
		server.state.mutex.Unlock()
		return nil
	}
}

func (server *Server) playlistAddOne(entry Entry, toTop bool) error {
	if strings.TrimSpace(entry.Title) == "" {
		return nil
	}

	if err := DatabasePlaylistAdd(server.db, &entry); err != nil {
		return err
	}

	var event PlaylistEvent
	if toTop {
		playlist := make([]Entry, 0)
		playlist = append(playlist, entry)
		server.state.playlist = append(playlist, server.state.playlist...)
		event = createPlaylistEvent("addtop", entry)
	} else {
		server.state.playlist = append(server.state.playlist, entry)
		event = createPlaylistEvent("add", entry)
	}

	server.writeEventToAllConnections("playlist", event, entry.UserId)
	return nil
}

func (server *Server) playlistAddMany(entries []Entry, toTop bool) {
	if len(entries) == 0 {
		return
	}

	var event PlaylistEvent

	DatabasePlaylistAddMany(server.db, entries)

	if toTop {
		server.state.playlist = append(entries, server.state.playlist...)
		event = createPlaylistEvent("addmanytop", entries)
	} else {
		server.state.playlist = append(server.state.playlist, entries...)
		event = createPlaylistEvent("addmany", entries)
	}

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
	go server.setNewEntry(entry, userId)

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

func (server *Server) historyAdd(entry Entry) error {
	if strings.TrimSpace(entry.Title) == "" {
		return nil
	}

	compareFunc := func(existing Entry) bool {
		return compareEntries(entry, existing)
	}

	index := slices.IndexFunc(server.state.history, compareFunc)
	if index != -1 {
		// De-duplicate history entries.
		removed := server.state.history[index]
		server.state.history = slices.Delete(server.state.history, index, index+1)
		DatabaseHistoryDelete(server.db, removed.Id)
		server.writeEventToAllConnections("historydelete", removed.Id, 0)

		// Preserve subtitles if new entry has none
		if len(entry.Subtitles) == 0 && len(removed.Subtitles) > 0 {
			entry.Subtitles = removed.Subtitles
		}
	}

	if err := DatabaseHistoryAdd(server.db, &entry); err != nil {
		return err
	}

	server.state.history = append(server.state.history, entry)

	if len(server.state.history) > MAX_HISTORY_SIZE {
		removed := server.state.history[0]
		server.state.history = slices.Delete(server.state.history, 0, 1)
		DatabaseHistoryDelete(server.db, removed.Id)
	}

	server.writeEventToAllConnections("historyadd", entry, 0)
	return nil
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
	defer server.state.mutex.Unlock()

	message := ChatMessage{
		Content:   messageContent,
		CreatedAt: createdAt,
		EditedAt:  createdAt,
		UserId:    userId,
	}

	if err := DatabaseMessageAdd(server.db, &message); err != nil {
		return err
	}

	server.state.messages = append(server.state.messages, message)
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

func (server *Server) fetchLyricsForCurrentEntry(userId uint64) error {
	if !server.state.isLyricsFetched.CompareAndSwap(false, true) {
		return nil
	}

	server.state.mutex.Lock()
	entry := server.state.entry
	server.state.mutex.Unlock()

	subtitle, err := server.state.fetchLyrics(entry.Title, entry.Metadata)
	if err != nil {
		server.state.isLyricsFetched.Store(false)
		server.writeEventToAllConnections("playererror", err.Error(), SERVER_ID)
		return err
	}

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if server.state.entry.Id == entry.Id {
		DatabaseSubtitleAdd(server.db, server.state.entry.Id, &subtitle)
		server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
		server.writeEventToAllConnections("subtitleattach", subtitle, userId)
	} else {
		server.state.isLyricsFetched.Store(false)
		LogDebug("Found entry with id %v", entry.Id)
	}

	return nil
}

func (server *Server) createNewInvite(userId uint64) Invite {
	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	invite := Invite{
		InviteCode: randomBase64(6),
		ExpiresAt:  time.Now().Add(time.Hour * time.Duration(12)),
		CreatedBy:  userId,
	}

	server.state.invite = invite
	server.writeEventToAllConnections("invitecreate", invite, SERVER_ID)
	return invite
}

// relativizeUrl makes the url relative or returns the url unchanged; in case of invalid urls an error is returned
func (server *Server) relativizeUrl(url string) (string, error) {
	_, safe := safeJoin(url)
	if !safe {
		// The path could be simplified and then processed
		return "", errors.New("the path is traversed")
	}
	urlStruct, err := net_url.Parse(url)
	if err != nil {
		return "", err
	}

	// Cover links containing #
	urlStruct.Fragment = ""

	if urlStruct.Scheme == "" || server.isTrustedUrl(urlStruct) {
		// Maintain query params
		relativeUrl := strings.TrimPrefix(urlStruct.Path, PAGE_ROOT)
		if !strings.HasPrefix(relativeUrl, CONTENT_MEDIA) {
			return "", errors.New("relative URL does not point to " + CONTENT_MEDIA)
		}
		urlStruct.Scheme = ""
		urlStruct.Host = ""
		urlStruct.Path = relativeUrl
	}
	return urlStruct.String(), nil
}

func (entry *Entry) cacheThumbnail() {
	if entry.Thumbnail == "" || !isAbsolute(entry.Thumbnail) {
		return
	}
	options := &DownloadOptions{
		referer:   entry.RefererUrl,
		hasty:     false,
		bodyLimit: MAX_THUMBNAIL_SIZE,
	}

	// This mkdir should be done on server startup?
	os.MkdirAll(MEDIA_THUMB, os.ModePerm)
	thumbnailPath := MEDIA_THUMB + uint64ToString(rand2.Uint64())
	fetchErr := downloadFile(entry.Thumbnail, thumbnailPath, options)
	if fetchErr != nil {
		LogError("Thumbnail could not be downloaded: %v", fetchErr)
		return
	}
	entry.Thumbnail = thumbnailPath
	err := KeepLatestFiles(MEDIA_THUMB, MAX_THUMBNAIL_COUNT)
	if err != nil {
		LogError("Thumbnail directory could not be culled: %v", err)
	}
}

func (server *Server) loadYoutubePlaylist(url string, skipCount uint, maxSize uint, addToTop bool, userId uint64) error {
	query := url
	parsedUrl, err := net_url.Parse(query)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return fmt.Errorf("Failed to parse youtube source url: %v", err)
	}

	if !parsedUrl.Query().Has("list") {
		videoId := parsedUrl.Query().Get("v")

		query := parsedUrl.Query()
		query.Add("list", "RD"+videoId)
		parsedUrl.RawQuery = query.Encode()

		LogDebug("Url was not a playlist. Constructed youtube playlist url is now: %v", parsedUrl)
	}

	size := maxSize + skipCount
	if size > 1000 {
		size = 1000
	} else if size <= 0 {
		size = 20
	}

	query = parsedUrl.String()
	playlist, err := fetchYoutubePlaylist(query, skipCount + 1, size)
	if err != nil {
		return err
	}

	entries := make([]Entry, 0)

	for _, ytEntry := range playlist.Entries {
		entry := Entry{
			Url:       ytEntry.Url,
			Title:     ytEntry.Title,
			UserId:    userId,
			Thumbnail: pickSmallestThumbnail(ytEntry.Thumbnails),
			CreatedAt: time.Now(),
		}

		entries = append(entries, entry)
	}

	server.state.mutex.Lock()
	server.playlistAddMany(entries, addToTop)
	server.state.mutex.Unlock()

	go server.preloadYoutubeSourceOnNextEntry()
	return nil
}
