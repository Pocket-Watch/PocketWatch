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

type Subtitle struct {
	Start, End Timecode
	Content    string
}

type Timecode struct {
	Hours, Minutes, Seconds, Milliseconds int
}

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
	for i := 0; i < pad; i++ {
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

func newSubtitle(start, end Timecode, content string) Subtitle {
	return Subtitle{start, end, content}
}

//  Each subtitle has four parts in the SRT file.
//    1. A numeric counter indicating the number or position of the subtitle.
//    2. Start and end time of the subtitle separated by –> characters
//    3. Subtitle text in one or more lines.
//    4. A blank line indicating the end of the subtitle.

func parseSRT(path string) ([]Subtitle, error) {

	file, err := os.Open(path)
	if err != nil {
		return make([]Subtitle, 0), err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	subtitles := make([]Subtitle, 0, 2048)

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
		sub := newSubtitle(start, end, content)
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

func serializeToVTT(subtitles []Subtitle, path string) error {
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
func (sub *Subtitle) shiftBy(ms int) {
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
	IsMovie bool   `json:"isMovie"`
	Title   string `json:"title"`
	Lang    string `json:"lang"`
	Year    string `json:"year"`
	Season  string `json:"season"`
	Episode string `json:"episode"`
}

const OUT_DIR = "../web/media/subs"

// TODO: Check if executable exists or was disabled at launch with a flag
func downloadSubtitle(executable string, search *Search) (string, error) {
	command := exec.Command(executable, search.Title, "--skip-select", "--to", "vtt", "--out", OUT_DIR)
	if search.Lang != "" {
		command.Args = append(command.Args, "--lang", search.Lang)
	}
	if search.Year != "" {
		command.Args = append(command.Args, "-y", search.Year)
	}
	if !search.IsMovie && search.Season != "" && search.Episode != "" {
		command.Args = append(command.Args, "-s", search.Season, "-e", search.Episode)
	}
	return executeSubtitleDownload(command)
}

func executeSubtitleDownload(command *exec.Cmd) (string, error) {
	stdout, err := command.Output()
	output := string(stdout)
	if err != nil {
		if output != "" {
			output = strings.Trim(output, "\n")
			return "", errors.Join(errors.New(output), err)
		}
		return "", errors.Join(err)
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	// Scan through the output line by line picking out the modified filename
	firstLine := ""
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine == "" {
			// This is needed to log the first error
			firstLine = line
		}
		if strings.HasPrefix(line, "New name:") {
			return line[len("New name:")+1:], nil
		}
	}
	return "", errors.New(firstLine)
}
