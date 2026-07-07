package handler

import (
	"net/http"
	"yuexi/internal/db"
)

func Settings(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	persons, _ := db.GetPersonsByUser(userID)

	data := injectUser(r, map[string]interface{}{
		"Persons": persons,
	})

	tmpl, err := parseTemplates("layout.html", "settings.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}
