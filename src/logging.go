package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

type LogLevel = uint16

const (
	LOG_FATAL LogLevel = iota
	LOG_ERROR
	LOG_WARN
	LOG_INFO
	LOG_DEBUG
)

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
	logLevel    uint16
	saveToFile  bool
	outputFile  os.File
	outputDir   string
	outputPath  string
}

func (*Logger) Write(p []byte) (n int, err error) {
	message := strings.TrimSpace(string(p))
	LogWarn(message)
	return len(p), nil
}

// NOTE(kihau):
//
//	Golang http server requires you to create a custom log.Logger for the internal logging of the http module.
//	This function wraps our logger in the log.Logger from the golang standard library.
func CreateInternalLoggerForHttpServer() *log.Logger {
	return log.New(&logger, "", log.Lmsgprefix)
}

var logger Logger

// NOTE(kihau): This will construct the global logger instead, but for now only log_config is used.
func SetupGlobalLogger(config LoggingConfig) bool {
	logger = Logger{
		enabled:     config.Enabled,
		printColors: config.EnableColors,
		logLevel:    config.LogLevel,
		saveToFile:  config.SaveToFile,
	}

	return true
}

func logLevelString(level LogLevel) string {
	switch level {
	case LOG_FATAL:
		return "FATAL"
	case LOG_ERROR:
		return "ERROR"
	case LOG_WARN:
		return "WARN "
	case LOG_INFO:
		return "INFO "
	case LOG_DEBUG:
		return "DEBUG"
	}

	panic("Unreachable code detected.")
}

func logLevelColor(level LogLevel) string {
	switch level {
	case LOG_FATAL:
		return COLOR_FATAL
	case LOG_ERROR:
		return COLOR_RED
	case LOG_WARN:
		return COLOR_YELLOW
	case LOG_INFO:
		return COLOR_BLUE
	case LOG_DEBUG:
		return COLOR_PURPLE
	}

	panic("Unreachable code detected.")
}

func LogFatal(format string, args ...any) {
	logOutput(LOG_FATAL, 0, format, args...)
}

func LogError(format string, args ...any) {
	logOutput(LOG_ERROR, 0, format, args...)
}

func LogWarn(format string, args ...any) {
	logOutput(LOG_WARN, 0, format, args...)
}

func LogInfo(format string, args ...any) {
	logOutput(LOG_INFO, 0, format, args...)
}

func LogDebug(format string, args ...any) {
	logOutput(LOG_DEBUG, 0, format, args...)
}

func LogFatalUp(stackUp int, format string, args ...any) {
	logOutput(LOG_FATAL, stackUp, format, args...)
}

func LogErrorUp(stackUp int, format string, args ...any) {
	logOutput(LOG_ERROR, stackUp, format, args...)
}

func LogWarnUp(stackUp int, format string, args ...any) {
	logOutput(LOG_WARN, stackUp, format, args...)
}

func LogInfoUp(stackUp int, format string, args ...any) {
	logOutput(LOG_INFO, stackUp, format, args...)
}

func LbgDebugUp(stackUp int, format string, args ...any) {
	logOutput(LOG_DEBUG, stackUp, format, args...)
}

func logOutput(logLevel LogLevel, stackUp int, format string, args ...any) {
	if !logger.enabled {
		return
	}

	if logger.logLevel < logLevel {
		return
	}

	_, file, line, ok := runtime.Caller(stackUp + 2)

	if !ok {
		file = "unknown"
		line = 0
	}

	filename := path.Base(file)
	codeLocation := fmt.Sprintf("%v:%v", filename, line)

	date := time.Now().Format("02 Jan 2006 15:04:05.00")
	levelString := logLevelString(logLevel)

	message := fmt.Sprintf(format, args...)
	if logger.printColors {
		levelColor := logLevelColor(logLevel)
		fmt.Printf("%v[%v] %v[%-16s] %v[%v]%v %v\n", COLOR_GREEN_LIGHT, date, COLOR_CYAN, codeLocation, levelColor, levelString, COLOR_RESET, message)
	} else {
		fmt.Printf("[%v] [%-16s] [%v] %v\n", date, codeLocation, levelString, message)
	}
}
