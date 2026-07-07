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

	data := db.ExportData{
		Person:  *person,
		Records: records,
	}

	return json.MarshalIndent(data, "", "  ")
}

func ExportAll() ([]byte, error) {
	persons, err := db.GetAllPersons()
	if err != nil {
		return nil, fmt.Errorf("failed to get persons: %w", err)
	}

	var allData []db.ExportData
	for _, p := range persons {
		records, err := db.GetRecordsByPerson(p.ID)
		if err != nil {
			continue
		}
		allData = append(allData, db.ExportData{
			Person:  p,
			Records: records,
		})
	}

	return json.MarshalIndent(allData, "", "  ")
}

func ImportData(reader io.Reader) (int, error) {
	var allData []db.ExportData
	if err := json.NewDecoder(reader).Decode(&allData); err != nil {
		return 0, fmt.Errorf("invalid JSON format: %w", err)
	}

	count := 0
	for _, data := range allData {
		person := data.Person
		person.ID = 0
		newPerson, err := db.CreatePerson(person.Name, person.CycleLength, person.PeriodLength)
		if err != nil {
			continue
		}

		for _, rec := range data.Records {
			rec.ID = 0
			rec.PersonID = newPerson.ID
			_, err := db.CreateRecord(rec.PersonID, rec.StartDate, rec.EndDate, rec.Note)
			if err != nil {
				continue
			}
			count++
		}
	}

	return count, nil
}
