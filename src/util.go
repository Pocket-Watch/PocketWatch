package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"mime"
	"net"
	"net/http"
	net_url "net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var defaultClient = http.Client{
	Timeout: time.Second * 35,
}

var hastyClient = http.Client{
	Timeout: time.Second * 7,
}

var userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; rv:115.0) Gecko/20100101 Firefox/115.0"

func FindEntryIndex(entries []Entry, targetId uint64) int {
	compareFunc := func(entry Entry) bool {
		return entry.Id == targetId
	}

	index := slices.IndexFunc(entries, compareFunc)
	return index
}

func GeneratePrettyTableItem(header string, data string) string {
	padding := utf8.RuneCountInString(header)
	builder := strings.Builder{}

	// Top part
	builder.WriteString("┌")
	for range padding + 2 {
		builder.WriteString("─")
	}
	builder.WriteString("┐\n")

	// Middle part
	str := fmt.Sprintf("│ %*s │\n", padding, header)
	builder.WriteString(str)

	// Bottom part
	builder.WriteString("└")
	for range padding + 2 {
		builder.WriteString("─")
	}
	builder.WriteString("┘\n")

	builder.WriteString(data)
	return builder.String()
}

func GeneratePrettyTable(headers []string, data []string) string {
	if len(headers) == 0 {
		return ""
	}

	columnCount := len(headers)
	rowCount := len(data) / columnCount

	if columnCount == 1 && rowCount == 1 {
		return GeneratePrettyTableItem(headers[0], data[0])
	}

	// Trim string to fit in the console buffer (assuming that the column count is not too crazy)

	for i := range data {
		data[i] = strings.ReplaceAll(data[i], "\n", "")

		runes := []rune(data[i])
		if len(runes) > 80 {
			data[i] = string(runes[:80])
		}
	}

	// Find max size of each column.

	paddings := make([]int, columnCount)
	for i, header := range headers {
		paddings[i] = consoleTextWidth(header)
	}

	for row := 0; row < rowCount; row += 1 {
		for column := 0; column < columnCount; column += 1 {
			value := data[row*columnCount+column]
			width := consoleTextWidth(value)
			paddings[column] = max(paddings[column], width)
		}
	}

	// Generate separators.

	buildTop := strings.Builder{}
	buildMid := strings.Builder{}
	buildBot := strings.Builder{}

	buildTop.WriteString("┌")
	buildMid.WriteString("├")
	buildBot.WriteString("└")

	for i, pad := range paddings {
		for range pad + 2 {
			buildTop.WriteString("─")
			buildMid.WriteString("─")
			buildBot.WriteString("─")
		}

		if i == len(paddings)-1 {
			break
		}

		buildTop.WriteString("┬")
		buildMid.WriteString("┼")
		buildBot.WriteString("┴")
	}

	buildTop.WriteString("┐\n")
	buildMid.WriteString("┤\n")
	buildBot.WriteString("┘\n")

	separatorTop := buildTop.String()
	separatorMid := buildMid.String()
	separatorBot := buildBot.String()

	// Pretty print the table.

	table := strings.Builder{}

	table.WriteString(separatorTop)
	for i, name := range headers {
		pad := max(paddings[i]-consoleTextWidth(name), 0)
		str := fmt.Sprintf("│ %s%s ", strings.Repeat(" ", pad), name)
		table.WriteString(str)
	}
	table.WriteString("│\n")

	if rowCount == 0 {
		table.WriteString(separatorBot)
		return table.String()
	}

	table.WriteString(separatorMid)

	for row := 0; row < rowCount; row += 1 {
		for column := 0; column < columnCount; column += 1 {
			value := data[row*columnCount+column]
			pad := max(paddings[column]-consoleTextWidth(value), 0)
			str := fmt.Sprintf("│ %s%s ", strings.Repeat(" ", pad), value)
			table.WriteString(str)
		}

		table.WriteString("│\n")
	}
	table.WriteString(separatorBot)

	return table.String()
}

func consoleTextWidth(s string) int {
	width := 0
	isAnsi := false

	for _, r := range s {
		if !isAnsi && r == 0x1b {
			isAnsi = true
			continue
		}

		if isAnsi {
			if r == 'm' {
				isAnsi = false
			}
			continue
		}

		if isEmoji(r) {
			width += 2
		} else {
			width++
		}
	}

	return width
}

func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F300 && r <= 0x1F5FF: // Misc symbols & pictographs
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport & map
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental symbols
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF: // Extended-A
		return true
	}
	return false
}

func GeneratePrettyVerticalTable(tableName string, headers []string, values []string) string {
	if len(headers) == 0 {
		return ""
	}

	if len(headers) != len(values) {
		return ""
	}

	maxHeader := 0
	for _, header := range headers {
		headerWidth := consoleTextWidth(header)
		maxHeader = max(maxHeader, headerWidth)
	}

	maxValue := 0
	for _, value := range values {
		valueWidth := consoleTextWidth(value)
		maxValue = max(maxValue, valueWidth)
	}

	tableNameWidth := consoleTextWidth(tableName)
	paddingTop := maxHeader + maxValue + 3
	if tableNameWidth > paddingTop {
		maxValue += tableNameWidth - paddingTop
		paddingTop = tableNameWidth
	}

	buildTop := strings.Builder{}
	buildMid := strings.Builder{}
	buildBot := strings.Builder{}

	buildTop.WriteString("┌")
	buildMid.WriteString("├")
	buildBot.WriteString("└")

	for range maxHeader + 2 {
		buildTop.WriteString("─")
		buildMid.WriteString("─")
		buildBot.WriteString("─")
	}

	buildTop.WriteString("─")
	buildMid.WriteString("┬")
	buildBot.WriteString("┴")

	for range maxValue + 2 {
		buildTop.WriteString("─")
		buildMid.WriteString("─")
		buildBot.WriteString("─")
	}

	buildTop.WriteString("┐\n")
	buildMid.WriteString("┤\n")
	buildBot.WriteString("┘\n")

	separatorTop := buildTop.String()
	separatorMid := buildMid.String()
	separatorBot := buildBot.String()

	table := strings.Builder{}

	table.WriteString(separatorTop)

	pad := max(paddingTop-tableNameWidth, 0)
	str := fmt.Sprintf("│ %s%s │\n", tableName, strings.Repeat(" ", pad))
	table.WriteString(str)

	table.WriteString(separatorMid)

	for i := range headers {
		header := headers[i]
		pad1 := max(maxHeader-consoleTextWidth(header), 0)
		str1 := fmt.Sprintf("│ %s%s ", strings.Repeat(" ", pad1), header)
		table.WriteString(str1)

		value := values[i]
		pad2 := max(maxValue-consoleTextWidth(value), 0)
		str2 := fmt.Sprintf("│ %s%s │\n", value, strings.Repeat(" ", pad2))
		table.WriteString(str2)
	}

	table.WriteString(separatorBot)
	return table.String()
}

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
	title = cleanupResourceName(title)
	return title
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

func stripLastSegmentStr(url string) *string {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		return nil
	}
	reducedUrl := stripLastSegment(parsedUrl)
	return &reducedUrl
}

func stripParams(path string) string {
	before, _, ok := strings.Cut(path, "?")
	if !ok {
		return path
	}
	return before
}

func getBaseNoParams(urlPath string) string {
	endpoint := stripParams(urlPath)
	return path.Base(endpoint)
}

func stripPathPrefix(path string, parts ...string) string {
	segments := strings.Split(path, "/")

	stIndex := 0
	partIndex := 0
	for i, segment := range segments {
		if partIndex >= len(parts) {
			break
		}
		if segment == "" {
			stIndex = i + 1
			continue
		}
		if segment == parts[partIndex] {
			stIndex = i + 1
			partIndex++
			continue
		}
		break
	}
	return strings.Join(segments[stIndex:], "/")
}

func cleanupResourceName(oldName string) string {
	newName := strings.ReplaceAll(oldName, ".", " ")
	newName = strings.ReplaceAll(newName, "-", " ")
	newName = strings.ReplaceAll(newName, "_", " ")
	return newName
}

func toString(num int) string {
	return strconv.Itoa(num)
}

func int64ToString(num int64) string {
	return strconv.FormatInt(num, 10)
}

func lastUrlSegment(url string) string {
	url = path.Base(url)
	before, _, ok := strings.Cut(url, "?")
	if !ok {
		return url
	}
	return before
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

	response, err := defaultClient.Do(request)
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
		return nil, &DownloadError{Code: response.StatusCode, Message: err.Error()}
	}
	return buffer, nil
}

func readAtOffset(f *os.File, offset int64, count int) ([]byte, error) {
	buffer := make([]byte, count)
	_, err := f.ReadAt(buffer, offset)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || err == io.EOF {
			return buffer, err
		}
		return nil, err
	}
	return buffer, nil
}

func writeAtOffset(f *os.File, offset int64, data []byte) (int, error) {
	n, err := f.WriteAt(data, offset)
	if err != nil && !errors.Is(err, io.ErrShortWrite) {
		return n, err
	}
	return n, nil
}

func isAbsolute(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

// Maybe there's some specification for proxied URLs
func getParamUrl(url *net_url.URL) *net_url.URL {
	for _, values := range url.Query() {
		if len(values) == 0 {
			continue
		}
		paramUrl := values[0]
		if !isAbsolute(paramUrl) {
			continue
		}
		// Ensure that param url is a valid url
		paramUrlStruct, err := net_url.Parse(paramUrl)
		if err == nil {
			return paramUrlStruct
		}
	}
	return nil
}

func openFileDownload(url string, from int64, referer string) (*http.Response, error) {
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	request.Header.Set("Range", fmt.Sprintf("bytes=%v-", from))

	response, err := hastyClient.Do(request)
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
		return nil, &DownloadError{Code: response.StatusCode, Message: err.Error()}
	}
	return buffer, nil
}

type ContentType = string

// This function tests the server's intent to serve and return data (at most 512 bytes)
// Returns true if test status code is successful, false if unsuccessful or body couldn't be read
func testGetResponse(url string, referer string) (bool, *bytes.Buffer, ContentType) {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		LogDebug("Tested %v", parsedUrl)
		return false, nil, ""
	}
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	response, err := defaultClient.Do(request)
	if err != nil {
		return false, nil, ""
	}
	defer response.Body.Close()

	contentType := response.Header.Get("Content-Type")
	length := min(int(response.ContentLength), 512)
	buffer := new(bytes.Buffer)
	limitedResponse := io.LimitReader(response.Body, int64(length))
	_, readErr := io.Copy(buffer, limitedResponse)
	if readErr != nil && readErr != io.EOF {
		return false, nil, contentType
	}

	success := response.StatusCode >= 200 && response.StatusCode < 300
	return success, buffer, contentType
}

type DownloadOptions struct {
	method    string
	bodyLimit int64
	referer   string
	hasty     bool
}

const DEFAULT_BODY_LIMIT = 32 * GB

func NewDefaultDownloadOptions() DownloadOptions {
	return DownloadOptions{
		method:    "GET",
		bodyLimit: DEFAULT_BODY_LIMIT,
	}
}

func downloadFile(url string, path string, options *DownloadOptions) error {
	if options == nil {
		defaults := NewDefaultDownloadOptions()
		options = &defaults
	}
	if options.bodyLimit <= 0 {
		options.bodyLimit = DEFAULT_BODY_LIMIT
	}

	request, _ := http.NewRequest(options.method, url, nil)
	request.Header.Set("User-Agent", userAgent)
	if options.referer != "" {
		request.Header.Set("Referer", options.referer)
		request.Header.Set("Origin", inferOrigin(options.referer))
	}

	client := defaultClient
	if options.hasty {
		client = hastyClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.ContentLength > options.bodyLimit {
		return fmt.Errorf("file is too large")
	}
	limitedBody := io.LimitReader(response.Body, options.bodyLimit)

	if response.StatusCode != 200 && response.StatusCode != 206 {
		errBody, err := io.ReadAll(limitedBody)
		var bodyError = ""
		if err == nil {
			bodyError = string(errBody)
		}
		return &DownloadError{
			Code:    response.StatusCode,
			Message: "Failed to download file. " + bodyError,
		}
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, limitedBody)
	if err != nil {
		return err
	}
	return nil
}

func pathExists(aPath string) bool {
	_, err := os.Stat(aPath)
	return err == nil
}

func formatFloat(num float64, precision int) string {
	return strconv.FormatFloat(num, 'f', precision, 64)
}

func formatMegabytes(bytes int64, precision int) string {
	return formatFloat(float64(bytes)/float64(1024*1024), precision)
}

func getContentLength(url string, referer string) (int64, error) {
	// HEAD method returns metadata of a resource, but it may not be supported, better use GET
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return -1, err
	}
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	response, err := defaultClient.Do(request)
	if err != nil {
		return -1, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return -1, errors.New("Status code: " + response.Status)
	}
	length := response.Header.Get("Content-Length")
	if length == "" {
		return -1, errors.New("Content-Length header is empty")
	}
	return parseInt64(length)
}

func isTimeoutError(err error) bool {
	var urlErr *net_url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Timeout()
	}
	return false
}

func getDownloadErrorCode(err error) int {
	var downloadErr *DownloadError
	if errors.As(err, &downloadErr) {
		return downloadErr.Code
	}
	return -1
}

const BYTES_UNIT = "bytes="

// parseRangeHeader - parses header, returning the first available range or nil if the range is invalid
func parseRangeHeader(header string, max int64) (*Range, error) {
	if !strings.HasPrefix(header, BYTES_UNIT) {
		return nil, errors.New("expected 'bytes=' to be header's prefix")
	}
	header = header[len(BYTES_UNIT):]

	if end := strings.Index(header, ","); end != -1 {
		header = header[:end]
	}

	start, end, found := strings.Cut(header, "-")
	if !found {
		return nil, errors.New("invalid 'Range' header syntax")
	}
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" && end == "" {
		return nil, errors.New("invalid 'Range' header syntax")
	}

	if start == "" {
		// this is actually <suffix-length> and it follows different rules
		suffixLength, err := parseInt64(end)
		if err != nil {
			return nil, err
		}
		return newRange(max-suffixLength, max-1), nil
	}

	rangeSt, err := parseInt64(start)
	if err != nil {
		return nil, err
	}

	rangeEnd := max - 1
	if end != "" {
		rangeEnd, err = parseInt64(end)
		if err != nil {
			return nil, err
		}
	}

	return newRange(rangeSt, rangeEnd), nil
}

func parseInt64(number string) (int64, error) {
	return strconv.ParseInt(number, 10, 64)
}

func parseInt(number string) (int, error) {
	num, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(num), nil
}

type DownloadError struct {
	Code    int
	Message string
}

// Returns true if is DownloadError and any of the error codes
func isErrorStatus(err error, codes ...int) bool {
	var downloadErr *DownloadError
	if !errors.As(err, &downloadErr) {
		return false
	}

	return slices.Contains(codes, downloadErr.Code)
}

// Implements the error interface
func (e *DownloadError) Error() string {
	return fmt.Sprintf("NetworkError: Code=%d, Message=%s", e.Code, e.Message)
}

// Range - represents http byte range:
// start, end are inclusive and positive numbers, where start <= end
type Range struct {
	start int64
	end   int64
}

type Overlap int

const (
	LEFT Overlap = iota
	RIGHT
	NONE
	MIXED
)

var NO_RANGE = Range{-1, -1}

func (r *Range) String() string {
	return fmt.Sprintf("[%d,%d]", r.start, r.end)
}

func (r *Range) StringMB() string {
	startMB, endMB := formatMegabytes(r.start, 2)+"MB", formatMegabytes(r.end, 2)+"MB"
	return fmt.Sprintf("[%s,%s]", startMB, endMB)
}

func (r *Range) toContentRange(length int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.end, length)
}

func (r *Range) exceedsSize(size int64) bool {
	if r.start >= size || r.end >= size {
		return true
	}
	return false
}

func (r *Range) shift(by int64) {
	r.start += by
	r.end += by
}

// newRange creates a new range and returns it, if start and/or end is invalid NO_RANGE is returned
func newRange(start, end int64) *Range {
	if start < 0 || end < 0 || start > end {
		return &Range{NO_RANGE.start, NO_RANGE.end}
	}
	return &Range{start, end}
}

// This can only be merged if they overlap or connect with no gap
func (r *Range) mergeWith(other *Range) Range {
	mergedStart := min(r.start, other.start)
	mergedEnd := max(r.end, other.end)
	return Range{start: mergedStart, end: mergedEnd}
}

func (r *Range) overlaps(other *Range) bool {
	return r.start <= other.end && other.start <= r.end
}

// getOverlap determines the overlap type
func (r *Range) getOverlap(other *Range) Overlap {
	if !r.overlaps(other) {
		return NONE
	}
	if other.start <= r.start && other.end <= r.end {
		return LEFT
	}
	if r.start <= other.start && r.end <= other.end {
		return RIGHT
	}
	return MIXED
}

func (r *Range) connects(other *Range) bool {
	return r.end+1 == other.start || r.start == other.end+1
}

func (r *Range) encompasses(other *Range) bool {
	return r.start <= other.start && other.end <= r.end
}

func (r *Range) includes(value int64) bool {
	return r.start <= value && value <= r.end
}

// intersection returns the intersection of the two ranges, bool indicates if the ranges intersect at all
func (r *Range) intersection(other *Range) (Range, bool) {
	if !r.overlaps(other) {
		return NO_RANGE, false
	}
	start := max(r.start, other.start)
	end := min(r.end, other.end)
	return Range{start, end}, true
}

// difference gets the range-difference (relative complement) of 'r' with respect to 'other'
// in simple words - what is in the current range that's not in the other range
func (r *Range) difference(other *Range) []Range {
	if !r.overlaps(other) {
		return []Range{*r}
	}
	if other.encompasses(r) {
		return []Range{}
	}
	if r.encompasses(other) {
		if r.start == other.start {
			return []Range{*newRange(other.end+1, r.end)}
		}
		if r.end == other.end {
			return []Range{*newRange(r.start, other.start-1)}
		}
		return []Range{
			*newRange(r.start, other.start-1),
			*newRange(other.end+1, r.end),
		}
	}
	if r.start < other.start {
		return []Range{*newRange(r.start, other.start-1)}
	}
	return []Range{*newRange(other.end+1, r.end)}
}

// incorporateRange inserts range sequentially or merges with an overlapping range (slice of ranges must be sorted)
func incorporateRange(newRange *Range, ranges []Range) []Range {
	for i := 0; i < len(ranges); i++ {
		r := &ranges[i]
		if r.encompasses(newRange) {
			return ranges
		}

		if newRange.encompasses(r) || newRange.overlaps(r) || newRange.connects(r) {
			ranges = resolveMixedMerge(*newRange, ranges, i)
			return ranges
		}
		if newRange.end < r.start {
			return slices.Insert(ranges, i, *newRange)
		}
	}
	return append(ranges, *newRange)
}

// Resolves a mix of overlaps or contained ranges
func resolveMixedMerge(result Range, ranges []Range, from int) []Range {
	// The resolution always ends up at 'from' index
	for i := from; i < len(ranges); i++ {
		r := &ranges[i]

		if result.encompasses(r) {
			continue
		}
		if result.overlaps(r) || result.connects(r) {
			result = r.mergeWith(&result)
			continue
		}
		ranges[from] = result
		return append(ranges[:from+1], ranges[i:]...)
	}
	ranges[from] = result
	return ranges[:from+1]
}

func (r *Range) equals(other *Range) bool {
	return r.start == other.start && r.end == other.end
}

func (r *Range) length() int64 {
	return r.end - r.start + 1
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
	return path.Join(segments...), true
}

func isSlash(char uint8) bool {
	return char == '/' || char == '\\'
}

func Conditional[T any](condition bool, trueResult, falseResult T) T {
	if condition {
		return trueResult
	}
	return falseResult
}

func respondBadRequest(writer http.ResponseWriter, format string, args ...any) {
	output := fmt.Sprintf(format, args...)
	LogWarnUp(1, "%v", output)
	http.Error(writer, output, http.StatusBadRequest)
}

func respondUnauthorized(writer http.ResponseWriter, format string, args ...any) {
	output := fmt.Sprintf(format, args...)
	LogWarnUp(2, "%v", output)
	http.Error(writer, output, http.StatusUnauthorized)
}

func respondInternalError(writer http.ResponseWriter, format string, args ...any) {
	output := fmt.Sprintf(format, args...)
	LogErrorUp(1, "%v", output)
	http.Error(writer, output, http.StatusInternalServerError)
}

func respondTooManyRequests(writer http.ResponseWriter, ip string, retryAfter int) {
	output := fmt.Sprintf("Too many requests triggered by %v, retry-after: %v", ip, retryAfter)
	LogWarnUp(1, "%v", output)
	writer.Header().Add("Retry-After", toString(retryAfter))
	http.Error(writer, output, http.StatusTooManyRequests)
}

func generateRandomNickname() string {
	prefixes := []string{
		// Positive adjectives
		"Adaptable", "Adventurous", "Affectionate", "Altruistic", "Ambitious", "Amiable", "Amusing", "Analytical",
		"Articulate", "Artistic", "Attentive", "Authentic", "Benevolent", "Bold", "Brilliant", "Caring", "Charismatic",
		"Charitable", "Charming", "Cheerful", "Compassionate", "Confident", "Considerate", "Courageous", "Courteous",
		"Creative", "Determined", "Diligent", "Dynamic", "Eloquent", "Empathetic", "Empowering", "Endearing", "Energetic",
		"Enthusiastic", "Friendly", "Generous", "Genuine", "Gracious", "Grateful", "Happy", "Hardworking", "Honest",
		"Humble", "Innovative", "Inspirational", "Inspiring", "Intelligent", "Jovial", "Joyful", "Kind", "Lively",
		"Loving", "Loyal", "Modest", "Motivated", "Nurturing", "Optimistic", "Outgoing", "Passionate", "Patient", "Peaceful",
		"Persevering", "Playful", "Positive", "Proactive", "Professional", "Radiant", "Relaxed", "Reliable", "Resilient",
		"Resourceful", "Respectful", "Sensible", "Sincere", "Smart", "Sociable", "Steadfast", "Strong", "Supportive",
		"Sympathetic", "Tactful", "Talented", "Tenacious", "Thoughtful", "Trusting", "Trustworthy", "Understanding",
		"Upbeat", "Vibrant", "Visionary", "Vivacious", "Warm", "Warmhearted", "Wise", "Witty", "Zealous",

		// Colors
		"Amber", "Amethyst", "Apricot", "Aqua", "Aquamarine", "Auburn", "Azure", "Beige", "Black", "Blue", "Bronze",
		"Brown", "Buff", "Cardinal", "Carmine", "Celadon", "Cerise", "Cerulean", "Charcoal", "Chartreuse", "Chocolate",
		"Cinnamon", "Complementary", "Copper", "Coral", "Cream", "Crimson", "Cyan", "Dark", "Denim", "Ecru", "Emerald",
		"Fuchsia", "Gold", "Goldenrod", "Gray", "Green", "Grey", "Hue", "Indigo", "Ivory", "Jade", "Jet", "Khaki", "Lavender",
		"Lemon", "Light", "Lilac", "Lime", "Magenta", "Mahogany", "Maroon", "Mauve", "Mustard", "Ocher", "Olive", "Orange",
		"Orchid", "Pale", "Pastel", "Peach", "Periwinkle", "Persimmon", "Pewter", "Pink", "Puce", "Pumpkin", "Purple",
		"Rainbow", "Red", "Rose", "Ruby", "Russet", "Rusty", "Saffron", "Salmon", "Sapphire", "Scarlet", "Secondary",
		"Sepia", "Shade", "Shamrock", "Sienna", "Silver", "Slate", "Spectrum", "Tan", "Tangerine", "Taupe", "Teal",
		"Terracotta", "Thistle", "Tint", "Tomato", "Topaz", "Turquoise", "Ultramarine", "Umber", "Vermilion", "Violet",
		"Viridian", "Wheat", "White", "Wisteria", "Yellow",
	}

	suffixes := []string{
		// Fruits
		"Apple", "Apricot", "Avocado", "Banana", "Blackberry", "Blueberry", "Carambola", "Cherry", "Clementine",
		"Cloudberry", "Coconut", "Cranberry", "Cucumber", "Currant", "Eggplant", "Grapes", "Grapefruit", "Jackfruit",
		"Jujube", "Kiwi", "Kumquat", "Lemon", "Lime", "Lychee", "Mammee", "Mandarin", "Mango", "Mangosteen",
		"Mulberry", "Nance", "Nectarine", "Noni", "Olive", "Orange", "Papaya", "Pawpaw", "Peach", "Pear", "Persimmon",
		"Pineapple", "Plantain", "Plum", "Pomegranate", "Pomelo", "Pulasan", "Quine", "Rambutan", "Raspberries",
		"Rhubarb", "Rose Apple", "Sapodilla", "Satsuma", "Soursop", "Strawberry", "Sugar Apple", "Tamarillo", "Tamarind",
		"Tangelo", "Tangerine", "Ugli", "Watermelon",

		// Animals
		"Dog", "Cow", "Cat", "Horse", "Donkey", "Tiger", "Lion", "Panther", "Leopard", "Cheetah", "Bear", "Elephant",
		"Polar", "Turtle", "Tortoise", "Crocodile", "Rabbit", "Porcupine", "Hare", "Hen", "Pigeon", "Albatross",
		"Crow", "Fish", "Dolphin", "Frog", "Whale", "Alligator", "Eagle", "Flying", "squirrel", "Ostrich", "Fox", "Goat",
		"Jackal", "Emu", "Armadillo", "Eel", "Goose", "Arctic", "fox", "Wolf", "Beagle", "Gorilla", "Chimpanzee", "Monkey",
		"Beaver", "Orangutan", "Antelope", "Bat", "Badger", "Giraffe", "Hermit", "Crab", "Giant", "Panda", "Hamster",
		"Cobra", "Hammerhead", "Shark", "Camel", "Hawk", "Deer", "Chameleon", "Hippopotamus", "Jaguar", "Chihuahua",
		"King", "Cobra", "Ibex", "Lizard", "Koala", "Kangaroo", "Iguana", "Llama", "Chinchillas", "Dodo", "Jellyfish",
		"Rhinoceros", "Hedgehog", "Zebra", "Possum", "Wombat", "Bison", "Bull", "Buffalo", "Sheep", "Meerkat", "Mouse",
		"Otter", "Sloth", "Owl", "Vulture", "Flamingo", "Racoon", "Mole", "Duck", "Swan", "Lynx", "Monitor", "Lizard",
		"Elk", "Boar", "Lemur", "Mule", "Baboon", "Mammoth", "Blue", "Whale", "Rat", "Snake", "Peacock",
	}

	prefix := prefixes[rand.Intn(len(prefixes))]
	suffix := suffixes[rand.Intn(len(suffixes))]

	return prefix + " " + suffix
}

type RateLimiter struct {
	// hits store timestamps
	hits  *RingBuffer
	perMs int64
	mutex *sync.Mutex
}

// NewLimiter creates a new instance of RateLimiter based on:
//
//	hits - number of allowed hits per given time period
//	perSeconds  - time period in seconds
func NewLimiter(hits int, perSeconds int) *RateLimiter {
	return &RateLimiter{
		NewRingBuffer(hits),
		int64(perSeconds) * 1000,
		new(sync.Mutex),
	}
}

// Returns true if the call should be blocked, false otherwise
func (limiter *RateLimiter) block() bool {
	nowMs := time.Now().UnixMilli()
	// Update hits with the latest unix timestamp to keep blocking when requests continue to arrive
	if !limiter.hits.ForcePush(nowMs) {
		return false
	}

	madeSpace := false
	hits := limiter.hits
	for hits.Len() > 0 {
		msAgo := nowMs - hits.PeekEnd()
		// Remove the oldest block, one at a time
		if msAgo >= limiter.perMs {
			hits.PopEnd()
			madeSpace = true
		} else {
			break
		}
	}

	return !madeSpace
}

// Returns the number of seconds that must elapse before the next request can be made.
func (limiter *RateLimiter) getRetryAfter() int {
	nowMs := time.Now().UnixMilli()
	msAgo := nowMs - limiter.hits.PeekEnd()
	if msAgo >= limiter.perMs {
		return 0
	}
	remainingSeconds := float64(limiter.perMs-msAgo) / 1000
	return int(math.Ceil(remainingSeconds))
}

type RingBuffer struct {
	head, length, capacity int
	buffer                 []int64
}

func NewRingBuffer(size int) *RingBuffer {
	if size < 1 {
		panic("ring buffer size must be at least 1")
	}

	return &RingBuffer{
		head:     0,
		length:   0,
		capacity: size,
		buffer:   make([]int64, size),
	}
}

// Len returns the number of elements currently in the buffer
func (ring *RingBuffer) Len() int {
	return ring.length
}

// Push returns true if the element was successfully added to the underlying buffer,
// false if the operation was unsuccessful, indicating the buffer may be full
func (ring *RingBuffer) Push(element int64) bool {
	if ring.length == ring.capacity {
		return false
	}
	ring.buffer[ring.head] = element
	ring.length++

	if ring.head == ring.capacity-1 {
		ring.head = 0
	} else {
		ring.head++
	}
	return true
}

// ForcePush adds the element, returns true if the element had to be force pushed, false otherwise
func (ring *RingBuffer) ForcePush(element int64) bool {
	forcePush := false
	if ring.length == ring.capacity {
		forcePush = true
	} else {
		ring.length++
	}
	ring.buffer[ring.head] = element

	if ring.head == ring.capacity-1 {
		ring.head = 0
	} else {
		ring.head++
	}
	return forcePush
}

// PeekEnd returns the tail value.
// For Len() == 0 it returns the head value
func (ring *RingBuffer) PeekEnd() int64 {
	end := ring.getEndIndex()
	return ring.buffer[end]
}

func (ring *RingBuffer) getEndIndex() int {
	end := ring.head - ring.length
	if end < 0 {
		end = ring.capacity + end
	}
	return end
}

func (ring *RingBuffer) PopEnd() {
	if ring.length > 0 {
		ring.length--
	}
}

func (ring *RingBuffer) Clear() {
	ring.length = 0
}

const TIME_LAYOUT = "Mon, 02 Jan 2006 15:04:05 GMT"
const VERSION_LAYOUT = "02-Jan-2006-15:04:05"

func shareFile(w http.ResponseWriter, r *http.Request, path string) {
	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		respondBadRequest(w, "Range header missing")
		return
	}

	f, err := os.Open(path)
	if err != nil {
		respondInternalError(w, "Internal error occurred while opening file")
		LogError("Can't open file: %v", err)
		return
	}
	defer f.Close()

	stats, err := f.Stat()
	if err != nil {
		respondInternalError(w, "Internal error occurred while retrieving file info")
		LogError("Can't retrieve file info: %v", err)
		return
	}

	fileSize := stats.Size()
	byteRange, err := parseRangeHeader(rangeHeader, fileSize)
	if err != nil {
		LogInfo("Bad request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	shareFileInRange(w, r, byteRange, f, stats)
}

func shareFileInRange(w http.ResponseWriter, r *http.Request, byteRange *Range, f *os.File, stats os.FileInfo) {
	fileSize := stats.Size()
	if fileSize == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	if byteRange.exceedsSize(fileSize) {
		LogInfo("Range %v-%v is out of bounds", byteRange.start, byteRange.end)
		http.Error(w, "Range out of bounds", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	if _, err := f.Seek(byteRange.start, io.SeekStart); err != nil {
		LogError("Unable to seek within file but range is satisfiable: %v", err)
		http.Error(w, "Seek failed.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	SetLastModified(w, stats.ModTime())

	rangeLength := byteRange.length()
	w.Header().Set("Content-Length", int64ToString(rangeLength))

	w.Header().Set("Content-Range", byteRange.toContentRange(fileSize))

	w.WriteHeader(http.StatusPartialContent)
	if r.Method != "HEAD" {
		io.CopyN(w, f, rangeLength)
	}
}

func SetLastModified(w http.ResponseWriter, lastModified time.Time) {
	w.Header().Set("Last-Modified", lastModified.UTC().Format(TIME_LAYOUT))
}

func minOf(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func endsWithAny(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func startsWithAny(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

const CDM_THRESHOLD = 0

// validateName validates a string ensuring it is non-empty, consists only of valid UTF8 characters,
// where space is the only separator and consecutive Combining Diacritical Marks count doesn't exceed CDM_THRESHOLD
func validateName(str string) bool {
	cdmCount := 0
	isEmpty := true
	for _, char := range str {
		switch char {
		case utf8.RuneError:
			return false
		case ' ':
			continue
		case '\t', '\r', '\n', '\u200b', '\u200c', '\u000b':
			return false
		}
		isEmpty = false
		if isCombiningDiacriticalMark(char) {
			cdmCount++
		} else {
			cdmCount = 0
		}
		if cdmCount > CDM_THRESHOLD {
			return false
		}
	}
	return !isEmpty
}

func isCombiningDiacriticalMark(r rune) bool {
	return r >= 0x0300 && r <= 0x036F
}

type Set[T comparable] struct {
	set map[T]struct{}
}

func (s *Set[T]) Contains(el T) bool {
	_, contains := s.set[el]
	return contains
}

func (s *Set[T]) Add(el T) {
	s.set[el] = struct{}{}
}

func (s *Set[T]) Remove(el T) {
	delete(s.set, el)
}

func NewSet[T comparable](capacity int) *Set[T] {
	return &Set[T]{
		make(map[T]struct{}, capacity),
	}
}

// IpV4Range - start & end IPs are inclusive
type IpV4Range struct {
	start, end net.IP
}

func newIpV4Range(start, end string) *IpV4Range {
	startIp := net.ParseIP(start).To4()
	if startIp == nil {
		return nil
	}

	endIp := net.ParseIP(end).To4()
	if endIp == nil {
		return nil
	}

	if Precedes(startIp, endIp) {
		return &IpV4Range{startIp, endIp}
	} else {
		return nil
	}
}

func Precedes(start net.IP, end net.IP) bool {
	for i := range 4 {
		if start[i] > end[i] {
			return false
		}

		if start[i] < end[i] {
			return true
		}
	}

	return true
}

func (r *IpV4Range) Contains(ipv4Raw string) bool {
	ip := net.ParseIP(ipv4Raw).To4()
	if ip == nil {
		return false
	}

	for i := range 4 {
		if r.start[i] > ip[i] {
			return false
		}
		if r.start[i] < ip[i] {
			break
		}
	}

	for i := range 4 {
		if r.end[i] < ip[i] {
			return false
		}
		if r.end[i] > ip[i] {
			break
		}
	}

	return true
}

func isRequestDone(request *http.Request) bool {
	select {
	case <-request.Context().Done():
		return true
	default:
		return false
	}
}

var TITLE_CUT_OFF_RUNES = []rune{'|'}

// parseSongTitle returns artist and trackName from "Artist - Song Title" format.
// The resulting track name is stripped of any title descriptors.
// If there's no separator the track name becomes the title.
// If artist is given the track name terminates on any TITLE_CUT_OFF_RUNES
func parseSongTitle(title string) (string, string) {
	artist, trackName, found := strings.Cut(title, "-")
	if !found {
		return "", strings.TrimSpace(title)
	}
	cleanName := strings.Builder{}
	trackChars := []rune(trackName)
	openRoundBrackets := 0
	openSquareBrackets := 0
	for i := range trackChars {
		switch trackChars[i] {
		case '(':
			openRoundBrackets++
			continue
		case ')':
			openRoundBrackets--
			continue
		case '[':
			openSquareBrackets++
			continue
		case ']':
			openSquareBrackets--
			continue
		}

		if openRoundBrackets == 0 && openSquareBrackets == 0 {
			if slices.Contains(TITLE_CUT_OFF_RUNES, trackChars[i]) {
				break
			}
			cleanName.WriteRune(trackChars[i])
		}
	}

	return strings.TrimSpace(artist), strings.TrimSpace(cleanName.String())
}

func createSubtitle(filename string, extension string) Subtitle {
	os.MkdirAll(CONTENT_SUBS, os.ModePerm)

	randomString := generateSubName()
	outputName := fmt.Sprintf("%v%v", randomString, extension)
	outputPath := path.Join(CONTENT_SUBS, outputName)

	name := strings.TrimSuffix(filename, extension)
	name = cleanupResourceName(name)

	subtitle := Subtitle{
		Name: name,
		Url:  outputPath,
	}

	return subtitle
}

func bufferStartsWith(buffer *bytes.Buffer, prefix []byte) bool {
	b := buffer.Bytes()
	prefixLen := len(prefix)
	if len(b) < prefixLen {
		return false
	}
	return bytes.Equal(b[:prefixLen], prefix)
}

// SpeedTest measures download or upload speed
type SpeedTest struct {
	lastSnapshot time.Time
	frequency    time.Duration
	snapshots    *RingBuffer
}

func (test *SpeedTest) TestMBps(byteCount int64) float64 {
	now := time.Now()
	// Don't restrict the first write but don't measure it
	if test.snapshots.Len() == 0 {
		test.snapshots.Push(byteCount)
		test.lastSnapshot = now
		return 0
	}

	// Remove outdated snapshots to maintain accuracy
	lastPushDiff := now.Sub(test.lastSnapshot)
	bufferTime := test.frequency * time.Duration(test.snapshots.capacity)
	discardRatio := lastPushDiff.Seconds() / bufferTime.Seconds()
	discardCount := int(discardRatio * float64(test.snapshots.capacity))
	targetSize := test.snapshots.capacity - discardCount
	for test.snapshots.Len() > targetSize {
		test.snapshots.PopEnd()
	}

	if lastPushDiff >= test.frequency {
		test.snapshots.ForcePush(byteCount)
		test.lastSnapshot = now
	}
	if test.snapshots.Len() == 1 {
		return 0
	}
	lastCount := test.snapshots.PeekEnd()
	if lastCount > byteCount {
		return -1
	}
	byteDiff := byteCount - lastCount
	multiplier := time.Duration(test.snapshots.Len()) - 1
	timeFrame := multiplier * test.frequency
	bytesPerSecond := float64(byteDiff) / timeFrame.Seconds()
	return bytesPerSecond / MB
}

func (test *SpeedTest) Reset() {
	test.snapshots.Clear()
}

func NewDefaultSpeedTest() *SpeedTest {
	return NewSpeedTest(time.Millisecond*100, 20)
}

func NewSpeedTest(frequency time.Duration, snapshotCount int) *SpeedTest {
	if snapshotCount < 2 {
		snapshotCount = 2
	}
	return &SpeedTest{
		lastSnapshot: time.Now(),
		frequency:    frequency,
		snapshots:    NewRingBuffer(snapshotCount),
	}
}

type Sleeper struct {
	ch      chan struct{}
	rwMutex sync.RWMutex
}

func NewSleeper() *Sleeper { return &Sleeper{ch: make(chan struct{})} }

// Sleep sleeps until woken or timeout, returning true and false respectively
func (s *Sleeper) Sleep(timeout time.Duration) bool {
	s.rwMutex.RLock()
	channel := s.ch
	s.rwMutex.RUnlock()
	select {
	case <-channel:
		return true
	case <-time.After(timeout):
		return false
	}
}

// WakeAll Wake closes and replaces the channel to wake all current waiters.
func (s *Sleeper) WakeAll() {
	s.rwMutex.Lock()
	close(s.ch)
	s.ch = make(chan struct{})
	s.rwMutex.Unlock()
}

func getFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func replacePrefix(s, oldPrefix, newPrefix string) string {
	if strings.HasPrefix(s, oldPrefix) {
		return newPrefix + s[len(oldPrefix):]
	}
	return s
}

type CachedFile struct {
	content  []byte
	mimeType string
	modTime  time.Time
	modLock  *sync.RWMutex
}

func (file *CachedFile) RefreshIfOutdated(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	freshModTime := info.ModTime()
	file.modLock.RLock()
	if freshModTime.Equal(file.modTime) {
		file.modLock.RUnlock()
		return nil
	}
	file.modLock.RUnlock()
	file.modLock.Lock()
	LogDebug("Refreshing cached %v", filePath)
	defer file.modLock.Unlock()

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	buf := make([]byte, info.Size())
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return err
	}

	extension := filepath.Ext(info.Name())
	file.content = buf
	file.mimeType = mime.TypeByExtension(extension)
	file.modTime = info.ModTime()
	return nil
}

type CachedFsHandler struct {
	fsHandler http.Handler
	// The map is safe to use for concurrent access because only the lock-protected values are modified at runtime.
	// Serving function can correct the state of a *CachedFile.
	paths     map[string]*CachedFile
	validated bool
}

func (cache *CachedFsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cache.fsHandler.ServeHTTP(w, r)
}

func (cache *CachedFsHandler) populateCache(dir string) error {
	if cache.paths == nil {
		cache.paths = make(map[string]*CachedFile)
	}
	size := int64(0)
	err := WalkFiles(dir, func(filePath string, fileEntry os.DirEntry) {
		info, err := fileEntry.Info()
		if err != nil {
			return
		}

		f, err := os.Open(filePath)
		if err != nil {
			return
		}
		buf := make([]byte, info.Size())
		_, err = io.ReadFull(f, buf)
		if err != nil {
			return
		}
		size += int64(len(buf))
		extension := filepath.Ext(info.Name())

		cache.paths[filePath] = &CachedFile{
			content:  buf,
			mimeType: mime.TypeByExtension(extension),
			modTime:  info.ModTime(),
			modLock:  new(sync.RWMutex),
		}
	})
	if err != nil {
		return err
	}
	mbFormat := formatMegabytes(size, 2)
	LogDebug("Cached %v file paths from directory '%v' (%v MB)", len(cache.paths), dir, mbFormat)
	return nil
}

// RefreshCache refreshes only the existing path entries
func (cache *CachedFsHandler) RefreshCache() {
	for fPath, cachedFile := range cache.paths {
		err := cachedFile.RefreshIfOutdated(fPath)
		if err != nil {
			LogError("Failed to refresh cache entry for %v: %v", fPath, err)
		}
	}
}

func serveCachedFile(w http.ResponseWriter, r *http.Request, path string, cachedFile *CachedFile, validated bool) {
	if cachedFile == nil {
		LogError("Cached file is nil for path: %v", path)
		return
	}
	LogDebug("Connection %s requested resource %v from cache", getIp(r), path)
	if validated {
		err := cachedFile.RefreshIfOutdated(path)
		if err != nil {
			respondInternalError(w, "Error on refresh: %v", err.Error())
			return
		}
	}

	cachedFile.modLock.RLock()
	buffer := cachedFile.content
	contentType := cachedFile.mimeType
	modTime := cachedFile.modTime
	cachedFile.modLock.RUnlock()
	SetLastModified(w, modTime)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", toString(len(buffer)))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer)
}

func NewCachedFsHandler(strippedPrefix, dir string, validated bool) *CachedFsHandler {
	cache := &CachedFsHandler{validated: validated}
	err := cache.populateCache(dir)
	if err != nil {
		LogError("Failed to populate cache: %v", err)
	}
	cachedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upath := r.URL.Path
		if !strings.HasPrefix(upath, "/") {
			upath = "/" + upath
			r.URL.Path = upath
		}
		upath = path.Clean(upath)
		// The path could still be traversed with reverse slash which path.Clean doesn't handle
		safePath, safe := safeJoin(upath)
		if safe {
			diskPath := path.Join(dir, safePath)
			cachedFile, found := cache.paths[diskPath]
			if found {
				serveCachedFile(w, r, diskPath, cachedFile, cache.validated)
				return
			}
			// Serve the default page if there's a mapping for index.html
			diskPath = path.Join(diskPath, "index.html")
			if cachedFile, found = cache.paths[diskPath]; found {
				serveCachedFile(w, r, diskPath, cachedFile, cache.validated)
				return
			}
			http.NotFound(w, r)
			return
		}
		respondBadRequest(w, "Invalid path: %v", upath)
	})
	cache.fsHandler = http.StripPrefix(strippedPrefix, cachedHandler)
	return cache
}

// WalkFiles walks all files contained in this directory and subdirectories iteratively
func WalkFiles(root string, fn func(filePath string, entry os.DirEntry)) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("root is not a directory")
	}

	var dirStack []string
	dirStack = append(dirStack, root)

	for len(dirStack) > 0 {
		n := len(dirStack) - 1
		dir := dirStack[n]
		dirStack = dirStack[:n]

		// Must be a directory
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			aPath := path.Join(dir, entry.Name())
			if entry.IsDir() {
				dirStack = append(dirStack, aPath)
			} else {
				fn(aPath, entry)
			}
		}
	}
	return nil
}
