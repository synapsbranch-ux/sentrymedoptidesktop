package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func (a *testApp) searchDiagnosisCodes(t *testing.T, query string) []map[string]any {
	t.Helper()
	response := a.request(http.MethodGet, "/api/v1/codes/icd10?q="+query, nil, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("search %q: %d %s", query, response.Code, response.Body.String())
	}
	body := decodeResponse[map[string]any](t, response)
	raw, _ := body["items"].([]any)
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		items = append(items, entry)
	}
	return items
}

func containsCode(items []map[string]any, code string) bool {
	for _, item := range items {
		if item["code"] == code {
			return true
		}
	}
	return false
}

func TestDiagnosisSearchUnderstandsPlainLanguage(t *testing.T) {
	a := newTestApp(t)
	for _, testCase := range []struct{ query, wantCode string }{
		{"red%20eye", "H10.9"},
		{"blurry%20vision", "H52.7"},
		{"lazy%20eye", "H53.003"},
		{"double%20vision", "H53.2"},
		{"cloudy%20lens", "H26.9"},
		{"high%20eye%20pressure", "H40.9"},
		{"watery%20eye", "H04.203"},
		{"stye", "H00.019"},
		{"cannot%20see%20in%20the%20dark", "H53.60"},
	} {
		if items := a.searchDiagnosisCodes(t, testCase.query); !containsCode(items, testCase.wantCode) {
			t.Errorf("searching %q did not offer %s; got %v", testCase.query, testCase.wantCode, codesOf(items))
		}
	}
}

func codesOf(items []map[string]any) []string {
	codes := make([]string, 0, len(items))
	for _, item := range items {
		codes = append(codes, item["code"].(string))
	}
	return codes
}

func TestDiagnosisSearchStillAcceptsACodeTypedDirectly(t *testing.T) {
	a := newTestApp(t)
	items := a.searchDiagnosisCodes(t, "H52.13")
	if len(items) == 0 || items[0]["code"] != "H52.13" {
		t.Fatalf("typing a code did not return it first: %v", codesOf(items))
	}
	if items[0]["description"] != "Myopia, bilateral" {
		t.Fatalf("code lookup returned %v", items[0]["description"])
	}
}

func TestDiagnosisReferenceCanBeBrowsedByCategory(t *testing.T) {
	a := newTestApp(t)
	categories := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/diagnosis-codes/categories", nil, a.nurse))
	items, _ := categories["items"].([]any)
	if len(items) < 10 {
		t.Fatalf("the reference offers only %d categories", len(items))
	}
	page := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/diagnosis-codes?category=Glaucoma&limit=10&page=1", nil, a.nurse))
	if total, _ := page["total"].(float64); total < 20 {
		t.Fatalf("the glaucoma category holds only %v codes", page["total"])
	}
	rows, _ := page["items"].([]any)
	if len(rows) != 10 {
		t.Fatalf("a page of 10 returned %d rows", len(rows))
	}
	if page["hasMore"] != true {
		t.Fatal("a first page of a larger category should report more to come")
	}
}

func TestDiagnosisReferenceIsSubstantial(t *testing.T) {
	a := newTestApp(t)
	page := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/diagnosis-codes?limit=1", nil, a.nurse))
	if total, _ := page["total"].(float64); total < 400 {
		t.Fatalf("the bundled reference holds only %v codes, want a real library", page["total"])
	}
}

func TestClinicCanImportItsOwnCodeFileAndItSurvivesAReload(t *testing.T) {
	a := newTestApp(t)
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	file, err := form.CreateFormFile("file", "codes.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("code,description,category,synonyms\n9A00.0,Locally coded corneal scar,Cornea,white mark on the eye\nH52.13,Myopia bilateral (clinic wording),Refraction,short sighted\n,Missing code is skipped,,\n")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/diagnosis-codes/import", body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := a.requestRaw(request, a.doctor)
	if response.Code != http.StatusOK {
		t.Fatalf("import: %d %s", response.Code, response.Body.String())
	}
	result := decodeResponse[map[string]any](t, response)
	if result["imported"] != float64(2) || result["skipped"] != float64(1) {
		t.Fatalf("import counted %v imported and %v skipped", result["imported"], result["skipped"])
	}
	if items := a.searchDiagnosisCodes(t, "white%20mark"); !containsCode(items, "9A00.0") {
		t.Fatalf("an imported code is not searchable: %v", codesOf(items))
	}

	// A later reference upgrade replaces only the bundled rows; the clinic's own
	// wording for a code it imported stays.
	if _, err := a.server.db.Exec("UPDATE reference_data_versions SET checksum='stale' WHERE name='icd10_ophthalmology'"); err != nil {
		t.Fatal(err)
	}
	if err := a.server.syncDiagnosisReference(t.Context()); err != nil {
		t.Fatal(err)
	}
	items := a.searchDiagnosisCodes(t, "H52.13")
	if len(items) == 0 || items[0]["description"] != "Myopia bilateral (clinic wording)" {
		t.Fatalf("a reference reload overwrote the clinic's imported code: %v", items)
	}
	if items[0]["source"] != "clinic" {
		t.Fatalf("imported code reports source %v", items[0]["source"])
	}
}

func TestOnlyADoctorCanImportDiagnosisCodes(t *testing.T) {
	a := newTestApp(t)
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	file, _ := form.CreateFormFile("file", "codes.csv")
	_, _ = file.Write([]byte("code,description\nH52.13,Myopia\n"))
	_ = form.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/diagnosis-codes/import", body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	if response := a.requestRaw(request, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse importing codes = %d, want 403", response.Code)
	}
}

func TestDiagnosisSearchDoesNotTreatPunctuationAsQuerySyntax(t *testing.T) {
	a := newTestApp(t)
	// An FTS5 expression typed by accident must return nothing, not an error.
	for _, query := range []string{"%22", "*", "AND%20OR", "NEAR("} {
		if response := a.request(http.MethodGet, "/api/v1/codes/icd10?q="+query, nil, a.doctor); response.Code != http.StatusOK {
			t.Fatalf("searching %q = %d %s", query, response.Code, response.Body.String())
		}
	}
}
