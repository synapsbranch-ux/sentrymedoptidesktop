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
