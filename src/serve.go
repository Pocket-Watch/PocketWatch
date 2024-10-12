package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	url2 "net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const ANNOUNCE_RECEIVED = true
const BODY_LIMIT = 1024
const RETRY = 5000 // Retry time in milliseconds

var state = State{}
var connections = makeConnections()

func StartServer(options *Options) {
	state.lastTimeUpdate = time.Now()
	registerEndpoints(options)

	var address = options.Address + ":" + strconv.Itoa(int(options.Port))
	log_info("Starting server on address: %s", address)

	const CERT = "./secret/certificate.pem"
	const PRIV_KEY = "./secret/privatekey.pem"

	_, err_cert := os.Stat(CERT)
	_, err_priv := os.Stat(PRIV_KEY)

	missing_ssl_keys := errors.Is(err_priv, os.ErrNotExist) || errors.Is(err_cert, os.ErrNotExist)

	if options.Ssl && missing_ssl_keys {
		log_error("Failed to find either SSL certificate or the private key.")
	}

	var server_start_error error
	if !options.Ssl || missing_ssl_keys {
		log_warn("Server is running in unencrypted http mode.")
		server_start_error = http.ListenAndServe(address, nil)
	} else {
		server_start_error = http.ListenAndServeTLS(address, CERT, PRIV_KEY, nil)
	}

	if server_start_error != nil {
		log_error("Error starting the server: %v", server_start_error)
	}
}

const PROXY_ROUTE = "/watch/proxy/"
const WEB_PROXY = "web/proxy/"
const WEB_MEDIA = "web/media/"
const ORIGINAL_M3U8 = "original.m3u8"
const PROXY_M3U8 = "proxy.m3u8"

func registerEndpoints(options *Options) {
	fileserver := http.FileServer(http.Dir("./web"))
	// fix trailing suffix
	http.Handle("/", http.StripPrefix("/watch/", fileserver))

	http.HandleFunc("/watch/api/version", apiVersion)
	http.HandleFunc("/watch/api/login", apiLogin)
	http.HandleFunc("/watch/api/get", apiGet)
	http.HandleFunc("/watch/api/seturl", apiSetUrl)
	http.HandleFunc("/watch/api/play", apiStart)
	http.HandleFunc("/watch/api/pause", apiPause)
	http.HandleFunc("/watch/api/seek", apiSeek)
	http.HandleFunc("/watch/api/upload", apiUpload)

	http.HandleFunc("/watch/api/playlist/get", apiPlaylistGet)
	http.HandleFunc("/watch/api/playlist/add", apiPlaylistAdd)
	http.HandleFunc("/watch/api/playlist/clear", apiPlaylistClear)
	http.HandleFunc("/watch/api/playlist/next", apiPlaylistNext)
	http.HandleFunc("/watch/api/playlist/remove", apiPlaylistRemove)
	http.HandleFunc("/watch/api/playlist/autoplay", apiPlaylistAutoplay)
	http.HandleFunc("/watch/api/playlist/looping", apiPlaylistLooping)
	http.HandleFunc("/watch/api/playlist/shuffle", apiPlaylistShuffle)
	http.HandleFunc("/watch/api/playlist/move", apiPlaylistMove)

	http.HandleFunc("/watch/api/history/get", apiHistoryGet)
	http.HandleFunc("/watch/api/history/clear", apiHistoryClear)

	http.HandleFunc("/watch/api/events", apiEvents)
	http.HandleFunc(PROXY_ROUTE, watchProxy)
}

// the upload method needs to keep track of bytes to be able to limit filesize
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

	log_info("User is uploading file: %s, size: %v", header.Filename, header.Size)

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

// This endpoints should serve HLS chunks
// If the chunk is out of range or has no id, then 404 should be returned
// 1. Download m3u8 provided by a user
// 2. Serve a modified m3u8 to every user that wants to use a proxy
// 3. In memory use:
//   - 0-indexed string[] for original chunk URLs
//   - 0-indexed mutex[] to ensure the same chunk is not requested while it's being fetched
func watchProxy(writer http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		log_warn("Proxy not called with GET, received: %v", request.Method)
		return
	}
	urlPath := request.URL.Path
	chunk := path.Base(urlPath)

	if chunk == PROXY_M3U8 {
		log_debug("Serving %v", PROXY_M3U8)
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
	fetchErr := downloadFile(state.originalChunks[chunk_id], WEB_PROXY+chunk)
	if fetchErr != nil {
		mutex.Unlock()
		log_error("FAILED TO FETCH CHUNK %v", fetchErr)
		http.Error(writer, "Failed to fetch chunk", 500)
		return
	}
	state.fetchedChunks[chunk_id] = true
	mutex.Unlock()

	http.ServeFile(writer, request, WEB_PROXY+chunk)
}

func downloadFile(url string, filename string) error {
	// Get the data
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return fmt.Errorf("error downloading file: status code %d", response.StatusCode)
	}
	defer response.Body.Close()

	// Create the file
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, response.Body)
	if err != nil {
		return err
	}
	return nil
}

func apiVersion(w http.ResponseWriter, r *http.Request) {
	log_info("Connection %s requested server version.", r.RemoteAddr)
	io.WriteString(w, VERSION)
}

func apiLogin(w http.ResponseWriter, r *http.Request) {
	log_info("Connection %s attempted to log in.", r.RemoteAddr)
	io.WriteString(w, "This is unimplemented")
}

func apiPlaylistGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	log_info("Connection %s requested playlist get.", r.RemoteAddr)

	state.playlist_lock.RLock()
	jsonData, err := json.Marshal(state.playlist)
	state.playlist_lock.RUnlock()

	if err != nil {
		log_warn("Failed to serialize playlist get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func apiPlaylistAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist add.", r.RemoteAddr)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entry PlaylistEntry
	err = json.Unmarshal(body, &entry)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log_info("Adding '%s' url to the playlist.", entry.Url)

	state.playlist_lock.Lock()
	state.playlist = append(state.playlist, entry)
	state.playlist_lock.Unlock()

	jsonData := string(body)

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistadd", jsonData)
	}
	connections.mutex.RUnlock()
}

func apiPlaylistClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist clear.", r.RemoteAddr)

	state.playlist_lock.Lock()
	state.playlist = state.playlist[:0]
	state.playlist_lock.Unlock()

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistclear", "")
	}
	connections.mutex.RUnlock()
}

func apiPlaylistNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist next.", r.RemoteAddr)

	// NOTE(kihau):
	//     We need to check whether currently set URL on the player side matches current URL on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log_error("Playlist next request handler failed to read current client URL.")
		return
	}

	var current_url string
	err = json.Unmarshal(data, &current_url)
	if err != nil {
		log_error("Playlist next request handler failed to parse current client URL.")
		return
	}

	if state.url != current_url {
		log_warn("Current URL on the server is not equal to the one provided by the client.")
		return
	}

	var url string
	state.playlist_lock.Lock()

	if state.looping.Load() {
		// TODO(kihau): This needs to be changed.
		dummyEntry := PlaylistEntry{
			Uuid:     string(rand.Int() % 999999999999999),
			Username: "<unknown>",
			Url:      current_url,
		}
		state.playlist = append(state.playlist, dummyEntry)
	}

	if len(state.playlist) == 0 {
		url = ""
	} else {
		url = state.playlist[0].Url
		state.playlist = state.playlist[1:]
	}
	state.playlist_lock.Unlock()

	jsonData, err := json.Marshal(url)
	if err != nil {
		log_error("Failed to serialize json data")
		return
	}

	connections.mutex.RLock()
	if state.url != "" {
		state.history = append(state.history, state.url)
	}

	// TODO(kihau): This might cause problems in the future and needs to be cleaned up.
	state.url = url
	state.playing.Swap(state.autoplay.Load())
	state.timestamp = 0

	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistnext", string(jsonData))
	}

	connections.mutex.RUnlock()
}

func apiPlaylistRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist remove.", r.RemoteAddr)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log_error("Playlist remove request handler failed to read remove index.")
		return
	}

	var index int
	err = json.Unmarshal(data, &index)
	if err != nil {
		log_error("Playlist remove request handler failed to parse current remove index.")
		return
	}

	// NOTE(kihau):
	//     This is potentially faulty. Instead of sending just an index, client should also
	//     send a URL or a unique ID than corresponds to a playlist entry.
	//     That way, when multiple clients perform remove request (at the same time) for certain index,
	//     only one playlist entry will be removed.

	state.playlist_lock.Lock()
	if index < 0 || index >= len(state.playlist) {
		log_error("Failed to remove playlist element at index %v.", index)
	} else {
		state.playlist = append(state.playlist[:index], state.playlist[index+1:]...)
	}
	state.playlist_lock.Unlock()

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistremove", string(data))
	}
	connections.mutex.RUnlock()
}

func apiPlaylistAutoplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist autoplay.", r.RemoteAddr)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log_error("Failed to read autoplay payload")
	}

	var autoplay bool
	err = json.Unmarshal(data, &autoplay)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state.autoplay.Store(autoplay)
	log_info("Setting playlist autoplay to %v.", autoplay)

	jsonData := string(data)

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistautoplay", jsonData)
	}
	connections.mutex.RUnlock()
}

func apiPlaylistLooping(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist looping.", r.RemoteAddr)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log_error("Failed to read looping payload")
	}

	var looping bool
	err = json.Unmarshal(data, &looping)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state.looping.Store(looping)
	log_info("Setting playlist looping to %v.", looping)

	jsonData := string(data)

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistlooping", jsonData)
	}
	connections.mutex.RUnlock()
}

func apiPlaylistShuffle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist shuffle.", r.RemoteAddr)

	state.playlist_lock.Lock()
	for i := range state.playlist {
		j := rand.Intn(i + 1)
		state.playlist[i], state.playlist[j] = state.playlist[j], state.playlist[i]
	}

	jsonData, err := json.Marshal(state.playlist)
	state.playlist_lock.Unlock()

	if err != nil {
		log_error("Failed to serialize get event: %v.", err)
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeEvent(conn.writer, "playlistshuffle", string(jsonData))
	}
	connections.mutex.RUnlock()
}

func apiPlaylistMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested playlist move.", r.RemoteAddr)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log_error("Failed to read json body for playlist move event.")
		return
	}

	var move PlaylistMoveEventFromUser
	err = json.Unmarshal(body, &move)
	if err != nil {
		log_error("Failed to deserialize json data for playlist move event.")
		return
	}

	state.playlist_lock.Lock()

	if move.SourceIndex < 0 || move.SourceIndex >= len(state.playlist) {
		log_error("Playlist move failed, source index out of bounds")
		return
	}

	if move.DestIndex < 0 || move.DestIndex >= len(state.playlist) {
		log_error("Playlist move failed, source index out of bounds")
		return
	}

	entry := state.playlist[move.SourceIndex]

	// Remove element from the slice:
	state.playlist = append(state.playlist[:move.SourceIndex], state.playlist[move.SourceIndex+1:]...)

	list := make([]PlaylistEntry, 0)

	// Appned removed element to a new list:
	list = append(list, state.playlist[:move.DestIndex]...)
	list = append(list, entry)
	list = append(list, state.playlist[move.DestIndex:]...)

	state.playlist = list

	jsonData, err := json.Marshal(state.playlist)
	state.playlist_lock.Unlock()

	if err != nil {
		log_error("Failed to serialize move event: %v.", err)
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		// NOTE(kihau):
		//     Sending entire playlist in the playlist move event is pretty wasteful,
		//     but this will do for now.
		writeEvent(conn.writer, "playlistmove", string(jsonData))
	}
	connections.mutex.RUnlock()
}

func apiHistoryGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	log_info("Connection %s requested history get.", r.RemoteAddr)

	connections.mutex.RLock()
	jsonData, err := json.Marshal(state.history)
	connections.mutex.RUnlock()

	if err != nil {
		log_warn("Failed to serialize history get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

func apiHistoryClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested history clear.", r.RemoteAddr)

	connections.mutex.RLock()
	state.history = state.history[:0]

	for _, conn := range connections.slice {
		writeEvent(conn.writer, "historyclear", "")
	}
	connections.mutex.RUnlock()
}

func writeEvent(writer http.ResponseWriter, event string, jsonData string) {
	// fmt.Printf("Writing set event");
	event_id := state.eventId.Add(1)
	fmt.Fprintln(writer, "id:", event_id)
	fmt.Fprintln(writer, "event:", event)
	fmt.Fprintln(writer, "data:", jsonData)
	fmt.Fprintln(writer, "retry:", RETRY)
	fmt.Fprintln(writer)

	// Flush the response to ensure the client receives the event
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}
}

func apiGet(w http.ResponseWriter, r *http.Request) {
	log_info("Connection %s requested get.", r.RemoteAddr)

	var getEvent GetEventForUser
	getEvent.Url = state.url
	getEvent.IsPlaying = state.playing.Load()
	getEvent.Timestamp = state.timestamp
	getEvent.Autoplay = state.autoplay.Load()
	getEvent.Looping = state.looping.Load()
	getEvent.Subtitles = getSubtitles()

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		log_error("Failed to serialize get event.")
		return
	}

	io.WriteString(w, string(jsonData))
}

var SUBTITLE_EXTENSIONS = [...]string{".vtt", ".srt"}

const MAX_SUBTITLE_SIZE = 512 * 1024

func getSubtitles() []string {
	subtitles := make([]string, 0)
	// could create a separate folder for subs if it gets too big
	files, err := os.ReadDir(WEB_MEDIA)
	if err != nil {
		log_error("Failed to read %v and find subtitles.", WEB_MEDIA)
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
	log_info("Served subtitles: %v", subtitles)
	return subtitles
}

func apiSetUrl(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	connections.mutex.RLock()
	if state.url != "" {
		state.history = append(state.history, state.url)
	}
	connections.mutex.RUnlock()

	log_info("Connection %s requested media url change.", r.RemoteAddr)
	if !readSetEventAndUpdateState(w, r) {
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSetEvent(conn.writer)
	}
	connections.mutex.RUnlock()

	io.WriteString(w, "Setting media url!")

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSetEvent(conn.writer)
	}
	connections.mutex.RUnlock()
}

func apiStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	state.playing.Swap(true)

	log_info("Connection %s requested player start.", r.RemoteAddr)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSyncEvent(conn.writer, Play, true, syncEvent.Username)
	}
	connections.mutex.RUnlock()

	io.WriteString(w, "Broadcasting start!\n")
}

func apiPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	state.playing.Store(false)

	log_info("Connection %s requested player pause.", r.RemoteAddr)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSyncEvent(conn.writer, Pause, true, syncEvent.Username)
	}
	connections.mutex.RUnlock()

	io.WriteString(w, "Broadcasting pause!\n")
}

func apiSeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	log_info("Connection %s requested player seek.", r.RemoteAddr)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}
	// this needs a rewrite: /pause /start /seek - a unified format way of
	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSyncEvent(conn.writer, Seek, true, syncEvent.Username)
	}
	connections.mutex.RUnlock()
	io.WriteString(w, "Broadcasting seek!\n")
}

func receiveSyncEventFromUser(w http.ResponseWriter, r *http.Request) *SyncEventFromUser {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// Unmarshal the JSON data
	var sync SyncEventFromUser
	err = json.Unmarshal(body, &sync)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	// Update state
	state.timestamp = sync.Timestamp
	state.lastTimeUpdate = time.Now()
	return &sync
}
func readSetEventAndUpdateState(w http.ResponseWriter, r *http.Request) bool {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}

	// Unmarshal the JSON data
	var setEvent SetEventFromUser
	err = json.Unmarshal(body, &setEvent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	state.timestamp = 0
	state.url = setEvent.Url

	lastSegment := lastUrlSegment(setEvent.Url)
	if setEvent.Proxy && strings.HasSuffix(lastSegment, ".m3u8") {
		setupProxy(setEvent.Url)
	} else {
		state.url = setEvent.Url
	}

	log_info("New url is now: '%s'.", state.url)
	state.playing.Swap(state.autoplay.Load())
	return true
}

func stripLastSegment(url string) (*string, error) {
	pUrl, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}
	lastSlash := strings.LastIndex(pUrl.Path, "/")
	stripped := pUrl.Scheme + "://" + pUrl.Host + pUrl.Path[:lastSlash+1]
	return &stripped, nil
}

func toString(num int) string {
	return strconv.Itoa(num)
}

func setupProxy(url string) {
	_ = os.Mkdir(WEB_PROXY, os.ModePerm)
	m3u, err := downloadM3U(url, WEB_PROXY+ORIGINAL_M3U8)
	if err != nil {
		log_error("Failed to fetch m3u8: %v", err)
		state.url = err.Error()
		return
	}
	log_debug("%v %v", EXT_X_PLAYLIST_TYPE, m3u.ext_x_playlist_type)
	log_debug("%v %v", EXT_X_VERSION, m3u.ext_x_version)
	log_debug("%v %v", EXT_X_TARGETDURATION, m3u.ext_x_target_duration)
	log_debug("tracks: %v", len(m3u.tracks))
	log_debug("total duration: %v", m3u.totalDuration())

	if len(m3u.tracks) == 0 {
		log_warn("No tracks found")
		state.url = "No tracks found"
		return
	}

	// Sometimes m3u8 chunks are not fully qualified
	if !strings.HasPrefix(m3u.tracks[0].url, "http") {
		segment, err := stripLastSegment(url)
		if err != nil {
			log_error(err.Error())
			return
		}
		m3u.prefixTracks(*segment)
	}

	routedM3U := m3u.copy()
	// lock on proxy setup here! also discard the previous proxy state somehow?
	state.chunkLocks = make([]sync.Mutex, 0, len(m3u.tracks))
	state.originalChunks = make([]string, 0, len(m3u.tracks))
	state.fetchedChunks = make([]bool, 0, len(m3u.tracks))
	for i := 0; i < len(routedM3U.tracks); i++ {
		state.chunkLocks = append(state.chunkLocks, sync.Mutex{})
		state.originalChunks = append(state.originalChunks, m3u.tracks[i].url)
		state.fetchedChunks = append(state.fetchedChunks, false)
		routedM3U.tracks[i].url = "ch-" + toString(i)
	}

	routedM3U.serialize(WEB_PROXY + PROXY_M3U8)
	log_info("Prepared proxy file %v", PROXY_M3U8)

	state.url = PROXY_ROUTE + "proxy.m3u8"
}

func apiEvents(w http.ResponseWriter, r *http.Request) {
	connections.mutex.Lock()
	connection_id := connections.add(w)
	connection_count := connections.len()
	connections.mutex.Unlock()

	log_info("New connection established with %s. Current connection count: %d", r.RemoteAddr, connection_count)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		var eventType string
		if state.playing.Load() {
			eventType = Play
		} else {
			eventType = Pause
		}
		connection_error := writeSyncEvent(w, eventType, false, "SERVER")

		if connection_error != nil {
			connections.mutex.Lock()
			connections.remove(connection_id)
			connection_count = connections.len()
			connections.mutex.Unlock()

			log_info("Connection with %s dropped. Current connection count: %d", r.RemoteAddr, connection_count)
			break
		}

		smartSleep()
	}
}

const BROADCAST_INTERVAL = 2 * time.Second

// this will prevent LAZY broadcasts when users make frequent updates
func smartSleep() {
	time.Sleep(BROADCAST_INTERVAL)
	for {
		now := time.Now()
		diff := now.Sub(state.lastTimeUpdate)

		if diff > BROADCAST_INTERVAL {
			break
		}
		time.Sleep(BROADCAST_INTERVAL - diff)
	}
}

const Play = "play"
const Pause = "pause"
const Seek = "seek"

func writeSyncEvent(writer http.ResponseWriter, eventType string, haste bool, user string) error {
	var priority string
	if haste {
		priority = "HASTY"
	} else {
		priority = "LAZY"
	}

	var syncEvent SyncEventForUser
	// this needs to be reviewed
	var timestamp = state.timestamp
	if state.playing.Load() {
		now := time.Now()
		diff := now.Sub(state.lastTimeUpdate)
		timestamp = state.timestamp + diff.Seconds()
	}
	syncEvent = SyncEventForUser{
		Timestamp: timestamp,
		Priority:  priority,
		Origin:    user,
	}
	jsonData, err := json.Marshal(syncEvent)
	if err != nil {
		log_error("Failed to serialize sync event")
		return nil
	}
	eventData := string(jsonData)

	event_id := state.eventId.Add(1)
	_, err = fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\nretry: %d\n\n", event_id, eventType, eventData, RETRY)
	if err != nil {
		return err
	}

	// Flush the response to ensure the client receives the event
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func writeSetEvent(writer http.ResponseWriter) {
	// fmt.Printf("Writing set event");
	event_id := state.eventId.Add(1)
	fmt.Fprintln(writer, "id:", event_id)
	fmt.Fprintln(writer, "event: seturl")
	fmt.Fprintln(writer, "data:", "{\"url\":\""+state.url+"\"}")
	fmt.Fprintln(writer, "retry:", RETRY)
	fmt.Fprintln(writer)

	// Flush the response to ensure the client receives the event
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}
}

func lastUrlSegment(url string) string {
	url = path.Base(url)
	questionMark := strings.Index(url, "?")
	if questionMark == -1 {
		return url
	}
	return url[:questionMark]
}

// NOTE(kihau): Some fields are non atomic. This needs to change.
type State struct {
	autoplay       atomic.Bool
	looping        atomic.Bool
	playing        atomic.Bool
	timestamp      float64
	url            string
	eventId        atomic.Uint64
	lastTimeUpdate time.Time
	history        []string

	playlist_lock sync.RWMutex
	playlist      []PlaylistEntry

	proxying       atomic.Bool
	chunkLocks     []sync.Mutex
	fetchedChunks  []bool
	originalChunks []string
}

type Connection struct {
	id     uint64
	writer http.ResponseWriter
}

type Connections struct {
	mutex      sync.RWMutex
	id_counter uint64
	slice      []Connection
}

func makeConnections() *Connections {
	conns := new(Connections)
	conns.slice = make([]Connection, 0)
	conns.id_counter = 0
	return conns
}

func (conns *Connections) add(writer http.ResponseWriter) uint64 {
	id := conns.id_counter
	conns.id_counter += 1

	conn := Connection{id, writer}
	conns.slice = append(conns.slice, conn)

	return id
}

func (conns *Connections) remove(id uint64) {
	for i, conn := range conns.slice {
		if conn.id != id {
			continue
		}

		length := conns.len()
		conns.slice[i], conns.slice[length-1] = conns.slice[length-1], conns.slice[i]
		conns.slice = conns.slice[:length-1]
		break
	}
}

func (conns *Connections) len() int {
	return len(conns.slice)
}

type PlaylistEntry struct {
	Uuid     string `json:"uuid"`
	Username string `json:"username"`
	Url      string `json:"url"`
}

type GetEventForUser struct {
	Url       string   `json:"url"`
	Timestamp float64  `json:"timestamp"`
	IsPlaying bool     `json:"is_playing"`
	Autoplay  bool     `json:"autoplay"`
	Looping   bool     `json:"looping"`
	Subtitles []string `json:"subtitles"`
}

type SyncEventForUser struct {
	Timestamp float64 `json:"timestamp"`
	Priority  string  `json:"priority"`
	Origin    string  `json:"origin"`
}

type SyncEventFromUser struct {
	Timestamp float64 `json:"timestamp"`
	UUID      string  `json:"uuid"`
	Username  string  `json:"username"`
}

type SetEventFromUser struct {
	UUID  string `json:"uuid"`
	Url   string `json:"url"`
	Proxy bool   `json:"proxy"`
}

type PlaylistMoveEventFromUser struct {
	SourceIndex int `json:"source_index"`
	DestIndex   int `json:"dest_index"`
}
