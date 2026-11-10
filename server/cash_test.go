package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestCashPaginationAndTotalsAtScale(t *testing.T) {
	s := updateTestServer(t)
	today := time.Now().In(clinicTimeZone).Format(time.DateOnly)
	start, end := cashAuditBounds(today, today)
	_, err := s.db.Exec(`INSERT INTO patients(id,record_number,name) VALUES (1,'HC-QA','Paciente ficticio');
		INSERT INTO procedures(id,patient_id,name,tooth,description,price_cents,created_at)
		VALUES (1,1,'Procedimiento de prueba','16','',10000000,'2026-01-01T00:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`WITH RECURSIVE nums(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM nums WHERE n<60000)
		INSERT INTO payments(id,procedure_id,amount_cents,method,received_date,confirmed_at,username,request_id,settled)
		SELECT n,1,100,'cash',?,?,'demo',printf('cobro-%d',n),0 FROM nums`, today, start+"Z")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`WITH RECURSIVE nums(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM nums WHERE n<150000)
		INSERT INTO audit_events(id,action,entity,entity_id,username,occurred_at,detail)
		SELECT n,'patient.updated','patient',1,'demo',?,'Evento ficticio' FROM nums`, start+"Z")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`INSERT INTO audit_events(action,entity,entity_id,username,occurred_at,detail)
		VALUES ('fractional','patient',1,'demo',?,'Fracción del primer segundo'),
		('tomorrow','patient',1,'demo',?,'Fuera del período')`, start+".000000001Z", end+"Z")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`INSERT INTO payments(procedure_id,amount_cents,method,received_date,confirmed_at,username,request_id,settled)
		VALUES (1,100,'cash',date(?, '-1 day'),?,'demo','fuera-del-periodo',0)`, today, start+"Z")
	if err != nil {
		t.Fatal(err)
	}

	rangeQuery := "?from=" + today + "&to=" + today
	checkTotal := func(path string, count int64) {
		t.Helper()
		var summary cashSummary
		cashTestJSON(t, s, path, &summary)
		if summary.ActivePayments != count || summary.CollectedCents != count*100 {
			t.Fatalf("resumen incorrecto: %+v", summary)
		}
	}
	checkTotal("/api/cash/summary"+rangeQuery, 60000)
	paymentsPath := "/api/cash/payments" + rangeQuery
	auditPath := "/api/cash/audit" + rangeQuery
	var payments, nextPayments cashPage[payment]
	var audit, nextAudit cashPage[auditEvent]
	cashTestJSON(t, s, paymentsPath, &payments)
	cashTestJSON(t, s, paymentsPath+"&before="+url.QueryEscape(payments.NextCursor), &nextPayments)
	cashTestJSON(t, s, auditPath, &audit)
	cashTestJSON(t, s, auditPath+"&before="+url.QueryEscape(audit.NextCursor), &nextAudit)
	if len(payments.Items) != 50 || len(nextPayments.Items) != 50 || payments.Items[0].ID != 60000 || nextPayments.Items[0].ID != 59950 {
		t.Fatal("páginas de movimientos incompletas, solapadas o fuera de orden")
	}
	if len(audit.Items) != 50 || len(nextAudit.Items) != 50 || audit.Items[0].Action != "fractional" || nextAudit.Items[0].ID != 149951 {
		t.Fatal("páginas de auditoría incompletas, solapadas o con límites de Lima incorrectos")
	}

	updateRequest(t, s, "POST", "/api/payments/60000/void", `{"reason":"Prueba de caja"}`, http.StatusOK)
	checkTotal("/api/cash/summary"+rangeQuery, 59999)
	cashTestJSON(t, s, paymentsPath, &payments)
	cashTestJSON(t, s, auditPath, &audit)
	if payments.Items[0].VoidedAt == "" || audit.Items[0].Action != "payment.voided" {
		t.Fatal("faltan la anulación o su evento de auditoría")
	}
	updateRequest(t, s, "GET", "/api/cash/summary?from=2026-10-02&to=2026-10-01", "", http.StatusBadRequest)
	updateRequest(t, s, "GET", paymentsPath+"&before=incorrecto", "", http.StatusBadRequest)
	for _, path := range []string{"/api/cash/summary", paymentsPath, auditPath} {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1"+path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s permite acceso sin sesión", path)
		}
	}
}

func cashTestJSON(t *testing.T, s *server, path string, value any) {
	t.Helper()
	w := updateRequest(t, s, "GET", path, "", http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), value); err != nil {
		t.Fatal(err)
	}
}
