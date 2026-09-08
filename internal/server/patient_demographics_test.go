package server

import (
	"net/http"
	"testing"
)

func TestPatientCivilStatusAndReligionAreOptionalAndEditable(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": "Optional", "lastName": "Demographics", "tags": []string{},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create without the new fields: %d %s", created.Code, created.Body.String())
	}
	record := decodeResponse[patientRecord](t, created)
	if record.CivilStatus != "" || record.Religion != "" {
		t.Fatalf("blank record came back with civilStatus=%q religion=%q", record.CivilStatus, record.Religion)
	}

	updated := a.request(http.MethodPut, "/api/v1/patients/"+record.ID, map[string]any{
		"firstName": "Optional", "lastName": "Demographics", "civilStatus": "common_law",
		"religion": "other", "religionOther": "Rastafari", "tags": []string{}, "version": record.Version,
	}, a.doctor)
	if updated.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	after := decodeResponse[patientRecord](t, updated)
	if after.CivilStatus != "common_law" || after.Religion != "other" || after.ReligionOther != "Rastafari" {
		t.Fatalf("stored %+v", after)
	}

	reloaded := decodeResponse[patientRecord](t, a.request(http.MethodGet, "/api/v1/patients/"+record.ID, nil, a.doctor))
	if reloaded.ReligionOther != "Rastafari" {
		t.Fatalf("reloaded record lost the free text: %+v", reloaded)
	}
}

func TestPatientDemographicsRejectUnlistedValues(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []map[string]any{
		{"firstName": "Bad", "lastName": "Status", "civilStatus": "its complicated", "tags": []string{}},
		{"firstName": "Bad", "lastName": "Religion", "religion": "made up", "tags": []string{}},
	} {
		response := a.request(http.MethodPost, "/api/v1/patients", body, a.doctor)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v status = %d, want 422", body, response.Code)
		}
	}
}

func TestReligionFreeTextIsDroppedWhenALabelIsChosen(t *testing.T) {
	a := newTestApp(t)
	created := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
		"firstName": "Switching", "lastName": "Religion", "religion": "other", "religionOther": "Rastafari", "tags": []string{},
	}, a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	record := decodeResponse[patientRecord](t, created)
	updated := a.request(http.MethodPut, "/api/v1/patients/"+record.ID, map[string]any{
		"firstName": "Switching", "lastName": "Religion", "religion": "catholic", "religionOther": "Rastafari",
		"tags": []string{}, "version": record.Version,
	}, a.doctor)
	if updated.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	if after := decodeResponse[patientRecord](t, updated); after.ReligionOther != "" {
		t.Fatalf("free text %q survived a change away from \"other\"", after.ReligionOther)
	}
}

func TestPatientsCanBeFilteredByCivilStatus(t *testing.T) {
	a := newTestApp(t)
	for status, name := range map[string]string{"married": "Married", "widowed": "Widowed"} {
		response := a.request(http.MethodPost, "/api/v1/patients", map[string]any{
			"firstName": name, "lastName": "Civilfilter", "civilStatus": status, "tags": []string{},
		}, a.doctor)
		if response.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", status, response.Code, response.Body.String())
		}
	}
	names := patientNames(a.searchPatients(t, "q=Civilfilter&civilStatus=widowed"))
	if len(names) != 1 || names[0] != "Widowed Civilfilter" {
		t.Fatalf("civil status filter returned %v", names)
	}
	if all := patientNames(a.searchPatients(t, "q=Civilfilter")); len(all) != 2 {
		t.Fatalf("unfiltered search returned %v, want both", all)
	}
}

func TestDemographicOptionsAreOfferedToTheClient(t *testing.T) {
	a := newTestApp(t)
	options := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/patients/filter-options", nil, a.doctor))
	statuses, _ := options["civilStatuses"].([]any)
	if len(statuses) != 6 {
		t.Fatalf("civil statuses = %v, want the six the clinic asked for", options["civilStatuses"])
	}
	religionOptions, _ := options["religions"].([]any)
	found := map[string]bool{}
	for _, value := range religionOptions {
		found[value.(string)] = true
	}
	if !found["other"] || !found["prefer_not_to_say"] {
		t.Fatalf("religion options must include \"other\" and \"prefer not to say\": %v", religionOptions)
	}
}
