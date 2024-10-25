package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

var YOUTUBE_ENABLED bool = true

func isYoutube(url string) bool {
	if !YOUTUBE_ENABLED {
		return false
	}

	if strings.HasPrefix(url, "https://youtube.com/") {
		return true
	}

	if strings.HasPrefix(url, "https://youtu.be/") {
		return true
	}

	if strings.HasPrefix(url, "https://music.youtube.com/") {
		return true
	}

	return false
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

func loadYoutubeEntries(entry *Entry) {
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

	// TODO(kihau): Preload second entry?

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
				SourceUrl:  "",
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

func getYoutubeAudioUrl(youtubeUrl string) string {
	cmd := exec.Command("yt-dlp", "--get-url", "--extract-audio", "--no-playlist", youtubeUrl)
	output, err := cmd.Output()
	if err != nil {
		LogWarn("Failed to extract youtube url: %v", err)
		return ""
	}

	return string(output)
}
