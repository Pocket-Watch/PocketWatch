package main

import (
	"fmt"
	"slices"
)

func maizn() {

	names := []string{"1", "2", "3", "4"}
	names = slices.Replace(names, 1, 3, "a")
	fmt.Println(names)

	/*url := "https://video.blender.org/static/web-videos/264ff760-803e-430e-8d81-15648e904183-720.mp4"
	const MB int64 = 1024 * 1024*/
	/*var start int64 = 0
	var end int64 = 5 * MB
	r := newRange(start, end)
	size, err := getContentRange(url, "")
	if err != nil {
		panic(err)
	}
	LogInfo("%v", size)*/
}
