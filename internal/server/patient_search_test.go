package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestPatientMatchExpressionEscapesUserInput(t *testing.T) {
	for _, testCase := range []struct{ query, want string }{
		{"jean", `"jean"*`},
		{"  jean   pierre ", `"jean"* AND "pierre"*`},
		{"O'Brien", `"O'Brien"*`},
		{"Jéan", `"Jéan"*`},
		{"PT-000123", `"PT-000123"*`},
		{"AND OR NOT", `"AND"* AND "OR"* AND "NOT"*`},
		{`" * ^ : -`, ""},
		{"", ""},
	} {
		if got := buildMatchExpression(testCase.query); got != testCase.want {
			t.Fatalf("buildMatchExpression(%q) = %q, want %q", testCase.query, got, testCase.want)
		}
	}
}

func (a *testApp) searchPatients(t *testing.T, query string) map[string]any {
	t.Helper()
	response := a.request(http.MethodGet, "/api/v1/patients?"+query, nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("search %q: %d %s", query, response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](t, response)
}

func patientNames(result map[string]any) []string {
	items, _ := result["items"].([]any)
	names := make([]string, 0, len(items))
	for _, entry := range items {
		record, _ := entry.(map[string]any)
		names = append(names, fmt.Sprintf("%v %v", record["firstName"], record["lastName"]))
	}
	return names
}

func TestPatientSearchIsAccentInsensitiveAndPaginated(t *testing.T) {
	a := newTestApp(t)
	accented := a.createPatient(a.doctor, "Jéan-Baptiste", "Étienne")
	a.createPatient(a.doctor, "Marie", "Clairval")

	for _, query := range []string{"jean", "JEAN", "Jéan", "etienne", "Étienne", "baptiste"} {
		names := patientNames(a.searchPatients(t, "q="+query))
		if len(names) != 1 || names[0] != "Jéan-Baptiste Étienne" {
			t.Fatalf("search %q returned %v, want only the accented patient", query, names)
		}
	}
	// Both terms must match: "jean clairval" belongs to nobody.
	if names := patientNames(a.searchPatients(t, "q=jean+clairval")); len(names) != 0 {
		t.Fatalf("multi-term search returned %v, want none", names)
	}
	if names := patientNames(a.searchPatients(t, "q="+accented.MedicalRecordNumber)); len(names) != 1 {
		t.Fatalf("file-number search returned %v, want one", names)
	}
}

func TestPatientSearchMatchesPhoneAndDateOfBirth(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": "Rose", "lastName": "Célestin", "phone": "+509 3456 7890", "dateOfBirth": "1985-03-04", "tags": []string{},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	for _, query := range []string{"3456", "34567890", "1985", "1985-03-04"} {
		if names := patientNames(a.searchPatients(t, "q="+query)); len(names) != 1 {
			t.Fatalf("search %q returned %v, want one", query, names)
		}
	}
}

func TestPatientSearchStaysIndexedAtFiveThousandRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("seeding 5,000 patients")
	}
	a := newTestApp(t)
	a.seedPatients(t, 5000)

	start := time.Now()
	result := a.searchPatients(t, "q=Zephirin&limit=25")
	elapsed := time.Since(start)
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("search over 5,000 records returned %d items, want 1", len(items))
	}
	// The acceptance target is 500ms on clinic hardware; a wide margin here
	// still fails loudly if the query ever falls back to a full-table LIKE scan.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("search over 5,000 records took %s, want under 500ms", elapsed)
	}

	page := a.searchPatients(t, "limit=25")
	if total, _ := page["total"].(float64); int(total) < 5000 {
		t.Fatalf("total = %v, want at least 5000", page["total"])
	}
	if listed, _ := page["items"].([]any); len(listed) != 25 {
		t.Fatalf("page returned %d items, want the requested 25 and never the whole table", len(listed))
	}
	if more, _ := page["hasMore"].(bool); !more {
		t.Fatal("hasMore should be true while 5,000 records remain unpaged")
	}
}

func TestPatientFiltersNarrowTheList(t *testing.T) {
	a := newTestApp(t)
	young := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": "Young", "lastName": "Filtercase", "sex": "female",
		"dateOfBirth": time.Now().UTC().AddDate(-8, 0, 0).Format("2006-01-02"), "tags": []string{},
	}, a.doctor)
	if young.Code != http.StatusCreated {
		t.Fatalf("create young: %d %s", young.Code, young.Body.String())
	}
	old := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": "Older", "lastName": "Filtercase", "sex": "male",
		"dateOfBirth": time.Now().UTC().AddDate(-70, 0, 0).Format("2006-01-02"), "tags": []string{},
	}, a.doctor)
	if old.Code != http.StatusCreated {
		t.Fatalf("create old: %d %s", old.Code, old.Body.String())
	}

	if names := patientNames(a.searchPatients(t, "q=Filtercase&ageMin=60")); len(names) != 1 || names[0] != "Older Filtercase" {
		t.Fatalf("ageMin=60 returned %v", names)
	}
	if names := patientNames(a.searchPatients(t, "q=Filtercase&ageMax=18")); len(names) != 1 || names[0] != "Young Filtercase" {
		t.Fatalf("ageMax=18 returned %v", names)
	}
	if names := patientNames(a.searchPatients(t, "q=Filtercase&sex=female")); len(names) != 1 || names[0] != "Young Filtercase" {
		t.Fatalf("sex filter returned %v", names)
	}
	if names := patientNames(a.searchPatients(t, "q=Filtercase&status=archived")); len(names) != 0 {
		t.Fatalf("archived filter returned %v before anything was archived", names)
	}

	archivedID := decodeResponse[patientRecord](t, young).ID
	if archived := a.request(http.MethodDelete, "/api/v1/patients/"+archivedID, nil, a.doctor); archived.Code != http.StatusNoContent {
		t.Fatalf("archive: %d", archived.Code)
	}
	if names := patientNames(a.searchPatients(t, "q=Filtercase")); len(names) != 1 || names[0] != "Older Filtercase" {
		t.Fatalf("default search returned %v, want only the active patient", names)
	}
	if names := patientNames(a.searchPatients(t, "q=Filtercase&status=archived")); len(names) != 1 {
		t.Fatalf("archived filter returned %v, want the archived patient", names)
	}
	if names := patientNames(a.searchPatients(t, "q=Filtercase&status=all")); len(names) != 2 {
		t.Fatalf("status=all returned %v, want both", names)
	}
}

func TestPatientListReportsLastVisit(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Lastvisit", "Reported")
	before := a.searchPatients(t, "q=Lastvisit")
	items, _ := before["items"].([]any)
	if record, _ := items[0].(map[string]any); record["lastVisitAt"] != "" {
		t.Fatalf("lastVisitAt = %v before any consultation, want empty", record["lastVisitAt"])
	}
	encounter := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Check"}, a.doctor)
	if encounter.Code != http.StatusCreated {
		t.Fatalf("encounter: %d %s", encounter.Code, encounter.Body.String())
	}
	after, _ := a.searchPatients(t, "q=Lastvisit")["items"].([]any)
	record, _ := after[0].(map[string]any)
	if record["lastVisitAt"] == "" {
		t.Fatal("lastVisitAt is empty after a consultation was recorded")
	}
}

func TestPatientFilterOptionsDoNotRequireLoadingEveryPatient(t *testing.T) {
	a := newTestApp(t)
	response := a.request(http.MethodGet, "/api/v1/patients/filter-options", nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("filter options: %d %s", response.Code, response.Body.String())
	}
	options := decodeResponse[map[string]any](t, response)
	practitioners, _ := options["practitioners"].([]any)
	if len(practitioners) == 0 {
		t.Fatal("filter options listed no practitioners")
	}
	if _, ok := options["insurers"].([]any); !ok {
		t.Fatal("filter options did not include an insurers list")
	}
}

// seedPatients inserts directly so that the volume test measures the query, not
// the HTTP create path. The search triggers still run, so the FTS index is real.
func (a *testApp) seedPatients(t *testing.T, count int) {
	t.Helper()
	surnames := []string{"Pierre", "Louis", "Jean", "Célestin", "Étienne", "Moïse", "Toussaint", "Désir", "Charles", "Zephirin"}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := a.server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var actor string
	if err := tx.QueryRow("SELECT id FROM users LIMIT 1").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO patients(id,medical_record_number,first_name,last_name,sex,date_of_birth,phone,tags_json,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,'[]',?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer statement.Close()
	for index := 0; index < count; index++ {
		// Exactly one patient carries the surname the volume test searches for.
		surname := surnames[index%(len(surnames)-1)]
		if index == count/2 {
			surname = "Zephirin"
		}
		id := fmt.Sprintf("seed-%06d", index)
		if _, err := statement.Exec(id, fmt.Sprintf("PT-SEED-%06d", index), fmt.Sprintf("Patient%d", index), surname,
			[]string{"female", "male"}[index%2], fmt.Sprintf("19%02d-0%d-1%d", 40+index%59, 1+index%9, index%9),
			fmt.Sprintf("509%07d", index), now, now, actor, actor); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
