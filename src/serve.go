package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var ANNOUNCE_RECEIVED = true
var BODY_LIMIT = 1024

var html = "The main page hasn't loaded yet!"
var script = "Script hasn't loaded yet!"
var media = http.FileServer(http.Dir("media"))

var state = State{}

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
	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/", fs)

	http.HandleFunc("/version", versionGet)
	http.HandleFunc("/login", login)

	http.HandleFunc("/watch/get", watchGet)
	http.HandleFunc("/watch/set", watchSet)
	http.HandleFunc("/watch/pause", watchPause)
	http.HandleFunc("/watch/seek", watchSeek)
	http.HandleFunc("/watch/start", watchStart)
	http.HandleFunc("/watch/events", watchEvents)
}

func versionGet(w http.ResponseWriter, r *http.Request) {
	print("version was requested.")
	io.WriteString(w, VERSION)
}

func login(w http.ResponseWriter, r *http.Request) {
	print("login was attempted.")
	io.WriteString(w, "This is unimplemented")
}

func watchGet(w http.ResponseWriter, r *http.Request) {
	print("watchGet was called")
	msg := fmt.Sprintf("Playing state: %t", state.playing.Load())
	io.WriteString(w, msg)
}

func watchSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	print("watchSet was called")
	if !readSetEventAndUpdateState(w, r) {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeSetEvent(eWriter)
	}
	io.WriteString(w, "Setting url!")
}

func watchStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	state.playing.Swap(true)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeSyncEvent(eWriter, true, true, syncEvent.Username)
	}
	print("watchStart was called")
	io.WriteString(w, "Broadcasting start!\n")
}

func watchPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	state.playing.Swap(false)
	syncEvent := receiveSyncEventFromUser(w, r)
	if syncEvent == nil {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeSyncEvent(eWriter, false, true, syncEvent.Username)
	}
	print("watchPause was called")
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
	print("watchSeek was called")
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
	print(state.url)
	state.playing.Swap(false)
	return true
}

var eventWriters = CreateEventWriters()

var RETRY = 5000 // Retry time in milliseconds

func watchEvents(w http.ResponseWriter, r *http.Request) {
	eventWriters.Add(w)
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		writeSyncEvent(w, state.playing.Load(), false, "SERVER")

		time.Sleep(2 * time.Second)
	}
}

func writeSyncEvent(writer http.ResponseWriter, playing bool, haste bool, user string) {
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
	}
	eventData := string(jsonData)

	fmt.Fprintln(writer, "id:", state.eventId)
	fmt.Fprintln(writer, "event:", eventType)
	fmt.Fprintln(writer, "data:", eventData)
	fmt.Fprintln(writer, "retry:", RETRY)
	fmt.Fprintln(writer)

	// Flush the response to ensure the client receives the event
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}

	// Increment event ID and wait before sending the next event
	state.eventId++
}

func writeSetEvent(writer http.ResponseWriter) {

	fmt.Fprintln(writer, "id:", state.eventId)
	fmt.Fprintln(writer, "event: set")
	fmt.Fprintln(writer, "data:", "{\"url\":\""+state.url+"\"}")
	fmt.Fprintln(writer, "retry:", RETRY)
	fmt.Fprintln(writer)

	// Flush the response to ensure the client receives the event
	if f, ok := writer.(http.Flusher); ok {
		f.Flush()
	}

	// Increment event ID and wait before sending the next event
	state.eventId++
}

func print(endpoint string) {
	if !ANNOUNCE_RECEIVED {
		return
	}
	fmt.Printf("%s\n", endpoint)
}

type State struct {
	playing        atomic.Bool
	timestamp      float64
	url            string
	eventId        uint64
	lastTimeUpdate time.Time
}

// this slice needs to be sync
type EventWriters struct {
	slice []http.ResponseWriter
}

func (writer *EventWriters) Add(element http.ResponseWriter) {
	writer.slice = append(writer.slice, element)
}
func (writer *EventWriters) RemoveIndex(index int) {
	arr := writer.slice
	length := len(arr)
	// swap index with last index
	arr[index], arr[length-1] = arr[length-1], arr[length]
	// remove last index (whole operation is O(1) regardless of the number of connections)
	arr = arr[:length-1]
}

func CreateEventWriters() EventWriters {
	writers := EventWriters{}
	writers.slice = make([]http.ResponseWriter, 0)
	return writers
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
