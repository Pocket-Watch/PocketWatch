package main

import (
	"encoding/json"
	"fmt"
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

func isYoutubeSourceExpired(sourceUrl string) bool {
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

type YoutubeVideo struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail"`
	OriginalUrl string `json:"original_url"`
	SourceUrl   string `json:"url"` // NOTE(kihau): This will be replaced with youtube sources structure.
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
	command := exec.Command("yt-dlp", nextEntry.Url, "--playlist-items", "1", "--extract-audio", "--print", "%(.{id,title,thumbnail,original_url,url})j")
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return
	}

	var video YoutubeVideo
	json.Unmarshal(output, &video)

	nextEntry.Thumbnail = video.Thumbnail
	nextEntry.SourceUrl = video.SourceUrl

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

func (server *Server) loadYoutubePlaylist(query string, videoId string, userId uint64, size uint) {
	url, err := neturl.Parse(query)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return
	}

	if !url.Query().Has("list") {
		query := url.Query()
		query.Add("list", "RD"+videoId)
		url.RawQuery = query.Encode()

		LogDebug("%v", url)
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
	server.state.playlist = append(server.state.playlist, entries...)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("addmany", entries)
	server.writeEventToAllConnections(nil, "playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
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
	}

	command := exec.Command("yt-dlp", query, "--playlist-items", "1", "--extract-audio", "--print", "%(.{id,title,thumbnail,original_url,url})j")
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return
	}

	var video YoutubeVideo
	json.Unmarshal(output, &video)

	entry.Url = video.OriginalUrl
	entry.Title = video.Title
	entry.SourceUrl = video.SourceUrl
	entry.Thumbnail = video.Thumbnail

	if requested.SearchVideo {
		query = video.OriginalUrl
	}

	if requested.IsPlaylist {
		go server.loadYoutubePlaylist(query, video.Id, entry.UserId, requested.PlaylistMaxSize)
	}
}
