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

type PlayerState struct {
	Playing   bool    `json:"playing"`
	Autoplay  bool    `json:"autoplay"`
	Looping   bool    `json:"looping"`
	Timestamp float64 `json:"timestamp"`
}

type Entry struct {
	Id          uint64    `json:"id"`
	Url         string    `json:"url"`
	Title       string    `json:"title"`
	UserId      uint64    `json:"user_id"`
	UseProxy    bool      `json:"use_proxy"`
	RefererUrl  string    `json:"referer_url"`
	SourceUrl   string    `json:"source_url"`
	SubtitleUrl string    `json:"subtitle_url"`
	Thumbnail   string    `json:"thumbnail"`
	Created     time.Time `json:"created"`
}

type ServerState struct {
	mutex sync.RWMutex

	player  PlayerState
	entry   Entry
	entryId uint64

	eventId    atomic.Uint64
	lastUpdate time.Time

	playlist  []Entry
	history   []Entry
	messages  []ChatMessage
	messageId uint64

	proxy Proxy
}

type Proxy struct {
	// HLS proxy
	chunkLocks     []sync.Mutex
	fetchedChunks  []bool
	originalChunks []string
	isLive         bool
	// Live resources
	liveUrl      string
	liveSegments sync.Map
	randomizer   atomic.Int64
	lastRefresh  time.Time

	isHls     bool
	setupLock sync.Mutex

	// Generic proxy
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
	mutex     sync.RWMutex
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
	mutex     sync.RWMutex
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
	Url               string `json:"url"`
	Title             string `json:"title"`
	UseProxy          bool   `json:"use_proxy"`
	RefererUrl        string `json:"referer_url"`
	SubtitleUrl       string `json:"subtitle_url"`
	SearchVideo       bool   `json:"search_video"`
	IsPlaylist        bool   `json:"is_playlist"`
	AddToTop          bool   `json:"add_to_top"`
	PlaylistSkipCount uint   `json:"playlist_skip_count"`
	PlaylistMaxSize   uint   `json:"playlist_max_size"`
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

var state = ServerState{}
var users = makeUsers()
var conns = makeConnections()

// Constants - assignable only once!
var serverRootAddress string
var startTime = time.Now()
var subsEnabled bool

func StartServer(options *Options) {
	state.lastUpdate = time.Now()
	subsEnabled = options.Subs
	registerEndpoints(options)

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

func registerEndpoints(options *Options) {
	_ = options

	fileserver := http.FileServer(http.Dir("./web"))
	http.Handle("/watch/", http.StripPrefix("/watch/", fileserver))

	http.HandleFunc("/", handleUnknownEndpoint)

	// Unrelated API calls.
	http.HandleFunc("/watch/api/version", apiVersion)
	http.HandleFunc("/watch/api/login", apiLogin)
	http.HandleFunc("/watch/api/uploadmedia", apiUploadMedia)
	http.HandleFunc("/watch/api/uploadsubs", apiUploadSubs)
	http.HandleFunc("/watch/api/searchsubs", apiSearchSubs)

	// User related API calls.
	http.HandleFunc("/watch/api/user/create", apiUserCreate)
	http.HandleFunc("/watch/api/user/getall", apiUserGetAll)
	http.HandleFunc("/watch/api/user/verify", apiUserVerify)
	http.HandleFunc("/watch/api/user/updatename", apiUserUpdateName)
	http.HandleFunc("/watch/api/user/updateavatar", apiUserUpdateAvatar)

	// API calls that change state of the player.
	http.HandleFunc("/watch/api/player/get", apiPlayerGet)
	http.HandleFunc("/watch/api/player/set", apiPlayerSet)
	http.HandleFunc("/watch/api/player/end", apiPlayerEnd)
	http.HandleFunc("/watch/api/player/next", apiPlayerNext)
	http.HandleFunc("/watch/api/player/play", apiPlayerPlay)
	http.HandleFunc("/watch/api/player/pause", apiPlayerPause)
	http.HandleFunc("/watch/api/player/seek", apiPlayerSeek)
	http.HandleFunc("/watch/api/player/autoplay", apiPlayerAutoplay)
	http.HandleFunc("/watch/api/player/looping", apiPlayerLooping)

	// API calls that change state of the playlist.
	http.HandleFunc("/watch/api/playlist/get", apiPlaylistGet)
	http.HandleFunc("/watch/api/playlist/play", apiPlaylistPlay)
	http.HandleFunc("/watch/api/playlist/add", apiPlaylistAdd)
	http.HandleFunc("/watch/api/playlist/clear", apiPlaylistClear)
	http.HandleFunc("/watch/api/playlist/remove", apiPlaylistRemove)
	http.HandleFunc("/watch/api/playlist/shuffle", apiPlaylistShuffle)
	http.HandleFunc("/watch/api/playlist/move", apiPlaylistMove)
	http.HandleFunc("/watch/api/playlist/update", apiPlaylistUpdate)

	// API calls that change state of the history.
	http.HandleFunc("/watch/api/history/get", apiHistoryGet)
	http.HandleFunc("/watch/api/history/clear", apiHistoryClear)

	http.HandleFunc("/watch/api/chat/messagecreate", apiChatSend)
	http.HandleFunc("/watch/api/chat/get", apiChatGet)

	// Server events and proxy.
	http.HandleFunc("/watch/api/events", apiEvents)
	http.HandleFunc(PROXY_ROUTE, watchProxy)
}

func apiVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s requested server version.", r.RemoteAddr)
	uptime := time.Now().Sub(startTime)
	response := fmt.Sprintf("%v Uptime=%v", VERSION, uptime)
	io.WriteString(w, response)
}

func apiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s attempted to log in.", r.RemoteAddr)
	io.WriteString(w, "This is unimplemented")
}

func apiUploadMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST was expected", http.StatusMethodNotAllowed)
		return
	}

	if !isAuthorized(w, r) {
		return
	}

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

func apiUploadSubs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST was expected", http.StatusMethodNotAllowed)
		return
	}

	if !isAuthorized(w, r) {
		return
	}

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

	outputPath, isSafe := safeJoin("web", "subs", filename)
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

	networkPath, isSafe := safeJoin("subs", filename)
	if checkTraversal(w, isSafe) {
		return
	}
	jsonData, _ := json.Marshal(networkPath)
	io.WriteString(w, string(jsonData))
}

func apiSearchSubs(w http.ResponseWriter, r *http.Request) {
	if !subsEnabled {
		http.Error(w, "Feature unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "POST was expected", http.StatusMethodNotAllowed)
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	var search Search
	if !readJsonDataFromRequest(w, r, &search) {
		return
	}

	os.MkdirAll("web/media/subs", 0750)
	subtitlePath, err := downloadSubtitle("subs", &search)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Expect it to be directed to /subs/
	servedPath, isSafe := safeJoin("media/subs", filepath.Base(subtitlePath))
	if checkTraversal(w, isSafe) {
		return
	}

	jsonData, err := json.Marshal(servedPath)
	io.WriteString(w, string(jsonData))
}

func apiUserCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection requested %s user creation.", r.RemoteAddr)

	users.mutex.Lock()
	user := users.create()
	users.mutex.Unlock()

	tokenJson, err := json.Marshal(user.token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	io.WriteString(w, string(tokenJson))
	writeEventToAllConnections(w, "usercreate", user)
}

func apiUserGetAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection requested %s user get all.", r.RemoteAddr)

	users.mutex.Lock()
	usersJson, err := json.Marshal(users.slice)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		users.mutex.Unlock()
		return
	}
	users.mutex.Unlock()

	io.WriteString(w, string(usersJson))
}

func apiUserVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
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

func apiUserUpdateName(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	LogInfo("Connection requested %s user name change.", r.RemoteAddr)

	var newUsername string
	if !readJsonDataFromRequest(w, r, &newUsername) {
		return
	}

	users.mutex.Lock()
	userIndex := getAuthorizedIndex(w, r)

	if userIndex == -1 {
		users.mutex.Unlock()
		return
	}

	users.slice[userIndex].Username = newUsername
	users.slice[userIndex].lastUpdate = time.Now()
	user := users.slice[userIndex]
	users.mutex.Unlock()

	io.WriteString(w, "Username updated")
	writeEventToAllConnections(w, "userupdate", user)
}

func apiUserUpdateAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	LogInfo("Connection requested %s user avatar change.", r.RemoteAddr)

	users.mutex.Lock()
	userIndex := getAuthorizedIndex(w, r)
	if userIndex == -1 {
		users.mutex.Unlock()
		return
	}
	user := users.slice[userIndex]
	users.mutex.Unlock()

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

	users.mutex.Lock()
	users.slice[userIndex].Avatar = avatarUrl
	users.slice[userIndex].lastUpdate = time.Now()
	user = users.slice[userIndex]
	users.mutex.Unlock()

	jsonData, _ := json.Marshal(avatarUrl)

	io.WriteString(w, string(jsonData))
	writeEventToAllConnections(w, "userupdate", user)
}

func apiPlayerGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested get.", r.RemoteAddr)

	state.mutex.RLock()
	getEvent := PlayerGetResponseData{
		Player: state.player,
		Entry:  state.entry,
		// Subtitles: getSubtitles(),
	}
	state.mutex.RUnlock()

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		LogError("Failed to serialize get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func setNewEntry(newEntry Entry) Entry {
	prevEntry := state.entry

	if prevEntry.Url != "" {
		state.history = append(state.history, prevEntry)
	}

	// TODO(kihau): Proper proxy setup for youtube entries.

	lastSegment := lastUrlSegment(newEntry.Url)
	if newEntry.UseProxy {
		if strings.HasSuffix(lastSegment, ".m3u8") {
			setup := setupHlsProxy(newEntry.Url, newEntry.RefererUrl)
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
				newEntry.SourceUrl = PROXY_ROUTE + "proxy" + state.proxy.extensionWithDot
				LogInfo("Generic file proxy setup was successful.")
			} else {
				LogWarn("Generic file proxy setup failed!")
			}
		}
	}

	state.entry = newEntry

	state.player.Timestamp = 0
	state.lastUpdate = time.Now()
	state.player.Playing = state.player.Autoplay

	return prevEntry
}

func apiPlayerSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested media url change.", r.RemoteAddr)

	var data PlayerSetRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	state.mutex.Lock()
	state.entryId += 1
	id := state.entryId
	state.mutex.Unlock()

	newEntry := Entry{
		Id:          id,
		Url:         data.RequestEntry.Url,
		UserId:      user.Id,
		Title:       data.RequestEntry.Title,
		UseProxy:    data.RequestEntry.UseProxy,
		RefererUrl:  data.RequestEntry.RefererUrl,
		SourceUrl:   "",
		SubtitleUrl: data.RequestEntry.SubtitleUrl,
		Created:     time.Now(),
	}

	newEntry.Title = constructTitleWhenMissing(&newEntry)

	loadYoutubeEntry(&newEntry, data.RequestEntry)

	state.mutex.Lock()
	if state.entry.Url != "" && state.player.Looping {
		state.playlist = append(state.playlist, state.entry)
	}

	prevEntry := setNewEntry(newEntry)
	state.mutex.Unlock()

	LogInfo("New url is now: '%s'.", state.entry.Url)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  state.entry,
	}
	writeEventToAllConnections(w, "playerset", setEvent)
	io.WriteString(w, "{}")
}

func apiPlayerEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s reported that video ended.", r.RemoteAddr)

	var data PlaybackEnded
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}
	state.mutex.Lock()
	if data.EntryId == state.entry.Id {
		state.player.Playing = false
	}
	state.mutex.Unlock()
}

func apiPlayerNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist next.", r.RemoteAddr)

	var data PlayerNextRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	// NOTE(kihau):
	//     We need to check whether currently set entry ID on the clent side matches current entry ID on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	if state.entry.Id != data.EntryId {
		LogWarn("Current entry ID on the server is not equal to the one provided by the client.")
		return
	}

	state.mutex.Lock()
	if state.entry.Url != "" && state.player.Looping {
		state.playlist = append(state.playlist, state.entry)
	}

	newEntry := Entry{}
	if len(state.playlist) != 0 {
		newEntry = state.playlist[0]
		state.playlist = state.playlist[1:]
	}

	loadYoutubeEntry(&newEntry, RequestEntry{})
	prevEntry := setNewEntry(newEntry)

	state.mutex.Unlock()

	nextEvent := PlayerNextEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	writeEventToAllConnections(w, "playernext", nextEvent)
	io.WriteString(w, "{}")
	go preloadYoutubeSourceOnNextEntry()
}

func apiPlayerPlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player start.", r.RemoteAddr)

	var data SyncRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	updatePlayerState(true, data.Timestamp)
	event := createSyncEvent("play", user.Id)

	io.WriteString(w, "Broadcasting start!\n")
	writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func apiPlayerPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player pause.", r.RemoteAddr)

	var data SyncRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	updatePlayerState(false, data.Timestamp)
	event := createSyncEvent("pause", user.Id)

	io.WriteString(w, "Broadcasting pause!\n")
	writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func apiPlayerSeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player seek.", r.RemoteAddr)

	var data SyncRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	state.mutex.Lock()
	state.player.Timestamp = data.Timestamp
	state.lastUpdate = time.Now()
	state.mutex.Unlock()

	event := createSyncEvent("seek", user.Id)

	io.WriteString(w, "Broadcasting seek!\n")
	writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func apiPlayerAutoplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist autoplay.", r.RemoteAddr)

	var autoplay bool
	if !readJsonDataFromRequest(w, r, &autoplay) {
		return
	}

	LogInfo("Setting playlist autoplay to %v.", autoplay)

	state.mutex.Lock()
	state.player.Autoplay = autoplay
	state.mutex.Unlock()

	writeEventToAllConnections(w, "playerautoplay", autoplay)
}

func apiPlayerLooping(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist looping.", r.RemoteAddr)

	var looping bool
	if !readJsonDataFromRequest(w, r, &looping) {
		return
	}

	LogInfo("Setting playlist looping to %v.", looping)

	state.mutex.Lock()
	state.player.Looping = looping
	state.mutex.Unlock()

	writeEventToAllConnections(w, "playerlooping", looping)
}

func apiPlaylistGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s requested playlist get.", r.RemoteAddr)

	state.mutex.RLock()
	jsonData, err := json.Marshal(state.playlist)
	state.mutex.RUnlock()

	if err != nil {
		LogWarn("Failed to serialize playlist get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func apiPlaylistPlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	var data PlaylistPlayRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	LogInfo("Connection %s requested playlist play.", r.RemoteAddr)

	state.mutex.Lock()
	if data.Index < 0 || data.Index >= len(state.playlist) {
		LogError("Failed to remove playlist element at index %v.", data.Index)
		state.mutex.Unlock()
		return
	}

	if state.playlist[data.Index].Id != data.EntryId {
		LogWarn("Entry ID on the server is not equal to the one provided by the client.")
		state.mutex.Unlock()
		return
	}

	if state.entry.Url != "" && state.player.Looping {
		state.playlist = append(state.playlist, state.entry)
	}

	newEntry := state.playlist[data.Index]
	loadYoutubeEntry(&newEntry, RequestEntry{})
	prevEntry := setNewEntry(newEntry)
	state.playlist = append(state.playlist[:data.Index], state.playlist[data.Index+1:]...)
	state.mutex.Unlock()

	event := createPlaylistEvent("remove", data.Index)
	writeEventToAllConnections(w, "playlist", event)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  state.entry,
	}
	writeEventToAllConnections(w, "playerset", setEvent)
	io.WriteString(w, "{}")
	go preloadYoutubeSourceOnNextEntry()
}

func apiPlaylistAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist add.", r.RemoteAddr)

	var data PlaylistAddRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	localDir, path := isLocalDirectory(data.RequestEntry.Url)
	if data.RequestEntry.IsPlaylist && localDir {
		LogInfo("Adding directory '%s' to the playlist.", path)
		localEntries := getEntriesFromDirectory(path, user.Id)

		state.mutex.Lock()
		state.playlist = append(state.playlist, localEntries...)
		state.mutex.Unlock()

		event := createPlaylistEvent("addmany", localEntries)
		writeEventToAllConnections(w, "playlist", event)
	} else {
		LogInfo("Adding '%s' url to the playlist.", data.RequestEntry.Url)

		state.mutex.Lock()
		state.entryId += 1
		id := state.entryId
		state.mutex.Unlock()

		newEntry := Entry{
			Id:          id,
			Url:         data.RequestEntry.Url,
			UserId:      user.Id,
			Title:       data.RequestEntry.Title,
			UseProxy:    data.RequestEntry.UseProxy,
			RefererUrl:  data.RequestEntry.RefererUrl,
			SourceUrl:   "",
			SubtitleUrl: data.RequestEntry.SubtitleUrl,
			Created:     time.Now(),
		}

		newEntry.Title = constructTitleWhenMissing(&newEntry)

		loadYoutubeEntry(&newEntry, data.RequestEntry)

		state.mutex.Lock()
		state.playlist = append(state.playlist, newEntry)
		state.mutex.Unlock()

		event := createPlaylistEvent("add", newEntry)
		writeEventToAllConnections(w, "playlist", event)
	}
}

func apiPlaylistClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist clear.", r.RemoteAddr)

	var connectionId uint64
	if !readJsonDataFromRequest(w, r, &connectionId) {
		return
	}

	state.mutex.Lock()
	state.playlist = state.playlist[:0]
	state.mutex.Unlock()

	event := createPlaylistEvent("clear", nil)
	writeEventToAllConnections(w, "playlist", event)
}

func apiPlaylistRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist remove.", r.RemoteAddr)

	var data PlaylistRemoveRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	state.mutex.Lock()
	if data.Index < 0 || data.Index >= len(state.playlist) {
		LogError("Failed to remove playlist element at index %v.", data.Index)
		state.mutex.Unlock()
		return
	}

	if state.playlist[data.Index].Id != data.EntryId {
		LogWarn("Entry ID on the server is not equal to the one provided by the client.")
		state.mutex.Unlock()
		return
	}

	state.playlist = append(state.playlist[:data.Index], state.playlist[data.Index+1:]...)
	state.mutex.Unlock()

	event := createPlaylistEvent("remove", data.Index)
	writeEventToAllConnections(w, "playlist", event)
	go preloadYoutubeSourceOnNextEntry()
}

func apiPlaylistShuffle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist shuffle.", r.RemoteAddr)

	state.mutex.Lock()
	for i := range state.playlist {
		j := rand.Intn(i + 1)
		state.playlist[i], state.playlist[j] = state.playlist[j], state.playlist[i]
	}
	state.mutex.Unlock()

	event := createPlaylistEvent("shuffle", state.playlist)
	writeEventToAllConnections(w, "playlist", event)
	go preloadYoutubeSourceOnNextEntry()
}

func apiPlaylistMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist move.", r.RemoteAddr)

	var move PlaylistMoveRequestData
	if !readJsonDataFromRequest(w, r, &move) {
		return
	}

	state.mutex.Lock()
	if move.SourceIndex < 0 || move.SourceIndex >= len(state.playlist) {
		LogError("Playlist move failed, source index out of bounds")
		state.mutex.Unlock()
		return
	}

	if state.playlist[move.SourceIndex].Id != move.EntryId {
		LogWarn("Entry ID on the server is not equal to the one provided by the client.")
		state.mutex.Unlock()
		return
	}

	if move.DestIndex < 0 || move.DestIndex >= len(state.playlist) {
		LogError("Playlist move failed, source index out of bounds")
		state.mutex.Unlock()
		return
	}

	entry := state.playlist[move.SourceIndex]

	// Remove element from the slice:
	state.playlist = append(state.playlist[:move.SourceIndex], state.playlist[move.SourceIndex+1:]...)

	list := make([]Entry, 0)

	// Appned removed element to a new list:
	list = append(list, state.playlist[:move.DestIndex]...)
	list = append(list, entry)
	list = append(list, state.playlist[move.DestIndex:]...)

	state.playlist = list
	state.mutex.Unlock()

	eventData := PlaylistMoveEventData{
		SourceIndex: move.SourceIndex,
		DestIndex:   move.DestIndex,
	}

	event := createPlaylistEvent("move", eventData)
	writeEventToAllConnectionsExceptSelf(w, "playlist", event, user.Id, move.ConnectionId)
	go preloadYoutubeSourceOnNextEntry()
}

func apiPlaylistUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist update.", r.RemoteAddr)

	var data PlaylistUpdateRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	entry := data.Entry

	state.mutex.Lock()
	updatedEntry := Entry{Id: 0}

	for i := 0; i < len(state.playlist); i++ {
		if state.playlist[i].Id == entry.Id {
			state.playlist[i].Title = entry.Title
			state.playlist[i].Url = entry.Url
			updatedEntry = state.playlist[i]
			break
		}
	}

	state.mutex.Unlock()

	if updatedEntry.Id == 0 {
		LogWarn("Failed to find entry to update")
		return
	}

	event := createPlaylistEvent("update", entry)
	writeEventToAllConnectionsExceptSelf(w, "playlist", event, user.Id, data.ConnectionId)
}

func apiHistoryGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s requested history get.", r.RemoteAddr)

	conns.mutex.RLock()
	jsonData, err := json.Marshal(state.history)
	conns.mutex.RUnlock()

	if err != nil {
		LogWarn("Failed to serialize history get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func apiHistoryClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested history clear.", r.RemoteAddr)

	state.mutex.Lock()
	state.history = state.history[:0]
	state.mutex.Unlock()

	writeEventToAllConnections(w, "historyclear", nil)
}

func isAuthorized(w http.ResponseWriter, r *http.Request) bool {
	users.mutex.RLock()
	index := getAuthorizedIndex(w, r)
	users.mutex.RUnlock()

	return index != -1
}

func getAuthorized(w http.ResponseWriter, r *http.Request) *User {
	users.mutex.RLock()
	defer users.mutex.RUnlock()

	index := getAuthorizedIndex(w, r)

	if index == -1 {
		return nil
	}

	user := users.slice[index]
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

func getAuthorizedIndex(w http.ResponseWriter, r *http.Request) int {
	token := r.Header.Get("Authorization")
	if token == "" {
		LogError("Invalid token")
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return -1
	}

	for i, user := range users.slice {
		if user.token == token {
			return i
		}
	}

	LogError("Failed to find user")
	http.Error(w, "User not found", http.StatusUnauthorized)
	return -1
}

func readJsonDataFromRequest(w http.ResponseWriter, r *http.Request, data any) bool {
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

func writeEvent(w http.ResponseWriter, eventName string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)

		http.Error(w, "Failed to serialize welcome message", http.StatusInternalServerError)
		return err
	}

	jsonString := string(jsonData)
	eventId := state.eventId.Add(1)

	conns.mutex.Lock()
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\nretry: %d\n\n", eventId, eventName, jsonString, RETRY)
	conns.mutex.Unlock()

	if err != nil {
		return err
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func writeEventToAllConnections(origin http.ResponseWriter, eventName string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)
		if origin != nil {
			http.Error(origin, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	jsonString := string(jsonData)

	eventId := state.eventId.Add(1)
	event := fmt.Sprintf("id: %v\nevent: %v\ndata: %v\nretry: %v\n\n", eventId, eventName, jsonString, RETRY)

	conns.mutex.RLock()
	for _, conn := range conns.slice {
		fmt.Fprintln(conn.writer, event)

		if f, ok := conn.writer.(http.Flusher); ok {
			f.Flush()
		}
	}
	conns.mutex.RUnlock()
}

func writeEventToAllConnectionsExceptSelf(origin http.ResponseWriter, eventName string, data any, userId uint64, connectionId uint64) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to serialize data for event '%v': %v", eventName, err)
		http.Error(origin, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonString := string(jsonData)

	eventId := state.eventId.Add(1)
	event := fmt.Sprintf("id: %v\nevent: %v\ndata: %v\nretry: %v\n\n", eventId, eventName, jsonString, RETRY)

	conns.mutex.RLock()
	for _, conn := range conns.slice {
		if userId == conn.userId && conn.id == connectionId {
			continue
		}

		fmt.Fprintln(conn.writer, event)

		if f, ok := conn.writer.(http.Flusher); ok {
			f.Flush()
		}
	}
	conns.mutex.RUnlock()
}

// It should be possible to use this list in a dropdown and attach to entry
func getSubtitles() []string {
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

func setupGenericFileProxy(url string, referer string) bool {
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
	proxy := &state.proxy
	proxy.isHls = false
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

func setupHlsProxy(url string, referer string) bool {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		LogError("The provided URL is invalid: %v", err)
		return false
	}

	_ = os.RemoveAll(WEB_PROXY)
	_ = os.Mkdir(WEB_PROXY, os.ModePerm)
	var m3u *M3U
	if strings.HasPrefix(url, MEDIA) || strings.HasPrefix(url, serverRootAddress) {
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

	state.proxy.setupLock.Lock()
	state.proxy.isHls = true
	var result bool
	if m3u.isLive {
		result = setupLiveProxy(m3u, url)
		state.proxy.setupLock.Unlock()
	} else {
		result = setupVodProxy(m3u)
		state.proxy.setupLock.Unlock()
	}
	return result
}

func setupLiveProxy(m3u *M3U, liveUrl string) bool {
	proxy := &state.proxy
	proxy.isLive = true
	proxy.liveUrl = liveUrl
	proxy.liveSegments.Clear()
	proxy.randomizer.Store(0)
	return true
}

func setupVodProxy(m3u *M3U) bool {
	segmentCount := len(m3u.segments)
	proxy := &state.proxy
	proxy.isLive = false

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

func apiEvents(w http.ResponseWriter, r *http.Request) {
	LogDebug("URL is %v", r.URL)

	token := r.URL.Query().Get("token")
	if token == "" {
		response := "Failed to parse token from the event url."
		http.Error(w, response, http.StatusInternalServerError)
		LogError("%v", response)
		return
	}

	users.mutex.Lock()
	userIndex := users.findIndex(token)
	if userIndex == -1 {
		http.Error(w, "User not found", http.StatusUnauthorized)
		LogError("Failed to connect to event stream. User not found.")
		users.mutex.Unlock()
		return
	}

	users.slice[userIndex].connections += 1

	was_online_before := users.slice[userIndex].Online
	is_online := users.slice[userIndex].connections != 0

	users.slice[userIndex].Online = is_online

	user := users.slice[userIndex]
	users.mutex.Unlock()

	conns.mutex.Lock()
	connectionId := conns.add(w, user.Id)
	connectionCount := len(conns.slice)
	conns.mutex.Unlock()

	LogInfo("New connection established with user %v on %s. Current connection count: %d", user.Id, r.RemoteAddr, connectionCount)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	welcomeErr := writeEvent(w, "userwelcome", connectionId)
	if welcomeErr != nil {
		return
	}

	if !was_online_before && is_online {
		writeEventToAllConnectionsExceptSelf(w, "userconnected", user.Id, user.Id, connectionId)
	}

	for {
		var event SyncEventData
		state.mutex.RLock()
		playing := state.player.Playing
		state.mutex.RUnlock()

		if playing {
			event = createSyncEvent("play", 0)
		} else {
			event = createSyncEvent("pause", 0)
		}

		connectionErr := writeEvent(w, "sync", event)

		if connectionErr != nil {
			conns.mutex.Lock()
			conns.remove(connectionId)
			connectionCount = len(conns.slice)
			conns.mutex.Unlock()

			users.mutex.Lock()
			userIndex := users.findIndex(token)
			disconnected := false
			if userIndex != -1 {
				users.slice[userIndex].connections -= 1
				disconnected = users.slice[userIndex].connections == 0
				users.slice[userIndex].Online = !disconnected
			}
			users.mutex.Unlock()

			if disconnected {
				writeEventToAllConnectionsExceptSelf(w, "userdisconnected", user.Id, user.Id, connectionId)
			}

			LogInfo("Connection with user %v on %s dropped. Current connection count: %d", user.Id, r.RemoteAddr, connectionCount)
			LogDebug("Drop error message: %v", connectionErr)
			return
		}

		smartSleep()
	}
}

// This endpoints should serve HLS chunks
// If the chunk is out of range or has no id, then 404 should be returned
// 1. Download m3u8 provided by a user
// 2. Serve a modified m3u8 to every user that wants to use a proxy
// 3. In memory use:
//   - 0-indexed string[] for original chunk URLs
//   - 0-indexed mutex[] to ensure the same chunk is not requested while it's being fetched
func watchProxy(writer http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		LogWarn("Proxy not called with GET, received: %v", request.Method)
		return
	}
	urlPath := request.URL.Path
	chunk := path.Base(urlPath)

	if state.proxy.isHls {
		if state.proxy.isLive {
			serveHlsLive(writer, request, chunk)
		} else {
			serveHlsVod(writer, request, chunk)
		}
	} else {
		serveGenericFile(writer, request, chunk)
	}
}

type FetchedSegment struct {
	realUrl  string
	obtained bool
	mutex    sync.Mutex
	created  time.Time
}

func serveHlsLive(writer http.ResponseWriter, request *http.Request, chunk string) {
	proxy := &state.proxy
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

		liveM3U, err := downloadM3U(proxy.liveUrl, WEB_PROXY+ORIGINAL_M3U8, state.entry.RefererUrl)
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
	fetchErr := downloadFile(fetchedChunk.realUrl, WEB_PROXY+chunk, state.entry.RefererUrl)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch live chunk %v", fetchErr)
		http.Error(writer, "Failed to fetch chunk", 500)
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

func serveHlsVod(writer http.ResponseWriter, request *http.Request, chunk string) {
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
	chunk_id, err := strconv.Atoi(chunk[3:])
	if err != nil {
		http.Error(writer, "Not a correct chunk id", 404)
		return
	}

	proxy := &state.proxy
	if chunk_id < 0 || chunk_id >= len(proxy.fetchedChunks) {
		http.Error(writer, "Chunk ID not in range", 404)
		return
	}

	if proxy.fetchedChunks[chunk_id] {
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}

	mutex := &proxy.chunkLocks[chunk_id]
	mutex.Lock()
	if proxy.fetchedChunks[chunk_id] {
		mutex.Unlock()
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}
	fetchErr := downloadFile(proxy.originalChunks[chunk_id], WEB_PROXY+chunk, state.entry.RefererUrl)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch chunk %v", fetchErr)
		http.Error(writer, "Failed to fetch chunk", 500)
		return
	}
	proxy.fetchedChunks[chunk_id] = true
	mutex.Unlock()

	http.ServeFile(writer, request, WEB_PROXY+chunk)
}

const GENERIC_CHUNK_SIZE = 4 * MB

func serveGenericFile(writer http.ResponseWriter, request *http.Request, pathFile string) {
	proxy := &state.proxy
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
		response, err := openFileDownload(proxy.fileUrl, byteRange.start, state.entry.RefererUrl)
		if err != nil {
			http.Error(writer, "Unable to open file download", 500)
			return
		}
		proxy.download = response
		go downloadProxyFilePeriodically()
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

func downloadProxyFilePeriodically() {
	proxy := &state.proxy
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
		insertContentRangeSequentially(newRange(offset, offset+GENERIC_CHUNK_SIZE-1))
		mergeContentRanges()
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
func insertContentRangeSequentially(newRange *Range) {
	proxy := &state.proxy
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

func mergeContentRanges() {
	proxy := &state.proxy
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
func smartSleep() {
	time.Sleep(BROADCAST_INTERVAL)
	for {
		now := time.Now()
		diff := now.Sub(state.lastUpdate)

		if diff > BROADCAST_INTERVAL {
			break
		}
		time.Sleep(BROADCAST_INTERVAL - diff)
	}
}

func updatePlayerState(isPlaying bool, newTimestamp float64) {
	state.mutex.Lock()

	state.player.Playing = isPlaying
	state.player.Timestamp = newTimestamp
	state.lastUpdate = time.Now()

	state.mutex.Unlock()
}

func createSyncEvent(action string, userId uint64) SyncEventData {
	state.mutex.RLock()
	var timestamp = state.player.Timestamp
	if state.player.Playing {
		now := time.Now()
		diff := now.Sub(state.lastUpdate)
		timestamp = state.player.Timestamp + diff.Seconds()
	}
	state.mutex.RUnlock()

	event := SyncEventData{
		Timestamp: timestamp,
		Action:    action,
		UserId:    userId,
	}

	return event
}

const MAX_MESSAGE_CHARACTERS = 1000

func apiChatGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s requested messages.", r.RemoteAddr)

	conns.mutex.RLock()
	jsonData, err := json.Marshal(state.messages)
	conns.mutex.RUnlock()

	if err != nil {
		LogWarn("Failed to serialize messages get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func apiChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	LogInfo("Connection %s posted a chat message.", r.RemoteAddr)

	user := getAuthorized(w, r)
	if user == nil {
		return
	}

	var newMessage ChatMessageFromUser
	if !readJsonDataFromRequest(w, r, &newMessage) {
		return
	}
	if len(newMessage.Message) > MAX_MESSAGE_CHARACTERS {
		http.Error(w, "Message exceeds 1000 chars", http.StatusForbidden)
		return
	}

	state.mutex.Lock()
	state.messageId += 1
	chatMessage := ChatMessage{
		Id:       1,
		Message:  newMessage.Message,
		AuthorId: user.Id,
		UnixTime: time.Now().UnixMilli(),
		Edited:   false,
	}
	state.messages = append(state.messages, chatMessage)
	state.mutex.Unlock()
	writeEventToAllConnections(w, "messagecreate", chatMessage)
}

func isLocalDirectory(url string) (bool, string) {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		return false, ""
	}

	if !strings.HasPrefix(url, MEDIA) && !strings.HasPrefix(url, serverRootAddress) {
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

func getEntriesFromDirectory(path string, userId uint64) []Entry {
	entries := make([]Entry, 0)

	items, _ := os.ReadDir("./web/" + path)
	for _, item := range items {
		if !item.IsDir() {
			webpath := path + "/" + item.Name()
			url := net_url.URL{
				Path: webpath,
			}

			LogDebug("File URL: %v", url.String())

			state.mutex.Lock()
			state.entryId += 1
			id := state.entryId
			state.mutex.Unlock()

			entry := Entry{
				Id:          id,
				Url:         url.String(),
				UserId:      userId,
				Title:       "",
				UseProxy:    false,
				RefererUrl:  "",
				SourceUrl:   "",
				SubtitleUrl: "",
				Created:     time.Now(),
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
