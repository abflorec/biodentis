package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type patient struct {
	ID            int64  `json:"id"`
	RecordNumber  string `json:"recordNumber"`
	Name          string `json:"name"`
	Document      string `json:"document"`
	BirthDate     string `json:"birthDate"`
	Age           *int   `json:"age"`
	Sex           string `json:"sex"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Address       string `json:"address"`
	Occupation    string `json:"occupation"`
	MaritalStatus string `json:"maritalStatus"`
	Other         string `json:"other"`
}

const patientColumns = "id, record_number, name, document, birth_date, sex, phone, email, address, occupation, marital_status, other, age"

type scanner interface {
	Scan(dest ...any) error
}

func scanPatient(row scanner) (patient, error) {
	var p patient
	err := row.Scan(&p.ID, &p.RecordNumber, &p.Name, &p.Document, &p.BirthDate, &p.Sex, &p.Phone, &p.Email, &p.Address, &p.Occupation, &p.MaritalStatus, &p.Other, &p.Age)
	return p, err
}

func (p *patient) validate() string {
	for _, field := range []struct {
		value *string
		limit int
	}{
		{&p.Name, 150}, {&p.Document, 30}, {&p.BirthDate, 10}, {&p.Sex, 30},
		{&p.Phone, 40}, {&p.Email, 254}, {&p.Address, 300}, {&p.Occupation, 150},
		{&p.MaritalStatus, 50}, {&p.Other, 1000},
	} {
		*field.value = strings.TrimSpace(*field.value)
		if utf8.RuneCountInString(*field.value) > field.limit {
			return "Uno de los campos del paciente supera el máximo permitido."
		}
	}

	if p.Age != nil && (*p.Age < 0 || *p.Age > 130) {
		return "La edad debe ser un número entero entre 0 y 130 años."
	}
	p.Document = strings.ToUpper(p.Document)
	if p.Name == "" {
		return "Ingresa el nombre del paciente."
	}
	if p.BirthDate != "" && (!validDate(p.BirthDate) || p.BirthDate > time.Now().Format(time.DateOnly)) {
		return "Revisa la fecha de nacimiento."
	}
	return ""
}

func validDate(value string) bool {
	date, err := time.Parse(time.DateOnly, value)
	return err == nil && date.Year() > 0 && date.Format(time.DateOnly) == value
}

func (s *server) listPatients(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT "+patientColumns+" FROM patients ORDER BY id DESC")
	if err != nil {
		internalError(w, "consultar pacientes", err)
		return
	}
	defer rows.Close()

	patients := []patient{}
	for rows.Next() {
		p, err := scanPatient(rows)
		if err != nil {
			internalError(w, "leer paciente", err)
			return
		}
		patients = append(patients, p)
	}
	if err := rows.Err(); err != nil {
		internalError(w, "leer pacientes", err)
		return
	}
	jsonResponse(w, http.StatusOK, patients)
}

func (s *server) getPatient(w http.ResponseWriter, r *http.Request) {
	id, err := requestedID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Identificador de paciente no válido.")
		return
	}
	p, err := scanPatient(s.db.QueryRowContext(r.Context(), "SELECT "+patientColumns+" FROM patients WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusNotFound, "Paciente no encontrado.")
		return
	}
	if err != nil {
		internalError(w, "consultar paciente", err)
		return
	}
	jsonResponse(w, http.StatusOK, p)
}

func (s *server) createPatient(w http.ResponseWriter, r *http.Request) {
	s.savePatient(w, r, 0)
}

func (s *server) updatePatient(w http.ResponseWriter, r *http.Request) {
	id, err := requestedID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Identificador de paciente no válido.")
		return
	}
	s.savePatient(w, r, id)
}

func (s *server) savePatient(w http.ResponseWriter, r *http.Request, id int64) {
	var p patient
	if !decodeJSON(w, r, &p) {
		return
	}
	if message := p.validate(); message != "" {
		jsonError(w, http.StatusBadRequest, message)
		return
	}
	p.ID = id

	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(w, "iniciar guardado del paciente", err)
		return
	}
	defer tx.Rollback()
	if p.Document != "" {
		var duplicate bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM patients WHERE document = ? AND id <> ?)", p.Document, id).Scan(&duplicate); err != nil {
			internalError(w, "validar documento del paciente", err)
			return
		}
		if duplicate {
			jsonError(w, http.StatusConflict, "Ya existe un paciente con ese documento.")
			return
		}
	}

	status, action, detail := http.StatusOK, "patient.updated", "Ficha de identificación actualizada"
	values := []any{p.Name, p.Document, p.BirthDate, p.Sex, p.Phone, p.Email, p.Address, p.Occupation, p.MaritalStatus, p.Other, p.Age}
	if id == 0 {
		status, action, detail = http.StatusCreated, "patient.created", "Ficha de identificación creada"
		query := `INSERT INTO patients (record_number, name, document, birth_date, sex, phone, email, address, occupation, marital_status, other, age)
			SELECT printf('HC-%06d', COALESCE(MAX(id), 0) + 1), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? FROM patients
			RETURNING id, record_number`
		err = tx.QueryRowContext(ctx, query, values...).Scan(&p.ID, &p.RecordNumber)
	} else {
		query := `UPDATE patients SET name=?, document=?, birth_date=COALESCE(NULLIF(?, ''), birth_date), sex=?, phone=?, email=?, address=?, occupation=?, marital_status=?, other=?, age=?
			WHERE id=? RETURNING record_number`
		err = tx.QueryRowContext(ctx, query, append(values, id)...).Scan(&p.RecordNumber)
	}
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusNotFound, "Paciente no encontrado.")
		return
	}
	if err != nil {
		internalError(w, "guardar paciente", err)
		return
	}
	if err := writeCareAudit(ctx, tx, actor(r), action, "patient", p.ID, detail); err != nil {
		internalError(w, "registrar auditoría de paciente", err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(w, "guardar paciente", err)
		return
	}
	jsonResponse(w, status, p)
}

func parseMinutes(value string) (int, error) {
	t, err := time.Parse("15:04", value)
	if err != nil || t.Format("15:04") != value {
		return 0, fmt.Errorf("hora no válida")
	}
	return t.Hour()*60 + t.Minute(), nil
}
