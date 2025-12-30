package main

import (
	"bytes"
	"net"
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
	expected := []string{"abc", "123", "45"}
	path, isSafe := safeJoin("abc/", "/123/", "/45")
	if !isSafe {
		t.Errorf("Path should be safe!")
		return
	}
	given := strings.Split(path, sep)

	if !slices.Equal(given, expected) {
		t.Errorf("Path is %v, different from expected %v", given, expected)
		return
	}
}

func TestSafeDoubleDot(t *testing.T) {
	sep := getPathSeparator()
	expected := "Really..." + sep + "45"
	path, isSafe := safeJoin("Really...", "/45")

	if !isSafe {
		t.Errorf("Path should be safe!")
		return
	}
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
	expected := []string{"ABC ..", "DEF", "...", "bin"}
	input := "./.\\ABC ../DEF/.../bin"

	joined, isSafe := safeJoin(input)
	if !isSafe {
		t.Errorf("Path %v is safe!", input)
		return
	}
	sep := getPathSeparator()
	given := strings.Split(joined, sep)

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

func TestIpV4Range_Contains_IpByFirstOctet(t *testing.T) {
	start := "0.0.0.0"
	end := "200.0.0.0"
	ipRange := newIpV4Range(start, end)
	if ipRange == nil {
		t.Errorf("%v -> %v range should be valid", start, end)
		return
	}
	ip := "192.168.1.10"
	if !ipRange.Contains(ip) {
		t.Errorf("%v -> %v range must contain %v", start, end, ip)
		return
	}
}

func TestIpV4Range_Precedes_False(t *testing.T) {
	start := net.ParseIP("12.255.0.0").To4()
	end := net.ParseIP("12.11.1.1").To4()
	precedes := Precedes(start, end)
	if precedes {
		t.Errorf("%v should not precede %v", start, end)
	}
}

func TestIpV4Range_Precedes_TrueEdge(t *testing.T) {
	start := net.ParseIP("12.1.1.1").To4()
	end := net.ParseIP("12.1.1.1").To4()
	precedes := Precedes(start, end)
	if !precedes {
		t.Errorf("%v should precede %v", start, end)
	}
}

func TestParseMigrationNumber(t *testing.T) {
	name := "0035-something.sql"
	success, num := parseMigrationNumber(name)
	if !success {
		t.Errorf("Parse should be successful for %v", name)
	}
	if num != 35 {
		t.Errorf("Number should be 35, got %v", num)
	}
}

func TestInvalidRange(t *testing.T) {
	if newRange(5, 4) != nil {
		t.Errorf("The range should be nil because start > end")
	}
	if newRange(-1, 1234) != nil {
		t.Errorf("The range should be nil because one of the values is negative")
	}
}

func TestRangeEncompassesTrue(t *testing.T) {
	r := newRange(10, 100)
	contained := newRange(20, 60)
	if !r.encompasses(contained) {
		t.Errorf("%v should encompass %v", r, contained)
	}
}

func TestRangeEncompassesFalse(t *testing.T) {
	r := newRange(10, 100)
	contained := newRange(90, 101)
	if r.encompasses(contained) {
		t.Errorf("%v shouldn't encompass %v", r, contained)
	}
}

func TestRangeOverlaps(t *testing.T) {
	range1 := newRange(10, 20)
	range2 := newRange(20, 1000)
	if !range1.overlaps(range2) || !range2.overlaps(range1) {
		t.Errorf("%v should overlap %v", range1, range2)
	}
}

func TestRangeOverlapsContained(t *testing.T) {
	range1 := newRange(50, 60)
	range2 := newRange(20, 80)
	if !range1.overlaps(range2) || !range2.overlaps(range1) {
		t.Errorf("%v should overlap %v", range1, range2)
	}
}

func TestRangeConnects(t *testing.T) {
	range1 := newRange(10, 30)
	range2 := newRange(31, 100)
	if !range1.connects(range2) || !range2.connects(range1) {
		t.Errorf("%v should connect %v", range1, range2)
	}
}

func TestRangeConnectsFalse(t *testing.T) {
	range1 := newRange(10, 30)
	range2 := newRange(32, 100)
	if range1.connects(range2) || range2.connects(range1) {
		t.Errorf("%v shouldn't connect %v", range1, range2)
	}
}

func TestRangeDifferenceDisjoint(t *testing.T) {
	left := newRange(1, 5)
	right := newRange(6, 10)
	diff := left.difference(right)

	if !diff[0].equals(left) {
		t.Errorf("The difference %v is different from expected", diff)
	}
}

func TestRangeDifferenceJoint(t *testing.T) {
	range1 := newRange(50, 100)
	range2 := newRange(25, 75)
	expected := newRange(76, 100)
	diff := range1.difference(range2)

	if !diff[0].equals(expected) {
		t.Errorf("The difference %v is different from expected", diff)
	}
}

func TestRangeDifferenceJointEdge(t *testing.T) {
	left := newRange(1, 5)
	right := newRange(5, 10)
	expected := newRange(1, 4)
	diff := left.difference(right)

	if !diff[0].equals(expected) {
		t.Errorf("The difference %v is different from expected", diff)
	}
}

func TestRangeDifferenceContained(t *testing.T) {
	outer := newRange(1, 10)
	inner := newRange(5, 7)
	expected := []Range{*newRange(1, 4), *newRange(8, 10)}
	diff := outer.difference(inner)

	if len(diff) != 2 {
		t.Errorf("The difference should contain two separate ranges. Actual: %v", diff)
		return
	}
	if !diff[0].equals(&expected[0]) || !diff[1].equals(&expected[1]) {
		t.Errorf("The difference %v is different from expected %v", diff, expected)
	}
}

func TestRangeIntersection(t *testing.T) {
	range1 := newRange(66, 99)
	range2 := newRange(77, 111)
	expected := newRange(77, 99)
	intersection1, intersects1 := range1.intersection(range2)
	intersection2, intersects2 := range2.intersection(range1)

	if !intersects1 || !intersects2 {
		t.Errorf("Intersection should be possible both ways")
	}

	if !intersection1.equals(expected) {
		t.Errorf("Intersection %v is different from expected %v", intersection1, expected)
	}

	if !intersection1.equals(&intersection2) {
		t.Errorf("Intersections should be the same both ways")
	}
}

func TestRangeMerge(t *testing.T) {
	range1 := newRange(10, 20)
	range2 := newRange(15, 35)
	merge12 := range1.mergeWith(range2)
	merge21 := range2.mergeWith(range1)

	if merge12.start != 10 || merge12.end != 35 {
		t.Errorf("The merged range %v is different from expected", merge12)
	}
	if merge21.start != 10 || merge21.end != 35 {
		t.Errorf("The merged range %v is different from expected", merge21)
	}
}

func TestRangeIncorporateIntoEmpty(t *testing.T) {
	toIncorporate := newRange(15, 30)

	result := incorporateRange(toIncorporate, nil)
	if len(result) != 1 {
		t.Errorf("The merged range's length is not 1 but %v", len(result))
		return
	}
	expected0 := newRange(15, 30)
	if !expected0.equals(&result[0]) {
		t.Errorf("The element at 0 %v is different from expected", result[0])
	}
}

func TestRangeIncorporate(t *testing.T) {
	left := newRange(10, 20)
	right := newRange(60, 70)
	toIncorporate := newRange(15, 30)

	result := incorporateRange(toIncorporate, []Range{*left, *right})
	if len(result) != 2 {
		t.Errorf("The merged range's length is not 2 but %v", len(result))
		return
	}
	expected0 := newRange(10, 30)
	if !expected0.equals(&result[0]) {
		t.Errorf("The element at 0 %v is different from expected", result[0])
	}
}

func TestRangeIncorporateComplex(t *testing.T) {
	range1 := newRange(10, 20)
	range2 := newRange(30, 40)
	range3 := newRange(70, 90)
	range4 := newRange(110, 200)
	range5 := newRange(300, 400)
	toIncorporate := newRange(25, 150)

	result := incorporateRange(toIncorporate, []Range{*range1, *range2, *range3, *range4, *range5})
	if len(result) != 3 {
		t.Errorf("There should be 3 resulting ranges but instead there's %v", len(result))
		return
	}
	if !newRange(10, 20).equals(&result[0]) {
		t.Errorf("The element at 0 %v is different from expected", result[0])
	}

	if !newRange(25, 200).equals(&result[1]) {
		t.Errorf("The element at 1 %v is different from expected", result[1])
	}

	if !newRange(300, 400).equals(&result[2]) {
		t.Errorf("The element at 2 %v is different from expected", result[2])
	}
}

func TestRangeIncorporateEncompassed(t *testing.T) {
	r1 := newRange(100, 200)
	r2 := newRange(300, 400)
	toIncorporate := newRange(300, 400)

	result := incorporateRange(toIncorporate, []Range{*r1, *r2})
	if len(result) != 2 {
		t.Errorf("There should be 2 resulting ranges but instead there's %v", len(result))
		return
	}
	if !newRange(100, 200).equals(&result[0]) {
		t.Errorf("The element at 0 %v is different from expected", result[0])
	}
	if !newRange(300, 400).equals(&result[1]) {
		t.Errorf("The element at 1 %v is different from expected", result[1])
	}
}

func TestRangeIncorporateConnectedRange(t *testing.T) {
	r1 := newRange(2, 3)
	r2 := newRange(100, 200)
	toIncorporate := newRange(201, 299)
	r3 := newRange(300, 400)
	r4 := newRange(888, 999)

	result := incorporateRange(toIncorporate, []Range{*r1, *r2, *r3, *r4})
	if len(result) != 3 {
		t.Errorf("There should be 3 resulting ranges but instead there's %v", len(result))
		return
	}
	if !newRange(2, 3).equals(&result[0]) {
		t.Errorf("The element at 0 %v is different from expected", result[0])
	}
	if !newRange(100, 400).equals(&result[1]) {
		t.Errorf("The element at 1 %v is different from expected", result[1])
	}
}

func TestParseSongTitleSimple(t *testing.T) {
	artist, songTitle := parseSongTitle("Artist - Song Title")
	if artist != "Artist" {
		t.Errorf("The artist should be 'Artist' but actual is %v", artist)
	}

	if songTitle != "Song Title" {
		t.Errorf("The song title should be 'Song Title' but actual is %v", songTitle)
	}
}

func TestParseSongTitleJustTrackName(t *testing.T) {
	artist, songTitle := parseSongTitle("Song Title")
	if artist != "" {
		t.Errorf("The artist should be empty but actual is %v", artist)
	}

	if songTitle != "Song Title" {
		t.Errorf("The song title should be 'Song Title' but actual is %v", songTitle)
	}
}

func TestParseSongTitleWithDescriptors(t *testing.T) {
	expectedArtist := "Artist Name"
	expectedTrackName := "Song Title"

	artist, songTitle := parseSongTitle("Artist Name - Song Title (ft. Some) (Official) [Prod. DJ]")

	if artist != expectedArtist {
		t.Errorf("The artist should be '%v' but actual is %v", expectedArtist, artist)
	}

	if songTitle != expectedTrackName {
		t.Errorf("The song title should be '%v' but actual is %v", expectedTrackName, songTitle)
	}
}

func TestParseSongTitleWithNestedDescriptors(t *testing.T) {
	expectedArtist := "Artist Name"
	expectedTrackName := "Song Title"

	artist, songTitle := parseSongTitle("Artist Name -     Song Title (Official Video (ft. Someone) [LIVE])")

	if artist != expectedArtist {
		t.Errorf("The artist should be '%v' but actual is %v", expectedArtist, artist)
	}

	if songTitle != expectedTrackName {
		t.Errorf("The song title should be '%v' but actual is %v", expectedTrackName, songTitle)
	}
}

func TestParseSongTitleWithPipes(t *testing.T) {
	expectedArtist := "Artist1 | Artist2"
	expectedTrackName := "Title"

	artist, songTitle := parseSongTitle("Artist1 | Artist2 - Title | With a pipe")

	if artist != expectedArtist {
		t.Errorf("The artist should be '%v' but actual is %v", expectedArtist, artist)
	}

	if songTitle != expectedTrackName {
		t.Errorf("The song title should be '%v' but actual is %v", expectedTrackName, songTitle)
	}
}

func TestByteBufferStartsWith(t *testing.T) {
	bufferContent := "123-XYZ-222-333"
	buffer := bytes.NewBuffer([]byte(bufferContent))

	if !bufferStartsWith(buffer, []byte("123")) {
		t.Errorf("The buffer doesn't start with '123'")
	}
}
