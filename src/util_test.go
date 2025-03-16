package main

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Range: <unit>=<range-start>-
// Range: <unit>=<range-start>-<range-end>
// Range: <unit>=<range-start>-<range-end>, <range-start>-<range-end>, …
// Range: <unit>=-<suffix-length>

func TestEmptyHeader(t *testing.T) {
	header := ""
	_, err := parseRangeHeader(header, 1929129)
	if err != nil {
		// the error is some, that's expected
		return
	}
	t.Errorf("Header should be invalid - %v", err)
}

func TestSingleFullRange(t *testing.T) {
	header := "bytes=0-12444"
	r, err := parseRangeHeader(header, 5996969)
	if err != nil || r == nil {
		t.Errorf("Expected success")
		return
	}
	if r.start == 0 && r.end == 12444 {
		// a valid range is expected
		return
	}
	t.Errorf("Header should be valid - %v", err)
}

func TestIgnoreMultipleRanges(t *testing.T) {
	header := "bytes=0-12444, 15000-29123"
	r, err := parseRangeHeader(header, 20000)
	if err != nil || r == nil {
		t.Errorf("Expected success")
		return
	}
	if r.start == 0 && r.end == 12444 {
		// a valid range is expected
		return
	}
	t.Errorf("Header should be valid %v", err)
}

func TestRangeWithoutEndValue(t *testing.T) {
	contentLength := int64(2000)
	header := "bytes=999-"
	r, err := parseRangeHeader(header, contentLength)
	if err != nil || r == nil {
		t.Errorf("Expected success")
		return
	}
	if r.start == 999 && r.end == contentLength-1 {
		// a valid range is expected
		return
	}
	t.Errorf("Header should be valid %v", err)
}

func TestDecreasingRange(t *testing.T) {
	header := "bytes=999-122"
	r, err := parseRangeHeader(header, 9999)
	if err == nil || r == nil {
		// no error is expected but the range should be nil
		return
	}
	t.Errorf("Invalid range %v", err)
}

func TestOnlySuffix(t *testing.T) {
	contentLength := int64(1000)
	header := "bytes=-122"
	r, err := parseRangeHeader(header, contentLength)
	if err != nil {
		t.Errorf("Expected success %v", err)
		return
	}
	if r == nil {
		t.Errorf("Expected valid range")
		return
	}
	if r.start == 878 && r.end == 999 {
		return
	}
	t.Errorf("Invalid range boundaries %v", r)
}

func TestNoEnd(t *testing.T) {
	header := "bytes=0-"
	r, err := parseRangeHeader(header, 738876331)
	if err != nil {
		t.Errorf("Expected success %v", err)
		return
	}
	if r == nil {
		t.Errorf("Expected valid range")
		return
	}
	if r.start == 0 && r.end == 738876330 {
		return
	}
}

// Path sanitization - path.Join calls path.Clean
func TestPathTraversal(t *testing.T) {
	// path.Clean does not simplify \..
	_, isSafe := safeJoin("win", "dow", "\\..")
	if isSafe {
		t.Errorf("Path is unsafe")
		return
	}
}

func TestSuccessfulJoin(t *testing.T) {
	sep := getPathSeparator()
	path, isSafe := safeJoin("abc/", "/123/", "/45")
	if !isSafe {
		t.Errorf("Path should be safe!")
		return
	}
	given := strings.Split(path, sep)
	expected := []string{"abc", "123", "45"}
	if !slices.Equal(given, expected) {
		t.Errorf("Path is %v, different from expected %v", given, expected)
		return
	}
}

func TestSafeDoubleDot(t *testing.T) {
	sep := getPathSeparator()
	path, isSafe := safeJoin("Really...", "/45")
	if !isSafe {
		t.Errorf("Path should be safe!")
		return
	}
	expected := "Really..." + sep + "45"
	if path != expected {
		t.Errorf("Path %v is different from expected %v", path, expected)
		return
	}
}

func TestSuspiciousYetSafePath(t *testing.T) {
	input := ".../.../"
	_, isSafe := safeJoin(input)
	if !isSafe {
		t.Errorf("Path %v should be safe!", input)
		return
	}
}

func TestRepeatedDots(t *testing.T) {
	input := ".../../"
	_, isSafe := safeJoin(".../../")
	if isSafe {
		t.Errorf("Path %v is not safe!", input)
		return
	}
}

func TestMixedTraversal(t *testing.T) {
	input := "abc/rly../..\\bin"
	_, isSafe := safeJoin(input)
	if isSafe {
		t.Errorf("Path %v is not safe!", input)
		return
	}
}

func TestManySafeDots(t *testing.T) {
	input := "./.\\ABC ../DEF/.../bin"
	joined, isSafe := safeJoin(input)
	if !isSafe {
		t.Errorf("Path %v is safe!", input)
		return
	}
	sep := getPathSeparator()
	given := strings.Split(joined, sep)
	expected := []string{"ABC ..", "DEF", "...", "bin"}
	if !slices.Equal(given, expected) {
		t.Errorf("Path is %v, different from expected %v", given, expected)
		return
	}
}

func getPathSeparator() string {
	sep := "/"
	if runtime.GOOS == "windows" {
		sep = "\\"
	}
	return sep
}
