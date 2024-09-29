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
	if !readEventAndUpdateState(w, r) {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeSyncEvent(eWriter, true, true)
	}
	print("watchStart was called")
	io.WriteString(w, "Broadcasting start!\n")
}

func watchPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	state.playing.Swap(false)
	if !readEventAndUpdateState(w, r) {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeSyncEvent(eWriter, false, true)
	}
	print("watchPause was called")
	io.WriteString(w, "Broadcasting pause!\n")
}

func watchSeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	if !readEventAndUpdateState(w, r) {
		return
	}
	// this needs a rewrite: /pause /start /seek - a unified format way of
	print("watchSeek was called")
	io.WriteString(w, "SEEK CALLED!\n")
}

func readEventAndUpdateState(w http.ResponseWriter, r *http.Request) bool {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	defer r.Body.Close()

	// Unmarshal the JSON data
	var sync SyncEventFromUser
	err = json.Unmarshal(body, &sync)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	state.timestamp = sync.Timestamp
	state.lastTimeUpdate = time.Now()
	return true
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
		writeSyncEvent(w, state.playing.Load(), false)

		time.Sleep(2 * time.Second)
	}
}

func writeSyncEvent(writer http.ResponseWriter, playing bool, haste bool) {
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
		}
	} else {
		syncEvent = SyncEventForUser{
			Timestamp: state.timestamp,
			Priority:  priority,
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

type EventWriters struct {
	slice []http.ResponseWriter
}

func (writer *EventWriters) Add(element http.ResponseWriter) {
	writer.slice = append(writer.slice, element)
}

func CreateEventWriters() EventWriters {
	writers := EventWriters{}
	writers.slice = make([]http.ResponseWriter, 0)
	return writers
}

type SyncEventForUser struct {
	Timestamp float64 `json:"timestamp"`
	Priority  string  `json:"priority"`
}

type SyncEventFromUser struct {
	Timestamp float64 `json:"timestamp"`
	UUID      string  `json:"uuid"`
}

type SetEventFromUser struct {
	UUID string `json:"uuid"`
	Url  string `json:"url"`
}
