package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) <= 1 {
		DisplayHelp()
		return
	}
	options := FromArgs()
	fmt.Println("port =", options.Port)
	StartServer(&options)
}
