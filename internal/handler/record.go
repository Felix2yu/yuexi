package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"yuexi/internal/db"
	"yuexi/internal/service"
)

func RecordAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := GetUserID(r)

	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))
	if year == 0 || month == 0 {
		now := time.Now()
		year = now.Year()
		month = int(now.Month())
	}

	persons, _ := db.GetPersonsByUser(userID)
	allRecords, _ := db.GetAllRecords()

	var allPeriods, allOvulations []db.DateRange

	// Build a map of person_id to period_length for calculating effective end dates
	personMap := make(map[int64]db.Person)
	for _, p := range persons {
		if p.UserID == userID {
			personMap[p.ID] = p
		}
	}

	for _, p := range persons {
		if p.UserID != userID {
			continue
		}
		var recs []db.Record
		for _, r := range allRecords {
			if r.PersonID == p.ID {
				recs = append(recs, r)
			}
		}
		periods, ovulations := service.CalculateMonthData(p, recs, year, month)
		allPeriods = append(allPeriods, periods...)
		allOvulations = append(allOvulations, ovulations...)
	}

	// Add effective_end_date to each record (actual end_date or calculated from period_length)
	type RecordWithEndDate struct {
		ID               int64   `json:"id"`
		PersonID         int64   `json:"person_id"`
		StartDate        string  `json:"start_date"`
		EndDate          *string `json:"end_date"`
		EffectiveEndDate string  `json:"effective_end_date"`
		Note             string  `json:"note"`
		CreatedAt        string  `json:"created_at"`
	}

	var recordsWithEnd []RecordWithEndDate
	for _, rec := range allRecords {
		effectiveEnd := rec.StartDate
		if rec.EndDate != nil && *rec.EndDate != "" {
			effectiveEnd = *rec.EndDate
		} else if p, ok := personMap[rec.PersonID]; ok {
			start, err := time.Parse("2006-01-02", rec.StartDate)
			if err == nil {
				end := start.AddDate(0, 0, p.PeriodLength-1)
				effectiveEnd = end.Format("2006-01-02")
			}
		}
		recordsWithEnd = append(recordsWithEnd, RecordWithEndDate{
			ID:               rec.ID,
			PersonID:         rec.PersonID,
			StartDate:        rec.StartDate,
			EndDate:          rec.EndDate,
			EffectiveEndDate: effectiveEnd,
			Note:             rec.Note,
			CreatedAt:        rec.CreatedAt,
		})
	}

	result := map[string]interface{}{
		"periods":    allPeriods,
		"ovulations": allOvulations,
		"records":    recordsWithEnd,
		"persons":    persons,
	}

	json.NewEncoder(w).Encode(result)
}

func RecordCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	personID, _ := strconv.ParseInt(r.FormValue("person_id"), 10, 64)
	startDate := r.FormValue("start_date")
	note := r.FormValue("note")

	var endDate *string
	if ed := r.FormValue("end_date"); ed != "" {
		endDate = &ed
	}

	if personID == 0 || startDate == "" {
		http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
		return
	}

	db.CreateRecord(personID, startDate, endDate, note)

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func RecordEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	startDate := r.FormValue("start_date")
	note := r.FormValue("note")

	var endDate *string
	if ed := r.FormValue("end_date"); ed != "" {
		endDate = &ed
	}

	if id == 0 || startDate == "" {
		http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
		return
	}

	db.UpdateRecord(id, startDate, endDate, note)

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func RecordDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)

	if id != 0 {
		db.DeleteRecord(id)
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
