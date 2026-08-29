package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain spins up an isolated SQLite database for the db package tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yuexi-db-test")
	if err != nil {
		panic(err)
	}
	Init(filepath.Join(dir, "test.db"))
	code := m.Run()
	Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestNormalizeDate(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"2024-01-01":      "2024-01-01",
		"2024-01-01T10:00": "2024-01-01",
		"2024-01-01T23:59:59Z": "2024-01-01",
	}
	for in, want := range cases {
		if got := normalizeDate(in); got != want {
			t.Errorf("normalizeDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUserCRUD(t *testing.T) {
	u, err := CreateUser("alice", "hash1")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("expected non-zero user id")
	}
	got, err := GetUser(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "alice" {
		t.Errorf("username = %s, want alice", got.Username)
	}
	byName, err := GetUserByUsername("alice")
	if err != nil {
		t.Fatal(err)
	}
	if byName.ID != u.ID {
		t.Errorf("GetUserByUsername id mismatch")
	}
	if err := UpdateUserPassword(u.ID, "hash2"); err != nil {
		t.Fatal(err)
	}
	updated, _ := GetUser(u.ID)
	if updated.PasswordHash != "hash2" {
		t.Errorf("password not updated: %s", updated.PasswordHash)
	}
	if UserCount() < 1 {
		t.Error("expected at least 1 user")
	}
}

func TestPersonCRUD(t *testing.T) {
	u, _ := CreateUser("person-owner", "h")
	p, err := CreatePerson(u.ID, "Zoe", 29, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetPerson(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Zoe" || got.CycleLength != 29 || got.PeriodLength != 4 {
		t.Errorf("unexpected person: %+v", got)
	}
	if err := UpdatePerson(p.ID, "Zoe R", 30, 5); err != nil {
		t.Fatal(err)
	}
	got, _ = GetPerson(p.ID)
	if got.Name != "Zoe R" || got.CycleLength != 30 {
		t.Errorf("update failed: %+v", got)
	}
	persons, err := GetPersonsByUser(u.ID)
	if err != nil || len(persons) != 1 {
		t.Fatalf("GetPersonsByUser = %v, %v", persons, err)
	}
	all, err := GetAllPersons()
	if err != nil || len(all) == 0 {
		t.Fatalf("GetAllPersons = %v, %v", all, err)
	}
	if err := DeletePerson(p.ID); err != nil {
		t.Fatal(err)
	}
	persons, _ = GetPersonsByUser(u.ID)
	if len(persons) != 0 {
		t.Errorf("expected person deleted, got %d", len(persons))
	}
}

func TestRecordCRUD(t *testing.T) {
	u, _ := CreateUser("record-owner", "h")
	p, _ := CreatePerson(u.ID, "Mia", 28, 5)
	end := "2024-03-05"
	r, err := CreateRecord(p.ID, "2024-03-01", &end, "note")
	if err != nil {
		t.Fatal(err)
	}
	if r.EndDate == nil || *r.EndDate != "2024-03-05" {
		t.Errorf("end date not stored: %+v", r)
	}
	byPerson, err := GetRecordsByPerson(p.ID)
	if err != nil || len(byPerson) != 1 {
		t.Fatalf("GetRecordsByPerson = %v, %v", byPerson, err)
	}
	byMonth, err := GetRecordsByPersonAndMonth(p.ID, 2024, 3)
	if err != nil || len(byMonth) != 1 {
		t.Fatalf("GetRecordsByPersonAndMonth = %v, %v", byMonth, err)
	}
	if err := UpdateRecord(r.ID, "2024-03-02", &end, "updated"); err != nil {
		t.Fatal(err)
	}
	updated, _ := getRecord(r.ID)
	if updated.StartDate != "2024-03-02" || updated.Note != "updated" {
		t.Errorf("update failed: %+v", updated)
	}
	all, err := GetAllRecords()
	if err != nil || len(all) == 0 {
		t.Fatalf("GetAllRecords = %v, %v", all, err)
	}
	byUser, err := GetRecordsByUser(u.ID)
	if err != nil || len(byUser) != 1 {
		t.Fatalf("GetRecordsByUser = %v, %v", byUser, err)
	}
	if err := DeleteRecord(r.ID); err != nil {
		t.Fatal(err)
	}
	byPerson, _ = GetRecordsByPerson(p.ID)
	if len(byPerson) != 0 {
		t.Errorf("expected record deleted, got %d", len(byPerson))
	}
}

func TestDailyLogCRUD(t *testing.T) {
	u, _ := CreateUser("log-owner", "h")
	p, _ := CreatePerson(u.ID, "Lily", 28, 5)
	flow := 3
	w := 55.5
	temp := 36.6
	if err := UpsertDailyLog(p.ID, "2024-04-01", &flow, "cramps", "ok", &w, &temp); err != nil {
		t.Fatal(err)
	}
	// Upsert again should update, not duplicate.
	flow2 := 1
	if err := UpsertDailyLog(p.ID, "2024-04-01", &flow2, "none", "fine", nil, nil); err != nil {
		t.Fatal(err)
	}
	logs, err := GetDailyLogsByPerson(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 daily log after upsert, got %d", len(logs))
	}
	if logs[0].FlowLevel == nil || *logs[0].FlowLevel != 1 {
		t.Errorf("flow level not updated: %+v", logs[0].FlowLevel)
	}
	one, err := GetDailyLog(p.ID, "2024-04-01")
	if err != nil {
		t.Fatal(err)
	}
	if one.Symptoms != "none" {
		t.Errorf("symptoms = %s, want none", one.Symptoms)
	}
	all, err := GetAllDailyLogs()
	if err != nil || len(all) == 0 {
		t.Fatalf("GetAllDailyLogs = %v, %v", all, err)
	}
	if err := DeleteDailyLog(p.ID, "2024-04-01"); err != nil {
		t.Fatal(err)
	}
	logs, _ = GetDailyLogsByPerson(p.ID)
	if len(logs) != 0 {
		t.Errorf("expected daily log deleted, got %d", len(logs))
	}
}

func TestNotificationConfig(t *testing.T) {
	u, _ := CreateUser("notify-owner", "h")
	cfg := NotificationConfig{Enabled: true, ShoutrrrURL: "logger://", DaysBefore: 2}
	if err := SaveNotificationConfig(u.ID, cfg); err != nil {
		t.Fatal(err)
	}
	got := GetNotificationConfig(u.ID)
	if !got.Enabled || got.ShoutrrrURL != "logger://" || got.DaysBefore != 2 {
		t.Errorf("config mismatch: %+v", got)
	}
	today := time.Now().Format("2006-01-02")
	if err := UpdateNotificationLastNotified(u.ID, today); err != nil {
		t.Fatal(err)
	}
	got = GetNotificationConfig(u.ID)
	if got.LastNotified != today {
		t.Errorf("last notified = %s, want %s", got.LastNotified, today)
	}
}

func TestSessionCRUD(t *testing.T) {
	u, _ := CreateUser("session-owner", "h")
	expires := time.Now().Add(1 * time.Hour).Format("2006-01-02 15:04:05")
	if err := CreateSession("tok123", u.ID, "session-owner", expires); err != nil {
		t.Fatal(err)
	}
	s, err := GetSession("tok123")
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.UserID != u.ID {
		t.Errorf("session not found or mismatch: %+v", s)
	}
	if err := DeleteSession("tok123"); err != nil {
		t.Fatal(err)
	}
	s, _ = GetSession("tok123")
	if s != nil {
		t.Error("expected session deleted")
	}
}

func TestExpiredSession(t *testing.T) {
	u, _ := CreateUser("expired-owner", "h")
	// A clearly past timestamp; robust against non-monotonic test clocks.
	past := "2000-01-01T00:00:00Z"
	if err := CreateSession("exptok", u.ID, "expired-owner", past); err != nil {
		t.Fatal(err)
	}
	s, err := GetSession("exptok")
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Error("expected expired session to be nil")
	}
}
