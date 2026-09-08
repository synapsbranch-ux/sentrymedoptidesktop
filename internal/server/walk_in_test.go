package server

import (
	"net/http"
	"testing"
)

func TestWalkInRegistersAndQueuesInOneRequest(t *testing.T) {
	a := newTestApp(t)
	response := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{
		"firstName": "Rosemène", "lastName": "Walkin", "phone": "50931110000", "reason": "Red eye since yesterday",
	}, a.nurse)
	if response.Code != http.StatusCreated {
		t.Fatalf("walk-in: %d %s", response.Code, response.Body.String())
	}
	created := decodeResponse[map[string]any](t, response)
	if created["patientCreated"] != true {
		t.Fatal("walk-in did not register the new patient")
	}
	if created["medicalRecordNumber"] == "" {
		t.Fatal("walk-in did not allocate a file number")
	}

	queue := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/queue", nil, a.nurse))
	items, _ := queue["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("queue has %d entries, want 1", len(items))
	}
	entry, _ := items[0].(map[string]any)
	if entry["patientName"] != "Rosemène Walkin" {
		t.Fatalf("queue entry name = %v", entry["patientName"])
	}
	if entry["visitReason"] != "Red eye since yesterday" {
		t.Fatalf("queue entry reason = %v, want the walk-in reason", entry["visitReason"])
	}
	if entry["phone"] != "50931110000" {
		t.Fatalf("queue entry phone = %v", entry["phone"])
	}
	if entry["stage"] != "waiting_nurse" {
		t.Fatalf("queue entry stage = %v, want waiting_nurse", entry["stage"])
	}
	if entry["source"] != "staff" {
		t.Fatalf("queue entry source = %v, want staff", entry["source"])
	}
	if entry["appointmentId"] != "" {
		t.Fatalf("walk-in entry has appointment %v; a walk-in is identified by having none", entry["appointmentId"])
	}

	// The new record must be findable immediately, which means the search index
	// was maintained by the same transaction.
	found := patientNames(a.searchPatients(t, "q=Rosemene"))
	if len(found) != 1 {
		t.Fatalf("accent-insensitive search for the new walk-in returned %v", found)
	}
}

func TestWalkInRefusesToCreateADuplicateRecord(t *testing.T) {
	a := newTestApp(t)
	first := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"firstName": "Pierre", "lastName": "Duplicate", "phone": "50931110001"}, a.nurse)
	if first.Code != http.StatusCreated {
		t.Fatalf("first walk-in: %d %s", first.Code, first.Body.String())
	}
	patientID := decodeResponse[map[string]any](t, first)["patientId"].(string)

	again := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"firstName": "pierre", "lastName": "duplicate", "phone": "50931110001"}, a.nurse)
	if again.Code != http.StatusConflict {
		t.Fatalf("duplicate walk-in status = %d, want 409", again.Code)
	}
	details, _ := decodeResponse[map[string]any](t, again)["details"].(map[string]any)
	if details["patientId"] != patientID {
		t.Fatalf("conflict pointed at %v, want the existing record %s", details["patientId"], patientID)
	}

	// Choosing that existing record must still be refused while they are queued.
	queued := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"patientId": patientID, "reason": "Follow-up"}, a.nurse)
	if queued.Code != http.StatusConflict {
		t.Fatalf("re-queueing status = %d, want 409", queued.Code)
	}
}

func TestWalkInQueuesAnExistingPatientWithoutCreatingOne(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Existing", "Walkin")
	response := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"patientId": patient.ID, "reason": "Broken frame"}, a.nurse)
	if response.Code != http.StatusCreated {
		t.Fatalf("walk-in: %d %s", response.Code, response.Body.String())
	}
	if decodeResponse[map[string]any](t, response)["patientCreated"] != false {
		t.Fatal("walk-in created a second record for an existing patient")
	}
	total := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/patients?q=Walkin", nil, a.doctor))["total"]
	if total != float64(1) {
		t.Fatalf("patient count = %v, want 1", total)
	}
}

func TestWalkInRequiresAName(t *testing.T) {
	a := newTestApp(t)
	response := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"phone": "50931110002", "reason": "Eye pain"}, a.nurse)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("nameless walk-in status = %d, want 422", response.Code)
	}
}

func TestQueueStagesAdvanceWithoutReopeningTheEntry(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"firstName": "Stage", "lastName": "Progression", "reason": "Routine"}, a.nurse)
	if created.Code != http.StatusCreated {
		t.Fatalf("walk-in: %d %s", created.Code, created.Body.String())
	}
	entry := decodeResponse[map[string]any](t, created)
	id := entry["id"].(string)
	version := 1
	// Waiting to in-consultation to checkout to completed, one call each.
	for _, stage := range []string{"in_consultation", "checkout", "completed"} {
		response := a.request(http.MethodPatch, "/api/v1/queue/"+id, map[string]any{"stage": stage, "priority": 0, "version": version}, a.nurse)
		if response.Code != http.StatusOK {
			t.Fatalf("stage %s: %d %s", stage, response.Code, response.Body.String())
		}
		version = int(decodeResponse[map[string]any](t, response)["version"].(float64))
	}
	queue := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/queue", nil, a.nurse))
	if items, _ := queue["items"].([]any); len(items) != 0 {
		t.Fatalf("completed entry is still in the waiting room: %v", items)
	}
}
