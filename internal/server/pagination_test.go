package server

import (
	"fmt"
	"net/http"
	"testing"
)

// Every list a clinic grows without bound must be paged by the server. A
// frontend slice of a full result set still ships every row over the LAN, which
// is the failure this guards against.
func TestListEndpointsPageOnTheServer(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Paged", "Lists")
	for index := 0; index < 7; index++ {
		if code := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": fmt.Sprintf("Visit %d", index)}, a.doctor).Code; code != http.StatusCreated {
			t.Fatalf("seed consultation %d: %d", index, code)
		}
		if code := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{"patientId": patient.ID, "type": "spectacle", "od": map[string]string{"sphere": "-1.00"}}, a.doctor).Code; code != http.StatusCreated {
			t.Fatalf("seed prescription %d: %d", index, code)
		}
		if code := a.request(http.MethodPost, "/api/v1/inventory", map[string]any{"sku": fmt.Sprintf("SKU-%03d", index), "category": "frame", "name": fmt.Sprintf("Frame %d", index), "salePriceMinor": 1000, "trackStock": true}, a.doctor).Code; code != http.StatusCreated {
			t.Fatalf("seed inventory %d: %d", index, code)
		}
	}

	for _, path := range []string{"/api/v1/encounters", "/api/v1/prescriptions", "/api/v1/inventory", "/api/v1/audit"} {
		first := a.pageOf(t, path+"?limit=3&page=1")
		if len(first.items) != 3 {
			t.Fatalf("%s first page returned %d rows, want 3", path, len(first.items))
		}
		if first.total < 7 {
			t.Fatalf("%s reported a total of %d", path, first.total)
		}
		if !first.hasMore {
			t.Fatalf("%s did not report more pages to come", path)
		}
		second := a.pageOf(t, path+"?limit=3&page=2")
		if len(second.items) != 3 {
			t.Fatalf("%s second page returned %d rows, want 3", path, len(second.items))
		}
		if first.items[0]["id"] == second.items[0]["id"] {
			t.Fatalf("%s returned the same first row on both pages", path)
		}
	}
}

func TestPagingBeyondTheLastPageReturnsAnEmptyPageNotEveryRow(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Beyond", "End")
	if code := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Only one"}, a.doctor).Code; code != http.StatusCreated {
		t.Fatal("seed consultation")
	}
	page := a.pageOf(t, "/api/v1/encounters?limit=10&page=5")
	if len(page.items) != 0 || page.hasMore {
		t.Fatalf("a page past the end returned %d rows (hasMore=%v)", len(page.items), page.hasMore)
	}
}

func TestListLimitsAreClampedRatherThanTrusted(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Clamped", "Limit")
	for index := 0; index < 3; index++ {
		_ = a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Visit"}, a.doctor)
	}
	// A limit past the endpoint's ceiling falls back to its default instead of
	// letting a caller ask for the whole table in one request.
	page := a.pageOf(t, "/api/v1/encounters?limit=100000")
	if page.limit != 50 {
		t.Fatalf("an oversized limit was honoured as %d", page.limit)
	}
	// Nonsense values are read as a request for the default page, not refused.
	for _, query := range []string{"?limit=0", "?limit=-5", "?page=0", "?page=abc&limit=xyz"} {
		if response := a.request(http.MethodGet, "/api/v1/encounters"+query, nil, a.doctor); response.Code != http.StatusOK {
			t.Fatalf("listing with %q = %d", query, response.Code)
		}
	}
}

type listPage struct {
	items   []map[string]any
	total   int
	limit   int
	hasMore bool
}

func (a *testApp) pageOf(t *testing.T, path string) listPage {
	t.Helper()
	response := a.request(http.MethodGet, path, nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
	}
	body := decodeResponse[map[string]any](t, response)
	raw, _ := body["items"].([]any)
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		items = append(items, entry)
	}
	total, _ := body["total"].(float64)
	limit, _ := body["limit"].(float64)
	hasMore, _ := body["hasMore"].(bool)
	if _, ok := body["total"]; !ok {
		t.Fatalf("GET %s returned no total; it is not paged on the server", path)
	}
	return listPage{items: items, total: int(total), limit: int(limit), hasMore: hasMore}
}
