package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const careSchema = `
CREATE TABLE IF NOT EXISTS encounters (
	id INTEGER PRIMARY KEY,
	patient_id INTEGER NOT NULL REFERENCES patients(id),
	appointment_id INTEGER UNIQUE REFERENCES appointments(id),
	visit_date TEXT NOT NULL,
	questionnaire TEXT NOT NULL,
	vitals TEXT NOT NULL,
	diagnosis TEXT NOT NULL,
	prognosis TEXT NOT NULL,
	extraoral TEXT NOT NULL,
	intraoral TEXT NOT NULL,
	notes TEXT NOT NULL,
	laboratory TEXT NOT NULL,
	teeth TEXT NOT NULL,
	username TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS encounters_patient_idx ON encounters(patient_id, visit_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS procedures (
	id INTEGER PRIMARY KEY,
	patient_id INTEGER NOT NULL REFERENCES patients(id),
	encounter_id INTEGER REFERENCES encounters(id),
	name TEXT NOT NULL,
	tooth TEXT NOT NULL,
	description TEXT NOT NULL,
	price_cents INTEGER NOT NULL CHECK(price_cents >= 0),
	clinical_status TEXT NOT NULL DEFAULT 'planned' CHECK(clinical_status IN ('planned','in_progress','completed')),
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS procedures_patient_idx ON procedures(patient_id, id DESC);

CREATE TABLE IF NOT EXISTS consents (
	id INTEGER PRIMARY KEY,
	procedure_id INTEGER NOT NULL REFERENCES procedures(id),
	name TEXT NOT NULL,
	mime TEXT NOT NULL,
	data BLOB NOT NULL,
	username TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS consents_procedure_idx ON consents(procedure_id, id DESC);

CREATE TABLE IF NOT EXISTS payments (
	id INTEGER PRIMARY KEY,
	procedure_id INTEGER NOT NULL REFERENCES procedures(id),
	amount_cents INTEGER NOT NULL CHECK(amount_cents > 0),
	method TEXT NOT NULL,
	received_date TEXT NOT NULL,
	confirmed_at TEXT NOT NULL,
	username TEXT NOT NULL,
	request_id TEXT NOT NULL UNIQUE,
	settled INTEGER NOT NULL,
	voided_at TEXT,
	voided_by TEXT,
	void_reason TEXT
);
CREATE INDEX IF NOT EXISTS payments_procedure_idx ON payments(procedure_id, id DESC);

CREATE TABLE IF NOT EXISTS audit_events (
	id INTEGER PRIMARY KEY,
	action TEXT NOT NULL,
	entity TEXT NOT NULL,
	entity_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	occurred_at TEXT NOT NULL,
	detail TEXT NOT NULL
);
`

type encounterInput struct {
	AppointmentID *int64            `json:"appointmentId"`
	Date          string            `json:"date"`
	Questionnaire map[string]string `json:"questionnaire"`
	Vitals        map[string]string `json:"vitals"`
	Diagnosis     string            `json:"diagnosis"`
	Prognosis     string            `json:"prognosis"`
	Extraoral     string            `json:"extraoral"`
	Intraoral     string            `json:"intraoral"`
	Notes         string            `json:"notes"`
	Laboratory    string            `json:"laboratory"`
	Teeth         map[string]string `json:"teeth"`
}

type encounter struct {
	ID        int64  `json:"id"`
	PatientID int64  `json:"patientId"`
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt"`
	encounterInput
}

type auditEvent struct {
	ID       int64  `json:"id"`
	Action   string `json:"action"`
	Entity   string `json:"entity"`
	EntityID int64  `json:"entityId"`
	Username string `json:"username"`
	At       string `json:"at"`
	Detail   string `json:"detail"`
}

const encounterColumns = "id, patient_id, appointment_id, visit_date, questionnaire, vitals, diagnosis, prognosis, extraoral, intraoral, notes, laboratory, teeth, username, created_at"

type careQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *server) registerCare(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/encounters", s.listEncountersByDate)
	mux.HandleFunc("GET /api/patients/{id}/encounters", s.listEncounters)
	mux.HandleFunc("POST /api/patients/{id}/encounters", s.createEncounter)
	mux.HandleFunc("PUT /api/encounters/{id}", s.updateEncounter)
	mux.HandleFunc("GET /api/patients/{id}/procedures", s.listProcedures)
	mux.HandleFunc("POST /api/patients/{id}/procedures", s.createProcedure)
	mux.HandleFunc("PUT /api/procedures/{id}/status", s.updateProcedureStatus)
	mux.HandleFunc("POST /api/procedures/{id}/consent", s.uploadConsent)
	mux.HandleFunc("GET /api/procedures/{id}/consent", s.downloadConsent)

	mux.HandleFunc("GET /api/procedures/{id}/radiographs", s.listRadiographs)
	mux.HandleFunc("POST /api/procedures/{id}/radiographs", s.uploadRadiograph)
	mux.HandleFunc("GET /api/radiographs/{id}", s.downloadRadiograph)
	mux.HandleFunc("GET /api/radiographs/{id}/thumbnail", s.downloadRadiographThumbnail)
	mux.HandleFunc("GET /api/payments", s.listPayments)
	mux.HandleFunc("POST /api/procedures/{id}/payments", s.createPayment)
	mux.HandleFunc("POST /api/payments/{id}/void", s.voidPayment)
	mux.HandleFunc("GET /api/audit", s.listCareAudit)
	mux.HandleFunc("GET /api/cash/summary", s.cashTotals)
	mux.HandleFunc("GET /api/cash/payments", s.cashPayments)
	mux.HandleFunc("GET /api/cash/audit", s.cashAudit)
}

func careID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := requestedID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Identificador inválido.")
		return 0, false
	}
	return id, true
}

func careInternal(w http.ResponseWriter, err error) {
	internalError(w, "operación clínica o de caja", err)
}

func careRowError(w http.ResponseWriter, err error, notFound string) {
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusNotFound, notFound)
		return
	}
	careInternal(w, err)
}

func careTime() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func validTooth(tooth string) bool {
	if len(tooth) != 2 {
		return false
	}
	quadrant, position := tooth[0], tooth[1]
	return (quadrant >= '1' && quadrant <= '4' && position >= '1' && position <= '8') ||
		(quadrant >= '5' && quadrant <= '8' && position >= '1' && position <= '5')
}

func scanEncounter(row scanner) (encounter, error) {
	var value encounter
	var questionnaire, vitals, teeth string
	err := row.Scan(&value.ID, &value.PatientID, &value.AppointmentID, &value.Date,
		&questionnaire, &vitals, &value.Diagnosis, &value.Prognosis, &value.Extraoral,
		&value.Intraoral, &value.Notes, &value.Laboratory, &teeth, &value.Username, &value.CreatedAt)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal([]byte(questionnaire), &value.Questionnaire); err != nil {
		return value, err
	}
	if err := json.Unmarshal([]byte(vitals), &value.Vitals); err != nil {
		return value, err
	}
	err = json.Unmarshal([]byte(teeth), &value.Teeth)
	return value, err
}

func requireCarePatient(w http.ResponseWriter, ctx context.Context, db careQuerier, id int64) bool {
	var found int
	if err := db.QueryRowContext(ctx, "SELECT 1 FROM patients WHERE id = ?", id).Scan(&found); err != nil {
		careRowError(w, err, "No encontramos al paciente.")
		return false
	}
	return true
}

func (s *server) listEncounters(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok || !requireCarePatient(w, r.Context(), s.db, id) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT "+encounterColumns+" FROM encounters WHERE patient_id = ? ORDER BY visit_date DESC, id DESC", id)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	values := []encounter{}
	for rows.Next() {
		value, err := scanEncounter(rows)
		if err != nil {
			careInternal(w, err)
			return
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, values)
}

func (s *server) listEncountersByDate(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if !validDate(date) {
		jsonError(w, http.StatusBadRequest, "Indica una fecha de atención válida.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT "+encounterColumns+" FROM encounters WHERE visit_date = ? ORDER BY id DESC", date)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	values := []encounter{}
	for rows.Next() {
		value, err := scanEncounter(rows)
		if err != nil {
			careInternal(w, err)
			return
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, values)
}

func decodeEncounter(w http.ResponseWriter, r *http.Request) (encounterInput, bool) {
	var value encounterInput
	if !decodeJSON(w, r, &value) {
		return value, false
	}
	if !validDate(value.Date) {
		jsonError(w, http.StatusBadRequest, "Indica una fecha de atención válida.")
		return value, false
	}
	if value.AppointmentID != nil && *value.AppointmentID < 1 {
		jsonError(w, http.StatusBadRequest, "Identificador de cita inválido.")
		return value, false
	}
	fields := []*string{&value.Diagnosis, &value.Prognosis, &value.Extraoral, &value.Intraoral, &value.Notes, &value.Laboratory}
	for _, field := range fields {
		*field = strings.TrimSpace(*field)
		if len(*field) > 12000 {
			jsonError(w, http.StatusBadRequest, "Uno de los campos clínicos supera el límite permitido.")
			return value, false
		}
	}
	maps := []*map[string]string{&value.Questionnaire, &value.Vitals, &value.Teeth}
	for _, entries := range maps {
		if *entries == nil {
			*entries = map[string]string{}
		}
		if len(*entries) > 60 {
			jsonError(w, http.StatusBadRequest, "Hay demasiados campos en la atención.")
			return value, false
		}
		for key, entry := range *entries {
			if key == "" || len(key) > 120 || len(entry) > 2000 {
				jsonError(w, http.StatusBadRequest, "Uno de los valores de la atención es inválido.")
				return value, false
			}
		}
	}

	for _, key := range []string{"hipertension", "hemorragias", "alergias", "embarazo", "sincope", "diabetes", "hepaticas", "renal", "cardiacos", "angina", "fiebre_reumatica", "otros"} {
		answer := value.Questionnaire[key]
		if answer == "" {
			value.Questionnaire[key] = "no"
		} else if answer != "yes" && answer != "no" {
			jsonError(w, http.StatusBadRequest, "Selecciona Sí o No en los antecedentes.")
			return value, false
		}
	}
	for tooth := range value.Teeth {
		if !validTooth(tooth) {
			jsonError(w, http.StatusBadRequest, "El odontograma contiene una pieza inválida.")
			return value, false
		}
	}
	return value, true
}

func encounterFields(value encounterInput) []any {
	// Maps of strings always encode as valid JSON.
	questionnaire, _ := json.Marshal(value.Questionnaire)
	vitals, _ := json.Marshal(value.Vitals)
	teeth, _ := json.Marshal(value.Teeth)
	return []any{value.Date, string(questionnaire), string(vitals), value.Diagnosis, value.Prognosis,
		value.Extraoral, value.Intraoral, value.Notes, value.Laboratory, string(teeth)}
}

func checkEncounterAppointment(w http.ResponseWriter, ctx context.Context, tx *sql.Tx, patientID int64, appointmentID *int64) bool {
	if appointmentID == nil {
		return true
	}
	var owner int64
	var status string
	err := tx.QueryRowContext(ctx, "SELECT patient_id, status FROM appointments WHERE id = ?", *appointmentID).Scan(&owner, &status)
	if err != nil {
		careRowError(w, err, "No encontramos la cita.")
		return false
	}
	if owner != patientID || status == "canceled" {
		jsonError(w, http.StatusConflict, "La cita no está disponible para esta atención.")
		return false
	}
	var existing int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM encounters WHERE appointment_id = ?", *appointmentID).Scan(&existing)
	if err == nil {
		jsonError(w, http.StatusConflict, "Esta cita ya tiene una atención. Ábrela desde la historia clínica.")
		return false
	}
	if !errors.Is(err, sql.ErrNoRows) {
		careInternal(w, err)
		return false
	}
	return true
}

func (s *server) createEncounter(w http.ResponseWriter, r *http.Request) {
	patientID, ok := careID(w, r)
	if !ok {
		return
	}
	value, ok := decodeEncounter(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer tx.Rollback()
	if !requireCarePatient(w, ctx, tx, patientID) || !checkEncounterAppointment(w, ctx, tx, patientID, value.AppointmentID) {
		return
	}
	username, now := actor(r), careTime()
	args := append([]any{patientID, value.AppointmentID}, encounterFields(value)...)
	args = append(args, username, now)
	result, err := tx.ExecContext(ctx, "INSERT INTO encounters(patient_id,appointment_id,visit_date,questionnaire,vitals,diagnosis,prognosis,extraoral,intraoral,notes,laboratory,teeth,username,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", args...)
	if err != nil {
		careInternal(w, err)
		return
	}
	id, err := result.LastInsertId()
	if err == nil && value.AppointmentID != nil {
		_, err = tx.ExecContext(ctx, "UPDATE appointments SET status = 'completed' WHERE id = ?", *value.AppointmentID)
	}
	if err == nil {
		err = writeCareAudit(ctx, tx, username, "encounter.created", "encounter", id, "Atención registrada.")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, http.StatusCreated, encounter{ID: id, PatientID: patientID, Username: username, CreatedAt: now, encounterInput: value})
}

func (s *server) updateEncounter(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	value, ok := decodeEncounter(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer tx.Rollback()
	previous, err := scanEncounter(tx.QueryRowContext(ctx, "SELECT "+encounterColumns+" FROM encounters WHERE id = ?", id))
	if err != nil {
		careRowError(w, err, "No encontramos la atención.")
		return
	}
	if value.AppointmentID != nil && (previous.AppointmentID == nil || *value.AppointmentID != *previous.AppointmentID) {
		jsonError(w, http.StatusConflict, "La cita de una atención guardada se conserva.")
		return
	}
	value.AppointmentID = previous.AppointmentID
	_, err = tx.ExecContext(ctx, "UPDATE encounters SET visit_date=?,questionnaire=?,vitals=?,diagnosis=?,prognosis=?,extraoral=?,intraoral=?,notes=?,laboratory=?,teeth=? WHERE id=?", append(encounterFields(value), id)...)
	if err == nil {
		err = writeCareAudit(ctx, tx, actor(r), "encounter.updated", "encounter", id, "Atención actualizada.")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	previous.encounterInput = value
	jsonResponse(w, http.StatusOK, previous)
}

func writeCareAudit(ctx context.Context, tx *sql.Tx, username, action, entity string, entityID int64, detail string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO audit_events(action,entity,entity_id,username,occurred_at,detail) VALUES(?,?,?,?,?,?)", action, entity, entityID, username, careTime(), detail)
	return err
}

const auditSelect = "SELECT id, action, entity, entity_id, username, occurred_at, detail FROM audit_events"

func scanAudit(row scanner) (auditEvent, error) {
	var event auditEvent
	err := row.Scan(&event.ID, &event.Action, &event.Entity, &event.EntityID, &event.Username, &event.At, &event.Detail)
	return event, err
}

func (s *server) listCareAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), auditSelect+" ORDER BY id DESC")
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	events := []auditEvent{}
	for rows.Next() {
		event, err := scanAudit(rows)
		if err != nil {
			careInternal(w, err)
			return
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, events)
}
