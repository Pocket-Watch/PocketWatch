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

const LIMITER_HITS = 800
const LIMITER_PER_SECOND = 5
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

const PROXY_ROUTE = "/watch/proxy/"
const STREAM_ROUTE = "/watch/stream/"
const WEB_STREAM = "web/stream/"
const WEB_PROXY = "web/proxy/"
const WEB_MEDIA = "web/media/"
const MEDIA = "media/"
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

const M3U8_CONTENT_TYPE = "application/vnd.apple.mpegurl"
const MAX_MESSAGE_CHARACTERS = 1000
const GENERIC_CHUNK_SIZE = 1 * MB

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

type Server struct {
	config ServerConfig
	state  ServerState
	users  *Users
	conns  *Connections
	db     *sql.DB
}

type PlayerState struct {
	Playing   bool    `json:"playing"`
	Autoplay  bool    `json:"autoplay"`
	Looping   bool    `json:"looping"`
	Timestamp float64 `json:"timestamp"`
}

type WelcomeMessage struct {
	Version string `json:"version"`
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

// TODO(kihau): Dynamically add/remove metadata from website room tab when needed.
// NOTE(kihau): Placeholder, Not used anywhere yet.
type Metadata struct {
	TrackNumber int    `json:"track_number"`
	AlbumName   string `json:"album_name"`
	AuthorName  string `json:"author_name"`
	ReleaseDate int    `json:"release_date"`
	Duration    int    `json:"duration"`
}

type Entry struct {
	Id         uint64 `json:"id"`
	Url        string `json:"url"`
	Title      string `json:"title"`
	UserId     uint64 `json:"user_id"`
	UseProxy   bool   `json:"use_proxy"`
	RefererUrl string `json:"referer_url"`
	SourceUrl  string `json:"source_url"`
	ProxyUrl   string `json:"proxy_url"`
	// NOTE(kihau): Placeholder until client side source switching is implemented.
	// Sources    []Source   `json:"sources"`
	Subtitles []Subtitle `json:"subtitles"`
	Thumbnail string     `json:"thumbnail"`
	Created   time.Time  `json:"created"`
}

type ServerState struct {
	mutex sync.Mutex

	player     PlayerState
	entry      Entry
	entryId    atomic.Uint64
	isLoading  atomic.Bool
	lastUpdate time.Time

	playlist  []Entry
	history   []Entry
	messages  []ChatMessage
	messageId uint64
	subsId    atomic.Uint64

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
	contentLength       int64
	extensionWithDot    string
	fileUrl             string
	file                *os.File
	contentRanges       []Range // must remain sorted
	rangesMutex         sync.Mutex
	download            *http.Response
	downloadMutex       sync.Mutex
	downloadBeginOffset int64
	referer             string
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
	mutex     sync.Mutex
	idCounter uint64
	slice     []Connection
	upgrader  websocket.Upgrader
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
	Player PlayerState `json:"player"`
	Entry  Entry       `json:"entry"`
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
	Type string `json:"type"`
	Data any    `json:"data"`
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
