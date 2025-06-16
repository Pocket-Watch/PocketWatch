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

type YoutubeFormat struct {
	ManifestUrl string `json:"manifest_url"`
}

type YoutubeVideo2 struct {
	Id          string          `json:"id"`
	Title       string          `json:"title"`
	Thumbnail   string          `json:"thumbnail"`
	OriginalUrl string          `json:"original_url"`
	Formats     []YoutubeFormat `json:"requested_formats"`
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
	ok, playlist := fetchYoutubePlaylist(query, 2, size)
	if !ok {
		return
	}

	entries := make([]Entry, 0)

	for _, ytEntry := range playlist.Entries {
		entry := Entry{
			Id:         server.state.entryId.Add(1),
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
	}

	server.state.mutex.Lock()
	if addToTop {
		server.state.playlist = append(entries, server.state.playlist...)
	} else {
		server.state.playlist = append(server.state.playlist, entries...)
	}

	DatabasePlaylistAddMany(server.db, entries)
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

type InternalServerVideoFetch struct {
	Query string `json:"query"`
}

type InternalServerPlaylistFetch struct {
	Query string `json:"query"`
	Start uint   `json:"start"`
	End   uint   `json:"end"`
}

func fetchVideoWithInternalServer(query string) (bool, YoutubeVideo) {
	data := InternalServerVideoFetch{
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
	args := []string{
		query, "--playlist-items", "1",
		"--extractor-args", "youtube:player_client=ios",
		"--format", "(bv*[vcodec~='^((he|a)vc|h26[45])']+ba)",
		"--print", "%(.{id,title,thumbnail,original_url,requested_formats})j",
	}

	command := exec.Command("yt-dlp", args...)
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return false, YoutubeVideo{}
	}

	var video2 YoutubeVideo2
	err = json.Unmarshal(output, &video2)
	if err != nil {
		LogError("Failed to unmarshal yt-dlp output json: %v", err)
		return false, YoutubeVideo{}
	}

	if len(video2.Formats) < 2 {
		return false, YoutubeVideo{}
	}

	video := YoutubeVideo{
		Id:          video2.Id,
		Title:       video2.Title,
		Thumbnail:   video2.Thumbnail,
		OriginalUrl: video2.OriginalUrl,
		AudioUrl:    video2.Formats[1].ManifestUrl,
		VideoUrl:    video2.Formats[0].ManifestUrl,
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

func fetchPlaylistWithInternalServer(query string, start uint, end uint) (bool, YoutubePlaylist) {
	data := InternalServerPlaylistFetch{
		Query: query,
		Start: start,
		End:   end,
	}

	request, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to marshal JSON request data for the internal server: %v", err)
		return false, YoutubePlaylist{}
	}

	response, nil := http.Post("http://localhost:2345/youtube/playlist", "application/json", bytes.NewBuffer(request))
	if err != nil {
		LogError("Request POST to the internal server failed: %v", err)
		return false, YoutubePlaylist{}
	}
	defer response.Body.Close()

	var playlist YoutubePlaylist
	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		LogError("Failed to unmarshal playlist data from the internal server: %v", err)
		return false, YoutubePlaylist{}
	}

	json.Unmarshal(responseData, &playlist)
	return true, playlist
}

func fetchPlaylistWithYtdlp(query string, start uint, end uint) (bool, YoutubePlaylist) {
	startArg := fmt.Sprint(start)
	endArg := fmt.Sprint(end)
	command := exec.Command("yt-dlp", query, "--flat-playlist", "--playlist-start", startArg, "--playlist-end", endArg, "--dump-single-json")
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return false, YoutubePlaylist{}
	}

	var playlist YoutubePlaylist
	err = json.Unmarshal(output, &playlist)
	if err != nil {
		LogError("Failed to unmarshal yt-dlp output json: %v", err)
		return false, YoutubePlaylist{}
	}

	if len(playlist.Entries) == 0 {
		LogError("Deserialized yt-dlp array for url '%v' is empty", query)
		return false, YoutubePlaylist{}
	}

	return true, playlist
}

func fetchYoutubePlaylist(query string, start uint, end uint) (bool, YoutubePlaylist) {
	ok, playlist := fetchPlaylistWithInternalServer(query, start, end)
	if ok {
		return true, playlist
	}

	LogWarn("Internal server playlist fetch failed. Falling back to yt-dlp command fetch.")
	ok, playlist = fetchPlaylistWithYtdlp(query, start, end)
	if ok {
		return true, playlist
	}

	return false, YoutubePlaylist{}
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
	entry.SourceUrl = video.VideoUrl
	entry.Thumbnail = video.Thumbnail

	if requested.SearchVideo {
		query = video.OriginalUrl
	}

	if requested.IsPlaylist {
		go server.loadYoutubePlaylist(query, video.Id, entry.UserId, requested.PlaylistMaxSize, requested.AddToTop)
	}
}
