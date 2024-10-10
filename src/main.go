package main

import (
	"os"
)

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
	StartServer(&options)
}
