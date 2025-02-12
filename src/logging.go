package main

import (
	"fmt"
	"path"
	"runtime"
	"time"
)

const COLOR_RESET = "\x1b[0m"
const COLOR_FATAL = "\x1b[41;1;30m"
const COLOR_RED = "\x1b[1;31m"
const COLOR_GREEN = "\x1b[1;32m"
const COLOR_YELLOW = "\x1b[1;33m"
const COLOR_BLUE = "\x1b[1;34m"
const COLOR_PURPLE = "\x1b[1;35m"
const COLOR_CYAN = "\x1b[0;36m"

const COLOR_GREEN_LIGHT = "\x1b[0;32m"
const COLOR_GREEN_DARK = "\x1b[0;92m"

var ENABLE_COLORS = true

func LogError(format string, args ...any) {
	logOutput("ERROR", COLOR_RED, format, args...)
}

func LogWarn(format string, args ...any) {
	logOutput("WARN ", COLOR_YELLOW, format, args...)
}

func LogInfo(format string, args ...any) {
	logOutput("INFO ", COLOR_BLUE, format, args...)
}

func LogDebug(format string, args ...any) {
	logOutput("DEBUG", COLOR_PURPLE, format, args...)
}

func logOutput(severity string, color string, format string, args ...any) {
	_, file, line, ok := runtime.Caller(2)

	if !ok {
		file = "unknown"
		line = 0
	}

	filename := path.Base(file)
	codeLocation := fmt.Sprintf("%v:%v", filename, line)

	date := time.Now().Format(time.RFC1123)
	message := fmt.Sprintf(format, args...)
	if ENABLE_COLORS {
		fmt.Printf("%v[%v] %v[%-16s] %v[%v]%v %v\n", COLOR_GREEN_LIGHT, date, COLOR_CYAN, codeLocation, color, severity, COLOR_RESET, message)
	} else {
		fmt.Printf("[%v] [%-16s] [%v] %v\n", date, codeLocation, severity, message)
	}
}
