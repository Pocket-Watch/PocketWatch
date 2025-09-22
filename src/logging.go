package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LogLevel = uint32

const (
	LOG_FATAL LogLevel = iota
	LOG_ERROR
	LOG_WARN
	LOG_INFO
	LOG_DEBUG
)

func LogLevelToString(level LogLevel) string {
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
	default:
		return "DEBUG"
	}
}

func LogLevelFromString(levelString string) (LogLevel, bool) {
	strip := strings.TrimSpace(levelString)
	level := strings.ToLower(strip)

	switch level {
	case "fatal":
		return LOG_FATAL, true
	case "error":
		return LOG_ERROR, true
	case "warn":
		return LOG_WARN, true
	case "info":
		return LOG_INFO, true
	case "debug":
		return LOG_DEBUG, true
	default:
		return 0, false
	}
}

func LogLevelColor(level LogLevel) string {
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
	default:
		return COLOR_PURPLE
	}
}

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

// TODO(kihau): Store last log date to split latest.log by days/weeks/months?, archive and compress old logs.
type Logger struct {
	enabled      bool
	logToConsole atomic.Bool
	printColors  bool
	logLevel     atomic.Uint32

	saveToFile       bool
	fileLock         sync.Mutex
	outputDir        string
	outputFile       *os.File
	lastCompressTime time.Time
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

func setupFileLogging(logDirectory string) bool {
	if err := os.MkdirAll(logDirectory, os.ModePerm); err != nil {
		reason := err
		if err := err.(*os.PathError); err != nil {
			reason = err.Err
		}

		LogError("Failed to create log directory at %v: %v", logDirectory, reason)
		return false
	}

	logpath := path.Join(logDirectory, "latest.log")

	// NOTE(kihau): This is technically not correct, should instead be CreateTime, but there is not such function in Go.
	lastModified := time.Now()
	if info, err := os.Stat(logpath); err == nil {
		lastModified = info.ModTime()
	}

	file, err := os.OpenFile(logpath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		reason := err
		if err := err.(*os.PathError); err != nil {
			reason = err.Err
		}

		LogError("Failed to create log file at path %v: %v", logpath, reason)
		return false
	}

	logger.outputFile = file
	logger.saveToFile = true
	logger.outputDir = logDirectory
	logger.lastCompressTime = lastModified

	return true
}

func SetupGlobalLogger(config LoggingConfig) bool {
	logger = Logger{
		enabled:     config.Enabled,
		printColors: config.EnableColors,
	}

	logger.logToConsole.Store(true)

	level, ok := LogLevelFromString(config.LogLevel)
	if !ok {
		logger.logLevel.Store(LOG_ERROR)
		LogError("Failed to set correct log level. Provided log level '%v' does not exist.", config.LogLevel)
		return false
	}

	logger.logLevel.Store(level)

	if config.SaveToFile {
		if !setupFileLogging(config.LogDirectory) {
			return false
		}
	}

	return true
}

func CompressCurrentLogFile() {
	date := logger.lastCompressTime.Format("2006.01.02")
	newName := fmt.Sprintf("%v.gz", date)
	newPath := path.Join(logger.outputDir, newName)

	destination, err := os.OpenFile(newPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		fmt.Printf("[INTERNAL LOGGER ERROR] Failed to create new log archive file at %v: %v", newPath, err)
		return
	}

	zipper := gzip.NewWriter(destination)

	if _, err := logger.outputFile.Seek(0, 0); err != nil {
		fmt.Printf("[INTERNAL LOGGER ERROR] Failed to seek to the beginning of the current log file: %v", err)
		return
	}

	if _, err = io.Copy(zipper, logger.outputFile); err != nil {
		fmt.Printf("[INTERNAL LOGGER ERROR] Failed to compress current log file: %v", err)
		return
	}

	if err = zipper.Close(); err != nil {
		fmt.Printf("[INTERNAL LOGGER ERROR] Failed to close archive file compression stream: %v", err)
		return
	}

	if err = destination.Close(); err != nil {
		fmt.Printf("[INTERNAL LOGGER ERROR] Failed to close new log archive file: %v", err)
		return
	}

	if err := logger.outputFile.Truncate(0); err != nil {
		fmt.Printf("[INTERNAL LOGGER ERROR] Failed truncate current log file: %v", err)
		return
	}
}

func LogToFile(message string) {
	if !logger.saveToFile {
		return
	}

	logger.fileLock.Lock()
	defer logger.fileLock.Unlock()

	now := time.Now()

	if now.Month() != logger.lastCompressTime.Month() {
		CompressCurrentLogFile()
		logger.lastCompressTime = now
	}

	logger.outputFile.WriteString(message)
}

func SetLogLevel(level LogLevel) {
	logger.logLevel.Store(level)
}

func GetLogLevel() LogLevel {
	return logger.logLevel.Load()
}

func DisableConsoleLogging() {
	logger.logToConsole.Store(false)
}

func EnableConsoleLogging() {
	logger.logToConsole.Store(true)
}

func printStackTrace(skip int) {
	callers := [1024]uintptr{}
	count := runtime.Callers(skip+3, callers[:])

	date := time.Now().Format("02 Jan 2006 15:04:05.00")
	maxName := 0

	for i := 0; i < count-1; i += 1 {
		callerFunc := runtime.FuncForPC(callers[i])
		funcname := callerFunc.Name()
		if len(funcname) > maxName {
			maxName = len(funcname)
		}
	}

	maxName += 1

	var outputForConsole strings.Builder
	var outputForFile strings.Builder

	for i := 0; i < count-1; i += 1 {
		callerFunc := runtime.FuncForPC(callers[i])

		funcname := callerFunc.Name()
		filepath, line := callerFunc.FileLine(callerFunc.Entry())

		filename := path.Base(filepath)
		codeLocation := fmt.Sprintf("%v:%v", filename, line)

		levelString := LogLevelToString(LOG_FATAL)

		if logger.printColors {
			levelColor := LogLevelColor(LOG_FATAL)
			text := fmt.Sprintf("%v[%v] %v[%-16s] %v[%v]%v   at %-*v %v:%v\n", COLOR_GREEN_LIGHT, date, COLOR_CYAN, codeLocation, levelColor, levelString, COLOR_RESET, maxName, funcname, filepath, line)
			outputForConsole.WriteString(text)
		} else {
			text := fmt.Sprintf("[%v] [%-16s] [%v]   at %-*v %v:%v\n", date, codeLocation, levelString, maxName, funcname, filepath, line)
			outputForConsole.WriteString(text)
		}

		if logger.saveToFile {
			text := fmt.Sprintf("[%v] [%-16s] [%v]   at %-*v %v:%v\n", date, codeLocation, levelString, maxName, funcname, filepath, line)
			outputForFile.WriteString(text)
		}
	}

	fmt.Print(outputForConsole.String())
	LogToFile(outputForFile.String())
}

func LogFatal(format string, args ...any) {
	logOutput(LOG_FATAL, 0, format, args...)
	printStackTrace(0)
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
	printStackTrace(stackUp)
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

func LogDebugUp(stackUp int, format string, args ...any) {
	logOutput(LOG_DEBUG, stackUp, format, args...)
}

func logOutput(logLevel LogLevel, stackUp int, format string, args ...any) {
	if !logger.enabled {
		return
	}

	if logger.logLevel.Load() < logLevel {
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
	levelString := LogLevelToString(logLevel)

	message := fmt.Sprintf(format, args...)

	if logger.saveToFile {
		text := fmt.Sprintf("[%v] [%-16s] [%v] %v\n", date, codeLocation, levelString, message)
		LogToFile(text)
	}

	if !logger.logToConsole.Load() {
		return
	}

	if logger.printColors {
		levelColor := LogLevelColor(logLevel)
		fmt.Printf("%v[%v] %v[%-16s] %v[%v]%v %v\n", COLOR_GREEN_LIGHT, date, COLOR_CYAN, codeLocation, levelColor, levelString, COLOR_RESET, message)
	} else {
		fmt.Printf("[%v] [%-16s] [%v] %v\n", date, codeLocation, levelString, message)
	}
}

type Logsite struct {
	mutex    sync.Mutex
	lastCall time.Time
}

// Allows or denies calls based on time passed since the last allowed call
func (site *Logsite) atMostEvery(interval time.Duration) bool {
	now := time.Now()
	site.mutex.Lock()
	defer site.mutex.Unlock()
	if now.Sub(site.lastCall) < interval {
		return false
	}
	site.lastCall = now
	return true
}
