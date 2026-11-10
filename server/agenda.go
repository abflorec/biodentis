package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

type appointment struct {
	ID          int64  `json:"id"`
	PatientID   int64  `json:"patientId"`
	PatientName string `json:"patientName"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	Duration    int    `json:"duration"`
	Reason      string `json:"reason"`
	Status      string `json:"status"`
}

func (s *server) listAppointments(w http.ResponseWriter, r *http.Request) {
	query := `SELECT a.id, a.patient_id, p.name, a.date, a.time, a.duration, a.reason, a.status
		FROM appointments a JOIN patients p ON p.id = a.patient_id`
	date := r.URL.Query().Get("date")
	args := []any{}
	if date != "" {
		if !validDate(date) {
			jsonError(w, http.StatusBadRequest, "Revisa la fecha de la agenda.")
			return
		}
		query += " WHERE a.date = ?"
		args = append(args, date)
	}
	query += " ORDER BY a.date, a.minute_of_day, a.id"
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		internalError(w, "consultar agenda", err)
		return
	}
	defer rows.Close()

	appointments := []appointment{}
	for rows.Next() {
		var a appointment
		if err := rows.Scan(&a.ID, &a.PatientID, &a.PatientName, &a.Date, &a.Time, &a.Duration, &a.Reason, &a.Status); err != nil {
			internalError(w, "leer cita", err)
			return
		}
		appointments = append(appointments, a)
	}
	if err := rows.Err(); err != nil {
		internalError(w, "leer citas", err)
		return
	}
	jsonResponse(w, http.StatusOK, appointments)
}

func (s *server) createAppointment(w http.ResponseWriter, r *http.Request) {
	s.saveAppointment(w, r, 0)
}

func (s *server) updateAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := requestedID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Identificador de cita no válido.")
		return
	}
	s.saveAppointment(w, r, id)
}

func (s *server) saveAppointment(w http.ResponseWriter, r *http.Request, id int64) {
	var a appointment
	if !decodeJSON(w, r, &a) {
		return
	}
	a.ID = id
	a.Reason = strings.TrimSpace(a.Reason)
	minute, err := parseMinutes(a.Time)
	if a.PatientID < 1 || !validDate(a.Date) || err != nil || a.Duration < 5 || a.Duration > 720 || minute+a.Duration > 1440 || utf8.RuneCountInString(a.Reason) > 500 {
		jsonError(w, http.StatusBadRequest, "Revisa el paciente, el motivo y el horario de la cita.")
		return
	}

	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(w, "iniciar guardado de la cita", err)
		return
	}
	defer tx.Rollback()

	if id != 0 {
		var old appointment
		err := tx.QueryRowContext(ctx, "SELECT patient_id, date, time, duration, status FROM appointments WHERE id = ?", id).
			Scan(&old.PatientID, &old.Date, &old.Time, &old.Duration, &old.Status)
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, http.StatusNotFound, "Cita no encontrada.")
			return
		}
		if err != nil {
			internalError(w, "consultar cita", err)
			return
		}
		if a.Status == "" {
			a.Status = old.Status
		}
		if a.PatientID != old.PatientID || (old.Status == "completed" && (a.Status != old.Status || a.Date != old.Date || a.Time != old.Time || a.Duration != old.Duration)) {
			jsonError(w, http.StatusConflict, "El paciente de la cita y el horario de una cita atendida están fijados.")
			return
		}
	} else if a.Status == "" {
		a.Status = "scheduled"
	}
	switch a.Status {
	case "scheduled", "completed", "canceled":
	default:
		jsonError(w, http.StatusBadRequest, "Estado de cita no válido.")
		return
	}

	err = tx.QueryRowContext(ctx, "SELECT name FROM patients WHERE id = ?", a.PatientID).Scan(&a.PatientName)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusNotFound, "Paciente no encontrado.")
		return
	}
	if err != nil {
		internalError(w, "consultar paciente de la cita", err)
		return
	}
	if a.Status != "canceled" {
		var overlap bool
		query := `SELECT EXISTS(SELECT 1 FROM appointments
			WHERE date = ? AND status <> 'canceled' AND id <> ?
			AND minute_of_day < ? AND minute_of_day + duration > ?)`
		if err := tx.QueryRowContext(ctx, query, a.Date, id, minute+a.Duration, minute).Scan(&overlap); err != nil {
			internalError(w, "validar horario de la cita", err)
			return
		}
		if overlap {
			jsonError(w, http.StatusConflict, "Ya hay una cita que se cruza con ese horario.")
			return
		}
	}

	status, action, detail := http.StatusOK, "appointment.updated", "Cita actualizada"
	values := []any{a.PatientID, a.Date, a.Time, minute, a.Duration, a.Reason, a.Status}
	if id == 0 {
		status, action, detail = http.StatusCreated, "appointment.created", "Cita creada"
		query := `INSERT INTO appointments (patient_id, date, time, minute_of_day, duration, reason, status)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`
		err = tx.QueryRowContext(ctx, query, values...).Scan(&a.ID)
	} else {
		query := `UPDATE appointments SET patient_id=?, date=?, time=?, minute_of_day=?, duration=?, reason=?, status=? WHERE id=?`
		_, err = tx.ExecContext(ctx, query, append(values, id)...)
		if a.Status == "canceled" {
			action, detail = "appointment.canceled", "Cita cancelada"
		}
	}
	if err != nil {
		internalError(w, "guardar cita", err)
		return
	}
	if err := writeCareAudit(ctx, tx, actor(r), action, "appointment", a.ID, detail); err != nil {
		internalError(w, "registrar auditoría de cita", err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(w, "guardar cita", err)
		return
	}
	jsonResponse(w, status, a)
}
