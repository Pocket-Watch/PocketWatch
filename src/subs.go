package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
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

var INVALID_TIMECODE = Timecode{-1, -1, -1, -1}

func (timecode *Timecode) ToSrt() string {
	return formatUnit(timecode.Hours, 2) + ":" +
		formatUnit(timecode.Minutes, 2) + ":" +
		formatUnit(timecode.Seconds, 2) + "," +
		formatUnit(timecode.Milliseconds, 3)
}

func (timecode *Timecode) toVtt() string {
	return formatUnit(timecode.Hours, 2) + ":" +
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
		_ = scanner.Text()
		if !scanner.Scan() {
			return subtitles, errors.New("expected timestamps [start --> end]")
		}
		timestamps := scanner.Text()
		start, end, err := parseTimestamps(timestamps)
		if err != nil {
			return subtitles, err
		}
		// Parse sub text (may span over one or more lines)
		var content strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				break
			}
			content.WriteString(line)
			content.WriteString("\n")
		}
		sub := newSubtitle(start, end, content.String())
		subtitles = append(subtitles, sub)
	}
	return subtitles, nil
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
		timestamps := fmt.Sprintf("%s --> %s\n", sub.Start.toVtt(), sub.End.toVtt())
		file.WriteString(timestamps)
		file.WriteString(sub.Content)
		file.WriteString("\n\n")
	}
	return nil
}

func (sub *Subtitle) shiftForwardBy(ms int) {
	sub.Start.shiftForwardBy(ms)
	sub.End.shiftForwardBy(ms)
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
