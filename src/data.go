package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const KB = 1024
const MB = 1024 * KB
const GB = 1024 * MB

const SERVER_ID = 0

const STATIC_LIMITER_HITS = 150
const STATIC_LIMITER_PER_SECOND = 6
const CONTENT_LIMITER_HITS = 440
const CONTENT_LIMITER_PER_SECOND = 8
const TOKEN_LENGTH = 32
const BROADCAST_INTERVAL = 2 * time.Second
const HEARTBEAT_INTERVAL = 2 * time.Second
const BLACK_HOLE_PERIOD = 20 * time.Minute

const MAX_NICKNAME_LENGTH = 255
const MAX_UNKNOWN_PATH_LENGTH = 30
const MAX_HISTORY_SIZE = 120
const MAX_CHAT_LOAD = 100

const SUBTITLE_SIZE_LIMIT = 512 * KB
const AVATAR_SIZE_LIMIT = 8 * MB
const PROXY_FILE_SIZE_LIMIT = 4 * GB
const BODY_LIMIT = 8 * KB
const MAX_STREAM_CHUNK_SIZE = 10 * MB
const MAX_CHUNK_SIZE = 16 * MB

var SUBTITLE_EXTENSIONS = [...]string{".vtt", ".srt"}

const PAGE_ROOT = "/watch/"
const PROXY_ROUTE = PAGE_ROOT + "proxy/"
const STREAM_ROUTE = PAGE_ROOT + "stream/"
const CONTENT_ROUTE = PAGE_ROOT + "content/"

const WEB_ROOT = "web/"

const CONTENT_ROOT = "content/"
const CONTENT_MEDIA = CONTENT_ROOT + "media/"
const CONTENT_PROXY = CONTENT_ROOT + "proxy/"
const CONTENT_STREAM = CONTENT_ROOT + "stream/"
const CONTENT_SUBS = CONTENT_ROOT + "subs/"
const CONTENT_USERS = CONTENT_ROOT + "users/"

const ORIGINAL_M3U8 = "original.m3u8"
const PROXY_M3U8 = "proxy.m3u8"
const VIDEO_M3U8 = "video.m3u8"
const AUDIO_M3U8 = "audio.m3u8"
const STREAM_M3U8 = "stream.m3u8"
const VIDEO_PREFIX = "vi-"
const AUDIO_PREFIX = "au-"
const LIVE_PREFIX = "live-"
const MIS_PREFIX = "mis-"
const MEDIA_INIT_SECTION = "mis.key"
const MAX_PLAYLIST_DEPTH = 2
const MAX_CHUNK_NAME_LENGTH = 26
const MAX_PLAYLIST_DURATION_SECONDS = 86400 // 24hours
const MIN_SEGMENT_LENGTH = 0.038            // 38ms

const M3U8_CONTENT_TYPE = "application/vnd.apple.mpegurl"
const MAX_MESSAGE_CHARACTERS = 1000
const GENERIC_CHUNK_SIZE = 1_000_000

type EventType uint64

const (
	EVENT_PLAYER_PLAY         EventType = 0
	EVENT_PLAYER_PAUSE        EventType = 1
	EVENT_PLAYER_SEEK         EventType = 2
	EVENT_PLAYER_SET          EventType = 3
	EVENT_PLAYER_NEXT         EventType = 4
	EVENT_PLAYER_AUTOPLAY     EventType = 5
	EVENT_PLAYER_LOOPING      EventType = 6
	EVENT_PLAYER_UPDATE_TITLE EventType = 7

	EVENT_CHAT_SEND   EventType = 8
	EVENT_CHAT_EDIT   EventType = 9
	EVENT_CHAT_DELETE EventType = 10

	EVENT_PLAYLIST_ADD     EventType = 11
	EVENT_PLAYLIST_PLAY    EventType = 12
	EVENT_PLAYLIST_MOVE    EventType = 13
	EVENT_PLAYLIST_CLEAR   EventType = 14
	EVENT_PLAYLIST_DELETE  EventType = 15
	EVENT_PLAYLIST_UPDATE  EventType = 16
	EVENT_PLAYLIST_SHUFFLE EventType = 17
)

// Constants - assignable only once!
var serverRootAddress string
var startTime = time.Now()
var behindProxy bool

// Main server data structure.
type Server struct {
	// Server configuration loaded at startup.
	config ServerConfig

	// State of the server. Includes things such as current entry, playlist and history.
	state ServerState

	// User account data.
	users *Users

	// Currently open WebSocket connections.
	conns *Connections

	// Connection to the PostgreSQL database. When database support is disabled, this field is set to nil.
	db *sql.DB
}

// Current state of the player.
type PlayerState struct {
	// Indicates whether player playback is on or off.
	Playing bool `json:"playing"`

	// Indicates whether autoplay is on or off.
	// When autoplay is enabled and a playerNext event is received, first item from the playlist is set a current entry.
	Autoplay bool `json:"autoplay"`

	// Indicates whether looping is on or off.
	// When looping is set to true and a playerNext event is received, the server adds current entry to the end of the playlist
	// or if the playlist is empty - replay the current entry.
	//
	// In the future looping might include multiple modes such as:
	//     - disabled
	//     - player
	//     - playlist
	//     - ...
	Looping bool `json:"looping"`

	// Last saved player playback timestamp (in seconds).
	// Note that, the timestamp is only updated on user interaction (such as playerPlay, playerPause and playerSeek).
	// The actual player playback position can be calculated using the getCurrentTimestamp function.
	Timestamp float64 `json:"timestamp"`
}

// Server welcome event, sent to the client as a first event when a WebSocket connection is opened.
type WelcomeMessage struct {
	// Current server version. Currently based on the server build time.
	Version  string            `json:"version"`
}

type GetAllMessage struct {
	Users    []User            `json:"users"`
	Player   PlayerGetResponse `json:"player"`
	Playlist []Entry           `json:"playlist"`
	History  []Entry           `json:"history"`
	Messages []ChatMessage     `json:"messages"`
}

// Subtitle data layout
type Subtitle struct {
	// Unique ID of the subtitle, seeded by ServerState.subsId ID counter.
	Id uint64 `json:"id"`

	// Name of the subtitle displayed client side. Users are allowed to modify this name.
	Name string `json:"name"`

	// Resource URL to the subtitle.
	Url string `json:"url"`

	// Current subtitle shift, applied to all subtitle cues on the client side.
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

// TODO(kihau):
//
//	Dynamically add/remove metadata from website room tab when needed.
//	The metadata will include additional information for entries loaded via YTDlp,
//	as well as metadata for entries loaded from server storage (video and audio files)
type Metadata struct {
	TrackNumber int    `json:"track_number"`
	AlbumName   string `json:"album_name"`
	AuthorName  string `json:"author_name"`

	// TODO(kihau): Make those two time.Time instead.
	ReleaseDate string `json:"release_date"`
	Duration    int64  `json:"duration"`
}

type Entry struct {
	Id uint64 `json:"id"`

	// Original network URL path of the entry.
	Url string `json:"url"`

	// Entry title
	Title string `json:"title"`

	// ID of the user that created the entry.
	UserId uint64 `json:"user_id"`

	// Whether an entry should be loaded via a server proxy.
	UseProxy bool `json:"use_proxy"`

	// Proxy referrer URL.
	RefererUrl string `json:"referer_url"`
	SourceUrl  string `json:"source_url"`
	ProxyUrl   string `json:"proxy_url"`

	// NOTE(kihau): Placeholder until client side source switching is implemented.
	// Sources    []Source   `json:"sources"`
	Subtitles []Subtitle `json:"subtitles"`

	// Entry network URL path of the thumbnal.
	Thumbnail string `json:"thumbnail"`

	// Original creation time of the entry.
	CreatedAt time.Time `json:"created_at"`

	// Time when the entry was set as current.
	LastSetAt time.Time `json:"last_set_at"`

	// Optional metadata added on YtDlp entry fetch.
	Metadata Metadata `json:"metadata"`
}

type ServerState struct {
	mutex sync.Mutex

	// Current state of the player.
	player PlayerState

	// Currently set entry.
	entry Entry

	// Entry ID seed counter, incremented for every new entry.
	entryId atomic.Uint64

	// Indicates whether the server is waiting for the entry to load. Loading includes both YouTube fetch and proxy setup.
	isLoading atomic.Bool

	// Indicates server is fetching or searching a subtitle for current entry.
	isLoadingSubs atomic.Bool

	// Last update time of player timestamp.
	lastUpdate time.Time

	playlist []Entry
	history  []Entry
	messages []ChatMessage

	// Message ID seed counter, incremented for every new chat message.
	messageId uint64

	// Subtitle ID seed counter, incremented for every new subtitle.
	subsId atomic.Uint64

	// Tiny array of recent actions which are displayed in "Recent Actions" section in the room tab.
	// Should be kept relatively small (somewhere between 3 and 10 elements).
	actions []Action

	// Setup lock for the proxy.
	setupLock    sync.Mutex
	proxy        *HlsProxy
	audioProxy   *HlsProxy
	isLive       bool
	isHls        bool
	genericProxy GenericProxy

	liveStream LiveStream
}

type HlsProxy struct {
	// Common
	referer string
	// HLS proxy
	chunkLocks     []sync.Mutex
	fetchedChunks  []bool
	originalChunks []string
	// Live resources
	liveUrl      string
	liveSegments sync.Map
	lastRefresh  time.Time
}

type GenericProxy struct {
	contentLength    int64
	extensionWithDot string
	fileUrl          string
	referer          string
	rangeSeed        atomic.Uint64
	file             *os.File
	fileMutex        sync.Mutex
	diskRanges       []Range // must remain sorted
	futureRanges     []*FutureRange
	rangeMutex       sync.Mutex
}

type FutureRange struct {
	ready   chan struct{}
	success bool
	id      uint64
	r       Range
}

type RangeAction int

const (
	READ RangeAction = iota
	AWAIT
	FETCH
)

type ActionableRange struct {
	action RangeAction
	r      Range
	future *FutureRange // Present if action is AWAIT
}

type LiveStream struct {
	userId          uint64
	dataTransferred int64
	lastChunkId     uint64
}

type Connection struct {
	id     uint64
	userId uint64
	events chan []byte
	close  chan bool
}

type Connections struct {
	mutex sync.Mutex

	// Connection ID counter incremented for each new server-client connection established.
	idCounter uint64

	// An array of established connections with clients.
	slice []Connection

	// The gorilla WebSocket upgrader for HTTP WebSocket requests.
	upgrader websocket.Upgrader
}

type MuxPortPatterns struct {
	Mux      *http.ServeMux
	Port     uint16
	Patterns *Set[string]
}

type User struct {
	Id         uint64    `json:"id"`
	Username   string    `json:"username"`
	Avatar     string    `json:"avatar"`
	Online     bool      `json:"online"`
	CreatedAt  time.Time `json:"created_at"`
	LastUpdate time.Time `json:"last_update"`
	LastOnline time.Time `json:"last_online"`

	connections uint64
	token       string
}

type Users struct {
	mutex     sync.Mutex
	idCounter uint64
	slice     []User
}

type Action struct {
	Action string    `json:"action"`
	UserId uint64    `json:"user_id"`
	Data   any       `json:"data"`
	Date   time.Time `json:"date"`
}

type ChatMessage struct {
	Id        uint64 `json:"id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	EditedAt  int64  `json:"edited_at"`
	UserId    uint64 `json:"user_id"`
}

type ChatMessageEdit struct {
	MessageId uint64 `json:"message_id"`
	Content   string `json:"content"`
}

type MessageHistoryRequest struct {
	Count          uint32 `json:"count"`
	BackwardOffset uint64 `json:"backwardOffset"`
}

type LiveSegment struct {
	realUrl        string
	realMapUri     string
	obtainedUrl    bool
	obtainedMapUri bool
	mutex          sync.Mutex
	created        time.Time
}

type PlayerGetResponse struct {
	Player  PlayerState `json:"player"`
	Entry   Entry       `json:"entry"`
	Actions []Action    `json:"actions"`
}

type SyncEvent struct {
	Timestamp float64 `json:"timestamp"`
	Action    string  `json:"action"`
	UserId    uint64  `json:"user_id"`
}

type PlayerSetEvent struct {
	PrevEntry Entry `json:"prev_entry"`
	NewEntry  Entry `json:"new_entry"`
}

type PlayerNextEvent struct {
	PrevEntry Entry `json:"prev_entry"`
	NewEntry  Entry `json:"new_entry"`
}

type SubtitleUpdateRequest struct {
	Id   uint64 `json:"id"`
	Name string `json:"name"`
}

type SubtitleShiftRequest struct {
	Id    uint64  `json:"id"`
	Shift float64 `json:"shift"`
}

type SubtitleDownloadRequest struct {
	Url     string `json:"url"`
	Name    string `json:"name"`
	Referer string `json:"referer"`
}

type WebsocketEventResponse struct {
	Type   string `json:"type"`
	UserId uint64 `json:"user_id"`
	Data   any    `json:"data"`
}

type WebsocketDataResponse struct {
	Type   string          `json:"type"`
	UserId uint64          `json:"user_id"`
	Data   json.RawMessage `json:"data"`
}

type WebsocketEvent struct {
	Type EventType       `json:"type"`
	Data json.RawMessage `json:"data"`
}

type PlaylistEvent struct {
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
	FetchLyrics       bool       `json:"fetch_lyrics"`
	AddToTop          bool       `json:"add_to_top"`
	Subtitles         []Subtitle `json:"subtitles"`
	PlaylistSkipCount uint       `json:"playlist_skip_count"`
	PlaylistMaxSize   uint       `json:"playlist_max_size"`
}

type PlaylistMoveRequest struct {
	EntryId   uint64 `json:"entry_id"`
	DestIndex int    `json:"dest_index"`
}

type PlaylistMoveEvent struct {
	EntryId   uint64 `json:"entry_id"`
	DestIndex int    `json:"dest_index"`
}

type MediaUploadResponse struct {
	Url      string `json:"url"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Format   string `json:"format"`
	Category string `json:"category"`
}
