package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func detectM3U(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() && scanner.Text() == "#EXTM3U" {
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
		if strings.HasPrefix(line, "#EXTINF") {
			duration, err := getExtInfDuration(line)
			if err != nil {
				continue
			}
			if !scanner.Scan() {
				fmt.Println("Unexpected EOF, expected URL after #EXTINF")
				return m3u, scanner.Err()
			}
			url := scanner.Text()
			track := Track{duration, url}
			m3u.addTrack(track)
			continue
		}

		if strings.HasPrefix(line, "#EXT-X") {
			if strings.HasSuffix(line, "-ENDLIST") {
				break
			}
			parseManifestLine(line, m3u)
			continue
		}

	}

	return m3u, nil
}

func getExtInfDuration(ext_inf string) (float64, error) {
	end := len(ext_inf)
	if end <= 8 {
		return 0, fmt.Errorf("invalid #EXTINF")
	}
	comma := strings.Index(ext_inf, ",")
	if comma != -1 {
		end = comma
	}
	return strconv.ParseFloat(ext_inf[8:end], 64)
}

func parseManifestLine(line string, m3u *M3U) {
	colon := strings.Index(line, ":")
	if colon == -1 {
		fmt.Println("Error no colon in line:", line)
		return
	}
	if colon == len(line)-1 {
		fmt.Println("Error no value after colon in line:", line)
		return
	}
	if strings.HasPrefix(line, "#EXT-X-VERSION") {
		version, err := strconv.ParseFloat(line[colon+1:], 64)
		if err != nil {
			if DEBUG {
				fmt.Println("Error parsing version", line, err)
			}
			return
		}
		m3u.ext_x_version = version
		return
	}
	if strings.HasPrefix(line, "#EXT-X-TARGETDURATION") {
		target_duration, err := strconv.ParseFloat(line[colon+1:], 64)
		if err != nil {
			if DEBUG {
				fmt.Println("Error parsing target duration", line, err)
			}
			return
		}
		m3u.ext_x_target_duration = target_duration
		return
	}
	if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE") {
		media_sequence, err := strconv.ParseUint(line[colon+1:], 10, 32)
		if err != nil {
			if DEBUG {
				fmt.Println("Error parsing media sequence", line, err)
			}
			return
		}
		m3u.ext_x_media_sequence = uint32(media_sequence)
		return
	}
	if strings.HasPrefix(line, "#EXT-X-PLAYLIST-TYPE") {
		m3u.ext_x_playlist_type = line[colon+1:]
		return
	}
}

type M3U struct {
	tracks                []Track // #EXTINF tracks with URLs appearing in an ordered sequence
	ext_x_version         float64
	ext_x_target_duration float64 // The maximum Media Segment duration in seconds
	ext_x_media_sequence  uint32  // The Media Sequence Number of the first Media Segment appearing in the playlist file
	ext_x_playlist_type   string
}

type Track struct {
	length float64
	url    string
}

func (track *Track) getUrl(prefix string) string {
	if prefix == "" {
		return track.url
	}
	if strings.HasSuffix(prefix, "/") || strings.HasPrefix(track.url, "/") {
		return prefix + track.url
	}
	return prefix + "/" + track.url
}

func newM3U(capacity uint32) *M3U {
	m3u := new(M3U)
	m3u.tracks = make([]Track, 0, capacity)
	return m3u
}

func (m3u *M3U) addTrack(track Track) {
	m3u.tracks = append(m3u.tracks, track)
}

func (m3u *M3U) avgTrackLength() float64 {
	var sum float64
	for _, track := range m3u.tracks {
		sum += track.length
	}
	return sum / float64(len(m3u.tracks))
}

func (m3u *M3U) copy() M3U {
	m3uCopy := newM3U(uint32(len(m3u.tracks)))

	m3uCopy.ext_x_version = m3u.ext_x_version
	m3uCopy.ext_x_target_duration = m3u.ext_x_target_duration
	m3uCopy.ext_x_playlist_type = m3u.ext_x_playlist_type
	m3uCopy.ext_x_media_sequence = m3u.ext_x_media_sequence

	for _, track := range m3u.tracks {
		m3uCopy.addTrack(track)
	}
	return *m3uCopy
}

func stripSuffix(url string) string {
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash == -1 {
		// this could be more robust
		return url
	}
	return url[:lastSlash]
}

func (m3u *M3U) prefixTracks(urlPrefix string) {
	// if a range loop is used the track url is effectively not reassigned
	for i := 0; i < len(m3u.tracks); i++ {
		fullUrl := m3u.tracks[i].getUrl(urlPrefix)
		m3u.tracks[i].url = fullUrl
	}
}

func (m3u *M3U) serialize(path string) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	file.WriteString("#EXTM3U\n")
	file.WriteString(fmt.Sprintf("#EXT-X-VERSION:%v\n", m3u.ext_x_version))
	file.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%v\n", m3u.ext_x_target_duration))
	file.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%v\n", m3u.ext_x_media_sequence))
	file.WriteString(fmt.Sprintf("#EXT-X-PLAYLIST-TYPE:%v\n", m3u.ext_x_playlist_type))
	for _, track := range m3u.tracks {
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
