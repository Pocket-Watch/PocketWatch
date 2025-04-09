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

const (
	EXTM3U               = "EXTM3U"
	EXT_X_VERSION        = "EXT-X-VERSION"
	EXT_X_TARGETDURATION = "EXT-X-TARGETDURATION"
	EXT_X_PLAYLIST_TYPE  = "EXT-X-PLAYLIST-TYPE"
	EXT_X_MEDIA_SEQUENCE = "EXT-X-MEDIA-SEQUENCE"
	EXT_X_MEDIA          = "EXT-X-MEDIA"
	EXT_X_DISCONTINUITY  = "EXT-X-DISCONTINUITY"
	EXT_X_BYTERANGE      = "EXT-X-BYTERANGE"
	EXT_X_ALLOW_CACHE    = "EXT-X-ALLOW-CACHE"
	EXT_X_START          = "EXT-X-START"
	EXT_X_SESSION_DATA   = "EXT-X-SESSION-DATA"
	EXT_X_SESSION_KEY    = "EXT-X-SESSION-KEY"
	EXT_X_KEY            = "EXT-X-KEY"
	EXT_X_PREFETCH       = "EXT-X-PREFETCH"
)

var GENERAL_ATTRIBUTES = []string{
	EXTM3U,
	EXT_X_VERSION,
	EXT_X_PLAYLIST_TYPE,
	EXT_X_TARGETDURATION,
	EXT_X_MEDIA,
	EXT_X_DISCONTINUITY,
	EXT_X_BYTERANGE,
	EXT_X_MEDIA_SEQUENCE,
	EXT_X_ALLOW_CACHE,
	EXT_X_START,
	EXT_X_SESSION_DATA,
	EXT_X_SESSION_KEY,
	EXT_X_KEY,
	EXT_X_PREFETCH,
}

// EXT_X_ENDLIST Media playlist tag (4.3.3.4)
// This tag helps determine whether the list is a live playlist
const EXT_X_ENDLIST = "EXT-X-ENDLIST"

// EXT_X_STREAM_INF Master playlist tag (4.3.4) - track only
const EXT_X_STREAM_INF = "EXT-X-STREAM-INF"

// EXTINF Media segment tag (4.3.2)
const EXTINF = "EXTINF"

const EXT_X_PROGRAM_DATE_TIME = "EXT-X-PROGRAM-DATE-TIME"

func detectM3U(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() && strings.HasPrefix(scanner.Text(), EXTM3U) {
		return true, nil
	}
	return false, nil
}

type KeyValuePair struct {
	key   string
	value string
}

// This function assumes the pair starts with #
// A key is required for it to produce a valid result, value can be empty
func getKeyValue(line string) *KeyValuePair {
	line = line[1:]
	// The shortest tag is 6 characters long (eg. EXTINF)
	if len(line) < 6 {
		return nil
	}
	colon := strings.Index(line, ":")
	if colon == -1 || colon == len(line)-1 {
		return &KeyValuePair{line, ""}
	}
	if colon < 6 {
		return nil
	}
	key := line[:colon]
	value := line[colon+1:]
	return &KeyValuePair{key, value}
}

func parseM3U(path string) (*M3U, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hasEnd := false
	scanner := bufio.NewScanner(file)
	m3u := newM3U(1000)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && line[0] == '#' {
			pair := getKeyValue(line)
			if pair == nil {
				// Skip the tag, invalid pair
				continue
			}
			if pair.key == EXTINF {
				duration, err := parseFirstFloat(pair.value)
				if err != nil {
					continue
				}
				if !scanner.Scan() {
					return m3u, fmt.Errorf("unexpected EOF, expected URL after %v", EXTINF)
				}
				url := scanner.Text()
				segment := Segment{nil, duration, url}
				m3u.addSegment(segment)
				continue
			}

			if !strings.HasPrefix(pair.key, "EXT-X") {
				continue
			}

			if pair.key == EXT_X_PROGRAM_DATE_TIME {
				// TODO
				continue
			}

			if pair.key == EXT_X_STREAM_INF {
				m3u.isMasterPlaylist = true
				params := parseParams(pair.value)
				if !scanner.Scan() {
					return m3u, fmt.Errorf("unexpected EOF, expected URL after %v", EXT_X_STREAM_INF)
				}
				url := scanner.Text()
				track := Track{params, url}
				m3u.addTrack(track)
				continue
			}
			if pair.key == EXT_X_ENDLIST {
				hasEnd = true
				// It MAY occur anywhere in the Media Playlist file.
				continue
			}

			if slices.Contains(GENERAL_ATTRIBUTES, pair.key) {
				m3u.addGeneralAttribute(*pair)
				continue
			}
			// LogWarn("Unrecognized pair %v:%v", pair.key, pair.value)
			continue
		}
		// TODO: Adjust flow to parse EXT-X-PROGRAM-DATE-TIME along with segment
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

// Track - Variant Stream (represents a m3u8 entry along with its metadata in a master playlist)
type Track struct {
	// #EXT-X-STREAM-INF info about m3u8 playlists
	streamInfo []Param
	url        string
}

// internally modifies track
func (track *Track) prefixUrl(prefix string) {
	if prefix == "" {
		return
	}
	if strings.HasSuffix(prefix, "/") || strings.HasPrefix(track.url, "/") {
		track.url = prefix + track.url
		return
	}
	track.url = prefix + "/" + track.url
}

type M3U struct {
	isMasterPlaylist bool
	isLive           bool
	tracks           []Track
	// ^^^ tracks are exclusive to master playlists
	attributePairs []KeyValuePair // key:value properties which describe the playlist
	segments       []Segment      // #EXTINF segments with URLs appearing in an ordered sequence
}

type Segment struct {
	dateTime *string
	length   float64
	url      string
}

// internally modify segment
func (segment *Segment) prefixUrl(prefix string) {
	if prefix == "" {
		return
	}
	if strings.HasSuffix(prefix, "/") || strings.HasPrefix(segment.url, "/") {
		segment.url = prefix + segment.url
		return
	}
	segment.url = prefix + "/" + segment.url
}

func newM3U(segmentCapacity uint32) *M3U {
	m3u := new(M3U)
	m3u.segments = make([]Segment, 0, segmentCapacity)
	m3u.tracks = make([]Track, 0)
	return m3u
}

// Returns the value associated with the given key or "" if key is missing
func (m3u *M3U) getAttribute(key string) string {
	length := len(m3u.attributePairs)
	for i := range length {
		pair := &m3u.attributePairs[i]
		if pair.key == key {
			return pair.value
		}
	}
	return ""
}

func (m3u *M3U) addGeneralAttribute(pair KeyValuePair) {
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

// Fetches highest resolution from m3u.tracks
// this method should only be used if the m3u is a master playlist
func (m3u *M3U) getBestTrack() *Track {
	var bestTrack *Track = nil
	var bestWidth int64 = 0
	for _, track := range m3u.tracks {
		for _, param := range track.streamInfo {
			if param.key != "RESOLUTION" {
				continue
			}
			res := strings.ToLower(param.value)
			x := strings.Index(res, "x")
			if x == -1 {
				continue
			}
			vidWidth, err := strconv.ParseInt(res[:x], 10, 32)
			if err != nil {
				continue
			}
			if vidWidth > bestWidth {
				bestWidth = vidWidth
				bestTrack = &track
			}
		}
	}
	return bestTrack
}

// This method will only prefix relative URLs
func (m3u *M3U) prefixRelativeTracks(prefix string) {
	for i := range m3u.tracks {
		if !strings.HasPrefix(m3u.tracks[i].url, "http") {
			m3u.tracks[i].prefixUrl(prefix)
		}
	}
}

func (m3u *M3U) copy() M3U {
	m3uCopy := newM3U(uint32(len(m3u.segments)))

	m3uCopy.isMasterPlaylist = m3u.isMasterPlaylist

	if m3u.isMasterPlaylist {
		for _, track := range m3u.tracks {
			m3uCopy.addTrack(track)
		}
	} else {
		m3uCopy.isLive = m3u.isLive
		for _, pair := range m3u.attributePairs {
			m3uCopy.addGeneralAttribute(pair)
		}
		for _, seg := range m3u.segments {
			m3uCopy.addSegment(seg)
		}
	}

	return *m3uCopy
}

// This will only prefix URLs which are not fully qualified
func (m3u *M3U) prefixRelativeSegments(prefix string) {
	// if a range loop is used the track url is effectively not reassigned
	for i := range m3u.segments {
		if !strings.HasPrefix(m3u.segments[i].url, "http") {
			m3u.segments[i].prefixUrl(prefix)
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
	file.WriteString("#EXTM3U\n")

	for _, track := range m3u.tracks {
		infoLine := strings.Builder{}
		infoLine.WriteString("#EXT-X-STREAM-INF:")
		for i, param := range track.streamInfo {
			infoLine.WriteString(fmt.Sprintf("%v=%v,", param.key, param.value))
			if i == len(track.streamInfo)-1 {
				break
			}
			infoLine.WriteString(",")
		}
		infoLine.WriteString("\n")
		_, err := file.WriteString(infoLine.String())
		if err != nil {
			fmt.Println(err)
			continue
		}
		_, err2 := file.WriteString(track.url + "\n")
		if err2 != nil {
			fmt.Println(err2)
			continue
		}
	}
}

func (m3u *M3U) serializePlaylist(file *os.File) {
	file.WriteString("#EXTM3U\n")
	pairBuffer := strings.Builder{}
	for _, pair := range m3u.attributePairs {
		pairBuffer.WriteString("#")
		pairBuffer.WriteString(pair.key)
		pairBuffer.WriteString(":")
		pairBuffer.WriteString(pair.value)
		pairBuffer.WriteString("\n")
	}
	_, err := file.WriteString(pairBuffer.String())
	for _, seg := range m3u.segments {
		_, err = file.WriteString(fmt.Sprintf("#EXTINF:%v,\n", seg.length))
		if err != nil {
			fmt.Println(err)
			continue
		}
		_, err = file.WriteString(seg.url + "\n")
		if err != nil {
			fmt.Println(err)
			continue
		}
	}
	if !m3u.isLive {
		file.WriteString(fmt.Sprintf("#EXT-X-ENDLIST\n"))
	}
}

func downloadM3U(url string, filename string, referer string) (*M3U, error) {
	err := downloadFile(url, filename, referer)
	if err != nil {
		return nil, err
	}
	return parseM3U(filename)
}
