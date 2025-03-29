package main

import (
	"fmt"
	"path"
	"runtime"
	"time"
)

// Log levels
const (
	LOG_FATAL = 0
	LOG_ERROR = 1
	LOG_WARN  = 2
	LOG_INFO  = 3
	LOG_DEBUG = 4
	LOG_ALL   = 5
)

// Log colors
const (
	COLOR_RESET  = "\x1b[0m"
	COLOR_FATAL  = "\x1b[41;1;30m"
	COLOR_RED    = "\x1b[1;31m"
	COLOR_GREEN  = "\x1b[1;32m"
	COLOR_YELLOW = "\x1b[1;33m"
	COLOR_BLUE   = "\x1b[1;34m"
	COLOR_PURPLE = "\x1b[1;35m"
	COLOR_CYAN   = "\x1b[0;36m"

	COLOR_GREEN_LIGHT = "\x1b[0;32m"
	COLOR_GREEN_DARK  = "\x1b[0;92m"
)

type Logger struct {
	enabled     bool
	printColors bool
	logLevel    uint32
	logToFile   bool
	outputPath  string
	// outputFile os.File
}

var LOG_CONFIG LoggingConfig

func LogError(format string, args ...any) {
	logOutput("ERROR", LOG_ERROR, COLOR_RED, 0, format, args...)
}

func LogWarn(format string, args ...any) {
	logOutput("WARN ", LOG_WARN, COLOR_YELLOW, 0, format, args...)
}

func LogInfo(format string, args ...any) {
	logOutput("INFO ", LOG_INFO, COLOR_BLUE, 0, format, args...)
}

func LogDebug(format string, args ...any) {
	logOutput("DEBUG", LOG_DEBUG, COLOR_PURPLE, 0, format, args...)
}

func LogErrorSkip(stackDepthSkip int, format string, args ...any) {
	logOutput("ERROR", LOG_ERROR, COLOR_RED, stackDepthSkip, format, args...)
}

func LogWarnSkip(stackDepthSkip int, format string, args ...any) {
	logOutput("WARN ", LOG_WARN, COLOR_YELLOW, stackDepthSkip, format, args...)
}

func LogInfoSkip(stackDepthSkip int, format string, args ...any) {
	logOutput("INFO ", LOG_INFO, COLOR_BLUE, stackDepthSkip, format, args...)
}

func LogDebugSkip(stackDepthSkip int, format string, args ...any) {
	logOutput("DEBUG", LOG_DEBUG, COLOR_PURPLE, stackDepthSkip, format, args...)
}

func logOutput(severity string, level uint16, color string, stackDepthSkip int, format string, args ...any) {
	if !LOG_CONFIG.Enabled {
		return
	}

	if LOG_CONFIG.LogLevel < level {
		return
	}

	_, file, line, ok := runtime.Caller(stackDepthSkip + 2)

	if !ok {
		file = "unknown"
		line = 0
	}

	filename := path.Base(file)
	codeLocation := fmt.Sprintf("%v:%v", filename, line)

	date := time.Now().Format("02 Jan 2006 15:04:05.00")
	// date := time.Now().Format("2006.01.02 15:04:05.00")
	// date := time.Now().Format(time.RFC1123)

	message := fmt.Sprintf(format, args...)
	if LOG_CONFIG.EnableColors {
		fmt.Printf("%v[%v] %v[%-16s] %v[%v]%v %v\n", COLOR_GREEN_LIGHT, date, COLOR_CYAN, codeLocation, color, severity, COLOR_RESET, message)
	} else {
		fmt.Printf("[%v] [%-16s] [%v] %v\n", date, codeLocation, severity, message)
	}
}
