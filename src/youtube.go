package main

import (
	"encoding/json"
	neturl "net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var YOUTUBE_ENABLED bool = true

func isYoutubeUrl(url string) bool {
    if strings.HasPrefix(url, "ytsearch:") {
        return true
    }

	parsedUrl, err := neturl.Parse(url)
	if err != nil {
		return false
	}

	/*if parsedUrl.Scheme != "https" {
		return false
	}*/

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

func getYoutubeAudioSource(url string) string {
	cmd := exec.Command("yt-dlp", "--get-url", "--extract-audio", "--no-playlist", url)
	output, err := cmd.Output()
	if err != nil {
		LogError("Youtube audio source load failed for url %v with: %v", url, err)
		return ""
	}

	source := string(output)
	return strings.TrimSpace(source)
}

type YoutubeVideo struct {
	Url       string `json:"original_url"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
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

func preloadYoutubeSourceOnNextEntry() {
	state.mutex.RLock()
	if len(state.playlist) == 0 {
		state.mutex.RUnlock()
		return
	}

	nextEntry := state.playlist[0]
	state.mutex.RUnlock()

	if !isYoutubeUrl(nextEntry.Url) {
		return
	}

	if !isYoutubeSourceExpired(nextEntry.SourceUrl) {
		return
	}

	LogInfo("Preloading youtube source for an entry with an ID: %v", nextEntry.Id)
	audioSource := getYoutubeAudioSource(nextEntry.Url)
	nextEntry.SourceUrl = audioSource

	state.mutex.Lock()
	if len(state.playlist) == 0 {
		state.mutex.Unlock()
		return
	}

	if state.playlist[0].Id == nextEntry.Id {
		state.playlist[0] = nextEntry
	}
	state.mutex.Unlock()
}

func loadYoutubePlaylist(entry *Entry, playlist YoutubePlaylist) {
	if len(playlist.Entries) == 0 {
		LogError("Deserialized yt-dlp array for url '%v' is empty", entry.Url)
		return
	}

	firstYtEntry := playlist.Entries[0]
	playlist.Entries = playlist.Entries[1:]

	entry.Url = firstYtEntry.Url
	entry.Title = firstYtEntry.Title
	entry.SourceUrl = getYoutubeAudioSource(firstYtEntry.Url)

	userId := entry.UserId

	go func() {
		entries := make([]Entry, 0)

		for _, ytEntry := range playlist.Entries {
			state.mutex.Lock()
			state.entryId += 1

			entry := Entry{
				Id:         state.entryId,
				Url:        ytEntry.Url,
				Title:      ytEntry.Title,
				UserId:     userId,
				UseProxy:   false,
				RefererUrl: "",
				SourceUrl:  "",
				Created:    time.Now(),
			}

			entries = append(entries, entry)
			state.mutex.Unlock()
		}

		state.mutex.Lock()
		state.playlist = append(state.playlist, entries...)
		state.mutex.Unlock()

		event := createPlaylistEvent("addmany", entries)
		writeEventToAllConnections(nil, "playlist", event)
		preloadYoutubeSourceOnNextEntry()
	}()
}

func loadYoutubeEntry(entry *Entry) {
	if !YOUTUBE_ENABLED {
		return
	}

	go preloadYoutubeSourceOnNextEntry()

	if !isYoutubeUrl(entry.Url) {
		return
	}

	if !isYoutubeSourceExpired(entry.SourceUrl) {
		return
	}

	cmd := exec.Command("yt-dlp", "--flat-playlist", "--dump-single-json", "--playlist-end", "200", entry.Url)
	output, err := cmd.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return
	}

	var ytContent YoutubeContent
	err = json.Unmarshal(output, &ytContent)

	if ytContent.Type == "video" {
		var video YoutubeVideo
		json.Unmarshal(output, &video)

		entry.Url = video.Url
		entry.Title = video.Title
		entry.SourceUrl = getYoutubeAudioSource(video.Url)
	} else if ytContent.Type == "playlist" {
		var ytPlaylist YoutubePlaylist
		json.Unmarshal(output, &ytPlaylist)
		loadYoutubePlaylist(entry, ytPlaylist)
	} else {
		LogError("Unknown yt-dlp content type: %v", ytContent.Type)
	}
}
