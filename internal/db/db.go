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
		`CREATE TABLE IF NOT EXISTS persons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			cycle_length INTEGER NOT NULL DEFAULT 28,
			period_length INTEGER NOT NULL DEFAULT 5,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
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
			id INTEGER PRIMARY KEY CHECK (id = 1),
			enabled INTEGER NOT NULL DEFAULT 0,
			shoutrrr_url TEXT NOT NULL DEFAULT '',
			days_before INTEGER NOT NULL DEFAULT 3,
			last_notified TEXT DEFAULT ''
		)`,
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
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
