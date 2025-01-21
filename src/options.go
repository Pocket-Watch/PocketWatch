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
	Ssl     bool
	Color   bool
	Subs    bool
	Help    bool
}

func (o *Options) prettyPrint() {
	fmt.Println(" -------ARGS---------")
	fmt.Println(" | ip     |", o.Address)
	fmt.Println(" | port   |", o.Port)
	fmt.Println(" | ssl    |", o.Ssl)
	fmt.Println(" | color  |", o.Color)
	fmt.Println(" | subs   |", o.Subs)
	fmt.Println(" | help   |", o.Help)
	fmt.Println(" --------------------")
}

func Defaults() Options {
	return Options{
		Address: "localhost",
		Port:    1234,
		Ssl:     false,
		Color:   true,
		Subs:    true,
		Help:    false,
	}
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
		switch flag {
		case "port":
			fallthrough
		case "p":
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Println("ERROR:", err.Error())
				os.Exit(1)
			}
			settings.Port = uint16(port)
			i++
		case "address", "addr", "ip":
			settings.Address = args[i+1]
		case "nc", "no-color", "no-colors":
			settings.Color = false
			ENABLE_COLORS = false
		case "ns", "no-sub", "no-subs":
			settings.Subs = false
		case "ssl":
			settings.Ssl = true
		case "h", "help":
			settings.Help = true
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
	fmt.Println("    ", exe, "[OPTIONS]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("    -h, -help                   Displays this help message")
	fmt.Println("    -ip, -address [10.0.0.1]    Binds server to IP (default: localhost)")
	fmt.Println("    -p, -port [443]             Sets port size (0-65535) (default: 1234)")
	fmt.Println("    -nc, -no-color              Disables colored logging (default: enabled)")
	fmt.Println("    -ns, -no-subs               Disabled support for subtitle search")
	fmt.Println("    -ssl                        Enables SSL. Secrets are read from:")
	fmt.Println("                                 - CERTIFICATE: ./secret/certificate.pem")
	fmt.Println("                                 - PRIVATE KEY: ./secret/privatekey.pem")
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
