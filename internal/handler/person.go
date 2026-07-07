package handler

import (
	"html/template"
	"net/http"
	"strconv"
	"yuexi/internal/db"
)

func PersonList(w http.ResponseWriter, r *http.Request) {
	persons, _ := db.GetAllPersons()

	data := map[string]interface{}{
		"Persons": persons,
	}

	tmpl, err := template.ParseFiles(
		"internal/template/layout.html",
		"internal/template/person.html",
	)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}

func PersonCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	cycleLength, _ := strconv.Atoi(r.FormValue("cycle_length"))
	periodLength, _ := strconv.Atoi(r.FormValue("period_length"))

	if name == "" {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	if cycleLength <= 0 {
		cycleLength = 28
	}
	if periodLength <= 0 {
		periodLength = 5
	}

	db.CreatePerson(name, cycleLength, periodLength)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func PersonEdit(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		cycleLength, _ := strconv.Atoi(r.FormValue("cycle_length"))
		periodLength, _ := strconv.Atoi(r.FormValue("period_length"))

		if name != "" {
			if cycleLength <= 0 {
				cycleLength = 28
			}
			if periodLength <= 0 {
				periodLength = 5
			}
			db.UpdatePerson(id, name, cycleLength, periodLength)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	person, err := db.GetPerson(id)
	if err != nil {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"Person": person,
	}

	tmpl, err := template.ParseFiles(
		"internal/template/layout.html",
		"internal/template/person_edit.html",
	)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}

func PersonDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.FormValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	db.DeletePerson(id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
