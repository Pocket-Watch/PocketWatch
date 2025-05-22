package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

func readCliCommand() string {
	reader := bufio.NewReader(os.Stdin)
	command, _ := reader.ReadString('\n')

	command = strings.TrimSpace(command)
	command = strings.ToLower(command)

	return command
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

func RunInteractiveCli(ctrlcChan chan os.Signal) {
	fmt.Println()
	fmt.Println()
	fmt.Println("PocketWatch Interactive CLI:")
	fmt.Println("  type 'help' to show available command.")
	fmt.Println()

	exitChan := make(chan bool, 1)
	go handleCtrlC(ctrlcChan, exitChan)
	defer func() { exitChan <- true }()

	for {
		fmt.Print("> ")
		command := readCliCommand()

		switch command {
		case "shutdown":
			os.Exit(0)

		case "exit":
			return

		case "help":
			fmt.Println("  shutdown - Exit the server")
			fmt.Println("  exit     - Exit the interactive CLI")
			fmt.Println("  help     - Display this help prompt")
			fmt.Println("  version  - Print server version")
			fmt.Println("  uptime   - Print server uptime")
			fmt.Println("  loglevel - Set log level")
			fmt.Println("  sql      - Execute SQL query")
			fmt.Println()

		case "version", "v":
			fmt.Printf("Server version: %v_%v\n", VERSION, BuildTime)
			fmt.Println()

		case "uptime", "up":
			uptime := time.Now().Sub(startTime)
			fmt.Printf("Server uptime: %v\n", uptime)
			fmt.Println()

		case "loglevel", "ll":
			fmt.Println("Not implemented yet")
			fmt.Println()

		case "sqlquery", "sql":
			fmt.Println("Not implemented yet")
			fmt.Println()

		default:
			if command != "" {
				fmt.Printf("'%s' is not a valid command. Type help to see available command.\n", command)
				fmt.Println()
			}
		}
	}
}
