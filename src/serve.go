package main

import (
	"fmt"
	"io"
	"net/http"
	_ "os"
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
	if err := http.ListenAndServe(address, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
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
