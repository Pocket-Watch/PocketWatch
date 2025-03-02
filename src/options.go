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
	// hint
	Domain string
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
		case "domain":
			settings.Domain = args[i+1]
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
		return "pocketwatch"
	}

	path = filepath.Base(path)
	return path
}

var VERSION string = "0.0.1-alpha"

func DisplayHelp() {
	exe := GetExecutableName()

	fmt.Println()
	fmt.Println("Pocket Watch ", "v"+VERSION)
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
	fmt.Println("    -domain [example.com]       Domain, if any, that the server is hosted on")
	fmt.Println("                                Serves as a hint to associate URLs with local env")
	fmt.Println()
	fmt.Println("Example usage:")
	fmt.Println("    ", exe, "-port 8888")
}

const leftColumnWidth = 8

var maxValueWidth = 0

func (o *Options) prettyPrint() {
	maxValueWidth = 2 + MaxOf(
		len(o.Address),
		LengthOfInt(int(o.Port)),
		LengthOfBool(o.Ssl),
		LengthOfBool(o.Color),
		LengthOfBool(o.Subs),
		len(o.Domain),
		len("VALUE"))

	format := &strings.Builder{}
	format.WriteString("+")
	WriteNTimes(format, "-", leftColumnWidth+1+maxValueWidth)
	format.WriteString("+\n|")
	CenterPad(format, "KEY", leftColumnWidth)
	format.WriteRune('|')
	CenterPad(format, "VALUE", maxValueWidth)
	format.WriteString("|\n+")
	WriteNTimes(format, "-", leftColumnWidth+1+maxValueWidth)
	format.WriteString("+\n")

	RowFormatKeyValue(format, "ip", o.Address)
	RowFormatKeyValue(format, "port", o.Port)
	RowFormatKeyValue(format, "ssl", o.Ssl)
	RowFormatKeyValue(format, "color", o.Color)
	RowFormatKeyValue(format, "subs", o.Subs)
	RowFormatKeyValue(format, "domain", o.Domain)

	format.WriteString("+")
	WriteNTimes(format, "-", leftColumnWidth+1+maxValueWidth)
	format.WriteString("+\n")
	fmt.Println(format.String())
}

func RowFormatKeyValue(builder *strings.Builder, key string, value any) {
	builder.WriteRune('|')
	CenterPad(builder, key, leftColumnWidth)
	builder.WriteRune('|')
	val := AnyToString(value)
	CenterPad(builder, val, maxValueWidth)
	builder.WriteString("|\n")
}

func AnyToString(anything any) string {
	return fmt.Sprintf("%v", anything)
}

func CenterPad(builder *strings.Builder, text string, length int) {
	rem := length - len(text)
	half := rem / 2
	for i := 0; i < half; i++ {
		builder.WriteString(" ")
	}
	builder.WriteString(text)
	for i := half; i < rem; i++ {
		builder.WriteString(" ")
	}
}

func WriteNTimes(builder *strings.Builder, character string, times int) {
	for i := 0; i < times; i++ {
		builder.WriteString(character)
	}
}

func LengthOfInt(n int) int {
	return len(strconv.Itoa(n))
}

func LengthOfBool(b bool) int {
	if b {
		return 4
	} else {
		return 5
	}
}

func MaxOf(values ...int) int {
	maxValue := 0
	for _, val := range values {
		if val > maxValue {
			maxValue = val
		}
	}
	return maxValue
}
