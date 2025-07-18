package main

import "testing"

func TestStringedParamWithCommas(t *testing.T) {
	input := "CODECS=\"avc1.4d4028,mp4a.40.2,stpp.ttml.im1t\",RESOLUTION=1920x800"
	params := parseParams(input)
	resolution := getParamValue("RESOLUTION", params)
	if resolution != "1920x800" {
		t.Errorf("Wrong resolution rate, actual %v", resolution)
	}
	codecs := getParamValue("CODECS", params)
	if codecs != "avc1.4d4028,mp4a.40.2,stpp.ttml.im1t" {
		t.Errorf("Different codecs, actual %v", codecs)
	}
}

func TestLastParam(t *testing.T) {
	input := "FRAME-RATE=24.000,LAST=\"last\""
	params := parseParams(input)
	lastValue := getParamValue("LAST", params)
	if lastValue != "last" {
		t.Errorf("Last value is different, actual %v", lastValue)
	}
}

func TestMiscParams(t *testing.T) {
	input := "BANDWIDTH=6725464,AVERAGE-BANDWIDTH=6296707,CODECS=\"avc1.4d4028,mp4a.40.2,stpp.ttml.im1t\",RESOLUTION=1920x800,FRAME-RATE=24.000,AUDIO=\"audio\""
	params := parseParams(input)
	audioValue := getParamValue("AUDIO", params)
	if audioValue != "audio" {
		t.Errorf("Audio value is different, actual %v", audioValue)
	}
}

func TestUtf8Params(t *testing.T) {
	input := "NAME=\"Ján的\",K=V"
	params := parseParams(input)
	name := getParamValue("NAME", params)
	if name != "Ján的" {
		t.Errorf("Name value is different, actual %v", name)
	}
}

func TestKeylessParamsDontCrash(t *testing.T) {
	parseParams("=V")
}

func TestShortParamsAndTrailingComma(t *testing.T) {
	input := "K=V,"
	params := parseParams(input)
	if len(params) != 1 {
		t.Errorf("Params length is not 1, actual %v", len(params))
		return
	}
	val := getParamValue("K", params)
	if val != "V" {
		t.Errorf("Value is different, expected: V, actual: %v", val)
	}
}

func TestAddingPairsAttributeRetrievalAndRemoval(t *testing.T) {
	m3u := M3U{}
	m3u.isMasterPlaylist = true
	m3u.addPair(KeyValue{key: EXT_X_SESSION_DATA, value: "DATA-ID=\"key1\",VALUE=\"value1\""})
	m3u.addPair(KeyValue{key: EXT_X_SESSION_DATA, value: "DATA-ID=\"key2\",VALUE=\"value2\""})
	m3u.addPair(KeyValue{key: EXT_X_PLAYLIST_TYPE, value: "VOD"})
	if len(m3u.attributePairs) != 3 {
		t.Errorf("M3U should contain 3 attribute pairs")
		return
	}
	attributeValue := m3u.getAttribute(EXT_X_SESSION_DATA)
	sessionParams := parseParams(attributeValue)
	if getParamValue("DATA-ID", sessionParams) != "key1" {
		t.Errorf("Data ID is different, expected: key1, actual: %v", attributeValue)
		return
	}
	m3u.removeAttributes(EXT_X_SESSION_DATA)
	if len(m3u.attributePairs) != 1 {
		t.Errorf("M3U should contain 1 attribute pair")
		return
	}
	if m3u.getAttribute(EXT_X_PLAYLIST_TYPE) != "VOD" {
		t.Errorf("M3U should be VOD")
		return
	}
}
