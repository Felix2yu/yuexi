package service

import (
	"time"
	"yuexi/internal/db"
)

func CalculateMonthData(person db.Person, records []db.Record, year, month int) ([]db.DateRange, []db.DateRange) {
	var periods, ovulations []db.DateRange

	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endOfMonth := startOfMonth.AddDate(0, 1, -1)

	for _, rec := range records {
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
			periods = append(periods, db.DateRange{
				Start:    recStart.Format("2006-01-02"),
				End:      periodEnd.Format("2006-01-02"),
				Type:     "period",
				PersonID: person.ID,
			})
		}

		// Predicted next periods and ovulation for up to 6 cycles ahead
		for i := 1; i <= 6; i++ {
			nextPeriodStart := recStart.AddDate(0, 0, person.CycleLength*i)
			nextPeriodEnd := nextPeriodStart.AddDate(0, 0, person.PeriodLength-1)

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
