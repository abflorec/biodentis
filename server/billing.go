package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"
)

type procedureInput struct {
	Name        string `json:"name"`
	Tooth       string `json:"tooth"`
	Description string `json:"description"`
	PriceCents  int64  `json:"priceCents"`
	EncounterID *int64 `json:"encounterId"`
}

type procedure struct {
	ID             int64  `json:"id"`
	PatientID      int64  `json:"patientId"`
	PaidCents      int64  `json:"paidCents"`
	BalanceCents   int64  `json:"balanceCents"`
	ClinicalStatus string `json:"clinicalStatus"`
	PaymentStatus  string `json:"paymentStatus"`
	ConsentName    string `json:"consentName"`
	CreatedAt      string `json:"createdAt"`
	procedureInput
}

type paymentInput struct {
	AmountCents  int64  `json:"amountCents"`
	Method       string `json:"method"`
	ReceivedDate string `json:"receivedDate"`
	RequestID    string `json:"requestId"`
	Settle       bool   `json:"settle"`
}

type payment struct {
	ID            int64  `json:"id"`
	ProcedureID   int64  `json:"procedureId"`
	ProcedureName string `json:"procedureName"`
	PatientName   string `json:"patientName"`
	AmountCents   int64  `json:"amountCents"`
	Method        string `json:"method"`
	ReceivedDate  string `json:"receivedDate"`
	ConfirmedAt   string `json:"confirmedAt"`
	Username      string `json:"username"`
	VoidedAt      string `json:"voidedAt"`
	VoidedBy      string `json:"voidedBy"`
	VoidReason    string `json:"voidReason"`
	settled       bool
}

const proceduresSelect = `SELECT p.id, p.patient_id, p.encounter_id, p.name, p.tooth, p.description,
	p.price_cents, p.clinical_status, p.created_at,
	COALESCE((SELECT SUM(b.amount_cents) FROM payments b WHERE b.procedure_id=p.id AND b.voided_at IS NULL), 0),
	COALESCE((SELECT c.name FROM consents c WHERE c.procedure_id=p.id ORDER BY c.id DESC LIMIT 1), '')
	FROM procedures p`

const paymentsSelect = `SELECT b.id, b.procedure_id, p.name, patient.name, b.amount_cents,
	b.method, b.received_date, b.confirmed_at, b.username, COALESCE(b.voided_at, ''),
	COALESCE(b.voided_by, ''), COALESCE(b.void_reason, ''), b.settled
	FROM payments b JOIN procedures p ON p.id=b.procedure_id
	JOIN patients patient ON patient.id=p.patient_id`

func scanProcedure(row scanner) (procedure, error) {
	var value procedure
	err := row.Scan(&value.ID, &value.PatientID, &value.EncounterID, &value.Name, &value.Tooth, &value.Description,
		&value.PriceCents, &value.ClinicalStatus, &value.CreatedAt, &value.PaidCents, &value.ConsentName)
	if err != nil {
		return value, err
	}
	value.BalanceCents = value.PriceCents - value.PaidCents
	switch {
	case value.BalanceCents == 0:
		value.PaymentStatus = "paid"
	case value.PaidCents > 0:
		value.PaymentStatus = "partial"
	default:
		value.PaymentStatus = "pending"
	}
	return value, nil
}

func loadProcedure(ctx context.Context, db careQuerier, id int64) (procedure, error) {
	return scanProcedure(db.QueryRowContext(ctx, proceduresSelect+" WHERE p.id=?", id))
}

func (s *server) listProcedures(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok || !requireCarePatient(w, r.Context(), s.db, id) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), proceduresSelect+" WHERE p.patient_id=? ORDER BY p.id DESC", id)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	values := []procedure{}
	for rows.Next() {
		value, err := scanProcedure(rows)
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

func (s *server) createProcedure(w http.ResponseWriter, r *http.Request) {
	patientID, ok := careID(w, r)
	if !ok {
		return
	}
	var input procedureInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Tooth = strings.TrimSpace(input.Tooth)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || len(input.Name) > 200 || len(input.Description) > 4000 || input.PriceCents < 0 || input.PriceCents > 999999999999 {
		jsonError(w, http.StatusBadRequest, "Revisa el nombre, el detalle y el importe del procedimiento.")
		return
	}
	if input.Tooth != "" && !validTooth(input.Tooth) {
		jsonError(w, http.StatusBadRequest, "La pieza dental seleccionada es inválida.")
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer tx.Rollback()
	if !requireCarePatient(w, ctx, tx, patientID) {
		return
	}
	if input.EncounterID != nil {
		var owner int64
		err := tx.QueryRowContext(ctx, "SELECT patient_id FROM encounters WHERE id=?", *input.EncounterID).Scan(&owner)
		if err != nil {
			careRowError(w, err, "No encontramos la atención.")
			return
		}
		if owner != patientID {
			jsonError(w, http.StatusConflict, "La atención pertenece a otro paciente.")
			return
		}
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO procedures(patient_id,encounter_id,name,tooth,description,price_cents,created_at) VALUES(?,?,?,?,?,?,?)",
		patientID, input.EncounterID, input.Name, input.Tooth, input.Description, input.PriceCents, careTime())
	if err != nil {
		careInternal(w, err)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		careInternal(w, err)
		return
	}
	err = writeCareAudit(ctx, tx, actor(r), "procedure.created", "procedure", id, "Procedimiento registrado.")
	if err != nil {
		careInternal(w, err)
		return
	}
	commitProcedure(w, ctx, tx, id, http.StatusCreated)
}

func (s *server) updateProcedureStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	var input struct {
		ClinicalStatus string `json:"clinicalStatus"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	switch input.ClinicalStatus {
	case "planned", "in_progress", "completed":
	default:
		jsonError(w, http.StatusBadRequest, "Estado clínico inválido.")
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer tx.Rollback()
	current, err := loadProcedure(ctx, tx, id)
	if err != nil {
		careRowError(w, err, "No encontramos el procedimiento.")
		return
	}
	if current.ClinicalStatus == input.ClinicalStatus {
		jsonResponse(w, http.StatusOK, current)
		return
	}
	_, err = tx.ExecContext(ctx, "UPDATE procedures SET clinical_status=? WHERE id=?", input.ClinicalStatus, id)
	if err == nil {
		err = writeCareAudit(ctx, tx, actor(r), "procedure.status_updated", "procedure", id,
			fmt.Sprintf("Estado clínico de %s a %s.", clinicalStatusLabel(current.ClinicalStatus), clinicalStatusLabel(input.ClinicalStatus)))
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	commitProcedure(w, ctx, tx, id, http.StatusOK)
}

func clinicalStatusLabel(status string) string {
	switch status {
	case "completed":
		return "completado"
	case "in_progress":
		return "en curso"
	default:
		return "planificado"
	}
}

func commitProcedure(w http.ResponseWriter, ctx context.Context, tx *sql.Tx, id int64, status int) {
	value, err := loadProcedure(ctx, tx, id)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, status, value)
}

const maxConsentBytes = 8 << 20

func readConsent(w http.ResponseWriter, r *http.Request) (name, mediaType string, contents []byte, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxConsentBytes+(32<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Selecciona un documento para adjuntar.")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		jsonError(w, http.StatusBadRequest, "Selecciona un documento para adjuntar.")
		return
	}
	defer part.Close()
	contents, err = io.ReadAll(io.LimitReader(part, maxConsentBytes+1))
	if len(contents) > maxConsentBytes {
		jsonError(w, http.StatusRequestEntityTooLarge, "El documento supera los 8 MB.")
		return
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, "No se pudo leer el documento.")
		return
	}
	mediaType = http.DetectContentType(contents)
	switch mediaType {
	case "application/pdf":
	case "image/jpeg", "image/png":
		config, _, err := image.DecodeConfig(bytes.NewReader(contents))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 16000 || config.Height > 16000 ||
			int64(config.Width)*int64(config.Height) > 64_000_000 {
			jsonError(w, http.StatusBadRequest, "La imagen no es válida o tiene un tamaño demasiado grande.")
			return
		}
	default:
		jsonError(w, http.StatusUnsupportedMediaType, "Adjunta un documento PDF, PNG o JPG.")
		return
	}
	name = path.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
	name = strings.TrimSpace(strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return -1
		}
		return value
	}, name))
	if name == "" || name == "." || name == ".." || len(name) > 240 {
		jsonError(w, http.StatusBadRequest, "El nombre del archivo es inválido.")
		return
	}
	ok = true
	return
}

func (s *server) uploadConsent(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	name, mediaType, contents, ok := readConsent(w, r)
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
	if _, err = loadProcedure(ctx, tx, id); err != nil {
		careRowError(w, err, "No encontramos el procedimiento.")
		return
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO consents(procedure_id,name,mime,data,username,created_at) VALUES(?,?,?,?,?,?)",
		id, name, mediaType, contents, actor(r), careTime())
	if err == nil {
		err = writeCareAudit(ctx, tx, actor(r), "consent.attached", "procedure", id, "Se adjuntó un documento de consentimiento.")
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	commitProcedure(w, ctx, tx, id, http.StatusCreated)
}

func (s *server) downloadConsent(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	var name, mediaType string
	var contents []byte
	err := s.db.QueryRowContext(r.Context(), "SELECT name,mime,data FROM consents WHERE procedure_id=? ORDER BY id DESC LIMIT 1", id).
		Scan(&name, &mediaType, &contents)
	if err != nil {
		careRowError(w, err, "Este procedimiento todavía no tiene un consentimiento adjunto.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(contents)))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(contents)
}

func scanPayment(row scanner) (payment, error) {
	var value payment
	err := row.Scan(&value.ID, &value.ProcedureID, &value.ProcedureName, &value.PatientName, &value.AmountCents,
		&value.Method, &value.ReceivedDate, &value.ConfirmedAt, &value.Username, &value.VoidedAt, &value.VoidedBy,
		&value.VoidReason, &value.settled)
	return value, err
}

func (s *server) listPayments(w http.ResponseWriter, r *http.Request) {
	query := paymentsSelect
	var args []any
	if input := r.URL.Query().Get("procedureId"); input != "" {
		id, err := strconv.ParseInt(input, 10, 64)
		if err != nil || id < 1 {
			jsonError(w, http.StatusBadRequest, "Identificador de procedimiento inválido.")
			return
		}
		query += " WHERE b.procedure_id=?"
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(r.Context(), query+" ORDER BY b.id DESC", args...)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	values := []payment{}
	for rows.Next() {
		value, err := scanPayment(rows)
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

func normalizedPaymentMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "cash", "efectivo":
		return "cash"
	case "card", "tarjeta":
		return "card"
	case "transfer", "transferencia":
		return "transfer"
	case "yape":
		return "yape"
	case "plin":
		return "plin"
	case "other", "otro":
		return "other"
	default:
		return ""
	}
}

func soles(cents int64) string {
	return fmt.Sprintf("S/ %d.%02d", cents/100, cents%100)
}

func (s *server) createPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	var input paymentInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Method = normalizedPaymentMethod(input.Method)
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.Method == "" || !validDate(input.ReceivedDate) || input.RequestID == "" || len(input.RequestID) > 128 {
		jsonError(w, http.StatusBadRequest, "Indica un medio, una fecha y una confirmación de cobro válidos.")
		return
	}
	if input.AmountCents < 0 || (!input.Settle && input.AmountCents == 0) {
		jsonError(w, http.StatusBadRequest, "El importe debe ser mayor a cero.")
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer tx.Rollback()
	previous, err := scanPayment(tx.QueryRowContext(ctx, paymentsSelect+" WHERE b.request_id=?", input.RequestID))
	if err == nil {
		if previous.ProcedureID != id || previous.Method != input.Method || previous.ReceivedDate != input.ReceivedDate ||
			previous.settled != input.Settle || (input.AmountCents != 0 && previous.AmountCents != input.AmountCents) {
			jsonError(w, http.StatusConflict, "Esa confirmación ya se usó para otro cobro.")
			return
		}
		jsonResponse(w, http.StatusOK, previous)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		careInternal(w, err)
		return
	}
	current, err := loadProcedure(ctx, tx, id)
	if err != nil {
		careRowError(w, err, "No encontramos el procedimiento.")
		return
	}
	if current.BalanceCents <= 0 {
		jsonError(w, http.StatusConflict, "El procedimiento ya está pagado.")
		return
	}
	amount := input.AmountCents
	if input.Settle {
		if amount != 0 && amount != current.BalanceCents {
			jsonError(w, http.StatusConflict, "El saldo cambió. Revisa el importe antes de confirmar.")
			return
		}
		amount = current.BalanceCents
	}
	if amount > current.BalanceCents {
		jsonError(w, http.StatusConflict, "El importe supera el saldo pendiente.")
		return
	}
	username, confirmedAt := actor(r), careTime()
	result, err := tx.ExecContext(ctx, "INSERT INTO payments(procedure_id,amount_cents,method,received_date,confirmed_at,username,request_id,settled) VALUES(?,?,?,?,?,?,?,?)",
		id, amount, input.Method, input.ReceivedDate, confirmedAt, username, input.RequestID, input.Settle)
	if err != nil {
		careInternal(w, err)
		return
	}
	paymentID, err := result.LastInsertId()
	if err == nil {
		detail := fmt.Sprintf("Cobro de %s recibido el %s. Saldo de %s a %s.",
			soles(amount), input.ReceivedDate, soles(current.BalanceCents), soles(current.BalanceCents-amount))
		err = writeCareAudit(ctx, tx, username, "payment.confirmed", "payment", paymentID, detail)
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	commitPayment(w, ctx, tx, paymentID, http.StatusCreated)
}

func (s *server) voidPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || len(input.Reason) > 500 {
		jsonError(w, http.StatusBadRequest, "Indica un motivo de anulación de hasta 500 caracteres.")
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer tx.Rollback()
	previous, err := scanPayment(tx.QueryRowContext(ctx, paymentsSelect+" WHERE b.id=?", id))
	if err != nil {
		careRowError(w, err, "No encontramos el cobro.")
		return
	}
	if previous.VoidedAt != "" {
		jsonResponse(w, http.StatusOK, previous)
		return
	}
	current, err := loadProcedure(ctx, tx, previous.ProcedureID)
	if err != nil {
		careInternal(w, err)
		return
	}
	username := actor(r)
	_, err = tx.ExecContext(ctx, "UPDATE payments SET voided_at=?,voided_by=?,void_reason=? WHERE id=?", careTime(), username, input.Reason, id)
	if err == nil {
		detail := fmt.Sprintf("Cobro de %s anulado. Saldo de %s a %s.",
			soles(previous.AmountCents), soles(current.BalanceCents), soles(current.BalanceCents+previous.AmountCents))
		err = writeCareAudit(ctx, tx, username, "payment.voided", "payment", id, detail)
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	commitPayment(w, ctx, tx, id, http.StatusOK)
}

func commitPayment(w http.ResponseWriter, ctx context.Context, tx *sql.Tx, id int64, status int) {
	value, err := scanPayment(tx.QueryRowContext(ctx, paymentsSelect+" WHERE b.id=?", id))
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, status, value)
}
