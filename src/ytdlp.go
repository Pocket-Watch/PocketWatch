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

var YTDLP_ENABLED bool = true

type YoutubeVideo struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail"`
	OriginalUrl string `json:"original_url"`
	SourceUrl   string `json:"manifest_url"`
	AvailableAt int64  `json:"available_at"`
	Duration    int64  `json:"duration"`
	UploadDate  string `json:"upload_date"`
	Uploader    string `json:"uploader"`
	ArtistName  string `json:"artist_name"`
	AlbumName   string `json:"album_name"`
	ReleaseDate string `json:"release_date"`
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

type TwitchStream struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail"`
	OriginalUrl string `json:"original_url"`
	StreamUrl   string `json:"url"`
}

type InternalServerVideoFetch struct {
	Query string `json:"query"`
}

type InternalServerPlaylistFetch struct {
	Query string `json:"query"`
	Start uint   `json:"start"`
	End   uint   `json:"end"`
}

// Makes a request to the internal server that runs YtDlp.
// Returns:
// - A flag indicating whether the server request was successful. (Note that a non 200 response is still considered a success)
// - JSON data received from the server.
// - An error message when the server responded with a non 200 status code.
func postToInternalServer[T any](endpoint string, data any) (bool, T, error) {
	var output T

	request, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to marshal JSON request data for the internal server: %v", err)
		return false, output, err
	}

	url := "http://localhost:2345" + endpoint
	response, err := http.Post(url, "application/json", bytes.NewBuffer(request))
	if err != nil {
		// LogError("Request POST %v to the internal server failed: %v", endpoint, err)
		return false, output, err
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		LogError("Failed to unmarshal data from the internal server: %v", err)
		return false, output, nil
	}

	if response.StatusCode != http.StatusOK {
		var errorMessage string
		json.Unmarshal(responseData, &errorMessage)

		LogWarn("Internal server returned status code %v with message: %v", response.StatusCode, errorMessage)
		return true, output, fmt.Errorf("%v", errorMessage)
	}

	json.Unmarshal(responseData, &output)
	return true, output, err
}

func isTwitchEntry(entry Entry) bool {
	if !YTDLP_ENABLED {
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

func fetchTwitchStream(url string) (TwitchStream, error) {
	ok, stream, err := postToInternalServer[TwitchStream]("/twitch/fetch", url)
	if ok {
		return stream, err
	}

	LogWarn("Internal server twitch stream fetch failed. Falling back to yt-dlp command fetch.")
	ok, stream = fetchTwitchWithYtdlp(url)
	if ok {
		return stream, nil
	}

	return TwitchStream{}, nil
}

func loadTwitchEntry(entry *Entry) error {
	if !YTDLP_ENABLED {
		return nil
	}

	stream, err := fetchTwitchStream(entry.Url)
	if err != nil {
		return err
	}

	entry.Url = stream.OriginalUrl
	entry.Title = stream.Title
	entry.SourceUrl = stream.StreamUrl
	entry.Thumbnail = stream.Thumbnail

	return nil
}

func isYoutubeEntry(entry Entry, requested RequestEntry) bool {
	if !YTDLP_ENABLED {
		return false
	}

	// SearchVideo bool handles only YT searches
	if !isYoutubeUrl(entry.Url) && !requested.SearchVideo {
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

func waitForAvailability(availableAtUnix int64) {
	availableAt := time.Unix(availableAtUnix, 0)
	currentTime := time.Now()

	if availableAt.Before(currentTime) {
		return
	}

	waitTime := availableAt.Sub(currentTime)

	LogInfo("Waiting for YouTube source to become available in %vs", waitTime.Seconds())
	time.Sleep(waitTime)
}

func (server *Server) preloadYoutubeSourceOnNextEntry() {
	server.state.mutex.Lock()
	if len(server.state.playlist) == 0 {
		server.state.mutex.Unlock()
		return
	}

	nextEntry := server.state.playlist[0]
	server.state.mutex.Unlock()

	if !YTDLP_ENABLED || !isYoutubeUrl(nextEntry.Url) {
		return
	}

	if !isYoutubeSourceExpired(nextEntry.SourceUrl) {
		return
	}

	LogInfo("Preloading youtube source for an entry with an ID: %v", nextEntry.Id)
	video, err := fetchYoutubeVideo(nextEntry.Url)
	if err != nil {
		return
	}

	waitForAvailability(video.AvailableAt)

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	index := FindEntryIndex(server.state.playlist, nextEntry.Id)
	if index == -1 {
		return
	}

	server.state.playlist[index].Thumbnail = video.Thumbnail
	server.state.playlist[index].SourceUrl = video.SourceUrl
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
	if !YTDLP_ENABLED {
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
	playlist, err := fetchYoutubePlaylist(query, 1, size)
	if err != nil {
		return []Entry{}, err
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

func fetchVideoWithYtdlp(query string) (bool, YoutubeVideo) {
	args := []string{
		query, "--playlist-items", "1",
		"--extractor-args", "youtube:player_client=web_safari",
		"--print", "%(.{id,title,thumbnail,original_url,manifest_url,available_at,duration,uploader_date,uploader,artist,album,release_date})j",
	}

	command := exec.Command("yt-dlp", args...)
	output, err := command.Output()

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return false, YoutubeVideo{}
	}

	var video YoutubeVideo
	err = json.Unmarshal(output, &video)
	if err != nil {
		LogError("Failed to unmarshal yt-dlp output json: %v", err)
		return false, YoutubeVideo{}
	}

	return true, video
}

func fetchYoutubeVideo(query string) (YoutubeVideo, error) {
	data := InternalServerVideoFetch{Query: query}
	ok, video, err := postToInternalServer[YoutubeVideo]("/youtube/fetch", data)
	if ok {
		return video, err
	}

	LogWarn("Internal server video fetch failed. Falling back to yt-dlp command fetch.")
	ok, video = fetchVideoWithYtdlp(query)
	if ok {
		return video, nil
	}

	return YoutubeVideo{}, nil
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

func fetchYoutubePlaylist(query string, start uint, end uint) (YoutubePlaylist, error) {
	data := InternalServerPlaylistFetch{
		Query: query,
		Start: start,
		End:   end,
	}

	ok, playlist, err := postToInternalServer[YoutubePlaylist]("/youtube/playlist", data)
	if ok {
		return playlist, err
	}

	LogWarn("Internal server playlist fetch failed. Falling back to yt-dlp command fetch.")
	ok, playlist = fetchPlaylistWithYtdlp(query, start, end)
	if ok {
		return playlist, nil
	}

	return YoutubePlaylist{}, nil
}

// loadYoutubeEntry can only be called after the entry was approved for further processing
func loadYoutubeEntry(entry *Entry, requested RequestEntry) error {
	if !isYoutubeSourceExpired(entry.SourceUrl) {
		return nil
	}
	LogInfo("Determined entry titled '%v' of source url %v is expired", entry.Title, entry.SourceUrl)

	query := entry.Url
	if requested.SearchVideo {
		query = "ytsearch:" + query
		LogInfo("Searching for youtube video with query: %v.", entry.Url)
	} else {
		LogInfo("Loading youtube entry with url: %v.", entry.Url)
	}

	video, err := fetchYoutubeVideo(query)
	if err != nil {
		return err
	}

	waitForAvailability(video.AvailableAt)

	entry.Url = video.OriginalUrl
	entry.Title = video.Title
	entry.SourceUrl = video.SourceUrl
	entry.Thumbnail = video.Thumbnail

	return nil
}
