package main

import (
	"net/http"
	"os"
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

const MAX_MESSAGE_CHARACTERS = 1000
const GENERIC_CHUNK_SIZE = 4 * MB

// Constants - assignable only once!
var serverRootAddress string
var startTime = time.Now()
var subsEnabled bool
var serverDomain string

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

// NOTE(kihau): Placeholder until client side source switching is implemented.
type Source struct {
	AudioUrl  string `json:"audio_url"`
	VideoUrl  string `json:"video_url"`
	AudioOnly bool   `json:"audio_only"`
	Quality   string `json:"quality" ` // ex. "1080p"?
	Type      string `json:"type" `    // ex. "hls_vod", "live", "file"?
}

type Entry struct {
	Id         uint64 `json:"id"`
	Url        string `json:"url"`
	Title      string `json:"title"`
	UserId     uint64 `json:"user_id"`
	UseProxy   bool   `json:"use_proxy"`
	RefererUrl string `json:"referer_url"`
	SourceUrl  string `json:"source_url"`
	// NOTE(kihau): Placeholder until client side source switching is implemented.
	// Sources    []Source   `json:"sources"`
	Subtitles []Subtitle `json:"subtitles"`
	Thumbnail string     `json:"thumbnail"`
	Created   time.Time  `json:"created"`
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
	events chan string
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

type FetchedSegment struct {
	realUrl  string
	obtained bool
	mutex    sync.Mutex
	created  time.Time
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
