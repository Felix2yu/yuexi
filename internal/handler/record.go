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

	personID, err := strconv.ParseInt(r.URL.Query().Get("person_id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error": "invalid person_id"}`, 400)
		return
	}

	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))
	if year == 0 || month == 0 {
		now := time.Now()
		year = now.Year()
		month = int(now.Month())
	}

	person, err := db.GetPerson(personID)
	if err != nil {
		http.Error(w, `{"error": "person not found"}`, 404)
		return
	}

	records, _ := db.GetRecordsByPerson(personID)
	periods, ovulations := service.CalculateMonthData(*person, records, year, month)

	result := map[string]interface{}{
		"periods":    periods,
		"ovulations": ovulations,
		"records":    records,
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

	// Redirect back to referer or home
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/?person_id=" + strconv.FormatInt(personID, 10)
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
	personID, _ := strconv.ParseInt(r.FormValue("person_id"), 10, 64)

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
		referer = "/?person_id=" + strconv.FormatInt(personID, 10)
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func RecordDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	personID := r.FormValue("person_id")

	if id != 0 {
		db.DeleteRecord(id)
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/?person_id=" + personID
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
