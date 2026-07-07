package handler

import (
	"encoding/json"
	"net/http"
	"time"
	"yuexi/internal/db"
	"yuexi/internal/service"
)

func NotificationConfigAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		cfg := db.GetNotificationConfig()
		json.NewEncoder(w).Encode(cfg)
		return
	}

	if r.Method == http.MethodPost {
		var cfg db.NotificationConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, `{"error":"invalid request"}`, 400)
			return
		}
		if err := db.SaveNotificationConfig(cfg); err != nil {
			http.Error(w, `{"error":"save failed"}`, 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	http.Error(w, "Method not allowed", 405)
}

func NotificationTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		http.Error(w, `{"error":"url is required"}`, 400)
		return
	}

	if err := service.TestNotification(url); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func NotificationStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cfg := db.GetNotificationConfig()
	persons, _ := db.GetAllPersons()

	type personNext struct {
		Name       string `json:"name"`
		NextPeriod string `json:"next_period"`
		DaysLeft   int    `json:"days_left"`
	}

	var upcoming []personNext
	today := time.Now()

	for _, p := range persons {
		records, _ := db.GetRecordsByPerson(p.ID)
		if len(records) == 0 {
			continue
		}
		next := service.GetNextPeriodDate(p, records)
		if next == nil {
			continue
		}
		daysLeft := int(next.Sub(today).Hours() / 24)
		if daysLeft >= 0 && daysLeft <= 30 {
			upcoming = append(upcoming, personNext{
				Name:       p.Name,
				NextPeriod: next.Format("2006-01-02"),
				DaysLeft:   daysLeft,
			})
		}
	}

	result := map[string]interface{}{
		"config":   cfg,
		"upcoming": upcoming,
	}
	json.NewEncoder(w).Encode(result)
}
