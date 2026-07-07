package handler

import (
	"net/http"
	"strconv"
	"time"
	"yuexi/internal/db"
	"yuexi/internal/service"
)

func Home(w http.ResponseWriter, r *http.Request) {
	year, month := getYearMonth(r)

	persons, _ := db.GetAllPersons()
	if len(persons) == 0 {
		// Redirect to person creation
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	currentPersonID := getCurrentPersonID(r, persons)
	person, _ := db.GetPerson(currentPersonID)

	records, _ := db.GetRecordsByPerson(currentPersonID)
	periods, ovulations := service.CalculateMonthData(*person, records, year, month)

	data := map[string]interface{}{
		"Person":        person,
		"Persons":       persons,
		"Year":          year,
		"Month":         month,
		"Periods":       periods,
		"Ovulations":    ovulations,
		"Records":       records,
		"CurrentPerson": currentPersonID,
		"Today":         time.Now().Format("2006-01-02"),
	}

	tmpl, err := parseTemplates("layout.html", "home.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}

func getYearMonth(r *http.Request) (int, int) {
	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if y := r.URL.Query().Get("year"); y != "" {
		if val, err := strconv.Atoi(y); err == nil {
			year = val
		}
	}
	if m := r.URL.Query().Get("month"); m != "" {
		if val, err := strconv.Atoi(m); err == nil && val >= 1 && val <= 12 {
			month = val
		}
	}
	return year, month
}

func getCurrentPersonID(r *http.Request, persons []db.Person) int64 {
	if pid := r.URL.Query().Get("person_id"); pid != "" {
		if val, err := strconv.ParseInt(pid, 10, 64); err == nil {
			return val
		}
	}
	if len(persons) > 0 {
		return persons[0].ID
	}
	return 0
}
