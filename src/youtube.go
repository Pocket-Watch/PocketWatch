package main

import (
	"os/exec"
	"strings"
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
