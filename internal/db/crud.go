package db

import "time"

// Person CRUD

func GetAllPersons() ([]Person, error) {
	rows, err := DB.Query("SELECT id, name, cycle_length, period_length, created_at FROM persons ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var persons []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.CycleLength, &p.PeriodLength, &p.CreatedAt); err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	return persons, nil
}

func GetPerson(id int64) (*Person, error) {
	var p Person
	err := DB.QueryRow("SELECT id, name, cycle_length, period_length, created_at FROM persons WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.CycleLength, &p.PeriodLength, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func CreatePerson(name string, cycleLength, periodLength int) (*Person, error) {
	result, err := DB.Exec("INSERT INTO persons (name, cycle_length, period_length) VALUES (?, ?, ?)",
		name, cycleLength, periodLength)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return GetPerson(id)
}

func UpdatePerson(id int64, name string, cycleLength, periodLength int) error {
	_, err := DB.Exec("UPDATE persons SET name = ?, cycle_length = ?, period_length = ? WHERE id = ?",
		name, cycleLength, periodLength, id)
	return err
}

func DeletePerson(id int64) error {
	_, err := DB.Exec("DELETE FROM records WHERE person_id = ?", id)
	if err != nil {
		return err
	}
	_, err = DB.Exec("DELETE FROM persons WHERE id = ?", id)
	return err
}

// Record CRUD

func GetRecordsByPerson(personID int64) ([]Record, error) {
	rows, err := DB.Query("SELECT id, person_id, start_date, end_date, note, created_at FROM records WHERE person_id = ? ORDER BY start_date DESC", personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.PersonID, &r.StartDate, &r.EndDate, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func GetRecordsByPersonAndMonth(personID int64, year, month int) ([]Record, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	endDate := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.Local).Format("2006-01-02")

	rows, err := DB.Query(`SELECT id, person_id, start_date, end_date, note, created_at 
		FROM records WHERE person_id = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?) 
		ORDER BY start_date`, personID, endDate, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.PersonID, &r.StartDate, &r.EndDate, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func CreateRecord(personID int64, startDate string, endDate *string, note string) (*Record, error) {
	result, err := DB.Exec("INSERT INTO records (person_id, start_date, end_date, note) VALUES (?, ?, ?, ?)",
		personID, startDate, endDate, note)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return getRecord(id)
}

func UpdateRecord(id int64, startDate string, endDate *string, note string) error {
	_, err := DB.Exec("UPDATE records SET start_date = ?, end_date = ?, note = ? WHERE id = ?",
		startDate, endDate, note, id)
	return err
}

func DeleteRecord(id int64) error {
	_, err := DB.Exec("DELETE FROM records WHERE id = ?", id)
	return err
}

func getRecord(id int64) (*Record, error) {
	var r Record
	err := DB.QueryRow("SELECT id, person_id, start_date, end_date, note, created_at FROM records WHERE id = ?", id).
		Scan(&r.ID, &r.PersonID, &r.StartDate, &r.EndDate, &r.Note, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
