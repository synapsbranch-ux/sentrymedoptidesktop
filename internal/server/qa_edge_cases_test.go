package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Section 6 of the stabilisation brief: a systematic pass over edge data and
// permissions, rather than only the five records a demo uses.

func TestPatientRecordsAcceptAwkwardRealNames(t *testing.T) {
	a := newTestApp(t)
	names := []struct{ first, last string }{
		{"Jéan-Baptiste", "Étienne"},                       // accents and a hyphen
		{"O'Brien", "D'Argenson"},                          // apostrophes
		{"Ñuño", "Müller-Ødegård"},                         // mixed diacritics
		{strings.Repeat("Maximilian", 12), "Verylongname"}, // 120 characters
		{"李", "王"},                                          // non-Latin script
	}
	for _, name := range names {
		created := a.request(http.MethodPost, "/api/v1/patients", map[string]any{"firstName": name.first, "lastName": name.last, "tags": []string{}}, a.doctor)
		if created.Code != http.StatusCreated {
			t.Fatalf("create %q %q: %d %s", name.first, name.last, created.Code, created.Body.String())
		}
		record := decodeResponse[patientRecord](t, created)
		if record.FirstName != name.first || record.LastName != name.last {
			t.Fatalf("stored %q %q, want %q %q", record.FirstName, record.LastName, name.first, name.last)
		}
		// An apostrophe is an FTS operator character; it must be searchable, not
		// an error, and must not be treated as a query operator.
		found := patientNames(a.searchPatients(t, "q="+strings.ReplaceAll(name.last, " ", "+")))
		if len(found) == 0 {
			t.Fatalf("searching for %q found nothing", name.last)
		}
	}
}

func TestPatientRejectsMissingIdentityAndImpossibleDates(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []map[string]any{
		{"lastName": "Nofirst", "tags": []string{}},
		{"firstName": "Nolast", "tags": []string{}},
		{"firstName": "   ", "lastName": "   ", "tags": []string{}},
		{"firstName": "Bad", "lastName": "Date", "dateOfBirth": "04/05/1990", "tags": []string{}},
		{"firstName": "Bad", "lastName": "Date", "dateOfBirth": "1990-13-45", "tags": []string{}},
	} {
		response := a.request(http.MethodPost, "/api/v1/patients", body, a.doctor)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v status = %d, want 422", body, response.Code)
		}
	}
}

func TestPatientAcceptsDatesFarInThePastAndTheFuture(t *testing.T) {
	a := newTestApp(t)
	// A date of birth is not range-checked; the age filter must simply not
	// mis-handle either extreme.
	for _, dateOfBirth := range []string{"1901-01-01", "2099-12-31"} {
		created := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
			"firstName": "Extreme", "lastName": "Dates" + dateOfBirth[:4], "dateOfBirth": dateOfBirth, "tags": []string{},
		}, a.doctor)
		if created.Code != http.StatusCreated {
			t.Fatalf("create with dateOfBirth %s: %d %s", dateOfBirth, created.Code, created.Body.String())
		}
	}
	if names := patientNames(a.searchPatients(t, "q=Extreme&ageMin=0&ageMax=150")); len(names) != 1 {
		t.Fatalf("age filter over extreme dates returned %v; the future date of birth must not match an age range", names)
	}
}

func TestEmptyStatesReturnListsRatherThanNulls(t *testing.T) {
	a := newTestApp(t)
	// A JSON null instead of [] is what makes a list screen throw on .map.
	for _, path := range []string{
		"/api/v1/patients", "/api/v1/appointments", "/api/v1/queue", "/api/v1/encounters",
		"/api/v1/prescriptions", "/api/v1/documents", "/api/v1/invoices", "/api/v1/lab-orders",
		"/api/v1/inventory", "/api/v1/suppliers", "/api/v1/catalogs/prescription_item",
	} {
		response := a.request(http.MethodGet, path, nil, a.doctor)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"items":[`) {
			t.Fatalf("%s returned %s; an empty list must be [] and never null", path, response.Body.String())
		}
	}
}

func TestUnknownRecordsReturnNotFoundRatherThanAnEmptyPage(t *testing.T) {
	a := newTestApp(t)
	for _, path := range []string{
		"/api/v1/patients/does-not-exist",
		"/api/v1/encounters/does-not-exist",
		"/api/v1/documents/does-not-exist/content",
		"/api/v1/prescriptions/does-not-exist/signature",
	} {
		if response := a.request(http.MethodGet, path, nil, a.doctor); response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, response.Code)
		}
	}
}

func TestNurseCannotReachDoctorOnlyDataByDirectURL(t *testing.T) {
	a := newTestApp(t)
	// The client hides these screens from a nurse; the server must refuse them
	// even when the URL is typed in directly.
	for _, path := range []string{
		"/api/v1/audit", "/api/v1/users", "/api/v1/backups", "/api/v1/expenses",
		"/api/v1/finance/summary", "/api/v1/reports/sales", "/api/v1/purchase-orders",
	} {
		if response := a.request(http.MethodGet, path, nil, a.nurse); response.Code != http.StatusForbidden {
			t.Fatalf("nurse GET %s = %d, want 403", path, response.Code)
		}
	}
	// And the writes a nurse must not perform.
	if response := a.request(http.MethodPost, "/api/v1/users", map[string]any{"username": "x", "displayName": "X", "role": "doctor", "password": "Secret-Passphrase-2026"}, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse creating a user = %d, want 403", response.Code)
	}
	if response := a.request(http.MethodPut, "/api/v1/settings/clinic", map[string]any{"value": map[string]any{}, "version": 1}, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse changing settings = %d, want 403", response.Code)
	}
}

func TestSignedOutRequestsAreRefusedAcrossTheAPI(t *testing.T) {
	a := newTestApp(t)
	for _, path := range []string{"/api/v1/patients", "/api/v1/queue", "/api/v1/encounters", "/api/v1/prescriptions", "/api/v1/documents", "/api/v1/catalogs/appointment_reason", "/api/v1/me/signature"} {
		if response := a.request(http.MethodGet, path, nil, nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous GET %s = %d, want 401", path, response.Code)
		}
	}
}

func TestConcurrentEditsAreRefusedRatherThanSilentlyOverwriting(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Concurrent", "Edit")
	body := map[string]any{"firstName": "Concurrent", "lastName": "Edit", "tags": []string{}, "version": patient.Version}
	if first := a.request(http.MethodPut, "/api/v1/patients/"+patient.ID, body, a.doctor); first.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", first.Code, first.Body.String())
	}
	// The second device still holds the old version: a double submit must not
	// silently overwrite the first save.
	second := a.request(http.MethodPut, "/api/v1/patients/"+patient.ID, body, a.doctor)
	if second.Code != http.StatusConflict {
		t.Fatalf("stale save = %d, want 409", second.Code)
	}
}

func TestArchivedPatientDisappearsFromTheDefaultListButKeepsItsRecord(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Archive", "Lifecycle")
	if removed := a.request(http.MethodDelete, "/api/v1/patients/"+patient.ID, nil, a.doctor); removed.Code != http.StatusNoContent {
		t.Fatalf("archive: %d", removed.Code)
	}
	if names := patientNames(a.searchPatients(t, "q=Lifecycle")); len(names) != 0 {
		t.Fatalf("archived patient still listed: %v", names)
	}
	if again := a.request(http.MethodDelete, "/api/v1/patients/"+patient.ID, nil, a.doctor); again.Code != http.StatusNotFound {
		t.Fatalf("archiving twice = %d, want 404", again.Code)
	}
	if reachable := a.request(http.MethodGet, "/api/v1/patients/"+patient.ID, nil, a.doctor); reachable.Code != http.StatusNotFound {
		t.Fatalf("archived patient fetch = %d, want 404", reachable.Code)
	}
}

func TestQueueRefusesToCheckTheSamePatientInTwice(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Double", "Checkin")
	first := a.request(http.MethodPost, "/api/v1/queue/check-in", map[string]any{"patientId": patient.ID, "priority": 0}, a.nurse)
	if first.Code != http.StatusCreated {
		t.Fatalf("first check-in: %d %s", first.Code, first.Body.String())
	}
	// A double submit from an impatient click must not create two queue entries.
	second := a.request(http.MethodPost, "/api/v1/queue/check-in", map[string]any{"patientId": patient.ID, "priority": 0}, a.nurse)
	if second.Code != http.StatusConflict {
		t.Fatalf("second check-in = %d, want 409", second.Code)
	}
	queue := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/queue", nil, a.nurse))
	if items, _ := queue["items"].([]any); len(items) != 1 {
		t.Fatalf("queue has %d entries after a double submit, want 1", len(items))
	}
}

func TestAppointmentRejectsAnUnparseableStartTime(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Bad", "Appointment")
	for _, startsAt := range []string{"", "not-a-time", "2026-13-45T99:99"} {
		response := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{
			"patientId": patient.ID, "startsAt": startsAt, "durationMinutes": 30, "type": "eye_exam",
		}, a.doctor)
		if response.Code < 400 {
			t.Fatalf("appointment with startsAt %q was accepted (%d)", startsAt, response.Code)
		}
	}
	// A far-future appointment is legitimate and must be accepted.
	future := time.Now().AddDate(5, 0, 0).UTC().Format(time.RFC3339)
	if response := a.request(http.MethodPost, "/api/v1/appointments", map[string]any{
		"patientId": patient.ID, "startsAt": future, "durationMinutes": 30, "type": "eye_exam",
	}, a.doctor); response.Code != http.StatusCreated {
		t.Fatalf("far-future appointment = %d %s", response.Code, response.Body.String())
	}
}
