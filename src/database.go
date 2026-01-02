package main

import (
	"cmp"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

const SQL_MIGRATIONS_DIR = "sql/"
const TABLE_USERS = "users"

// Used as an ID seeder when database disabled.
var idSeeder atomic.Uint64

type DbMigration struct {
	number uint
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

	LogInfo("Connection to the database established on port %v.", port)
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

func migCompareByNumber(a, b DbMigration) int {
	return cmp.Compare(a.number, b.number)
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

		success, number := parseMigrationNumber(name)
		if !success {
			LogError("Failed to parse migration number from %v", name)
			return migrations, false
		}
		mig := DbMigration{
			number: number,
			name:   name,
			query:  string(bytes),
		}

		migrations = append(migrations, mig)
	}

	slices.SortFunc(migrations, migCompareByNumber)
	if !migrationsOk(migrations) {
		return migrations, false
	}
	return migrations, true
}

func migrationsOk(migrations []DbMigration) bool {
	if len(migrations) > 0 && migrations[0].number != 1 {
		LogError("Migrations should be numbered starting at 1. Example: 001-init_tables.sql")
		return false
	}
	for i := 0; i < len(migrations)-1; i++ {
		left := migrations[i]
		right := migrations[i+1]
		if left.number == right.number {
			LogError("Migration number %v is repeated.", left.number)
			return false
		}
		if left.number != right.number-1 {
			LogError("Gap found, migration numbers should be sequential, yet %v skips to %v", left.number, right.number)
			return false
		}
	}
	return true
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
		// Currently full names of migrations are stored. Migrate migrations?
		success, number := parseMigrationNumber(mig.name)
		if !success {
			LogError("Failed to parse migration number from %v", mig.name)
			return applied, false
		}
		mig.number = number
		applied = append(applied, mig)
	}

	if err := rows.Err(); err != nil {
		LogError("Iterating over rows of the database migration table failed: %v.", err)
		return applied, false
	}

	slices.SortFunc(applied, migCompareByNumber)
	if !migrationsOk(applied) {
		return applied, false
	}
	return applied, true
}

func parseMigrationNumber(name string) (bool, uint) {
	dash := strings.Index(name, "-")
	if dash == -1 {
		return false, 0
	}
	number, err := strconv.ParseUint(name[0:dash], 10, 64)
	if err != nil {
		return false, 0
	}
	return true, uint(number)
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

func databaseMoveUserToTable(db *sql.DB, fromTable string, toTable string, user User) bool {
	query := fmt.Sprintf(`
		WITH moved_users AS (
			DELETE FROM %v
			WHERE id = $1
			RETURNING *
		)
		INSERT INTO %v
		SELECT * FROM moved_users
	`, fromTable, toTable)

	res, err := db.Exec(query, user.Id)
	LogDebug("%v database user move from %v to %v: %v ---- %v", user.Id, fromTable, toTable, res, err)

	if err != nil {
		return false
	}

	return true
}

func DatabaseLoadUsers(db *sql.DB) (*Users, bool) {
	users := makeUsers()

	if db == nil {
		return users, true
	}

	query := "SELECT * FROM users"
	rows, err := db.Query(query)
	if err != nil {
		LogError("Failed to load users from the database. An error occurred while querying database 'users' table: %v", err)
		return users, false
	}

	defer rows.Close()

	for rows.Next() {
		var user User

		err := rows.Scan(&user.Id, &user.Username, &user.Avatar, &user.token, &user.CreatedAt, &user.LastUpdate, &user.LastOnline)
		if err != nil {
			LogError("Failed to load users from the database. An error occurred while reading a user from the database 'users' row: %v", err)
			return users, false
		}

		users.slice = append(users.slice, user)
	}

	err = rows.Err()
	if err != nil {
		LogError("Failed to load users from the database. An error occurred while iterating 'users' rows: %v", err)
		return users, false
	}

	return users, true
}

func DatabaseAddUser(db *sql.DB, user *User) error {
	if db == nil {
		user.Id = idSeeder.Add(1)
		return nil
	}

	var lastInsertId uint64
	query := `
		INSERT INTO users (
			username, avatar_path, token, created_at, last_update, last_online
		) VALUES ($1, $2, $3, $4, $5, $6) 
		RETURNING id
	`

	row := db.QueryRow(query, user.Username, user.Avatar, user.token, user.CreatedAt, user.LastUpdate, user.LastOnline)
	err := row.Err()
	if err != nil {
		LogError("Failed to save user token:%v to the database: %v", user.token, err)
		return err
	}

	err = row.Scan(&lastInsertId)
	if err != nil {
		LogError("Failed to get inserted id for user token:%v from the database: %v", user.token, err)
		return err
	}

	user.Id = lastInsertId
	return nil
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

	_, err := db.Exec("UPDATE users SET username = $1, avatar_path = $2, last_update = $3 WHERE id = $4", user.Username, user.Avatar, user.LastUpdate, user.Id)
	if err != nil {
		LogError("Failed to update user id:%v in the database: %v", user.Id, err)
		return false
	}

	return true
}

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

func DatabaseShowTables(db *sql.DB) {
	query := "SELECT * FROM pg_catalog.pg_tables WHERE schemaname = 'public'"
	DatabaseSqlQuery(db, query)
}

func DatabaseShowViews(db *sql.DB) {
	query := "SELECT table_name FROM information_schema.views WHERE table_schema = 'public'"
	DatabaseSqlQuery(db, query)
}

func DatabaseSqlQuery(db *sql.DB, query string) {
	if db == nil {
		fmt.Printf("Database is not running\n")
		return
	}

	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("Failed to execute SQL query: %v\n", err)
		return
	}
	defer rows.Close()

	columnNames, _ := rows.Columns()
	columnCount := len(columnNames)
	columns := make([]any, columnCount)

	for i := range columnNames {
		columns[i] = new(sql.NullString)
	}

	data := make([]string, 0)

	for rows.Next() {
		err = rows.Scan(columns...)
		if err != nil {
			fmt.Printf("Failed to read row query: %v\n", err)
			return
		}

		for _, column := range columns {
			value := *column.(*sql.NullString)
			data = append(data, value.String)
		}
	}

	prettyTable := GeneratePrettyTable(columnNames, data)
	fmt.Print(prettyTable)
}

func DatabasePrintTableLayout(db *sql.DB, tableName string) {
	if db == nil {
		fmt.Printf("Database is not running\n")
		return
	}

	query := fmt.Sprintf("SELECT * from %v;", tableName)
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("Failed to find table: %v\n", err)
		return
	}
	defer rows.Close()

	columnNames, _ := rows.Columns()
	fmt.Printf("Layout for table '%v'\n", tableName)

	prettyTable := GeneratePrettyTable(columnNames, nil)
	fmt.Print(prettyTable)
}

func databaseEntryAdd(db *sql.DB, entry *Entry) error {
	if db == nil {
		entry.Id = idSeeder.Add(1)
		for i := range entry.Subtitles {
			entry.Subtitles[i].Id = idSeeder.Add(1)
		}

		return nil
	}

	// Do a transaction here

	query := `
		INSERT INTO entries (
			url, title, user_id, use_proxy, referer_url, source_url, thumbnail, created_at, last_set_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
		RETURNING id
	`

	row := db.QueryRow(query, entry.Url, entry.Title, entry.UserId, entry.UseProxy, entry.RefererUrl, entry.SourceUrl, entry.Thumbnail, entry.CreatedAt, entry.LastSetAt)
	err := row.Err()
	if err != nil {
		LogError("Failed to add entry to the database: %v", err)
		return err
	}

	var lastEntryId uint64
	err = row.Scan(&lastEntryId)
	if err != nil {
		LogError("Failed to get inserted entry id from the database: %v", err)
		return err
	}

	entry.Id = lastEntryId

	for i := range entry.Subtitles {
		if err := DatabaseSubtitleAdd(db, entry.Id, &entry.Subtitles[i]); err != nil {
			return err
		}
	}

	return nil
}

func databaseEntryDelete(db *sql.DB, entryId uint64) bool {
	_, err := db.Exec("DELETE FROM entries WHERE id = $1", entryId)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabaseGetAutoplay(db *sql.DB) bool {
	if db == nil {
		return false
	}

	var autoplay sql.NullBool
	db.QueryRow("SELECT autoplay FROM player_state").Scan(&autoplay)
	return autoplay.Bool
}

func DatabaseGetLooping(db *sql.DB) bool {
	if db == nil {
		return false
	}

	var looping sql.NullBool
	db.QueryRow("SELECT looping FROM player_state").Scan(&looping)
	return looping.Bool
}

func DatabaseGetTimestamp(db *sql.DB) float64 {
	if db == nil {
		return 0.0
	}

	var timestamp sql.NullFloat64
	db.QueryRow("SELECT timestamp FROM player_state").Scan(&timestamp)
	return timestamp.Float64
}

func DatabaseSetAutoplay(db *sql.DB, autoplay bool) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE player_state SET autoplay = $1", autoplay)
	return err != nil
}

func DatabaseSetLooping(db *sql.DB, looping bool) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE player_state SET looping = $1", looping)
	return err != nil
}

func DatabaseSetTimestamp(db *sql.DB, timestamp float64) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE player_state SET timestamp = $1", timestamp)
	return err != nil
}

func DatabaseCurrentEntryGet(db *sql.DB) (Entry, bool) {
	if db == nil {
		return Entry{}, false
	}

	query := `
		SELECT e.*, s.* FROM entries e
		LEFT JOIN subtitles s ON e.id = s.entry_id
		WHERE e.id = (SELECT entry_id FROM current_entry LIMIT 1);
	`

	rows, err := db.Query(query)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return Entry{}, false
	}

	defer rows.Close()

	var entry Entry
	entry.Subtitles = make([]Subtitle, 0)

	for rows.Next() {
		var subId sql.NullInt64
		var entryId sql.NullInt64
		var subName sql.NullString
		var subUrl sql.NullString
		var subShift sql.NullFloat64

		err := rows.Scan(
			&entry.Id, &entry.Url, &entry.Title, &entry.UserId, &entry.UseProxy, &entry.RefererUrl, &entry.SourceUrl, &entry.Thumbnail, &entry.CreatedAt, &entry.LastSetAt,
			&subId, &entryId, &subName, &subUrl, &subShift,
		)

		if err != nil {
			LogError("SQL query failed: %v", err)
			return Entry{}, false
		}

		if subId.Valid {
			sub := Subtitle{
				Id:    uint64(subId.Int64),
				Name:  subName.String,
				Url:   subUrl.String,
				Shift: subShift.Float64,
			}

			entry.Subtitles = append(entry.Subtitles, sub)
		}
	}

	if err := rows.Err(); err != nil {
		LogError("SQL query failed: %v", err)
		return Entry{}, false
	}

	return entry, true
}

func DatabaseCurrentEntrySet(db *sql.DB, entry *Entry) error {
	if db == nil {
		entry.Id = idSeeder.Add(1)
		for i := range entry.Subtitles {
			entry.Subtitles[i].Id = idSeeder.Add(1)
		}

		return nil
	}

	_, err := db.Exec("DELETE FROM entries WHERE id = (SELECT entry_id FROM current_entry LIMIT 1)")
	if err != nil {
		LogError("SQL query failed: %v", err)
		return err
	}

	if err := databaseEntryAdd(db, entry); err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO current_entry (entry_id) VALUES ($1)", entry.Id)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return err
	}

	return nil
}

func DatabaseCurrentEntryUpdateTitle(db *sql.DB, title string) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE entries SET title = $1 WHERE id = (SELECT entry_id FROM current_entry LIMIT 1)", title)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabaseSubtitleDelete(db *sql.DB, subId uint64) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("DELETE FROM subtitles WHERE id = $1", subId)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabaseSubtitleUpdate(db *sql.DB, id uint64, name string) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE subtitles SET name = $1 WHERE id = $2", name, id)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabaseSubtitleAdd(db *sql.DB, entryId uint64, sub *Subtitle) error {
	if db == nil {
		sub.Id = idSeeder.Add(1)
		return nil
	}

	query := `
		INSERT INTO subtitles (
			entry_id, name, url, shift
		) VALUES ($1, $2, $3, $4) 
		RETURNING id
	`

	row := db.QueryRow(query, entryId, sub.Name, sub.Url, sub.Shift)
	err := row.Err()
	if err != nil {
		LogError("Failed to add subtitle to the database: %v", err)
		return err
	}

	var lastSubId uint64
	err = row.Scan(&lastSubId)
	if err != nil {
		LogError("Failed to get inserted subtitle id from the database: %v", err)
		return err
	}

	sub.Id = lastSubId
	return nil
}

func DatabaseSubtitleShift(db *sql.DB, id uint64, shift float64) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE subtitles SET shift = $1 WHERE id = $2", shift, id)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabasePlaylistGet(db *sql.DB) ([]Entry, bool) {
	if db == nil {
		return []Entry{}, true
	}

	query := `
		SELECT e.*, s.* FROM playlist p
		JOIN entries e ON p.entry_id = e.id
		LEFT JOIN subtitles s ON e.id = s.entry_id
		ORDER BY p.added_at;
	`

	rows, err := db.Query(query)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return []Entry{}, false
	}

	defer rows.Close()

	entries := make([]Entry, 0)
	prev := Entry{}

	for rows.Next() {
		var subId sql.NullInt64
		var entryId sql.NullInt64
		var subName sql.NullString
		var subUrl sql.NullString
		var subShift sql.NullFloat64

		temp := Entry{}
		err := rows.Scan(
			&temp.Id, &temp.Url, &temp.Title, &temp.UserId, &temp.UseProxy, &temp.RefererUrl, &temp.SourceUrl, &temp.Thumbnail, &temp.CreatedAt, &temp.LastSetAt,
			&subId, &entryId, &subName, &subUrl, &subShift,
		)

		if err != nil {
			LogError("SQL query failed: %v", err)
			return []Entry{}, false
		}

		if prev.Id == 0 {
			prev = temp
		} else if temp.Id != prev.Id {
			entries = append(entries, prev)
			prev = temp
		}

		if subId.Valid {
			sub := Subtitle{
				Id:    uint64(subId.Int64),
				Name:  subName.String,
				Url:   subUrl.String,
				Shift: subShift.Float64,
			}

			prev.Subtitles = append(prev.Subtitles, sub)
		}
	}

	if prev.Id != 0 {
		entries = append(entries, prev)
	}

	return entries, true
}

func DatabasePlaylistAdd(db *sql.DB, entry *Entry) error {
	if db == nil {
		entry.Id = idSeeder.Add(1)
		for i := range entry.Subtitles {
			entry.Subtitles[i].Id = idSeeder.Add(1)
		}

		return nil
	}

	if err := databaseEntryAdd(db, entry); err != nil {
		return err
	}

	_, err := db.Exec("INSERT INTO playlist (entry_id) VALUES ($1)", entry.Id)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return err
	}

	return nil
}

func DatabasePlaylistAddMany(db *sql.DB, entries []Entry) error {
	for i := range entries {
		if err := DatabasePlaylistAdd(db, &entries[i]); err != nil {
			return err
		}
	}

	return nil
}

func DatabasePlaylistDelete(db *sql.DB, entryId uint64) bool {
	if db == nil {
		return true
	}

	return databaseEntryDelete(db, entryId)
}

func DatabasePlaylistClear(db *sql.DB) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("DELETE FROM entries WHERE id IN (SELECT entry_id FROM playlist)")
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabasePlaylistUpdate(db *sql.DB, id uint64, title string, url string) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE entries SET title = $1, url = $2 WHERE id = $3", title, url, id)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabaseHistoryGet(db *sql.DB) ([]Entry, bool) {
	if db == nil {
		return []Entry{}, true
	}

	query := `
		SELECT e.*, s.* FROM history h
		JOIN entries e ON h.entry_id = e.id
		LEFT JOIN subtitles s ON e.id = s.entry_id
		ORDER BY h.added_at;
	`

	rows, err := db.Query(query)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return []Entry{}, false
	}

	defer rows.Close()

	entries := make([]Entry, 0)
	prev := Entry{}

	for rows.Next() {
		var subId sql.NullInt64
		var entryId sql.NullInt64
		var subName sql.NullString
		var subUrl sql.NullString
		var subShift sql.NullFloat64

		temp := Entry{}
		err := rows.Scan(
			&temp.Id, &temp.Url, &temp.Title, &temp.UserId, &temp.UseProxy, &temp.RefererUrl, &temp.SourceUrl, &temp.Thumbnail, &temp.CreatedAt, &temp.LastSetAt,
			&subId, &entryId, &subName, &subUrl, &subShift,
		)

		if err != nil {
			LogError("SQL query failed: %v", err)
			return []Entry{}, false
		}

		if prev.Id == 0 {
			prev = temp
		} else if temp.Id != prev.Id {
			entries = append(entries, prev)
			prev = temp
		}

		if subId.Valid {
			sub := Subtitle{
				Id:    uint64(subId.Int64),
				Name:  subName.String,
				Url:   subUrl.String,
				Shift: subShift.Float64,
			}

			prev.Subtitles = append(prev.Subtitles, sub)
		}
	}

	if prev.Id != 0 {
		entries = append(entries, prev)
	}

	return entries, true
}

func DatabaseHistoryAdd(db *sql.DB, entry *Entry) error {
	if db == nil {
		entry.Id = idSeeder.Add(1)
		for i := range entry.Subtitles {
			entry.Subtitles[i].Id = idSeeder.Add(1)
		}

		return nil
	}

	if err := databaseEntryAdd(db, entry); err != nil {
		return err
	}

	_, err := db.Exec("INSERT INTO history (entry_id) VALUES ($1)", entry.Id)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return err
	}

	return nil
}

func DatabaseHistoryDelete(db *sql.DB, entryId uint64) bool {
	if db == nil {
		return true
	}

	return databaseEntryDelete(db, entryId)
}

func DatabaseHistoryClear(db *sql.DB) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("DELETE FROM entries WHERE id IN (SELECT entry_id FROM history)")
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}

func DatabaseMessageGet(db *sql.DB, count int, skip int) ([]ChatMessage, bool) {
	if db == nil {
		return []ChatMessage{}, true
	}

	queryText := `
		SELECT * FROM (
			SELECT * FROM messages
			ORDER BY created_at DESC
			LIMIT %v OFFSET %v
		) rows ORDER BY created_at ASC;
	`

	query := fmt.Sprintf(queryText, count, skip)
	// query := fmt.Sprintf("SELECT * from messages ORDER BY created_at LIMIT %v OFFSET %v", count, skip)

	rows, err := db.Query(query)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return []ChatMessage{}, false
	}

	messages := make([]ChatMessage, 0)

	for rows.Next() {
		var temp ChatMessage

		err := rows.Scan(&temp.Id, &temp.Content, &temp.CreatedAt, &temp.EditedAt, &temp.UserId)
		if err != nil {
			LogError("SQL query failed: %v", err)
			return []ChatMessage{}, false
		}

		messages = append(messages, temp)
	}

	return messages, true
}

func DatabaseMessageAdd(db *sql.DB, message *ChatMessage) error {
	if db == nil {
		message.Id = idSeeder.Add(1)
		return nil
	}

	var lastInsertId uint64
	query := `
		INSERT INTO messages (
			content, created_at, edited_at, user_id
		) VALUES ($1, $2, $3, $4) 
		RETURNING id
	`

	row := db.QueryRow(query, message.Content, message.CreatedAt, message.EditedAt, message.UserId)

	err := row.Err()
	if err != nil {
		LogError("Failed to save messages for user id:%v because of: %v", message.UserId, err)
		return err
	}

	err = row.Scan(&lastInsertId)
	if err != nil {
		LogError("Failed to get inserted message id for user id:%v from the database: %v", message.UserId, err)
		return err
	}

	message.Id = lastInsertId
	return nil
}

func DatabaseMessageEdit(db *sql.DB, message ChatMessage) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("UPDATE messages SET content = $1, edited_at = $2 WHERE id = $3", message.Content, message.EditedAt, message.Id)
	if err != nil {
		LogError("Failed to update message id:%v in the database: %v", message.Id, err)
		return false
	}

	return false
}

func DatabaseMessageDelete(db *sql.DB, messageId uint64) bool {
	if db == nil {
		return true
	}

	_, err := db.Exec("DELETE FROM messages WHERE id = $1", messageId)
	if err != nil {
		LogError("SQL query failed: %v", err)
		return false
	}

	return true
}
