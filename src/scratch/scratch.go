package scratch

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const ANNOUNCE_RECEIVED = true
const BODY_LIMIT = 1024
const RETRY = 5000 // Retry time in milliseconds
const TOKEN_LENGTH = 32

const PROXY_ROUTE = "/watch/proxy/"
const WEB_PROXY = "web/proxy/"
const WEB_MEDIA = "web/media/"
const ORIGINAL_M3U8 = "original.m3u8"
const PROXY_M3U8 = "proxy.m3u8"

var SUBTITLE_EXTENSIONS = [...]string{".vtt", ".srt"}

const MAX_SUBTITLE_SIZE = 512 * 1024

type Entry struct {
	Id          uint64 `json:"id"`
	Url         string `json:"url"`
	Title       string `json:"title"`
	RequestedBy uint64 `json:"requested_by"`
	useProxy    bool
	refererUrl  string
	created     time.Time
}

type PlayerState struct {
	Playing   bool    `json:"playing"`
	Autoplay  bool    `json:"timestamp"`
	Looping   bool    `json:"autoplay"`
	Timestamp float64 `json:"looping"`
}

type User struct {
	Id         uint64 `json:"id"`
	Username   string `json:"username"`
	Avatar     string `json:"avatar"`
	Connected  bool   `json:"connected"`
	token      string
	created    time.Time
	lastUpdate time.Time
	writer     *http.ResponseWriter
}

type ServerState struct {
	mutex          sync.RWMutex
	entry          Entry
	entryIdCounter uint64
	player         PlayerState
	users          []User
	userIdCounter  uint64
	playlist       []Entry
	history        []Entry
	eventId        uint64
	lastUpdate     time.Time
}

type GetResponse struct {
	Entry    Entry       `json:"entry"`
	State    PlayerState `json:"state"`
	Users    []User      `json:"users"`
	Playlist []Entry     `json:"playlist"`
	History  []Entry     `json:"history"`
	Subs     []string    `json:"subtitles"`
}

type SetUrlRequest struct {
	Token      string `json:"token"`
	Url        string `json:"url"`
	Title      string `json:"title"`
	UseProxy   bool   `json:"use_proxy"`
	RefererUrl string `json:"referer_url"`
}

type SetUrlEvent struct {
	UserId  uint64 `json:"user_id"`
	EntryId uint64 `json:"entry_id"`
	Url     string `json:"url"`
	Title   string `json:"title"`
}

type SyncRequest struct {
	Token     string `json:"token"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
}

type SyncEvent struct {
	UserId    string `json:"user_id"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
}

func generateToken() string {
	bytes := make([]byte, TOKEN_LENGTH)
	_, err := rand.Read(bytes)

	if err != nil {
		log_error("Token generation failed, this should not happen!")
		return ""
	}

	return base64.URLEncoding.EncodeToString(bytes)
}

// type Connections struct {
// 	slice []Connection
// }
// func makeConnections() *Connections {
// 	conns := new(Connections)
// 	conns.slice = make([]Connection, 0)
// 	conns.id_counter = 1
// 	return conns
// }
//
//
// func (conns *Connections) create(writer http.ResponseWriter) Connection {
// 	conn := Connection{
// 		Id:       conns.id_counter,
// 		Username: "",
// 		token:    generateToken(),
// 		writer:   writer,
// 	}
// 	conns.slice = append(conns.slice, conn)
// 	conns.id_counter += 1
//
// 	return conn
// }
//
// func (conns *Connections) remove(id uint64) {
// 	for i, conn := range conns.slice {
// 		if conn.Id != id {
// 			continue
// 		}
//
// 		length := conns.len()
// 		conns.slice[i], conns.slice[length-1] = conns.slice[length-1], conns.slice[i]
// 		conns.slice = conns.slice[:length-1]
// 		break
// 	}
// }
//
// func (conns *Connections) len() int {
// 	return len(conns.slice)
// }

func pruneOldUsers(state *ServerState) {
	for {
		time.Sleep(time.Hour * 24)

		state.mutex.Lock()
		for _, user := range state.users {
			_ = user
		}
		state.mutex.Unlock()
	}
}

func StartServer(options *Options) {
	state := ServerState{
		// entry:       Entry{},
		// playerState: PlayerState{},
		// connections: makeConnections(),
		// playlist:    make([]Entry, 0),
		// history:     make([]Entry, 0),
		// eventId:     1,
		// lastUpdate:  time.Now(),
	}

	go pruneOldUsers(&state)

	registerEndpoints(&state, options)

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

func registerEndpoints(state *ServerState, options *Options) {
	// TODO: Fix trailing suffix
	fileserver := http.FileServer(http.Dir("./web"))
	http.Handle("/", http.StripPrefix("/watch/", fileserver))

	http.HandleFunc("/watch/api/version", state.apiVersion)
	http.HandleFunc("/watch/api/createsession", state.apiCreateSession)

	// http.HandleFunc("/watch/api/login", state.apiLogin)
	// http.HandleFunc("/watch/api/get", state.apiGet)
	// http.HandleFunc("/watch/api/seturl", state.apiSetUrl)
	// http.HandleFunc("/watch/api/play", apiPlay)
	// http.HandleFunc("/watch/api/pause", apiPause)
	// http.HandleFunc("/watch/api/seek", apiSeek)
	// http.HandleFunc("/watch/api/upload", apiUpload)
	//
	// http.HandleFunc("/watch/api/playlist/get", apiPlaylistGet)
	// http.HandleFunc("/watch/api/playlist/add", apiPlaylistAdd)
	// http.HandleFunc("/watch/api/playlist/clear", apiPlaylistClear)
	// http.HandleFunc("/watch/api/playlist/next", apiPlaylistNext)
	// http.HandleFunc("/watch/api/playlist/remove", apiPlaylistRemove)
	// http.HandleFunc("/watch/api/playlist/autoplay", apiPlaylistAutoplay)
	// http.HandleFunc("/watch/api/playlist/looping", apiPlaylistLooping)
	// http.HandleFunc("/watch/api/playlist/shuffle", apiPlaylistShuffle)
	// http.HandleFunc("/watch/api/playlist/move", apiPlaylistMove)
	//
	// http.HandleFunc("/watch/api/history/get", apiHistoryGet)
	// http.HandleFunc("/watch/api/history/clear", apiHistoryClear)
	//
	// http.HandleFunc("/watch/api/events", apiEvents)
	// http.HandleFunc(PROXY_ROUTE, watchProxy)
}

func (state *ServerState) apiVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Expected http method GET.", http.StatusMethodNotAllowed)
		return
	}

	log_info("Connection %s requested server version.", r.RemoteAddr)
	io.WriteString(w, VERSION)
}

func (state *ServerState) apiCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Expected http method GET.", http.StatusMethodNotAllowed)
		return
	}

	log_info("Connection %s requested session creation.", r.RemoteAddr)

	state.mutex.Lock()
	new_user := User{
		Id:        state.userIdCounter,
		Username:  fmt.Sprintf("User %v", state.userIdCounter),
		Avatar:    "",
		Connected: false,
		token:     generateToken(),
		writer:    nil,
	}
	state.users = append(state.users, new_user)
	state.userIdCounter += 1
	state.mutex.Unlock()

	log_debug("Created new session id%v with token: %v", new_user.Id, new_user.token)
	io.WriteString(w, new_user.token)
}
