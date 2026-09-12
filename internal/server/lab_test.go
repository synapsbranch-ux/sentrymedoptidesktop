package server

import (
	"fmt"
	"net/http"
	"testing"
)

func (a *testApp) labDocument(t *testing.T, orderID string) map[string]any {
	t.Helper()
	response := a.request(http.MethodGet, "/api/v1/lab-orders/"+orderID, nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("lab order document: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](t, response)
}

func section(t *testing.T, document map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := document[key].(map[string]any)
	if !ok {
		t.Fatalf("the document carries no %s: %v", key, document[key])
	}
	return value
}

// The defect the clinic reported: the glazing company received a sheet naming a
// frame and a coating, with no powers on it at all. A lens cannot be ground from
// that, and nothing on the order said which consultation or which sale it came
// from.
func TestAnOpticalOrderCarriesItsPrescriptionToTheWorkshop(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Workshop", "Order")

	consultation := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Blurred distance vision"}, a.doctor)
	if consultation.Code != http.StatusCreated {
		t.Fatalf("consultation: %d %s", consultation.Code, consultation.Body.String())
	}
	encounterID := decodeResponse[map[string]any](t, consultation)["id"].(string)

	issued := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "encounterId": encounterID, "type": "spectacle",
		"od":      map[string]string{"sphere": "-2.25", "cylinder": "-0.75", "axis": "175", "add": "+2.00"},
		"os":      map[string]string{"sphere": "-1.75", "cylinder": "-0.50", "axis": "10", "add": "+2.00"},
		"details": map[string]string{"pd": "63", "pdOD": "31.5", "pdOS": "31.5"},
	}, a.doctor)
	if issued.Code != http.StatusCreated {
		t.Fatalf("prescription: %d %s", issued.Code, issued.Body.String())
	}
	prescriptionID := decodeResponse[map[string]any](t, issued)["id"].(string)

	frameID := a.createInventoryItem(map[string]any{"sku": "FR-LAB", "category": "frame", "name": "Aviator Gold", "brand": "Rayline", "salePriceMinor": 180000, "currency": "HTG", "quantity": 3, "trackStock": true})
	lensID := a.createInventoryItem(map[string]any{"sku": "LN-LAB", "category": "ophthalmic_lens", "name": "Progressive 1.61", "salePriceMinor": 220000, "currency": "HTG", "quantity": 10, "trackStock": true})
	sale := a.request(http.MethodPost, "/api/v1/pos/checkout", map[string]any{
		"invoice": map[string]any{"patientId": patient.ID, "currency": "HTG", "items": []map[string]any{
			{"inventoryItemId": frameID, "description": "Aviator Gold", "quantity": 1, "unitPriceMinor": 180000},
			{"inventoryItemId": lensID, "description": "Progressive 1.61", "quantity": 1, "unitPriceMinor": 220000},
		}},
		"payments": []map[string]any{{"paymentMethodId": a.cashMethodID(t), "amountMinor": 400000, "currency": "HTG"}},
	}, a.doctor)
	if sale.Code != http.StatusCreated {
		t.Fatalf("sale: %d %s", sale.Code, sale.Body.String())
	}
	invoiceID := decodeResponse[map[string]any](t, sale)["invoiceId"].(string)

	created := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{
		"patientId": patient.ID, "prescriptionId": prescriptionID, "invoiceId": invoiceID,
		"frameItemId": frameID, "lensItemId": lensID,
		"lensType": "Progressive", "material": "High index 1.61", "tint": "Photogray",
		"coatings": []string{"Anti-reflective", "UV protection"}, "treatments": []string{"Photochromic"},
		"measurements": map[string]any{"pd": "63", "fittingHeight": "22"},
		"notes":        "Mount with the patient's own case.",
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("lab order: %d %s", created.Code, created.Body.String())
	}
	orderID := decodeResponse[map[string]any](t, created)["id"].(string)

	document := a.labDocument(t, orderID)

	prescription := section(t, document, "prescription")
	od, _ := prescription["od"].(map[string]any)
	os, _ := prescription["os"].(map[string]any)
	details, _ := prescription["details"].(map[string]any)
	if od["sphere"] != "-2.25" || od["cylinder"] != "-0.75" || od["axis"] != "175" {
		t.Fatalf("the right eye did not reach the workshop: %v", od)
	}
	if os["sphere"] != "-1.75" || os["add"] != "+2.00" {
		t.Fatalf("the left eye did not reach the workshop: %v", os)
	}
	if details["pd"] != "63" {
		t.Fatalf("the pupillary distance did not reach the workshop: %v", details)
	}

	order := section(t, document, "order")
	if order["tint"] != "Photogray" {
		t.Fatalf("the tint the patient paid for is not on the order: %v", order["tint"])
	}
	if order["kind"] != labKindOptical {
		t.Fatalf("an order with a frame and lenses is %v, want optical", order["kind"])
	}

	// The three records the clinic said were never tied together.
	if section(t, document, "encounter")["encounterNumber"] == "" {
		t.Fatal("the order does not name the consultation it came from")
	}
	invoice := section(t, document, "invoice")
	if lines, _ := invoice["lines"].([]any); len(lines) != 2 {
		t.Fatalf("the sale reached the document with %d lines, want the frame and the lenses", len(lines))
	}
	if invoice["balanceMinor"] != float64(0) {
		t.Fatalf("a fully paid sale shows a balance of %v", invoice["balanceMinor"])
	}
	if section(t, document, "frame")["name"] != "Aviator Gold" {
		t.Fatal("the frame is not named on the document")
	}
}

func TestALaboratoryRequestNeedsAtLeastOneExam(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Empty", "Request")
	empty := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{"kind": "medical", "patientId": patient.ID}, a.doctor)
	if empty.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an exam request with no exams: %d %s", empty.Code, empty.Body.String())
	}
	if code := decodeResponse[map[string]any](t, empty)["code"]; code != "LAB_TESTS_REQUIRED" {
		t.Fatalf("code=%v, want LAB_TESTS_REQUIRED", code)
	}
	if unknown := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{"kind": "dental", "patientId": patient.ID}, a.doctor); unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown kind of order: %d", unknown.Code)
	}

	tests := make([]map[string]any, maxLabTestsPerOrder+1)
	for index := range tests {
		tests[index] = map[string]any{"label": fmt.Sprintf("Exam %d", index)}
	}
	if long := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{"kind": "medical", "patientId": patient.ID, "tests": tests}, a.doctor); long.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a request past the exam limit: %d", long.Code)
	}
}

// Quality control inspects a lens seated in a frame. Applied to a blood test it
// would strand every exam request one step short of the patient.
func TestALaboratoryRequestIsNotHeldForOpticalQualityControl(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Exam", "Request")
	created := a.request(http.MethodPost, "/api/v1/lab-orders", map[string]any{
		"kind": "medical", "patientId": patient.ID, "notes": "Fasting since midnight",
		"tests": []map[string]any{{"label": "Fasting glucose", "code": "GLU", "specimen": "Blood"}, {"label": "HbA1c", "specimen": "Blood"}},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("exam request: %d %s", created.Code, created.Body.String())
	}
	orderID := decodeResponse[map[string]any](t, created)["id"].(string)

	if response := a.request(http.MethodPatch, "/api/v1/lab-orders/"+orderID+"/status", map[string]any{"status": "ready", "version": 1}, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("an exam request was held for optical quality control: %d %s", response.Code, response.Body.String())
	}

	document := a.labDocument(t, orderID)
	exams, _ := document["tests"].([]any)
	if len(exams) != 2 {
		t.Fatalf("the request carries %d exams, want 2", len(exams))
	}
	if first, _ := exams[0].(map[string]any); first["label"] != "Fasting glucose" || first["specimen"] != "Blood" {
		t.Fatalf("the first exam did not survive the round trip: %v", exams[0])
	}

	// An optical order is still gated, so relaxing the rule for exams did not
	// remove the check on glasses.
	optical := a.createLabOrder(t, patient.ID, "Glasses")
	if response := a.request(http.MethodPatch, "/api/v1/lab-orders/"+optical+"/status", map[string]any{"status": "ready", "version": 1}, a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("glasses reached ready without quality control: %d %s", response.Code, response.Body.String())
	}
}

// Who collected the glasses and when is a record of something that happened.
// Correcting a status typed in error must not quietly erase it.
func TestAnOrderLeavingDeliveredKeepsWhoCollectedIt(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Delivered", "Record")
	orderID := a.createLabOrder(t, patient.ID, "Glasses")
	if response := a.request(http.MethodPost, "/api/v1/lab-orders/"+orderID+"/quality-control", map[string]any{
		"prescriptionVerified": true, "powerVerified": true, "axisVerified": true, "frameCondition": true,
		"lensCondition": true, "fittingVerified": true, "finalCleaning": true,
	}, a.doctor); response.Code != http.StatusCreated {
		t.Fatalf("quality control: %d %s", response.Code, response.Body.String())
	}
	if response := a.request(http.MethodPatch, "/api/v1/lab-orders/"+orderID+"/status", map[string]any{"status": "delivered", "receivedByName": "Marie Joseph", "version": 1}, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("deliver: %d %s", response.Code, response.Body.String())
	}
	if response := a.request(http.MethodPatch, "/api/v1/lab-orders/"+orderID+"/status", map[string]any{"status": "ready", "version": 2}, a.doctor); response.Code != http.StatusOK {
		t.Fatalf("correct the status: %d %s", response.Code, response.Body.String())
	}
	order := section(t, a.labDocument(t, orderID), "order")
	if order["receivedByName"] != "Marie Joseph" || order["deliveredAt"] == "" {
		t.Fatalf("correcting a status erased the handover: receivedBy=%v deliveredAt=%v", order["receivedByName"], order["deliveredAt"])
	}
}

func TestAnUnknownLabOrderIsNotFound(t *testing.T) {
	a := newTestApp(t)
	if response := a.request(http.MethodGet, "/api/v1/lab-orders/does-not-exist", nil, a.doctor); response.Code != http.StatusNotFound {
		t.Fatalf("reading a missing order: %d", response.Code)
	}
	moved := a.request(http.MethodPatch, "/api/v1/lab-orders/does-not-exist/status", map[string]any{"status": "ordered", "version": 1}, a.doctor)
	if moved.Code != http.StatusNotFound {
		t.Fatalf("moving a missing order: %d %s", moved.Code, moved.Body.String())
	}
}

// The lens options a glazing company needs spelled out are the clinic's to edit,
// like every other list in the application.
func TestTheClinicEditsItsOwnLensAndExamLists(t *testing.T) {
	a := newTestApp(t)
	for _, catalog := range []string{"lens_type", "lens_material", "lens_coating", "lens_tint", "lens_treatment", "lab_test"} {
		if response := a.request(http.MethodGet, "/api/v1/catalogs/"+catalog, nil, a.doctor); response.Code != http.StatusOK {
			t.Fatalf("catalog %s: %d %s", catalog, response.Code, response.Body.String())
		}
	}
	tints, _ := a.catalog(t, "lens_tint", "")["items"].([]any)
	labels := map[string]bool{}
	for _, entry := range tints {
		if row, ok := entry.(map[string]any); ok {
			labels[fmt.Sprint(row["label"])] = true
		}
	}
	if !labels["Photogray"] {
		t.Fatalf("the tint list does not offer Photogray: %v", labels)
	}
	// Clinical content nobody approved must not be invented for them.
	if exams, _ := a.catalog(t, "lab_test", "")["items"].([]any); len(exams) != 0 {
		t.Fatalf("the exam catalog was seeded with %d entries the clinic never approved", len(exams))
	}
	added := a.request(http.MethodPost, "/api/v1/catalogs/lab_test", map[string]any{"label": "Fasting glucose", "sortOrder": 10}, a.doctor)
	if added.Code != http.StatusCreated {
		t.Fatalf("add an exam: %d %s", added.Code, added.Body.String())
	}
}
