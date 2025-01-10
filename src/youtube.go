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

type YoutubeThumbnail struct {
	Url    string `json:"url"`
	Height uint64 `json:"height"`
	Width  uint64 `json:"width"`
}

type YoutubeEntry struct {
	Url        string             `json:"url"`
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

	cmd := exec.Command("yt-dlp", "--flat-playlist", "--dump-json", entry.Url)
	stdout, _ := cmd.StdoutPipe()

	done := make(chan struct{})

	scanner := bufio.NewScanner(stdout)

	ytEntries := make([]YoutubeEntry, 0)
	go func() {
		for scanner.Scan() {
			var entry YoutubeEntry

			bytes := scanner.Bytes()
			err := json.Unmarshal(bytes, &entry)
			if err != nil {
				LogError("Failed to deserialize array from the yt-dlp command: %v", err)
				break
			}

			ytEntries = append(ytEntries, entry)

		}

		done <- struct{}{}
	}()

	cmd.Start()
	<-done

	cmd.Wait()

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
