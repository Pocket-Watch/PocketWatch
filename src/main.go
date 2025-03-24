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
		success := SaveConfig(config, configPath)
		if !success {
			os.Exit(1)
		}

		fmt.Printf("Default config file was generated in '%v'. You can edit it if you wish to configure the server.\n", configPath)
		return
	}

	config := createDefaultConfig()
	success, errorMessage := LoadConfig(&config, configPath)

	// Log error when config path was explicitly set, but config loading failed.
	if !success && flags.ConfigPath != "" {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", errorMessage)
		os.Exit(1)
	}

	// Flags have priority over config and overwrite its values.
	ApplyInputFlags(&config, flags)

	exePath, _ := os.Executable()
	file, err := os.Stat(exePath)
	if err == nil {
		BuildTime = file.ModTime().String()
	}

	PrettyPrintConfig(config)
	LOG_CONFIG = config.Logging

	db, success := ConnectToDatabase(config.Database)
	if !success {
		os.Exit(1)
	}

	if !MigrateDatabase(db) {
		return
	}

	StartServer(config.Server, db)
}
