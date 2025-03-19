package main

import (
	"fmt"
	"os"
)

var BuildTime string

func main() {
	flags, success := ParseInputArgs()
	if !success {
		os.Exit(1)
	}

	if flags.ShowHelp {
		DisplayHelp()
		return
	}

	configPath := "config.json"
	if flags.ConfigPath != "" {
		configPath = flags.ConfigPath
	}

	if flags.GenerateConfig {
		config := createDefaultConfig()
		SaveConfig(config, configPath)
		return
	}

	config := createDefaultConfig()
	success, _ = LoadConfig(&config, configPath)

	// Log error when config path was explicitly set, but config loading failed.
	if !success && flags.ConfigPath != "" {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to load config file the json file.")
		os.Exit(1)
	}

	ApplyInputFlags(&config, flags)
	PrettyPrintConfig(config)

	exePath, _ := os.Executable()
	file, err := os.Stat(exePath)
	if err == nil {
		BuildTime = file.ModTime().String()
	}

	LOG_CONFIG = config.Logging
	StartServer(config.Server)
}
