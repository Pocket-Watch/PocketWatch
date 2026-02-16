package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
const MAX_UNKNOWN_PATH_LENGTH = 40
const MAX_HISTORY_SIZE = 120
const MAX_CHAT_LOAD = 100
const MAX_SPEED = 2.5
const MIN_SPEED = 0.1
const MAX_THUMBNAIL_COUNT = 100
const MAX_SHARE_LIFETIME_SECONDS = 365 * 24 * 60 * 60

const SUBTITLE_SIZE_LIMIT = 512 * KB
const AVATAR_SIZE_LIMIT = 8 * MB
const PROXY_FILE_SIZE_LIMIT = 4 * GB
const BODY_LIMIT = 8 * KB
const MAX_STREAM_CHUNK_SIZE = 10 * MB
const MAX_CHUNK_SIZE = 50 * MB
const MAX_PRELOAD_SIZE = 20 * MB
const MAX_THUMBNAIL_SIZE = 4 * MB
const HEURISTIC_BITRATE_MB_S = 1.75 * MB

var SUBTITLE_EXTENSIONS = [...]string{".vtt", ".srt"}

// Read-only. Assigned on server startup.
var PAGE_ROOT string
var PROXY_ROUTE string
var STREAM_ROUTE string
var CONTENT_ROUTE string

func configureRoutes() {
	PAGE_ROOT += fmt.Sprintf("/room/%s/", randomBase64(6))
	PROXY_ROUTE = PAGE_ROOT + "proxy/"
	STREAM_ROUTE = PAGE_ROOT + "stream/"
	CONTENT_ROUTE = PAGE_ROOT + "content/"
}

// const PAGE_ROOT = "/watch/"
// const PROXY_ROUTE = PAGE_ROOT + "proxy/"
// const STREAM_ROUTE = PAGE_ROOT + "stream/"
// const CONTENT_ROUTE = PAGE_ROOT + "content/"

const WEB_ROOT = "web/"

const CONTENT_ROOT = "content/"
const CONTENT_MEDIA = CONTENT_ROOT + "media/"
const CONTENT_PROXY = CONTENT_ROOT + "proxy/"
const CONTENT_STREAM = CONTENT_ROOT + "stream/"
const CONTENT_SUBS = CONTENT_ROOT + "subs/"
const CONTENT_USERS = CONTENT_ROOT + "users/"

const MEDIA_VIDEO = CONTENT_MEDIA + "video/"
const MEDIA_AUDIO = CONTENT_MEDIA + "audio/"
const MEDIA_SUBS = CONTENT_MEDIA + "subs/"
const MEDIA_IMAGE = CONTENT_MEDIA + "image/"
const MEDIA_THUMB = CONTENT_MEDIA + "thumb/"

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
const MEDIA_INIT_SECTION_AUDIO = "mis-audio.key"
const MEDIA_DECRYPTION_KEY = "decrypt.key"
const MEDIA_DECRYPTION_KEY_AUDIO = "decrypt-audio.key"
const MAX_PLAYLIST_DEPTH = 2
const MAX_CHUNK_NAME_LENGTH = 26
const MAX_PLAYLIST_DURATION_SECONDS = 86400 // 24hours
const MIN_SEGMENT_LENGTH = 0.038            // 38ms

const M3U8_CONTENT_TYPE = "application/vnd.apple.mpegurl"
const MAX_MESSAGE_CHARACTERS = 1000
const GENERIC_CHUNK_SIZE = 1_000_000
const TRAILING_PULL_SIZE = 256 * KB
const PRE_OFFSET = 256 * KB

type EventType uint64

const (
	EVENT_PLAYER_PLAY EventType = iota
	EVENT_PLAYER_PAUSE
	EVENT_PLAYER_SEEK
	EVENT_PLAYER_SET
	EVENT_PLAYER_NEXT
	EVENT_PLAYER_AUTOPLAY
	EVENT_PLAYER_LOOPING
	EVENT_PLAYER_UPDATE_TITLE
	EVENT_PLAYER_SPEED_CHANGE

	EVENT_CHAT_SEND
	EVENT_CHAT_EDIT
	EVENT_CHAT_DELETE

	EVENT_PLAYLIST_ADD
	EVENT_PLAYLIST_PLAY
	EVENT_PLAYLIST_MOVE
	EVENT_PLAYLIST_CLEAR
	EVENT_PLAYLIST_DELETE
	EVENT_PLAYLIST_UPDATE
	EVENT_PLAYLIST_SHUFFLE
)

type PlayerSyncType uint64

const (
	PLAYER_SYNC_PLAY PlayerSyncType = iota
	PLAYER_SYNC_PAUSE
	PLAYER_SYNC_SEEK
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

	// Indicates the playback speed, default: x1
	Speed float64 `json:"speed"`
}

// Server welcome event, sent to the client as a first event when a WebSocket connection is opened.
type WelcomeMessage struct {
	// Current server version. Currently based on the server build time.
	Version string `json:"version"`
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

type Invite struct {
	InviteCode string    `json:"invite_code"`
	ExpiresAt  time.Time `json:"expires_after"`
	CreatedBy  uint64    `json:"created_by"`
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
	ArtistName  string `json:"artist_name"`

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

	// ID of the user that set the entry.
	SetById uint64 `json:"set_by_id"`

	// Whether an entry should be loaded via a server proxy.
	UseProxy bool `json:"use_proxy"`

	// Proxy referrer URL.
	RefererUrl string `json:"referer_url"`

	// Represents the primary URL that the media is seeded from
	SourceUrl string `json:"source_url"`

	// Used when SplitTracks is true, it can be also used for background playback
	AudioUrl string `json:"audio_url"`

	// Proxied SourceUrl, it should be used first if available
	ProxyUrl string `json:"proxy_url"`

	// Indicates whether the audio & video track are separate and must be played in sync
	SplitTracks bool `json:"split_tracks"`

	// NOTE(kihau): Placeholder until client side source switching is implemented.
	// Sources    []Source   `json:"sources"`

	// Attached subtitles (or lyrics)
	Subtitles []Subtitle `json:"subtitles"`

	// Entry network URL path of the thumbnail.
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

	// A room invite for the website. With time, this will become an invite list, and later, grant per-room access (instead of website-wide access).
	invite Invite

	// Room resources shared outside of it
	resources    map[string]SharedResource
	resourceLock sync.RWMutex

	// Indicates whether the server is waiting for the entry to load. Loading includes both YouTube fetch and proxy setup.
	isLoadingEntry atomic.Bool

	// Indicates whether the server is currently fetching or has fetched lyrics for current entry.
	isLyricsFetched atomic.Bool

	// Last update time of player timestamp.
	lastUpdate time.Time

	playlist []Entry
	history  []Entry
	messages []ChatMessage

	// Tiny array of recent actions which are displayed in "Recent Actions" section in the room tab.
	// Should be kept relatively small (somewhere between 3 and 10 elements).
	actions []Action

	// Setup lock for the proxy.
	setupLock      sync.Mutex
	proxy          *HlsProxy
	audioProxy     *HlsProxy
	isLive         bool
	isHls          bool
	fileProxy      FileProxy
	audioFileProxy FileProxy

	liveStream LiveStream
}

type GatewayHandler struct {
	handler             http.Handler
	ipToLimiters        map[string]*RateLimiter
	mapMutex            *sync.Mutex
	blacklistedIpRanges []IpV4Range
	hits, perSecond     int
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

type FileProxy struct {
	url           string
	referer       string
	contentLength int64
	filename      string
	contentType   string
	downloader    *GenericDownloader
	file          *os.File
	fileMutex     sync.RWMutex
	diskRanges    []Range // must remain sorted
	rangeMutex    sync.Mutex
}

func (proxy *FileProxy) loadBytes(offset, count int64) bool {
	response, err := openFileDownload(proxy.url, offset, proxy.referer)
	if err != nil {
		LogWarn("Failed open file download at offset=%v due to %v", offset, err)
		return false
	}
	defer response.Body.Close()
	_, err = proxy.pullAndStoreBytes(response, offset, count)
	if err != nil {
		LogWarn("Failed to pull last %v bytes: %v", count, err)
		return false
	}
	return true
}

type GenericDownloader struct {
	mutex    sync.Mutex // locks download, offset
	download *http.Response
	offset   int64
	preload  int64
	speed    *SpeedTest
	sleeper  *Sleeper
	destroy  chan bool
	closed   bool
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
	mutex sync.Mutex
	slice []User
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

type PlayerNextRequest struct {
	CurrentEntryId uint64 `json:"current_entry_id"`
	Programmatic   bool   `json:"programmatic"`
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

type UserVerifyResponse struct {
	UserId   uint64 `json:"user_id"`
	PagePath string `json:"page_path"`
}

type PlayerSyncRequest struct {
	Timestamp      float64 `json:"timestamp"`
	Programmatic   bool    `json:"programmatic"`
	CurrentEntryId uint64  `json:"current_entry_id"`
}

type SharedResource struct {
	path    string
	expires time.Time
}

type ShareResourceRequest struct {
	Url             string `json:"url"`
	LifetimeSeconds uint   `json:"lifetime_seconds"`
}

type ShareResourceResponse struct {
	SharedPath string `json:"shared_path"`
}
