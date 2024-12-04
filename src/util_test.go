package main

import (
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
