package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type SubtitleCue struct {
	Start, End Timecode
	Content    string
}

type Timecode struct {
	Hours, Minutes, Seconds, Milliseconds int
}

var END_TIMECODE = Timecode{99, 59, 59, 999}
var ZERO_TIMECODE = Timecode{0, 0, 0, 0}
var INVALID_TIMECODE = Timecode{-1, -1, -1, -1}

func (timecode *Timecode) isNegative() bool {
	return timecode.Hours < 0 || timecode.Minutes < 0 || timecode.Seconds < 0 || timecode.Milliseconds < 0
}

func (timecode *Timecode) ToSrt() string {
	return formatUnit(timecode.Hours, 2) + ":" +
		formatUnit(timecode.Minutes, 2) + ":" +
		formatUnit(timecode.Seconds, 2) + "," +
		formatUnit(timecode.Milliseconds, 3)
}

func (timecode *Timecode) toVtt() string {
	hourFormat := ""
	if timecode.Hours > 0 {
		hourFormat = formatUnit(timecode.Hours, 2) + ":"
	}
	return hourFormat +
		formatUnit(timecode.Minutes, 2) + ":" +
		formatUnit(timecode.Seconds, 2) + "." +
		formatUnit(timecode.Milliseconds, 3)
}

func formatUnit(value int, length int) string {
	var format strings.Builder
	formattedValue := strconv.Itoa(value)
	pad := length - len(formattedValue)
	for range pad {
		format.WriteByte('0')
	}
	format.WriteString(formattedValue)
	return format.String()
}

// Expects a timestamp in the following format:
// hr:min:sec,ms (00:00:00,000)
func fromSrtTimestamp(timestamp string) (Timecode, error) {
	split := strings.Split(timestamp, ":")
	if len(split) != 3 {
		return INVALID_TIMECODE, errors.New("invalid timestamp, not a triple split")
	}
	hours, err := strconv.Atoi(split[0])
	if err != nil {
		return INVALID_TIMECODE, err
	}
	minutes, err := strconv.Atoi(split[1])
	if err != nil {
		return INVALID_TIMECODE, err
	}
	subSplit := strings.Split(split[2], ",")
	if len(subSplit) != 2 {
		return INVALID_TIMECODE, errors.New("invalid sub stamp, not a double split")
	}
	seconds, err := strconv.Atoi(subSplit[0])
	if err != nil {
		return INVALID_TIMECODE, err
	}
	milliseconds, err := strconv.Atoi(subSplit[1])
	if err != nil {
		return INVALID_TIMECODE, err
	}
	return Timecode{hours, minutes, seconds, milliseconds}, nil
}

func newSubtitleCue(start, end Timecode, content string) SubtitleCue {
	return SubtitleCue{start, end, content}
}

//  Each subtitle has four parts in the SRT file.
//    1. A numeric counter indicating the number or position of the subtitle.
//    2. Start and end time of the subtitle separated by –> characters
//    3. Subtitle text in one or more lines.
//    4. A blank line indicating the end of the subtitle.

func parseSRT(path string) ([]SubtitleCue, error) {

	file, err := os.Open(path)
	if err != nil {
		return make([]SubtitleCue, 0), err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	subtitles := make([]SubtitleCue, 0, 2048)

	for scanner.Scan() {
		counter := scanner.Text()
		if _, err := strconv.Atoi(counter); err != nil {
			break
		}
		if !scanner.Scan() {
			return subtitles, errors.New("expected timestamps [start --> end]")
		}
		timestamps := scanner.Text()
		start, end, err := parseTimestamps(timestamps)
		if err != nil {
			return subtitles, err
		}
		var content string = parseContent(scanner)
		sub := newSubtitleCue(start, end, content)
		subtitles = append(subtitles, sub)
	}
	return subtitles, nil
}

var LRC_TAGS = []string{"ti", "ar", "al", "au", "lr", "length", "by", "offset", "re", "tool", "ve"}

func parseLRC(path string) ([]SubtitleCue, error) {
	file, err := os.Open(path)
	if err != nil {
		return make([]SubtitleCue, 0), err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	subtitles := make([]SubtitleCue, 0, 256)

	firstCue := true
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '[' {
			continue
		}

		lastIndex := len(line) - 1
		if line[lastIndex] == ']' {
			isTag := false
			for _, tag := range LRC_TAGS {
				if strings.HasPrefix(line[1:lastIndex], tag) {
					isTag = true
					break
				}
			}
			if isTag {
				continue
			}
		}

		if len(line) < 11 {
			// Silently skip empty cues
			continue
		}

		minutes, err := strconv.Atoi(line[1:3])
		if err != nil {
			return subtitles, err
		}
		seconds, err := strconv.Atoi(line[4:6])
		if err != nil {
			return subtitles, err
		}
		hundredths, err := strconv.Atoi(line[7:9])
		if err != nil {
			return subtitles, err
		}

		hours := minutes / 60
		minutes %= 60
		start := Timecode{hours, minutes, seconds, hundredths * 10}
		content := line[10:]
		if content[0] == ' ' {
			content = content[1:]
		}

		sub := newSubtitleCue(start, END_TIMECODE, content)
		if firstCue {
			firstCue = false
			subtitles = append(subtitles, sub)
			continue
		}
		lastSub := subtitles[len(subtitles)-1]
		lastSub.End = sub.Start
		subtitles = append(subtitles, sub)
	}
	return subtitles, nil
}

func parseContent(scanner *bufio.Scanner) string {
	// Parse sub text (may span over one or more lines)
	var content strings.Builder
	if scanner.Scan() {
		firstLine := scanner.Text()
		if firstLine != "" {
			content.WriteString(firstLine)
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		content.WriteString("\n")
		content.WriteString(line)
	}
	return content.String()
}

func parseTimestamps(timestamps string) (Timecode, Timecode, error) {
	if len(timestamps) < 29 {
		return INVALID_TIMECODE, INVALID_TIMECODE, errors.New("the timestamps are not full")
	}
	separator := strings.Index(timestamps, "-->")
	if separator == -1 {
		return INVALID_TIMECODE, INVALID_TIMECODE, errors.New("no timecode separator found")
	}
	startStamp := timestamps[0:12]
	endStamp := timestamps[17:29]

	start, err := fromSrtTimestamp(startStamp)
	if err != nil {
		return INVALID_TIMECODE, INVALID_TIMECODE, err
	}
	end, err := fromSrtTimestamp(endStamp)
	if err != nil {
		return start, INVALID_TIMECODE, err
	}
	return start, end, nil
}

func serializeToVTT(subtitles []SubtitleCue, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("WEBVTT\n\n")

	for _, sub := range subtitles {
		if sub.Start.isNegative() {
			if sub.End.isNegative() {
				continue
			}
			// it's possible to preserve the subtitle if the end stamp is non-negative
			sub.Start = ZERO_TIMECODE
		}
		timestamps := fmt.Sprintf("%s --> %s\n", sub.Start.toVtt(), sub.End.toVtt())
		file.WriteString(timestamps)
		file.WriteString(sub.Content)
		file.WriteString("\n\n")
	}
	return nil
}

// negative / positive
func (sub *SubtitleCue) shiftBy(ms int) {
	if ms > 0 {
		sub.Start.shiftForwardBy(ms)
		sub.End.shiftForwardBy(ms)
	} else if ms < 0 {
		ms = -ms
		sub.Start.shiftBackBy(ms)
		sub.End.shiftBackBy(ms)
	}
}

// expects a positive ms value despite shifting back
func (timecode *Timecode) shiftBackBy(ms int) {
	timecode.Milliseconds -= ms

	secondsOffset := timecode.Milliseconds / 1000
	if timecode.Milliseconds < 0 {
		timecode.Milliseconds %= 1000
		if timecode.Milliseconds < 0 {
			timecode.Milliseconds += 1000
			secondsOffset -= 1
		}
	}
	timecode.Seconds += secondsOffset

	minutesOffset := timecode.Seconds / 60
	if timecode.Seconds < 0 {
		timecode.Seconds %= 60
		if timecode.Seconds < 0 {
			timecode.Seconds += 60
			minutesOffset -= 1
		}
	}
	timecode.Minutes += minutesOffset

	hoursOffset := timecode.Minutes / 60
	if timecode.Minutes < 0 {
		timecode.Minutes %= 60
		if timecode.Minutes < 0 {
			timecode.Minutes += 60
			hoursOffset -= 1
		}
	}
	timecode.Hours += hoursOffset
	// if hours are negative then the time code will not be serialized
}

func (timecode *Timecode) shiftForwardBy(ms int) {
	timecode.Milliseconds = timecode.Milliseconds + ms
	// 01:59:900 + 00:00:3500 = 01:59:4400

	additionalSeconds := timecode.Milliseconds / 1000
	if timecode.Milliseconds >= 1000 {
		timecode.Milliseconds = timecode.Milliseconds % 1000
	}
	timecode.Seconds += additionalSeconds
	// 01:63:400

	additionalMinutes := timecode.Seconds / 60
	if timecode.Seconds >= 60 {
		timecode.Seconds = timecode.Seconds % 60
	}
	timecode.Minutes += additionalMinutes
	// 02:03:4400

	additionalHours := timecode.Minutes / 60
	if timecode.Minutes >= 60 {
		timecode.Minutes = timecode.Minutes % 60
	}
	timecode.Hours += additionalHours

	// cannot mod hours because it cannot be carried over to a higher unit
}

type Search struct {
	Title   string `json:"title"`
	Lang    string `json:"lang"`
	Year    string `json:"year"`
	Season  string `json:"season"`
	Episode string `json:"episode"`
}

// TODO: Check if executable exists or was disabled at launch with a flag
func downloadSubtitle(search *Search, outputDirectory string) (string, error) {
	command := exec.Command("subs", search.Title, "--auto-select", "--to", "vtt", "--out", outputDirectory)

	if search.Lang != "" {
		command.Args = append(command.Args, "--lang", search.Lang)
	}

	if search.Year != "" {
		command.Args = append(command.Args, "-y", search.Year)
	}

	if search.Season != "" && search.Episode != "" {
		command.Args = append(command.Args, "-s", search.Season, "-e", search.Episode)
	}

	stdout, err := command.Output()
	if err != nil {
		return "", errors.Join(err)
	}

	output := string(stdout)
	scanner := bufio.NewScanner(strings.NewReader(output))

	// Scan through the output line by line picking out the modified filename
	firstLine := ""
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine == "" {
			// This is needed to log the first error
			firstLine = line
		}

		if strings.HasPrefix(line, "Saved to ") {
			return line[len("Saved to "):], nil
		}
	}

	return "", errors.New(firstLine)

}
