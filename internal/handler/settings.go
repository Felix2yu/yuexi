package handler

import (
	"net/http"
	"yuexi/internal/db"
)

func Settings(w http.ResponseWriter, r *http.Request) {
	persons, _ := db.GetAllPersons()

	data := map[string]interface{}{
		"Persons": persons,
	}

	tmpl, err := parseTemplates("layout.html", "settings.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}
