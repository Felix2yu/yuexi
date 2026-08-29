package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"yuexi/internal/db"
)

// TestMain initializes a throwaway SQLite database shared by the service tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yuexi-service-test")
	if err != nil {
		panic(err)
	}
	db.Init(filepath.Join(dir, "test.db"))
	code := m.Run()
	db.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func strPtr(s string) *string { return &s }

func TestSortRecordsByDate(t *testing.T) {
	recs := []db.Record{
		{StartDate: "2024-03-01"},
		{StartDate: "2024-01-15"},
		{StartDate: "2024-02-10"},
	}
	sorted := SortRecordsByDate(recs)
	want := []string{"2024-01-15", "2024-02-10", "2024-03-01"}
	for i, r := range sorted {
		if r.StartDate != want[i] {
			t.Errorf("index %d: got %s want %s", i, r.StartDate, want[i])
		}
	}
	if recs[0].StartDate != "2024-03-01" {
		t.Error("original slice was mutated")
	}
}

func TestRangesOverlap(t *testing.T) {
	const layout = "2006-01-02"
	cases := []struct {
		name                         string
		aStart, aEnd, bStart, bEnd   string
		want                         bool
	}{
		{"overlap", "2024-01-05", "2024-01-10", "2024-01-08", "2024-01-12", true},
		{"disjoint", "2024-01-05", "2024-01-10", "2024-01-15", "2024-01-20", false},
		{"touching", "2024-01-05", "2024-01-10", "2024-01-10", "2024-01-12", true},
		{"contained", "2024-01-06", "2024-01-07", "2024-01-05", "2024-01-10", true},
	}
	for _, c := range cases {
		aS, _ := time.Parse(layout, c.aStart)
		aE, _ := time.Parse(layout, c.aEnd)
		bS, _ := time.Parse(layout, c.bStart)
		bE, _ := time.Parse(layout, c.bEnd)
		if got := rangesOverlap(aS, aE, bS, bE); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestGetNextPeriodDate(t *testing.T) {
	person := db.Person{CycleLength: 28}
	if got := GetNextPeriodDate(person, nil); got != nil {
		t.Error("expected nil for empty records")
	}
	got := GetNextPeriodDate(person, []db.Record{{StartDate: "2024-01-01"}})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Format("2006-01-02") != "2024-01-29" {
		t.Errorf("got %s want 2024-01-29", got.Format("2006-01-02"))
	}
}

func TestGetOvulationDate(t *testing.T) {
	person := db.Person{CycleLength: 28}
	if got := GetOvulationDate(person, nil); got != nil {
		t.Error("expected nil for empty records")
	}
	got := GetOvulationDate(person, []db.Record{{StartDate: "2024-01-01"}})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Format("2006-01-02") != "2024-01-15" {
		t.Errorf("got %s want 2024-01-15", got.Format("2006-01-02"))
	}
}

func TestCalculateMonthDataActualAndPredicted(t *testing.T) {
	person := db.Person{CycleLength: 28, PeriodLength: 5}
	recs := []db.Record{{StartDate: "2024-01-01"}}

	// January 2024: one actual period (Jan 1-5) and a predicted period (Jan 29 - Feb 2).
	periods, ovulations := CalculateMonthData(person, recs, 2024, 1)
	if len(periods) != 2 {
		t.Fatalf("want 2 periods, got %d (%+v)", len(periods), periods)
	}
	if periods[0].Type != "period" {
		t.Errorf("periods[0].Type = %s, want period", periods[0].Type)
	}
	if periods[1].Type != "predicted_period" {
		t.Errorf("periods[1].Type = %s, want predicted_period", periods[1].Type)
	}
	if len(ovulations) != 2 {
		t.Fatalf("want 2 ovulation ranges, got %d (%+v)", len(ovulations), ovulations)
	}
	if ovulations[0].Type != "ovulation_window" || ovulations[1].Type != "ovulation_day" {
		t.Errorf("unexpected ovulation types: %+v", ovulations)
	}
}

func TestCalculateMonthDataNoRecords(t *testing.T) {
	person := db.Person{CycleLength: 28, PeriodLength: 5}
	periods, ovulations := CalculateMonthData(person, nil, 2024, 1)
	if len(periods) != 0 || len(ovulations) != 0 {
		t.Errorf("expected empty results, got periods=%v ovulations=%v", periods, ovulations)
	}
}

func TestCalculateMonthDataPredictedOnly(t *testing.T) {
	// Record on 2024-02-20 ends 2024-02-24 (fully inside Feb). Its first predicted
	// cycle lands on 2024-03-19, so March yields exactly one predicted period.
	person := db.Person{CycleLength: 28, PeriodLength: 5}
	recs := []db.Record{{StartDate: "2024-02-20"}}
	periods, ovulations := CalculateMonthData(person, recs, 2024, 3)
	var actual, predicted int
	for _, p := range periods {
		switch p.Type {
		case "period":
			actual++
		case "predicted_period":
			predicted++
		}
	}
	if actual != 0 {
		t.Errorf("did not expect an actual period in March, got %d", actual)
	}
	if predicted != 1 {
		t.Fatalf("want 1 predicted_period, got %d (%+v)", predicted, periods)
	}
	if periods[0].Start != "2024-03-19" {
		t.Errorf("predicted start = %s, want 2024-03-19", periods[0].Start)
	}
	hasWindow, hasDay := false, false
	for _, o := range ovulations {
		if o.Type == "ovulation_window" {
			hasWindow = true
		}
		if o.Type == "ovulation_day" {
			hasDay = true
		}
	}
	if !hasWindow || !hasDay {
		t.Errorf("expected at least one ovulation window and one ovulation day, got %+v", ovulations)
	}
}

func TestDetectCycleAnomalyTooFewRecords(t *testing.T) {
	person := db.Person{CycleLength: 28}
	if got := DetectCycleAnomaly(person, []db.Record{{StartDate: "2024-01-01"}}); len(got) != 0 {
		t.Errorf("expected no anomalies with <2 records, got %+v", got)
	}
}

func TestDetectCycleAnomalyRegular(t *testing.T) {
	person := db.Person{CycleLength: 28}
	recs := []db.Record{
		{StartDate: "2024-01-01"},
		{StartDate: "2024-01-29"},
		{StartDate: "2024-02-26"},
	}
	if got := DetectCycleAnomaly(person, recs); len(got) != 0 {
		t.Errorf("expected no anomalies for regular cycles, got %+v", got)
	}
}

func TestDetectCycleAnomalyIrregular(t *testing.T) {
	person := db.Person{CycleLength: 28}
	recs := []db.Record{
		{StartDate: "2024-01-01"},
		{StartDate: "2024-02-08"}, // +38
		{StartDate: "2024-03-05"}, // +26
		{StartDate: "2024-04-20"}, // +46
	}
	got := DetectCycleAnomaly(person, recs)
	if len(got) == 0 {
		t.Fatal("expected anomalies for irregular cycles")
	}
	has := func(typ string) bool {
		for _, a := range got {
			if a.Type == typ {
				return true
			}
		}
		return false
	}
	if !has("irregular_cycle") {
		t.Errorf("missing irregular_cycle anomaly: %+v", got)
	}
	if !has("long_cycle") {
		t.Errorf("missing long_cycle anomaly: %+v", got)
	}
	if !has("sudden_change") {
		t.Errorf("missing sudden_change anomaly: %+v", got)
	}
}

func TestDetectCycleAnomalyShort(t *testing.T) {
	// Need at least 3 cycle lengths (>=4 records) for the anomaly block to run.
	person := db.Person{CycleLength: 28}
	recs := []db.Record{
		{StartDate: "2024-01-01"},
		{StartDate: "2024-01-19"}, // +18
		{StartDate: "2024-02-06"}, // +18
		{StartDate: "2024-02-24"}, // +18
	}
	got := DetectCycleAnomaly(person, recs)
	found := false
	for _, a := range got {
		if a.Type == "short_cycle" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected short_cycle anomaly, got %+v", got)
	}
}

func TestAvg(t *testing.T) {
	if avg(nil) != 0 {
		t.Error("avg(nil) should be 0")
	}
	if got := avg([]float64{2, 4, 6}); got != 4 {
		t.Errorf("avg([2,4,6]) = %v, want 4", got)
	}
}

func TestStdDev(t *testing.T) {
	if stdDev(nil) != 0 {
		t.Error("stdDev(nil) should be 0")
	}
	got := stdDev([]float64{2, 4, 6})
	if got < 1.63 || got > 1.64 {
		t.Errorf("stdDev([2,4,6]) = %v, want ~1.633", got)
	}
}

func TestExportPerson(t *testing.T) {
	user, err := db.CreateUser("export-user", "hash")
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.CreatePerson(user.ID, "Alice", 28, 5)
	if err != nil {
		t.Fatal(err)
	}
	end := "2024-01-05"
	if _, err := db.CreateRecord(p.ID, "2024-01-01", &end, "cramps"); err != nil {
		t.Fatal(err)
	}

	data, err := ExportPerson(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []db.ExportData
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 export record, got %d", len(out))
	}
	if out[0].Person.Name != "Alice" {
		t.Errorf("person name = %s, want Alice", out[0].Person.Name)
	}
	if len(out[0].Records) != 1 {
		t.Errorf("want 1 record, got %d", len(out[0].Records))
	}
}

func TestExportAllByUser(t *testing.T) {
	user, err := db.CreateUser("export-all-user", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePerson(user.ID, "P1", 28, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePerson(user.ID, "P2", 30, 6); err != nil {
		t.Fatal(err)
	}

	data, err := ExportAllByUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []db.ExportData
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 export records, got %d", len(out))
	}
}

func TestImportData(t *testing.T) {
	src, err := db.CreateUser("import-src", "hash")
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.CreatePerson(src.ID, "Bob", 30, 6)
	if err != nil {
		t.Fatal(err)
	}
	end := "2024-02-05"
	if _, err := db.CreateRecord(p.ID, "2024-02-01", &end, "note"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertDailyLog(p.ID, "2024-02-01", nil, "bloating", "", nil, nil); err != nil {
		t.Fatal(err)
	}

	data, err := ExportPerson(p.ID)
	if err != nil {
		t.Fatal(err)
	}

	dst, err := db.CreateUser("import-dst", "hash")
	if err != nil {
		t.Fatal(err)
	}
	count, err := ImportData(bytes.NewReader(data), dst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("want 1 imported record, got %d", count)
	}

	persons, err := db.GetPersonsByUser(dst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persons) != 1 {
		t.Fatalf("want 1 imported person, got %d", len(persons))
	}
	if persons[0].Name != "Bob" {
		t.Errorf("imported person name = %s, want Bob", persons[0].Name)
	}
	recs, err := db.GetRecordsByPerson(persons[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Errorf("want 1 record for imported person, got %d", len(recs))
	}
	logs, err := db.GetDailyLogsByPerson(persons[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Errorf("want 1 daily log for imported person, got %d", len(logs))
	}
}

func TestImportDataInvalidJSON(t *testing.T) {
	dst, err := db.CreateUser("import-bad", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportData(bytes.NewReader([]byte("not json")), dst.ID); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
