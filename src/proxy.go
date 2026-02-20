package main

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	net_url "net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func (server *Server) setupHlsProxy(url string, referer string) bool {
	server.state.setupLock.Lock()
	defer server.state.setupLock.Unlock()
	start := time.Now()
	_ = os.RemoveAll(CONTENT_PROXY)
	_ = os.MkdirAll(CONTENT_PROXY, os.ModePerm)
	var m3u *M3U
	var err error
	if strings.HasPrefix(url, CONTENT_MEDIA) {
		m3u, err = parseM3U(url)
		if err != nil {
			LogError("Failed to parse m3u8: %v", err)
			return false
		}
		m3u.url = url
	} else {
		m3u, err = downloadM3U(url, CONTENT_PROXY+ORIGINAL_M3U8, referer)
	}

	if err != nil {
		LogError("Failed to fetch m3u8: %v", err)
		return false
	}

	if m3u.isMasterPlaylist {
		if len(m3u.audioRenditions) > 0 {
			success, videoProxy, audioProxy := setupDualTrackProxy(m3u, referer)
			if success {
				server.state.isHls = true
				server.state.isLive = false
				server.state.proxy = videoProxy
				server.state.audioProxy = audioProxy
				duration := time.Since(start)
				LogDebug("Time taken to setup proxy: %v", duration)
			}
			return success
		}
		if m3u = prepareMediaPlaylistFromMasterPlaylist(m3u, referer, 0); m3u == nil {
			return false
		}
	}

	segmentCount := len(m3u.segments)
	duration := int(m3u.totalDuration())
	LogDebug("[Playlist info]")
	LogDebug("Type:     %v", m3u.getAttribute(EXT_X_PLAYLIST_TYPE))
	LogDebug("LIVE:     %v", m3u.isLive)
	LogDebug("Segments: %v", segmentCount)
	LogDebug("Duration: %vs", duration)

	if m3u.removeTrailingSegment(MIN_SEGMENT_LENGTH) {
		LogDebug("Removed trailing playlist segment.")
	}

	if segmentCount == 0 {
		LogWarn("No segments found")
		return false
	}
	if duration > MAX_PLAYLIST_DURATION_SECONDS {
		LogWarn("Playlist exceeds max duration")
		return false
	}

	m3u.prefixRelativeSegments()
	// Test if the first chunk is available and the source operates as intended (prevents broadcasting broken entries)
	if !validatePlaylist(m3u, referer) {
		LogError("Chunk/Track 0 was not available!")
		return false
	}

	// Check encryption map uri
	if err = setupMapUri(&m3u.segments[0], referer, MEDIA_INIT_SECTION); err != nil {
		return false
	}
	// Check decryption key
	if err = setupKeyUri(&m3u.segments[0], referer, MEDIA_DECRYPTION_KEY); err != nil {
		return false
	}

	server.state.isHls = true
	server.state.isLive = m3u.isLive

	var newProxy *HlsProxy
	if m3u.isLive {
		newProxy = setupLiveProxy(m3u.url, referer)
	} else {
		newProxy = setupVodProxy(m3u, CONTENT_PROXY+PROXY_M3U8, referer, VIDEO_PREFIX)
	}
	server.state.proxy = newProxy
	setupDuration := time.Since(start)
	LogDebug("Time taken to setup proxy: %v", setupDuration)
	return true
}

// setupDualTrackProxy for the time being will handle only 0-depth master playlists.
// returns (success, video proxy, audio proxy)
func setupDualTrackProxy(originalM3U *M3U, referer string) (bool, *HlsProxy, *HlsProxy) {
	originalM3U.prefixRelativeTracks()
	bestTrack := originalM3U.getTrackByVideoHeight(1080)
	if bestTrack == nil {
		return false, nil, nil
	}
	audioId := getParamValue("AUDIO", bestTrack.streamInfo)
	if audioId == "" {
		LogError("Best track's AUDIO param is empty, unable to match to audio.")
		return false, nil, nil
	}

	matchedAudio := false
	audioUrl := ""
	var audioRendition []Param
	for i := range originalM3U.audioRenditions {
		rendition := originalM3U.audioRenditions[i]
		groupId := getParamValue("GROUP-ID", rendition)
		if groupId != audioId {
			continue
		}
		matchedAudio = true
		audioUrl = getParamValue("URI", rendition)
		audioRendition = rendition

		audioDefault := getParamValue("DEFAULT", rendition)
		if audioDefault == "YES" {
			break
		}
		// YT hack: Look for original in audio track name
		audioName := getParamValue("NAME", rendition)
		if strings.Contains(audioName, "original") {
			break
		}
	}

	if !matchedAudio {
		LogError("No corresponding audio track found for audio id: %v", audioId)
		return false, nil, nil
	}

	LogInfo("Video URL: %v", bestTrack.url)
	LogInfo("Audio URL: %v", audioUrl)

	videoM3U, err := downloadM3U(bestTrack.url, CONTENT_PROXY+VIDEO_M3U8, referer)
	if err != nil {
		LogError("Failed to download m3u8 video track: %v", err.Error())
		return false, nil, nil
	}
	videoM3U.prefixRelativeSegments()
	if !validatePlaylist(videoM3U, referer) {
		LogError("Chunk 0 was not available in video m3u!")
		return false, nil, nil
	}
	if videoM3U.removeTrailingSegment(MIN_SEGMENT_LENGTH) {
		LogDebug("Removed trailing playlist segment in video track.")
	}

	audioM3U, err := downloadM3U(audioUrl, CONTENT_PROXY+AUDIO_M3U8, referer)
	if err != nil {
		LogError("Failed to download m3u8 audio track: %v", err.Error())
		return false, nil, nil
	}
	audioM3U.prefixRelativeSegments()
	if audioM3U.removeTrailingSegment(MIN_SEGMENT_LENGTH) {
		LogDebug("Removed trailing playlist segment in audio track.")
	}

	if videoM3U.isMasterPlaylist || audioM3U.isMasterPlaylist {
		LogWarn("Unimplemented: Either video or audio is a master playlist")
		return false, nil, nil
	}

	if len(videoM3U.segments) == 0 || len(audioM3U.segments) == 0 {
		LogWarn("One of the playlists contains 0 segments")
		return false, nil, nil
	}

	// Check video & audio encryption map uri
	if err = setupMapUri(&videoM3U.segments[0], referer, MEDIA_INIT_SECTION); err != nil {
		return false, nil, nil
	}
	// Don't fail on audio alone
	err = setupMapUri(&audioM3U.segments[0], referer, MEDIA_INIT_SECTION_AUDIO)

	// Check video decryption key
	if err = setupKeyUri(&videoM3U.segments[0], referer, MEDIA_DECRYPTION_KEY); err != nil {
		return false, nil, nil
	}
	// Check audio decryption key
	if err = setupKeyUri(&audioM3U.segments[0], referer, MEDIA_DECRYPTION_KEY_AUDIO); err != nil {
		return false, nil, nil
	}

	vidProxy := setupVodProxy(videoM3U, CONTENT_PROXY+VIDEO_M3U8, referer, VIDEO_PREFIX)
	audioProxy := setupVodProxy(audioM3U, CONTENT_PROXY+AUDIO_M3U8, referer, AUDIO_PREFIX)
	// Craft proxied master playlist for the client
	originalM3U.tracks = originalM3U.tracks[:0]
	originalM3U.audioRenditions = originalM3U.audioRenditions[:0]
	uriParam := getParam("URI", audioRendition)
	uriParam.value = AUDIO_M3U8
	bestTrack.url = VIDEO_M3U8
	originalM3U.tracks = append(originalM3U.tracks, *bestTrack)
	originalM3U.audioRenditions = append(originalM3U.audioRenditions, audioRendition)

	originalM3U.serialize(CONTENT_PROXY + PROXY_M3U8)
	return true, vidProxy, audioProxy
}

func ytAudioFilter(track *Track) bool {
	urlStruct, err := net_url.Parse(track.url)
	if err != nil {
		return false
	}
	return strings.Contains(urlStruct.Path, "acont=original")
}

func prepareMediaPlaylistFromMasterPlaylist(m3u *M3U, referer string, depth int) *M3U {
	if len(m3u.tracks) == 0 {
		LogError("Master playlist contains 0 tracks!")
		return nil
	}

	masterUrl, _ := net_url.Parse(m3u.url)
	var bestTrack *Track
	if masterUrl.Host == "manifest.googlevideo.com" {
		bestTrack = m3u.getBestTrack(ytAudioFilter)
	} else {
		bestTrack = m3u.getBestTrack(nil)
	}

	res := getParamValue("RESOLUTION", bestTrack.streamInfo)
	LogInfo("Best track selected from master playlist has resolution of %v", res)

	m3u.prefixRelativeTracks()
	bestUrl := bestTrack.url

	var err error = nil
	m3u, err = downloadM3U(bestUrl, CONTENT_PROXY+ORIGINAL_M3U8, referer)

	if isErrorStatus(err, 404) {
		LogError("Best url returned 404. %v", err.Error())
		return nil
	} else if err != nil {
		LogError("Failed to fetch track from master playlist: %v", err.Error())
		return nil
	}

	// Master playlists can point to other master playlists
	if m3u.isMasterPlaylist {
		if depth < MAX_PLAYLIST_DEPTH {
			return prepareMediaPlaylistFromMasterPlaylist(m3u, referer, depth+1)
		} else {
			LogError("Exceeded maximum playlist depth of %v. Failed to get media playlist.", MAX_PLAYLIST_DEPTH)
			return nil
		}
	}

	return m3u
}

func setupMapUri(segment *Segment, referer, fileName string) error {
	if segment.mapUri != "" {
		err := downloadFile(segment.mapUri, CONTENT_PROXY+fileName, &DownloadOptions{referer: referer, hasty: true})
		if err != nil {
			LogWarn("Failed to obtain map uri key from %v\n: %v", segment.mapUri, err.Error())
			return err
		}
		segment.mapUri = fileName
	}
	return nil
}

func setupKeyUri(segment *Segment, referer, fileName string) error {
	if !segment.hasKey {
		return nil
	}
	keyUri := segment.key.uri
	err := downloadFile(keyUri, CONTENT_PROXY+fileName, &DownloadOptions{referer: referer, hasty: true})
	if err != nil {
		LogWarn("Failed to obtain decryption key from %v\n: %v", keyUri, err.Error())
		return err
	}
	segment.key.uri = fileName
	return nil
}

func setupLiveProxy(liveUrl string, referer string) *HlsProxy {
	proxy := HlsProxy{}
	proxy.referer = referer
	proxy.liveUrl = liveUrl
	proxy.liveSegments.Clear()
	return &proxy
}

func setupVodProxy(m3u *M3U, osPath, referer, chunkPrefix string) *HlsProxy {
	proxy := HlsProxy{}
	segmentCount := len(m3u.segments)

	proxy.referer = referer
	proxy.chunkLocks = make([]sync.Mutex, segmentCount)
	proxy.fetchedChunks = make([]bool, segmentCount)
	proxy.originalChunks = make([]string, segmentCount)
	for i := range segmentCount {
		segment := &m3u.segments[i]
		proxy.originalChunks[i] = segment.url

		chunkName := chunkPrefix + toString(i)
		segment.url = chunkName
	}

	m3u.serialize(osPath)
	LogDebug("Prepared VOD proxy file.")
	return &proxy
}

var EXTM3U_BYTES = []byte("#EXTM3U")

// validatePlaylist will validate the availability of track 0 or segment 0
func validatePlaylist(m3u *M3U, referer string) bool {
	var checkedUrl string
	if m3u.isMasterPlaylist {
		if len(m3u.tracks) == 0 {
			return false
		}
		checkedUrl = m3u.tracks[0].url
		// Perhaps check audio renditions instead?
	} else {
		if len(m3u.segments) == 0 {
			return false
		}
		checkedUrl = m3u.segments[0].url
	}

	if !isAbsolute(checkedUrl) {
		LogError("M3U url %v is not absolute, make sure the URLs are prefixed before validation.", checkedUrl)
		return false
	}
	success, buffer, _ := testGetResponse(checkedUrl, referer)
	if !success {
		return false
	}
	if m3u.isMasterPlaylist {
		return bytes.HasPrefix(buffer.Bytes(), EXTM3U_BYTES)
	}
	return true
}

func (server *Server) setupFileProxy(proxy *FileProxy, url, referer, baseFilename string) bool {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		LogWarn("The provided URL is invalid: %v", err)
		return false
	}
	if parsedUrl.Host == "" {
		LogInfo("The provided URL has no host: %v", url)
		return false
	}

	size, err := getContentLength(url, referer)
	if err != nil {
		LogWarn("Couldn't retrieve resource's Content-Length: %v", err)
		return false
	}
	if size > PROXY_FILE_SIZE_LIMIT {
		LogWarn("The file exceeds the specified limit of 4 GBs.")
		return false
	}
	server.state.setupLock.Lock()
	defer server.state.setupLock.Unlock()
	server.state.isHls = false
	server.state.isLive = false

	proxy.referer = referer
	proxy.url = url
	proxy.contentLength = size

	// Detect content type by pulling at most the first 512 bytes
	response, err := openFileDownload(proxy.url, 0, proxy.referer)
	if err != nil {
		LogWarn("Failed open file download at 0 due to %v", err)
		return false
	}
	buffer := make([]byte, 512)
	_, err = io.ReadFull(response.Body, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		LogWarn("Failed to read leading bytes due to %v", err)
		return false
	}
	proxy.contentType = http.DetectContentType(buffer)

	extensionWithDot := path.Ext(parsedUrl.Path)
	proxy.filename = baseFilename + extensionWithDot
	proxyFile, err := os.OpenFile(CONTENT_PROXY+proxy.filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		LogError("Failed to open proxy file for writing: %v", err)
		return false
	}

	proxy.file = proxyFile
	proxy.diskRanges = make([]Range, 0)

	if size > TRAILING_PULL_SIZE {
		// Preload the end of the file
		offset := size - TRAILING_PULL_SIZE
		if !proxy.loadBytes(offset, TRAILING_PULL_SIZE) {
			return false
		}
	}

	proxy.downloader = &GenericDownloader{
		mutex:    sync.Mutex{},
		download: nil,
		offset:   0,
		preload:  0,
		speed:    NewDefaultSpeedTest(),
		sleeper:  NewSleeper(),
		destroy:  make(chan bool),
	}
	proxy.downloader.mutex.Lock()
	proxy.replaceDownload(0)
	proxy.downloader.mutex.Unlock()
	go server.startDownloadLoop(proxy)
	LogInfo("Successfully setup proxy for file of size %v MB", formatMegabytes(size, 2))
	return true
}

var loopIdSeed atomic.Int64

func (server *Server) startDownloadLoop(proxy *FileProxy) {
	lastTimestamp := server.getCurrentTimestamp()

	downloader := proxy.downloader
	loopId := loopIdSeed.Add(1)
	for {
		select {
		case <-downloader.destroy:
			downloader.closed = true
			LogInfo("Terminating download loop#%v", loopId)
			// cleanup and exit loop
			downloader.closeDownload()
			return
		default:
			// If a download enters disk range:
			// a. Determine if the range is worth terminating the currently active download
			// b. Cover state where there's no active download

			downloader.mutex.Lock()
			offset := downloader.offset
			count := int64(HEURISTIC_BITRATE_MB_S)
			if offset+count >= proxy.contentLength {
				count = proxy.contentLength - offset
			}

			currentRange := newRange(offset, offset+count-1)
			if count == 0 || proxy.isRangeAvailableOnDisk(currentRange) {
				// The downloader should keep looping even on disk because clients are waiting in cycles
				downloader.closeDownload()
				downloader.sleeper.WakeAll()
			}
			// If no active download
			if downloader.download == nil {
				// Best solution: wait on another download indefinitely
				downloader.mutex.Unlock()
				time.Sleep(time.Second)
				continue
			}

			// Bitrate/length could be obtained from ffprobe
			currentTimestamp := server.getCurrentTimestamp()
			consumedPreload := (currentTimestamp - lastTimestamp) * HEURISTIC_BITRATE_MB_S
			if consumedPreload > 0 {
				downloader.preload -= int64(consumedPreload)
				downloader.preload = max(0, downloader.preload)
			}
			lastTimestamp = currentTimestamp

			// It could monitor download speed and detect disk range
			if downloader.preload > MAX_PRELOAD_SIZE {
				LogInfo("[Download loop#%v] exceeding max preload", loopId)
				downloader.mutex.Unlock()
				time.Sleep(time.Second)
				continue
			}

			nextOffset, err := proxy.pullAndStoreBytes(downloader.download, downloader.offset, count)
			if err != nil {
				LogWarn("[Download loop#%v] error while pulling from source %v - replacing download", loopId, err)
				// Why does it timeout?
				proxy.replaceDownload(downloader.offset)
				downloader.mutex.Unlock()
				time.Sleep(time.Second)
				continue
			}
			downloader.preload += count
			downloader.offset = nextOffset
			offsetMB, preloadMB := formatMegabytes(downloader.offset, 2), formatMegabytes(downloader.preload, 2)
			LogDebug("[Download loop#%v] Offset=%vMB Preload=%vMB", loopId, offsetMB, preloadMB)
			downloader.sleeper.WakeAll()
			downloader.mutex.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (downloader *GenericDownloader) closeDownload() {
	if downloader.download != nil {
		_ = downloader.download.Body.Close()
		downloader.download = nil
	}
}

func (proxy *FileProxy) getPrefetchStart(start int64) int64 {
	if start == 0 {
		return 0
	}
	preStart := max(0, start-PRE_OFFSET)
	preRange := newRange(preStart, start-1)
	if proxy.isRangeAvailableOnDisk(preRange) {
		return start
	}
	return preStart
}

func (proxy *FileProxy) isRangeAvailableOnDisk(r *Range) bool {
	proxy.rangeMutex.Lock()
	defer proxy.rangeMutex.Unlock()
	for _, diskRange := range proxy.diskRanges {
		if diskRange.encompasses(r) {
			return true
		}
		if r.end < diskRange.start {
			// No point in searching further
			break
		}
	}
	return false
}

func (proxy *FileProxy) getRangeOverlapWithDisk(r *Range) (Overlap, Range) {
	proxy.rangeMutex.Lock()
	defer proxy.rangeMutex.Unlock()
	for _, diskRange := range proxy.diskRanges {
		overlap := r.getOverlap(&diskRange)
		if overlap != NONE {
			return overlap, diskRange
		}
		if r.end < diskRange.start {
			// No point in searching further
			break
		}
	}
	return NONE, NO_RANGE
}

// replaceDownload replaces the current download synchronously
func (proxy *FileProxy) replaceDownload(from int64) {
	downloader := proxy.downloader
	if downloader == nil {
		LogError("Unable to replace download because downloader is nil")
		return
	}

	if downloader.download != nil {
		downloader.download.Body.Close()
	}
	newDownload, err := openFileDownload(proxy.url, from, proxy.referer)
	if err != nil {
		LogWarn("Failed to reopen response %v", err)
		return
	}
	if from < downloader.offset {
		downloader.preload = 0
	}
	downloader.offset = from
	downloader.download = newDownload
	downloader.sleeper.WakeAll()
	LogInfo("Download was replaced, new offset = %vMB", formatMegabytes(from, 2))
}

func (proxy *FileProxy) logDiskRanges() {
	view := strings.Builder{}
	proxy.rangeMutex.Lock()
	for _, diskRange := range proxy.diskRanges {
		view.WriteString(diskRange.StringMB() + ", ")
	}
	LogDebug("Disk ranges: %v", view.String())
	proxy.rangeMutex.Unlock()
}

func (proxy *FileProxy) serveRangeFromDisk(writer http.ResponseWriter, request *http.Request, servedRange *Range, totalWritten *int64) bool {
	length := servedRange.length()
	proxy.fileMutex.RLock()
	rangeBytes, err := readAtOffset(proxy.file, servedRange.start, int(length))
	if err != nil {
		proxy.fileMutex.RUnlock()
		LogInfo("Unable to serve bytes at %v: %v", servedRange.start, err)
		return false
	}
	proxy.fileMutex.RUnlock()

	written, err := io.CopyN(writer, bytes.NewReader(rangeBytes), length)
	*totalWritten += written
	if err != nil {
		megabytes := formatMegabytes(*totalWritten, 2)
		LogInfo("Connection %v terminated having downloaded %v MB", getIp(request), megabytes)
		return false
	}
	return true
}

// pullAndStoreBytes pulls the specified number of bytes from the response and returns the next readable offset
func (proxy *FileProxy) pullAndStoreBytes(response *http.Response, offset, count int64) (int64, error) {
	if count < 0 {
		return offset, errors.New("the count of bytes to pull is negative")
	}
	if count == 0 {
		return offset, nil
	}
	chunkBytes, err := pullBytesFromResponse(response, int(count))
	if err != nil {
		LogWarn("Failed to pull bytes from response at %v count=%v due to %v", offset, count, err)
		return offset, err
	}
	proxy.fileMutex.Lock()
	_, err = writeAtOffset(proxy.file, offset, chunkBytes)
	if err != nil {
		proxy.fileMutex.Unlock()
		LogError("An error occurred while writing to destination: %v", err)
		return offset, err
	}
	proxy.fileMutex.Unlock()
	until := offset + count - 1
	nextRange := newRange(offset, until)
	proxy.rangeMutex.Lock()
	proxy.diskRanges = incorporateRange(nextRange, proxy.diskRanges)
	proxy.rangeMutex.Unlock()
	return nextRange.end + 1, nil
}

func (proxy *FileProxy) destruct() {
	if proxy == nil {
		return
	}
	if proxy.downloader != nil && !proxy.downloader.closed {
		proxy.downloader.destroy <- true
	}
	if proxy.file != nil {
		proxy.fileMutex.Lock()
		_ = proxy.file.Close()
		proxy.fileMutex.Unlock()
	}
}

func (server *Server) serveHlsVod(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")

	switch chunk {
	case PROXY_M3U8, VIDEO_M3U8, AUDIO_M3U8:
		LogDebug("Serving %v", chunk)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	case MEDIA_INIT_SECTION, MEDIA_INIT_SECTION_AUDIO, MEDIA_DECRYPTION_KEY, MEDIA_DECRYPTION_KEY_AUDIO:
		LogDebug("Serving %v", chunk)
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	}

	if !server.isAuthorized(writer, request) {
		return
	}

	if len(chunk) > MAX_CHUNK_NAME_LENGTH || len(chunk) < len(VIDEO_PREFIX) {
		http.Error(writer, "Not found", 404)
		return
	}

	chunkId, err := strconv.Atoi(chunk[3:])
	if err != nil {
		http.Error(writer, "Chunk ID is not a number", 404)
		return
	}

	var proxy *HlsProxy
	server.state.setupLock.Lock()
	if strings.HasPrefix(chunk, VIDEO_PREFIX) {
		proxy = server.state.proxy
	} else if strings.HasPrefix(chunk, AUDIO_PREFIX) {
		proxy = server.state.audioProxy
	}
	server.state.setupLock.Unlock()
	if proxy != nil {
		serveHlsChunk(writer, request, proxy, chunk, chunkId)
	}
}

var chunkLogsite = Logsite{}

func serveHlsChunk(writer http.ResponseWriter, request *http.Request, proxy *HlsProxy, chunk string, chunkId int) {
	if chunkId < 0 || chunkId >= len(proxy.fetchedChunks) {
		http.Error(writer, "Chunk ID out of range", 404)
		return
	}

	mutex := &proxy.chunkLocks[chunkId]
	mutex.Lock()

	if proxy.fetchedChunks[chunkId] {
		mutex.Unlock()
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	}

	destinationUrl := proxy.originalChunks[chunkId]
	options := &DownloadOptions{
		referer:   proxy.referer,
		hasty:     false,
		bodyLimit: MAX_CHUNK_SIZE,
	}
	fetchErr := downloadFile(destinationUrl, CONTENT_PROXY+chunk, options)
	if fetchErr != nil {
		mutex.Unlock()
		if chunkLogsite.atMostEvery(time.Second) {
			LogError("Failed to fetch chunk #%v due to %v from %v", chunkId, fetchErr, destinationUrl)
		}

		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		} else {
			if downloadCode := getDownloadErrorCode(fetchErr); downloadCode != -1 {
				code = downloadCode
			}
		}
		http.Error(writer, "Failed to fetch vod chunk", code)
		return
	}
	proxy.fetchedChunks[chunkId] = true
	mutex.Unlock()

	http.ServeFile(writer, request, CONTENT_PROXY+chunk)
}

func (server *Server) serveFileProxyNaive(writer http.ResponseWriter, request *http.Request, filename string) {
	proxy := &server.state.fileProxy
	if filename != proxy.filename {
		http.Error(writer, "Filename for proxy wasn't found", 404)
		return
	}

	byteRange, ok := ensureRangeHeader(writer, request, proxy.contentLength)
	if !ok {
		return
	}

	writer.Header().Set("Accept-Ranges", "bytes")
	writer.Header().Set("Content-Length", int64ToString(byteRange.length()))
	writer.Header().Set("Content-Range", byteRange.toContentRange(proxy.contentLength))

	response, err := openFileDownload(proxy.url, byteRange.start, proxy.referer)
	if err != nil {
		http.Error(writer, "Unable to open file download", 500)
		return
	}

	if request.Method == "HEAD" {
		writer.WriteHeader(http.StatusOK)
		return
	}

	writer.WriteHeader(http.StatusPartialContent)
	var totalWritten int64
	for {
		chunkBytes, err := pullBytesFromResponse(response, GENERIC_CHUNK_SIZE)
		if err != nil {
			LogError("An error occurred while pulling from source: %v", err)
			return
		}
		written, err := io.CopyN(writer, bytes.NewReader(chunkBytes), GENERIC_CHUNK_SIZE)
		totalWritten += written
		if err != nil {
			megabytes := formatMegabytes(totalWritten, 2)
			LogInfo("Connection %v terminated having downloaded %v MB", getIp(request), megabytes)
			return
		}
	}
}

func (server *Server) serveFileProxy(writer http.ResponseWriter, request *http.Request, filename string) {
	found := false
	proxy := &server.state.fileProxy
	if proxy.filename == filename {
		found = true
	}
	audioProxy := &server.state.audioFileProxy
	if !found && audioProxy.filename == filename {
		found = true
		proxy = audioProxy
	}

	if !found {
		http.Error(writer, "File proxy for this filename wasn't found", 404)
		return
	}

	requestedRange, ok := ensureRangeHeader(writer, request, proxy.contentLength)
	if !ok {
		return
	}

	LogDebug("Connection %v requested proxied file at range %v", getIp(request), requestedRange.StringMB())

	writer.Header().Set("Accept-Ranges", "bytes")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("Content-Length", int64ToString(requestedRange.length()))
	writer.Header().Set("Content-Range", requestedRange.toContentRange(proxy.contentLength))
	writer.Header().Set("Content-Type", proxy.contentType)

	if request.Method == "HEAD" {
		writer.WriteHeader(http.StatusOK)
		return
	}
	writer.WriteHeader(http.StatusPartialContent)

	proxy.logDiskRanges()

	var totalWritten int64
	currentRange := requestedRange
	currentRange.end = currentRange.start + GENERIC_CHUNK_SIZE - 1

	downloader := proxy.downloader
	lastTimestamp := server.getCurrentTimestamp()
	preload := int64(0)
	for {
		if currentRange.start >= proxy.contentLength {
			LogDebug("Closing connection %v, having been served %vMB", getIp(request), formatMegabytes(totalWritten, 2))
			break
		}
		if currentRange.end >= proxy.contentLength {
			currentRange.end = proxy.contentLength - 1
		}

		// Bitrate/length could be obtained from ffprobe
		currentTimestamp := server.getCurrentTimestamp()
		consumedPreload := (currentTimestamp - lastTimestamp) * HEURISTIC_BITRATE_MB_S
		if consumedPreload > 0 {
			preload -= int64(math.Abs(consumedPreload))
			preload = max(0, preload)
		}
		lastTimestamp = currentTimestamp

		clientDelayed := false
		if preload > MAX_PRELOAD_SIZE {
			clientDelayed = true
			time.Sleep(time.Second)
		}

		if isRequestDone(request) {
			LogDebug("Connection %v closed", getIp(request))
			return
		}

		if clientDelayed {
			//LogDebug("Connection %v hit preload at %v = %v", getIp(request), &currentRange, currentRange.StringMB())
			continue
		}

		// Handle multiple possible states:
		// sync.Mutex is not FIFO so clients are not guaranteed to be served (use sync.Cond?)
		downloader.mutex.Lock()
		// 1. Next chunk is fully available from disk
		available := proxy.isRangeAvailableOnDisk(&currentRange)
		if available {
			downloader.mutex.Unlock()
			if proxy.serveRangeFromDisk(writer, request, &currentRange, &totalWritten) {
				currentRange.shift(GENERIC_CHUNK_SIZE)
				preload += currentRange.length()
				continue
			}
			return
		}

		// 2. Next chunk intersects a range available from disk
		overlap, diskRange := proxy.getRangeOverlapWithDisk(&currentRange)
		if overlap == LEFT {
			downloader.mutex.Unlock()
			// Serve whatever is available
			availableRange := newRange(currentRange.start, diskRange.end)

			if proxy.serveRangeFromDisk(writer, request, availableRange, &totalWritten) {
				LogDebug("Served LEFT overlapped range=%v", availableRange.StringMB())
				length := availableRange.length()
				currentRange.shift(length)
				preload += length
				continue
			}
			return
		}

		// 3. Next chunk can be supplied from an existing in-progress download

		// The forward offset causing clients to reopen their active connection is unknown therefore upon changing
		// the current download it's necessary to terminate connections with other clients
		isDownloadActive := downloader.download != nil
		offset := downloader.offset
		forwardOffset := offset + 2*HEURISTIC_BITRATE_MB_S
		if isDownloadActive && (offset <= currentRange.start && currentRange.start < forwardOffset) {
			downloader.mutex.Unlock()
			// Always wait one cycle
			if !downloader.sleeper.Sleep(2 * time.Second) {
				LogWarn("Connection %v is delayed because the next chunk wasn't available at %v = %v", getIp(request), &currentRange, currentRange.StringMB())
				// If connection was to be terminated here with no bytes served the browser would decide it's EOF
			}
			// Serve client from disk (at step 1)
			continue
		}

		// 4. Next chunk requires new connection to be opened (serve at step 3)
		start := proxy.getPrefetchStart(currentRange.start)
		proxy.replaceDownload(start)
		downloader.mutex.Unlock()
	}
}

func (server *Server) serveHlsLive(writer http.ResponseWriter, request *http.Request, chunk string) {
	writer.Header().Add("Cache-Control", "no-cache")
	server.state.setupLock.Lock()
	proxy := server.state.proxy
	server.state.setupLock.Unlock()

	segmentMap := &proxy.liveSegments
	lastRefresh := &proxy.lastRefresh

	now := time.Now()
	if chunk == PROXY_M3U8 {
		cleanupSegmentMap(segmentMap)
		refreshedAgo := now.Sub(*lastRefresh)
		// Optimized to refresh at most once every 1.5 seconds
		if refreshedAgo.Seconds() < 1.5 {
			LogDebug("Serving unmodified %v", PROXY_M3U8)
			writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
			http.ServeFile(writer, request, CONTENT_PROXY+PROXY_M3U8)
			return
		}

		liveM3U, err := downloadM3U(proxy.liveUrl, CONTENT_PROXY+ORIGINAL_M3U8, proxy.referer)
		var downloadErr *DownloadError
		if errors.As(err, &downloadErr) {
			LogError("Download error of the live url [%v] %v", proxy.liveUrl, err.Error())
			http.Error(writer, downloadErr.Message, downloadErr.Code)
			return
		} else if err != nil {
			LogError("Failed to fetch live url: %v", err.Error())
			http.Error(writer, err.Error(), 500)
			return
		}

		if len(liveM3U.segments) == 0 {
			respondInternalError(writer, "No live segments received!")
			return
		}

		id := 0
		if mediaSequence := liveM3U.getAttribute(EXT_X_MEDIA_SEQUENCE); mediaSequence != "" {
			if sequenceId, err := parseInt(mediaSequence); err == nil {
				id = sequenceId
			}
		}

		liveM3U.prefixRelativeSegments()

		segmentCount := len(liveM3U.segments)
		for i := range segmentCount {
			segment := &liveM3U.segments[i]

			realUrl := segment.url
			segName := LIVE_PREFIX + toString(id)

			if _, exists := segmentMap.Load(segName); !exists {
				liveSegment := LiveSegment{realUrl: realUrl, realMapUri: segment.mapUri, created: time.Now()}
				segmentMap.Store(segName, &liveSegment)
				if segment.mapUri != "" {
					segment.mapUri = MIS_PREFIX + toString(id)
				}
			}

			segment.url = segName
			id++
		}

		liveM3U.serialize(CONTENT_PROXY + PROXY_M3U8)
		writer.Header().Add("content-type", M3U8_CONTENT_TYPE)
		http.ServeFile(writer, request, CONTENT_PROXY+PROXY_M3U8)
		return
	}

	if !server.isAuthorized(writer, request) {
		return
	}

	if len(chunk) > MAX_CHUNK_NAME_LENGTH {
		http.Error(writer, "Not found", 404)
		return
	}

	if strings.HasPrefix(chunk, MIS_PREFIX) {
		fetchOrServeMediaInitSection(writer, request, chunk, segmentMap, proxy.referer)
		return
	}

	maybeChunk, found := segmentMap.Load(chunk)
	if !found {
		http.Error(writer, "Not found", 404)
		return
	}

	fetchedChunk := maybeChunk.(*LiveSegment)
	mutex := &fetchedChunk.mutex
	mutex.Lock()
	if fetchedChunk.obtainedUrl {
		mutex.Unlock()
		http.ServeFile(writer, request, CONTENT_PROXY+chunk)
		return
	}

	options := &DownloadOptions{
		referer:   proxy.referer,
		hasty:     false,
		bodyLimit: MAX_CHUNK_SIZE,
	}
	fetchErr := downloadFile(fetchedChunk.realUrl, CONTENT_PROXY+chunk, options)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch live chunk %v", fetchErr)

		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		}

		http.Error(writer, "Failed to fetch live chunk", code)
		return
	}

	fetchedChunk.obtainedUrl = true
	mutex.Unlock()
	http.ServeFile(writer, request, CONTENT_PROXY+chunk)
}

func fetchOrServeMediaInitSection(writer http.ResponseWriter, request *http.Request, init string, segmentMap *sync.Map, referer string) {
	_, after, ok := strings.Cut(init, "-")
	if !ok || after == "" {
		http.Error(writer, "Bad media section", http.StatusBadRequest)
		return
	}
	id, err := parseInt64(after)
	if err != nil {
		http.Error(writer, "Bad media section", http.StatusBadRequest)
		return
	}
	segName := LIVE_PREFIX + int64ToString(id)
	maybeChunk, found := segmentMap.Load(segName)
	if !found {
		http.Error(writer, "Corresponding live segment not found", http.StatusNotFound)
		return
	}
	liveSegment := maybeChunk.(*LiveSegment)
	mutex := &liveSegment.mutex
	mutex.Lock()
	initKeyPath := CONTENT_PROXY + init
	if liveSegment.obtainedMapUri {
		mutex.Unlock()
		http.ServeFile(writer, request, initKeyPath)
		return
	}
	options := &DownloadOptions{
		referer:   referer,
		hasty:     true,
		bodyLimit: MAX_CHUNK_SIZE,
	}
	fetchErr := downloadFile(liveSegment.realMapUri, initKeyPath, options)
	if fetchErr != nil {
		mutex.Unlock()
		LogError("Failed to fetch media init section %v", fetchErr)
		code := 500
		if isTimeoutError(fetchErr) {
			code = 504
		}
		http.Error(writer, "Failed to fetch media init section", code)
		return
	}
	liveSegment.obtainedMapUri = true
	mutex.Unlock()

	http.ServeFile(writer, request, initKeyPath)
}

func cleanupSegmentMap(segmentMap *sync.Map) {
	// Cleanup map - remove old entries to avoid memory leaks
	var keysToRemove []string
	now := time.Now()
	size := 0
	segmentMap.Range(func(key, value any) bool {
		fSegment := value.(*LiveSegment)
		age := now.Sub(fSegment.created)
		if age.Seconds() > 30 {
			keysToRemove = append(keysToRemove, key.(string))
		}
		size++
		// true continues iteration
		return true
	})

	// Remove the collected keys
	for _, key := range keysToRemove {
		segmentMap.Delete(key)
	}
}
