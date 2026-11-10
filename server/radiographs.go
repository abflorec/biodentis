package main

import (
	"crypto/sha256"
	"fmt"
	"mime"
	"net/http"
	"strconv"
)

// Preserve legacy dates and vital signs; the new interface uses declared age.
const schemaV2 = `
ALTER TABLE patients ADD COLUMN age INTEGER CHECK(age BETWEEN 0 AND 130);
UPDATE patients SET age = CAST(strftime('%Y', 'now', '-5 hours') AS INTEGER)
    - CAST(substr(birth_date, 1, 4) AS INTEGER)
    - (strftime('%m-%d', 'now', '-5 hours') < substr(birth_date, 6, 5))
WHERE birth_date <> '' AND date(birth_date) IS NOT NULL
    AND CAST(strftime('%Y', 'now', '-5 hours') AS INTEGER) - CAST(substr(birth_date, 1, 4) AS INTEGER) BETWEEN 0 AND 130;
CREATE TABLE radiographs (
    id INTEGER PRIMARY KEY,
    procedure_id INTEGER NOT NULL REFERENCES procedures(id),
    name TEXT NOT NULL,
    mime TEXT NOT NULL CHECK(mime IN ('image/png', 'image/jpeg', 'application/pdf')),
    data BLOB NOT NULL,
    digest TEXT NOT NULL,
    username TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(procedure_id, digest)
);
PRAGMA user_version = 2;
`

type radiograph struct {
	ID          int64  `json:"id"`
	ProcedureID int64  `json:"procedureId"`
	Name        string `json:"name"`
	Mime        string `json:"mime"`
	CreatedAt   string `json:"createdAt"`
}

func (s *server) listRadiographs(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	if _, err := loadProcedure(r.Context(), s.db, id); err != nil {
		careRowError(w, err, "No encontramos el procedimiento.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,procedure_id,name,mime,created_at FROM radiographs WHERE procedure_id=? ORDER BY id DESC", id)
	if err != nil {
		careInternal(w, err)
		return
	}
	defer rows.Close()
	values := []radiograph{}
	for rows.Next() {
		var value radiograph
		if err := rows.Scan(&value.ID, &value.ProcedureID, &value.Name, &value.Mime, &value.CreatedAt); err != nil {
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

func (s *server) uploadRadiograph(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	// Same content validation and bounded upload as signed documents.
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
	digest := fmt.Sprintf("%x", sha256.Sum256(contents))
	result, err := tx.ExecContext(ctx, `INSERT INTO radiographs(procedure_id,name,mime,data,digest,username,created_at)
        VALUES(?,?,?,?,?,?,?) ON CONFLICT(procedure_id,digest) DO NOTHING`, id, name, mediaType, contents, digest, actor(r), careTime())
	if err != nil {
		careInternal(w, err)
		return
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		careInternal(w, err)
		return
	}
	if inserted > 0 {
		if err := writeCareAudit(ctx, tx, actor(r), "radiograph.attached", "procedure", id, "Se adjuntó una radiografía al procedimiento."); err != nil {
			careInternal(w, err)
			return
		}
	}
	var value radiograph
	err = tx.QueryRowContext(ctx, "SELECT id,procedure_id,name,mime,created_at FROM radiographs WHERE procedure_id=? AND digest=?", id, digest).
		Scan(&value.ID, &value.ProcedureID, &value.Name, &value.Mime, &value.CreatedAt)
	if err != nil {
		careInternal(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		careInternal(w, err)
		return
	}
	status := http.StatusCreated
	if inserted == 0 {
		status = http.StatusOK
	}
	jsonResponse(w, status, value)
}

func (s *server) downloadRadiograph(w http.ResponseWriter, r *http.Request) {
	id, ok := careID(w, r)
	if !ok {
		return
	}
	var name, mediaType string
	var contents []byte
	err := s.db.QueryRowContext(r.Context(), "SELECT name,mime,data FROM radiographs WHERE id=?", id).Scan(&name, &mediaType, &contents)
	if err != nil {
		careRowError(w, err, "No encontramos la radiografía.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(contents)))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(contents)
}
