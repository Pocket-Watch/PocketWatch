package main

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// https://datatracker.ietf.org/doc/html/rfc8216
// https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis

const (
	// 4.3.1. [Basic Tags]
	EXTM3U        = "EXTM3U"        // standalone
	EXT_X_VERSION = "EXT-X-VERSION" // value: protocol version

	// 4.3.2. [Media Segment Tags]
	EXTINF                  = "EXTINF"                  // <duration in seconds> <title> (title is optional)
	EXT_X_BYTERANGE         = "EXT-X-BYTERANGE"         // length@start
	EXT_X_DISCONTINUITY     = "EXT-X-DISCONTINUITY"     // standalone
	EXT_X_KEY               = "EXT-X-KEY"               // attribute-list keys: METHOD,URI,IV,KEYFORMAT,KEYFORMATVERSIONS
	EXT_X_MAP               = "EXT-X-MAP"               // attribute-list keys: URI,BYTERANGE
	EXT_X_PROGRAM_DATE_TIME = "EXT-X-PROGRAM-DATE-TIME" // time YYYY-MM-DDThh:mm:ss.SSSZ
	EXT_X_DATERANGE         = "EXT-X-DATERANGE"         // attribute-list keys: ID,CLASS,START-DATE,END-DATE,(5 more)
	EXT_X_BITRATE           = "EXT-X-BITRATE"           // value in rate

	// 4.3.3. [Media Playlist Tags]
	EXT_X_TARGETDURATION         = "EXT-X-TARGETDURATION"         // <duration in seconds>
	EXT_X_MEDIA_SEQUENCE         = "EXT-X-MEDIA-SEQUENCE"         // <number>
	EXT_X_DISCONTINUITY_SEQUENCE = "EXT-X-DISCONTINUITY-SEQUENCE" // <number>
	// EXT_X_ENDLIST Media playlist tag - helps determine whether the list is a live playlist
	EXT_X_ENDLIST       = "EXT-X-ENDLIST"       // standalone (MAY occur anywhere in the Media Playlist file)
	EXT_X_PLAYLIST_TYPE = "EXT-X-PLAYLIST-TYPE" // value: EVENT/VOD
	EXT_X_ALLOW_CACHE   = "EXT-X-ALLOW-CACHE"   // value: YES/NO

	// 4.3.4. [Master Playlist Tags]
	EXT_X_MEDIA        = "EXT-X-MEDIA"        // attribute-list keys: TYPE,URI,GROUP-ID,LANGUAGE,ASSOC-LANGUAGE,NAME,DEFAULT(...)
	EXT_X_STREAM_INF   = "EXT-X-STREAM-INF"   // <attribute-list> <URI> keys: BANDWIDTH,AVERAGE-BANDWIDTH,CODECS,RESOLUTION,FRAME-RATE,AUDIO,VIDEO(...)
	EXT_X_SESSION_DATA = "EXT-X-SESSION-DATA" // attribute-list keys: DATA-ID,VALUE,URI,LANGUAGE ; carries arbitrary data
	EXT_X_SESSION_KEY  = "EXT-X-SESSION-KEY"  // <attribute-list>

	// 4.3.5. [Media or Master Playlist Tags]
	EXT_X_INDEPENDENT_SEGMENTS = "EXT-X-INDEPENDENT-SEGMENTS" // standalone
	EXT_X_START                = "EXT-X-START"                // attribute-list keys: TIME-OFFSET,PRECISE
	EXT_X_PREFETCH             = "EXT-X-PREFETCH"             // apparently is followed by url?

	//  4.4.6. [Multivariant Playlist Tags]
	EXT_X_I_FRAME_STREAM_INF = "EXT-X-I-FRAME-STREAM-INF"
)

// GENERIC_TAGS are tags which are not directly handled in any case statement and appear in no particular order
var GENERIC_TAGS = []string{
	EXT_X_VERSION,
	EXT_X_TARGETDURATION,
	EXT_X_MEDIA_SEQUENCE,
	EXT_X_DISCONTINUITY_SEQUENCE,
	EXT_X_PLAYLIST_TYPE,
	EXT_X_ALLOW_CACHE,

	EXT_X_SESSION_DATA,
	EXT_X_SESSION_KEY,

	EXT_X_INDEPENDENT_SEGMENTS,
	EXT_X_START,
	EXT_X_PREFETCH,

	EXT_X_I_FRAME_STREAM_INF,
}

func detectM3U(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() && strings.HasPrefix(scanner.Text(), "#"+EXTM3U) {
		return true, nil
	}
	return false, nil
}

type KeyValue struct {
	key   string
	value string
}

// getKeyValue assumes the pair starts with #
// Value can be empty
func getKeyValue(line string) *KeyValue {
	// The shortest tag is 6 characters long (eg. EXTINF)
	if len(line) < 7 {
		return nil
	}
	line = line[1:]
	colon := strings.Index(line, ":")
	if colon == -1 || colon == len(line)-1 {
		return &KeyValue{line, ""}
	}
	if colon < 6 {
		return nil
	}
	key := line[:colon]
	value := line[colon+1:]
	return &KeyValue{key, value}
}

func parseM3U(path string) (*M3U, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hasEnd := false
	scanner := bufio.NewScanner(file)

	if scanner.Scan() && scanner.Text() != ("#"+EXTM3U) {
		return nil, fmt.Errorf("not a valid M3U playlist, missing EXTM3U tag")
	}

	m3u := newM3U(32)
	parsingSegment := false

	segment := Segment{}
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		isTag := strings.HasPrefix(line, "#EXT")
		if isTag {
			pair := getKeyValue(line)
			if pair == nil {
				// Skip the tag, invalid pair
				continue
			}

			switch pair.key {
			case EXTINF:
				duration, err := parseFirstFloat(pair.value)
				if err != nil {
					continue
				}
				segment.length = duration
				parsingSegment = true
			case EXT_X_MAP:
				params := parseParams(pair.value)
				if len(params) == 0 {
					continue
				}
				// BYTERANGE is rare so ignoring for now
				segment.mapUri = getParamValue("URI", params)
				parsingSegment = true
			case EXT_X_KEY:
				params := parseParams(pair.value)
				if len(params) == 0 {
					continue
				}
				// ignoring KEYFORMAT for now

				key := &segment.key
				key.uri = getParamValue("URI", params)
				key.method = getParamValue("METHOD", params)
				key.initVec = getParamValue("IV", params)
				segment.hasKey = true
				parsingSegment = true
			case EXT_X_PROGRAM_DATE_TIME, EXT_X_BITRATE, EXT_X_DATERANGE, EXT_X_BYTERANGE, EXT_X_DISCONTINUITY:
				segment.addPair(*pair)
				parsingSegment = true
			case EXT_X_ENDLIST:
				hasEnd = true
				parsingSegment = false
			case EXT_X_STREAM_INF:
				m3u.isMasterPlaylist = true
				params := parseParams(pair.value)
				if !scanner.Scan() {
					// Maybe it can be on the same line?
					return m3u, fmt.Errorf("unexpected EOF, expected URL after %v", EXT_X_STREAM_INF)
				}
				url := scanner.Text()
				track := Track{url: url, streamInfo: params}
				m3u.addTrack(track)
			case EXT_X_MEDIA:
				m3u.isMasterPlaylist = true
				params := parseParams(pair.value)
				typeValue := getParamValue("TYPE", params)
				// possible types: AUDIO, VIDEO, SUBTITLES, CLOSED-CAPTIONS
				if typeValue == "AUDIO" {
					m3u.audioRenditions = append(m3u.audioRenditions, params)
				}
			default:
				if slices.Contains(GENERIC_TAGS, pair.key) {
					m3u.addPair(*pair)
				}
			}
		} else if parsingSegment {
			// This should copy the current segment
			segment.url = line
			m3u.addSegment(segment)
			parsingSegment = false
			segment = Segment{}
		} // else Probably garbage?
	}

	m3u.isLive = !hasEnd && !m3u.isMasterPlaylist
	listType := m3u.getAttribute(EXT_X_PLAYLIST_TYPE)
	if m3u.isLive && listType == "VOD" {
		m3u.isLive = false
	}
	return m3u, nil
}

func parseFirstFloat(values string) (float64, error) {
	end := len(values)
	comma := strings.Index(values, ",")
	if comma != -1 {
		end = comma
	}
	return strconv.ParseFloat(values[:end], 64)
}

// Key - Media Segments MAY be encrypted
type Key struct {
	method  string // NONE, AES-128, and SAMPLE-AES, SAMPLE-AES-CTR
	uri     string // URI where the key can be obtained
	initVec string // hexadecimal-sequence that specifies a 128-bit Initialization Vector
}

func parseParams(line string) []Param {
	params := make([]Param, 0)
	var key strings.Builder
	var value strings.Builder
	onKey := true
	inString := false

	for i := range len(line) {
		switch line[i] {
		case ',':
			if inString {
				// comma is part of value
				value.WriteByte(',')
				break
			}
			// comma acts as a pair separator here
			pair := Param{key.String(), value.String()}
			params = append(params, pair)
			key.Reset()
			value.Reset()
			onKey = true
		case '"':
			// maybe don't include " in value
			inString = !inString
		case '=':
			if onKey {
				onKey = false
				break
			}
			// an '=' is part of value
			value.WriteByte('=')
		default:
			if onKey {
				key.WriteByte(line[i])
			} else {
				value.WriteByte(line[i])
			}
		}
	}
	if key.Len() > 0 || value.Len() > 0 {
		pair := Param{key.String(), value.String()}
		params = append(params, pair)
	}
	return params
}

type Param struct {
	key, value string
}

func (param *Param) String() string {
	return fmt.Sprintf("%v=%v", param.key, param.value)
}

// Track - Variant Stream (represents a m3u8 entry along with its metadata in a master playlist)
type Track struct {
	url string
	// #EXT-X-STREAM-INF info about m3u8 playlists
	streamInfo []Param
}

// getParamValue searches params and returns the value associated with the first given key, or empty if not found
func getParamValue(paramKey string, params []Param) string {
	if paramKey == "" {
		return ""
	}
	for _, pair := range params {
		if pair.key == paramKey {
			return pair.value
		}
	}
	return ""
}

// getParamValue searches params and returns the param pair of the given key or nil if not found
func getParam(paramKey string, params []Param) *Param {
	for i := range params {
		pair := &params[i]
		if pair.key == paramKey {
			return pair
		}
	}
	return nil
}

// removeAttributes searches attribute pairs and removes every pair matching the given key
func (m3u *M3U) removeAttributes(key string) {
	pairs := m3u.attributePairs
	for i := 0; i < len(pairs); i++ {
		if pairs[i].key == key {
			pairs[i] = pairs[len(pairs)-1]
			pairs = pairs[:len(pairs)-1]
			i--
		}
	}
	m3u.attributePairs = pairs
}

type M3U struct {
	url              string // the URL where the playlist was originally obtained
	isMasterPlaylist bool
	isLive           bool
	tracks           []Track    // exclusive to master playlists
	audioRenditions  [][]Param  // EXT-X-MEDIA only of TYPE=AUDIO
	attributePairs   []KeyValue // key:value properties which describe the playlist
	segments         []Segment  // Segment URLs appearing in an ordered sequence
}

type Segment struct {
	url            string
	length         float64
	mapUri         string // [optional] Media Initialization Section
	hasKey         bool
	key            Key
	attributePairs []KeyValue
}

func (segment *Segment) addPair(pair KeyValue) {
	segment.attributePairs = append(segment.attributePairs, pair)
}

func (segment *Segment) getAttribute(key string) string {
	for i := range segment.attributePairs {
		pair := &segment.attributePairs[i]
		if pair.key == key {
			return pair.value
		}
	}
	return ""
}

func prefixUrl(prefix, url string) string {
	if prefix == "" {
		return url
	}
	if strings.HasSuffix(prefix, "/") || strings.HasPrefix(url, "/") {
		return prefix + url
	}
	return prefix + "/" + url
}

func newM3U(segmentCapacity uint32) *M3U {
	m3u := new(M3U)
	m3u.segments = make([]Segment, 0, segmentCapacity)
	return m3u
}

// Returns the value associated with the first given key or "" if key is missing
func (m3u *M3U) getAttribute(key string) string {
	for i := range m3u.attributePairs {
		pair := &m3u.attributePairs[i]
		if pair.key == key {
			return pair.value
		}
	}
	return ""
}

func (m3u *M3U) addPair(pair KeyValue) {
	m3u.attributePairs = append(m3u.attributePairs, pair)
}

func (m3u *M3U) addSegment(seg Segment) {
	m3u.segments = append(m3u.segments, seg)
}

func (m3u *M3U) addTrack(track Track) {
	m3u.tracks = append(m3u.tracks, track)
}

func (m3u *M3U) avgSegmentLength() float64 {
	var sum float64
	for _, track := range m3u.segments {
		sum += track.length
	}
	return sum / float64(len(m3u.segments))
}

// duration of all segments summed up in seconds
func (m3u *M3U) totalDuration() float64 {
	var seconds float64
	for _, seg := range m3u.segments {
		seconds += seg.length
	}
	return seconds
}

// Chooses best track based on highest resolution and provided filter.
// This method should only be called on master playlists.
func (m3u *M3U) getBestTrack(filter func(track *Track) bool) *Track {
	if len(m3u.tracks) == 0 {
		return nil
	}
	var bestTrack *Track = nil
	var bestWidth int64 = 0
	for i := range m3u.tracks {
		track := &m3u.tracks[i]
		res := getParamValue("RESOLUTION", track.streamInfo)
		if res == "" {
			continue
		}
		success, width, _ := parseResolution(res)
		if !success {
			continue
		}
		if width > bestWidth && (filter == nil || filter(track)) {
			bestWidth = width
			bestTrack = track
		}
	}
	if bestTrack == nil {
		// If none match return the last
		length := len(m3u.tracks)
		return &m3u.tracks[length-1]
	}
	return bestTrack
}

// Targets given height but if one is not found defaults to lower available quality.
// this method should only be used if the m3u is a master playlist
func (m3u *M3U) getTrackByVideoHeight(targetHeight int64) *Track {
	if len(m3u.tracks) == 0 {
		return nil
	}
	var targetTrack *Track = nil
	var maxHeight int64
	for i := range m3u.tracks {
		track := &m3u.tracks[i]
		res := getParamValue("RESOLUTION", track.streamInfo)
		if res == "" {
			continue
		}
		success, _, height := parseResolution(res)
		if !success {
			continue
		}
		if height == targetHeight {
			return track
		}
		if height < targetHeight && height > maxHeight {
			maxHeight = height
			targetTrack = track
		}
	}
	if targetTrack == nil {
		// If none match return first
		return &m3u.tracks[0]
	}
	return targetTrack
}

type Width = int64
type Height = int64

func parseResolution(res string) (bool, Width, Height) {
	res = strings.ToLower(res)
	x := strings.Index(res, "x")
	if x == -1 {
		return false, 0, 0
	}
	width, err := strconv.ParseInt(res[:x], 10, 32)
	if err != nil {
		return false, 0, 0
	}
	height, err := strconv.ParseInt(res[x+1:], 10, 32)
	if err != nil {
		return false, 0, 0
	}
	return true, width, height
}

// This method will only prefix relative URLs
func (m3u *M3U) prefixRelativeTracks(prefix string) {
	for i := range m3u.audioRenditions {
		rendition := &m3u.audioRenditions[i]
		uriParam := getParam("URI", *rendition)
		if uriParam != nil && !isAbsolute(uriParam.value) {
			uriParam.value = prefixUrl(prefix, uriParam.value)
		}
	}
	for i := range m3u.tracks {
		track := &m3u.tracks[i]
		if !isAbsolute(track.url) {
			track.url = prefixUrl(prefix, track.url)
		}
	}
}

func (m3u *M3U) copy() M3U {
	m3uCopy := newM3U(uint32(len(m3u.segments)))

	m3uCopy.isMasterPlaylist = m3u.isMasterPlaylist

	for _, pair := range m3u.attributePairs {
		m3uCopy.addPair(pair)
	}

	if m3u.isMasterPlaylist {
		for _, track := range m3u.tracks {
			m3uCopy.addTrack(track)
		}
	} else {
		m3uCopy.isLive = m3u.isLive
		for _, seg := range m3u.segments {
			m3uCopy.addSegment(seg)
		}
	}

	return *m3uCopy
}

// This will only prefix URLs which are not fully qualified
// As well as map mapUri
func (m3u *M3U) prefixRelativeSegments(prefix string) {
	// if a range loop is used the track url is effectively not reassigned
	for i := range m3u.segments {
		segment := &m3u.segments[i]
		if !isAbsolute(segment.url) {
			segment.url = prefixUrl(prefix, segment.url)
		}
		if segment.mapUri != "" && !isAbsolute(segment.mapUri) {
			segment.mapUri = prefixUrl(prefix, segment.mapUri)
		}
		if segment.hasKey && !isAbsolute(segment.key.uri) {
			segment.key.uri = prefixUrl(prefix, segment.key.uri)
		}
	}
}

func (m3u *M3U) serialize(path string) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	if m3u.isMasterPlaylist {
		m3u.serializeMasterPlaylist(file)
	} else {
		m3u.serializePlaylist(file)
	}
}

func (m3u *M3U) serializeMasterPlaylist(file *os.File) {
	output := strings.Builder{}
	output.WriteString("#EXTM3U\n")

	for _, pair := range m3u.attributePairs {
		if pair.value == "" {
			output.WriteString("#" + pair.key + "\n")
		} else {
			output.WriteString("#" + pair.key + ":" + pair.value + "\n")
		}
	}
	output.WriteByte('\n')
	for i := range m3u.audioRenditions {
		output.WriteString("#EXT-X-MEDIA:")
		writeParams(&output, m3u.audioRenditions[i])
		output.WriteByte('\n')
	}
	for _, track := range m3u.tracks {
		output.WriteString("#EXT-X-STREAM-INF:")
		writeParams(&output, track.streamInfo)
		output.WriteString("\n" + track.url + "\n")
	}
	file.WriteString(output.String())
}

func writeParams(output *strings.Builder, params []Param) {
	length := len(params)
	for i := range length {
		output.WriteString(params[i].String())
		if i == length-1 {
			break
		}
		output.WriteString(",")
	}
}

func (m3u *M3U) serializePlaylist(file *os.File) {
	output := strings.Builder{}
	output.WriteString("#EXTM3U\n")

	for _, pair := range m3u.attributePairs {
		if pair.value == "" {
			output.WriteString("#" + pair.key + "\n")
		} else {
			output.WriteString("#" + pair.key + ":" + pair.value + "\n")
		}
	}

	for _, seg := range m3u.segments {
		if seg.mapUri != "" {
			output.WriteString("#EXT-X-MAP:URI=\"" + seg.mapUri + "\"\n")
		}
		// #EXT-X-KEY:METHOD=AES-128,URI="key4.json?f=1041&s=0&p=1822770&m=1506045858",IV=0x000000000000000000000000001BD032
		if seg.hasKey {
			output.WriteString("#EXT-X-KEY:")
			if seg.key.method != "" {
				output.WriteString("METHOD=" + seg.key.method + ",")
			}
			if seg.key.uri != "" {
				output.WriteString("URI=" + seg.key.uri + ",")
			}
			if seg.key.initVec != "" {
				output.WriteString("IV=" + seg.key.initVec + ",")
			}
			output.WriteString("\n")
		}
		for _, segPair := range seg.attributePairs {
			if segPair.value == "" {
				output.WriteString("#" + segPair.key + "\n")
			} else {
				output.WriteString("#" + segPair.key + ":" + segPair.value + "\n")
			}
		}
		extInf := fmt.Sprintf("#EXTINF:%v\n", seg.length)
		output.WriteString(extInf)
		output.WriteString(seg.url + "\n")
	}

	if !m3u.isLive {
		output.WriteString("#EXT-X-ENDLIST\n")
	}
	file.WriteString(output.String())
}

func downloadM3U(url string, filename string, referer string) (*M3U, error) {
	err := downloadFile(url, filename, referer, true)
	if err != nil {
		return nil, err
	}
	m3u, err := parseM3U(filename)
	if m3u != nil {
		m3u.url = url
	}
	return m3u, err
}
