package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
)

var BuildTime string

func StopServer(server *Server) {
	EnableConsoleLogging()

	server.state.mutex.Lock()
	timestamp := server.getCurrentTimestamp()
	server.state.mutex.Unlock()

	DatabaseSetTimestamp(server.db, timestamp)
	pprof.StopCPUProfile()

	LogInfo("Shutting down the server")
	os.Exit(0)
}

func CaptureCtrlC(server *Server) {
	channel := make(chan os.Signal, 1)
	signal.Notify(channel, os.Interrupt)

	go func() {
		for {
			<-channel

			if server.config.EnableShell {
				RunInteractiveShell(channel, server)
			} else {
				StopServer(server)
			}
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
	if flags.ConfigPath != "" || ConfigExists(configPath) {
		success, errorMessage := LoadConfig(&config, configPath)
		if !success {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", errorMessage)
			os.Exit(1)
		}
	}

	// Flags have priority over config and overwrite its values.
	ApplyInputFlags(&config, flags)

	exePath, _ := os.Executable()
	file, err := os.Stat(exePath)
	if err == nil {
		exeModTime := file.ModTime()
		BuildTime = exeModTime.Format(VERSION_LAYOUT)
	}

	YTDLP_ENABLED = config.Server.EnableYtdlp

	PrettyPrintConfig(config)
	if !SetupGlobalLogger(config.Logging) {
		os.Exit(1)
	}

	db, success := ConnectToDatabase(config.Database)
	if !success {
		os.Exit(1)
	}

	if !MigrateDatabase(db) {
		os.Exit(1)
	}

	if flags.ProfileOutput != "" {
		file, err := os.Create(flags.ProfileOutput)
		if err != nil {
			LogError("Failed to create profiler output file for the recorded data: %v", err)
			os.Exit(1)
		}

		err = pprof.StartCPUProfile(file)
		if err != nil {
			LogError("Failed to start the profiler: %v", err)
			os.Exit(1)
		}

		LogInfo("Profiler started with output data available at: %v", flags.ProfileOutput)
	}

	StartServer(config.Server, db)
}
