package main

import (
	"cmp"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	_ "github.com/lib/pq"
)

const SQL_MIGRATIONS_DIR = "sql/"

// TODO(kihau): 
// Migration system robustness:
//   - Do not sort filenames alphabetically, instead sort by migration number
//   - Make sure multiple migrations with the same number cannot exist at the same time (ex. 001-create_user.sql and 001-create_messages.sql)
//   - Make sure there are no gaps between migration numbers (ex. gap between files 001-create_user.sql and 003-alter_tables.sql, 002 missing)
//   - (Maybe) Implement system to upgrade and downgrade migrations (ex. 01-create_user.up.sql and 01-create_user.down.sql)
//   - Add migration grouping for migrations, so fresh database setups won't need to apply all migrations sequentially
//     (ex. 001-create_user.migrate.sql, 002-alter_users.migrate.sql, ..., 007-create_messages.migrate.sql -> 007-create_data.init.sql which is equivalent to all migrations up to 007???)
type DbMigration struct {
	// number uint
	// downgrade bool
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
		LogError("Failed to open the database: %v.", err)
		return nil, false
	}

	err = db.Ping()
	if err != nil {
		LogError("Failed to establish connection to the database: %v.", err)
		return nil, false
	}

	LogInfo("Connection to the database established.")
	return db, true
}

func createMigrationsTable(db *sql.DB) bool {
	appliedTable := `
		CREATE TABLE IF NOT EXISTS migrations (
			name        VARCHAR(255) NOT NULL UNIQUE,
			query       TEXT         NOT NULL,
			applied_at  TIMESTAMP    NOT NULL
		)
	`

	_, err := db.Exec(appliedTable)
	if err != nil {
		LogError("Failed to execute table creation query for the 'migrations' table: %v", err)
		return false
	}

	return true
}

func migCompare(a, b DbMigration) int {
	return cmp.Compare(a.name, b.name)
}

func loadLocalMigrations() ([]DbMigration, bool) {
	migrations := make([]DbMigration, 0)

	entries, err := os.ReadDir(SQL_MIGRATIONS_DIR)
	if err != nil {
		LogError("Failed to read directory '%v' with SQL migrations: %v", SQL_MIGRATIONS_DIR, err)
		return migrations, false
	}

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
			LogError("Failed to read migration file %v: %v", name, err)
			return migrations, false
		}

		mig := DbMigration{
			name:  name,
			query: string(bytes),
		}

		migrations = append(migrations, mig)
	}

	slices.SortFunc(migrations, migCompare)
	return migrations, true
}

func loadAppliedMigrations(db *sql.DB) ([]DbMigration, bool) {
	applied := make([]DbMigration, 0)

	rows, err := db.Query("SELECT * FROM migrations")
	if err != nil {
		LogError("Failed to query database migration table: %v", err)
		return applied, false
	}

	defer rows.Close()

	for rows.Next() {
		var mig DbMigration
		var appliedAt time.Time

		err := rows.Scan(&mig.name, &mig.query, &appliedAt)
		if err != nil {
			LogError("Failed to read migration from the database migration table: %v", err)
			return applied, false
		}

		applied = append(applied, mig)
	}

	if err := rows.Err(); err != nil {
		LogError("Iterating over rows of the database migration table failed: %v.", err)
		return applied, false
	}

	slices.SortFunc(applied, migCompare)
	return applied, true
}

func applyMigration(db *sql.DB, mig DbMigration) bool {
	_, err := db.Exec(mig.query)
	if err != nil {
		LogError("Database migration failed. Could not apply local migration '%v': %v.", mig.name, err)
		return false
	}

	_, err = db.Exec("INSERT INTO migrations VALUES ($1, $2, $3)", mig.name, mig.query, time.Now())
	if err != nil {
		LogError("Failed to insert local migration %v into the applied migrations table: %v.", mig.name, err)
		return false
	}

	LogInfo("Database migration '%v' successfully applied.", mig.name)
	return true
}

func MigrateDatabase(db *sql.DB) bool {
	if db == nil {
		return true
	}

	if !createMigrationsTable(db) {
		return false
	}

	localMigrations, ok := loadLocalMigrations()
	if !ok {
		return false
	}

	appliedMigrations, ok := loadAppliedMigrations(db)
	if !ok {
		return false
	}

	if len(appliedMigrations) > len(localMigrations) {
		LogError("Number of applied migrations (%v) is greater than those available locally (%v).", len(appliedMigrations), len(localMigrations))
		return false
	}

	// Validate that applied migrations match those available locally.
	for i, applied := range appliedMigrations {
		local := localMigrations[i]

		if local.query != applied.query {
			LogError("SQL query for local migration '%v' is different than the migration '%v' applied to the database.\n-------- Local migration query: -------\n%v\v-------- Applied migration query: -------\n%v\n-----------------------------------------\n", local.name, applied.name, local.query, applied.query)
			return false
		}
	}

	if len(appliedMigrations) == len(localMigrations) {
		LogInfo("All %v applied migrations are up-to-date.", len(appliedMigrations))
		return true
	}

	notAppliedMigrations := localMigrations[len(appliedMigrations):]
	for _, mig := range notAppliedMigrations {
		if !applyMigration(db, mig) {
			return false
		}
	}

	return true
}

// Remove temporary 'dummy' accounts of users inactive for more than 1 days and without any user profile updates.
func DatabaseRemoveDummyUsers(db *sql.DB) bool {
	if db == nil {
		return true
	}

	now := time.Now()
	dayAgo := now.AddDate(0, 0, -1)

	// NOTE(kihau): Maybe just move them to archive instead?
	query := "DELETE FROM users WHERE last_online < $1 AND last_update = created_at"
	result, err := db.Exec(query, dayAgo)
	if err != nil {
		LogError("Failed to execute query to remove dummy users: %v", err)
		return false
	}

	removed, _ := result.RowsAffected()
	if removed > 0 {
		LogInfo("Removed %v dummy user accounts inactive for more than 1 day.", removed)
	}

	return true
}

// TODO(kihau): Run this function on server startup and once every day?
func DatabaseArchiveInactiveUsers(db *sql.DB) bool {
	if db == nil {
		return true
	}

	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	query := `
		WITH moved_users AS (
			DELETE FROM users
			WHERE last_online < $1
			RETURNING *
		)
		INSERT INTO inactive_users
		SELECT * FROM moved_users

	`

	res, err := db.Exec(query, weekAgo)
	LogDebug("%v database archive: %v ---- %v", weekAgo, res, err)

	return true
}

func DatabaseMoveUserToArchive(db *sql.DB, user User) bool {
	if db == nil {
		return true
	}

	query := `
		WITH user AS (
			DELETE FROM users
			WHERE id = $1
			RETURNING *
		)
		INSERT INTO inactive_users
		SELECT * FROM user
	`

	res, err := db.Exec(query, user.Id)
	LogDebug("%v database archive: %v ---- %v", user.Id, res, err)

	if err != nil {
		return false
	}

	return true
}

// TODO(kihau):
//
//	Archive users with more than 1 week of inactivity. Archived users will not appear in the user list until they open the room.
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

	// Load highest id from the archived users...
	// Move archived users on connect...

	// NOTE(kihau): This is kind of hacky, maybe seed counter should be stored in the database?
	var maxId uint64 = 1

	for rows.Next() {
		var user User

		err := rows.Scan(&user.Id, &user.Username, &user.Avatar, &user.token, &user.createdAt, &user.lastUpdate, &user.lastOnline)
		if err != nil {
			LogError("Failed to load users from the database. An error occurred while reading a user from the database 'users' row: %v", err)
			return users, false
		}

		if user.Id >= maxId {
			maxId = user.Id + 1
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

	_, err := db.Exec("INSERT INTO users VALUES ($1, $2, $3, $4, $5, $6, $7)", user.Id, user.Username, user.Avatar, user.token, user.createdAt, user.lastUpdate, user.lastOnline)
	if err != nil {
		LogError("Failed to save user id:%v to the database: %v", user.Id, err)
		return false
	}

	return true
}

func DatabaseDeleteUser(db *sql.DB, user User) bool {
	if db == nil {
		return true
	}

	// NOTE(kihau): Instead of removing the users, move them to deleted_users table?
	_, err := db.Exec("DELETE FROM users WHERE id = $1", user.Id)
	if err != nil {
		LogError("Failed to delete user id:%v in the database: %v", user.Id, err)
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

// NOTE(kihau): User last online is a little bit faulty due to how connect and disconnect works.
func DatabaseUpdateUserLastOnline(db *sql.DB, id uint64, lastOnline time.Time) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE users SET last_online = $1 WHERE id = $2", lastOnline, id)
	if err != nil {
		LogError("Failed to update last online for user id:%v in the database: %v", id, err)
		return false
	}

	return true
}
