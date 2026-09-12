package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func multipartFileRequest(t *testing.T, method, path, fieldName, filename string, content []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

var testCardJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 0xFF, 0xD9}

func TestInsuranceProviderCRUDWithNewFields(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{
		"name": "OFATMA", "website": "https://ofatma.ht", "acceptedCoverage": "Consultations, spectacles",
		"billingInfo": "Net 30", "claimInstructions": "Submit within 60 days", "defaultCoveragePercent": 65,
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create provider: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)

	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/payers/"+id, nil, a.nurse))
	if detail["website"] != "https://ofatma.ht" || detail["claimInstructions"] != "Submit within 60 days" || detail["defaultCoveragePercent"] != float64(65) {
		t.Fatalf("provider detail lost its fields: %v", detail)
	}

	updated := a.request(http.MethodPut, "/api/v1/insurance/payers/"+id, map[string]any{
		"name": "OFATMA", "notes": "Preferred insurer", "defaultCoveragePercent": 70, "active": true, "version": 1,
	}, a.doctor)
	if updated.Code != http.StatusOK {
		t.Fatalf("update provider: %d %s", updated.Code, updated.Body.String())
	}
	detail = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/payers/"+id, nil, a.doctor))
	if detail["notes"] != "Preferred insurer" || detail["defaultCoveragePercent"] != float64(70) {
		t.Fatalf("provider update did not persist: %v", detail)
	}

	if forbidden := a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "Nurse Insurer"}, a.nurse); forbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse create provider status = %d, want 403", forbidden.Code)
	}

	invalid := a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "Bad", "defaultCoveragePercent": 140}, a.doctor)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-range coverage = %d, want 422", invalid.Code)
	}
}

func TestOnlyOneActivePrimaryPolicyPerPatient(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Primary", "Policy")
	first := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "OFATMA", "coveragePercent": 60, "isPrimary": true,
	}, a.doctor))
	second := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "Secondary Co", "coveragePercent": 20, "isPrimary": true,
	}, a.doctor))

	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/policies?patientId="+patient.ID, nil, a.doctor))
	items, _ := list["items"].([]any)
	primaries := 0
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["isPrimary"] == true {
			primaries++
		}
	}
	if primaries != 1 {
		t.Fatalf("expected exactly one primary policy, found %d: %v", primaries, items)
	}
	firstID, secondID := first["id"].(string), second["id"].(string)

	// Explicitly promoting the first policy back to primary must demote the second.
	promote := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance/"+firstID+"/set-primary", map[string]any{"version": 1}, a.doctor)
	if promote.Code != http.StatusOK {
		t.Fatalf("set-primary: %d %s", promote.Code, promote.Body.String())
	}
	list = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/policies?patientId="+patient.ID, nil, a.doctor))
	items, _ = list["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["id"] == firstID && item["isPrimary"] != true {
			t.Fatalf("promoted policy is not primary: %v", item)
		}
		if item["id"] == secondID && item["isPrimary"] == true {
			t.Fatalf("the previous primary was not demoted: %v", item)
		}
	}
}

func TestDeactivatingAPolicyClearsPrimaryAndReactivateRestoresIt(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Deactivate", "Policy")
	policy := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "OFATMA", "coveragePercent": 60, "isPrimary": true,
	}, a.doctor))
	id := policy["id"].(string)

	deactivated := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance/"+id+"/deactivate", map[string]any{"version": 1}, a.doctor)
	if deactivated.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", deactivated.Code, deactivated.Body.String())
	}
	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/policies?patientId="+patient.ID, nil, a.doctor))
	items, _ := list["items"].([]any)
	item, _ := items[0].(map[string]any)
	if item["isActive"] != false || item["isPrimary"] != false {
		t.Fatalf("deactivated policy should be inactive and not primary: %v", item)
	}

	reactivated := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance/"+id+"/reactivate", map[string]any{"version": 2}, a.doctor)
	if reactivated.Code != http.StatusOK {
		t.Fatalf("reactivate: %d %s", reactivated.Code, reactivated.Body.String())
	}
}

func TestManualInsuranceVerificationWorkflow(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Verify", "Policy")
	policy := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "OFATMA", "coveragePercent": 60,
	}, a.doctor))
	id := policy["id"].(string)

	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/policies?patientId="+patient.ID, nil, a.doctor))
	items, _ := list["items"].([]any)
	if item, _ := items[0].(map[string]any); item["verificationStatus"] != "not_verified" {
		t.Fatalf("a new policy should start not_verified: %v", item)
	}

	invalid := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance/"+id+"/verify", map[string]any{"status": "definitely-verified", "version": 1}, a.doctor)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid verification status = %d, want 400", invalid.Code)
	}
	verified := a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance/"+id+"/verify", map[string]any{
		"status": "verified", "reference": "REF-1", "contact": "Marie at OFATMA", "notes": "Called and confirmed", "version": 1,
	}, a.doctor)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", verified.Code, verified.Body.String())
	}
	list = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/policies?patientId="+patient.ID, nil, a.doctor))
	items, _ = list["items"].([]any)
	item, _ := items[0].(map[string]any)
	if item["verificationStatus"] != "verified" || item["verifiedBy"] == "" || item["verificationReference"] != "REF-1" {
		t.Fatalf("verification was not recorded: %v", item)
	}
}

func TestInsuranceCardUploadReplaceAndDeletePermissions(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Card", "Patient")
	policy := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "OFATMA", "coveragePercent": 60,
	}, a.doctor))
	id := policy["id"].(string)

	upload := a.requestRaw(multipartFileRequest(t, http.MethodPost, "/api/v1/patient-insurance/"+id+"/cards/front", "file", "front.jpg", testCardJPEG, nil), a.doctor)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload front card: %d %s", upload.Code, upload.Body.String())
	}
	invalidSide := a.requestRaw(multipartFileRequest(t, http.MethodPost, "/api/v1/patient-insurance/"+id+"/cards/sideways", "file", "x.jpg", testCardJPEG, nil), a.doctor)
	if invalidSide.Code != http.StatusBadRequest {
		t.Fatalf("invalid card side = %d, want 400", invalidSide.Code)
	}

	content := a.request(http.MethodGet, "/api/v1/patient-insurance/"+id+"/cards/front/content", nil, a.doctor)
	if content.Code != http.StatusOK || !bytes.Equal(content.Body.Bytes(), testCardJPEG) {
		t.Fatalf("card content mismatch: %d", content.Code)
	}
	anonymous := a.request(http.MethodGet, "/api/v1/patient-insurance/"+id+"/cards/front/content", nil, nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous card read = %d, want 401", anonymous.Code)
	}

	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/patient-insurance/"+id+"/cards", nil, a.doctor))
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("expected exactly one card recorded, got %v", items)
	}

	// A nurse may capture/replace a card during intake…
	replace := a.requestRaw(multipartFileRequest(t, http.MethodPost, "/api/v1/patient-insurance/"+id+"/cards/front", "file", "front2.jpg", testCardJPEG, nil), a.nurse)
	if replace.Code != http.StatusCreated {
		t.Fatalf("replace as nurse: %d %s", replace.Code, replace.Body.String())
	}
	list = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/patient-insurance/"+id+"/cards", nil, a.doctor))
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("a replace must not leave two rows for the same side: %v", items)
	}

	// …but deleting one outright is doctor-only.
	forbidden := a.request(http.MethodDelete, "/api/v1/patient-insurance/"+id+"/cards/front", nil, a.nurse)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("nurse delete card status = %d, want 403", forbidden.Code)
	}
	deleted := a.request(http.MethodDelete, "/api/v1/patient-insurance/"+id+"/cards/front", nil, a.doctor)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("doctor delete card: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestPolicyDeletionIsBlockedWhileACardOrDocumentExists(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Blocked", "Delete")
	policy := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/patients/"+patient.ID+"/insurance", map[string]any{
		"payerName": "OFATMA", "coveragePercent": 60,
	}, a.doctor))
	id := policy["id"].(string)
	a.requestRaw(multipartFileRequest(t, http.MethodPost, "/api/v1/patient-insurance/"+id+"/cards/front", "file", "front.jpg", testCardJPEG, nil), a.doctor)

	blocked := a.request(http.MethodDelete, "/api/v1/patients/"+patient.ID+"/insurance/"+id+"?version=1", nil, a.doctor)
	if blocked.Code != http.StatusUnprocessableEntity {
		t.Fatalf("delete with a card attached = %d, want 422", blocked.Code)
	}
	a.request(http.MethodDelete, "/api/v1/patient-insurance/"+id+"/cards/front", nil, a.doctor)
	allowed := a.request(http.MethodDelete, "/api/v1/patients/"+patient.ID+"/insurance/"+id+"?version=1", nil, a.doctor)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("delete after removing the card: %d %s", allowed.Code, allowed.Body.String())
	}
}

func TestInsuranceDocumentRequestReceiveAndStatusWorkflow(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Forms", "Patient")
	payerID := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "OFATMA"}, a.doctor))["id"].(string)

	requested := a.request(http.MethodPost, "/api/v1/insurance/documents", map[string]any{
		"payerId": payerID, "patientId": patient.ID, "documentType": "claim_form", "status": "requested",
	}, a.doctor)
	if requested.Code != http.StatusCreated {
		t.Fatalf("create document request: %d %s", requested.Code, requested.Body.String())
	}
	docID := decodeResponse[map[string]any](t, requested)["id"].(string)

	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/documents?patientId="+patient.ID, nil, a.doctor))
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected the requested document to be listed: %v", items)
	}
	if item, _ := items[0].(map[string]any); item["status"] != "requested" || item["hasFile"] != false {
		t.Fatalf("unexpected initial document state: %v", item)
	}

	upload := a.requestRaw(multipartFileRequest(t, http.MethodPost, "/api/v1/insurance/documents/"+docID+"/upload", "file", "form.pdf", pdfBytes, nil), a.nurse)
	if upload.Code != http.StatusOK {
		t.Fatalf("upload form: %d %s", upload.Code, upload.Body.String())
	}
	list = decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/documents?patientId="+patient.ID, nil, a.doctor))
	items, _ = list["items"].([]any)
	item, _ := items[0].(map[string]any)
	if item["status"] != "received" || item["hasFile"] != true {
		t.Fatalf("uploading a file should move the document to received: %v", item)
	}

	content := a.request(http.MethodGet, "/api/v1/insurance/documents/"+docID+"/content", nil, a.doctor)
	if content.Code != http.StatusOK || !bytes.Equal(content.Body.Bytes(), pdfBytes) {
		t.Fatalf("document content mismatch: %d", content.Code)
	}

	completed := a.request(http.MethodPatch, "/api/v1/insurance/documents/"+docID+"/status", map[string]any{"status": "completed", "version": 2}, a.doctor)
	if completed.Code != http.StatusOK {
		t.Fatalf("complete document: %d %s", completed.Code, completed.Body.String())
	}

	forbiddenDelete := a.request(http.MethodDelete, "/api/v1/insurance/documents/"+docID+"?version=3", nil, a.nurse)
	if forbiddenDelete.Code != http.StatusForbidden {
		t.Fatalf("nurse delete document status = %d, want 403", forbiddenDelete.Code)
	}
	deleted := a.request(http.MethodDelete, "/api/v1/insurance/documents/"+docID+"?version=3", nil, a.doctor)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("doctor delete document: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestClaimLinksMustBelongToTheSamePatient(t *testing.T) {
	a := newTestApp(t)
	owner := a.createPatient(a.doctor, "Owner", "Patient")
	other := a.createPatient(a.doctor, "Other", "Patient")
	payerID := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "OFATMA"}, a.doctor))["id"].(string)
	invoiceID, _ := a.sellTo(t, owner.ID, 100000)

	mismatched := a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": other.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 100000, "payerPortionMinor": 100000, "patientPortionMinor": 0,
	}, a.doctor)
	if mismatched.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a claim for another patient's invoice = %d, want 422", mismatched.Code)
	}
}

func TestDraftClaimCanBeEditedButNotAfterSubmission(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Editable", "Claim")
	payerID := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "OFATMA"}, a.doctor))["id"].(string)
	invoiceID, _ := a.sellTo(t, patient.ID, 100000)
	claim := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 100000, "payerPortionMinor": 70000, "patientPortionMinor": 30000,
	}, a.doctor))
	id := claim["id"].(string)

	edited := a.request(http.MethodPut, "/api/v1/insurance/claims/"+id, map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 100000, "payerPortionMinor": 80000, "patientPortionMinor": 20000, "version": 1,
	}, a.doctor)
	if edited.Code != http.StatusOK {
		t.Fatalf("edit draft claim: %d %s", edited.Code, edited.Body.String())
	}

	detail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/claims/"+id, nil, a.doctor))
	if detail["payerPortionMinor"] != float64(80000) {
		t.Fatalf("draft edit did not persist: %v", detail)
	}

	submitted := a.request(http.MethodPatch, "/api/v1/insurance/claims/"+id+"/status", map[string]any{"status": "submitted", "version": 2}, a.doctor)
	if submitted.Code != http.StatusOK {
		t.Fatalf("submit claim: %d %s", submitted.Code, submitted.Body.String())
	}
	blockedEdit := a.request(http.MethodPut, "/api/v1/insurance/claims/"+id, map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 100000, "payerPortionMinor": 50000, "patientPortionMinor": 50000, "version": 3,
	}, a.doctor)
	if blockedEdit.Code != http.StatusUnprocessableEntity {
		t.Fatalf("editing a submitted claim = %d, want 422", blockedEdit.Code)
	}
}

// TestPatientAndInsurancePaymentsTogetherSettleTheInvoice is the acceptance
// test for the billing-split rule: the invoice must not read as fully paid
// just because the patient paid their own share, and it must become fully
// paid once the insurer pays the rest — without a second payments row for
// the insurer's money (which would double the income ledger).
func TestPatientAndInsurancePaymentsTogetherSettleTheInvoice(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Split", "Bill")
	payerID := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "OFATMA"}, a.doctor))["id"].(string)
	invoiceID, _ := a.sellTo(t, patient.ID, 1000000) // 10,000 HTG

	claim := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 1000000, "payerPortionMinor": 700000, "patientPortionMinor": 300000,
	}, a.doctor))
	claimID := claim["id"].(string)

	methods := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/payment-methods", nil, a.doctor))
	methodItems, _ := methods["items"].([]any)
	firstMethod, _ := methodItems[0].(map[string]any)
	methodID := firstMethod["id"].(string)

	patientPayment := a.request(http.MethodPost, "/api/v1/invoices/"+invoiceID+"/payments", map[string]any{
		"paymentMethodId": methodID, "amountMinor": 300000, "currency": "HTG",
	}, a.doctor)
	if patientPayment.Code != http.StatusCreated {
		t.Fatalf("patient payment: %d %s", patientPayment.Code, patientPayment.Body.String())
	}

	mid := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/invoices/"+invoiceID, nil, a.doctor))
	if mid["status"] == "paid" {
		t.Fatalf("the invoice must not read as fully paid before the insurer has paid anything: %v", mid)
	}
	if mid["balanceMinor"] != float64(700000) {
		t.Fatalf("outstanding balance after only the patient portion should be the insurance share: %v", mid["balanceMinor"])
	}

	approved := a.request(http.MethodPatch, "/api/v1/insurance/claims/"+claimID+"/status", map[string]any{"status": "approved", "approvedAmountMinor": 700000, "version": 1}, a.doctor)
	if approved.Code != http.StatusOK {
		t.Fatalf("approve claim: %d %s", approved.Code, approved.Body.String())
	}
	insurerPayment := a.request(http.MethodPost, "/api/v1/insurance/claims/"+claimID+"/payments", map[string]any{
		"amountMinor": 700000, "paymentDate": "2026-09-12",
	}, a.doctor)
	if insurerPayment.Code != http.StatusCreated {
		t.Fatalf("insurer payment: %d %s", insurerPayment.Code, insurerPayment.Body.String())
	}

	final := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/invoices/"+invoiceID, nil, a.doctor))
	if final["status"] != "paid" || final["balanceMinor"] != float64(0) {
		t.Fatalf("invoice should be fully settled once patient and insurer together cover the total: %v", final)
	}
	if final["insuranceExpectedMinor"] != float64(700000) || final["insuranceReceivedMinor"] != float64(700000) {
		t.Fatalf("insurance breakdown incorrect: %v", final)
	}
}

// TestPartialInsurancePaymentLeavesADisposableDifference exercises an
// insurer approving and paying less than the full expected amount: the
// remaining difference must stay visible as an outstanding balance rather
// than being silently absorbed anywhere.
func TestPartialInsurancePaymentLeavesADisposableDifference(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Partial", "Insurer")
	payerID := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/payers", map[string]any{"name": "OFATMA"}, a.doctor))["id"].(string)
	invoiceID, _ := a.sellTo(t, patient.ID, 1000000)
	claim := decodeResponse[map[string]any](t, a.request(http.MethodPost, "/api/v1/insurance/claims", map[string]any{
		"patientId": patient.ID, "payerId": payerID, "invoiceId": invoiceID,
		"claimAmountMinor": 1000000, "payerPortionMinor": 700000, "patientPortionMinor": 300000,
	}, a.doctor))
	claimID := claim["id"].(string)

	a.request(http.MethodPatch, "/api/v1/insurance/claims/"+claimID+"/status", map[string]any{"status": "approved", "approvedAmountMinor": 550000, "version": 1}, a.doctor)
	claimDetail := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/insurance/claims/"+claimID, nil, a.doctor))
	if claimDetail["partiallyApproved"] != true || claimDetail["approvedAmountMinor"] != float64(550000) {
		t.Fatalf("a lower approved amount should be flagged as a partial approval: %v", claimDetail)
	}

	paid := a.request(http.MethodPost, "/api/v1/insurance/claims/"+claimID+"/payments", map[string]any{"amountMinor": 550000, "paymentDate": "2026-09-12"}, a.doctor)
	if paid.Code != http.StatusCreated {
		t.Fatalf("partial insurer payment: %d %s", paid.Code, paid.Body.String())
	}
	// Trying to collect the originally-expected 700000 must fail: only what
	// was actually approved/paid is owed by the insurer now.
	overpay := a.request(http.MethodPost, "/api/v1/insurance/claims/"+claimID+"/payments", map[string]any{"amountMinor": 150000, "paymentDate": "2026-09-12"}, a.doctor)
	if overpay.Code != http.StatusUnprocessableEntity {
		t.Fatalf("paying beyond the approved amount = %d, want 422", overpay.Code)
	}

	final := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/invoices/"+invoiceID, nil, a.doctor))
	// The patient has not paid their 300000 in this test either, so only the
	// insurer's actual 550000 is settled: 1000000 total - 550000 = 450000
	// remains outstanding — the 150000 gap between expected (700000) and
	// approved/paid (550000) is not silently absorbed anywhere.
	if final["balanceMinor"] != float64(450000) {
		t.Fatalf("expected the un-collected difference to remain outstanding: %v", final)
	}
	if final["insuranceExpectedMinor"] != float64(700000) || final["insuranceReceivedMinor"] != float64(550000) {
		t.Fatalf("expected-vs-received insurance figures should stay distinct: %v", final)
	}
}
