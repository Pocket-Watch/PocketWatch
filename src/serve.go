package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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
	registerEndpoints(options)

	var address = options.Address + ":" + strconv.Itoa(int(options.Port))
	fmt.Println("HOSTING SERVER ON", address)

	const CERT = "./secret/certificate.pem"
	const PRIV_KEY = "./secret/privatekey.pem"

	_, err_cert := os.Stat(CERT)
	_, err_priv := os.Stat(PRIV_KEY)

	missing_ssl_keys := errors.Is(err_priv, os.ErrNotExist) || errors.Is(err_cert, os.ErrNotExist)

	if options.Ssl && missing_ssl_keys {
		fmt.Println("ERROR: Failed to find either SSL certificate or the private key.")
	}

	var server_start_error error
	if !options.Ssl || missing_ssl_keys {
		fmt.Println("WARNING: Server is running in unencrypted http mode.")
		server_start_error = http.ListenAndServe(address, nil)
	} else {
		server_start_error = http.ListenAndServeTLS(address, CERT, PRIV_KEY, nil)
	}

	if server_start_error != nil {
		fmt.Printf("Error starting server: %v\n", server_start_error)
	}
}

func registerEndpoints(options *Options) {
	fileserver := http.FileServer(http.Dir("./web"))
	http.Handle("/", http.StripPrefix("/watch/", fileserver))

	http.HandleFunc("/watch/api/version", versionGet)
	http.HandleFunc("/watch/api/login", login)
	http.HandleFunc("/watch/api/get", watchGet)
	http.HandleFunc("/watch/api/set/hls", watchSetHls)
	http.HandleFunc("/watch/api/set/mp4", watchSetMp4)
	http.HandleFunc("/watch/api/pause", watchPause)
	http.HandleFunc("/watch/api/seek", watchSeek)
	http.HandleFunc("/watch/api/start", watchStart)
	http.HandleFunc("/watch/api/events", watchEvents)
}

func versionGet(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("INFO: Connection %s requested server version.\n", r.RemoteAddr)
	io.WriteString(w, VERSION)
}

func login(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("INFO: Connection %s attempted to log in.\n", r.RemoteAddr)
	io.WriteString(w, "This is unimplemented")
}

func watchGet(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("INFO: Connection %s requested get.\n", r.RemoteAddr)

	var getEvent GetEventForUser
	getEvent.Url = state.url
	getEvent.IsHls = state.is_hsl.Load()
	getEvent.IsPlaying = state.playing.Load()
	getEvent.Timestamp = state.timestamp

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		fmt.Println("Failed to serialize sync event")
		return
	}

	io.WriteString(w, string(jsonData))
}

func watchSetHls(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	fmt.Printf("INFO: Connection %s requested hls url change.\n", r.RemoteAddr)
	state.is_hsl.Store(true)
	if !readSetEventAndUpdateState(w, r) {
		return
	}

	io.WriteString(w, "Setting hls url!")

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSetEvent(conn.writer, "hls")
	}
	connections.mutex.RUnlock()
}

func watchSetMp4(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	fmt.Printf("INFO: Connection %s requested mp4 url change.\n", r.RemoteAddr)
	state.is_hsl.Store(false)
	if !readSetEventAndUpdateState(w, r) {
		return
	}

	io.WriteString(w, "Setting mp4 url!")

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSetEvent(conn.writer, "mp4")
	}
	connections.mutex.RUnlock()
}

func watchStart(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("INFO: Connection %s requested player start.\n", r.RemoteAddr)

	if r.Method != "POST" {
		return
	}
	state.playing.Swap(true)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSyncEvent(conn.writer, true, true, syncEvent.Username)
	}
	connections.mutex.RUnlock()

	io.WriteString(w, "Broadcasting start!\n")
}

func watchPause(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("INFO: Connection %s requested player pause.\n", r.RemoteAddr)

	if r.Method != "POST" {
		return
	}
	state.playing.Swap(false)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}

	connections.mutex.RLock()
	for _, conn := range connections.slice {
		writeSyncEvent(conn.writer, false, true, syncEvent.Username)
	}
	connections.mutex.RUnlock()

	io.WriteString(w, "Broadcasting pause!\n")
}

func watchSeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}
	// this needs a rewrite: /pause /start /seek - a unified format way of
	fmt.Printf("INFO: Connection %s requested player seek.\n", r.RemoteAddr)
	io.WriteString(w, "SEEK CALLED!\n")
}

func receiveSyncEventFromUser(w http.ResponseWriter, r *http.Request) *SyncEventFromUser {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	defer r.Body.Close()

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
	defer r.Body.Close()

	// Unmarshal the JSON data
	var setEvent SetEventFromUser
	err = json.Unmarshal(body, &setEvent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	state.timestamp = 0
	state.url = setEvent.Url
	fmt.Printf("INFO: New url is now: \"%s\".\n", state.url)
	state.playing.Swap(false)
	return true
}

func watchEvents(w http.ResponseWriter, r *http.Request) {
	connections.mutex.Lock()
	connection_id := connections.add(w)
	connection_count := connections.len()
	connections.mutex.Unlock()

	fmt.Printf("INFO: New connection established with %s. Current connection count: %d\n", r.RemoteAddr, connection_count)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		connection_error := writeSyncEvent(w, state.playing.Load(), false, "SERVER")

		if connection_error != nil {
			connections.mutex.Lock()
			connections.remove(connection_id)
			connection_count = connections.len()
			connections.mutex.Unlock()

			fmt.Printf("INFO: Connection with %s dropped. Current connection count: %d\n", r.RemoteAddr, connection_count)
			break
		}

		time.Sleep(2 * time.Second)
	}
}

func writeSyncEvent(writer http.ResponseWriter, playing bool, haste bool, user string) error {
	var eventType string
	if playing {
		eventType = "start"
	} else {
		eventType = "pause"
	}

	var priority string
	if haste {
		priority = "HASTY"
	} else {
		priority = "LAZY"
	}

	var syncEvent SyncEventForUser
	// this needs to be reviewed
	if state.playing.Load() {
		now := time.Now()
		diff := now.Sub(state.lastTimeUpdate)
		syncEvent = SyncEventForUser{
			Timestamp: state.timestamp + diff.Seconds(),
			Priority:  priority,
			Origin:    user,
		}
	} else {
		syncEvent = SyncEventForUser{
			Timestamp: state.timestamp,
			Priority:  priority,
			Origin:    user,
		}
	}
	jsonData, err := json.Marshal(syncEvent)
	if err != nil {
		fmt.Println("Failed to serialize sync event")
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

func writeSetEvent(writer http.ResponseWriter, set_endpoint string) {
    // fmt.Printf("Writing set event");
	event_id := state.eventId.Add(1)
	fmt.Fprintln(writer, "id:", event_id)
	fmt.Fprintln(writer, "event: set/"+set_endpoint)
	fmt.Fprintln(writer, "data:", "{\"url\":\""+state.url+"\"}")
	fmt.Fprintln(writer, "retry:", RETRY)
	fmt.Fprintln(writer)

	// Flush the response to ensure the client receives the event
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}
}

func print(endpoint string) {
	if !ANNOUNCE_RECEIVED {
		return
	}
	fmt.Printf("%s\n", endpoint)
}

// NOTE(kihau): Some fields are non atomic. This needs to change.
type State struct {
	playing        atomic.Bool
	timestamp      float64
	url            string
	is_hsl         atomic.Bool
	eventId        atomic.Uint64
	lastTimeUpdate time.Time
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

type GetEventForUser struct {
	Url       string  `json:"url"`
	IsHls     bool    `json:"is_hls"`
	Timestamp float64 `json:"timestamp"`
	IsPlaying bool    `json:"is_playing"`
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
	UUID string `json:"uuid"`
	Url  string `json:"url"`
}
