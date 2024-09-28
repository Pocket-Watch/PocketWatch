package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var ANNOUNCE_RECEIVED = true

const INDEX_HTMl = "web/index.html"
const SCRIPT_JS = "web/script.js"
const FLUID_PLAYER_JS = "web/fluid_player.js"
const FAVICON = "web/favicon.ico"
const CONSTRUCTION = "web/under_construction.png"

var html = "The main page hasn't loaded yet!"
var script = "Script hasn't loaded yet!"
var media = http.FileServer(http.Dir("media"))

var state = State{}

func StartServer(options *Options) {
	registerEndpoints(options)

	var address = options.Address + ":" + strconv.Itoa(int(options.Port))
	fmt.Println("HOSTING SERVER ON", address)
	loadResources()
	if err := http.ListenAndServe(address, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func loadResources() {
	htmlBytes, err := os.ReadFile(INDEX_HTMl)
	if err != nil {
		fmt.Println("CRITICAL: failed to load index.html")
		return
	}
	html = string(htmlBytes)

	scriptBytes, err := os.ReadFile(SCRIPT_JS)
	if err != nil {
		fmt.Println("CRITICAL: failed to load", SCRIPT_JS)
		return
	}
	script = string(scriptBytes)
}

func registerEndpoints(options *Options) {
	http.HandleFunc("/", getRoot)
	http.HandleFunc("/index.html", getRoot)

	http.HandleFunc("/script.js", serveScript)
	http.HandleFunc("/fluid_player.js", servePlayer)
	http.HandleFunc("/favicon.ico", serveFavicon)
	http.HandleFunc("/under_construction.png", serveConstruction)

	http.HandleFunc("/watch/get", watchGet)
	http.HandleFunc("/watch/set", watchSet)
	http.HandleFunc("/watch/pause", watchPause)
	http.HandleFunc("/watch/start", watchStart)
	http.HandleFunc("/watch/events", watchEvents)
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	if len(r.RequestURI) <= 1 {
		print("HTML page was requested.")
		io.WriteString(w, html)
		return
	}
	msg := fmt.Sprintf("Unknown endpoint: %s", r.RequestURI)
	print(msg)
	io.WriteString(w, msg)
}

func serveScript(w http.ResponseWriter, r *http.Request) {
	print("script.js was requested.")
	// Preloading page script is essential
	io.WriteString(w, script)
}
func servePlayer(w http.ResponseWriter, r *http.Request) {
	print("fluid_player.js was requested.")
	http.ServeFile(w, r, FLUID_PLAYER_JS)
}
func serveFavicon(w http.ResponseWriter, r *http.Request) {
	print("favicon.ico was requested.")
	http.ServeFile(w, r, FAVICON)
}
func serveConstruction(w http.ResponseWriter, r *http.Request) {
	print("under_construction.png was requested.")
	http.ServeFile(w, r, CONSTRUCTION)
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
	io.WriteString(w, "<p> at /watchSet endpoint!\n</p>")
}

func watchStart(w http.ResponseWriter, r *http.Request) {
	state.playing.Swap(true)
	if r.Method != "POST" {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeEvent(eWriter, true, true)
	}
	print("watchStart was called")
	io.WriteString(w, "<p> at /watchStart endpoint!\n</p>")
}

func watchPause(w http.ResponseWriter, r *http.Request) {
	state.playing.Swap(false)
	if r.Method != "POST" {
		return
	}
	for _, eWriter := range eventWriters.slice {
		writeEvent(eWriter, false, true)
	}
	print("watchPause was called")
	io.WriteString(w, "<p> at /watchPause endpoint!\n</p>")
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
		writeEvent(w, state.playing.Load(), false)

		time.Sleep(2 * time.Second)
	}
}

func writeEvent(writer http.ResponseWriter, playing bool, haste bool) {
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

	fmt.Fprintln(writer, "id:", state.eventId)
	fmt.Fprintln(writer, "event:", eventType)
	fmt.Fprintln(writer, "data:", priority)
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
	playing atomic.Bool
	eventId uint64
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
