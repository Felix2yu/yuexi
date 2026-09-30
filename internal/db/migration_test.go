package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrateLegacyShoutrrrColumn verifies that a database created before the
// apprise-go migration (notification_config with a shoutrrr_url column) is
// upgraded in place without data loss.
func TestMigrateLegacyShoutrrrColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	// Build a pre-migration schema mirroring a real legacy database:
	// old notification_config layout plus the user_id column added later.
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE notification_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			enabled INTEGER NOT NULL DEFAULT 0,
			shoutrrr_url TEXT NOT NULL DEFAULT '',
			days_before INTEGER NOT NULL DEFAULT 3,
			last_notified TEXT DEFAULT '',
			user_id INTEGER NOT NULL DEFAULT 1
		)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO notification_config (user_id, enabled, shoutrrr_url, days_before) VALUES (1, 1, 'telegram://token@telegram?chats=42', 5)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	// Run the real migration path via Init, then restore the shared test DB.
	prev := DB
	defer func() {
		if DB != prev {
			DB.Close()
			DB = prev
		}
	}()
	Init(path)

	cfg := GetNotificationConfig(1)
	if !cfg.Enabled {
		t.Error("enabled = false, want true (legacy row lost)")
	}
	if cfg.NotifyURL != "telegram://token@telegram?chats=42" {
		t.Errorf("notify_url = %q, want legacy value preserved", cfg.NotifyURL)
	}
	if cfg.DaysBefore != 5 {
		t.Errorf("days_before = %d, want 5", cfg.DaysBefore)
	}

	// Save must work on the renamed column (upsert touches notify_url).
	if err := SaveNotificationConfig(1, NotificationConfig{Enabled: true, NotifyURL: "bark://example", DaysBefore: 2}); err != nil {
		t.Fatal(err)
	}
	if cfg := GetNotificationConfig(1); cfg.NotifyURL != "bark://example" {
		t.Errorf("notify_url after save = %q, want bark://example", cfg.NotifyURL)
	}
}
