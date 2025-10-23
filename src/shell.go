package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func readShellCommand() (string, string) {
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')

	space := strings.Index(input, " ")

	var command string
	var argument string

	if space == -1 {
		command = input
	} else {
		command = input[:space]
		argument = strings.TrimSpace(input[space:])
	}

	command = strings.TrimSpace(command)
	command = strings.ToLower(command)
	return command, argument
}

func handleCtrlC(ctrlcChan chan os.Signal, exit chan bool) {
	for {
		select {
		case <-exit:
			return
		case <-ctrlcChan:
			fmt.Println()
			fmt.Print("> ")
		}
	}
}

func RunInteractiveShell(ctrlcChan chan os.Signal, server *Server) {
	fmt.Println()
	LogInfo("Opening interactive shell. Console logging paused.")
	DisableConsoleLogging()

	fmt.Println()
	fmt.Println("PocketWatch Interactive Shell:")
	fmt.Println("  type 'help' to show available command.")
	fmt.Println()

	exitChan := make(chan bool, 1)
	go handleCtrlC(ctrlcChan, exitChan)
	defer func() { exitChan <- true }()

outer:
	for {
		fmt.Print("> ")
		command, argument := readShellCommand()

		switch command {
		case "shutdown":
			EnableConsoleLogging()

			server.state.mutex.Lock()
			timestamp := server.getCurrentTimestamp()
			server.state.mutex.Unlock()
			DatabaseSetTimestamp(server.db, timestamp)

			LogInfo("Shutting down the server.")
			os.Exit(0)

		case "quit", "q":
			EnableConsoleLogging()
			LogInfo("Exiting interactive shell.")
			return

		case "help", "h":
			fmt.Println("  shutdown        - Exit the server")
			fmt.Println("  quit,     q     - Quit the interactive shell")
			fmt.Println("  help,     h     - Display this help prompt")
			fmt.Println("  version,  v     - Print server version")
			fmt.Println("  uptime,   up    - Print server uptime")
			fmt.Println("  loglevel, log   - Print or set the log level")
			fmt.Println("  sqlquery, sql   - Execute SQL query")
			fmt.Println("  sqltable, tbl   - Show SQL tables or print layout of a specified SQL table")
			fmt.Println("  sqlviews, views - Show SQL views")
			fmt.Println("  users,    usr   - Show number of active users")
			fmt.Println("  cleanup,  cln   - Cleanup temporary data (such as inactive dummy users)")
			fmt.Println("  reload,   rel   - Reloads static web resources from disk, enable/disable hot-reload when on/off argument is provided")

		case "version", "v":
			fmt.Printf("Server version: %v_%v\n", VERSION, BuildTime)

		case "uptime", "up":
			uptime := time.Since(startTime)
			fmt.Printf("Server uptime: %v\n", uptime)

		case "loglevel", "log":
			if argument == "" {
				loglevel := GetLogLevel()
				fmt.Printf("Log level is currently set to %v\n", LogLevelToString(loglevel))
			} else {
				number, err := strconv.Atoi(argument)
				if err == nil {
					SetLogLevel(uint32(number))
					fmt.Printf("Log level is now set to %v\n", LogLevelToString(uint32(number)))
					break
				}

				loglevel, valid := LogLevelFromString(argument)
				if valid {
					SetLogLevel(loglevel)
					fmt.Printf("Log level is now set to %v\n", LogLevelToString(loglevel))
					break
				}

				fmt.Printf("'%v' is not a valid log level\n", argument)
			}

		case "sqlquery", "sql":
			DatabaseSqlQuery(server.db, argument)

		case "sqltable", "tbl":
			if argument == "" {
				DatabaseShowTables(server.db)
			} else {
				DatabasePrintTableLayout(server.db, argument)
			}
		case "sqlviews", "views":
			DatabaseShowViews(server.db)

		case "users", "usr":
			fmt.Println("Online users:")
			server.users.mutex.Lock()
			for _, user := range server.users.slice {
				if user.Online {
					fmt.Printf("  ID:%v - %v\n", user.Id, user.Username)
				}
			}
			server.users.mutex.Unlock()

		case "cleanup", "cln":
			removed := server.cleanupDummyUsers()
			for _, user := range removed {
				fmt.Printf("Removed user ID:%v - %v\n", user.Id, user.Username)
			}

		case "reload", "rel":
			fmt.Println("Command 'reload' it not implemented yet!")

		default:
			if command != "" {
				fmt.Printf("'%s' is not a valid command. Type 'help' to see available command.\n", command)
			} else {
				continue outer
			}
		}

		fmt.Println()
	}
}
