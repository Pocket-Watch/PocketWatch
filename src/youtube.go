package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var YOUTUBE_ENABLED bool = true

func isYoutubeUrl(url string) bool {
	parsedUrl, err := neturl.Parse(url)
	if err != nil {
		return false
	}

	host := parsedUrl.Host
	return strings.HasSuffix(host, "youtube.com") || strings.HasSuffix(host, "youtu.be")
}

func isYoutubeSourceExpiredFile(sourceUrl string) bool {
	if sourceUrl == "" {
		return true
	}

	parsedUrl, err := neturl.Parse(sourceUrl)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return true
	}

	expire := parsedUrl.Query().Get("expire")
	expire_unix, err := strconv.ParseInt(expire, 10, 64)
	if err != nil {
		LogError("Failed to parse expiration time from youtube source url: %v", err)
		return true
	}

	now_unix := time.Now().Unix()
	return now_unix > expire_unix
}

func isYoutubeSourceExpiredM3U(sourceUrl string) bool {
	if sourceUrl == "" {
		return true
	}

	parsedUrl, err := neturl.Parse(sourceUrl)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return true
	}

	LogDebug("Checking expiration for source url: %v", sourceUrl)
	segments := strings.Split(parsedUrl.Path, "/")

	index := -1
	for i, segment := range segments {
		if segment == "expire" {
			index = i + 1
			break
		}
	}

	if index == -1 || len(segments) <= index {
		LogError("Failed to find expiration time within the YouTube m3u8_native source URL: %v", sourceUrl)
		return true
	}

	expire := segments[index]
	expire_unix, err := strconv.ParseInt(expire, 10, 64)
	if err != nil {
		LogError("Failed to parse expiration time from youtube source url: %v", err)
		return true
	}

	now_unix := time.Now().Unix()
	return now_unix > expire_unix
}

func isYoutubeSourceExpired(sourceUrl string) bool {
	return isYoutubeSourceExpiredM3U(sourceUrl)
}

type YoutubeVideo struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail"`
	OriginalUrl string `json:"original_url"`
	AudioUrl    string `json:"audio_url"`
	VideoUrl    string `json:"video_url"`
}

type YoutubeContent struct {
	Type string `json:"_type"`
}

type YoutubeThumbnail struct {
	Url    string `json:"url"`
	Height uint64 `json:"height"`
	Width  uint64 `json:"width"`
}

type YoutubePlaylistVideo struct {
	Url        string             `json:"url"`
	Title      string             `json:"title"`
	Thumbnails []YoutubeThumbnail `json:"thumbnails"`
}

type YoutubePlaylist struct {
	Entries []YoutubePlaylistVideo `json:"entries"`
}

func (server *Server) preloadYoutubeSourceOnNextEntry() {
	server.state.mutex.Lock()
	if len(server.state.playlist) == 0 {
		server.state.mutex.Unlock()
		return
	}

	nextEntry := server.state.playlist[0]
	server.state.mutex.Unlock()

	if !isYoutubeUrl(nextEntry.Url) {
		return
	}

	if !isYoutubeSourceExpired(nextEntry.SourceUrl) {
		return
	}

	LogInfo("Preloading youtube source for an entry with an ID: %v", nextEntry.Id)
	ok, video := fetchYoutubeVideo(nextEntry.Url)
	if !ok {
		return
	}

	nextEntry.Thumbnail = video.Thumbnail
	nextEntry.SourceUrl = video.AudioUrl

	server.state.mutex.Lock()
	if len(server.state.playlist) == 0 {
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[0].Id == nextEntry.Id {
		server.state.playlist[0] = nextEntry
	}
	server.state.mutex.Unlock()
}

func pickBestThumbnail(thumbnails []YoutubeThumbnail) string {
	bestThumbnail := ""
	var smallestSize uint64 = 0

	for _, thumbnail := range thumbnails {
		thumbnailSize := thumbnail.Height * thumbnail.Width
		if thumbnailSize < smallestSize {
			smallestSize = thumbnailSize
			bestThumbnail = thumbnail.Url
		}
	}

	return bestThumbnail
}

func (server *Server) loadYoutubePlaylist(query string, videoId string, userId uint64, size uint, addToTop bool) {
	url, err := neturl.Parse(query)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return
	}

	if !url.Query().Has("list") {
		query := url.Query()
		query.Add("list", "RD"+videoId)
		url.RawQuery = query.Encode()

		LogDebug("Url was not a playlist. Constructed youtube playlist url is now: %v", url)
	}

	if size > 1000 {
		size = 1000
	} else if size == 0 {
		size = 20
	}

	query = url.String()
	end := fmt.Sprintf("%d", size)
	command := exec.Command("yt-dlp", query, "--flat-playlist", "--playlist-start", "2", "--playlist-end", end, "--dump-single-json")
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return
	}

	var playlist YoutubePlaylist
	json.Unmarshal(output, &playlist)

	if len(playlist.Entries) == 0 {
		LogError("Deserialized yt-dlp array for url '%v' is empty", query)
		return
	}

	entries := make([]Entry, 0)

	for _, ytEntry := range playlist.Entries {
		server.state.mutex.Lock()
		server.state.entryId += 1

		entry := Entry{
			Id:         server.state.entryId,
			Url:        ytEntry.Url,
			Title:      ytEntry.Title,
			UserId:     userId,
			UseProxy:   false,
			RefererUrl: "",
			SourceUrl:  "",
			Thumbnail:  pickBestThumbnail(ytEntry.Thumbnails),
			Created:    time.Now(),
		}

		if len(ytEntry.Thumbnails) > 0 {
			entry.Thumbnail = ytEntry.Thumbnails[0].Url
		}

		entries = append(entries, entry)
		server.state.mutex.Unlock()
	}

	server.state.mutex.Lock()
	if addToTop {
		server.state.playlist = append(entries, server.state.playlist...)
	} else {
		server.state.playlist = append(server.state.playlist, entries...)
	}
	server.state.mutex.Unlock()

	var event PlaylistEventData
	if addToTop {
		event = createPlaylistEvent("addmanytop", entries)
	} else {
		event = createPlaylistEvent("addmany", entries)
	}
	server.writeEventToAllConnections(nil, "playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

type InternalServerFetch struct {
	Query string `json:"query"`
}

func fetchVideoWithInternalServer(query string) (bool, YoutubeVideo) {
	data := InternalServerFetch{
		Query: query,
	}

	request, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to marshal JSON request data for the internal server: %v", err)
		return false, YoutubeVideo{}
	}

	response, nil := http.Post("http://localhost:2345/youtube/fetch", "application/json", bytes.NewBuffer(request))
	if err != nil {
		LogError("Request POST to the internal server failed: %v", err)
		return false, YoutubeVideo{}
	}
	defer response.Body.Close()

	var video YoutubeVideo
	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		LogError("Failed to unmarshal data from the internal server: %v", err)
		return false, YoutubeVideo{}
	}
	json.Unmarshal(responseData, &video)
	return true, video
}

func fetchVideoWithYtdlp(query string) (bool, YoutubeVideo) {
	// NOTE(kihau): Format 234 is m3u8_native audio-only source which can be used in combination with HLS proxy.
	command := exec.Command("yt-dlp", query, "--playlist-items", "1", "--format", "234", "--print", "%(.{id,title,thumbnail,original_url,url})j")
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return false, YoutubeVideo{}
	}

	var video YoutubeVideo
	err = json.Unmarshal(output, &video)
	if err != nil {
		LogError("Failed to unmarshal yt-dlp output json: %v", err)
		return false, YoutubeVideo{}
	}

	return true, video
}

func fetchYoutubeVideo(query string) (bool, YoutubeVideo) {
	ok, video := fetchVideoWithInternalServer(query)
	if ok {
		return true, video
	}

	LogWarn("Internal server video fetch failed. Falling back to yt-dlp command fetch.")
	ok, video = fetchVideoWithYtdlp(query)
	if ok {
		return true, video
	}

	return false, YoutubeVideo{}
}

func (server *Server) loadYoutubeEntry(entry *Entry, requested RequestEntry) {
	if !YOUTUBE_ENABLED {
		return
	}

	if !isYoutubeUrl(entry.Url) && !requested.SearchVideo {
		return
	}

	if !isYoutubeSourceExpired(entry.SourceUrl) {
		return
	}

	query := entry.Url
	if requested.SearchVideo {
		query = "ytsearch:" + query
		LogInfo("Searching for youtube video with query: %v.", entry.Url)
	} else {
		LogInfo("Loading youtube entry with url: %v.", entry.Url)
	}

	ok, video := fetchYoutubeVideo(query)
	if !ok {
		return
	}

	entry.Url = video.OriginalUrl
	entry.Title = video.Title
	entry.SourceUrl = video.AudioUrl
	entry.Thumbnail = video.Thumbnail

	if requested.SearchVideo {
		query = video.OriginalUrl
	}

	if requested.IsPlaylist {
		go server.loadYoutubePlaylist(query, video.Id, entry.UserId, requested.PlaylistMaxSize, requested.AddToTop)
	}
}
