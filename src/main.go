package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/signal"
)

var BuildTime string

func CaptureCtrlC(db *sql.DB) {
	channel := make(chan os.Signal, 1)
	signal.Notify(channel, os.Interrupt)

	go func() {
		for {
			<-channel
			RunInteractiveShell(channel, db)
		}
	}()
}

func main() {
	flags, success := ParseInputArgs()
	if !success {
		os.Exit(1)
	}

	if !VerifyInputFlags(flags) {
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
	SetupGlobalLogger(config.Logging)

	db, success := ConnectToDatabase(config.Database)
	if !success {
		os.Exit(1)
	}

	if !MigrateDatabase(db) {
		return
	}

	if config.Server.EnableShell {
		CaptureCtrlC(db)
	}

	StartServer(config.Server, db)
}
