package main

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var YOUTUBE_ENABLED bool = true

func isYoutubeUrl(url string) bool {
	if strings.HasPrefix(url, "https://youtube.com/") {
		return true
	}

	if strings.HasPrefix(url, "https://www.youtube.com/") {
		return true
	}

	if strings.HasPrefix(url, "https://youtu.be/") {
		return true
	}

	if strings.HasPrefix(url, "https://www.youtu.be/") {
		return true
	}

	if strings.HasPrefix(url, "https://music.youtube.com/") {
		return true
	}

	return false
}

func isYoutubeSourceExpired(sourceUrl string) bool {
	if sourceUrl == "" {
		return true
	}

	parsedUrl, err := url.Parse(sourceUrl)
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
	nextEntry.SourceUrl = getYoutubeAudioSource(nextEntry.Url)

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

	cmd := exec.Command("yt-dlp", "--extract-audio", "--dump-json", entry.Url)

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
	// NOTE(kihau):
	//     Allocation of 1 MB and JSON parsing of the ginormous yt-dlp output is (to put it lightly) suboptimal.
	//     This will be improved when a custom YouTube downloader gets implemented.
	bufferSize := 1024 * 1024
	scanner.Buffer(make([]byte, bufferSize), bufferSize)

	if scanner.Scan() {
		var ytdlpEntry YtdlpEntry
		err = json.Unmarshal(scanner.Bytes(), &ytdlpEntry)
		if err != nil {
			LogError("Failed to parse yt-dlp json entry: %v", err)
		} else {
			entry.Url = ytdlpEntry.OriginalUrl
			entry.SourceUrl = ytdlpEntry.SourceUrl
			entry.Title = ytdlpEntry.Title
		}
	}

	LogDebug("%v", entry)

	userId := entry.UserId

	go func() {
		for scanner.Scan() {
			var ytdlpEntry YtdlpEntry
			err = json.Unmarshal(scanner.Bytes(), &ytdlpEntry)
			if err != nil {
				LogError("Failed to parse yt-dlp json entry: %v", err)
				continue
			}

			state.mutex.Lock()
			state.entryId += 1

			entry := Entry{
				Id:         state.entryId,
				Url:        ytdlpEntry.OriginalUrl,
				Title:      ytdlpEntry.Title,
				UserId:     userId,
				UseProxy:   false,
				RefererUrl: "",
				SourceUrl:  ytdlpEntry.SourceUrl,
				Created:    time.Now(),
			}

			state.playlist = append(state.playlist, entry)
			state.mutex.Unlock()

			event := createPlaylistEvent("add", entry)
			writeEventToAllConnections(nil, "playlist", event)
		}

		if err := scanner.Err(); err != nil {
			LogError("Scanning yt-dlp output failed: %v", err)
		}

		if err := cmd.Wait(); err != nil {
			LogError("The yt-dlp command exited with an error: %v", err)
		}
	}()
}

func getYoutubeAudioSource(youtubeUrl string) string {
	cmd := exec.Command("yt-dlp", "--get-url", "--extract-audio", "--no-playlist", youtubeUrl)
	output, err := cmd.Output()
	if err != nil {
		LogWarn("Failed to extract youtube url: %v", err)
		return ""
	}

	return string(output)
}
