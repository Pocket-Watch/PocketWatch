package main

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
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

func TestShortPaths(t *testing.T) {
	// path.Clean does not simplify \..
	_, isSafe := safeJoin("http\\..\\admin")
	if isSafe {
		t.Errorf("Path is unsafe")
		return
	}
	_, isSafe = safeJoin("\\..\\")
	if isSafe {
		t.Errorf("Path is unsafe")
		return
	}

	_, isSafe = safeJoin("../")
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

func TestUnicodeTraversal(t *testing.T) {
	input := "abc/..日"
	_, isSafe := safeJoin(input)
	if !isSafe {
		t.Errorf("Path %v should be safe!", input)
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

// Rate limiter tests
func TestRateLimiter(t *testing.T) {
	hits := 20
	rateLimiter := NewLimiter(hits, 1)
	for h := range hits {
		if rateLimiter.block() {
			t.Errorf("Limiter blocked when it shouldn't - at hit: %v", h)
			return
		}
	}
	// Block subsequent calls
	for h := range hits {
		if rateLimiter.block() {
			continue
		}
		t.Errorf("Limiter passed when it shouldn't - at hit: %v", h)
		return
	}
	// Wait to unblock
	time.Sleep(1 * time.Second)
	for h := range hits {
		if rateLimiter.block() {
			t.Errorf("Limiter blocked when it shouldn't - at hit: %v", h)
		}
	}
}

// Ring buffer tests
func TestRingBufferPushAndLen(t *testing.T) {
	size := 5
	buffer := NewRingBuffer(size)
	if buffer.Len() != 0 {
		t.Errorf("Free buffer length is not 0")
		return
	}
	for i := range size {
		buffer.Push(0)
		expectedLen := i + 1
		if buffer.Len() != expectedLen {
			t.Errorf("Free buffer length is not %v", expectedLen)
			return
		}
	}
	if buffer.Push(int64(size + 1)) {
		t.Errorf("%vth push should have failed", size+1)
		return
	}
	if buffer.Len() != size {
		t.Errorf("Buffer length should still be %v. Actual: %v", size, buffer.Len())
		return
	}
}

func TestRingBufferFillAndDrain(t *testing.T) {
	size := 4
	buffer := NewRingBuffer(size)
	for i := range size {
		buffer.Push(int64(i))
	}

	for range size {
		buffer.PopEnd()
	}

	if buffer.Len() != 0 {
		t.Errorf("Buffer should be empty but length is %v", buffer.Len())
	}
}

func TestRingBufferPeekAndPop(t *testing.T) {
	size := 5
	buffer := NewRingBuffer(size)
	for i := range size {
		buffer.Push(int64(i))
	}

	for i := range size {
		last := buffer.PeekEnd()
		if last != int64(i) {
			t.Errorf("Peek end should be %v. Actual: %v", i, last)
			return
		}
		buffer.PopEnd()
	}
	buffer.Push(100)
	if buffer.PeekEnd() != 100 {
		t.Errorf("Peek end should be 100. Actual: %v", buffer.PeekEnd())
	}
}

func TestSingleElementRing(t *testing.T) {
	buffer := NewRingBuffer(1)
	for i := range 5 {
		buffer.Push(int64(i))
	}
	if buffer.PeekEnd() != 0 {
		t.Errorf("Peek end should be 0. Actual: %v", buffer.PeekEnd())
	}
	for range 5 {
		buffer.PopEnd()
	}
	buffer.Push(100)
	if buffer.PeekEnd() != 100 {
		t.Errorf("Peek end should be 100. Actual: %v", buffer.PeekEnd())
	}
}

func TestPushForceRing(t *testing.T) {
	buffer := NewRingBuffer(3)
	for i := range 3 {
		buffer.ForcePush(int64(i))
	}
	// 0, 1, 2
	if buffer.PeekEnd() != 0 {
		t.Errorf("Peek end should be 0. Actual: %v", buffer.PeekEnd())
	}
	buffer.ForcePush(3)
	buffer.ForcePush(4)
	buffer.ForcePush(5)
	if buffer.PeekEnd() != 3 {
		t.Errorf("Peek end should be 3. Actual: %v", buffer.PeekEnd())
	}
	buffer.PopEnd()
	if buffer.PeekEnd() != 4 {
		t.Errorf("Peek end should be 4. Actual: %v", buffer.PeekEnd())
	}
	buffer.ForcePush(1)
	if buffer.PeekEnd() != 4 {
		t.Errorf("Peek end should be 4. Actual: %v", buffer.PeekEnd())
	}
}

func TestValidASCIIString(t *testing.T) {
	input := "basic"
	if !validateName(input) {
		t.Errorf("%v should be a valid string", input)
	}
}

func TestValidVietnameseDiacritics(t *testing.T) {
	input := "៛ៜấី"
	if !validateName(input) {
		t.Errorf("%v should be a valid string", input)
	}
}

func TestValidJapanese(t *testing.T) {
	input := "日本語"
	if !validateName(input) {
		t.Errorf("%v should be a valid string", input)
	}
}

func TestZalgoTilde(t *testing.T) {
	input := "T̃"
	if validateName(input) {
		t.Errorf("%v should be invalid string", input)
	}
}

func TestLightZalgo(t *testing.T) {
	input := "café͂́"
	if validateName(input) {
		t.Errorf("%v should be invalid", input)
	}
}

func TestInsaneZalgoT(t *testing.T) {
	input := "T̃͟͏̧̟͓̯"
	if validateName(input) {
		t.Errorf("%v should be invalid", input)
	}
}

func TestInsaneZalgoU(t *testing.T) {
	input := "U̷̥̼͐̄̓̀̃̚͘"
	if validateName(input) {
		t.Errorf("%v should be invalid", input)
	}
}

func TestValidReservedAcuteAccents(t *testing.T) {
	input := "áéíóú"
	if !validateName(input) {
		t.Errorf("%v should be a valid string", input)
	}
}

func TestUBar(t *testing.T) {
	input := "ʉ"
	if !validateName(input) {
		t.Errorf("%v should be a valid string", input)
	}
}

func TestEmpty(t *testing.T) {
	input := "        "
	if validateName(input) {
		t.Errorf("%v should be an invalid string", input)
	}
}

func TestInvalidSeparators(t *testing.T) {
	input := "\n \t"
	if validateName(input) {
		t.Errorf("%v should be an invalid string", input)
	}
}

func TestWithSpaceOnly(t *testing.T) {
	input := "      n"
	if !validateName(input) {
		t.Errorf("%v should be a valid string", input)
	}
}

func TestIpV4Range_Contains_OnInvalidIp(t *testing.T) {
	start := "152.61.11.10"
	end := "152.61.11.20"
	ipRange := newIpV4Range(start, end)
	if ipRange == nil {
		t.Errorf("%v -> %v range should be valid", start, end)
		return
	}
	if ipRange.Contains("121.21") {
		t.Errorf("%v -> %v range shouldn't contain invalid IP", start, end)
		return
	}
}

func TestIpV4Range_Contains_IpInRange(t *testing.T) {
	start := "252.61.11.10"
	end := "252.61.11.20"
	ipRange := newIpV4Range(start, end)
	if ipRange == nil {
		t.Errorf("%v -> %v range should be valid", start, end)
		return
	}
	ip := "252.61.11.15"
	if !ipRange.Contains(ip) {
		t.Errorf("%v -> %v range should contain %v", start, end, ip)
		return
	}
}

func TestIpV4Range_Contains_IpInRangeEdgeCaseStart(t *testing.T) {
	ipRange := newIpV4Range("252.61.11.10", "252.61.11.20")
	if !ipRange.Contains("252.61.11.10") {
		t.Errorf("should contain ip")
		return
	}
}

func TestIpV4Range_Contains_IpInRangeEdgeCaseEnd(t *testing.T) {
	ipRange := newIpV4Range("252.61.11.10", "252.61.11.20")
	if !ipRange.Contains("252.61.11.20") {
		t.Errorf("should contain ip")
		return
	}
}

func TestIpV4Range_Contains_IpBarelyOutsideRange(t *testing.T) {
	ipRange := newIpV4Range("252.61.11.10", "252.61.11.20")
	if ipRange.Contains("252.61.11.21") {
		t.Errorf("should not contain ip")
		return
	}
}

func TestIpV4Range_Contains_IpOutsideRange(t *testing.T) {
	start := "100.61.11.10"
	end := "152.61.11.20"
	ipRange := newIpV4Range(start, end)
	if ipRange == nil {
		t.Errorf("%v -> %v range should be valid", start, end)
		return
	}
	ip := "99.2.2.2"
	if ipRange.Contains(ip) {
		t.Errorf("%v -> %v range shouldn't contain %v", start, end, ip)
		return
	}
}
