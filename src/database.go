package main

import (
	"cmp"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	_ "github.com/lib/pq"
)

const SQL_MIGRATIONS_DIR = "sql/"

type DbMigration struct {
	name  string
	query string
}

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

func MigrateDatabase(db *sql.DB) bool {
	if db == nil {
		return true
	}

	entries, err := os.ReadDir(SQL_MIGRATIONS_DIR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to read directory '%v' with sql migrations: %v\n", SQL_MIGRATIONS_DIR, err)
		return false
	}

	migrations := make([]DbMigration, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}

		migrationPath := filepath.Join(SQL_MIGRATIONS_DIR, name)
		bytes, err := os.ReadFile(migrationPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to read migration file %v: %v\n", name, err)
			return false
		}

		mig := DbMigration{
			name:  name,
			query: string(bytes),
		}

		migrations = append(migrations, mig)
	}

	compareFunc := func(a, b DbMigration) int {
		return cmp.Compare(a.name, b.name)
	}

	slices.SortFunc(migrations, compareFunc)

	// TODO: Apply migrations only once and save what migrations have already been applied.
	for _, mig := range migrations {
		_, err = db.Exec(mig.query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to execute migration query for %v: %v\n", mig.name, err.Error())
			return false
		}
	}

	return true
}

func DatabaseLoadUsers(db *sql.DB) (*Users, bool) {
	users := makeUsers()

	if db == nil {
		return users, true
	}

	rows, err := db.Query("SELECT * FROM users")
	if err != nil {
		LogError("Failed to load users from the database. An error occurred while querying database 'users' table: %v", err)
		return users, false
	}

	defer rows.Close()

	var maxId uint64 = 1

	for rows.Next() {
		var user User

		err := rows.Scan(&user.Id, &user.Username, &user.Avatar, &user.token, &user.created, &user.lastUpdate)
		if err != nil {
			LogError("Failed to load users from the database. An error occurred while reading a user from the database 'users' row: %v", err)
			return users, false
		}

		if user.Id > maxId {
			maxId = user.Id
		}

		users.slice = append(users.slice, user)
	}

	err = rows.Err()
	if err != nil {
		LogError("Failed to load users from the database. An error occurred while iterating 'users' rows: %v", err)
		return users, false
	}

	users.idCounter = maxId
	return users, true
}

func DatabaseAddUser(db *sql.DB, user User) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("INSERT INTO users VALUES ($1, $2, $3, $4, $5, $6)", user.Id, user.Username, user.Avatar, user.token, user.created, user.lastUpdate)
	if err != nil {
		LogError("Failed to save user id:%v to the database: %v", user.Id, err)
		return false
	}

	return true
}

func DatabaseUpdateUser(db *sql.DB, user User) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE users SET username = $1, avatar_path = $2, last_update = $3 WHERE id = $4", user.Username, user.Avatar, user.lastUpdate, user.Id)
	if err != nil {
		LogError("Failed to update user id:%v in the database: %v", user.Id, err)
		return false
	}

	return true
}
