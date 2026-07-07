package db

import "time"

type Person struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	CycleLength int       `json:"cycle_length"`
	PeriodLength int      `json:"period_length"`
	CreatedAt   time.Time `json:"created_at"`
}

type Record struct {
	ID         int64      `json:"id"`
	PersonID   int64      `json:"person_id"`
	StartDate  string     `json:"start_date"`
	EndDate    *string    `json:"end_date"`
	Note       string     `json:"note"`
	CreatedAt  time.Time  `json:"created_at"`
}

type CycleInfo struct {
	Person          Person   `json:"person"`
	Records         []Record `json:"records"`
	MonthPeriods    []DateRange `json:"month_periods"`
	MonthOvulations []DateRange `json:"month_ovulations"`
}

type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Type  string `json:"type"` // "period", "predicted_period", "ovulation_window", "ovulation_day"
}

type ExportData struct {
	Person  Person   `json:"person"`
	Records []Record `json:"records"`
}
