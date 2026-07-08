package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"yuexi/internal/db"
	"yuexi/internal/service"
)

func NotificationConfigAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := GetUserID(r)

	if r.Method == http.MethodGet {
		cfg := db.GetNotificationConfig(userID)
		json.NewEncoder(w).Encode(cfg)
		return
	}

	if r.Method == http.MethodPost {
		var cfg db.NotificationConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, `{"error":"invalid request"}`, 400)
			return
		}
		if err := db.SaveNotificationConfig(userID, cfg); err != nil {
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
	userID := GetUserID(r)

	cfg := db.GetNotificationConfig(userID)
	persons, _ := db.GetPersonsByUser(userID)

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

func CycleAnomalyAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := GetUserID(r)

	personIDStr := r.URL.Query().Get("person_id")
	if personIDStr == "" {
		persons, _ := db.GetPersonsByUser(userID)
		if len(persons) > 0 {
			personIDStr = fmt.Sprintf("%d", persons[0].ID)
		}
	}

	if personIDStr == "" {
		json.NewEncoder(w).Encode([]service.CycleAnomaly{})
		return
	}

	personID, _ := strconv.ParseInt(personIDStr, 10, 64)
	person, err := db.GetPerson(personID)
	if err != nil || person.UserID != userID {
		json.NewEncoder(w).Encode([]service.CycleAnomaly{})
		return
	}

	records, _ := db.GetRecordsByPerson(personID)
	anomalies := service.DetectCycleAnomaly(*person, records)

	json.NewEncoder(w).Encode(anomalies)
}
