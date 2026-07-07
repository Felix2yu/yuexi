package db

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    string `json:"created_at"`
}

type Person struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"user_id"`
	Name         string `json:"name"`
	CycleLength  int    `json:"cycle_length"`
	PeriodLength int    `json:"period_length"`
	CreatedAt    string `json:"created_at"`
}

type Record struct {
	ID        int64   `json:"id"`
	PersonID  int64   `json:"person_id"`
	StartDate string  `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Note      string  `json:"note"`
	CreatedAt string  `json:"created_at"`
}

type DateRange struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Type     string `json:"type"`
	PersonID int64  `json:"person_id"`
}

type ExportData struct {
	Person   Person     `json:"person"`
	Records  []Record   `json:"records"`
	DailyLogs []DailyLog `json:"daily_logs,omitempty"`
}

type DailyLog struct {
	ID        int64   `json:"id"`
	PersonID  int64   `json:"person_id"`
	Date      string  `json:"date"`
	FlowLevel *int    `json:"flow_level"`
	Symptoms  string  `json:"symptoms"`
	Note      string  `json:"note"`
	CreatedAt string  `json:"created_at"`
}

type CycleStats struct {
	AvgCycleLength  float64        `json:"avg_cycle_length"`
	MinCycleLength  int            `json:"min_cycle_length"`
	MaxCycleLength  int            `json:"max_cycle_length"`
	AvgPeriodLength float64        `json:"avg_period_length"`
	CycleCount      int            `json:"cycle_count"`
	CycleLengths    []CycleDataPoint `json:"cycle_lengths"`
	PeriodLengths   []CycleDataPoint `json:"period_lengths"`
	Regularity      string         `json:"regularity"`
}

type CycleDataPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}
