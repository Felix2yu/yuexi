package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(dbPath string) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	migrate()
}

func migrate() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS persons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 1,
			name TEXT NOT NULL,
			cycle_length INTEGER NOT NULL DEFAULT 28,
			period_length INTEGER NOT NULL DEFAULT 5,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_persons_user_id ON persons(user_id)`,
		`CREATE TABLE IF NOT EXISTS records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			person_id INTEGER NOT NULL,
			start_date DATE NOT NULL,
			end_date DATE,
			note TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_records_person_id ON records(person_id)`,
		`CREATE INDEX IF NOT EXISTS idx_records_start_date ON records(start_date)`,
		`CREATE TABLE IF NOT EXISTS notification_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 0,
			shoutrrr_url TEXT NOT NULL DEFAULT '',
			days_before INTEGER NOT NULL DEFAULT 3,
			last_notified TEXT DEFAULT '',
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_config_user ON notification_config(user_id)`,
		`CREATE TABLE IF NOT EXISTS daily_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			person_id INTEGER NOT NULL,
			date DATE NOT NULL,
			flow_level INTEGER DEFAULT 0,
			symptoms TEXT DEFAULT '',
			note TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(person_id, date),
			FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_daily_logs_person_date ON daily_logs(person_id, date)`,
	}

	for _, q := range queries {
		if _, err := DB.Exec(q); err != nil {
			log.Fatalf("Migration failed: %v\nQuery: %s", err, q)
		}
	}

	// Migration: add user_id columns to existing tables if missing
	migrateAddColumn("persons", "user_id", "INTEGER NOT NULL DEFAULT 1")
	migrateAddColumn("notification_config", "user_id", "INTEGER NOT NULL DEFAULT 1")

	// Migration: add weight and temperature columns to daily_logs
	migrateAddColumn("daily_logs", "weight", "REAL")
	migrateAddColumn("daily_logs", "temperature", "REAL")

	// Migration: sessions table
	if _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	_, _ = DB.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)")
	_, _ = DB.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)")

	// Migrate notification_config from single-row to per-user
	var count int
	DB.QueryRow("SELECT COUNT(*) FROM notification_config").Scan(&count)
	if count == 0 {
		DB.Exec("INSERT INTO notification_config (user_id, enabled, shoutrrr_url, days_before) VALUES (1, 0, '', 3)")
	}
}

func migrateAddColumn(table, column, typedef string) {
	var cnt int
	err := DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, column).Scan(&cnt)
	if err != nil || cnt == 0 {
		DB.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + typedef)
	}
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
