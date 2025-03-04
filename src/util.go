package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	net_url "net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var client = http.Client{
	Timeout: time.Second * 20,
}

var userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; rv:115.0) Gecko/20100101 Firefox/115.0"

func constructTitleWhenMissing(entry *Entry) string {
	if entry.Title != "" {
		return entry.Title
	}

	parsed, err := net_url.Parse(entry.Url)
	if err != nil {
		return "Unknown Media"
	}

	base := path.Base(parsed.Path)
	title := strings.TrimSuffix(base, filepath.Ext(base))
	return title
}

func stripSuffix(url string) string {
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash == -1 {
		// this could be more robust
		return url
	}
	return url[:lastSlash]
}

func inferOrigin(referer string) string {
	if strings.HasSuffix(referer, "/") {
		length := len(referer)
		return referer[:length-1]
	}
	return referer
}

func stripLastSegment(url *net_url.URL) string {
	lastSlash := strings.LastIndex(url.Path, "/")
	if url.Scheme == "" && url.Host == "" {
		return url.Path[:lastSlash+1]
	}
	stripped := url.Scheme + "://" + url.Host + url.Path[:lastSlash+1]
	return stripped
}

func toString(num int) string {
	return strconv.Itoa(num)
}

func int64ToString(num int64) string {
	return strconv.FormatInt(num, 10)
}

func lastUrlSegment(url string) string {
	url = path.Base(url)
	questionMark := strings.Index(url, "?")
	if questionMark == -1 {
		return url
	}
	return url[:questionMark]
}

func hasScheme(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func getRootDomain(url *net_url.URL) string {
	return url.Scheme + "://" + url.Host
}

// This will download a chunk of a file within the specified range
func downloadFileChunk(url string, r *Range, referer string) ([]byte, error) {
	request, _ := http.NewRequest("GET", url, nil)
	if r == nil {
		return nil, nil
	}
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	request.Header.Set("Range", fmt.Sprintf("bytes=%v-%v", r.start, r.end))

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return nil, &DownloadError{Code: response.StatusCode, Message: "Failed to receive file chunk."}
	}
	defer response.Body.Close()

	buffer := make([]byte, r.length())
	bytesRead, err := io.ReadFull(response.Body, buffer)
	if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
		return buffer[:bytesRead], nil
	}
	if err != nil {
		return nil, &DownloadError{Code: response.StatusCode, Message: "Failed to read response body."}
	}
	return buffer, nil
}

// This will download a chunk of a file within the specified range
func openFileDownload(url string, from int64, referer string) (*http.Response, error) {
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	request.Header.Set("Range", fmt.Sprintf("bytes=%v-", from))

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return nil, &DownloadError{Code: response.StatusCode, Message: "Failed to open file download."}
	}
	return response, nil
}

func pullBytesFromResponse(response *http.Response, byteCount int) ([]byte, error) {
	buffer := make([]byte, byteCount)
	bytesRead, err := io.ReadFull(response.Body, buffer)
	if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
		return buffer[:bytesRead], nil
	}
	if err != nil {
		return nil, &DownloadError{Code: response.StatusCode, Message: "Failed to read response body."}
	}
	return buffer, nil
}

func downloadFile(url string, filename string, referer string) error {
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		errBody, err := io.ReadAll(response.Body)
		var bodyError = ""
		if err == nil {
			bodyError = string(errBody)
		}
		return &DownloadError{Code: response.StatusCode, Message: "Failed to download file. " + bodyError}
	}
	defer response.Body.Close()

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	if !fileExists(filename) {
		LogError("Sanity check failed for %v", filename)
		return err
	}

	_, err = io.Copy(out, response.Body)
	if err != nil {
		return err
	}
	return nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func getContentRange(url string, referer string) (int64, error) {
	// HEAD method returns metadata of a resource
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return -1, err
	}
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	// Send the request
	response, err := client.Do(request)
	if err != nil {
		return -1, err
	}
	defer response.Body.Close()

	// Get the Content-Range header
	contentRange := response.Header.Get("Content-Length")
	return strconv.ParseInt(contentRange, 10, 64)
}

func isTimeoutError(err error) bool {
	var urlErr *net_url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Timeout()
	}
	return false
}

// This will parse only the first range, given header and max (content-length) of the whole resource
func parseRangeHeader(headerValue string, max int64) (*Range, error) {
	bytes := strings.Index(headerValue, "bytes=")
	if bytes == -1 {
		return nil, errors.New("expected 'bytes=' inside the header")
	}
	headerValue = headerValue[bytes+6:]
	if len(headerValue) < 2 {
		return nil, errors.New("the header is too short")
	}
	dash := strings.Index(headerValue, "-")
	if dash == -1 {
		return nil, errors.New("expected '-' inside the header")
	}
	end := strings.Index(headerValue, ",")
	if end != -1 {
		headerValue = headerValue[:end]
	}

	leftSide := headerValue[:dash]
	rightSide := headerValue[dash+1:]
	if leftSide == "" && rightSide == "" {
		return nil, errors.New("expected at least one number around '-'")
	}

	if leftSide == "" {
		// this is actually <suffix-length> and it follows different rules
		suffixLength, err := strconv.ParseUint(rightSide, 10, 64)
		if err != nil {
			return nil, err
		}
		return newRange(max-int64(suffixLength), max-1), nil
	}

	if rightSide == "" {
		rangeSt, err := strconv.ParseUint(leftSide, 10, 64)
		if err != nil {
			return nil, err
		}
		return newRange(int64(rangeSt), max-1), nil
	}

	rangeSt, err := strconv.ParseUint(leftSide, 10, 64)
	if err != nil {
		return nil, err
	}
	rangeEnd, err := strconv.ParseUint(rightSide, 10, 64)
	if err != nil {
		return nil, err
	}
	return newRange(int64(rangeSt), int64(rangeEnd)), nil
}

func generateUniqueId() uint64 {
	src := rand.NewSource(time.Now().UnixNano())
	entropy := rand.New(src).Uint64() // Generate a random number between 0 and 99
	return entropy
}

type DownloadError struct {
	Code    int
	Message string
}

// Implements the error interface
func (e *DownloadError) Error() string {
	return fmt.Sprintf("NetworkError: Code=%d, Message=%s", e.Code, e.Message)
}

// Range - both start and end are inclusive
type Range struct {
	start int64
	end   int64
}

func newRange(start, end int64) *Range {
	if start < 0 || end < 0 || start > end {
		return nil
	}
	return &Range{start, end}
}

// This can only be merged if they overlap
func (r *Range) mergeWith(other *Range) Range {
	mergedStart := min(r.start, other.start)
	mergedEnd := max(r.end, other.end)
	return Range{start: mergedStart, end: mergedEnd}
}

func (r *Range) overlaps(other *Range) bool {
	return r.start <= other.end && other.start <= r.end
}

func (r *Range) encompasses(other *Range) bool {
	return r.start <= other.start && other.end <= r.end
}

func (r *Range) includes(value int64) bool {
	return r.start <= value && value <= r.end
}

func (r *Range) length() int64 {
	return r.end - r.start + 1
}

type Barrier struct {
	blocked atomic.Bool
	wg      sync.WaitGroup
	result  bool
}

func (barrier *Barrier) block() {
	if !barrier.blocked.Swap(true) {
		barrier.wg.Add(1)
	}
}

func (barrier *Barrier) wait() {
	barrier.wg.Wait()
}

// Might throw  "sync: negative WaitGroup counter" ?
func (barrier *Barrier) releaseWithResult(result bool) {
	barrier.result = result
	barrier.wg.Done()
}

func getMediaType(extension string) string {
	extension = strings.TrimSpace(extension)
	extension = strings.ToLower(extension)

	mediaType := "other"

	switch extension {
	case
		".webm", ".mkv", ".flv", ".vob", ".ogv", ".drc", ".mng", ".avi", ".mts", ".m2ts", ".ts",
		".mov", ".qt", ".wmv", ".yuv", ".rm", ".rmvb", ".viv", ".asf", ".amv", ".mp4", ".m4v",
		".mpg", ".mp2", ".mpeg", ".mpe", ".mpv", ".m2v", ".svi", ".3gp", ".3g2", ".mxf", ".roq",
		".nsv", ".f4v", ".f4p", ".f4a", ".f4b":
		mediaType = "video"

	case
		".aa", ".aac", ".aax", ".act", ".aiff", ".alac", ".amr", ".ape", ".au", ".awb", ".dss",
		".dvf", ".flac", ".gsm", ".iklax", ".ivs", ".m4a", ".m4b", ".m4p", ".mmf", ".movpkg",
		".mp3", ".mpc", ".msv", ".nmf", ".ogg", ".opus", ".ra", ".raw", ".rf64", ".sln", ".tta",
		".voc", ".vox", ".wav", ".wma", ".wv", ".8svx", ".cda":
		mediaType = "audio"

	case ".srt", ".vtt", ".ssa", ".ass":
		mediaType = "subs"

	case ".png", ".jpg", ".jpeg", ".webp", ".svg", ".dng", ".gif":
		mediaType = "image"
	}

	return mediaType
}

// Safe equivalent of path.Join
// Returns (string, bool):
//   - string - joined path or empty string if path is unsafe
//   - bool   - true if path is safe (wasn't traversed), false otherwise
func safeJoin(segments ...string) (string, bool) {
	for _, seg := range segments {
		for {
			dotsIndex := strings.Index(seg, "..")
			if dotsIndex == -1 {
				break
			}
			leftSafe := 0 < dotsIndex && !isSlash(seg[dotsIndex-1])
			afterDotsIndex := dotsIndex + 2
			rightSafe := afterDotsIndex < len(seg) && !isSlash(seg[afterDotsIndex])
			if leftSafe || rightSafe {
				seg = seg[dotsIndex+2:]
				continue
			}
			return "", false
		}
	}
	return filepath.Join(segments...), true
}

func isSlash(char uint8) bool {
	return char == '/' || char == '\\'
}

func stall(delay time.Duration, maxChecks int, checkFunc func() bool) {
	for range maxChecks {
		time.Sleep(delay)
		if checkFunc() {
			return
		}
	}
}

func respondBadRequest(writer http.ResponseWriter, format string, args ...any) {
	output := fmt.Sprintf(format, args...)
	LogWarnSkip(1, "%v", output)
	http.Error(writer, output, http.StatusBadRequest)
}

func respondInternalError(writer http.ResponseWriter, format string, args ...any) {
	output := fmt.Sprintf(format, args...)
	LogErrorSkip(1, "%v", output)
	http.Error(writer, output, http.StatusInternalServerError)
}
