package main

import (
	"fmt"
	"io"
	"net/http"
	net_url "net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var client = http.Client{}

func constructTitleWhenMissing(entry *Entry) string {
	if entry.Title != "" {
		return entry.Title
	}

	base := path.Base(entry.Url)
	title := strings.TrimSuffix(base, filepath.Ext(base))
	return title
}

func stripSuffix(url string) string {
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash == -1 {
		// this could be more robust
		return url
	}
	return url[:lastSlash]
}

func inferOrigin(referer string) string {
	if strings.HasSuffix(referer, "/") {
		length := len(referer)
		return referer[:length-1]
	}
	return referer
}

func stripLastSegment(url *net_url.URL) string {
	lastSlash := strings.LastIndex(url.Path, "/")
	stripped := url.Scheme + "://" + url.Host + url.Path[:lastSlash+1]
	return stripped
}

func toString(num int) string {
	return strconv.Itoa(num)
}

func lastUrlSegment(url string) string {
	url = path.Base(url)
	questionMark := strings.Index(url, "?")
	if questionMark == -1 {
		return url
	}
	return url[:questionMark]
}

func getRootDomain(url *net_url.URL) string {
	return url.Scheme + "://" + url.Host
}

func downloadFile(url string, filename string, referer string) error {
	request, _ := http.NewRequest("GET", url, nil)
	if referer != "" {
		request.Header.Set("Referer", referer)
		request.Header.Set("Origin", inferOrigin(referer))
	}
	response, err := client.Do(request)

	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return &DownloadError{Code: response.StatusCode, Message: "Failed to download file"}
	}
	defer response.Body.Close()

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, response.Body)
	if err != nil {
		return err
	}
	return nil
}

type DownloadError struct {
	Code    int
	Message string
}

// Implement the error interface for NetworkError
func (e *DownloadError) Error() string {
	return fmt.Sprintf("NetworkError: Code=%d, Message=%s", e.Code, e.Message)
}
