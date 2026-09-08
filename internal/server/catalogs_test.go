package server

import (
	"net/http"
	"testing"
)

func catalogLabels(result map[string]any) []string {
	items, _ := result["items"].([]any)
	labels := make([]string, 0, len(items))
	for _, entry := range items {
		record, _ := entry.(map[string]any)
		labels = append(labels, record["label"].(string))
	}
	return labels
}

func (a *testApp) catalog(t *testing.T, name, query string) map[string]any {
	t.Helper()
	response := a.request(http.MethodGet, "/api/v1/catalogs/"+name+query, nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("read %s: %d %s", name, response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](t, response)
}

func TestAppointmentReasonsSeedTheListTheApplicationAlreadyHad(t *testing.T) {
	a := newTestApp(t)
	labels := catalogLabels(a.catalog(t, "appointment_reason", ""))
	for _, expected := range []string{"Eye examination", "Follow-up", "Contact lens", "Optical delivery"} {
		found := false
		for _, label := range labels {
			if label == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("seeded reasons %v are missing %q, which the hardcoded list had", labels, expected)
		}
	}
}

func TestPrescriptionCatalogStartsEmptyAndIsEditableWithoutADeployment(t *testing.T) {
	a := newTestApp(t)
	// Nothing clinical is invented on the clinic's behalf.
	if labels := catalogLabels(a.catalog(t, "prescription_item", "")); len(labels) != 0 {
		t.Fatalf("prescription catalog was seeded with %v; the starting list needs clinic confirmation", labels)
	}

	created := a.request(http.MethodPost, "/api/v1/catalogs/prescription_item", map[string]any{
		"label": "Timolol 0.5% drops", "sortOrder": 10,
		"details": map[string]string{"strength": "0.5%", "dosage": "1 drop", "frequency": "Twice daily", "route": "Topical, affected eye"},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	entry := decodeResponse[catalogEntry](t, created)
	if entry.Details["frequency"] != "Twice daily" {
		t.Fatalf("details were not stored: %+v", entry.Details)
	}
	if labels := catalogLabels(a.catalog(t, "prescription_item", "")); len(labels) != 1 {
		t.Fatalf("new entry is not visible to prescribers: %v", labels)
	}

	updated := a.request(http.MethodPut, "/api/v1/catalogs/prescription_item/"+entry.ID, map[string]any{
		"label": "Timolol 0.5% eye drops", "sortOrder": 10, "version": entry.Version,
		"details": map[string]string{"frequency": "Twice daily", "blank": "   "},
	}, a.doctor)
	if updated.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	if after := decodeResponse[catalogEntry](t, updated); after.Details["blank"] != "" || after.Label != "Timolol 0.5% eye drops" {
		t.Fatalf("update stored %+v", after)
	}
}

func TestRetiringACatalogEntryHidesItWithoutDeletingIt(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/catalogs/appointment_reason", map[string]any{"label": "Seasonal screening"}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[catalogEntry](t, created).ID

	if response := a.request(http.MethodDelete, "/api/v1/catalogs/appointment_reason/"+id, nil, a.doctor); response.Code != http.StatusNoContent {
		t.Fatalf("retire: %d %s", response.Code, response.Body.String())
	}
	for _, label := range catalogLabels(a.catalog(t, "appointment_reason", "")) {
		if label == "Seasonal screening" {
			t.Fatal("retired entry is still offered for new appointments")
		}
	}
	// It must still exist, so a record that already used it keeps its meaning.
	found := false
	for _, label := range catalogLabels(a.catalog(t, "appointment_reason", "?includeInactive=true")) {
		if label == "Seasonal screening" {
			found = true
		}
	}
	if !found {
		t.Fatal("retired entry was deleted rather than deactivated")
	}
}

func TestCatalogRejectsDuplicatesBlanksAndUnknownCatalogs(t *testing.T) {
	a := newTestApp(t)
	if response := a.request(http.MethodPost, "/api/v1/catalogs/appointment_reason", map[string]any{"label": "   "}, a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank label status = %d, want 422", response.Code)
	}
	if response := a.request(http.MethodPost, "/api/v1/catalogs/appointment_reason", map[string]any{"label": "eye examination"}, a.doctor); response.Code != http.StatusConflict {
		t.Fatalf("duplicate label status = %d, want 409 (matching is case-insensitive)", response.Code)
	}
	if response := a.request(http.MethodGet, "/api/v1/catalogs/made_up", nil, a.doctor); response.Code != http.StatusNotFound {
		t.Fatalf("unknown catalog status = %d, want 404", response.Code)
	}
}

func TestOnlyADoctorCanEditACatalog(t *testing.T) {
	a := newTestApp(t)
	if response := a.request(http.MethodGet, "/api/v1/catalogs/appointment_reason", nil, a.nurse); response.Code != http.StatusOK {
		t.Fatalf("nurse read status = %d, want 200: reasons are needed to book", response.Code)
	}
	if response := a.request(http.MethodPost, "/api/v1/catalogs/appointment_reason", map[string]any{"label": "Nurse added"}, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse write status = %d, want 403", response.Code)
	}
}
