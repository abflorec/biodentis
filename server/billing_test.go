package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaymentHistorySurvivesRetryAndVoid(t *testing.T) {
	db, err := openDatabase(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	insert, err := db.Exec("INSERT INTO patients(record_number,name) VALUES('QA-CASH','Paciente ficticio')")
	if err != nil {
		t.Fatal(err)
	}
	patientID, err := insert.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO procedures(id,patient_id,name,tooth,description,price_cents,created_at) VALUES(1000,?,'Prueba de caja','16','',30000,?)", patientID, careTime())
	if err != nil {
		t.Fatal(err)
	}
	s := &server{db: db}
	send := func(handler http.HandlerFunc, id, body string, expectedStatus int) payment {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("id", id)
		r = r.WithContext(context.WithValue(r.Context(), actorKey{}, "biodentis.demo"))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != expectedStatus {
			t.Fatalf("respuesta %d en lugar de %d: %s", w.Code, expectedStatus, w.Body.String())
		}
		var movement payment
		if err := json.Unmarshal(w.Body.Bytes(), &movement); err != nil {
			t.Fatal(err)
		}
		return movement
	}
	checkBalance := func(want int64) {
		t.Helper()
		procedure, err := loadProcedure(context.Background(), db, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if procedure.BalanceCents != want || procedure.ClinicalStatus != "planned" {
			t.Fatalf("saldo incorrecto o estado clínico modificado: %+v", procedure)
		}
	}

	const firstBody = `{"amountCents":10000,"method":"cash","receivedDate":"2026-10-01","requestId":"regression-abono"}`
	first := send(s.createPayment, "1000", firstBody, http.StatusCreated)
	if first.ConfirmedAt == "" || first.ReceivedDate != "2026-10-01" || first.Username != "biodentis.demo" {
		t.Fatalf("faltan datos de confirmación: %+v", first)
	}
	checkBalance(20000)
	retry := send(s.createPayment, "1000", firstBody, http.StatusOK)
	if retry.ID != first.ID {
		t.Fatal("la repetición creó otro movimiento")
	}
	checkBalance(20000)

	send(s.createPayment, "1000", `{"amountCents":20000,"method":"cash","receivedDate":"2026-10-02","requestId":"regression-saldo","settle":true}`, http.StatusCreated)
	checkBalance(0)

	var firstID string
	if err := db.QueryRow("SELECT id FROM payments WHERE request_id='regression-abono'").Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	voided := send(s.voidPayment, firstID, `{"reason":"Prueba de anulación"}`, http.StatusOK)
	if voided.ID != first.ID || voided.ConfirmedAt != first.ConfirmedAt || voided.VoidedAt == "" || voided.VoidReason != "Prueba de anulación" {
		t.Fatalf("no se preservó la operación original: %+v", voided)
	}
	checkBalance(10000)

	var movements, audits int
	if err := db.QueryRow("SELECT COUNT(*) FROM payments WHERE procedure_id=1000").Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE entity='payment' AND action IN ('payment.confirmed','payment.voided')").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if movements != 2 || audits != 3 {
		t.Fatalf("historial incompleto: %d movimientos, %d eventos", movements, audits)
	}
}
