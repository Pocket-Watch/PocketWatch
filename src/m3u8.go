package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// https://datatracker.ietf.org/doc/html/rfc8216

const EXTM3U = "#EXTM3U"
const EXTINF = "#EXTINF"
const EXT_X_VERSION = "#EXT-X-VERSION"
const EXT_X_TARGETDURATION = "#EXT-X-TARGETDURATION"
const EXT_X_MEDIA_SEQUENCE = "#EXT-X-MEDIA-SEQUENCE"
const EXT_X_PLAYLIST_TYPE = "#EXT-X-PLAYLIST-TYPE"

const EXT_X_STREAM_INF = "#EXT-X-STREAM-INF"

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

const DEBUG = true

func parseM3U(path string) (*M3U, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	m3u := newM3U(1028)
	for scanner.Scan() {
		line := scanner.Text()

		// Media segment tag (4.3.2)
		if strings.HasPrefix(line, EXTINF) {
			duration, err := getExtInfDuration(line)
			if err != nil {
				continue
			}
			if !scanner.Scan() {
				return m3u, fmt.Errorf("unexpected EOF, expected URL after %v", EXTINF)
			}
			url := scanner.Text()
			segment := Segment{duration, url}
			m3u.addSegment(segment)
			continue
		}

		// Master playlist tag (4.3.4)
		if strings.HasPrefix(line, EXT_X_STREAM_INF) {
			m3u.isMasterPlaylist = true
			colon, err := valueAfterColon(line)
			if err != nil {
				continue
			}
			paramsLine := line[colon+1:]
			params := parseParams(paramsLine)
			if !scanner.Scan() {
				return m3u, fmt.Errorf("unexpected EOF, expected URL after %v", EXT_X_STREAM_INF)
			}
			url := scanner.Text()
			track := Track{params, url}
			m3u.tracks = append(m3u.tracks, track)
			continue
		}

		// Playlist tags (4.3.3)
		if strings.HasPrefix(line, "#EXT-X") {
			if strings.HasSuffix(line, "-ENDLIST") {
				break
			}
			parsePlaylistTag(line, m3u)
			continue
		}
	}

	return m3u, nil
}

func getExtInfDuration(ext_inf string) (float64, error) {
	end := len(ext_inf)
	if end <= 8 {
		return 0, fmt.Errorf("invalid %v", EXTINF)
	}
	comma := strings.Index(ext_inf, ",")
	if comma != -1 {
		end = comma
	}
	return strconv.ParseFloat(ext_inf[8:end], 64)
}

func valueAfterColon(line string) (int, error) {
	colon := strings.Index(line, ":")
	if colon == -1 {
		return -1, fmt.Errorf("error no colon in line: %v", line)
	}
	if colon == len(line)-1 {
		return -1, fmt.Errorf("error no value after colon in line: %v", line)
	}
	return colon, nil
}

func parsePlaylistTag(line string, m3u *M3U) {
	colon, err := valueAfterColon(line)
	if err != nil {
		return
	}
	if strings.HasPrefix(line, EXT_X_VERSION) {
		version, err := strconv.ParseFloat(line[colon+1:], 64)
		if err != nil {
			if DEBUG {
				fmt.Println("Error parsing version", line, err)
			}
			return
		}
		m3u.version = version
		return
	}
	if strings.HasPrefix(line, EXT_X_TARGETDURATION) {
		target_duration, err := strconv.ParseFloat(line[colon+1:], 64)
		if err != nil {
			if DEBUG {
				fmt.Println("Error parsing target duration", line, err)
			}
			return
		}
		m3u.targetDuration = target_duration
		return
	}

	if strings.HasPrefix(line, EXT_X_MEDIA_SEQUENCE) {
		media_sequence, err := strconv.ParseUint(line[colon+1:], 10, 32)
		if err != nil {
			if DEBUG {
				fmt.Println("Error parsing media sequence", line, err)
			}
			return
		}
		m3u.mediaSequence = uint32(media_sequence)
		return
	}
	if strings.HasPrefix(line, EXT_X_PLAYLIST_TYPE) {
		m3u.playlistType = line[colon+1:]
		return
	}

}

func parseParams(line string) []Param {
	params := make([]Param, 0)
	var key strings.Builder
	var value strings.Builder
	onKey := true
	inString := false

	for i := 0; i < len(line); i++ {
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

type M3U struct {
	isMasterPlaylist bool
	tracks           []Track
	// ^^^ tracks are exclusive to master playlists
	segments       []Segment // #EXTINF segments with URLs appearing in an ordered sequence
	version        float64
	targetDuration float64 // The maximum Media Segment duration in seconds
	mediaSequence  uint32  // The Media Sequence Number of the first Media Segment appearing in the playlist file
	playlistType   string
}

type Segment struct {
	length float64
	url    string
}

func (segment *Segment) getUrl(prefix string) string {
	if prefix == "" {
		return segment.url
	}
	if strings.HasSuffix(prefix, "/") || strings.HasPrefix(segment.url, "/") {
		return prefix + segment.url
	}
	return prefix + "/" + segment.url
}

func newM3U(capacity uint32) *M3U {
	m3u := new(M3U)
	m3u.segments = make([]Segment, 0, capacity)
	m3u.tracks = make([]Track, 0)
	return m3u
}

func (m3u *M3U) addSegment(seg Segment) {
	m3u.segments = append(m3u.segments, seg)
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

func (m3u *M3U) copy() M3U {
	m3uCopy := newM3U(uint32(len(m3u.segments)))

	m3uCopy.version = m3u.version
	m3uCopy.targetDuration = m3u.targetDuration
	m3uCopy.playlistType = m3u.playlistType
	m3uCopy.mediaSequence = m3u.mediaSequence

	for _, seg := range m3u.segments {
		m3uCopy.addSegment(seg)
	}
	return *m3uCopy
}

func (m3u *M3U) prefixSegments(urlPrefix string) {
	// if a range loop is used the track url is effectively not reassigned
	for i := 0; i < len(m3u.segments); i++ {
		fullUrl := m3u.segments[i].getUrl(urlPrefix)
		m3u.segments[i].url = fullUrl
	}
}

func (m3u *M3U) serialize(path string) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	file.WriteString("#EXTM3U\n")
	file.WriteString(fmt.Sprintf("#EXT-X-VERSION:%v\n", m3u.version))
	file.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%v\n", m3u.targetDuration))
	file.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%v\n", m3u.mediaSequence))
	file.WriteString(fmt.Sprintf("#EXT-X-PLAYLIST-TYPE:%v\n", m3u.playlistType))
	for _, track := range m3u.segments {
		_, err := file.WriteString(fmt.Sprintf("#EXTINF:%v,\n", track.length))
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
	file.WriteString(fmt.Sprintf("#EXT-X-ENDLIST\n"))
}

func downloadM3U(url string, filename string, referer string) (*M3U, error) {
	err := downloadFile(url, filename, referer)
	if err != nil {
		return nil, err
	}
	return parseM3U(filename)
}
