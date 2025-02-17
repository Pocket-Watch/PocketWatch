package main

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	net_url "net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const KB = 1024
const MB = 1024 * KB
const GB = 1024 * MB

const RETRY = 5000 // Retry time in milliseconds
const TOKEN_LENGTH = 32
const BROADCAST_INTERVAL = 2 * time.Second

const SUBTITLE_SIZE_LIMIT = 512 * KB
const PROXY_FILE_SIZE_LIMIT = 4 * GB
const BODY_LIMIT = 8 * KB

var SUBTITLE_EXTENSIONS = [...]string{".vtt", ".srt"}

const PROXY_ROUTE = "/watch/proxy/"
const WEB_PROXY = "web/proxy/"
const WEB_MEDIA = "web/media/"
const MEDIA = "media/"
const ORIGINAL_M3U8 = "original.m3u8"
const PROXY_M3U8 = "proxy.m3u8"

type Server struct {
	state ServerState
	users *Users
	conns *Connections
}

type PlayerState struct {
	Playing   bool    `json:"playing"`
	Autoplay  bool    `json:"autoplay"`
	Looping   bool    `json:"looping"`
	Timestamp float64 `json:"timestamp"`
}

type Subtitle struct {
	Id    uint64  `json:"id"`
	Name  string  `json:"name"`
	Url   string  `json:"url"`
	Shift float64 `json:"shift"`
}

type Entry struct {
	Id         uint64     `json:"id"`
	Url        string     `json:"url"`
	Title      string     `json:"title"`
	UserId     uint64     `json:"user_id"`
	UseProxy   bool       `json:"use_proxy"`
	RefererUrl string     `json:"referer_url"`
	SourceUrl  string     `json:"source_url"`
	Subtitles  []Subtitle `json:"subtitles"`
	Thumbnail  string     `json:"thumbnail"`
	Created    time.Time  `json:"created"`
}

type ServerState struct {
	mutex sync.Mutex

	player  PlayerState
	entry   Entry
	entryId uint64

	eventId    atomic.Uint64
	lastUpdate time.Time

	playlist  []Entry
	history   []Entry
	messages  []ChatMessage
	messageId uint64
	subsId    atomic.Uint64

	setupLock    sync.Mutex
	proxy        *HlsProxy
	isLive       bool
	isHls        bool
	genericProxy GenericProxy
}

type HlsProxy struct {
	// HLS proxy
	chunkLocks     []sync.Mutex
	fetchedChunks  []bool
	originalChunks []string
	// Live resources
	liveUrl      string
	liveSegments sync.Map
	randomizer   atomic.Int64
	lastRefresh  time.Time
}

type GenericProxy struct {
	contentLength       int64
	extensionWithDot    string
	fileUrl             string
	file                *os.File
	contentRanges       []Range // must remain sorted
	rangesMutex         sync.Mutex
	download            *http.Response
	downloadMutex       sync.Mutex
	downloadBeginOffset int64
}

type Connection struct {
	id     uint64
	userId uint64
	writer http.ResponseWriter
}

type Connections struct {
	mutex     sync.Mutex
	idCounter uint64
	slice     []Connection
}

type User struct {
	Id          uint64 `json:"id"`
	Username    string `json:"username"`
	Avatar      string `json:"avatar"`
	Online      bool   `json:"online"`
	connections uint64
	token       string
	created     time.Time
	lastUpdate  time.Time
}

type Users struct {
	mutex     sync.Mutex
	idCounter uint64
	slice     []User
}

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

	new_user := User{
		Id:         id,
		Username:   fmt.Sprintf("User %v", id),
		Avatar:     "img/default_avatar.png",
		token:      generateToken(),
		created:    time.Now(),
		lastUpdate: time.Now(),
	}

	users.slice = append(users.slice, new_user)
	return new_user
}

func (users *Users) findIndex(token string) int {
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

func (conns *Connections) add(writer http.ResponseWriter, userId uint64) uint64 {
	id := conns.idCounter
	conns.idCounter += 1

	conn := Connection{
		id:     id,
		userId: userId,
		writer: writer,
	}
	conns.slice = append(conns.slice, conn)

	return id
}

func (conns *Connections) remove(id uint64) {
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

type PlayerGetResponseData struct {
	Player PlayerState `json:"player"`
	Entry  Entry       `json:"entry"`
	// Subtitles []string    `json:"subtitles"`
}

type SyncRequestData struct {
	ConnectionId uint64  `json:"connection_id"`
	Timestamp    float64 `json:"timestamp"`
}

type SyncEventData struct {
	Timestamp float64 `json:"timestamp"`
	Action    string  `json:"action"`
	UserId    uint64  `json:"user_id"`
}

type PlayerSetRequestData struct {
	ConnectionId uint64       `json:"connection_id"`
	RequestEntry RequestEntry `json:"request_entry"`
}

type PlayerSetEventData struct {
	PrevEntry Entry `json:"prev_entry"`
	NewEntry  Entry `json:"new_entry"`
}

type PlaybackEnded struct {
	EntryId uint64 `json:"entry_id"`
}

type PlayerNextRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	EntryId      uint64 `json:"entry_id"`
}

type PlayerNextEventData struct {
	PrevEntry Entry `json:"prev_entry"`
	NewEntry  Entry `json:"new_entry"`
}

type SubtitleUpdateRequestData struct {
	Id   uint64 `json:"id"`
	Name string `json:"name"`
}

type SubtitleShiftRequestData struct {
	Id    uint64  `json:"id"`
	Shift float64 `json:"shift"`
}

type PlaylistEventData struct {
	Action string `json:"action"`
	Data   any    `json:"data"`
}

func createPlaylistEvent(action string, data any) PlaylistEventData {
	event := PlaylistEventData{
		Action: action,
		Data:   data,
	}

	return event
}

type RequestEntry struct {
	Url               string     `json:"url"`
	Title             string     `json:"title"`
	UseProxy          bool       `json:"use_proxy"`
	RefererUrl        string     `json:"referer_url"`
	SearchVideo       bool       `json:"search_video"`
	IsPlaylist        bool       `json:"is_playlist"`
	AddToTop          bool       `json:"add_to_top"`
	Subtitles         []Subtitle `json:"subtitles"`
	PlaylistSkipCount uint       `json:"playlist_skip_count"`
	PlaylistMaxSize   uint       `json:"playlist_max_size"`
}

type PlaylistPlayRequestData struct {
	EntryId uint64 `json:"entry_id"`
	Index   int    `json:"index"`
}

type PlaylistAddRequestData struct {
	ConnectionId uint64       `json:"connection_id"`
	RequestEntry RequestEntry `json:"request_entry"`
}

type PlaylistRemoveRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	EntryId      uint64 `json:"entry_id"`
	Index        int    `json:"index"`
}

type PlaylistAutoplayRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	Autoplay     bool   `json:"autoplay"`
}

type PlaylistLoopingRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	Looping      bool   `json:"looping"`
}

type PlaylistMoveRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	EntryId      uint64 `json:"entry_id"`
	SourceIndex  int    `json:"source_index"`
	DestIndex    int    `json:"dest_index"`
}

type PlaylistMoveEventData struct {
	SourceIndex int `json:"source_index"`
	DestIndex   int `json:"dest_index"`
}

type PlaylistUpdateRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	Entry        Entry  `json:"entry"`
	Index        int    `json:"index"`
}

// Constants - assignable only once!
var serverRootAddress string
var startTime = time.Now()
var subsEnabled bool
var serverDomain string

func StartServer(options *Options) {
	server := Server{
		state: ServerState{},
		users: makeUsers(),
		conns: makeConnections(),
	}

	server.state.lastUpdate = time.Now()
	subsEnabled = options.Subs
	serverDomain = options.Domain
	registerEndpoints(&server)

	var address = options.Address + ":" + strconv.Itoa(int(options.Port))
	LogInfo("Starting server on address: %s", address)

	const CERT = "./secret/certificate.pem"
	const PRIV_KEY = "./secret/privatekey.pem"

	_, err_cert := os.Stat(CERT)
	_, err_priv := os.Stat(PRIV_KEY)

	missing_ssl_keys := errors.Is(err_priv, os.ErrNotExist) || errors.Is(err_cert, os.ErrNotExist)

	if options.Ssl && missing_ssl_keys {
		LogError("Failed to find either SSL certificate or the private key.")
	}

	var server_start_error error
	if !options.Ssl || missing_ssl_keys {
		serverRootAddress = "http://" + address
		LogWarn("Server is running in unencrypted http mode.")
		server_start_error = http.ListenAndServe(address, nil)
	} else {
		serverRootAddress = "https://" + address
		LogInfo("Server is running with TLS on.")
		server_start_error = http.ListenAndServeTLS(address, CERT, PRIV_KEY, nil)
	}

	if server_start_error != nil {
		LogError("Error starting the server: %v", server_start_error)
	}
}

func handleUnknownEndpoint(w http.ResponseWriter, r *http.Request) {
	LogWarn("User %v requested unknown endpoint: %v", r.RemoteAddr, r.RequestURI)
	http.Error(w, "¯\\_(ツ)_/¯", http.StatusTeapot)
}

func registerEndpoints(server *Server) {

	fileserver := http.FileServer(http.Dir("./web"))
	http.Handle("/watch/", http.StripPrefix("/watch/", fileserver))

	http.HandleFunc("/", handleUnknownEndpoint)

	// Unrelated API calls.
	server.HandleEndpoint("/watch/api/version", server.apiVersion, "GET", false)
	server.HandleEndpoint("/watch/api/uptime", server.apiUptime, "GET", false)
	server.HandleEndpoint("/watch/api/login", server.apiLogin, "GET", false)
	server.HandleEndpoint("/watch/api/uploadmedia", server.apiUploadMedia, "POST", true)

	// User related API calls.
	server.HandleEndpoint("/watch/api/user/create", server.apiUserCreate, "GET", false)
	server.HandleEndpoint("/watch/api/user/getall", server.apiUserGetAll, "GET", true)
	server.HandleEndpoint("/watch/api/user/verify", server.apiUserVerify, "POST", true)
	server.HandleEndpoint("/watch/api/user/updatename", server.apiUserUpdateName, "POST", true)
	server.HandleEndpoint("/watch/api/user/updateavatar", server.apiUserUpdateAvatar, "POST", true)

	// API calls that change state of the player.
	server.HandleEndpoint("/watch/api/player/get", server.apiPlayerGet, "GET", true)
	server.HandleEndpoint("/watch/api/player/set", server.apiPlayerSet, "POST", true)
	server.HandleEndpoint("/watch/api/player/end", server.apiPlayerEnd, "POST", true)
	server.HandleEndpoint("/watch/api/player/next", server.apiPlayerNext, "POST", true)
	server.HandleEndpoint("/watch/api/player/play", server.apiPlayerPlay, "POST", true)
	server.HandleEndpoint("/watch/api/player/pause", server.apiPlayerPause, "POST", true)
	server.HandleEndpoint("/watch/api/player/seek", server.apiPlayerSeek, "POST", true)
	server.HandleEndpoint("/watch/api/player/autoplay", server.apiPlayerAutoplay, "POST", true)
	server.HandleEndpoint("/watch/api/player/looping", server.apiPlayerLooping, "POST", true)
	server.HandleEndpoint("/watch/api/player/updatetitle", server.apiPlayerUpdateTitle, "POST", true)

	// Subtitle API calls.
	server.HandleEndpoint("/watch/api/subtitle/delete", server.apiSubtitleDelete, "POST", true)
	server.HandleEndpoint("/watch/api/subtitle/update", server.apiSubtitleUpdate, "POST", true)
	server.HandleEndpoint("/watch/api/subtitle/attach", server.apiSubtitleAttach, "POST", true)
	server.HandleEndpoint("/watch/api/subtitle/shift", server.apiSubtitleShift, "POST", true)
	server.HandleEndpoint("/watch/api/subtitle/upload", server.apiSubtitleUpload, "POST", true)
	server.HandleEndpoint("/watch/api/subtitle/search", server.apiSubtitleSearch, "POST", true)

	// API calls that change state of the playlist.
	server.HandleEndpoint("/watch/api/playlist/get", server.apiPlaylistGet, "GET", true)
	server.HandleEndpoint("/watch/api/playlist/play", server.apiPlaylistPlay, "POST", true)
	server.HandleEndpoint("/watch/api/playlist/add", server.apiPlaylistAdd, "POST", true)
	server.HandleEndpoint("/watch/api/playlist/clear", server.apiPlaylistClear, "POST", true)
	server.HandleEndpoint("/watch/api/playlist/remove", server.apiPlaylistRemove, "POST", true)
	server.HandleEndpoint("/watch/api/playlist/shuffle", server.apiPlaylistShuffle, "POST", true)
	server.HandleEndpoint("/watch/api/playlist/move", server.apiPlaylistMove, "POST", true)
	server.HandleEndpoint("/watch/api/playlist/update", server.apiPlaylistUpdate, "POST", true)

	// API calls that change state of the history.
	server.HandleEndpoint("/watch/api/history/get", server.apiHistoryGet, "GET", true)
	server.HandleEndpoint("/watch/api/history/clear", server.apiHistoryClear, "POST", true)

	server.HandleEndpoint("/watch/api/chat/messagecreate", server.apiChatSend, "POST", true)
	server.HandleEndpoint("/watch/api/chat/get", server.apiChatGet, "GET", true)

	// Server events and proxy.
	server.HandleEndpoint("/watch/api/events", server.apiEvents, "GET", false)

	server.HandleEndpoint(PROXY_ROUTE, server.watchProxy, "GET", false)

	// Voice chat
	server.HandleEndpoint("/watch/vc", voiceChat, "GET", false)
}

func (server *Server) HandleEndpoint(pattern string, endpointHandler func(w http.ResponseWriter, r *http.Request), method string, requireAuth bool) {
	// TODO Investigate if this function is made on every call
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			errMsg := fmt.Sprintf("Method not allowed. %v was expected.", method)
			http.Error(w, errMsg, http.StatusMethodNotAllowed)
			return
		}

		if requireAuth && !server.isAuthorized(w, r) {
			return
		}
		endpointHandler(w, r)
	}
	http.HandleFunc(pattern, genericHandler)
}

func (server *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested server version.", r.RemoteAddr)
	uptimeString := fmt.Sprintf("%v_%v", VERSION, BuildTime)
	response, _ := json.Marshal(uptimeString)
	io.WriteString(w, string(response))
}

func (server *Server) apiUptime(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested server version.", r.RemoteAddr)
	uptime := time.Now().Sub(startTime)
	uptimeString := fmt.Sprintf("%v", uptime)
	response, _ := json.Marshal(uptimeString)
	io.WriteString(w, string(response))
}

func (server *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s attempted to log in.", r.RemoteAddr)
	io.WriteString(w, "This is unimplemented")
}

func (server *Server) apiUploadMedia(w http.ResponseWriter, r *http.Request) {
	inputFile, headers, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	extension := path.Ext(headers.Filename)
	directory := getMediaType(extension)
	filename := headers.Filename

	outputPath, isSafe := safeJoin("web", "media", directory, filename)
	if checkTraversal(w, isSafe) {
		return
	}
	os.MkdirAll("web/media/"+directory, 0750)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer outputFile.Close()

	LogInfo("Saving uploaded media file to: %v.", outputPath)

	// TODO(kihau): Copy the input file in smaller parts instead.
	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	networkPath, isSafe := safeJoin("media", directory, filename)
	if checkTraversal(w, isSafe) {
		return
	}
	jsonData, _ := json.Marshal(networkPath)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiUserCreate(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user creation.", r.RemoteAddr)

	server.users.mutex.Lock()
	user := server.users.create()
	server.users.mutex.Unlock()

	tokenJson, err := json.Marshal(user.token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	io.WriteString(w, string(tokenJson))
	server.writeEventToAllConnections(w, "usercreate", user)
}

func (server *Server) apiUserGetAll(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user get all.", r.RemoteAddr)

	server.users.mutex.Lock()
	usersJson, err := json.Marshal(server.users.slice)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		server.users.mutex.Unlock()
		return
	}
	server.users.mutex.Unlock()

	io.WriteString(w, string(usersJson))
}

func (server *Server) apiUserVerify(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection requested %s user verification.", r.RemoteAddr)

	jsonData, err := json.Marshal(user.Id)
	if err != nil {
		LogError("Failed to serialize json data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiUserUpdateName(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user name change.", r.RemoteAddr)

	var newUsername string
	if !server.readJsonDataFromRequest(w, r, &newUsername) {
		return
	}

	server.users.mutex.Lock()
	userIndex := server.getAuthorizedIndex(w, r)

	if userIndex == -1 {
		server.users.mutex.Unlock()
		return
	}

	server.users.slice[userIndex].Username = newUsername
	server.users.slice[userIndex].lastUpdate = time.Now()
	user := server.users.slice[userIndex]
	server.users.mutex.Unlock()

	io.WriteString(w, "Username updated")
	server.writeEventToAllConnections(w, "userupdate", user)
}

func (server *Server) apiUserUpdateAvatar(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user avatar change.", r.RemoteAddr)

	server.users.mutex.Lock()
	userIndex := server.getAuthorizedIndex(w, r)
	if userIndex == -1 {
		server.users.mutex.Unlock()
		return
	}
	user := server.users.slice[userIndex]
	server.users.mutex.Unlock()

	formfile, _, err := r.FormFile("file")
	if err != nil {
		LogError("File to read from data from user avatar change request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	os.Mkdir("web/users/", os.ModePerm)
	avatarUrl := fmt.Sprintf("web/users/avatar%v", user.Id)

	os.Remove(avatarUrl)
	file, err := os.Create(avatarUrl)
	if err != nil {
		LogError("Failed to create avatar file: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer file.Close()
	io.Copy(file, formfile)

	now := time.Now()
	avatarUrl = fmt.Sprintf("users/avatar%v?%v", user.Id, now)

	server.users.mutex.Lock()
	server.users.slice[userIndex].Avatar = avatarUrl
	server.users.slice[userIndex].lastUpdate = time.Now()
	user = server.users.slice[userIndex]
	server.users.mutex.Unlock()

	jsonData, _ := json.Marshal(avatarUrl)

	io.WriteString(w, string(jsonData))
	server.writeEventToAllConnections(w, "userupdate", user)
}

func (server *Server) apiPlayerGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested get.", r.RemoteAddr)

	server.state.mutex.Lock()
	getEvent := PlayerGetResponseData{
		Player: server.state.player,
		Entry:  server.state.entry,
		// Subtitles: getSubtitles(),
	}
	server.state.mutex.Unlock()

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		LogError("Failed to serialize get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) setNewEntry(newEntry *Entry) Entry {
	prevEntry := server.state.entry

	if prevEntry.Url != "" {
		server.state.history = append(server.state.history, prevEntry)
	}

	// TODO(kihau): Proper proxy setup for youtube entries.

	lastSegment := lastUrlSegment(newEntry.Url)
	if newEntry.UseProxy {
		if strings.HasSuffix(lastSegment, ".m3u8") {
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

func (server *Server) apiPlayerSet(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested media url change.", r.RemoteAddr)

	var data PlayerSetRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	server.state.entryId += 1
	id := server.state.entryId
	server.state.mutex.Unlock()

	newEntry := Entry{
		Id:         id,
		Url:        data.RequestEntry.Url,
		UserId:     user.Id,
		Title:      data.RequestEntry.Title,
		UseProxy:   data.RequestEntry.UseProxy,
		RefererUrl: data.RequestEntry.RefererUrl,
		SourceUrl:  "",
		Subtitles:  data.RequestEntry.Subtitles,
		Created:    time.Now(),
	}

	newEntry.Title = constructTitleWhenMissing(&newEntry)

	server.loadYoutubeEntry(&newEntry, data.RequestEntry)

	server.state.mutex.Lock()
	if server.state.entry.Url != "" && server.state.player.Looping {
		server.state.playlist = append(server.state.playlist, server.state.entry)
	}

	prevEntry := server.setNewEntry(&newEntry)
	server.state.mutex.Unlock()

	LogInfo("New url is now: '%s'.", server.state.entry.Url)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	server.writeEventToAllConnections(w, "playerset", setEvent)
	io.WriteString(w, "{}")
}

func (server *Server) apiPlayerEnd(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s reported that video ended.", r.RemoteAddr)

	var data PlaybackEnded
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}
	server.state.mutex.Lock()
	if data.EntryId == server.state.entry.Id {
		server.state.player.Playing = false
	}
	server.state.mutex.Unlock()
}

func (server *Server) apiPlayerNext(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist next.", r.RemoteAddr)

	var data PlayerNextRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	// NOTE(kihau):
	//     We need to check whether currently set entry ID on the clent side matches current entry ID on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	if server.state.entry.Id != data.EntryId {
		LogWarn("Current entry ID on the server is not equal to the one provided by the client.")
		return
	}

	server.state.mutex.Lock()
	if server.state.entry.Url != "" && server.state.player.Looping {
		server.state.playlist = append(server.state.playlist, server.state.entry)
	}

	newEntry := Entry{}
	if len(server.state.playlist) != 0 {
		newEntry = server.state.playlist[0]
		server.state.playlist = server.state.playlist[1:]
	}

	server.loadYoutubeEntry(&newEntry, RequestEntry{})
	prevEntry := server.setNewEntry(&newEntry)
	server.state.mutex.Unlock()

	nextEvent := PlayerNextEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	server.writeEventToAllConnections(w, "playernext", nextEvent)
	io.WriteString(w, "{}")
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlayerPlay(w http.ResponseWriter, r *http.Request) {

	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player start.", r.RemoteAddr)

	var data SyncRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.updatePlayerState(true, data.Timestamp)
	event := server.createSyncEvent("play", user.Id)

	io.WriteString(w, "Broadcasting start!\n")
	server.writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func (server *Server) apiPlayerPause(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player pause.", r.RemoteAddr)

	var data SyncRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.updatePlayerState(false, data.Timestamp)
	event := server.createSyncEvent("pause", user.Id)

	io.WriteString(w, "Broadcasting pause!\n")
	server.writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func (server *Server) apiPlayerSeek(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player seek.", r.RemoteAddr)

	var data SyncRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	server.state.player.Timestamp = data.Timestamp
	server.state.lastUpdate = time.Now()
	server.state.mutex.Unlock()

	event := server.createSyncEvent("seek", user.Id)

	io.WriteString(w, "Broadcasting seek!\n")
	server.writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func (server *Server) apiPlayerAutoplay(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist autoplay.", r.RemoteAddr)

	var autoplay bool
	if !server.readJsonDataFromRequest(w, r, &autoplay) {
		return
	}

	LogInfo("Setting playlist autoplay to %v.", autoplay)

	server.state.mutex.Lock()
	server.state.player.Autoplay = autoplay
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "playerautoplay", autoplay)
}

func (server *Server) apiPlayerLooping(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist looping.", r.RemoteAddr)

	var looping bool
	if !server.readJsonDataFromRequest(w, r, &looping) {
		return
	}

	LogInfo("Setting playlist looping to %v.", looping)

	server.state.mutex.Lock()
	server.state.player.Looping = looping
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "playerlooping", looping)
}

func (server *Server) apiPlayerUpdateTitle(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested title update.", r.RemoteAddr)

	var title string
	if !server.readJsonDataFromRequest(w, r, &title) {
		return
	}

	server.state.mutex.Lock()
	server.state.entry.Title = title
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "playerupdatetitle", title)
}

func (server *Server) apiSubtitleDelete(w http.ResponseWriter, r *http.Request) {
	var subId uint64
	if !server.readJsonDataFromRequest(w, r, &subId) {
		return
	}

	server.state.mutex.Lock()
	for i, sub := range server.state.entry.Subtitles {
		if sub.Id == subId {
			subs := server.state.entry.Subtitles
			server.state.entry.Subtitles = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitledelete", subId)
}

func (server *Server) apiSubtitleUpdate(w http.ResponseWriter, r *http.Request) {
	var data SubtitleUpdateRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	for i, sub := range server.state.entry.Subtitles {
		if sub.Id == data.Id {
			server.state.entry.Subtitles[i].Name = data.Name
			break
		}
	}
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleupdate", data)
}

func (server *Server) apiSubtitleAttach(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested attach sub.", r.RemoteAddr)

	var subtitle Subtitle
	if !server.readJsonDataFromRequest(w, r, &subtitle) {
		return
	}

	server.state.mutex.Lock()
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleattach", subtitle)
}

func (server *Server) apiSubtitleShift(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested attach sub.", r.RemoteAddr)

	var data SubtitleShiftRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	for i, sub := range server.state.entry.Subtitles {
		if sub.Id == data.Id {
			server.state.entry.Subtitles[i].Shift = data.Shift
			break
		}
	}
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleshift", data)
}

func (server *Server) apiSubtitleSearch(w http.ResponseWriter, r *http.Request) {
	if !subsEnabled {
		http.Error(w, "Feature unavailable", http.StatusServiceUnavailable)
		return
	}

	var search Search
	if !server.readJsonDataFromRequest(w, r, &search) {
		return
	}

	os.MkdirAll("web/media/subs", 0750)
	downloadPath, err := downloadSubtitle(&search, "web/media/subs")
	if err != nil {
		respondBadRequest(w, "Subtitle download failed: %v", err)
		return
	}

	os.MkdirAll("web/subs/", 0750)
	inputSub, err := os.Open(downloadPath)
	if err != nil {
		respondInternalError(w, "Failed to open downloaded subtitle %v: %v", downloadPath, err)
		return
	}
	defer inputSub.Close()

	extension := path.Ext(downloadPath)
	subId := server.state.subsId.Add(1)

	outputName := fmt.Sprintf("subtitle%v%v", subId, extension)
	outputPath := path.Join("web", "subs", outputName)

	outputSub, err := os.Create(outputPath)
	if err != nil {
		respondInternalError(w, "Failed to created output subtitle in %v: %v", outputPath, err)
		return
	}

	_, err = io.Copy(outputSub, inputSub)
	if err != nil {
		respondInternalError(w, "Failed to copy downloaded subtitle file: %v", err)
		outputSub.Close()
		return
	}

	outputSub.Close()

	outputUrl := path.Join("subs", outputName)
	baseName := filepath.Base(downloadPath)
	subtitleName := strings.TrimSuffix(baseName, extension)

	subtitle := Subtitle{
		Id:    subId,
		Name:  subtitleName,
		Url:   outputUrl,
		Shift: 0.0,
	}

	server.state.mutex.Lock()
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleattach", subtitle)
	io.WriteString(w, "{}")
}

func (server *Server) apiSubtitleUpload(w http.ResponseWriter, r *http.Request) {

	networkFile, headers, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if headers.Size > SUBTITLE_SIZE_LIMIT {
		http.Error(w, "Subtitle file is too large", http.StatusRequestEntityTooLarge)
		return
	}

	filename := headers.Filename
	extension := path.Ext(filename)
	subId := server.state.subsId.Add(1)

	outputName := fmt.Sprintf("subtitle%v%v", subId, extension)
	outputPath, isSafe := safeJoin("web", "subs", outputName)
	if checkTraversal(w, isSafe) {
		return
	}

	os.MkdirAll("web/subs/", 0750)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer outputFile.Close()

	LogInfo("Saving uploaded subtitle file to: %v.", outputPath)

	// Read the file content to ensure it doesn't exceed the limit
	buf := make([]byte, headers.Size)
	_, err = io.ReadFull(networkFile, buf)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusBadRequest)
		return
	}

	_, err = outputFile.Write(buf)
	if err != nil {
		fmt.Println("Error: Failed to write to a subtitle file:", err)
		http.Error(w, "Error writing file", http.StatusInternalServerError)
		return
	}

	networkUrl, isSafe := safeJoin("subs", outputName)
	if checkTraversal(w, isSafe) {
		return
	}

	subtitle := Subtitle{
		Id:    subId,
		Name:  strings.TrimSuffix(filename, extension),
		Url:   networkUrl,
		Shift: 0.0,
	}

	jsonData, _ := json.Marshal(subtitle)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiPlaylistGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist get.", r.RemoteAddr)

	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.playlist)
	server.state.mutex.Unlock()

	if err != nil {
		LogWarn("Failed to serialize playlist get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiPlaylistPlay(w http.ResponseWriter, r *http.Request) {
	var data PlaylistPlayRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	LogInfo("Connection %s requested playlist play.", r.RemoteAddr)

	server.state.mutex.Lock()
	if data.Index < 0 || data.Index >= len(server.state.playlist) {
		LogError("Failed to remove playlist element at index %v.", data.Index)
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[data.Index].Id != data.EntryId {
		LogWarn("Entry ID on the server is not equal to the one provided by the client.")
		server.state.mutex.Unlock()
		return
	}

	if server.state.entry.Url != "" && server.state.player.Looping {
		server.state.playlist = append(server.state.playlist, server.state.entry)
	}

	newEntry := server.state.playlist[data.Index]
	server.loadYoutubeEntry(&newEntry, RequestEntry{})
	prevEntry := server.setNewEntry(&newEntry)
	server.state.playlist = append(server.state.playlist[:data.Index], server.state.playlist[data.Index+1:]...)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("remove", data.Index)
	server.writeEventToAllConnections(w, "playlist", event)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	server.writeEventToAllConnections(w, "playerset", setEvent)
	io.WriteString(w, "{}")
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistAdd(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist add.", r.RemoteAddr)

	var data PlaylistAddRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	localDir, path := isLocalDirectory(data.RequestEntry.Url)
	if data.RequestEntry.IsPlaylist && localDir {
		LogInfo("Adding directory '%s' to the playlist.", path)
		localEntries := server.getEntriesFromDirectory(path, user.Id)

		server.state.mutex.Lock()
		server.state.playlist = append(server.state.playlist, localEntries...)
		server.state.mutex.Unlock()

		event := createPlaylistEvent("addmany", localEntries)
		server.writeEventToAllConnections(w, "playlist", event)
	} else {
		LogInfo("Adding '%s' url to the playlist.", data.RequestEntry.Url)

		server.state.mutex.Lock()
		server.state.entryId += 1
		id := server.state.entryId
		server.state.mutex.Unlock()

		newEntry := Entry{
			Id:         id,
			Url:        data.RequestEntry.Url,
			UserId:     user.Id,
			Title:      data.RequestEntry.Title,
			UseProxy:   data.RequestEntry.UseProxy,
			RefererUrl: data.RequestEntry.RefererUrl,
			SourceUrl:  "",
			Subtitles:  data.RequestEntry.Subtitles,
			Created:    time.Now(),
		}

		newEntry.Title = constructTitleWhenMissing(&newEntry)

		server.loadYoutubeEntry(&newEntry, data.RequestEntry)

		server.state.mutex.Lock()
		server.state.playlist = append(server.state.playlist, newEntry)
		server.state.mutex.Unlock()

		event := createPlaylistEvent("add", newEntry)
		server.writeEventToAllConnections(w, "playlist", event)
	}
}

func (server *Server) apiPlaylistClear(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist clear.", r.RemoteAddr)

	var connectionId uint64
	if !server.readJsonDataFromRequest(w, r, &connectionId) {
		return
	}

	server.state.mutex.Lock()
	server.state.playlist = server.state.playlist[:0]
	server.state.mutex.Unlock()

	event := createPlaylistEvent("clear", nil)
	server.writeEventToAllConnections(w, "playlist", event)
}

func (server *Server) apiPlaylistRemove(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist remove.", r.RemoteAddr)

	var data PlaylistRemoveRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	if data.Index < 0 || data.Index >= len(server.state.playlist) {
		LogError("Failed to remove playlist element at index %v.", data.Index)
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[data.Index].Id != data.EntryId {
		LogWarn("Entry ID on the server is not equal to the one provided by the client.")
		server.state.mutex.Unlock()
		return
	}

	server.state.playlist = append(server.state.playlist[:data.Index], server.state.playlist[data.Index+1:]...)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("remove", data.Index)
	server.writeEventToAllConnections(w, "playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistShuffle(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist shuffle.", r.RemoteAddr)

	server.state.mutex.Lock()
	for i := range server.state.playlist {
		j := rand.Intn(i + 1)
		server.state.playlist[i], server.state.playlist[j] = server.state.playlist[j], server.state.playlist[i]
	}
	server.state.mutex.Unlock()

	event := createPlaylistEvent("shuffle", server.state.playlist)
	server.writeEventToAllConnections(w, "playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistMove(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist move.", r.RemoteAddr)

	var move PlaylistMoveRequestData
	if !server.readJsonDataFromRequest(w, r, &move) {
		return
	}

	server.state.mutex.Lock()
	if move.SourceIndex < 0 || move.SourceIndex >= len(server.state.playlist) {
		LogError("Playlist move failed, source index out of bounds")
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[move.SourceIndex].Id != move.EntryId {
		LogWarn("Entry ID on the server is not equal to the one provided by the client.")
		server.state.mutex.Unlock()
		return
	}

	if move.DestIndex < 0 || move.DestIndex >= len(server.state.playlist) {
		LogError("Playlist move failed, source index out of bounds")
		server.state.mutex.Unlock()
		return
	}

	entry := server.state.playlist[move.SourceIndex]

	// Remove element from the slice:
	server.state.playlist = append(server.state.playlist[:move.SourceIndex], server.state.playlist[move.SourceIndex+1:]...)

	list := make([]Entry, 0)

	// Appned removed element to a new list:
	list = append(list, server.state.playlist[:move.DestIndex]...)
	list = append(list, entry)
	list = append(list, server.state.playlist[move.DestIndex:]...)

	server.state.playlist = list
	server.state.mutex.Unlock()

	eventData := PlaylistMoveEventData{
		SourceIndex: move.SourceIndex,
		DestIndex:   move.DestIndex,
	}

	event := createPlaylistEvent("move", eventData)
	server.writeEventToAllConnectionsExceptSelf(w, "playlist", event, user.Id, move.ConnectionId)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistUpdate(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist update.", r.RemoteAddr)

	var data PlaylistUpdateRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	entry := data.Entry

	server.state.mutex.Lock()
	updatedEntry := Entry{Id: 0}

	for i := 0; i < len(server.state.playlist); i++ {
		if server.state.playlist[i].Id == entry.Id {
			server.state.playlist[i].Title = entry.Title
			server.state.playlist[i].Url = entry.Url
			updatedEntry = server.state.playlist[i]
			break
		}
	}

	server.state.mutex.Unlock()

	if updatedEntry.Id == 0 {
		LogWarn("Failed to find entry to update")
		return
	}

	event := createPlaylistEvent("update", entry)
	server.writeEventToAllConnectionsExceptSelf(w, "playlist", event, user.Id, data.ConnectionId)
}

func (server *Server) apiHistoryGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested history get.", r.RemoteAddr)

	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.history)
	server.state.mutex.Unlock()

	if err != nil {
		LogWarn("Failed to serialize history get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiHistoryClear(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested history clear.", r.RemoteAddr)

	server.state.mutex.Lock()
	server.state.history = server.state.history[:0]
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "historyclear", nil)
}

func (server *Server) isAuthorized(w http.ResponseWriter, r *http.Request) bool {
	server.users.mutex.Lock()
	index := server.getAuthorizedIndex(w, r)
	server.users.mutex.Unlock()

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

func checkTraversal(w http.ResponseWriter, isSafe bool) bool {
	if isSafe {
		return false
	}
	LogWarn("Traversal was attempted!")
	http.Error(w, "Invalid path", http.StatusUnprocessableEntity)
	return true
}

func (server *Server) getAuthorizedIndex(w http.ResponseWriter, r *http.Request) int {
	token := r.Header.Get("Authorization")
	if token == "" {
		LogError("Invalid token")
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return -1
	}

	for i, user := range server.users.slice {
		if user.token == token {
			return i
		}
	}

	LogError("Failed to find user")
	http.Error(w, "User not found", http.StatusUnauthorized)
	return -1
}

func (server *Server) readJsonDataFromRequest(w http.ResponseWriter, r *http.Request, data any) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		LogError("Request handler failed to read request body: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}

	if len(body) > BODY_LIMIT {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return false
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		LogError("Request handler failed to read json payload: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	server.conns.mutex.Lock()
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\nretry: %d\n\n", eventId, eventName, jsonString, RETRY)

	if err != nil {
		server.conns.mutex.Unlock()
		return err
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	server.conns.mutex.Unlock()

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
		fmt.Fprintln(conn.writer, event)

		if f, ok := conn.writer.(http.Flusher); ok {
			f.Flush()
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

		fmt.Fprintln(conn.writer, event)

		if f, ok := conn.writer.(http.Flusher); ok {
			f.Flush()
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

func isTrustedUrl(url string, parsedUrl *net_url.URL) bool {
	if strings.HasPrefix(url, MEDIA) || strings.HasPrefix(url, serverRootAddress) {
		return true
	}
	if parsedUrl != nil && parsedUrl.Host == serverDomain {
		return true
	}
	return false
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
	if isTrustedUrl(url, parsedUrl) {
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
		if len(m3u.tracks) == 0 {
			LogError("Master playlist contains 0 tracks!")
			return false
		}

		// Rarely tracks are not fully qualified
		originalMasterPlaylist := m3u.copy()
		m3u.prefixRelativeTracks(prefix)
		LogInfo("User provided a master playlist. The best track will be chosen based on quality.")
		bestTrack := m3u.getBestTrack()
		if bestTrack == nil {
			bestTrack = &m3u.tracks[0]
			url = bestTrack.url
		}
		m3u, err = downloadM3U(bestTrack.url, WEB_PROXY+ORIGINAL_M3U8, referer)
		var downloadErr *DownloadError
		if errors.As(err, &downloadErr) && downloadErr.Code == 404 {
			// Hacky trick for relative non-compliant m3u8's 💩
			domain := getRootDomain(parsedUrl)
			originalMasterPlaylist.prefixRelativeTracks(domain)
			bestTrack = originalMasterPlaylist.getBestTrack()
			if bestTrack == nil {
				bestTrack = &originalMasterPlaylist.tracks[0]
			}
			m3u, err = downloadM3U(bestTrack.url, WEB_PROXY+ORIGINAL_M3U8, referer)
			if err != nil {
				LogError("Fallback failed :( %v", err.Error())
				return false
			}
		} else if err != nil {
			LogError("Failed to fetch track from master playlist: %v", err.Error())
			return false
		}
		// Refreshing the prefix in case the newly assembled track consists of 2 or more components
		parsedUrl, err := net_url.Parse(bestTrack.url)
		if err != nil {
			LogError("Failed to parse URL from the best track, likely the segment is invalid: %v", err.Error())
			return false
		}
		prefix = stripLastSegment(parsedUrl)
	}
	// At this point it either succeeded or it already returned

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
	LogDebug("total duration: %v", m3u.totalDuration())

	// Sometimes m3u8 chunks are not fully qualified
	m3u.prefixRelativeSegments(prefix)

	server.state.isHls = true
	server.state.proxy = &HlsProxy{}
	var result bool
	if m3u.isLive {
		server.state.isLive = true
		result = server.setupLiveProxy(m3u, url)
	} else {
		server.state.isLive = false
		result = server.setupVodProxy(m3u)
	}
	duration := time.Since(start)
	LogDebug("Time taken to setup proxy: %v", duration)
	return result
}

func (server *Server) setupLiveProxy(m3u *M3U, liveUrl string) bool {
	proxy := server.state.proxy
	proxy.liveUrl = liveUrl
	proxy.liveSegments.Clear()
	proxy.randomizer.Store(0)
	return true
}

func (server *Server) setupVodProxy(m3u *M3U) bool {
	proxy := server.state.proxy
	segmentCount := len(m3u.segments)

	proxy.chunkLocks = make([]sync.Mutex, 0, segmentCount)
	proxy.originalChunks = make([]string, 0, segmentCount)
	proxy.fetchedChunks = make([]bool, 0, segmentCount)
	for i := 0; i < segmentCount; i++ {
		proxy.chunkLocks = append(proxy.chunkLocks, sync.Mutex{})
		proxy.originalChunks = append(proxy.originalChunks, m3u.segments[i].url)
		proxy.fetchedChunks = append(proxy.fetchedChunks, false)
		m3u.segments[i].url = "ch-" + toString(i)
	}

	m3u.serialize(WEB_PROXY + PROXY_M3U8)
	LogDebug("Prepared proxy file %v", PROXY_M3U8)
	return true
}

func (server *Server) apiEvents(w http.ResponseWriter, r *http.Request) {
	LogDebug("URL is %v", r.URL)

	token := r.URL.Query().Get("token")
	if token == "" {
		response := "Failed to parse token from the event url."
		http.Error(w, response, http.StatusInternalServerError)
		LogError("%v", response)
		return
	}

	server.users.mutex.Lock()
	userIndex := server.users.findIndex(token)
	if userIndex == -1 {
		http.Error(w, "User not found", http.StatusUnauthorized)
		LogError("Failed to connect to event stream. User not found.")
		server.users.mutex.Unlock()
		return
	}

	server.users.slice[userIndex].connections += 1

	was_online_before := server.users.slice[userIndex].Online
	is_online := server.users.slice[userIndex].connections != 0

	server.users.slice[userIndex].Online = is_online

	user := server.users.slice[userIndex]
	server.users.mutex.Unlock()

	server.conns.mutex.Lock()
	connectionId := server.conns.add(w, user.Id)
	connectionCount := len(server.conns.slice)
	server.conns.mutex.Unlock()

	LogInfo("New connection established with user %v on %s. Current connection count: %d", user.Id, r.RemoteAddr, connectionCount)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	welcomeErr := server.writeEvent(w, "userwelcome", connectionId)
	if welcomeErr != nil {
		return
	}

	if !was_online_before && is_online {
		server.writeEventToAllConnectionsExceptSelf(w, "userconnected", user.Id, user.Id, connectionId)
	}

	for {
		var event SyncEventData
		server.state.mutex.Lock()
		playing := server.state.player.Playing
		server.state.mutex.Unlock()

		if playing {
			event = server.createSyncEvent("play", 0)
		} else {
			event = server.createSyncEvent("pause", 0)
		}

		connectionErr := server.writeEvent(w, "sync", event)

		if connectionErr != nil {
			server.conns.mutex.Lock()
			server.conns.remove(connectionId)
			connectionCount = len(server.conns.slice)
			server.conns.mutex.Unlock()

			server.users.mutex.Lock()
			userIndex := server.users.findIndex(token)
			disconnected := false
			if userIndex != -1 {
				server.users.slice[userIndex].connections -= 1
				disconnected = server.users.slice[userIndex].connections == 0
				server.users.slice[userIndex].Online = !disconnected
			}
			server.users.mutex.Unlock()

			if disconnected {
				server.writeEventToAllConnectionsExceptSelf(w, "userdisconnected", user.Id, user.Id, connectionId)
			}

			LogInfo("Connection with user %v on %s dropped. Current connection count: %d", user.Id, r.RemoteAddr, connectionCount)
			LogDebug("Drop error message: %v", connectionErr)
			return
		}

		server.smartSleep()
	}
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
	if request.Method != "GET" {
		LogWarn("HlsProxy not called with GET, received: %v", request.Method)
		return
	}
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

type FetchedSegment struct {
	realUrl  string
	obtained bool
	mutex    sync.Mutex
	created  time.Time
}

func (server *Server) serveHlsLive(writer http.ResponseWriter, request *http.Request, chunk string) {
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

		liveM3U, err := downloadM3U(proxy.liveUrl, WEB_PROXY+ORIGINAL_M3U8, server.state.entry.RefererUrl)
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

		segmentCount := len(liveM3U.segments)
		for i := 0; i < segmentCount; i++ {
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
	fetchErr := downloadFile(fetchedChunk.realUrl, WEB_PROXY+chunk, server.state.entry.RefererUrl)
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
	segmentMap.Range(func(key, value interface{}) bool {
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

	if proxy.fetchedChunks[chunkId] {
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}

	mutex := &proxy.chunkLocks[chunkId]
	mutex.Lock()
	if proxy.fetchedChunks[chunkId] {
		mutex.Unlock()
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}
	fetchErr := downloadFile(proxy.originalChunks[chunkId], WEB_PROXY+chunk, server.state.entry.RefererUrl)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch chunk %v", fetchErr)
		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		}
		http.Error(writer, "Failed to fetch vod chunk", code)
		return
	}
	proxy.fetchedChunks[chunkId] = true
	mutex.Unlock()

	http.ServeFile(writer, request, WEB_PROXY+chunk)
}

const GENERIC_CHUNK_SIZE = 4 * MB

func (server *Server) serveGenericFile(writer http.ResponseWriter, request *http.Request, pathFile string) {
	proxy := &server.state.genericProxy
	if path.Ext(pathFile) != proxy.extensionWithDot {
		http.Error(writer, "Failed to fetch chunk", 404)
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
		response, err := openFileDownload(proxy.fileUrl, byteRange.start, server.state.entry.RefererUrl)
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
		for i := 0; i < len(proxy.contentRanges); i++ {
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
	for i := 0; i < len(proxy.contentRanges); i++ {
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
	for i := 0; i < len(proxy.contentRanges)-1; i++ {
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

const MAX_MESSAGE_CHARACTERS = 1000

func (server *Server) apiChatGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested messages.", r.RemoteAddr)

	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.messages)
	server.state.mutex.Unlock()

	if err != nil {
		LogWarn("Failed to serialize messages get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiChatSend(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s posted a chat message.", r.RemoteAddr)

	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var newMessage ChatMessageFromUser
	if !server.readJsonDataFromRequest(w, r, &newMessage) {
		return
	}
	if len(newMessage.Message) > MAX_MESSAGE_CHARACTERS {
		http.Error(w, "Message exceeds 1000 chars", http.StatusForbidden)
		return
	}

	server.state.mutex.Lock()
	server.state.messageId += 1
	chatMessage := ChatMessage{
		Id:       1,
		Message:  newMessage.Message,
		AuthorId: user.Id,
		UnixTime: time.Now().UnixMilli(),
		Edited:   false,
	}
	server.state.messages = append(server.state.messages, chatMessage)
	server.state.mutex.Unlock()
	server.writeEventToAllConnections(w, "messagecreate", chatMessage)
}

func isLocalDirectory(url string) (bool, string) {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		return false, ""
	}

	if !isTrustedUrl(url, parsedUrl) {
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

type ChatMessage struct {
	Message  string `json:"message"`
	UnixTime int64  `json:"unixTime"`
	Id       uint64 `json:"id"`
	AuthorId uint64 `json:"authorId"`
	Edited   bool   `json:"edited"`
}

type ChatMessageEdit struct {
	EditedMessage string `json:"editedMessage"`
	Id            uint64 `json:"id"`
}

type ChatMessageFromUser struct {
	Message string `json:"message"`
	Edited  bool   `json:"edited"`
}
