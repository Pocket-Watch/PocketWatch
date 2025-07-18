package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	neturl "net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var YOUTUBE_ENABLED bool = true

type TwitchStream struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail"`
	OriginalUrl string `json:"original_url"`
	StreamUrl   string `json:"url"`
}

func isTwitch(entry Entry) bool {
	if !YOUTUBE_ENABLED {
		return false
	}

	if !isTwitchUrl(entry.Url) {
		return false
	}

	return true
}

func isTwitchUrl(url string) bool {
	if strings.HasPrefix(url, "twitch.tv") {
		return true
	}

	parsedUrl, err := neturl.Parse(url)
	if err != nil {
		return false
	}

	host := parsedUrl.Host
	return strings.HasSuffix(host, "twitch.tv")
}

func fetchTwitchWithInternalServer(url string) (bool, TwitchStream) {
	request, err := json.Marshal(url)
	if err != nil {
		LogError("Failed to marshal JSON request data for the internal server: %v", err)
		return false, TwitchStream{}
	}

	response, nil := http.Post("http://localhost:2345/twitch/fetch", "application/json", bytes.NewBuffer(request))
	if err != nil {
		LogError("Request POST to the internal server failed: %v", err)
		return false, TwitchStream{}
	}
	defer response.Body.Close()

	var stream TwitchStream
	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		LogError("Failed to unmarshal data from the internal server: %v", err)
		return false, TwitchStream{}
	}

	json.Unmarshal(responseData, &stream)
	return true, stream
}

func fetchTwitchWithYtdlp(url string) (bool, TwitchStream) {
	args := []string{url, "--print", "%(.{id,title,thumbnail,original_url,url})j"}

	command := exec.Command("yt-dlp", args...)
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return false, TwitchStream{}
	}

	var stream TwitchStream
	err = json.Unmarshal(output, &stream)
	if err != nil {
		LogError("Failed to unmarshal yt-dlp output json: %v", err)
		return false, TwitchStream{}
	}

	return true, stream
}

func fetchTwitchStream(url string) (bool, TwitchStream) {
	ok, stream := fetchTwitchWithInternalServer(url)
	if ok {
		return true, stream
	}

	LogWarn("Internal server twitch stream fetch failed. Falling back to yt-dlp command fetch.")
	ok, stream = fetchTwitchWithYtdlp(url)
	if ok {
		return true, stream
	}

	return false, TwitchStream{}
}

func loadTwitchEntry(entry *Entry, requested RequestEntry) error {
	if !YOUTUBE_ENABLED {
		return nil
	}

	ok, stream := fetchTwitchStream(entry.Url)
	if !ok {
		return fmt.Errorf("Failed to fetch twitch stream")
	}

	entry.Url = stream.OriginalUrl
	entry.Title = stream.Title
	entry.SourceUrl = stream.StreamUrl
	entry.Thumbnail = stream.Thumbnail

	return nil
}

func isYoutube(entry Entry, requested RequestEntry) bool {
	if !YOUTUBE_ENABLED {
		return false
	}

	if !isYoutubeUrl(entry.Url) && !requested.SearchVideo {
		return false
	}

	if !isYoutubeSourceExpired(entry.SourceUrl) {
		return false
	}

	return true
}

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

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.playlist, nextEntry.Id)
	if index == -1 {
		return
	}

	server.state.playlist[index].Thumbnail = video.Thumbnail
	server.state.playlist[index].SourceUrl = video.VideoUrl
}

func pickSmallestThumbnail(thumbnails []YoutubeThumbnail) string {
	bestThumbnail := ""
	var smallestSize uint64 = math.MaxUint64

	for _, thumbnail := range thumbnails {
		thumbnailSize := thumbnail.Height * thumbnail.Width
		if thumbnailSize < smallestSize {
			smallestSize = thumbnailSize
			bestThumbnail = thumbnail.Url
		}
	}

	return bestThumbnail
}

func (server *Server) loadYoutubePlaylist(requested RequestEntry, userId uint64) ([]Entry, error) {
	if !YOUTUBE_ENABLED {
		return []Entry{}, nil
	}

	if !isYoutubeUrl(requested.Url) {
		return []Entry{}, nil
	}

	query := requested.Url
	url, err := neturl.Parse(query)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return []Entry{}, fmt.Errorf("Failed to parse youtube source url: %v", err)
	}

	if !url.Query().Has("list") {
		videoId := url.Query().Get("v")

		query := url.Query()
		query.Add("list", "RD"+videoId)
		url.RawQuery = query.Encode()

		LogDebug("Url was not a playlist. Constructed youtube playlist url is now: %v", url)
	}

	size := requested.PlaylistMaxSize
	if size > 1000 {
		size = 1000
	} else if size <= 0 {
		size = 20
	}

	query = url.String()
	ok, playlist := fetchYoutubePlaylist(query, 1, size)
	if !ok {
		return []Entry{}, fmt.Errorf("Failed to fetch YouTube playlist.")
	}

	entries := make([]Entry, 0)

	for _, ytEntry := range playlist.Entries {
		entry := Entry{
			Url:       ytEntry.Url,
			Title:     ytEntry.Title,
			UserId:    userId,
			Thumbnail: pickSmallestThumbnail(ytEntry.Thumbnails),
		}

		entry = server.constructEntry(entry)
		entries = append(entries, entry)
	}

	return entries, nil
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

func loadYoutubeEntry(entry *Entry, requested RequestEntry) error {
	if !YOUTUBE_ENABLED {
		return nil
	}

	if !isYoutubeUrl(entry.Url) && !requested.SearchVideo {
		return nil
	}

	if !isYoutubeSourceExpired(entry.SourceUrl) {
		return nil
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
		return fmt.Errorf("Failed to fetch youtube video")
	}

	entry.Url = video.OriginalUrl
	entry.Title = video.Title
	entry.SourceUrl = video.VideoUrl
	entry.Thumbnail = video.Thumbnail

	return nil
}
