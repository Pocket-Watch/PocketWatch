package main

import (
	"bufio"
	"database/sql"
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

func RunInteractiveShell(ctrlcChan chan os.Signal, db *sql.DB) {
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
			LogInfo("Shutting down the server.")
			os.Exit(0)

		case "quit", "q":
			EnableConsoleLogging()
			LogInfo("Exiting interactive shell.")
			return

		case "help", "h":
			fmt.Println("  shutdown      - Exit the server")
			fmt.Println("  quit,     q   - Quit the interactive shell")
			fmt.Println("  help,     h   - Display this help prompt")
			fmt.Println("  version,  v   - Print server version")
			fmt.Println("  uptime,   up  - Print server uptime")
			fmt.Println("  loglevel, log - Print or set the log level")
			fmt.Println("  sqlquery, sql - Execute SQL query")
			fmt.Println("  sqltable, tlb - Print layout a SQL table")
			fmt.Println("  users,    usr - Show number of active users")

		case "version", "v":
			fmt.Printf("Server version: %v_%v\n", VERSION, BuildTime)

		case "uptime", "up":
			uptime := time.Now().Sub(startTime)
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
			DatabaseSqlQuery(db, argument)

		case "sqltable", "tbl":
			DatabasePrintTableLayout(db, argument)

		case "users", "usr":
			fmt.Println("Command users it not implemented yet!")

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
