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
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const BODY_LIMIT = 1024
const RETRY = 5000 // Retry time in milliseconds
const TOKEN_LENGTH = 32
const BROADCAST_INTERVAL = 2 * time.Second
const MAX_SUBTITLE_SIZE = 512 * 1024

var SUBTITLE_EXTENSIONS = [...]string{".vtt", ".srt"}

const Play = "play"
const Pause = "pause"
const Seek = "seek"

const PROXY_ROUTE = "/watch/proxy/"
const WEB_PROXY = "web/proxy/"
const WEB_MEDIA = "web/media/"
const ORIGINAL_M3U8 = "original.m3u8"
const PROXY_M3U8 = "proxy.m3u8"

type PlayerState struct {
	Playing   bool    `json:"playing"`
	Autoplay  bool    `json:"autoplay"`
	Looping   bool    `json:"looping"`
	Timestamp float64 `json:"timestamp"`
}

type Entry struct {
	Id         uint64    `json:"id"`
	Url        string    `json:"url"`
	Title      string    `json:"title"`
	UserId     uint64    `json:"user_id"`
	UseProxy   bool      `json:"use_proxy"`
	RefererUrl string    `json:"referer_url"`
	SourceUrl  string    `json:"source_url"`
	Created    time.Time `json:"created"`
}

type ServerState struct {
	mutex sync.RWMutex

	player  PlayerState
	entry   Entry
	entryId uint64

	eventId    atomic.Uint64
	lastUpdate time.Time

	playlist []Entry
	history  []Entry

	chunkLocks     []sync.Mutex
	fetchedChunks  []bool
	originalChunks []string
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
	Id            uint64 `json:"id"`
	Username      string `json:"username"`
	Avatar        string `json:"avatar"`
	Connections   uint64 `json:"connections"`
	token         string
	created       time.Time
	lastUpdate    time.Time
	connIdCounter uint64
	connections   []Connection
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
		Id:            id,
		Username:      fmt.Sprintf("User %v", id),
		Avatar:        "",
		token:         generateToken(),
		created:       time.Now(),
		lastUpdate:    time.Now(),
		connIdCounter: 1,
		connections:   make([]Connection, 0),
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
	Player    PlayerState `json:"player"`
	Entry     Entry       `json:"entry"`
	Subtitles []string    `json:"subtitles"`
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
	ConnectionId uint64 `json:"connection_id"`
	Entry        Entry  `json:"entry"`
}

type PlayerSetEventData struct {
	PrevEntry Entry `json:"prev_entry"`
	NewEntry  Entry `json:"new_entry"`
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

type PlaylistAddRequestData struct {
	ConnectionId uint64 `json:"connection_id"`
	Entry        Entry  `json:"entry"`
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

var state = ServerState{}
var users = makeUsers()
var conns = makeConnections()

func StartServer(options *Options) {
	state.lastUpdate = time.Now()
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
		LogWarn("Server is running in unencrypted http mode.")
		server_start_error = http.ListenAndServe(address, nil)
	} else {
		server_start_error = http.ListenAndServeTLS(address, CERT, PRIV_KEY, nil)
	}

	if server_start_error != nil {
		LogError("Error starting the server: %v", server_start_error)
	}
}

func handleUnknownEndpoint(w http.ResponseWriter, r *http.Request) {
	LogWarn("User %v requested unknown endpoint: %v", r.RemoteAddr, r.RequestURI)
}

func registerEndpoints(options *Options) {
	_ = options

	fileserver := http.FileServer(http.Dir("./web"))
	http.Handle("/watch/", http.StripPrefix("/watch/", fileserver))

	http.HandleFunc("/", handleUnknownEndpoint)

	// Unrelated API calls.
	http.HandleFunc("/watch/api/version", apiVersion)
	http.HandleFunc("/watch/api/login", apiLogin)
	http.HandleFunc("/watch/api/upload", apiUpload)

	// User related API calls.
	http.HandleFunc("/watch/api/user/create", apiUserCreate)
	http.HandleFunc("/watch/api/user/getall", apiUserGetAll)
	http.HandleFunc("/watch/api/user/verify", apiUserVerify)
	http.HandleFunc("/watch/api/user/updatename", apiUserUpdateName)

	// API calls that change state of the player.
	http.HandleFunc("/watch/api/player/get", apiPlayerGet)
	http.HandleFunc("/watch/api/player/set", apiPlayerSet)
	http.HandleFunc("/watch/api/player/next", apiPlayerNext)
	http.HandleFunc("/watch/api/player/play", apiPlayerPlay)
	http.HandleFunc("/watch/api/player/pause", apiPlayerPause)
	http.HandleFunc("/watch/api/player/seek", apiPlayerSeek)
	http.HandleFunc("/watch/api/player/autoplay", apiPlayerAutoplay)
	http.HandleFunc("/watch/api/player/looping", apiPlayerLooping)

	// API calls that change state of the playlist.
	http.HandleFunc("/watch/api/playlist/get", apiPlaylistGet)
	http.HandleFunc("/watch/api/playlist/add", apiPlaylistAdd)
	http.HandleFunc("/watch/api/playlist/clear", apiPlaylistClear)
	http.HandleFunc("/watch/api/playlist/remove", apiPlaylistRemove)
	http.HandleFunc("/watch/api/playlist/shuffle", apiPlaylistShuffle)
	http.HandleFunc("/watch/api/playlist/move", apiPlaylistMove)

	// API calls that change state of the history.
	http.HandleFunc("/watch/api/history/get", apiHistoryGet)
	http.HandleFunc("/watch/api/history/clear", apiHistoryClear)

	// Server events and proxy.
	http.HandleFunc("/watch/api/events", apiEvents)
	http.HandleFunc(PROXY_ROUTE, watchProxy)
}

func apiVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s requested server version.", r.RemoteAddr)
	io.WriteString(w, VERSION)
}

func apiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	LogInfo("Connection %s attempted to log in.", r.RemoteAddr)
	io.WriteString(w, "This is unimplemented")
}

func apiUpload(writer http.ResponseWriter, request *http.Request) {
	if request.Method != "POST" {
		http.Error(writer, "POST was expected", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := request.FormFile("file")
	// It's weird because a temporary file is created in Temp/multipart-
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	LogInfo("User is uploading file: %s, size: %v", header.Filename, header.Size)

	out, err := os.Create(WEB_MEDIA + header.Filename)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(writer, "File uploaded successfully: %s", header.Filename)
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

	jsonData, err := json.Marshal(user)
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
	writeEventToAllConnections(w, "usernameupdate", user)
}

func apiPlayerGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested get.", r.RemoteAddr)

	state.mutex.RLock()
	getEvent := PlayerGetResponseData{
		Player:    state.player,
		Entry:     state.entry,
		Subtitles: getSubtitles(),
	}
	state.mutex.RUnlock()

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		LogError("Failed to serialize get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func apiPlayerSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested media url change.", r.RemoteAddr)

	var data PlayerSetRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	loadYoutubeEntry(&data.Entry)

	state.mutex.Lock()
	if state.entry.Url != "" {
		state.history = append(state.history, state.entry)
	}

	state.entryId += 1

	state.player.Timestamp = 0
	state.player.Playing = state.player.Autoplay

	prevEntry := state.entry

	state.entry = data.Entry
	state.entry.Created = time.Now()
	state.entry.Id = state.entryId
	state.entry.Title = constructTitleWhenMissing(&state.entry)

	lastSegment := lastUrlSegment(state.entry.Url)
	if state.entry.UseProxy && strings.HasSuffix(lastSegment, ".m3u8") {
		setupProxy(state.entry.Url, state.entry.RefererUrl)
	}
	state.mutex.Unlock()

	LogInfo("New url is now: '%s'.", state.entry.Url)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  state.entry,
	}
	writeEventToAllConnections(w, "playerset", setEvent)
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
	prevEntry := state.entry

	if prevEntry.Url != "" {
		state.history = append(state.history, prevEntry)
	}

	if state.player.Looping && prevEntry.Url != "" {
		state.playlist = append(state.playlist, prevEntry)
	}

	newEntry := Entry{}

	if len(state.playlist) != 0 {
		newEntry = state.playlist[0]
		state.playlist = state.playlist[1:]
	}

	lastSegment := lastUrlSegment(newEntry.Url)
	if newEntry.UseProxy && strings.HasSuffix(lastSegment, ".m3u8") {
		setupProxy(newEntry.Url, newEntry.RefererUrl)
	}
	state.mutex.Unlock()

	loadYoutubeEntry(&newEntry)

	state.mutex.Lock()
	state.player.Playing = state.player.Autoplay
	state.player.Timestamp = 0
	state.entry = newEntry
	state.mutex.Unlock()

	nextEvent := PlayerNextEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	writeEventToAllConnections(w, "playernext", nextEvent)
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

	// TODO(kihau): Add other looping modes, just like the discord bot had: none, single, playlist, shuffle

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

func apiPlaylistAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
		return
	}

	LogInfo("Connection %s requested playlist add.", r.RemoteAddr)

	var data PlaylistAddRequestData
	if !readJsonDataFromRequest(w, r, &data) {
		return
	}

	LogInfo("Adding '%s' url to the playlist.", data.Entry.Url)

	state.mutex.Lock()
	state.entryId += 1

	entry := data.Entry
	entry.Id = state.entryId
	entry.Created = time.Now()
	entry.Title = constructTitleWhenMissing(&entry)

	state.playlist = append(state.playlist, entry)
	state.mutex.Unlock()

	loadYoutubeEntry(&entry)

	state.mutex.Lock()
	state.playlist = append(state.playlist, entry)
	state.mutex.Unlock()

	event := createPlaylistEvent("add", entry)
	writeEventToAllConnections(w, "playlist", event)
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
}

func apiPlaylistMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	if !isAuthorized(w, r) {
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
	writeEventToAllConnections(w, "playlist", event)
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

func getAuthorizedIndex(w http.ResponseWriter, r *http.Request) int {
	token := r.Header.Get("Authorization")
	if token == "" {
		LogError("Inavlid token")
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

func getSubtitles() []string {
	subtitles := make([]string, 0)
	// could create a separate folder for subs if it gets too big
	files, err := os.ReadDir(WEB_MEDIA)
	if err != nil {
		LogError("Failed to read %v and find subtitles.", WEB_MEDIA)
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
				continue
			}
			if strings.HasSuffix(filename, ext) && info.Size() < MAX_SUBTITLE_SIZE {
				subtitles = append(subtitles, "media/"+filename)
			}
		}
	}
	LogInfo("Served subtitles: %v", subtitles)
	return subtitles
}

func setupProxy(url string, referer string) {
	_ = os.Mkdir(WEB_PROXY, os.ModePerm)
	m3u, err := downloadM3U(url, WEB_PROXY+ORIGINAL_M3U8, referer)
	if err != nil {
		LogError("Failed to fetch m3u8: %v", err)
		// state.entry.Url = err.Error()
		return
	}

	if m3u.isMasterPlaylist {
		// Rarely tracks are not fully qualified
		if !strings.HasPrefix(m3u.tracks[0].url, "http") {
			prefix, err := stripLastSegment(url)
			if err != nil {
				LogError(err.Error())
				return
			}
			m3u.prefixTracks(*prefix)
		}
		LogInfo("User provided a master playlist. The best track will be chosen based on quality.")
		track := m3u.getBestTrack()
		if track != nil {
			// a malicious user could cause an infinite setup loop if they provided a carefully crafted m3u8
			setupProxy(track.url, referer)
		}
		return
	}

	// state.entry.Url = url
	// state.entry.RefererUrl = referer

	LogDebug("%v %v", EXT_X_PLAYLIST_TYPE, m3u.playlistType)
	LogDebug("%v %v", EXT_X_VERSION, m3u.version)
	LogDebug("%v %v", EXT_X_TARGETDURATION, m3u.targetDuration)
	LogDebug("segments: %v", len(m3u.segments))
	LogDebug("total duration: %v", m3u.totalDuration())

	if len(m3u.segments) == 0 {
		LogWarn("No segments found")
		// state.entry.Url = "No segments found"
		return
	}

	// Sometimes m3u8 chunks are not fully qualified
	if !strings.HasPrefix(m3u.segments[0].url, "http") {
		prefix, err := stripLastSegment(url)
		if err != nil {
			LogError(err.Error())
			return
		}
		m3u.prefixSegments(*prefix)
	}

	routedM3U := m3u.copy()
	// lock on proxy setup here! also discard the previous proxy state somehow?
	state.chunkLocks = make([]sync.Mutex, 0, len(m3u.segments))
	state.originalChunks = make([]string, 0, len(m3u.segments))
	state.fetchedChunks = make([]bool, 0, len(m3u.segments))
	for i := 0; i < len(routedM3U.segments); i++ {
		state.chunkLocks = append(state.chunkLocks, sync.Mutex{})
		state.originalChunks = append(state.originalChunks, m3u.segments[i].url)
		state.fetchedChunks = append(state.fetchedChunks, false)
		routedM3U.segments[i].url = "ch-" + toString(i)
	}

	routedM3U.serialize(WEB_PROXY + PROXY_M3U8)
	LogInfo("Prepared proxy file %v", PROXY_M3U8)

	// state.entry.Url = PROXY_ROUTE + "proxy.m3u8"
}

func apiEvents(w http.ResponseWriter, r *http.Request) {
	LogDebug("URL is %v", r.URL)

	token := r.URL.Query().Get("token")
	if token == "" {
		response := "Failed to parse token from the event url."
		http.Error(w, response, http.StatusInternalServerError)
		LogError(response)
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

	users.slice[userIndex].Connections += 1
	user := users.slice[userIndex]
	users.mutex.Unlock()

	conns.mutex.Lock()
	connectionId := conns.add(w, user.Id)
	connectionCount := len(conns.slice)
	conns.mutex.Unlock()

	LogInfo("New connection established with %s. Current connection count: %d", r.RemoteAddr, connectionCount)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	welcomeErr := writeEvent(w, "welcome", connectionId)
	if welcomeErr != nil {
		return
	}

	writeEventToAllConnectionsExceptSelf(w, "connectionadd", user.Id, user.Id, connectionId)

	for {
		var event SyncEventData
		if state.player.Playing {
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

			writeEventToAllConnectionsExceptSelf(w, "connectiondrop", user.Id, user.Id, connectionId)

			users.mutex.Lock()
			userIndex := users.findIndex(token)
			if userIndex != -1 {
				users.slice[userIndex].Connections -= 1
			}
			users.mutex.Unlock()

			LogInfo("Connection with %s dropped. Current connection count: %d", r.RemoteAddr, connectionCount)
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

	if chunk_id < 0 || chunk_id >= len(state.fetchedChunks) {
		http.Error(writer, "Chunk ID not in range", 404)
		return
	}

	if state.fetchedChunks[chunk_id] {
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}

	mutex := &state.chunkLocks[chunk_id]
	mutex.Lock()
	if state.fetchedChunks[chunk_id] {
		mutex.Unlock()
		http.ServeFile(writer, request, WEB_PROXY+chunk)
		return
	}
	fetchErr := downloadFile(state.originalChunks[chunk_id], WEB_PROXY+chunk, state.entry.RefererUrl)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("FAILED TO FETCH CHUNK %v", fetchErr)
		http.Error(writer, "Failed to fetch chunk", 500)
		return
	}
	state.fetchedChunks[chunk_id] = true
	mutex.Unlock()

	http.ServeFile(writer, request, WEB_PROXY+chunk)
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
