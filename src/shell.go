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

	input = strings.TrimSpace(input)
	input = strings.ToLower(input)

	fields := strings.Fields(input)

	var command string
	var argument string

	if len(fields) > 0 {
		command = fields[0]
	}

	if len(fields) > 1 {
		argument = strings.Join(fields[1:], " ")
	}

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

	outer: for {
		fmt.Print("> ")
		command, argument := readShellCommand()

		switch command {
		case "shutdown":
			EnableConsoleLogging()
			LogInfo("Shutting down the server.")
			os.Exit(0)

		case "exit":
			EnableConsoleLogging()
			LogInfo("Exiting interactive shell.")
			return

		case "help":
			fmt.Println("  shutdown - Exit the server")
			fmt.Println("  exit     - Exit the interactive shell")
			fmt.Println("  help     - Display this help prompt")
			fmt.Println("  version  - Print server version")
			fmt.Println("  uptime   - Print server uptime")
			fmt.Println("  loglevel - Set log level")
			fmt.Println("  sqlquery - Execute SQL query")

		case "version", "v":
			fmt.Printf("Server version: %v_%v\n", VERSION, BuildTime)

		case "uptime", "up":
			uptime := time.Now().Sub(startTime)
			fmt.Printf("Server uptime: %v\n", uptime)

		case "loglevel", "ll":
			loglevel, err := strconv.Atoi(argument)
			if err != nil {
				fmt.Printf("'%v' is not a valid log level", argument)
			} else {
				SetLogLevel(uint32(loglevel))
				fmt.Printf("Log level is now set to '%v'", LogLevelToString(uint32(loglevel)))
			}

		case "sqlquery", "sql":
			DatabaseSqlQuery(db, argument)

		default:
			if command != "" {
				fmt.Printf("'%s' is not a valid command. Type help to see available command.\n", command)
			} else {
				continue outer
			}
		}

		fmt.Println()
	}
}
