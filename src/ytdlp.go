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

type Ytdlp struct {
	enabled        bool
	enableServer   bool
	serverPath     string
	enableFallback bool
	fallbackPath   string
}

var ytdlp Ytdlp

func SetupYtdlp(config YtdlpConfig) {
	ytdlp.enabled = config.Enabled
	ytdlp.enableServer = config.EnableServer
	ytdlp.serverPath = fmt.Sprintf("http://%v:%v", config.ServerAddress, config.ServerPort)
	ytdlp.enableFallback = config.EnableFallback

	if config.FallbackPath == "" {
		config.FallbackPath = "yt-dlp"
	}
	ytdlp.fallbackPath = config.FallbackPath
}

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
	ArtistName  string `json:"artist"`
	AlbumName   string `json:"album"`
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

type YtdlpServerVideoFetch struct {
	Query string `json:"query"`
}

type YtdlpServerPlaylistFetch struct {
	Query string `json:"query"`
	Start uint   `json:"start"`
	End   uint   `json:"end"`
}

type TwitterFormat struct {
	ManfestUrl string `json:"manifest_url"`
}

type TwitterSource struct {
	Id          string          `json:"id"`
	Title       string          `json:"title"`
	Thumbnail   string          `json:"thumbnail"`
	OriginalUrl string          `json:"original_url"`
	Formats     []TwitterFormat `json:"formats"`
	Duration    float64         `json:"duration"`
}

var ytdlpHosts []string = []string{"youtube.com", "youtu.be", "twitter.com", "x.com", "twitch.tv"}

func isYtdlpSource(url string) bool {
	if !ytdlp.enabled {
		return false
	}

	parsedUrl, err := neturl.Parse(url)
	if err != nil {
		return false
	}

	host := parsedUrl.Host
	for _, ytdlpHost := range ytdlpHosts {
		if strings.HasSuffix(host, ytdlpHost) {
			return true
		}
	}

	return false
}

func isYoutube(url string) bool {
	if !ytdlp.enabled {
		return false
	}

	parsedUrl, err := neturl.Parse(url)
	if err != nil {
		return false
	}

	host := parsedUrl.Host
	return host == "youtube.com" || host == "youtu.be"
}

// Makes a request to the internal server that runs YtDlp.
// Returns:
// - A flag indicating whether the server request was successful. (Note that a non 200 response is still considered a success)
// - JSON data received from the server.
// - An error message when the server responded with a non 200 status code.
func postToYtdlpServer[T any](endpoint string, data any) (bool, T, error) {
	var output T

	if !ytdlp.enableServer {
		return false, output, nil
	}

	request, err := json.Marshal(data)
	if err != nil {
		LogError("Failed to marshal JSON request data for the Ytdlp server: %v", err)
		return false, output, err
	}

	url := ytdlp.serverPath + endpoint
	response, err := http.Post(url, "application/json", bytes.NewBuffer(request))
	if err != nil {
		LogError("YtDlp server POST request to %v failed: %v", endpoint, err)
		return false, output, err
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		LogError("Failed to unmarshal data from the Ytdlp server: %v", err)
		return false, output, nil
	}

	if response.StatusCode != http.StatusOK {
		var errorMessage string
		json.Unmarshal(responseData, &errorMessage)

		LogWarn("Ytdlp server returned status code %v with message: %v", response.StatusCode, errorMessage)
		return true, output, fmt.Errorf("%v", errorMessage)
	}

	json.Unmarshal(responseData, &output)
	return true, output, err
}

func getToInternalServer[T any](endpoint string, pairs []Param, port int) (bool, T, error) {
	var output T

	url := "http://localhost:" + toString(port) + endpoint
	u, _ := neturl.Parse(url)
	params := neturl.Values{}
	for _, pair := range pairs {
		params.Add(pair.key, pair.value)
	}

	u.RawQuery = params.Encode()
	response, err := http.Get(u.String())
	if err != nil {
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

func fetchWithYtdlp[T any](url string, ytdlpFlags []string) (T, error) {
	var data T
	if !ytdlp.enableFallback {
		return data, nil
	}

	args := []string{url}
	args = append(args, ytdlpFlags...)

	command := exec.Command(ytdlp.fallbackPath, args...)
	output, err := command.Output()
	LogDebug("Output is = %v", string(output))

	if err != nil {
		LogError("Failed to get output from the yt-dlp command: %v", err)
		return data, err
	}

	err = json.Unmarshal(output, &data)
	if err != nil {
		LogError("Failed to unmarshal yt-dlp output json: %v", err)
		return data, err
	}

	return data, nil
}

func fetchTwitchStream(url string) (TwitchStream, error) {
	ok, stream, err := postToYtdlpServer[TwitchStream]("/twitch/fetch", url)
	if ok {
		return stream, err
	}

	flags := []string{"--print", "%(.{id,title,thumbnail,original_url,url})j"}
	return fetchWithYtdlp[TwitchStream](url, flags)
}

func loadTwitchEntry(entry *Entry) error {
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

func fetchTwitterSource(url string) (TwitterSource, error) {
	ok, source, err := postToYtdlpServer[TwitterSource]("/twitter/fetch", url)
	if ok {
		return source, err
	}

	flags := []string{"--print", "%(.{id,title,thumbnail,original_url,formats,duration})j"}
	return fetchWithYtdlp[TwitterSource](url, flags)
}

func loadTwitterEntry(entry *Entry) error {
	source, err := fetchTwitterSource(entry.Url)
	if err != nil {
		return err
	}

	entry.Url = source.OriginalUrl
	entry.Title = source.Title
	entry.Thumbnail = source.Thumbnail
	for _, format := range source.Formats {
		if format.ManfestUrl != "" {
			entry.SourceUrl = format.ManfestUrl
			break
		}
	}

	return nil
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

	if !isYoutube(nextEntry.Url) {
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

// func loadYoutubePlaylist(requested RequestEntry, userId uint64) ([]Entry, error) {
func loadYoutubePlaylist(url string, skipCount uint, maxSize uint, userId uint64) ([]Entry, error) {
	query := url
	parsedUrl, err := neturl.Parse(query)
	if err != nil {
		LogError("Failed to parse youtube source url: %v", err)
		return []Entry{}, fmt.Errorf("Failed to parse youtube source url: %v", err)
	}

	if !parsedUrl.Query().Has("list") {
		videoId := parsedUrl.Query().Get("v")

		query := parsedUrl.Query()
		query.Add("list", "RD"+videoId)
		parsedUrl.RawQuery = query.Encode()

		LogDebug("Url was not a playlist. Constructed youtube playlist url is now: %v", parsedUrl)
	}

	size := maxSize
	if size > 1000 {
		size = 1000
	} else if size <= 0 {
		size = 20
	}

	query = parsedUrl.String()
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
			CreatedAt: time.Now(),
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func fetchYoutubeVideo(query string) (YoutubeVideo, error) {
	ok, video, err := getToInternalServer[YoutubeVideo]("/fetch", []Param{{"url", query}}, 9090)
	if ok {
		return video, err
	}

	data := YtdlpServerVideoFetch{Query: query}
	ok, video, err = postToYtdlpServer[YoutubeVideo]("/youtube/fetch", data)
	if ok {
		return video, err
	}

	flags := []string{
		"--playlist-items", "1",
		"--extractor-args", "youtube:player_client=web_safari",
		"--print", "%(.{id,title,thumbnail,original_url,manifest_url,available_at,duration,uploader_date,uploader,artist,album,release_date})j",
	}

	return fetchWithYtdlp[YoutubeVideo](query, flags)
}

func searchYoutubeVideo(query string) (YoutubeVideo, error) {
	data := YtdlpServerVideoFetch{Query: query}
	ok, video, err := postToYtdlpServer[YoutubeVideo]("/youtube/search", data)
	if ok {
		return video, err
	}

	flags := []string{
		"--playlist-items", "1",
		"--extractor-args", "youtube:player_client=web_safari",
		"--print", "%(.{id,title,thumbnail,original_url,manifest_url,available_at,duration,uploader_date,uploader,artist,album,release_date})j",
	}

	return fetchWithYtdlp[YoutubeVideo]("ytsearch:"+query, flags)
}

func fetchYoutubePlaylist(query string, start uint, end uint) (YoutubePlaylist, error) {
	data := YtdlpServerPlaylistFetch{
		Query: query,
		Start: start,
		End:   end,
	}

	ok, playlist, err := postToYtdlpServer[YoutubePlaylist]("/youtube/playlist", data)
	if ok {
		return playlist, err
	}

	flags := []string{"--flat-playlist", "--playlist-start", fmt.Sprint(start + 1), "--playlist-end", fmt.Sprint(end), "--dump-single-json"}
	return fetchWithYtdlp[YoutubePlaylist](query, flags)
}

// loadYoutubeEntry can only be called after the entry was approved for further processing
func loadYoutubeEntry(entry *Entry, search bool) error {
	if !isYoutubeSourceExpired(entry.SourceUrl) {
		return nil
	}

	LogInfo("Determined entry titled '%v' of source url %v is expired", entry.Title, entry.SourceUrl)

	var video YoutubeVideo
	var err error

	query := entry.Url
	if search {
		LogInfo("Searching for youtube video with query: %v.", entry.Url)
		video, err = searchYoutubeVideo(query)
	} else {
		LogInfo("Loading youtube entry with url: %v.", entry.Url)
		video, err = fetchYoutubeVideo(query)
	}

	if err != nil {
		return err
	}

	waitForAvailability(video.AvailableAt)

	entry.Url = video.OriginalUrl
	entry.Title = video.Title
	entry.SourceUrl = video.SourceUrl
	entry.Thumbnail = video.Thumbnail

	metadata := Metadata{
		TrackNumber: 0,
		AlbumName:   video.AlbumName,
		ArtistName:  video.ArtistName,
		ReleaseDate: video.ReleaseDate,
		Duration:    video.Duration,
	}

	entry.Metadata = metadata

	return nil
}

func (state *ServerState) fetchLyrics(title string, meta Metadata) (Subtitle, error) {
	subtitle := Subtitle{}

	artist := meta.ArtistName
	parsedArtist, trackName := parseSongTitle(title)
	if artist == "" {
		artist = parsedArtist
	}
	LogInfo("Searching lyrics with artist=%v album=%v trackName=%v duration=%v",
		artist, meta.AlbumName, trackName, meta.Duration)

	lyrics, err := getLyrics(LrcQuery{
		ArtistName: artist,
		AlbumName:  meta.AlbumName,
		TrackName:  trackName,
		Duration:   int(meta.Duration),
	})

	if err != nil {
		LogWarn("Lyrics fetch failed: %v.", err)
		return subtitle, err
	}
	LogDebug("Fetched lyrics track name: %v", lyrics.TrackName)

	if lyrics.SyncedLyrics == "" {
		LogInfo("No synced lyrics available for %v - %v.", lyrics.ArtistName, lyrics.TrackName)
		return subtitle, fmt.Errorf("No synced lyrics available")
	}

	cues, err := parseLRC(lyrics.SyncedLyrics)
	if err != nil {
		LogWarn("Lyrics parse failed: %v.", err)
		return subtitle, err
	}

	subtitle = createSubtitle(title, ".vtt")

	err = serializeToVTT(cues, subtitle.Url)
	if err != nil {
		LogWarn("Failed to convert lyrics: %v.", err)
		return subtitle, err
	}

	return subtitle, nil
}
