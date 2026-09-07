package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/app"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
)

type testApp struct {
	t       *testing.T
	server  *Server
	handler http.Handler
	doctor  *http.Cookie
	nurse   *http.Cookie
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	dataDir := t.TempDir()
	for _, directory := range []string{"database", "documents", "backups", "logs"} {
		if err := os.MkdirAll(filepath.Join(dataDir, directory), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	db, err := database.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := SeedDevelopment(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(db, app.Config{DataDir: dataDir, Address: "127.0.0.1:0", Dev: true}, logger)
	result := &testApp{t: t, server: server, handler: server.Handler()}
	result.doctor = result.login("doctor.dev", "Doctor-Development-Only-2026")
	result.nurse = result.login("nurse.dev", "Nurse-Development-Only-2026")
	return result
}

func (a *testApp) login(identity, password string) *http.Cookie {
	a.t.Helper()
	response := a.request(http.MethodPost, "/api/v1/auth/login", map[string]any{"identity": identity, "password": password}, nil)
	if response.Code != http.StatusOK {
		a.t.Fatalf("login failed: %d %s", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookie {
			return cookie
		}
	}
	a.t.Fatal("login did not return session cookie")
	return nil
}

func (a *testApp) request(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	a.t.Helper()
	var encoded io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		encoded = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, encoded)
	request.RemoteAddr = "127.0.0.1:1234"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v (%s)", err, response.Body.String())
	}
	return result
}

func (a *testApp) createPatient(cookie *http.Cookie, first, last string) patientRecord {
	a.t.Helper()
	response := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": first, "middleName": "", "lastName": last, "preferredName": "", "sex": "", "dateOfBirth": "", "phone": "", "alternatePhone": "", "email": "", "address": "", "city": "", "occupation": "", "employer": "", "preferredLanguage": "", "communicationPreference": "", "referralSource": "", "referringProvider": "", "notes": "", "tags": []string{},
	}, cookie)
	if response.Code != http.StatusCreated {
		a.t.Fatalf("create patient: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[patientRecord](a.t, response)
}

func patientUpdateBody(patient patientRecord, notes string) map[string]any {
	return map[string]any{
		"firstName": patient.FirstName, "middleName": patient.MiddleName, "lastName": patient.LastName, "preferredName": patient.PreferredName,
		"sex": patient.Sex, "dateOfBirth": patient.DateOfBirth, "phone": patient.Phone, "alternatePhone": patient.AlternatePhone, "email": patient.Email,
		"address": patient.Address, "city": patient.City, "occupation": patient.Occupation, "employer": patient.Employer, "preferredLanguage": patient.PreferredLanguage,
		"communicationPreference": patient.CommunicationPreference, "referralSource": patient.ReferralSource, "referringProvider": patient.ReferringProvider,
		"notes": notes, "tags": patient.Tags, "version": patient.Version,
	}
}

func TestAuthenticationAndRBAC(t *testing.T) {
	a := newTestApp(t)
	unauthenticated := a.request(http.MethodGet, "/api/v1/patients", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	nurseUsers := a.request(http.MethodGet, "/api/v1/users", nil, a.nurse)
	if nurseUsers.Code != http.StatusForbidden {
		t.Fatalf("nurse users status = %d, want 403", nurseUsers.Code)
	}
	nurseDiagnosis := a.request(http.MethodPost, "/api/v1/encounters/not-real/diagnoses", map[string]any{"diagnosis": "Forbidden"}, a.nurse)
	if nurseDiagnosis.Code != http.StatusForbidden {
		t.Fatalf("nurse diagnosis status = %d, want 403", nurseDiagnosis.Code)
	}
	doctorUsers := a.request(http.MethodGet, "/api/v1/users", nil, a.doctor)
	if doctorUsers.Code != http.StatusOK || bytes.Contains(doctorUsers.Body.Bytes(), []byte("password_hash")) {
		t.Fatalf("doctor users response invalid or leaked hash: %d %s", doctorUsers.Code, doctorUsers.Body.String())
	}
}

func TestDesktopSessionTransportCannotBeUsedFromLAN(t *testing.T) {
	a := newTestApp(t)
	desktop := DesktopHandler(a.server.Handler())
	loginBody, _ := json.Marshal(map[string]string{
		"identity": "doctor.dev",
		"password": "Doctor-Development-Only-2026",
	})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginRequest.RemoteAddr = "192.0.2.1:1234"
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	desktop.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("desktop login: %d %s", loginResponse.Code, loginResponse.Body.String())
	}
	login := decodeResponse[struct {
		DesktopSessionToken string `json:"desktopSessionToken"`
	}](t, loginResponse)
	if login.DesktopSessionToken == "" {
		t.Fatal("desktop login did not return its in-process session token")
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.RemoteAddr = "192.0.2.1:1234"
	meRequest.Header.Set("Authorization", "SentryMed "+login.DesktopSessionToken)
	meResponse := httptest.NewRecorder()
	desktop.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("desktop token rejected in Wails handler: %d %s", meResponse.Code, meResponse.Body.String())
	}

	lanRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	lanRequest.RemoteAddr = "192.168.1.25:1234"
	lanRequest.Header.Set("Authorization", "SentryMed "+login.DesktopSessionToken)
	lanResponse := httptest.NewRecorder()
	a.handler.ServeHTTP(lanResponse, lanRequest)
	if lanResponse.Code != http.StatusUnauthorized {
		t.Fatalf("desktop token accepted over LAN: %d", lanResponse.Code)
	}
}

func TestPatientOptimisticConcurrency(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Marie", "Joseph")
	first := a.request(http.MethodPut, "/api/v1/patients/"+patient.ID, patientUpdateBody(patient, "Doctor update"), a.doctor)
	if first.Code != http.StatusOK {
		t.Fatalf("first update: %d %s", first.Code, first.Body.String())
	}
	second := a.request(http.MethodPut, "/api/v1/patients/"+patient.ID, patientUpdateBody(patient, "Nurse stale update"), a.nurse)
	if second.Code != http.StatusConflict {
		t.Fatalf("stale update: %d %s", second.Code, second.Body.String())
	}
	errorBody := decodeResponse[APIError](t, second)
	if errorBody.Code != "CONCURRENT_MODIFICATION" {
		t.Fatalf("error code = %s", errorBody.Code)
	}
}

func TestDoctorAndNurseConcurrentIndependentWrites(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Patient", "Concurrent")
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Blurred vision", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create encounter: %d %s", created.Code, created.Body.String())
	}
	encounter := decodeResponse[map[string]any](t, created)
	encounterID := encounter["id"].(string)
	start := make(chan struct{})
	results := make(chan int, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		response := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID, map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Blurred vision", "hpi": "Progressive", "assessment": "Myopia", "treatmentPlan": "Spectacles", "followUp": "1 year", "version": 1}, a.doctor)
		results <- response.Code
	}()
	go func() {
		defer wait.Done()
		<-start
		response := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/pretest", map[string]any{"chiefComplaint": "Blurred vision", "vitals": map[string]any{}, "visualAcuity": map[string]any{"odDistanceVA": "20/40", "osDistanceVA": "20/30"}, "autorefraction": map[string]any{"odSphere": "-1.50"}, "keratometry": map[string]any{}, "iop": map[string]any{"odIOP": "17", "osIOP": "16"}, "pupils": "PERRLA", "eom": "Full", "coverTest": "Ortho", "confrontationFields": "Full", "colorVision": "Normal", "stereopsis": "", "pachymetry": map[string]any{}, "lensometry": map[string]any{}, "complete": true, "version": 1}, a.nurse)
		results <- response.Code
	}()
	close(start)
	wait.Wait()
	close(results)
	for status := range results {
		if status != http.StatusOK {
			t.Fatalf("concurrent independent write status = %d", status)
		}
	}
	detail := a.request(http.MethodGet, "/api/v1/encounters/"+encounterID, nil, a.doctor)
	if detail.Code != http.StatusOK || !bytes.Contains(detail.Body.Bytes(), []byte("Progressive")) || !bytes.Contains(detail.Body.Bytes(), []byte("20/40")) {
		t.Fatalf("independent changes did not both persist: %d %s", detail.Code, detail.Body.String())
	}
}

func TestFinalizedEncounterIsLockedAndAudited(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Lock", "Test")
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Pain", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	encounterID := decodeResponse[map[string]any](t, created)["id"].(string)
	diagnosis := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/diagnoses", map[string]any{"diagnosis": "Dry eye", "code": "H04.129", "laterality": "OU", "notes": "", "primary": true}, a.doctor)
	if diagnosis.Code != http.StatusCreated {
		t.Fatalf("diagnosis: %d %s", diagnosis.Code, diagnosis.Body.String())
	}
	prescription := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{"patientId": patient.ID, "encounterId": encounterID, "type": "spectacle", "od": map[string]any{"sphere": "-1.00"}, "os": map[string]any{"sphere": "-0.75"}, "details": map[string]any{"pd": "62"}, "notes": "Distance wear", "expiresAt": ""}, a.doctor)
	if prescription.Code != http.StatusCreated {
		t.Fatalf("prescription: %d %s", prescription.Code, prescription.Body.String())
	}
	finalized := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if finalized.Code != http.StatusOK {
		t.Fatalf("finalize: %d %s", finalized.Code, finalized.Body.String())
	}
	edit := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID, map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Changed", "chiefComplaint": "", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": "", "version": 2}, a.doctor)
	if edit.Code != http.StatusLocked {
		t.Fatalf("finalized edit status = %d, want 423: %s", edit.Code, edit.Body.String())
	}
	var auditCount int
	if err := a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM audit_logs WHERE action='finalize' AND entity_id=?", encounterID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("finalize audit count = %d, error %v", auditCount, err)
	}
}

func TestPOSIsAtomicAndSupportsPartialPayment(t *testing.T) {
	a := newTestApp(t)
	itemResponse := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "FRM-TEST", "barcode": "", "category": "frame", "name": "Test Frame", "brand": "", "model": "", "attributes": map[string]any{}, "supplierId": "", "costMinor": 5000, "salePriceMinor": 10000, "currency": "HTG", "quantity": 2, "reorderLevel": 1, "trackStock": true}, a.nurse)
	if itemResponse.Code != http.StatusCreated {
		t.Fatalf("inventory item: %d %s", itemResponse.Code, itemResponse.Body.String())
	}
	itemID := decodeResponse[map[string]any](t, itemResponse)["id"].(string)
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice": map[string]any{"patientId": "", "currency": "HTG", "exchangeRate": "1", "discountMinor": 0, "taxMinor": 0, "dueAt": "", "notes": "", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Test Frame", "quantity": 1, "unitPriceMinor": 10000, "discountMinor": 0, "taxMinor": 0}}},
		"payment": map[string]any{"paymentMethodId": "pm_cash", "registerSessionId": "", "amountMinor": 4000, "currency": "HTG", "exchangeRate": "1", "reference": "", "notes": ""},
	}, a.nurse)
	if checkout.Code != http.StatusCreated {
		t.Fatalf("checkout: %d %s", checkout.Code, checkout.Body.String())
	}
	result := decodeResponse[map[string]any](t, checkout)
	if result["balanceMinor"].(float64) != 6000 {
		t.Fatalf("balance = %v", result["balanceMinor"])
	}
	currencyMismatch := a.request(http.MethodPost, "/api/v1/invoices/"+result["invoiceId"].(string)+"/payments", map[string]any{"paymentMethodId": "pm_cash", "registerSessionId": "", "amountMinor": 1000, "currency": "USD", "exchangeRate": "132.50", "reference": "", "notes": ""}, a.nurse)
	if currencyMismatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-currency payment status = %d, want 422: %s", currencyMismatch.Code, currencyMismatch.Body.String())
	}
	var quantity int
	var movementCount int
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT quantity FROM inventory_items WHERE id=?", itemID).Scan(&quantity)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM stock_movements WHERE item_id=? AND movement_type='sale'", itemID).Scan(&movementCount)
	if quantity != 1 || movementCount != 1 {
		t.Fatalf("stock quantity=%d movementCount=%d", quantity, movementCount)
	}
	finance := a.request(http.MethodGet, "/api/v1/finance/summary", nil, a.doctor)
	if finance.Code != http.StatusOK || !bytes.Contains(finance.Body.Bytes(), []byte(`"baseCurrency":"HTG"`)) {
		t.Fatalf("finance summary invalid: %d %s", finance.Code, finance.Body.String())
	}
}

func TestPurchaseOrderPartialReceiptIsTransactionalAndVersioned(t *testing.T) {
	a := newTestApp(t)
	itemResponse := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "PO-FRAME", "barcode": "", "category": "frame", "name": "Purchase Frame", "brand": "", "model": "", "attributes": map[string]any{}, "supplierId": "", "costMinor": 2500, "salePriceMinor": 6000, "currency": "HTG", "quantity": 2, "reorderLevel": 1, "trackStock": true}, a.nurse)
	if itemResponse.Code != http.StatusCreated {
		t.Fatalf("inventory item: %d %s", itemResponse.Code, itemResponse.Body.String())
	}
	itemID := decodeResponse[map[string]any](t, itemResponse)["id"].(string)
	supplierResponse := a.request(http.MethodPost, "/api/v1/suppliers", map[string]any{"company": "Optical Supply", "contactPerson": "Jean", "phone": "", "email": "", "address": "", "notes": "", "version": 0}, a.doctor)
	if supplierResponse.Code != http.StatusCreated {
		t.Fatalf("supplier: %d %s", supplierResponse.Code, supplierResponse.Body.String())
	}
	supplierID := decodeResponse[map[string]any](t, supplierResponse)["id"].(string)
	orderBody := map[string]any{"supplierId": supplierID, "currency": "HTG", "expectedAt": "", "notes": "", "items": []map[string]any{{"inventoryItemId": itemID, "quantity": 5, "unitCostMinor": 2500}}}
	forbidden := a.request(http.MethodPost, "/api/v1/purchase-orders", orderBody, a.nurse)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse purchase order status=%d, want 403", forbidden.Code)
	}
	created := a.request(http.MethodPost, "/api/v1/purchase-orders", orderBody, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("purchase order: %d %s", created.Code, created.Body.String())
	}
	orderID := decodeResponse[map[string]any](t, created)["id"].(string)
	detail := a.request(http.MethodGet, "/api/v1/purchase-orders/"+orderID, nil, a.doctor)
	if detail.Code != http.StatusOK {
		t.Fatalf("purchase order detail: %d %s", detail.Code, detail.Body.String())
	}
	detailBody := decodeResponse[map[string]any](t, detail)
	lineID := detailBody["items"].([]any)[0].(map[string]any)["id"].(string)
	sent := a.request(http.MethodPatch, "/api/v1/purchase-orders/"+orderID+"/status", map[string]any{"status": "sent", "version": 1}, a.doctor)
	if sent.Code != http.StatusOK {
		t.Fatalf("send purchase order: %d %s", sent.Code, sent.Body.String())
	}
	partial := a.request(http.MethodPost, "/api/v1/purchase-orders/"+orderID+"/receive", map[string]any{"version": 2, "notes": "first box", "items": []map[string]any{{"itemId": lineID, "quantity": 3}}}, a.doctor)
	if partial.Code != http.StatusOK || !bytes.Contains(partial.Body.Bytes(), []byte(`"status":"partial"`)) {
		t.Fatalf("partial receipt: %d %s", partial.Code, partial.Body.String())
	}
	stale := a.request(http.MethodPost, "/api/v1/purchase-orders/"+orderID+"/receive", map[string]any{"version": 2, "notes": "stale", "items": []map[string]any{{"itemId": lineID, "quantity": 1}}}, a.doctor)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale receipt status=%d, want 409: %s", stale.Code, stale.Body.String())
	}
	complete := a.request(http.MethodPost, "/api/v1/purchase-orders/"+orderID+"/receive", map[string]any{"version": 3, "notes": "remainder", "items": []map[string]any{{"itemId": lineID, "quantity": 2}}}, a.doctor)
	if complete.Code != http.StatusOK || !bytes.Contains(complete.Body.Bytes(), []byte(`"status":"received"`)) {
		t.Fatalf("complete receipt: %d %s", complete.Code, complete.Body.String())
	}
	var quantity, movements, receipts int
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT quantity FROM inventory_items WHERE id=?", itemID).Scan(&quantity)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM stock_movements WHERE item_id=? AND movement_type='purchase'", itemID).Scan(&movements)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM purchase_order_receipts WHERE purchase_order_id=?", orderID).Scan(&receipts)
	if quantity != 7 || movements != 2 || receipts != 2 {
		t.Fatalf("receipt persistence quantity=%d movements=%d receipts=%d", quantity, movements, receipts)
	}
}

func TestStockTakeCountsAndFinalizesAuditedCorrection(t *testing.T) {
	a := newTestApp(t)
	itemResponse := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "COUNT-CASE", "barcode": "", "category": "accessory", "name": "Counted Case", "brand": "", "model": "", "attributes": map[string]any{}, "supplierId": "", "costMinor": 100, "salePriceMinor": 200, "currency": "HTG", "quantity": 8, "reorderLevel": 1, "trackStock": true}, a.nurse)
	itemID := decodeResponse[map[string]any](t, itemResponse)["id"].(string)
	forbidden := a.request(http.MethodPost, "/api/v1/stock-takes", map[string]any{"category": "accessory", "notes": ""}, a.nurse)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse start stock take status=%d, want 403", forbidden.Code)
	}
	created := a.request(http.MethodPost, "/api/v1/stock-takes", map[string]any{"category": "accessory", "notes": "Monthly count"}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("start stock take: %d %s", created.Code, created.Body.String())
	}
	takeID := decodeResponse[map[string]any](t, created)["id"].(string)
	detail := a.request(http.MethodGet, "/api/v1/stock-takes/"+takeID, nil, a.nurse)
	detailBody := decodeResponse[map[string]any](t, detail)
	line := detailBody["items"].([]any)[0].(map[string]any)
	lineID := line["id"].(string)
	counted := a.request(http.MethodPut, "/api/v1/stock-takes/"+takeID+"/items/"+lineID, map[string]any{"countedQuantity": 7, "reason": "Damaged case removed", "version": 1}, a.nurse)
	if counted.Code != http.StatusOK {
		t.Fatalf("nurse count: %d %s", counted.Code, counted.Body.String())
	}
	finalizeForbidden := a.request(http.MethodPost, "/api/v1/stock-takes/"+takeID+"/finalize", map[string]any{"version": 1}, a.nurse)
	if finalizeForbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse finalize status=%d, want 403", finalizeForbidden.Code)
	}
	finalized := a.request(http.MethodPost, "/api/v1/stock-takes/"+takeID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if finalized.Code != http.StatusOK {
		t.Fatalf("finalize stock take: %d %s", finalized.Code, finalized.Body.String())
	}
	var quantity, corrections, auditCount int
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT quantity FROM inventory_items WHERE id=?", itemID).Scan(&quantity)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM stock_movements WHERE item_id=? AND movement_type='correction' AND reference_type='stock_take'", itemID).Scan(&corrections)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM audit_logs WHERE entity_type='stock_take' AND entity_id=? AND action='finalize'", takeID).Scan(&auditCount)
	if quantity != 7 || corrections != 1 || auditCount != 1 {
		t.Fatalf("stock take result quantity=%d corrections=%d audit=%d", quantity, corrections, auditCount)
	}
	locked := a.request(http.MethodPut, "/api/v1/stock-takes/"+takeID+"/items/"+lineID, map[string]any{"countedQuantity": 6, "reason": "late edit", "version": 2}, a.nurse)
	if locked.Code != http.StatusLocked {
		t.Fatalf("completed count edit status=%d, want 423", locked.Code)
	}
}

func TestStockTakeRejectsInventoryChangedAfterSnapshot(t *testing.T) {
	a := newTestApp(t)
	itemResponse := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "COUNT-LENS", "barcode": "", "category": "contact_lens", "name": "Counted Lens", "brand": "", "model": "", "attributes": map[string]any{}, "supplierId": "", "costMinor": 100, "salePriceMinor": 200, "currency": "HTG", "quantity": 4, "reorderLevel": 1, "trackStock": true}, a.nurse)
	item := decodeResponse[map[string]any](t, itemResponse)
	itemID := item["id"].(string)
	created := a.request(http.MethodPost, "/api/v1/stock-takes", map[string]any{"category": "contact_lens", "notes": ""}, a.doctor)
	takeID := decodeResponse[map[string]any](t, created)["id"].(string)
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/stock-takes/"+takeID, nil, a.nurse))
	lineID := detail["items"].([]any)[0].(map[string]any)["id"].(string)
	counted := a.request(http.MethodPut, "/api/v1/stock-takes/"+takeID+"/items/"+lineID, map[string]any{"countedQuantity": 4, "reason": "", "version": 1}, a.nurse)
	if counted.Code != http.StatusOK {
		t.Fatalf("count: %d %s", counted.Code, counted.Body.String())
	}
	movement := a.request(http.MethodPost, "/api/v1/inventory/"+itemID+"/movements", map[string]any{"type": "sale", "quantity": -1, "reason": "Concurrent sale", "version": 1}, a.nurse)
	if movement.Code != http.StatusCreated {
		t.Fatalf("concurrent stock movement: %d %s", movement.Code, movement.Body.String())
	}
	finalized := a.request(http.MethodPost, "/api/v1/stock-takes/"+takeID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if finalized.Code != http.StatusConflict || !bytes.Contains(finalized.Body.Bytes(), []byte("STOCK_CHANGED_DURING_COUNT")) {
		t.Fatalf("changed stock finalize status=%d, want 409: %s", finalized.Code, finalized.Body.String())
	}
}

func TestAppointmentRangeRescheduleAndConcurrency(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Calendar", "Patient")
	created := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": patient.ID, "practitionerId": "", "startsAt": "2026-09-08T14:00:00Z", "durationMinutes": 30, "type": "eye_exam", "reason": "Annual exam", "notes": ""}, a.nurse)
	if created.Code != http.StatusCreated {
		t.Fatalf("appointment create: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)
	rangeResponse := a.request(http.MethodGet, "/api/v1/appointments?from=2026-09-07T00:00:00Z&to=2026-09-14T00:00:00Z", nil, a.nurse)
	if rangeResponse.Code != http.StatusOK || !bytes.Contains(rangeResponse.Body.Bytes(), []byte(id)) {
		t.Fatalf("appointment range: %d %s", rangeResponse.Code, rangeResponse.Body.String())
	}
	updatedBody := map[string]any{"patientId": patient.ID, "practitionerId": "", "startsAt": "2026-09-09T15:30:00Z", "durationMinutes": 45, "type": "follow_up", "reason": "Rescheduled", "notes": "", "version": 1}
	updated := a.request(http.MethodPut, "/api/v1/appointments/"+id, updatedBody, a.nurse)
	if updated.Code != http.StatusOK {
		t.Fatalf("appointment reschedule: %d %s", updated.Code, updated.Body.String())
	}
	stale := a.request(http.MethodPut, "/api/v1/appointments/"+id, updatedBody, a.doctor)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale appointment reschedule=%d, want 409: %s", stale.Code, stale.Body.String())
	}
}

func TestAppointmentConflictWithoutPractitioner(t *testing.T) {
	a := newTestApp(t)
	first := a.createPatient(a.nurse, "First", "Patient")
	second := a.createPatient(a.nurse, "Second", "Patient")
	created := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": first.ID, "practitionerId": "", "startsAt": "2026-10-01T09:00:00Z", "durationMinutes": 30, "type": "eye_exam", "reason": "Exam", "notes": ""}, a.nurse)
	if created.Code != http.StatusCreated {
		t.Fatalf("first appointment create: %d %s", created.Code, created.Body.String())
	}
	overlapping := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": second.ID, "practitionerId": "", "startsAt": "2026-10-01T09:00:00Z", "durationMinutes": 30, "type": "eye_exam", "reason": "Exam", "notes": ""}, a.nurse)
	if overlapping.Code != http.StatusConflict || !bytes.Contains(overlapping.Body.Bytes(), []byte("APPOINTMENT_CONFLICT")) {
		t.Fatalf("expected 409 APPOINTMENT_CONFLICT for identical unassigned slot, got %d %s", overlapping.Code, overlapping.Body.String())
	}
	nonOverlapping := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{"patientId": second.ID, "practitionerId": "", "startsAt": "2026-10-01T10:00:00Z", "durationMinutes": 30, "type": "eye_exam", "reason": "Exam", "notes": ""}, a.nurse)
	if nonOverlapping.Code != http.StatusCreated {
		t.Fatalf("non-overlapping appointment create: %d %s", nonOverlapping.Code, nonOverlapping.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)
	rescheduleIntoConflict := a.request(http.MethodPut, "/api/v1/appointments/"+id, map[string]any{"patientId": first.ID, "practitionerId": "", "startsAt": "2026-10-01T10:15:00Z", "durationMinutes": 30, "type": "eye_exam", "reason": "Exam", "notes": "", "version": 1}, a.nurse)
	if rescheduleIntoConflict.Code != http.StatusConflict {
		t.Fatalf("expected 409 rescheduling into an occupied unassigned slot, got %d %s", rescheduleIntoConflict.Code, rescheduleIntoConflict.Body.String())
	}
}

func TestUserPasswordResetInvalidatesSessions(t *testing.T) {
	a := newTestApp(t)
	users := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/users", nil, a.doctor))["items"].([]any)
	var nurseID string
	for _, raw := range users {
		item := raw.(map[string]any)
		if item["username"] == "nurse.dev" {
			nurseID = item["id"].(string)
		}
	}
	if nurseID == "" {
		t.Fatal("development nurse not found")
	}
	reset := a.request(http.MethodPost, "/api/v1/users/"+nurseID+"/password", map[string]any{"password": "A-New-Secure-Nurse-Password-2026"}, a.doctor)
	if reset.Code != http.StatusOK {
		t.Fatalf("password reset: %d %s", reset.Code, reset.Body.String())
	}
	invalidated := a.request(http.MethodGet, "/api/v1/patients", nil, a.nurse)
	if invalidated.Code != http.StatusUnauthorized {
		t.Fatalf("old nurse session status=%d, want 401", invalidated.Code)
	}
	newCookie := a.login("nurse.dev", "A-New-Secure-Nurse-Password-2026")
	if response := a.request(http.MethodGet, "/api/v1/patients", nil, newCookie); response.Code != http.StatusOK {
		t.Fatalf("new nurse password login failed: %d", response.Code)
	}
}

func TestExternalBackupDestinationIsValidatedAndUsed(t *testing.T) {
	a := newTestApp(t)
	destination := filepath.Join(a.t.TempDir(), "external-backups")
	validated := a.request(http.MethodPost, "/api/v1/backups/validate-destination", map[string]any{"directory": destination}, a.doctor)
	if validated.Code != http.StatusOK {
		t.Fatalf("validate destination: %d %s", validated.Code, validated.Body.String())
	}
	settings := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/settings", nil, a.doctor))
	version := int(settings["versions"].(map[string]any)["backup"].(float64))
	update := a.request(http.MethodPut, "/api/v1/settings/backup", map[string]any{"value": map[string]any{"intervalHours": 4, "retentionDays": 30, "directory": destination}, "version": version}, a.doctor)
	if update.Code != http.StatusOK {
		t.Fatalf("save destination: %d %s", update.Code, update.Body.String())
	}
	backupResponse := a.request(http.MethodPost, "/api/v1/backups", map[string]any{}, a.doctor)
	if backupResponse.Code != http.StatusCreated {
		t.Fatalf("external backup: %d %s", backupResponse.Code, backupResponse.Body.String())
	}
	path := decodeResponse[map[string]any](t, backupResponse)["path"].(string)
	relative, err := filepath.Rel(destination, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		t.Fatalf("backup path %q is not under %q", path, destination)
	}
}

func TestClinicLogoUploadPersistsMetadataAndFile(t *testing.T) {
	a := newTestApp(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("logo", "clinic.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0})
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/branding/logo", &body)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(a.doctor)
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("logo upload: %d %s", response.Code, response.Body.String())
	}
	logo := a.request(http.MethodGet, "/api/v1/branding/logo", nil, a.doctor)
	if logo.Code != http.StatusOK || logo.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("logo load: %d %s", logo.Code, logo.Body.String())
	}
	publicLogo := a.request(http.MethodGet, "/api/v1/public/branding/logo", nil, nil)
	if publicLogo.Code != http.StatusOK || publicLogo.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("public logo load: %d %s", publicLogo.Code, publicLogo.Body.String())
	}
	var metadata int
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM branding_assets WHERE key='clinic_logo'").Scan(&metadata)
	if metadata != 1 {
		t.Fatalf("logo metadata count=%d", metadata)
	}
}

func TestPublicDisplayIsDisabledByDefaultAndPrivacyFiltered(t *testing.T) {
	a := newTestApp(t)
	if response := a.request(http.MethodGet, "/api/v1/public/display", nil, nil); response.Code != http.StatusNotFound {
		t.Fatalf("disabled public display status=%d, want 404", response.Code)
	}
	settings := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/settings", nil, a.doctor))
	version := int(settings["versions"].(map[string]any)["public_display"].(float64))
	configuration := map[string]any{"enabled": true, "privacyMode": "ticket_only", "showAppointments": true, "announcement": "Please watch for your number."}
	if response := a.request(http.MethodPut, "/api/v1/settings/public_display", map[string]any{"value": configuration, "version": version}, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse public display update status=%d, want 403", response.Code)
	}
	if response := a.request(http.MethodPut, "/api/v1/settings/public_display", map[string]any{"value": configuration, "version": version}, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("enable public display: %d %s", response.Code, response.Body.String())
	}
	patient := a.createPatient(a.nurse, "PrivateName", "PrivateSurname")
	if response := a.request(http.MethodPost, "/api/v1/queue/check-in", map[string]any{"patientId": patient.ID, "appointmentId": "", "assignedDoctorId": "", "priority": 0}, a.nurse); response.Code != http.StatusCreated {
		t.Fatalf("queue check in: %d %s", response.Code, response.Body.String())
	}
	display := a.request(http.MethodGet, "/api/v1/public/display", nil, nil)
	if display.Code != http.StatusOK {
		t.Fatalf("public display: %d %s", display.Code, display.Body.String())
	}
	if bytes.Contains(display.Body.Bytes(), []byte("PrivateName")) || bytes.Contains(display.Body.Bytes(), []byte("PrivateSurname")) || bytes.Contains(display.Body.Bytes(), []byte(patient.MedicalRecordNumber)) {
		t.Fatalf("public display leaked patient identity: %s", display.Body.String())
	}
	decoded := decodeResponse[map[string]any](t, display)
	queue := decoded["queue"].([]any)
	if len(queue) != 1 || queue[0].(map[string]any)["patientLabel"] == "" {
		t.Fatalf("public queue response invalid: %v", queue)
	}
}

func TestAppearanceSettingsAreValidated(t *testing.T) {
	a := newTestApp(t)
	settings := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/settings", nil, a.doctor))
	version := int(settings["versions"].(map[string]any)["appearance"].(float64))
	invalid := map[string]any{"baseColor": "unknown", "accentColor": "blue", "mode": "light", "radius": "medium"}
	if response := a.request(http.MethodPut, "/api/v1/settings/appearance", map[string]any{"value": invalid, "version": version}, a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid appearance status=%d, want 422", response.Code)
	}
	valid := map[string]any{"baseColor": "stone", "accentColor": "blue", "mode": "system", "radius": "large"}
	if response := a.request(http.MethodPut, "/api/v1/settings/appearance", map[string]any{"value": valid, "version": version}, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("valid appearance: %d %s", response.Code, response.Body.String())
	}
}

func TestLocalizationSettingsArePublicValidatedAndDoctorOnly(t *testing.T) {
	a := newTestApp(t)
	initial := decodeResponse[map[string]string](t, a.request(http.MethodGet, "/api/v1/public/localization", nil, nil))
	if initial["language"] != "en" {
		t.Fatalf("initial language=%q, want en", initial["language"])
	}
	settings := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/settings", nil, a.doctor))
	version := int(settings["versions"].(map[string]any)["localization"].(float64))
	if response := a.request(http.MethodPut, "/api/v1/settings/localization", map[string]any{"value": map[string]string{"language": "fr"}, "version": version}, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse localization update status=%d, want 403", response.Code)
	}
	if response := a.request(http.MethodPut, "/api/v1/settings/localization", map[string]any{"value": map[string]string{"language": "xx"}, "version": version}, a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid localization status=%d, want 422", response.Code)
	}
	if response := a.request(http.MethodPut, "/api/v1/settings/localization", map[string]any{"value": map[string]string{"language": "ht"}, "version": version}, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("valid localization: %d %s", response.Code, response.Body.String())
	}
	updated := decodeResponse[map[string]string](t, a.request(http.MethodGet, "/api/v1/public/localization", nil, nil))
	if updated["language"] != "ht" {
		t.Fatalf("updated language=%q, want ht", updated["language"])
	}
}

func TestBackupCreatesVerifiedSQLiteSnapshot(t *testing.T) {
	a := newTestApp(t)
	response := a.request(http.MethodPost, "/api/v1/backups", map[string]any{}, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("backup: %d %s", response.Code, response.Body.String())
	}
	record := decodeResponse[map[string]any](t, response)
	if record["verified"] != true {
		t.Fatalf("backup not verified: %v", record)
	}
	path := record["path"].(string)
	if stat, err := os.Stat(path); err != nil || stat.Size() == 0 {
		t.Fatalf("backup file invalid: %v", err)
	}
}

func TestManualInsuranceClaimFlowAndRBAC(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Assured", "Patient")
	p := a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "Assurance Haiti", "contactName": "Claims Desk", "phone": "1234", "email": "", "address": "Port-au-Prince"}, a.doctor)
	if p.Code != http.StatusCreated {
		t.Fatalf("payer: %d %s", p.Code, p.Body.String())
	}
	payerID := decodeResponse[map[string]any](t, p)["id"].(string)
	c := a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{"patientId": patient.ID, "payerId": payerID, "invoiceId": "", "authorization": "AUTH-42", "claimAmountMinor": 10000, "patientPortionMinor": 2000, "payerPortionMinor": 8000}, a.nurse)
	if c.Code != http.StatusCreated {
		t.Fatalf("claim: %d %s", c.Code, c.Body.String())
	}
	claimID := decodeResponse[map[string]any](t, c)["id"].(string)
	forbidden := a.request(http.MethodPatch, "/api/v1/insurance/claims/"+claimID+"/status", map[string]any{"status": "submitted", "version": 1}, a.nurse)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse status=%d", forbidden.Code)
	}
	submitted := a.request(http.MethodPatch, "/api/v1/insurance/claims/"+claimID+"/status", map[string]any{"status": "submitted", "version": 1}, a.doctor)
	if submitted.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", submitted.Code, submitted.Body.String())
	}
	approved := a.request(http.MethodPatch, "/api/v1/insurance/claims/"+claimID+"/status", map[string]any{"status": "approved", "version": 2}, a.doctor)
	if approved.Code != http.StatusOK { t.Fatalf("approve: %d %s", approved.Code, approved.Body.String()) }
	paid := a.request(http.MethodPost, "/api/v1/insurance/claims/"+claimID+"/payments", map[string]any{"amountMinor": 3000, "paymentDate": "2026-09-06", "reference": "CHK-1", "notes": ""}, a.doctor)
	if paid.Code != http.StatusCreated {
		t.Fatalf("claim payment: %d %s", paid.Code, paid.Body.String())
	}
	list := a.request(http.MethodGet, "/api/v1/insurance/claims", nil, a.nurse)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(`"outstandingMinor":5000`)) {
		t.Fatalf("claim list: %d %s", list.Code, list.Body.String())
	}
}

func TestRefundCreatesCreditNoteAndRestocksAtomically(t *testing.T) {
	a := newTestApp(t)
	item := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "RET-1", "barcode": "", "category": "frame", "name": "Return Frame", "brand": "", "model": "", "attributes": map[string]any{}, "supplierId": "", "costMinor": 1000, "salePriceMinor": 5000, "currency": "HTG", "quantity": 2, "reorderLevel": 0, "trackStock": true}, a.nurse)
	itemID := decodeResponse[map[string]any](t, item)["id"].(string)
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{"invoice": map[string]any{"patientId": "", "currency": "HTG", "exchangeRate": "1", "discountMinor": 0, "taxMinor": 0, "dueAt": "", "notes": "", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Return Frame", "quantity": 1, "unitPriceMinor": 5000, "discountMinor": 0, "taxMinor": 0}}}, "payment": map[string]any{"paymentMethodId": "pm_cash", "registerSessionId": "", "amountMinor": 5000, "currency": "HTG", "exchangeRate": "1", "reference": "", "notes": ""}}, a.nurse)
	res := decodeResponse[map[string]any](t, checkout)
	invoiceID := res["invoiceId"].(string)
	paymentID := res["paymentId"].(string)
	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/invoices/"+invoiceID, nil, a.doctor))
	lineID := detail["items"].([]any)[0].(map[string]any)["id"].(string)
	refund := a.request(http.MethodPost, "/api/v1/payments/"+paymentID+"/refunds", map[string]any{"amountMinor": 5000, "reason": "Frame returned", "restockItemIds": []string{lineID}}, a.doctor)
	if refund.Code != http.StatusCreated || !bytes.Contains(refund.Body.Bytes(), []byte("CRN-")) {
		t.Fatalf("refund: %d %s", refund.Code, refund.Body.String())
	}
	var quantity, credits, returns int
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT quantity FROM inventory_items WHERE id=?", itemID).Scan(&quantity)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM credit_notes WHERE invoice_id=?", invoiceID).Scan(&credits)
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM stock_movements WHERE item_id=? AND movement_type='return'", itemID).Scan(&returns)
	if quantity != 2 || credits != 1 || returns != 1 {
		t.Fatalf("quantity=%d credits=%d returns=%d", quantity, credits, returns)
	}
}

func TestFinanceAndDashboardAccountForRefunds(t *testing.T) {
	a := newTestApp(t)
	item := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": "REF-1", "barcode": "", "category": "frame", "name": "Refund Frame", "brand": "", "model": "", "attributes": map[string]any{}, "supplierId": "", "costMinor": 1000, "salePriceMinor": 10000, "currency": "HTG", "quantity": 2, "reorderLevel": 0, "trackStock": true}, a.nurse)
	itemID := decodeResponse[map[string]any](t, item)["id"].(string)
	checkout := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{"invoice": map[string]any{"patientId": "", "currency": "HTG", "exchangeRate": "1", "discountMinor": 0, "taxMinor": 0, "dueAt": "", "notes": "", "items": []map[string]any{{"inventoryItemId": itemID, "description": "Refund Frame", "quantity": 1, "unitPriceMinor": 10000, "discountMinor": 0, "taxMinor": 0}}}, "payment": map[string]any{"paymentMethodId": "pm_cash", "registerSessionId": "", "amountMinor": 8000, "currency": "HTG", "exchangeRate": "1", "reference": "", "notes": ""}}, a.nurse)
	res := decodeResponse[map[string]any](t, checkout)
	invoiceID, paymentID := res["invoiceId"].(string), res["paymentId"].(string)
	if refund := a.request(http.MethodPost, "/api/v1/payments/"+paymentID+"/refunds", map[string]any{"amountMinor": 3000, "reason": "Pricing correction", "restockItemIds": []string{}}, a.doctor); refund.Code != http.StatusCreated {
		t.Fatalf("refund: %d %s", refund.Code, refund.Body.String())
	}
	invoice := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/invoices/"+invoiceID, nil, a.doctor))
	if invoice["status"] != "partially_paid" || invoice["balanceMinor"].(float64) != 5000 {
		t.Fatalf("invoice after refund: %v", invoice)
	}
	summary := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/finance/summary", nil, a.doctor))
	if summary["refundsMinor"].(float64) != 3000 {
		t.Fatalf("finance summary refundsMinor=%v, want 3000", summary["refundsMinor"])
	}
	if summary["paymentsReceivedMinor"].(float64) != 5000 {
		t.Fatalf("finance summary paymentsReceivedMinor=%v, want 5000 (net of refund)", summary["paymentsReceivedMinor"])
	}
	dashboard := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/dashboard", nil, a.doctor))
	finance := dashboard["finance"].(map[string]any)
	if finance["outstandingMinor"].(float64) != 5000 {
		t.Fatalf("dashboard outstandingMinor=%v, want 5000 (must add back the refund)", finance["outstandingMinor"])
	}
	if finance["monthRefundsMinor"].(float64) != 3000 {
		t.Fatalf("dashboard monthRefundsMinor=%v, want 3000", finance["monthRefundsMinor"])
	}
}

func TestClinicalMacrosCRUD(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/macros", map[string]any{"label": "Normal exam", "category": "assessment", "body": "No abnormalities detected OU."}, a.nurse)
	if created.Code != http.StatusCreated {
		t.Fatalf("macro create: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)
	if bad := a.request(http.MethodPost, "/api/v1/macros", map[string]any{"label": "x", "category": "not_a_category", "body": "y"}, a.doctor); bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid category status=%d, want 422", bad.Code)
	}
	list := a.request(http.MethodGet, "/api/v1/macros?category=assessment", nil, a.doctor)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte("Normal exam")) {
		t.Fatalf("macro list: %d %s", list.Code, list.Body.String())
	}
	updated := a.request(http.MethodPut, "/api/v1/macros/"+id, map[string]any{"label": "Normal exam OU", "category": "assessment", "body": "No abnormalities detected OU.", "version": 1}, a.doctor)
	if updated.Code != http.StatusOK {
		t.Fatalf("macro update: %d %s", updated.Code, updated.Body.String())
	}
	if deleted := a.request(http.MethodDelete, "/api/v1/macros/"+id+"?version=2", nil, a.doctor); deleted.Code != http.StatusNoContent {
		t.Fatalf("macro delete: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestEyeDiagramRBACAndLocking(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Eye", "Diagram")
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Red eye", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("encounter create: %d %s", created.Code, created.Body.String())
	}
	encounterID := decodeResponse[map[string]any](t, created)["id"].(string)
	forbidden := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/eye-diagrams/anterior/OD", map[string]any{"annotations": []map[string]any{{"x": 50, "y": 50, "shape": "dot", "color": "#c00", "label": "nasal pterygium", "structure": "conjunctiva"}}, "notes": "Pterygium noted", "version": 0}, a.nurse)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse eye diagram save status=%d, want 403", forbidden.Code)
	}
	saved := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/eye-diagrams/anterior/OD", map[string]any{"annotations": []map[string]any{{"x": 50, "y": 50, "shape": "dot", "color": "#c00", "label": "nasal pterygium", "structure": "conjunctiva"}}, "notes": "Pterygium noted", "version": 0}, a.doctor)
	if saved.Code != http.StatusCreated {
		t.Fatalf("doctor eye diagram save: %d %s", saved.Code, saved.Body.String())
	}
	fetched := a.request(http.MethodGet, "/api/v1/encounters/"+encounterID+"/eye-diagrams", nil, a.nurse)
	if fetched.Code != http.StatusOK || !bytes.Contains(fetched.Body.Bytes(), []byte("nasal pterygium")) {
		t.Fatalf("eye diagram get: %d %s", fetched.Code, fetched.Body.String())
	}
	diagnosis := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/diagnoses", map[string]any{"diagnosis": "Pterygium of right eye", "code": "H11.001", "laterality": "OD", "notes": "", "primary": true}, a.doctor)
	if diagnosis.Code != http.StatusCreated {
		t.Fatalf("diagnosis create: %d %s", diagnosis.Code, diagnosis.Body.String())
	}
	finalized := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if finalized.Code != http.StatusOK {
		t.Fatalf("finalize: %d %s", finalized.Code, finalized.Body.String())
	}
	locked := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/eye-diagrams/anterior/OD", map[string]any{"annotations": []map[string]any{}, "notes": "edit after lock", "version": 1}, a.doctor)
	if locked.Code != http.StatusLocked {
		t.Fatalf("locked eye diagram update status=%d, want 423", locked.Code)
	}
}

func TestCodeSearchAndSuperbillGeneration(t *testing.T) {
	a := newTestApp(t)
	icd := a.request(http.MethodGet, "/api/v1/codes/icd10?q=myopia", nil, a.nurse)
	if icd.Code != http.StatusOK || !bytes.Contains(icd.Body.Bytes(), []byte("H52.1")) {
		t.Fatalf("icd10 search: %d %s", icd.Code, icd.Body.String())
	}
	procedures := a.request(http.MethodGet, "/api/v1/codes/procedures?q=92004", nil, a.nurse)
	if procedures.Code != http.StatusOK || !bytes.Contains(procedures.Body.Bytes(), []byte("Comprehensive ophthalmological exam")) {
		t.Fatalf("procedure search: %d %s", procedures.Code, procedures.Body.String())
	}
	patient := a.createPatient(a.nurse, "Super", "Bill")
	encounterResponse := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Annual exam", "chiefComplaint": "Checkup", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	encounterID := decodeResponse[map[string]any](t, encounterResponse)["id"].(string)
	a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/diagnoses", map[string]any{"diagnosis": "Myopia, bilateral", "code": "H52.13", "laterality": "OU", "notes": "", "primary": true}, a.doctor)
	invoice := a.request(http.MethodPost, "/api/v1/invoices", map[string]any{"patientId": patient.ID, "currency": "HTG", "exchangeRate": "1", "discountMinor": 0, "taxMinor": 0, "dueAt": "", "notes": "", "items": []map[string]any{{"inventoryItemId": "", "description": "Comprehensive eye exam", "quantity": 1, "unitPriceMinor": 250000, "discountMinor": 0, "taxMinor": 0, "procedureCode": "92004"}}}, a.nurse)
	if invoice.Code != http.StatusCreated {
		t.Fatalf("invoice create: %d %s", invoice.Code, invoice.Body.String())
	}
	invoiceID := decodeResponse[map[string]any](t, invoice)["id"].(string)
	forbiddenLink := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/superbill/link-invoice", map[string]any{"invoiceId": invoiceID}, a.nurse)
	if forbiddenLink.Code != http.StatusForbidden {
		t.Fatalf("nurse link invoice status=%d, want 403", forbiddenLink.Code)
	}
	link := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/superbill/link-invoice", map[string]any{"invoiceId": invoiceID}, a.doctor)
	if link.Code != http.StatusOK {
		t.Fatalf("link invoice: %d %s", link.Code, link.Body.String())
	}
	superbill := a.request(http.MethodGet, "/api/v1/encounters/"+encounterID+"/superbill", nil, a.doctor)
	if superbill.Code != http.StatusOK {
		t.Fatalf("superbill get: %d %s", superbill.Code, superbill.Body.String())
	}
	body := superbill.Body.String()
	for _, want := range []string{"H52.13", "Myopia, bilateral", "92004", "Comprehensive eye exam", "Super Bill"} {
		if !strings.Contains(body, want) {
			t.Fatalf("superbill missing %q: %s", want, body)
		}
	}
}

func TestKioskSelfCheckIn(t *testing.T) {
	a := newTestApp(t)
	patient := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": "Kiosk", "middleName": "", "lastName": "Walker", "preferredName": "", "sex": "", "dateOfBirth": "", "phone": "509-1234-5678", "alternatePhone": "", "email": "", "address": "", "city": "", "occupation": "", "employer": "", "preferredLanguage": "", "communicationPreference": "", "referralSource": "", "referringProvider": "", "notes": "", "tags": []string{},
	}, a.nurse)
	if patient.Code != http.StatusCreated {
		t.Fatalf("create patient: %d %s", patient.Code, patient.Body.String())
	}
	patientID := decodeResponse[map[string]any](t, patient)["id"].(string)

	wrongLastName := a.request(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "(509) 1234-5678", "lastName": "NotAMatch"}, nil)
	if wrongLastName.Code != http.StatusNotFound {
		t.Fatalf("wrong last name lookup status=%d, want 404", wrongLastName.Code)
	}

	lookup := a.request(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "(509) 1234-5678", "lastName": "walker"}, nil)
	if lookup.Code != http.StatusOK {
		t.Fatalf("kiosk lookup: %d %s", lookup.Code, lookup.Body.String())
	}
	found := decodeResponse[map[string]any](t, lookup)
	if found["patientId"] != patientID || found["firstName"] != "Kiosk" || found["lastInitial"] != "W." {
		t.Fatalf("kiosk lookup result mismatch: %v", found)
	}

	mismatchedCheckIn := a.request(http.MethodPost, "/api/v1/public/kiosk/checkin", map[string]any{"patientId": patientID, "phone": "509-0000-0000", "lastName": "Walker", "appointmentId": ""}, nil)
	if mismatchedCheckIn.Code != http.StatusForbidden {
		t.Fatalf("mismatched phone check-in status=%d, want 403", mismatchedCheckIn.Code)
	}

	checkIn := a.request(http.MethodPost, "/api/v1/public/kiosk/checkin", map[string]any{"patientId": patientID, "phone": "509-1234-5678", "lastName": "Walker", "appointmentId": ""}, nil)
	if checkIn.Code != http.StatusCreated {
		t.Fatalf("kiosk check-in: %d %s", checkIn.Code, checkIn.Body.String())
	}

	duplicate := a.request(http.MethodPost, "/api/v1/public/kiosk/checkin", map[string]any{"patientId": patientID, "phone": "509-1234-5678", "lastName": "Walker", "appointmentId": ""}, nil)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate kiosk check-in status=%d, want 409", duplicate.Code)
	}

	queue := a.request(http.MethodGet, "/api/v1/queue", nil, a.nurse)
	if queue.Code != http.StatusOK || !bytes.Contains(queue.Body.Bytes(), []byte(`"source":"kiosk"`)) {
		t.Fatalf("queue entry missing kiosk source: %d %s", queue.Code, queue.Body.String())
	}
}

func TestKioskLookupIsRateLimited(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < kioskLookupMaxAttempts; i++ {
		response := a.request(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "000", "lastName": "Nobody"}, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("attempt %d status=%d, want 404", i, response.Code)
		}
	}
	limited := a.request(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "000", "lastName": "Nobody"}, nil)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled lookup status=%d, want 429", limited.Code)
	}
}

func multipartAudioRequest(t *testing.T, path, mediaType, filename string, audioBytes []byte, extraFields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range extraFields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{`form-data; name="audio"; filename="` + filename + `"`}
	header["Content-Type"] = []string{mediaType}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

func (a *testApp) requestRaw(request *http.Request, cookie *http.Cookie) *httptest.ResponseRecorder {
	a.t.Helper()
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

func waitForTranscriptStatus(t *testing.T, a *testApp, recordingID string, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		var status string
		var raw string
		row := a.server.db.QueryRowContext(context.Background(), "SELECT transcript_status,COALESCE(transcript_text,'') FROM consultation_recordings WHERE id=?", recordingID)
		if err := row.Scan(&status, &raw); err == nil && status == want {
			return map[string]any{"transcriptStatus": status, "transcriptText": raw}
		}
		last = map[string]any{"transcriptStatus": status}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("recording %s did not reach status %q in time, last=%v", recordingID, want, last)
	return nil
}

func TestConsultationRecordingRequiresConsentAndLocksAfterFinalize(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Recorded", "Patient")
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Cough", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	encounterID := decodeResponse[map[string]any](t, created)["id"].(string)

	withoutConsent := multipartAudioRequest(t, "/api/v1/encounters/"+encounterID+"/recordings", "audio/wav", "rec.wav", []byte("RIFF0000WAVEfmt "), map[string]string{"durationSeconds": "5"})
	if response := a.requestRaw(withoutConsent, a.nurse); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("recording without consent status=%d, want 422: %s", response.Code, response.Body.String())
	}

	unsupported := multipartAudioRequest(t, "/api/v1/encounters/"+encounterID+"/recordings", "text/plain", "rec.txt", []byte("not audio"), map[string]string{"consentConfirmed": "true", "durationSeconds": "5"})
	if response := a.requestRaw(unsupported, a.nurse); response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported media status=%d, want 415: %s", response.Code, response.Body.String())
	}

	diagnosis := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/diagnoses", map[string]any{"diagnosis": "Acute bronchitis", "code": "", "laterality": "", "notes": "", "primary": true}, a.doctor)
	if diagnosis.Code != http.StatusCreated {
		t.Fatalf("diagnosis: %d %s", diagnosis.Code, diagnosis.Body.String())
	}
	finalized := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if finalized.Code != http.StatusOK {
		t.Fatalf("finalize: %d %s", finalized.Code, finalized.Body.String())
	}
	afterLock := multipartAudioRequest(t, "/api/v1/encounters/"+encounterID+"/recordings", "audio/wav", "rec.wav", []byte("RIFF0000WAVEfmt "), map[string]string{"consentConfirmed": "true", "durationSeconds": "5"})
	if response := a.requestRaw(afterLock, a.doctor); response.Code != http.StatusLocked {
		t.Fatalf("recording after finalize status=%d, want 423: %s", response.Code, response.Body.String())
	}
}

func TestConsultationRecordingUnavailableWithoutTranscriptionConfigured(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Untranscribed", "Patient")
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Cough", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	encounterID := decodeResponse[map[string]any](t, created)["id"].(string)

	req := multipartAudioRequest(t, "/api/v1/encounters/"+encounterID+"/recordings", "audio/wav", "rec.wav", []byte("RIFF0000WAVEfmt "), map[string]string{"consentConfirmed": "true", "durationSeconds": "12"})
	response := a.requestRaw(req, a.nurse)
	if response.Code != http.StatusCreated {
		t.Fatalf("recording create: %d %s", response.Code, response.Body.String())
	}
	recordingID := decodeResponse[map[string]any](t, response)["id"].(string)
	result := waitForTranscriptStatus(t, a, recordingID, "unavailable")
	if result["transcriptStatus"] != "unavailable" {
		t.Fatalf("expected unavailable status, got %v", result)
	}
}

func TestConsultationRecordingTranscriptionPipelineAndAudioPlayback(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Transcribed", "Patient")
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Cough", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	encounterID := decodeResponse[map[string]any](t, created)["id"].(string)

	settings := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/settings", nil, a.doctor))
	version := int(settings["versions"].(map[string]any)["clinical"].(float64))
	configure := a.request(http.MethodPut, "/api/v1/settings/clinical", map[string]any{"value": map[string]any{"appointmentDuration": 30, "enabledSections": []string{}, "transcriptionEnabled": true, "transcriptionCommand": "echo mocked-transcript"}, "version": version}, a.doctor)
	if configure.Code != http.StatusOK {
		t.Fatalf("configure transcription: %d %s", configure.Code, configure.Body.String())
	}

	req := multipartAudioRequest(t, "/api/v1/encounters/"+encounterID+"/recordings", "audio/wav", "rec.wav", []byte("RIFF0000WAVEfmt "), map[string]string{"consentConfirmed": "true", "durationSeconds": "8"})
	response := a.requestRaw(req, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("recording create: %d %s", response.Code, response.Body.String())
	}
	recordingID := decodeResponse[map[string]any](t, response)["id"].(string)

	result := waitForTranscriptStatus(t, a, recordingID, "done")
	if !strings.Contains(result["transcriptText"].(string), "mocked-transcript") {
		t.Fatalf("transcript text=%v, want to contain mocked-transcript", result["transcriptText"])
	}

	list := a.request(http.MethodGet, "/api/v1/encounters/"+encounterID+"/recordings", nil, a.nurse)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte("mocked-transcript")) {
		t.Fatalf("recording list: %d %s", list.Code, list.Body.String())
	}

	audio := a.request(http.MethodGet, "/api/v1/recordings/"+recordingID+"/audio", nil, a.nurse)
	if audio.Code != http.StatusOK || audio.Header().Get("Content-Type") != "audio/wav" || audio.Body.Len() == 0 {
		t.Fatalf("recording audio playback: %d content-type=%s len=%d", audio.Code, audio.Header().Get("Content-Type"), audio.Body.Len())
	}

	corrected := a.request(http.MethodPut, "/api/v1/recordings/"+recordingID+"/transcript", map[string]any{"text": "Patient reports a persistent dry cough for two weeks."}, a.doctor)
	if corrected.Code != http.StatusOK {
		t.Fatalf("transcript correction: %d %s", corrected.Code, corrected.Body.String())
	}
	var edited bool
	var text string
	_ = a.server.db.QueryRowContext(context.Background(), "SELECT transcript_edited,transcript_text FROM consultation_recordings WHERE id=?", recordingID).Scan(&edited, &text)
	if !edited || text != "Patient reports a persistent dry cough for two weeks." {
		t.Fatalf("transcript correction not persisted: edited=%v text=%q", edited, text)
	}

	forbiddenDelete := a.request(http.MethodDelete, "/api/v1/recordings/"+recordingID, nil, a.nurse)
	if forbiddenDelete.Code != http.StatusForbidden {
		t.Fatalf("nurse delete status=%d, want 403", forbiddenDelete.Code)
	}
	deleted := a.request(http.MethodDelete, "/api/v1/recordings/"+recordingID, nil, a.doctor)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("doctor delete: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestClinicalValueConversions(t *testing.T) {
	acuityCases := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"20/20", 0.00, true},
		{"6/6", 0.00, true},
		{"10/10", 0.00, true},
		{"20/40", 0.30, true},
		{"6/12", 0.30, true},
		{"5/10", 0.30, true},
		{"20/200", 1.00, true},
		{"0.5", 0.30, true},
		{"20/40-2", 0.30, true},
		{"CF", 1.90, true},
		{"hm", 2.30, true},
		{"LP", 2.70, true},
		{"NLP", 3.00, true},
		{"", 0, false},
		{"illegible", 0, false},
	}
	for _, testCase := range acuityCases {
		got, ok := snellenToLogMAR(testCase.input)
		if ok != testCase.ok || (ok && math.Abs(got-testCase.want) > 0.005) {
			t.Fatalf("snellenToLogMAR(%q)=%v,%v want %v,%v", testCase.input, got, ok, testCase.want, testCase.ok)
		}
	}
	if value, ok := parseClinicalNumber("−2,25"); !ok || value != -2.25 {
		t.Fatalf("parseClinicalNumber unicode minus and comma = %v,%v", value, ok)
	}
	if got := sphericalEquivalent(-2.00, -1.00, true); got != -2.50 {
		t.Fatalf("sphericalEquivalent = %v, want -2.50", got)
	}
	if got := sphericalEquivalent(-2.00, 0, false); got != -2.00 {
		t.Fatalf("sphericalEquivalent without cylinder = %v, want -2.00", got)
	}
}

func (a *testApp) recordVisit(t *testing.T, patientID string, acuity, iop, pachymetry, refraction map[string]string) string {
	t.Helper()
	created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patientID, "appointmentId": "", "visitReason": "Follow-up", "chiefComplaint": "", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("encounter create: %d %s", created.Code, created.Body.String())
	}
	encounterID := decodeResponse[map[string]any](t, created)["id"].(string)
	pretest := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/pretest", map[string]any{
		"chiefComplaint": "", "vitals": map[string]string{}, "visualAcuity": acuity, "autorefraction": map[string]string{},
		"keratometry": map[string]string{}, "iop": iop, "pupils": "", "eom": "", "coverTest": "", "confrontationFields": "",
		"colorVision": "", "stereopsis": "", "pachymetry": pachymetry, "lensometry": map[string]string{}, "complete": false, "version": 1,
	}, a.nurse)
	if pretest.Code != http.StatusOK {
		t.Fatalf("pretest save: %d %s", pretest.Code, pretest.Body.String())
	}
	section := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/sections/subjective_refraction", map[string]any{"data": refraction, "version": 0}, a.doctor)
	if section.Code != http.StatusCreated && section.Code != http.StatusOK {
		t.Fatalf("refraction section save: %d %s", section.Code, section.Body.String())
	}
	return encounterID
}

func TestClinicalTrendsAndVisitDelta(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Trend", "Patient")

	first := a.recordVisit(t, patient.ID,
		map[string]string{"odbestcorrectedva": "20/20", "osbestcorrectedva": "20/20"},
		map[string]string{"odiop": "16", "osiop": "15"},
		map[string]string{"odthickness": "500", "osthickness": "545"},
		map[string]string{"odsphere": "-2.00", "odcylinder": "-1.00", "ossphere": "-1.50", "oscylinder": "0"})
	// Back-date the first visit so the progression rate covers a real interval instead of
	// two encounters created seconds apart in the test.
	twoYearsAgo := time.Now().UTC().AddDate(-2, 0, 0).Format(time.RFC3339Nano)
	if _, err := a.server.db.ExecContext(context.Background(), "UPDATE encounters SET created_at=? WHERE id=?", twoYearsAgo, first); err != nil {
		t.Fatal(err)
	}

	second := a.recordVisit(t, patient.ID,
		map[string]string{"odbestcorrectedva": "20/60", "osbestcorrectedva": "20/20"},
		map[string]string{"odiop": "26", "osiop": "17"},
		map[string]string{"odthickness": "500", "osthickness": "545"},
		map[string]string{"odsphere": "-4.00", "odcylinder": "-1.00", "ossphere": "-1.50", "oscylinder": "0"})

	trends := a.request(http.MethodGet, "/api/v1/patients/"+patient.ID+"/clinical-trends", nil, a.nurse)
	if trends.Code != http.StatusOK {
		t.Fatalf("clinical trends: %d %s", trends.Code, trends.Body.String())
	}
	payload := decodeResponse[map[string]any](t, trends)
	points := payload["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("expected 2 trend points, got %d: %s", len(points), trends.Body.String())
	}
	latest := points[1].(map[string]any)
	refraction := latest["refraction"].(map[string]any)
	if refraction["source"] != "subjective" || refraction["odSphericalEquivalent"].(float64) != -4.50 {
		t.Fatalf("latest refraction = %v", refraction)
	}
	if acuity := latest["visualAcuity"].(map[string]any); math.Abs(acuity["od"].(float64)-0.48) > 0.005 {
		t.Fatalf("latest OD acuity = %v, want 0.48 logMAR", acuity["od"])
	}

	body := trends.Body.String()
	for _, want := range []string{"Rapid myopic shift", "Visual acuity dropped", "Elevated intraocular pressure", "Thin cornea", "Asymmetric intraocular pressure"} {
		if !strings.Contains(body, want) {
			t.Fatalf("alert %q missing from trends: %s", want, body)
		}
	}
	if summary := payload["summary"].(map[string]any); summary["refractionRateOD"].(float64) != -1.00 {
		t.Fatalf("OD progression rate = %v, want -1.00 D/year", summary["refractionRateOD"])
	}

	delta := a.request(http.MethodGet, "/api/v1/encounters/"+second+"/delta", nil, a.nurse)
	if delta.Code != http.StatusOK {
		t.Fatalf("encounter delta: %d %s", delta.Code, delta.Body.String())
	}
	deltaPayload := decodeResponse[map[string]any](t, delta)
	if deltaPayload["hasPrevious"] != true {
		t.Fatalf("expected a previous visit: %s", delta.Body.String())
	}
	deltaBody := delta.Body.String()
	for _, want := range []string{"Visual acuity", "Intraocular pressure", "Refraction (spherical equivalent)", "+10 mmHg", "-2.00 D"} {
		if !strings.Contains(deltaBody, want) {
			t.Fatalf("delta missing %q: %s", want, deltaBody)
		}
	}

	firstDelta := a.request(http.MethodGet, "/api/v1/encounters/"+first+"/delta", nil, a.nurse)
	if firstDelta.Code != http.StatusOK || !bytes.Contains(firstDelta.Body.Bytes(), []byte(`"hasPrevious":false`)) {
		t.Fatalf("first visit should have no previous consultation: %d %s", firstDelta.Code, firstDelta.Body.String())
	}
}

func TestRestoreReplacesDatabaseAndPreservesSafetySnapshot(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Restore", "Patient")
	backupResponse := a.request(http.MethodPost, "/api/v1/backups", map[string]any{}, a.doctor)
	if backupResponse.Code != http.StatusCreated {
		t.Fatalf("backup: %d %s", backupResponse.Code, backupResponse.Body.String())
	}
	backupID := decodeResponse[map[string]any](t, backupResponse)["id"].(string)
	update := a.request(http.MethodPut, "/api/v1/patients/"+patient.ID, patientUpdateBody(patient, "change after snapshot"), a.doctor)
	if update.Code != http.StatusOK {
		t.Fatalf("update after backup: %d %s", update.Code, update.Body.String())
	}
	restore := a.request(http.MethodPost, "/api/v1/backups/restore", map[string]any{"backupId": backupID}, a.doctor)
	if restore.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", restore.Code, restore.Body.String())
	}
	reloaded := a.request(http.MethodGet, "/api/v1/patients/"+patient.ID, nil, a.doctor)
	if reloaded.Code != http.StatusOK || bytes.Contains(reloaded.Body.Bytes(), []byte("change after snapshot")) {
		t.Fatalf("restored patient did not match snapshot: %d %s", reloaded.Code, reloaded.Body.String())
	}
	backups, err := filepath.Glob(filepath.Join(a.server.config.DataDir, "backups", "sentrymed-pre_restore-*.db"))
	if err != nil || len(backups) == 0 {
		t.Fatalf("pre-restore safety snapshot missing: %v", err)
	}
}

func TestClinicalChartTypesAndPreviousOverlay(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.nurse, "Chart", "Engine")
	newEncounter := func() string {
		created := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "appointmentId": "", "visitReason": "Exam", "chiefComplaint": "Field check", "hpi": "", "assessment": "", "treatmentPlan": "", "followUp": ""}, a.doctor)
		if created.Code != http.StatusCreated {
			t.Fatalf("encounter create: %d %s", created.Code, created.Body.String())
		}
		return decodeResponse[map[string]any](t, created)["id"].(string)
	}

	first := newEncounter()
	fieldMark := map[string]any{"x": 74, "y": 30, "shape": "cell", "color": "#dc2626", "label": "", "structure": "field", "cell": "superior-temporal", "grade": "absent"}
	if saved := a.request(http.MethodPut, "/api/v1/encounters/"+first+"/eye-diagrams/field/OD", map[string]any{"annotations": []map[string]any{fieldMark}, "notes": "Superior temporal loss", "version": 0}, a.doctor); saved.Code != http.StatusCreated {
		t.Fatalf("field chart save: %d %s", saved.Code, saved.Body.String())
	}

	second := newEncounter()
	if bad := a.request(http.MethodPut, "/api/v1/encounters/"+second+"/eye-diagrams/motility/OD", map[string]any{"annotations": []map[string]any{}, "notes": "", "version": 0}, a.doctor); bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("motility per-eye save status=%d, want 422", bad.Code)
	}
	if bad := a.request(http.MethodPut, "/api/v1/encounters/"+second+"/eye-diagrams/topography/OD", map[string]any{"annotations": []map[string]any{}, "notes": "", "version": 0}, a.doctor); bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown chart type status=%d, want 422", bad.Code)
	}
	motilityMark := map[string]any{"x": 20, "y": 20, "shape": "cell", "color": "#d97706", "label": "left eye lags", "structure": "motility", "cell": "up-right", "grade": "-2"}
	if saved := a.request(http.MethodPut, "/api/v1/encounters/"+second+"/eye-diagrams/motility/OU", map[string]any{"annotations": []map[string]any{motilityMark}, "notes": "", "version": 0}, a.doctor); saved.Code != http.StatusCreated {
		t.Fatalf("motility chart save: %d %s", saved.Code, saved.Body.String())
	}
	if saved := a.request(http.MethodPut, "/api/v1/encounters/"+second+"/eye-diagrams/fundus/OS", map[string]any{"annotations": []map[string]any{{"x": 32, "y": 50, "shape": "dot", "color": "#dc2626", "label": "disc haemorrhage", "structure": "optic_disc"}}, "notes": "", "version": 0}, a.doctor); saved.Code != http.StatusCreated {
		t.Fatalf("fundus chart save: %d %s", saved.Code, saved.Body.String())
	}

	fetched := a.request(http.MethodGet, "/api/v1/encounters/"+second+"/eye-diagrams", nil, a.nurse)
	if fetched.Code != http.StatusOK {
		t.Fatalf("charts get: %d %s", fetched.Code, fetched.Body.String())
	}
	var payload struct {
		Charts map[string]map[string]struct {
			Annotations []chartMark `json:"annotations"`
			Notes       string      `json:"notes"`
			Version     int         `json:"version"`
		} `json:"charts"`
		Previous *struct {
			EncounterNumber string `json:"encounterNumber"`
			Charts          map[string]map[string]struct {
				Annotations []chartMark `json:"annotations"`
			} `json:"charts"`
		} `json:"previous"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &payload); err != nil {
		t.Fatalf("charts decode: %v", err)
	}
	if got := payload.Charts["motility"]["OU"].Annotations; len(got) != 1 || got[0].Grade != "-2" || got[0].Cell != "up-right" {
		t.Fatalf("motility annotations = %+v", got)
	}
	if got := payload.Charts["fundus"]["OS"].Annotations; len(got) != 1 || got[0].Label != "disc haemorrhage" {
		t.Fatalf("fundus annotations = %+v", got)
	}
	// Charts never opened still come back as empty entries so the client renders the full set.
	if entry, ok := payload.Charts["anterior"]["OD"]; !ok || entry.Version != 0 || len(entry.Annotations) != 0 {
		t.Fatalf("unopened anterior chart = %+v (present=%v)", entry, ok)
	}
	if payload.Previous == nil {
		t.Fatal("expected the earlier charted visit to be returned for overlay")
	}
	if got := payload.Previous.Charts["field"]["OD"].Annotations; len(got) != 1 || got[0].Grade != "absent" {
		t.Fatalf("previous field annotations = %+v", got)
	}

	// The first visit has nothing before it, so no overlay is offered.
	firstFetched := a.request(http.MethodGet, "/api/v1/encounters/"+first+"/eye-diagrams", nil, a.nurse)
	var firstPayload struct {
		Previous *struct{} `json:"previous"`
	}
	if err := json.Unmarshal(firstFetched.Body.Bytes(), &firstPayload); err != nil {
		t.Fatalf("first charts decode: %v", err)
	}
	if firstPayload.Previous != nil {
		t.Fatalf("first visit should have no previous charts, got %s", firstFetched.Body.String())
	}
}
