package main

import (
	"fmt"
	"os"
	"watchlocally/opt"
	"watchlocally/serve"
)

func main() {
	args := os.Args[1:]
	if len(args) <= 1 {
		opt.DisplayHelp()
		return
	}
	options := opt.FromArgs()
	fmt.Println("port =", options.Port)
	serve.StartServer(&options)
}
