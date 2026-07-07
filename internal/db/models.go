package db

type Person struct {
	ID           int64  `json:"id"`
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
	Person  Person   `json:"person"`
	Records []Record `json:"records"`
}
