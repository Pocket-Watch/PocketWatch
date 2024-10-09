package main

import (
    "fmt"
    "time"
)

const COLOR_RESET = "\x1b[0m"
const COLOR_FATAL = "\x1b[41;1;30m"
const COLOR_RED = "\x1b[1;31m"
const COLOR_GREEN = "\x1b[1;32m"
const COLOR_YELLOW = "\x1b[1;33m"
const COLOR_BLUE = "\x1b[1;34m"
const COLOR_PURPLE = "\x1b[1;35m"

const COLOR_GREEN_LIGHT = "\x1b[0;32m"

const ENABLE_COLORS = true

func log_error(format string, args ...any) {
	log_output("ERROR", COLOR_RED, format, args...)
}

func log_warn(format string, args ...any) {
	log_output("WARN", COLOR_YELLOW, format, args...)
}

func log_info(format string, args ...any) {
	log_output("INFO", COLOR_BLUE, format, args...)
}

func log_debug(format string, args ...any) {
	log_output("DEBUG", COLOR_PURPLE, format, args...)
}

func log_output(severity string, color string, format string, args ...any) {
	date := time.Now().Format(time.RFC1123)
	message := fmt.Sprintf(format, args...)
	if ENABLE_COLORS {
		fmt.Printf("%v[%v] %v[%v]%v %v\n", COLOR_GREEN_LIGHT, date, color, severity, COLOR_RESET, message)
	} else {
		fmt.Printf("[%v] [%v] %v\n", date, severity, message)
	}
}
