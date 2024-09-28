package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Options struct {
	Address string
	Port    uint16
}

func Defaults() Options {
	return Options{"localhost", 1234}
}

func FromArgs() Options {
	args := os.Args
	argCount := len(args)
	settings := Defaults()
	for i := 1; i < argCount; i++ {
		if !strings.HasPrefix(args[i], "-") && i+1 >= argCount {
			continue
		}
		flag := args[i][1:]
		switch value := args[i+1]; flag {
		case "port":
		case "p":
			port, err := strconv.Atoi(value)
			if err != nil {
				fmt.Println("ERROR:", err.Error())
				os.Exit(1)
			}
			settings.Port = uint16(port)
			i++
		case "address":
		case "ip":
			settings.Address = value
		}

	}
	return settings
}

func GetExecutableName() string {
	path, err := os.Executable()
	if err != nil {
		return "watch-locally"
	}
	path = filepath.Base(path)
	dot := strings.Index(path, ".")
	if dot == -1 {
		return path
	}
	return path[:dot]
}

var VERSION string = "1.0.0"

func DisplayHelp() {
	exe := GetExecutableName()

	fmt.Println("  +----------------------+")
	fmt.Println("  |WATCH LOCALLY ", "v"+VERSION, "|")
	fmt.Println("  +----------------------+")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("    ", exe)
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("    -port []   Sets port size (0-65535) (default: 443)")
	fmt.Println()
	fmt.Println("Example usage:")
	fmt.Println("    ", exe, "-port 8888")
}

func errPrintln(format string, err error) {
	if err == nil {
		fmt.Fprintf(os.Stderr, format+"\n")
		return
	}
	fmt.Fprintf(os.Stderr, format+" [%s]\n", err.Error())
}
