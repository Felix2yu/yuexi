package service

import (
	"math"
	"time"
	"yuexi/internal/db"
)

func CalculateMonthData(person db.Person, records []db.Record, year, month int) ([]db.DateRange, []db.DateRange) {
	var periods, ovulations []db.DateRange

	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endOfMonth := startOfMonth.AddDate(0, 1, -1)

	// Sort records by date
	sorted := SortRecordsByDate(records)

	// First, collect all actual periods in this month
	var actualPeriodsInMonth []db.DateRange
	for _, rec := range sorted {
		recStart, err := time.Parse("2006-01-02", rec.StartDate)
		if err != nil {
			continue
		}

		// Use actual end_date if available, otherwise use period_length
		var periodEnd time.Time
		if rec.EndDate != nil && *rec.EndDate != "" {
			periodEnd, _ = time.Parse("2006-01-02", *rec.EndDate)
		}
		if periodEnd.IsZero() {
			periodEnd = recStart.AddDate(0, 0, person.PeriodLength-1)
		}

		if rangesOverlap(recStart, periodEnd, startOfMonth, endOfMonth) {
			actualPeriodsInMonth = append(actualPeriodsInMonth, db.DateRange{
				Start:    recStart.Format("2006-01-02"),
				End:      periodEnd.Format("2006-01-02"),
				Type:     "period",
				PersonID: person.ID,
			})
		}
	}

	// Add all actual periods
	periods = append(periods, actualPeriodsInMonth...)

	// Only predict from the most recent record to avoid duplicate predictions
	if len(sorted) > 0 {
		latestRec := sorted[len(sorted)-1]
		latestStart, err := time.Parse("2006-01-02", latestRec.StartDate)
		if err == nil {
			// Generate predictions for up to 6 cycles ahead from the latest record
			for i := 1; i <= 6; i++ {
				nextPeriodStart := latestStart.AddDate(0, 0, person.CycleLength*i)
				nextPeriodEnd := nextPeriodStart.AddDate(0, 0, person.PeriodLength-1)

				// Check if there's already an actual period that overlaps with this predicted period
				hasActualOverlap := false
				for _, actual := range actualPeriodsInMonth {
					actualStart, _ := time.Parse("2006-01-02", actual.Start)
					actualEnd, _ := time.Parse("2006-01-02", actual.End)
					// If the predicted period overlaps with an actual period, skip it
					if rangesOverlap(nextPeriodStart, nextPeriodEnd, actualStart, actualEnd) {
						hasActualOverlap = true
						break
					}
					// Also skip if the actual period is within a reasonable range (e.g., 7 days) of the predicted period
					diff := nextPeriodStart.Sub(actualStart).Hours() / 24
					if diff >= -7 && diff <= 7 {
						hasActualOverlap = true
						break
					}
				}

				if hasActualOverlap {
					continue
				}

				if rangesOverlap(nextPeriodStart, nextPeriodEnd, startOfMonth, endOfMonth) {
					periods = append(periods, db.DateRange{
						Start:    nextPeriodStart.Format("2006-01-02"),
						End:      nextPeriodEnd.Format("2006-01-02"),
						Type:     "predicted_period",
						PersonID: person.ID,
					})
				}

				ovulationDay := nextPeriodStart.AddDate(0, 0, -14)
				ovulationStart := ovulationDay.AddDate(0, 0, -5)
				ovulationEnd := ovulationDay.AddDate(0, 0, 1)

				if rangesOverlap(ovulationStart, ovulationEnd, startOfMonth, endOfMonth) {
					ovulations = append(ovulations, db.DateRange{
						Start:    ovulationStart.Format("2006-01-02"),
						End:      ovulationEnd.Format("2006-01-02"),
						Type:     "ovulation_window",
						PersonID: person.ID,
					})
				}
				if ovulationDay.After(startOfMonth.AddDate(0, 0, -1)) && ovulationDay.Before(endOfMonth.AddDate(0, 0, 1)) {
					ovulations = append(ovulations, db.DateRange{
						Start:    ovulationDay.Format("2006-01-02"),
						End:      ovulationDay.Format("2006-01-02"),
						Type:     "ovulation_day",
						PersonID: person.ID,
					})
				}
			}
		}
	}

	return periods, ovulations
}

func rangesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !aEnd.Before(bStart) && !aStart.After(bEnd)
}

func GetNextPeriodDate(person db.Person, records []db.Record) *time.Time {
	if len(records) == 0 {
		return nil
	}

	var latestStart time.Time
	for _, rec := range records {
		t, err := time.Parse("2006-01-02", rec.StartDate)
		if err != nil {
			continue
		}
		if latestStart.IsZero() || t.After(latestStart) {
			latestStart = t
		}
	}

	if latestStart.IsZero() {
		return nil
	}

	next := latestStart.AddDate(0, 0, person.CycleLength)
	return &next
}

func GetOvulationDate(person db.Person, records []db.Record) *time.Time {
	nextPeriod := GetNextPeriodDate(person, records)
	if nextPeriod == nil {
		return nil
	}

	ovulation := nextPeriod.AddDate(0, 0, -14)
	return &ovulation
}

type CycleAnomaly struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

func DetectCycleAnomaly(person db.Person, records []db.Record) []CycleAnomaly {
	var anomalies []CycleAnomaly

	if len(records) < 2 {
		return anomalies
	}

	sorted := SortRecordsByDate(records)

	// Check for irregular cycles
	var cycleLengths []float64
	for i := 1; i < len(sorted); i++ {
		prev, err1 := time.Parse("2006-01-02", sorted[i-1].StartDate)
		curr, err2 := time.Parse("2006-01-02", sorted[i].StartDate)
		if err1 != nil || err2 != nil {
			continue
		}
		diff := curr.Sub(prev).Hours() / 24
		if diff > 15 && diff < 60 {
			cycleLengths = append(cycleLengths, diff)
		}
	}

	if len(cycleLengths) >= 3 {
		std := stdDev(cycleLengths)

		// Check for very irregular cycles
		if std > 7 {
			anomalies = append(anomalies, CycleAnomaly{
				Type:        "irregular_cycle",
				Description: "周期波动较大，建议咨询医生",
				Severity:    "warning",
			})
		}

		// Check for very short cycles
		for _, length := range cycleLengths {
			if length < 21 {
				anomalies = append(anomalies, CycleAnomaly{
					Type:        "short_cycle",
					Description: "检测到短周期（少于21天）",
					Severity:    "warning",
				})
				break
			}
		}

		// Check for very long cycles
		for _, length := range cycleLengths {
			if length > 35 {
				anomalies = append(anomalies, CycleAnomaly{
					Type:        "long_cycle",
					Description: "检测到长周期（超过35天）",
					Severity:    "info",
				})
				break
			}
		}

		// Check for sudden changes
		if len(cycleLengths) >= 2 {
			lastTwo := cycleLengths[len(cycleLengths)-2:]
			change := lastTwo[1] - lastTwo[0]
			if change > 7 || change < -7 {
				anomalies = append(anomalies, CycleAnomaly{
					Type:        "sudden_change",
					Description: "近期周期变化较大",
					Severity:    "info",
				})
			}
		}
	}

	return anomalies
}

func SortRecordsByDate(records []db.Record) []db.Record {
	sorted := make([]db.Record, len(records))
	copy(sorted, records)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].StartDate < sorted[i].StartDate {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
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
