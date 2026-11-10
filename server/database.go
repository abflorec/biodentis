package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	"golang.org/x/crypto/bcrypt"
)

const demoUsername = "biodentis.demo"
const demoPassword = "BiodentisDemo2026!"

const coreSchema = `
CREATE TABLE IF NOT EXISTS metadata (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	password_hash BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS patients (
	id INTEGER PRIMARY KEY,
	record_number TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	document TEXT NOT NULL DEFAULT '',
	birth_date TEXT NOT NULL DEFAULT '',
	sex TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	address TEXT NOT NULL DEFAULT '',
	occupation TEXT NOT NULL DEFAULT '',
	marital_status TEXT NOT NULL DEFAULT '',
	other TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS patients_document_unique ON patients(document) WHERE document <> '';
CREATE TABLE IF NOT EXISTS appointments (
	id INTEGER PRIMARY KEY,
	patient_id INTEGER NOT NULL REFERENCES patients(id),
	date TEXT NOT NULL,
	time TEXT NOT NULL,
	minute_of_day INTEGER NOT NULL CHECK (minute_of_day BETWEEN 0 AND 1439),
	duration INTEGER NOT NULL CHECK (duration BETWEEN 5 AND 720),
	reason TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL CHECK (status IN ('scheduled', 'completed', 'canceled')),
	CHECK (minute_of_day + duration <= 1440)
);
CREATE INDEX IF NOT EXISTS appointments_date_idx ON appointments(date, minute_of_day);
CREATE INDEX IF NOT EXISTS appointments_patient_idx ON appointments(patient_id);
`

func openDatabase(dataDir string, demo bool) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "biodentis.sqlite")
	db, err := driver.Open(dbPath, func(conn *sqlite3.Conn) error {
		return conn.Exec("PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000")
	})
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := initDatabase(db, demo); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initDatabase(db *sql.DB, demo bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var schemaVersion int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&schemaVersion); err != nil {
		return err
	}
	switch schemaVersion {
	case 0:
		for _, schema := range []string{coreSchema, careSchema} {
			if _, err := tx.Exec(schema); err != nil {
				return fmt.Errorf("crear esquema: %w", err)
			}
		}
		if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
			return err
		}
	case 1, 2:
	default:
		return fmt.Errorf("la versión %d de la base requiere una versión posterior de BIODENTIS", schemaVersion)
	}

	if schemaVersion < 2 {
		if _, err := tx.Exec(schemaV2); err != nil {
			return fmt.Errorf("actualizar ficha y radiografías: %w", err)
		}
	}

	var storedMode string
	err = tx.QueryRow("SELECT value FROM metadata WHERE key = 'demo'").Scan(&storedMode)
	if err == nil {
		if storedMode != strconv.FormatBool(demo) {
			return errors.New("la carpeta de datos corresponde a otro modo de BIODENTIS")
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	username, password := "biodentis", os.Getenv("BIODENTIS_INITIAL_PASSWORD")
	if demo {
		username, password = demoUsername, demoPassword
	}
	if !validPassword(password) {
		return errors.New("define BIODENTIS_INITIAL_PASSWORD con una contraseña de 12 a 72 bytes para crear la cuenta")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, passwordHash); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO metadata (key, value) VALUES ('demo', ?)", strconv.FormatBool(demo)); err != nil {
		return err
	}
	return tx.Commit()
}
