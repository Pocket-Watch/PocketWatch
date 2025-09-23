package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
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

func GeneratePrettyTableStandard(headers []string, data []string) string {
	if len(headers) == 0 {
		return ""
	}

	columnCount := len(headers)
	rowCount := len(data) / columnCount

	// Find max size of each column.

	paddings := make([]int, columnCount)
	for i, header := range headers {
		paddings[i] = len(header)
	}

	for row := 0; row < rowCount; row += 1 {
		for column := 0; column < columnCount; column += 1 {
			value := data[row*columnCount+column]

			if len(value) > paddings[column] {
				paddings[column] = len(value)
			}
		}
	}

	// Generate separator.

	build := strings.Builder{}
	for _, pad := range paddings {
		build.WriteString("+")
		for range pad + 2 {
			build.WriteString("-")
		}
	}
	build.WriteString("+\n")

	separator := build.String()

	// Pretty print the table.

	table := strings.Builder{}

	table.WriteString(separator)
	for i, name := range headers {
		str := fmt.Sprintf("| %*s ", paddings[i], name)
		table.WriteString(str)
	}
	table.WriteString("|\n")
	table.WriteString(separator)

	if rowCount == 0 {
		return table.String()
	}

	for row := 0; row < rowCount; row += 1 {
		for column := 0; column < columnCount; column += 1 {
			value := data[row*columnCount+column]
			str := fmt.Sprintf("| %*s ", paddings[column], value)
			table.WriteString(str)
		}

		table.WriteString("|\n")
	}
	table.WriteString(separator)

	return table.String()
}

func GeneratePrettyTableAsciiExtended(headers []string, data []string) string {
	if len(headers) == 0 {
		return ""
	}

	columnCount := len(headers)
	rowCount := len(data) / columnCount

	// Find max size of each column.

	paddings := make([]int, columnCount)
	for i, header := range headers {
		paddings[i] = len(header)
	}

	for row := 0; row < rowCount; row += 1 {
		for column := 0; column < columnCount; column += 1 {
			value := data[row*columnCount+column]

			if len(value) > paddings[column] {
				paddings[column] = len(value)
			}
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
		str := fmt.Sprintf("│ %*s ", paddings[i], name)
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
			format := fmt.Sprintf("│ %*s ", paddings[column], value)
			table.WriteString(format)
		}

		table.WriteString("│\n")
	}
	table.WriteString(separatorBot)

	return table.String()
}

func GeneratePrettyTable(headers []string, data []string) string {
	return GeneratePrettyTableAsciiExtended(headers, data)
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
	end := strings.Index(path, "?")
	if end == -1 {
		return path
	}
	return path[:end]
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
	questionMark := strings.Index(url, "?")
	if questionMark == -1 {
		return url
	}
	return url[:questionMark]
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
		return nil, &DownloadError{Code: response.StatusCode, Message: "Failed to read response body."}
	}
	return buffer, nil
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

// This will download a chunk of a file within the specified range
func openFileDownload(url string, from int64, referer string) (*http.Response, error) {
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	request.Header.Set("Range", fmt.Sprintf("bytes=%v-", from))

	response, err := defaultClient.Do(request)
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

// This function tests the server's intent to serve and return data (at most 512 bytes)
// Returns true if test status code is successful, false if unsuccessful or body couldn't be read
func testGetResponse(url string, referer string) (bool, *bytes.Buffer) {
	parsedUrl, err := net_url.Parse(url)
	if err != nil {
		LogDebug("Tested %v", parsedUrl)
		return false, nil
	}
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}

	response, err := defaultClient.Do(request)
	if err != nil {
		return false, nil
	}
	defer response.Body.Close()

	length := 512
	if int(response.ContentLength) < length {
		length = int(response.ContentLength)
	}

	buffer := new(bytes.Buffer)
	limitedResponse := io.LimitReader(response.Body, int64(length))
	_, readErr := io.Copy(buffer, limitedResponse)
	if readErr != nil && readErr != io.EOF {
		return false, nil
	}
	success := response.StatusCode >= 200 && response.StatusCode < 300
	return success, buffer
}

type DownloadOptions struct {
	referer   string
	hasty     bool
	bodyLimit int64
	method    string
	writer    http.ResponseWriter
}

var defaultDownloadOptions = DownloadOptions{
	bodyLimit: 32 * GB,
	method:    "GET",
}

func downloadFile(url string, path string, options *DownloadOptions) error {
	if options == nil {
		options = &defaultDownloadOptions
	}
	if options.bodyLimit == 0 {
		options.bodyLimit = defaultDownloadOptions.bodyLimit
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

	if !fileExists(path) {
		LogError("File created but doesn't exist at %v", path)
		return err
	}

	_, err = io.Copy(out, limitedBody)
	if err != nil {
		return err
	}
	return nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func getContentLength(url string, referer string) (int64, error) {
	// HEAD method returns metadata of a resource
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return -1, err
	}
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	// Send the request
	response, err := defaultClient.Do(request)
	if err != nil {
		return -1, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return -1, errors.New("Status code: " + response.Status)
	}
	// Get the Content-Length header
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

func generateUniqueId() uint64 {
	src := rand.NewSource(time.Now().UnixNano())
	entropy := rand.New(src).Uint64() // Generate a random number between 0 and 99
	return entropy
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

func (r *Range) toContentRange(length int64) string {
	return fmt.Sprintf("bytes %v-%v/%v", r.start, r.end, length)
}

func (r *Range) exceedsSize(size int64) bool {
	if r.start >= size || r.end >= size {
		return true
	}
	return false
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

func Conditional[T any](condition bool, trueResult, falseResult T) T {
	if condition {
		return trueResult
	} else {
		return falseResult
	}
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
		&sync.Mutex{},
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

var zeroTime = time.Unix(0, 0)

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
	// TODO: take care of empty files and respond with 200

	byteRange, err := parseRangeHeader(rangeHeader, fileSize)
	if err != nil {
		LogInfo("Bad request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
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

type Pair[T1 any, T2 any] struct {
	_1 T1
	_2 T2
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
	for i := 0; i < 4; i++ {
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
	for i := 0; i < 4; i++ {
		if r.start[i] > ip[i] {
			return false
		}
		if r.start[i] < ip[i] {
			break
		}
	}
	for i := 0; i < 4; i++ {
		if r.end[i] < ip[i] {
			return false
		}
		if r.end[i] > ip[i] {
			break
		}
	}
	return true
}
