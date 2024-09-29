package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		DisplayHelp()
		return
	}
	options := FromArgs()
	fmt.Println("ARGS:", "|ip =", options.Address, "|p =", options.Port, "| ssl =", options.Ssl, "| help =", options.Help)
	if options.Help {
		DisplayHelp()
		return
	}
	StartServer(&options)
}
