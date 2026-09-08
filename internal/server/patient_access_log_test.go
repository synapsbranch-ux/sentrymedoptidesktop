package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func auditEntries(t *testing.T, a *testApp) []map[string]string {
	t.Helper()
	response := a.request(http.MethodGet, "/api/v1/audit?limit=200", nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("audit: %d %s", response.Code, response.Body.String())
	}
	raw := decodeResponse[map[string]any](t, response)
	items, _ := raw["items"].([]any)
	entries := make([]map[string]string, 0, len(items))
	for _, item := range items {
		record, _ := item.(map[string]any)
		entry := map[string]string{}
		for key, value := range record {
			if text, ok := value.(string); ok {
				entry[key] = text
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func countReads(entries []map[string]string, entityType, entityID string) int {
	total := 0
	for _, entry := range entries {
		if entry["action"] == "read" && entry["entityType"] == entityType && entry["entityId"] == entityID {
			total++
		}
	}
	return total
}

func TestOpeningAPatientRecordIsAttributable(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Access", "Logged")

	if response := a.request(http.MethodGet, "/api/v1/patients/"+patient.ID, nil, a.nurse); response.Code != http.StatusOK {
		t.Fatalf("open record: %d", response.Code)
	}
	entries := auditEntries(t, a)
	if countReads(entries, "patient_record", patient.ID) != 1 {
		t.Fatalf("opening a patient record was not recorded: %v", entries)
	}
	// The entry names who did it, which is the whole point of the control.
	found := false
	for _, entry := range entries {
		if entry["entityType"] == "patient_record" && strings.Contains(entry["summary"], "Access Logged") {
			if entry["user"] == "" {
				t.Fatalf("access entry does not name the user: %v", entry)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no readable access entry: %v", entries)
	}
}

func TestViewingAPatientDocumentIsAttributable(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Document", "Access")
	created := a.requestRaw(multipartDocumentRequest(t, patient.ID, "scan.pdf", pdfBytes), a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)

	if response := a.request(http.MethodGet, "/api/v1/documents/"+id+"/content", nil, a.nurse); response.Code != http.StatusOK {
		t.Fatalf("view: %d", response.Code)
	}
	if countReads(auditEntries(t, a), "patient_document", patient.ID) != 1 {
		t.Fatal("viewing a patient document was not recorded")
	}
}

func TestSearchingPatientsIsNotLoggedAsRecordAccess(t *testing.T) {
	a := newTestApp(t)
	a.createPatient(a.doctor, "Search", "Noise")
	for i := 0; i < 5; i++ {
		if response := a.request(http.MethodGet, "/api/v1/patients?q=Noise", nil, a.doctor); response.Code != http.StatusOK {
			t.Fatalf("search: %d", response.Code)
		}
	}
	// A search is not a record view; logging it would bury the entries that matter.
	for _, entry := range auditEntries(t, a) {
		if entry["action"] == "read" {
			t.Fatalf("a patient search produced a record-access entry: %v", entry)
		}
	}
}

func TestARecordLeftOpenIsNotLoggedOnEveryRefresh(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Refresh", "Window")
	for i := 0; i < 8; i++ {
		if response := a.request(http.MethodGet, "/api/v1/patients/"+patient.ID, nil, a.doctor); response.Code != http.StatusOK {
			t.Fatalf("open: %d", response.Code)
		}
	}
	if reads := countReads(auditEntries(t, a), "patient_record", patient.ID); reads != 1 {
		t.Fatalf("a record left open produced %d entries, want 1 within the window", reads)
	}

	// A different clinician opening the same record is its own entry.
	if response := a.request(http.MethodGet, "/api/v1/patients/"+patient.ID, nil, a.nurse); response.Code != http.StatusOK {
		t.Fatalf("nurse open: %d", response.Code)
	}
	if reads := countReads(auditEntries(t, a), "patient_record", patient.ID); reads != 2 {
		t.Fatalf("a second clinician's access was folded into the first: %d entries", reads)
	}
}

func TestAccessWindowExpiresAndDoesNotGrowWithoutBound(t *testing.T) {
	log := newPatientAccessLog()
	start := time.Now()
	if !log.shouldRecord("user|patient|kind", start) {
		t.Fatal("the first access must be recorded")
	}
	if log.shouldRecord("user|patient|kind", start.Add(time.Minute)) {
		t.Fatal("a repeat inside the window must not be recorded again")
	}
	if !log.shouldRecord("user|patient|kind", start.Add(patientAccessLogWindow+time.Second)) {
		t.Fatal("returning to a record after the window must be recorded")
	}
	// Aged entries are dropped, so a long-running clinic server does not leak.
	for i := 0; i < 500; i++ {
		log.shouldRecord(string(rune(i))+"|p|k", start)
	}
	log.shouldRecord("sweeper|p|k", start.Add(2*patientAccessLogWindow))
	if len(log.recent) > 2 {
		t.Fatalf("the access map kept %d aged entries", len(log.recent))
	}
}
