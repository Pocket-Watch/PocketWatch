package main

import (
	"os"
)

var BuildTime string

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		DisplayHelp()
		return
	}

	options := FromArgs()
	if options.Help {
		DisplayHelp()
		return
	}

	options.prettyPrint()

	exePath, _ := os.Executable()
	file, err := os.Stat(exePath)
	if err == nil {
		BuildTime = file.ModTime().String()
	}

	StartServer(&options)
}
