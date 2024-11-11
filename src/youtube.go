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

func loadYoutube(entry *Entry) {
	cmd := exec.Command("yt-dlp", "--get-url", "--extract-audio", "--no-playlist", entry.Url)
	output, err := cmd.Output()
	if err != nil {
		LogDebug("youtube load failed: %v", err)
		return
	}

	entry.Url = string(output)
	LogDebug("url is: %v", entry.Url)
}

type YtdlpEntry struct {
	Title       string `json:"title"`
	OriginalUrl string `json:"original_url"`
	SourceUrl   string `json:"url"`
}

type YoutubeEntry struct {
	Title string `json:"title"`
	Url   string `json:"url"`
}

type YoutubeSources struct {
	VideoSource string `json:"video_source"`
	AudioSource string `json:"audio_source"`
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
	ytSources := getYoutubeSources(nextEntry.Url)
	if ytSources != nil {
		nextEntry.SourceUrl = ytSources.AudioSource
	}

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

func getYoutubeSources(url string) *YoutubeSources {
	cmd := exec.Command("build/pocket-yt", url, "--get-sources")

	output, err := cmd.Output()
	if err != nil {
		LogError("Failed to get output sources from the pocket-yt command: %v", err)
		return nil
	}

	var ytSources YoutubeSources
	err = json.Unmarshal(output, &ytSources)
	if err != nil {
		LogError("Failed to deserialize sources from the pocket-yt command: %v", err)
		return nil
	}

	return &ytSources
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

	cmd := exec.Command("build/pocket-yt", entry.Url, "--dump-videos")

	output, err := cmd.Output()
	if err != nil {
		LogError("Failed to get output playlist from the pocket-yt command: %v", err)
		return
	}

	var ytEntries []YoutubeEntry
	err = json.Unmarshal(output, &ytEntries)
	if err != nil {
		LogError("Failed to deserialize array from the pocket-yt command: %v", err)
		return
	}

	if len(ytEntries) == 0 {
		LogError("Deserialize pocket-yt array for url '%v' is empty", entry.Url)
		return
	}

	firstYtEntry := ytEntries[0]
	ytEntries = ytEntries[1:]

	entry.Url = firstYtEntry.Url
	entry.Title = firstYtEntry.Title
	ytSources := getYoutubeSources(firstYtEntry.Url)
	if ytSources != nil {
		entry.SourceUrl = ytSources.AudioSource
	}

	userId := entry.UserId

	go func() {
		for _, ytEntry := range ytEntries {

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

			state.playlist = append(state.playlist, entry)
			state.mutex.Unlock()

			event := createPlaylistEvent("add", entry)
			writeEventToAllConnections(nil, "playlist", event)
		}
	}()
}
