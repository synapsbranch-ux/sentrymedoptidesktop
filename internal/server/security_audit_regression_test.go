package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestNurseCannotWriteOrEraseDoctorsPlan(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.nurse, "Clinical", "Authority")
	for _, field := range []string{"assessment", "treatmentPlan", "followUp"} {
		res := a.request("POST", "/api/v1/encounters", map[string]any{"patientId": p.ID, field: "unauthorized"}, a.nurse)
		if res.Code != 403 {
			t.Fatalf("nurse create %s: %d %s", field, res.Code, res.Body.String())
		}
	}
	created := a.request("POST", "/api/v1/encounters", map[string]any{"patientId": p.ID, "assessment": "Doctor assessment", "treatmentPlan": "Doctor plan", "followUp": "Doctor follow-up"}, a.doctor)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)
	updated := a.request("PUT", "/api/v1/encounters/"+id, map[string]any{"version": 1, "chiefComplaint": "Nurse history"}, a.nurse)
	if updated.Code != 200 {
		t.Fatal(updated.Body.String())
	}
	var assessment, plan, follow string
	if err := a.server.db.QueryRow("SELECT assessment,treatment_plan,follow_up FROM encounters WHERE id=?", id).Scan(&assessment, &plan, &follow); err != nil {
		t.Fatal(err)
	}
	if assessment != "Doctor assessment" || plan != "Doctor plan" || follow != "Doctor follow-up" {
		t.Fatal("nurse erased doctor-only fields")
	}
}

func TestOriginGuardsAndPrivateAPICaching(t *testing.T) {
	a := newTestApp(t)
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/setup/complete", "/api/v1/patients", "/api/v1/backups"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("Origin", "https://attacker.example")
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(a.doctor)
		out := httptest.NewRecorder()
		a.handler.ServeHTTP(out, req)
		if out.Code != 403 {
			t.Fatalf("cross-origin %s: %d", path, out.Code)
		}
	}
	for _, path := range []string{"/api/v1/patients", "/api/v1/auth/me", "/api/v1/settings"} {
		out := a.request("GET", path, nil, a.doctor)
		if out.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("sensitive cache on %s", path)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"identity":"doctor.dev","password":"Doctor-Development-Only-2026"}`))
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	a.handler.ServeHTTP(out, req)
	if out.Code != 200 {
		t.Fatalf("same-origin login: %d %s", out.Code, out.Body.String())
	}
}

func TestMalformedJSONAndInvalidHashesFailClosed(t *testing.T) {
	for _, body := range []string{"null", "[]", "{} {}", `{"field":"` + strings.Repeat("x", 2<<20) + `"}`, `{"field":1}`, `{"unknown":"do not echo this"}`} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var target struct {
			Field string `json:"field"`
		}
		if err := decodeJSON(req, &target); err == nil {
			t.Errorf("accepted malformed request starting %.20s", body)
		}
	}
	for _, hash := range []string{"$argon2id$v=19$m=8,t=0,p=0$AA$AA", "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$", "invalid"} {
		if verifyPassword("password", hash) {
			t.Fatal("invalid password encoding authenticated")
		}
	}
}

func TestRemoteSettingsCannotConfigureExecutable(t *testing.T) {
	a := newTestApp(t)
	settings := decodeResponse[map[string]any](t, a.request("GET", "/api/v1/settings", nil, a.doctor))
	version := settings["versions"].(map[string]any)["clinical"]
	res := a.request("PUT", "/api/v1/settings/clinical", map[string]any{"version": version, "value": map[string]any{"transcriptionEnabled": true, "transcriptionCommand": "arbitrary-command"}}, a.doctor)
	if res.Code != 422 {
		t.Fatalf("remote command configuration: %d %s", res.Code, res.Body.String())
	}
	// Legacy or tampered database command is not an execution source either.
	_, err := a.server.db.Exec(`UPDATE settings SET value_json='{"transcriptionEnabled":true,"transcriptionCommand":"arbitrary-command"}' WHERE key='clinical'`)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTRYMED_TRANSCRIPTION_COMMAND_JSON", "")
	if len(a.server.transcriptionCommand(context.Background())) != 0 {
		t.Fatal("database command was trusted")
	}
}

func TestFinalizedClinicalChildrenRejectWritesAtDatabaseBoundary(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.doctor, "Locked", "Clinical")
	created := a.request("POST", "/api/v1/encounters", map[string]any{"patientId": p.ID}, a.doctor)
	id := decodeResponse[map[string]any](t, created)["id"].(string)
	if res := a.request("POST", "/api/v1/encounters/"+id+"/diagnoses", map[string]any{"diagnosis": "Recorded"}, a.doctor); res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	if res := a.request("POST", "/api/v1/encounters/"+id+"/finalize", map[string]any{"version": 1}, a.doctor); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	for _, query := range []string{"UPDATE pretests SET chief_complaint='tamper' WHERE encounter_id=?", "DELETE FROM diagnoses WHERE encounter_id=?", "UPDATE encounters SET assessment='tamper' WHERE id=?", "DELETE FROM encounters WHERE id=?"} {
		if _, err := a.server.db.Exec(query, id); err == nil {
			t.Fatalf("finalized mutation accepted: %s", query)
		}
	}
	if res := a.request("POST", "/api/v1/encounters/"+id+"/addenda", map[string]any{"body": "Authorized amendment"}, a.doctor); res.Code != 201 {
		t.Fatal(res.Body.String())
	}
}

func TestAppointmentSearchSelectionValidationAndConcurrentBooking(t *testing.T) {
	a := newTestApp(t)
	res := a.request("POST", "/api/v1/patients", map[string]any{"firstName": "Élodie", "lastName": "Pierre", "phone": "+509 3456 7890", "email": "elodie@example.test"}, a.nurse)
	p := decodeResponse[patientRecord](t, res)
	for _, q := range []string{"elod", "ÉLODIE PIER", p.ID, p.MedicalRecordNumber, "34567890", "elodie@example"} {
		page := a.searchPatients(t, "q="+url.QueryEscape(q)+"&limit=10")
		items := page["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["id"] != p.ID {
			t.Fatalf("search %q returned %+v", q, items)
		}
	}
	payload := map[string]any{"patientId": p.ID, "startsAt": "2030-01-15T09:00:00-05:00", "durationMinutes": 30, "type": "eye_exam"}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- a.request("POST", "/api/v1/appointments", payload, a.nurse).Code }()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent booking: %v", counts)
	}
	var patientID, at string
	if err := a.server.db.QueryRow("SELECT patient_id,starts_at FROM appointments WHERE patient_id=?", p.ID).Scan(&patientID, &at); err != nil {
		t.Fatal(err)
	}
	if patientID != p.ID || at != "2030-01-15T14:00:00Z" {
		t.Fatalf("patient/time mapping: %s %s", patientID, at)
	}
	for _, invalid := range []map[string]any{{"patientId": "missing"}, {"durationMinutes": 9999}, {"type": "bogus"}, {"practitionerId": "missing"}} {
		body := map[string]any{}
		for k, v := range payload {
			body[k] = v
		}
		for k, v := range invalid {
			body[k] = v
		}
		out := a.request("POST", "/api/v1/appointments", body, a.nurse)
		if out.Code != 422 {
			t.Fatalf("invalid %v: %d %s", invalid, out.Code, out.Body.String())
		}
	}
}

func TestFinancialRetriesAreAtomicAndBodyBound(t *testing.T) {
	a := newTestApp(t)
	body := map[string]any{"items": []map[string]any{{"description": "Consultation", "quantity": 1, "unitPriceMinor": 1000}}, "currency": "HTG", "status": "issued"}
	request := func(path, key string, payload any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		req.AddCookie(a.doctor)
		out := httptest.NewRecorder()
		a.handler.ServeHTTP(out, req)
		return out
	}
	first := request("/api/v1/invoices", "invoice-1", body)
	if first.Code != 201 {
		t.Fatal(first.Body.String())
	}
	second := request("/api/v1/invoices", "invoice-1", body)
	if first.Body.String() != second.Body.String() {
		t.Fatal("invoice retry changed response")
	}
	invoice := decodeResponse[map[string]any](t, first)["id"].(string)
	payment := map[string]any{"amountMinor": 250, "paymentMethodId": "pm_cash", "currency": "HTG"}
	path := "/api/v1/invoices/" + invoice + "/payments"
	first = request(path, "payment-1", payment)
	if first.Code != 201 {
		t.Fatal(first.Body.String())
	}
	second = request(path, "payment-1", payment)
	if second.Body.String() != first.Body.String() {
		t.Fatal("payment retry changed response")
	}
	payment["amountMinor"] = 300
	if out := request(path, "payment-1", payment); out.Code != 422 {
		t.Fatalf("changed idempotency body accepted: %d", out.Code)
	}
	var paid, count int
	if err := a.server.db.QueryRow("SELECT count(*),sum(amount_minor) FROM payments WHERE invoice_id=?", invoice).Scan(&count, &paid); err != nil {
		t.Fatal(err)
	}
	if count != 1 || paid != 250 {
		t.Fatalf("payment duplicated: %d %d", count, paid)
	}
}

func TestInvoiceRejectsOverflowWithoutWriting(t *testing.T) {
	a := newTestApp(t)
	for _, items := range [][]invoiceLinePayload{
		{{Description: "overflow multiplication", Quantity: 4, UnitPriceMinor: 1<<62 + 1}},
		{{Description: "overflow addition", Quantity: 1, UnitPriceMinor: 1, TaxMinor: 1<<63 - 1}},
		{{Description: "over-discount", Quantity: 1, UnitPriceMinor: 100, DiscountMinor: 101}},
		{{Description: "sum A", Quantity: 1, UnitPriceMinor: maxExactMinor}, {Description: "sum B", Quantity: 1, UnitPriceMinor: 1}},
	} {
		res := a.request("POST", "/api/v1/invoices", invoicePayload{Items: items}, a.doctor)
		if res.Code != 422 {
			t.Fatalf("accepted unsafe amount: %d %s", res.Code, res.Body.String())
		}
	}
	var count int
	if err := a.server.db.QueryRow("SELECT COUNT(*) FROM invoices").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid invoice persisted: %d %v", count, err)
	}
	draft := a.request("POST", "/api/v1/invoices", invoicePayload{Status: "draft", Items: []invoiceLinePayload{{Description: "Draft", Quantity: 1, UnitPriceMinor: 100}}}, a.doctor)
	if draft.Code != 201 || decodeResponse[map[string]any](t, draft)["status"] != "draft" {
		t.Fatal("draft invoice response did not match persisted status")
	}

}

func TestRelatedRecordsMustBelongToPatient(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.doctor, "Link", "Owner")
	other := a.createPatient(a.doctor, "Different", "Patient")
	appointment := a.request("POST", "/api/v1/appointments", map[string]any{"patientId": p.ID, "startsAt": "2035-04-01T10:00:00Z", "type": "eye_exam", "durationMinutes": 30}, a.doctor)
	if appointment.Code != 201 {
		t.Fatal(appointment.Body.String())
	}
	appointmentID := decodeResponse[map[string]any](t, appointment)["id"]
	res := a.request("POST", "/api/v1/encounters", map[string]any{"patientId": other.ID, "appointmentId": appointmentID}, a.doctor)
	if res.Code != 422 {
		t.Fatalf("cross-patient consultation: %d %s", res.Code, res.Body.String())
	}
	invoice := a.request("POST", "/api/v1/invoices", invoicePayload{PatientID: p.ID, Items: []invoiceLinePayload{{Description: "Item", Quantity: 1, UnitPriceMinor: 100}}}, a.doctor)
	if invoice.Code != 201 {
		t.Fatal(invoice.Body.String())
	}
	invoiceID := decodeResponse[map[string]any](t, invoice)["id"]
	res = a.request("POST", "/api/v1/lab-orders", map[string]any{"patientId": other.ID, "invoiceId": invoiceID}, a.doctor)
	if res.Code != 422 {
		t.Fatalf("cross-patient lab: %d %s", res.Code, res.Body.String())
	}
	res = a.request("POST", "/api/v1/insurance/claims", map[string]any{"patientId": other.ID, "invoiceId": invoiceID, "payerId": "irrelevant", "claimAmountMinor": 100, "payerPortionMinor": 100}, a.doctor)
	if res.Code != 422 {
		t.Fatalf("cross-patient insurance: %d %s", res.Code, res.Body.String())
	}
	lab := a.request("POST", "/api/v1/lab-orders", map[string]any{"patientId": p.ID}, a.doctor)
	if lab.Code != 201 {
		t.Fatal(lab.Body.String())
	}
	labID := decodeResponse[map[string]any](t, lab)["id"].(string)
	res = a.request("PATCH", "/api/v1/lab-orders/"+labID+"/status", map[string]any{"status": "delivered", "version": 1}, a.doctor)
	if res.Code != 422 {
		t.Fatalf("delivery bypassed QC: %d %s", res.Code, res.Body.String())
	}
}

func TestTLSProxyRequiresExplicitOriginAndLoopback(t *testing.T) {
	s := &Server{}
	s.config.PublicURL = "https://clinic.example:9443"
	for _, check := range []struct {
		host, peer string
		secure     bool
	}{
		{"clinic.example:9443", "127.0.0.1:3456", true},
		{"attacker.example", "127.0.0.1:3456", false},
		{"clinic.example:9443", "192.168.1.2:3456", false},
	} {
		r := httptest.NewRequest("GET", "http://"+check.host+"/api/v1/auth/me", nil)
		r.RemoteAddr = check.peer
		r.Header.Set("X-Forwarded-Proto", "https")
		if s.secureRequest(r) != check.secure {
			t.Fatalf("proxy trust for %s via %s", check.host, check.peer)
		}
	}
}
