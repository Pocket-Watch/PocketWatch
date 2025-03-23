package main

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"os"
)

func ConnectToDatabase(config DatabaseConfig) (*sql.DB, bool) {
	if !config.Enabled {
		return nil, true
	}

	host := config.Address
	port := config.Port
	user := config.Username
	password := config.Password
	name := config.Name
	connectionString := fmt.Sprintf("host=%v port=%v user=%v password=%v dbname=%v sslmode=disable", host, port, user, password, name)

	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to open the database: %v.\n", err)
		return nil, false
	}

	err = db.Ping()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to establish connection to the database: %v.\n", err)
		return nil, false
	}

	return db, true
}
