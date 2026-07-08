package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"yuexi/internal/db"
	"yuexi/internal/service"
)

func StatsPage(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	persons, _ := db.GetPersonsByUser(userID)
	if len(persons) == 0 {
		http.Redirect(w, r, "/person", http.StatusSeeOther)
		return
	}

	data := injectUser(r, map[string]interface{}{
		"Persons": persons,
	})

	tmpl, err := parseTemplates("layout.html", "stats.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}

func StatsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := GetUserID(r)

	persons, _ := db.GetPersonsByUser(userID)
	allRecords, _ := db.GetRecordsByUser(userID)

	result := make(map[int64]db.CycleStats)

	for _, p := range persons {
		var recs []db.Record
		for _, rec := range allRecords {
			if rec.PersonID == p.ID {
				recs = append(recs, rec)
			}
		}
		if len(recs) < 2 {
			result[p.ID] = db.CycleStats{
				MinCycleLength:  p.CycleLength,
				MaxCycleLength:  p.CycleLength,
				AvgCycleLength:  float64(p.CycleLength),
				AvgPeriodLength: float64(p.PeriodLength),
				Regularity:      "数据不足",
			}
			continue
		}

		// Sort records by start date
		sorted := service.SortRecordsByDate(recs)

		var cycleLengths []float64
		var periodLengths []float64
		var cyclePoints []db.CycleDataPoint
		var periodPoints []db.CycleDataPoint

		for i := 1; i < len(sorted); i++ {
			prev, _ := time.Parse("2006-01-02", sorted[i-1].StartDate)
			curr, _ := time.Parse("2006-01-02", sorted[i].StartDate)
			diff := curr.Sub(prev).Hours() / 24
			if diff > 15 && diff < 60 { // reasonable cycle range
				cycleLengths = append(cycleLengths, diff)
				cyclePoints = append(cyclePoints, db.CycleDataPoint{
					Label: sorted[i].StartDate,
					Value: diff,
				})
			}
		}

		for _, rec := range sorted {
			if rec.EndDate != nil && *rec.EndDate != "" {
				start, _ := time.Parse("2006-01-02", rec.StartDate)
				end, _ := time.Parse("2006-01-02", *rec.EndDate)
				days := end.Sub(start).Hours()/24 + 1
				if days > 0 && days < 15 {
					periodLengths = append(periodLengths, days)
					periodPoints = append(periodPoints, db.CycleDataPoint{
						Label: rec.StartDate,
						Value: days,
					})
				}
			}
		}

		stats := db.CycleStats{
			CycleCount:  len(cycleLengths),
			CycleLengths: cyclePoints,
			PeriodLengths: periodPoints,
		}

		if len(cycleLengths) > 0 {
			stats.AvgCycleLength = avg(cycleLengths)
			stats.MinCycleLength = int(minVal(cycleLengths))
			stats.MaxCycleLength = int(maxVal(cycleLengths))
			std := stdDev(cycleLengths)
			if std < 2 {
				stats.Regularity = "非常规律"
			} else if std < 4 {
				stats.Regularity = "较为规律"
			} else if std < 7 {
				stats.Regularity = "略有波动"
			} else {
				stats.Regularity = "波动较大"
			}
		}

		if len(periodLengths) > 0 {
			stats.AvgPeriodLength = avg(periodLengths)
		}

		// Calculate symptom statistics
		logs, _ := db.GetDailyLogsByPerson(p.ID)
		symptomCounts := make(map[string]int)
		for _, log := range logs {
			if log.Symptoms != "" {
				symptoms := strings.Split(log.Symptoms, ",")
				for _, s := range symptoms {
					s = strings.TrimSpace(s)
					if s != "" {
						symptomCounts[s]++
					}
				}
			}
		}
		stats.SymptomCounts = symptomCounts

		result[p.ID] = stats
	}

	json.NewEncoder(w).Encode(result)
}

func DailyLogAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	personID, _ := strconv.ParseInt(r.URL.Query().Get("person_id"), 10, 64)

	switch r.Method {
	case http.MethodGet:
		if personID == 0 {
			logs, _ := db.GetAllDailyLogs()
			json.NewEncoder(w).Encode(logs)
			return
		}
		logs, _ := db.GetDailyLogsByPerson(personID)
		json.NewEncoder(w).Encode(logs)

	case http.MethodPost:
		var req struct {
			Date        string   `json:"date"`
			FlowLevel   *int     `json:"flow_level"`
			Symptoms    string   `json:"symptoms"`
			Note        string   `json:"note"`
			Weight      *float64 `json:"weight"`
			Temperature *float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, 400)
			return
		}
		if personID == 0 || req.Date == "" {
			http.Error(w, `{"error":"missing person_id or date"}`, 400)
			return
		}
		if err := db.UpsertDailyLog(personID, req.Date, req.FlowLevel, req.Symptoms, req.Note, req.Weight, req.Temperature); err != nil {
			http.Error(w, `{"error":"save failed"}`, 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case http.MethodDelete:
		date := r.URL.Query().Get("date")
		if personID == 0 || date == "" {
			http.Error(w, `{"error":"missing person_id or date"}`, 400)
			return
		}
		db.DeleteDailyLog(personID, date)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", 405)
	}
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return math.Round(sum/float64(len(vals))*10) / 10
}

func minVal(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals {
		if v < m {
			m = v
		}
	}
	return m
}

func maxVal(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func stdDev(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := avg(vals)
	sum := 0.0
	for _, v := range vals {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(vals)))
}
