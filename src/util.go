package main

import (
	"fmt"
	"io"
	"net/http"
	url2 "net/url"
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

func stripLastSegment(url string) (*string, error) {
	pUrl, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}
	lastSlash := strings.LastIndex(pUrl.Path, "/")
	stripped := pUrl.Scheme + "://" + pUrl.Host + pUrl.Path[:lastSlash+1]
	return &stripped, nil
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
		return fmt.Errorf("error downloading file: status code %d", response.StatusCode)
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
