package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/driver"
)

func TestLegacyRadiographsSurviveMigrationAndRestore(t *testing.T) {
	imageData := image.NewGray(image.Rect(0, 0, 800, 400))
	for i := range imageData.Pix {
		imageData.Pix[i] = uint8(i*13 ^ (i >> 4))
	}
	var original bytes.Buffer
	if err := png.Encode(&original, imageData); err != nil {
		t.Fatal(err)
	}
	pdf := []byte("%PDF-1.4\n%%EOF\n")
	legacy, err := driver.Open(filepath.Join(t.TempDir(), "biodentis.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(coreSchema + careSchema + schemaV2); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO patients(id,record_number,name) VALUES (1,'HC-TEST','Ficticio');
		INSERT INTO procedures(id,patient_id,name,tooth,description,price_cents,created_at) VALUES (1,1,'Brackets','16','',30000,'2026-01-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	for i, file := range []struct {
		name, mime string
		contents   []byte
	}{
		{"radiografia.png", "image/png", original.Bytes()},
		{"radiografia.pdf", "application/pdf", pdf},
		{"incompleta.png", "image/png", original.Bytes()[:33]},
	} {
		if _, err := legacy.Exec(`INSERT INTO radiographs(procedure_id,name,mime,data,digest,username,created_at)
			VALUES (1,?,?,?,?,'demo','2026-01-01T00:00:00Z')`, file.name, file.mime, file.contents, i); err != nil {
			t.Fatal(err)
		}
	}
	upgradedDir := t.TempDir()
	if _, err := legacy.Exec("VACUUM INTO ?", filepath.Join(upgradedDir, "biodentis.sqlite")); err != nil {
		t.Fatal(err)
	}
	open := func(dir string) *server {
		t.Helper()
		db, err := openDatabase(dir, true)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return &server{db: db, sessions: map[string]loginSession{"test": {
			username: demoUsername, idleUntil: time.Now().Add(time.Hour), expiresAt: time.Now().Add(time.Hour),
		}}}
	}
	s := open(upgradedDir)
	const previewPath = "/api/radiographs/1/thumbnail"
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1"+previewPath, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal("la miniatura se expuso sin sesión")
	}
	preview := updateRequest(t, s, "GET", previewPath, "", http.StatusOK)
	config, err := jpeg.DecodeConfig(bytes.NewReader(preview.Body.Bytes()))
	if err != nil || config.Width != 320 || config.Height != 160 || preview.Header().Get("Content-Type") != "image/jpeg" || preview.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("miniatura inválida: %+v %v", config, err)
	}
	updateRequest(t, s, "GET", "/api/radiographs/2/thumbnail", "", http.StatusNoContent)
	updateRequest(t, s, "GET", "/api/radiographs/3/thumbnail", "", http.StatusNoContent)
	updateRequest(t, s, "GET", "/api/radiographs/9999/thumbnail", "", http.StatusNotFound)
	var cached []byte
	if err := s.db.QueryRow("SELECT data FROM radiograph_thumbnails WHERE radiograph_id=1").Scan(&cached); err != nil || !bytes.Equal(cached, preview.Body.Bytes()) {
		t.Fatalf("miniatura no guardada: %v", err)
	}
	restoredDir := t.TempDir()
	start := time.Now()
	if _, err := s.db.Exec("VACUUM INTO ?", filepath.Join(restoredDir, "biodentis.sqlite")); err != nil {
		t.Fatal(err)
	}
	restored := open(restoredDir)
	t.Logf("Copia SQLite de prueba abierta en %s", time.Since(start).Round(time.Millisecond))
	for path, expected := range map[string][]byte{
		previewPath: cached, "/api/radiographs/1": original.Bytes(), "/api/radiographs/2": pdf,
	} {
		w := updateRequest(t, restored, "GET", path, "", http.StatusOK)
		if !bytes.Equal(w.Body.Bytes(), expected) {
			t.Fatalf("la restauración cambió los bytes de %s", path)
		}
	}
}
