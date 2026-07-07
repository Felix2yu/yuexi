package handler

import (
	"net/http"
	"strconv"
	"yuexi/internal/service"
)

func ExportPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func ExportDownload(w http.ResponseWriter, r *http.Request) {
	personIDStr := r.URL.Query().Get("person_id")

	var data []byte
	var err error
	var filename string

	if personIDStr != "" {
		personID, _ := strconv.ParseInt(personIDStr, 10, 64)
		data, err = service.ExportPerson(personID)
		filename = "yuexi_person_" + personIDStr + ".json"
	} else {
		data, err = service.ExportAll()
		filename = "yuexi_all.json"
	}

	if err != nil {
		http.Error(w, "Export failed: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Write(data)
}

func ImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/?import_error=1", http.StatusSeeOther)
		return
	}
	defer file.Close()

	count, err := service.ImportData(file)
	if err != nil {
		http.Redirect(w, r, "/?import_error=1", http.StatusSeeOther)
		return
	}

	_ = count
	http.Redirect(w, r, "/?import_success=1", http.StatusSeeOther)
}
