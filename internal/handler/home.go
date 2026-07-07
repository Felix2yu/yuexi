package handler

import (
	"net/http"
	"strconv"
	"time"
	"yuexi/internal/db"
)

func Home(w http.ResponseWriter, r *http.Request) {
	year, month := getYearMonth(r)
	userID := GetUserID(r)

	persons, _ := db.GetPersonsByUser(userID)
	if len(persons) == 0 {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	data := injectUser(r, map[string]interface{}{
		"Persons": persons,
		"Year":    year,
		"Month":   month,
		"Today":   time.Now().Format("2006-01-02"),
	})

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
