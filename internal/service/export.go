package service

import (
	"encoding/json"
	"fmt"
	"io"
	"yuexi/internal/db"
)

func ExportPerson(personID int64) ([]byte, error) {
	person, err := db.GetPerson(personID)
	if err != nil {
		return nil, fmt.Errorf("failed to get person: %w", err)
	}

	records, err := db.GetRecordsByPerson(personID)
	if err != nil {
		return nil, fmt.Errorf("failed to get records: %w", err)
	}

	logs, _ := db.GetDailyLogsByPerson(personID)

	data := db.ExportData{
		Person:    *person,
		Records:   records,
		DailyLogs: logs,
	}

	return json.MarshalIndent([]db.ExportData{data}, "", "  ")
}

func ExportAllByUser(userID int64) ([]byte, error) {
	persons, err := db.GetPersonsByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get persons: %w", err)
	}

	var allData []db.ExportData
	for _, p := range persons {
		records, err := db.GetRecordsByPerson(p.ID)
		if err != nil {
			continue
		}
		logs, _ := db.GetDailyLogsByPerson(p.ID)
		allData = append(allData, db.ExportData{
			Person:    p,
			Records:   records,
			DailyLogs: logs,
		})
	}

	return json.MarshalIndent(allData, "", "  ")
}

func ImportData(reader io.Reader, userID int64) (int, error) {
	var allData []db.ExportData

	// Read the entire content first
	content, err := io.ReadAll(reader)
	if err != nil {
		return 0, fmt.Errorf("failed to read input: %w", err)
	}

	// Try to decode as array first
	if err := json.Unmarshal(content, &allData); err != nil {
		// Try to decode as single object
		var singleData db.ExportData
		if err2 := json.Unmarshal(content, &singleData); err2 != nil {
			return 0, fmt.Errorf("invalid JSON format: %w", err)
		}
		allData = []db.ExportData{singleData}
	}

	// Get existing persons for deduplication
	existingPersons, _ := db.GetPersonsByUser(userID)
	personNameMap := make(map[string]*db.Person)
	for i := range existingPersons {
		personNameMap[existingPersons[i].Name] = &existingPersons[i]
	}

	count := 0
	for _, data := range allData {
		person := data.Person

		// Check if person with same name already exists
		var targetPerson *db.Person
		if existing, ok := personNameMap[person.Name]; ok {
			targetPerson = existing
		} else {
			// Create new person
			newPerson, err := db.CreatePerson(userID, person.Name, person.CycleLength, person.PeriodLength)
			if err != nil {
				continue
			}
			targetPerson = newPerson
			personNameMap[person.Name] = newPerson
		}

		// Get existing records for this person to avoid duplicates
		existingRecords, _ := db.GetRecordsByPerson(targetPerson.ID)
		recordKeyMap := make(map[string]bool)
		for _, rec := range existingRecords {
			key := rec.StartDate
			if rec.EndDate != nil && *rec.EndDate != "" {
				key += "_" + *rec.EndDate
			}
			recordKeyMap[key] = true
		}

		for _, rec := range data.Records {
			// Create dedup key from start_date and end_date
			key := rec.StartDate
			if rec.EndDate != nil && *rec.EndDate != "" {
				key += "_" + *rec.EndDate
			}

			// Skip if record already exists
			if recordKeyMap[key] {
				continue
			}

			rec.ID = 0
			rec.PersonID = targetPerson.ID
			_, err := db.CreateRecord(rec.PersonID, rec.StartDate, rec.EndDate, rec.Note)
			if err != nil {
				continue
			}
			recordKeyMap[key] = true
			count++
		}

		// Daily logs use upsert, so they won't duplicate
		for _, log := range data.DailyLogs {
			log.ID = 0
			log.PersonID = targetPerson.ID
			db.UpsertDailyLog(log.PersonID, log.Date, log.FlowLevel, log.Symptoms, log.Note, log.Weight, log.Temperature)
		}
	}

	return count, nil
}
