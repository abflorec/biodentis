package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/driver"
)

func TestMigrationPreservesLegacyPatient(t *testing.T) {
	db, err := driver.Open(filepath.Join(t.TempDir(), "legacy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(coreSchema + careSchema + `
        INSERT INTO patients(record_number,name,birth_date) VALUES('HC-LEGACY','Paciente previo','1995-06-20');
        PRAGMA user_version=1;`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := initDatabase(db, true); err != nil {
			t.Fatal(err)
		}
	}
	var age int
	var birthDate, name string
	if err := db.QueryRow("SELECT age,birth_date,name FROM patients WHERE record_number='HC-LEGACY'").Scan(&age, &birthDate, &name); err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(time.FixedZone("Lima", -5*60*60))
	expected := now.Year() - 1995
	if now.Format("01-02") < "06-20" {
		expected--
	}
	if age != expected || birthDate != "1995-06-20" || name != "Paciente previo" {
		t.Fatalf("migration lost data: %d %s %s", age, birthDate, name)
	}
}

func updateTestServer(t *testing.T) *server {
	t.Helper()
	db, err := openDatabase(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &server{db: db, sessions: map[string]loginSession{"test": {username: demoUsername, idleUntil: time.Now().Add(time.Hour), expiresAt: time.Now().Add(time.Hour)}}}
}

func updateRequest(t *testing.T, s *server, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s => %d want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w
}

func TestAgeAndNegativeHistoryDefaults(t *testing.T) {
	s := updateTestServer(t)
	updateRequest(t, s, "POST", "/api/patients", `{"name":"Ficticio","age":28}`, 201)
	for _, age := range []string{"-1", "131", "20.5"} {
		updateRequest(t, s, "POST", "/api/patients", `{"name":"Inválido","age":`+age+`}`, 400)
	}
	updateRequest(t, s, "PUT", "/api/patients/1", `{"name":"Ficticio","age":0}`, 200)
	var p patient
	json.Unmarshal(updateRequest(t, s, "GET", "/api/patients/1", "", 200).Body.Bytes(), &p)
	if p.Age == nil || *p.Age != 0 {
		t.Fatal("zero age not persisted")
	}
	updateRequest(t, s, "PUT", "/api/patients/1", `{"name":"Ficticio","age":null}`, 200)
	json.Unmarshal(updateRequest(t, s, "GET", "/api/patients/1", "", 200).Body.Bytes(), &p)
	if p.Age != nil {
		t.Fatal("empty age not persisted")
	}
	w := updateRequest(t, s, "POST", "/api/patients/1/encounters", `{"date":"2026-11-01","questionnaire":{"alergias":"yes"}}`, 201)
	var result struct {
		Questionnaire map[string]string `json:"questionnaire"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Questionnaire["alergias"] != "yes" || result.Questionnaire["diabetes"] != "no" || len(result.Questionnaire) != 12 {
		t.Fatalf("incorrect defaults: %+v", result)
	}
}

func TestRadiographsArePrivateAndPersistWithProcedure(t *testing.T) {
	s := updateTestServer(t)
	updateRequest(t, s, "POST", "/api/patients", `{"name":"Prueba de radiografías"}`, 201)
	updateRequest(t, s, "POST", "/api/patients/1/procedures", `{"name":"Brackets","priceCents":30000}`, 201)
	updateRequest(t, s, "POST", "/api/patients/1/procedures", `{"name":"Resina","priceCents":10000}`, 201)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewGray(image.Rect(0, 0, 10, 10))); err != nil {
		t.Fatal(err)
	}
	upload := func(data []byte, id string, status int) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, _ := writer.CreateFormFile("file", "radiografía.png")
		file.Write(data)
		writer.Close()
		r := httptest.NewRequest("POST", "http://127.0.0.1/api/procedures/"+id+"/radiographs", &body)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		r.AddCookie(&http.Cookie{Name: cookieName, Value: "test"})
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("upload => %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	upload(pngData.Bytes(), "1", 201)
	upload(pngData.Bytes(), "1", 200) // retries cannot create duplicate attachments
	upload([]byte("<script>not an image</script>"), "1", 415)
	upload(pngData.Bytes(), "9999", 404)
	var entries []radiograph
	json.Unmarshal(updateRequest(t, s, "GET", "/api/procedures/1/radiographs", "", 200).Body.Bytes(), &entries)
	if len(entries) != 1 || entries[0].Mime != "image/png" {
		t.Fatalf("invalid gallery: %+v", entries)
	}
	var other []radiograph
	json.Unmarshal(updateRequest(t, s, "GET", "/api/procedures/2/radiographs", "", 200).Body.Bytes(), &other)
	if len(other) != 0 {
		t.Fatal("attachment appeared under another procedure")
	}
	w := updateRequest(t, s, "GET", "/api/radiographs/1", "", 200)
	if !bytes.Equal(w.Body.Bytes(), pngData.Bytes()) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("download changed or cached")
	}
	for _, path := range []string{"/api/radiographs/1", "/api/procedures/1/radiographs"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1"+path, nil)
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("radiograph exposed without login")
		}
	}
	var audits int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='radiograph.attached'").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("wrong audit count: %d", audits)
	}
}
