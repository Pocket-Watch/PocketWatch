package main

import (
	"database/sql"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const KB = 1024
const MB = 1024 * KB
const GB = 1024 * MB

const LIMITER_HITS = 80
const LIMITER_PER_SECOND = 5
const RETRY = 5000 // Retry time in milliseconds
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
const MAX_CHUNK_SIZE = 10 * MB

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

// Constants - assignable only once!
var serverRootAddress string
var startTime = time.Now()

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
	AutorName   string `json:"autor_name"`
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

	player    PlayerState
	entry     Entry
	entryId   atomic.Uint64
	isLoading atomic.Bool

	eventId    atomic.Uint64
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
	events chan string
	close  chan bool
}

type Connections struct {
	mutex     sync.Mutex
	idCounter uint64
	slice     []Connection
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

type ChatMessageDeleteRequest struct {
	Id uint64 `json:"id"`
}

type ChatMessageFromUser struct {
	Message string `json:"message"`
	Edited  bool   `json:"edited"`
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
	// Subtitles []string    `json:"subtitles"`
}

type SyncRequest struct {
	Timestamp float64 `json:"timestamp"`
}

type SyncEvent struct {
	Timestamp float64 `json:"timestamp"`
	Action    string  `json:"action"`
	UserId    uint64  `json:"user_id"`
}

type PlayerSetRequest struct {
	RequestEntry RequestEntry `json:"request_entry"`
}

type PlayerSetEvent struct {
	PrevEntry Entry `json:"prev_entry"`
	NewEntry  Entry `json:"new_entry"`
}

type PlaybackEnded struct {
	EntryId uint64 `json:"entry_id"`
}

type PlayerNextRequest struct {
	EntryId uint64 `json:"entry_id"`
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
	Url  string `json:"url"`
	Name string `json:"name"`
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

type PlaylistPlayRequest struct {
	EntryId uint64 `json:"entry_id"`
}

type PlaylistAddRequest struct {
	ConnectionId uint64       `json:"connection_id"`
	RequestEntry RequestEntry `json:"request_entry"`
}

type PlaylistRemoveRequest struct {
	ConnectionId uint64 `json:"connection_id"`
	EntryId      uint64 `json:"entry_id"`
}

type PlaylistAutoplayRequest struct {
	ConnectionId uint64 `json:"connection_id"`
	Autoplay     bool   `json:"autoplay"`
}

type PlaylistLoopingRequest struct {
	ConnectionId uint64 `json:"connection_id"`
	Looping      bool   `json:"looping"`
}

type PlaylistMoveRequest struct {
	ConnectionId uint64 `json:"connection_id"`
	EntryId      uint64 `json:"entry_id"`
	DestIndex    int    `json:"dest_index"`
}

type PlaylistMoveEvent struct {
	EntryId   uint64 `json:"entry_id"`
	DestIndex int    `json:"dest_index"`
}

type PlaylistUpdateRequest struct {
	Entry Entry `json:"entry"`
}

type MediaUploadResponse struct {
	Url      string `json:"url"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Format   string `json:"format"`
	Category string `json:"category"`
}
