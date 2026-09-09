package server

import (
	"net/http"
	"testing"
)

func TestPrescriptionCanBeIssuedWithoutAConsultation(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Standalone", "Prescription")

	created := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "spectacle",
		"od": map[string]string{"sphere": "-1.50"}, "os": map[string]string{"sphere": "-1.25"},
		"notes": "Issued at the counter, no consultation today",
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("standalone prescription: %d %s", created.Code, created.Body.String())
	}
	record := decodeResponse[map[string]any](t, created)
	if record["standalone"] != true || record["encounterId"] != "" {
		t.Fatalf("standalone prescription was not flagged as one: %v", record)
	}

	// It belongs to the patient file and shows in their prescription history.
	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/prescriptions?patientId="+patient.ID, nil, a.doctor))
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("patient prescription history has %d entries, want 1", len(items))
	}
	entry, _ := items[0].(map[string]any)
	if entry["standalone"] != true {
		t.Fatalf("history entry is not flagged as issued outside a consultation: %v", entry)
	}

	timeline := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/patients/"+patient.ID+"/timeline", nil, a.doctor))
	entries, _ := timeline["items"].([]any)
	found := false
	for _, item := range entries {
		if record, _ := item.(map[string]any); record["type"] == "prescription" {
			found = true
		}
	}
	if !found {
		t.Fatal("standalone prescription is missing from the patient timeline")
	}
}

func TestPrescriptionFromAConsultationIsNotFlaggedStandalone(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Consulted", "Prescription")
	encounter := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Exam"}, a.doctor)
	if encounter.Code != http.StatusCreated {
		t.Fatalf("encounter: %d %s", encounter.Code, encounter.Body.String())
	}
	encounterID := decodeResponse[map[string]any](t, encounter)["id"].(string)

	created := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "encounterId": encounterID, "type": "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("prescription: %d %s", created.Code, created.Body.String())
	}
	if decodeResponse[map[string]any](t, created)["standalone"] != false {
		t.Fatal("a prescription issued from a consultation must not be flagged standalone")
	}
}

func TestPrescriptionRejectsAConsultationBelongingToAnotherPatient(t *testing.T) {
	a := newTestApp(t)
	owner := a.createPatient(a.doctor, "Encounter", "Owner")
	other := a.createPatient(a.doctor, "Someone", "Else")
	encounter := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": owner.ID, "visitReason": "Exam"}, a.doctor)
	if encounter.Code != http.StatusCreated {
		t.Fatalf("encounter: %d %s", encounter.Code, encounter.Body.String())
	}
	encounterID := decodeResponse[map[string]any](t, encounter)["id"].(string)

	mismatched := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": other.ID, "encounterId": encounterID, "type": "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.doctor)
	if mismatched.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-patient consultation status = %d, want 422", mismatched.Code)
	}
	missing := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": other.ID, "encounterId": "does-not-exist", "type": "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.doctor)
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown consultation status = %d, want 422", missing.Code)
	}
}

func TestOnlyADoctorCanIssueAStandalonePrescription(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Nurse", "Blocked")
	response := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.nurse)
	if response.Code != http.StatusForbidden {
		t.Fatalf("nurse issuing a prescription = %d, want 403", response.Code)
	}
}

func TestPrescriptionRegistersAnUnknownPersonInTheSameStep(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"newPatient": map[string]any{"firstName": "Walk", "lastName": "In", "sex": "female", "dateOfBirth": "1980-04-02", "phone": "3456 0011", "tags": []string{}},
		"type":       "spectacle", "od": map[string]string{"sphere": "-2.00"}, "os": map[string]string{"sphere": "-2.25"},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("prescription for a new person: %d %s", created.Code, created.Body.String())
	}
	record := decodeResponse[map[string]any](t, created)
	registered, _ := record["registeredPatient"].(map[string]any)
	if registered == nil || registered["medicalRecordNumber"] == "" {
		t.Fatalf("the person was not registered as a patient: %v", record)
	}
	patientID := registered["id"].(string)
	if record["patientId"] != patientID {
		t.Fatalf("the prescription is filed against %v, not the registered patient", record["patientId"])
	}

	// They are a full patient: findable, with the prescription in their file.
	patient := a.request(http.MethodGet, "/api/v1/patients/"+patientID, nil, a.doctor)
	if patient.Code != http.StatusOK {
		t.Fatalf("registered patient lookup: %d %s", patient.Code, patient.Body.String())
	}
	if decodeResponse[patientRecord](t, patient).LastName != "In" {
		t.Fatal("the registered patient does not carry the details given on the prescription")
	}
	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/prescriptions?patientId="+patientID, nil, a.doctor))
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("the new patient's file holds %d prescriptions, want 1", len(items))
	}
}

func TestPrescriptionForAnUnknownPersonPointsAtAnExistingDuplicate(t *testing.T) {
	a := newTestApp(t)
	existing := a.request(http.MethodPost, "/api/v1/patients", map[string]any{"firstName": "Marie", "lastName": "Joseph", "dateOfBirth": "1975-06-01", "phone": "", "tags": []string{}}, a.doctor)
	if existing.Code != http.StatusCreated {
		t.Fatalf("seed patient: %d %s", existing.Code, existing.Body.String())
	}
	existingID := decodeResponse[patientRecord](t, existing).ID

	duplicate := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"newPatient": map[string]any{"firstName": "Marie", "lastName": "Joseph", "dateOfBirth": "1975-06-01", "tags": []string{}},
		"type":       "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.doctor)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate registration = %d, want 409: %s", duplicate.Code, duplicate.Body.String())
	}
	body := decodeResponse[map[string]any](t, duplicate)
	details, _ := body["details"].(map[string]any)
	if body["code"] != "POSSIBLE_DUPLICATE_PATIENT" || details["patientId"] != existingID {
		t.Fatalf("the duplicate response does not point at the existing patient: %v", body)
	}
	// Nothing was written: no prescription and no second patient record.
	var patients, prescriptions int
	_ = a.server.db.QueryRow("SELECT COUNT(*) FROM patients WHERE lower(last_name)='joseph'").Scan(&patients)
	_ = a.server.db.QueryRow("SELECT COUNT(*) FROM prescriptions").Scan(&prescriptions)
	if patients != 1 || prescriptions != 0 {
		t.Fatalf("a refused registration left %d patients and %d prescriptions behind", patients, prescriptions)
	}
}

func TestPrescriptionRejectsBothAnExistingAndANewPatient(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Both", "Ways")
	response := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "newPatient": map[string]any{"firstName": "Someone", "lastName": "Else", "tags": []string{}},
		"type": "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("naming both an existing and a new patient = %d, want 422", response.Code)
	}
}

func TestPrescriptionValidatesTheNewPersonsDetails(t *testing.T) {
	a := newTestApp(t)
	response := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"newPatient": map[string]any{"firstName": "  ", "lastName": "", "tags": []string{}},
		"type":       "spectacle", "od": map[string]string{"sphere": "-1.00"},
	}, a.doctor)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a nameless new person = %d, want 422", response.Code)
	}
	var patients int
	_ = a.server.db.QueryRow("SELECT COUNT(*) FROM patients").Scan(&patients)
	if patients != 0 {
		t.Fatalf("an invalid registration created %d patients", patients)
	}
}
