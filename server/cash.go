package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const cashPageSize = 50

// Business dates use Lima time. Audit timestamps are stored in UTC.
var clinicTimeZone = time.FixedZone("America/Lima", -5*60*60)

type cashSummary struct {
	CollectedCents int64 `json:"collectedCents"`
	ActivePayments int64 `json:"activePayments"`
}

type cashPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

func cashRange(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" && to == "" {
		today := time.Now().In(clinicTimeZone).Format(time.DateOnly)
		return today, today, true
	}
	if !validDate(from) || !validDate(to) || from > to || to == "9999-12-31" {
		jsonError(w, http.StatusBadRequest, "Selecciona un rango de fechas válido.")
		return "", "", false
	}
	return from, to, true
}

func cashAuditBounds(from, to string) (string, string) {
	start, _ := time.ParseInLocation(time.DateOnly, from, clinicTimeZone)
	end, _ := time.ParseInLocation(time.DateOnly, to, clinicTimeZone)
	const utcWithoutZone = "2006-01-02T15:04:05"
	return start.UTC().Format(utcWithoutZone), end.AddDate(0, 0, 1).UTC().Format(utcWithoutZone)
}

func cashCursor(w http.ResponseWriter, r *http.Request, audit bool) (string, int64, bool) {
	raw := r.URL.Query().Get("before")
	if raw == "" {
		return "", 0, true
	}
	key, identifier, found := strings.Cut(raw, "~")
	id, err := strconv.ParseInt(identifier, 10, 64)
	validKey := validDate(key)
	if audit {
		instant, err := time.Parse(time.RFC3339Nano, key+"Z")
		validKey = err == nil && strings.TrimSuffix(instant.UTC().Format(time.RFC3339Nano), "Z") == key
	}
	if !found || err != nil || id < 1 || !validKey {
		jsonError(w, http.StatusBadRequest, "La página solicitada no es válida.")
		return "", 0, false
	}
	return key, id, true
}

func (s *server) cashTotals(w http.ResponseWriter, r *http.Request) {
	from, to, ok := cashRange(w, r)
	if !ok {
		return
	}
	var result cashSummary
	err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(amount_cents), 0), COUNT(*) FROM payments
		WHERE received_date BETWEEN ? AND ? AND voided_at IS NULL`, from, to).
		Scan(&result.CollectedCents, &result.ActivePayments)
	if err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

func (s *server) cashPayments(w http.ResponseWriter, r *http.Request) {
	from, to, ok := cashRange(w, r)
	if !ok {
		return
	}
	key, id, ok := cashCursor(w, r, false)
	if !ok {
		return
	}
	query := paymentsSelect + " WHERE b.received_date BETWEEN ? AND ?"
	args := []any{from, to}
	if id != 0 {
		query += " AND (b.received_date, b.id) < (?, ?)"
		args = append(args, key, id)
	}
	query += " ORDER BY b.received_date DESC, b.id DESC LIMIT ?"
	writeCashPage(w, r, s.db, query, args, scanPayment, func(p payment) (string, int64) {
		return p.ReceivedDate, p.ID
	})
}

func (s *server) cashAudit(w http.ResponseWriter, r *http.Request) {
	from, to, ok := cashRange(w, r)
	if !ok {
		return
	}
	key, id, ok := cashCursor(w, r, true)
	if !ok {
		return
	}
	start, end := cashAuditBounds(from, to)
	// Removing Z keeps exact seconds before fractional seconds in lexical order.
	const dateKey = "rtrim(occurred_at, 'Z')"
	query := auditSelect + " WHERE " + dateKey + " >= ? AND " + dateKey + " < ?"
	args := []any{start, end}
	if id != 0 {
		query += " AND (" + dateKey + ", id) < (?, ?)"
		args = append(args, key, id)
	}
	query += " ORDER BY " + dateKey + " DESC, id DESC LIMIT ?"
	writeCashPage(w, r, s.db, query, args, scanAudit, func(event auditEvent) (string, int64) {
		return strings.TrimSuffix(event.At, "Z"), event.ID
	})
}

func writeCashPage[T any](w http.ResponseWriter, r *http.Request, db *sql.DB, query string, args []any, scan func(scanner) (T, error), keyFor func(T) (string, int64)) {
	rows, err := db.QueryContext(r.Context(), query, append(args, cashPageSize+1)...)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	page := cashPage[T]{Items: make([]T, 0, cashPageSize)}
	for rows.Next() {
		if len(page.Items) == cashPageSize {
			key, id := keyFor(page.Items[cashPageSize-1])
			page.NextCursor = key + "~" + strconv.FormatInt(id, 10)
			break
		}
		item, err := scan(rows)
		if err != nil {
			careInternal(w, err)
			return
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		careInternal(w, err)
		return
	}
	if err := rows.Close(); err != nil {
		careInternal(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, page)
}
