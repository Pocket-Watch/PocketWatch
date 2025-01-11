package main

import (
	"bufio"
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

func getYoutubeAudioSource(url string) string {
	cmd := exec.Command("yt-dlp", "--get-url", "--extract-audio", "--no-playlist", url)
	output, err := cmd.Output()
	if err != nil {
		LogDebug("youtube load failed: %v", err)
		return ""
	}

	source := string(output)
	return strings.TrimSpace(source)
}

type YoutubeThumbnail struct {
	Url    string `json:"url"`
	Height uint64 `json:"height"`
	Width  uint64 `json:"width"`
}

type YoutubeEntry struct {
	Url        string             `json:"original_url"`
	Title      string             `json:"title"`
	Thumbnails []YoutubeThumbnail `json:"thumbnails"`
}

type YoutubeFormat struct {
	// "ext": "mhtml",
	// "audio_ext": "none",
	// "video_ext": "none",
	//
	// "protocol": "m3u8_native",
}

// type YoutubeEntry struct {
// 	Title string `json:"title"`
// 	Url   string `json:"url"`
// }

// type YoutubeSources struct {
// 	VideoSource string `json:"video_source"`
// 	AudioSource string `json:"audio_source"`
// }
//
// func getYoutubeSources(url string) *YoutubeSources {
// 	cmd := exec.Command("build/pocket-yt", url, "--get-sources")
//
// 	output, err := cmd.Output()
// 	if err != nil {
// 		LogError("Failed to get output sources from the pocket-yt command: %v", err)
// 		return nil
// 	}
//
// 	var ytSources YoutubeSources
// 	err = json.Unmarshal(output, &ytSources)
// 	if err != nil {
// 		LogError("Failed to deserialize sources from the pocket-yt command: %v", err)
// 		return nil
// 	}
//
// 	return &ytSources
// }

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

	cmd := exec.Command("yt-dlp", "--flat-playlist", "--dump-json", "--playlist-end", "200", entry.Url)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		LogError("Failed to get stdout pipe from the yt-dlp command: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		LogError("Failed to start the yt-dlp command: %v", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	bufferSize := 1024 * 1024
	scanner.Buffer(make([]byte, bufferSize), bufferSize)

	ytEntries := make([]YoutubeEntry, 0)
	for scanner.Scan() {
		var ytEntry YoutubeEntry

		bytes := scanner.Bytes()
		err := json.Unmarshal(bytes, &ytEntry)
		if err != nil {
			LogError("Failed to deserialize array from the yt-dlp command: %v", err)
			return
		}

		ytEntries = append(ytEntries, ytEntry)
	}

	if len(ytEntries) == 0 {
		LogError("Deserialize yt-dlp array for url '%v' is empty", entry.Url)
		return
	}

	firstYtEntry := ytEntries[0]
	ytEntries = ytEntries[1:]

	entry.Url = firstYtEntry.Url
	entry.Title = firstYtEntry.Title
	audioSource := getYoutubeAudioSource(firstYtEntry.Url)
	entry.SourceUrl = audioSource

	userId := entry.UserId

	go func() {
		entries := make([]Entry, 0)

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

			entries = append(entries, entry)
			state.mutex.Unlock()
		}

		state.mutex.Lock()
		state.playlist = append(state.playlist, entries...)
		state.mutex.Unlock()

		event := createPlaylistEvent("addmany", entries)
		writeEventToAllConnections(nil, "playlist", event)
	}()
}
