package server

import (
	"net/http"
	"reflect"
	"testing"
)

func TestPatientPartialUpdatesPreserveEveryOtherField(t *testing.T) {
	a := newTestApp(t)
	fields := map[string]any{
		"firstName": "Alice", "middleName": "Marie", "lastName": "Joseph", "preferredName": "Ali",
		"sex": "female", "dateOfBirth": "1990-02-28", "phone": "34567890", "alternatePhone": "34567891",
		"email": "alice@example.test", "address": "1 Clinic Road", "city": "Cap Haitien", "occupation": "Teacher",
		"employer": "School", "preferredLanguage": "ht", "communicationPreference": "phone", "referralSource": "Friend",
		"referringProvider": "Dr A", "civilStatus": "married", "religion": "other", "religionOther": "Personal belief",
		"notes": "Keep these notes", "tags": []any{"follow-up"},
	}
	created := a.request(http.MethodPost, "/api/v1/patients", fields, a.nurse)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	record := decodeResponse[map[string]any](t, created)
	path := "/api/v1/patients/" + record["id"].(string)
	for key, value := range fields {
		// Updating one field must never erase any of the other populated fields.
		res := a.request(http.MethodPut, path, map[string]any{key: value, "version": record["version"]}, a.nurse)
		if res.Code != 200 {
			t.Fatalf("update %s: %d %s", key, res.Code, res.Body.String())
		}
		record = decodeResponse[map[string]any](t, res)
		for name, want := range fields {
			if !reflect.DeepEqual(record[name], want) {
				t.Fatalf("updating %s changed %s: got %#v want %#v", key, name, record[name], want)
			}
		}
	}
	res := a.request(http.MethodPut, path, map[string]any{"sex": "male", "version": record["version"]}, a.nurse)
	if res.Code != 200 {
		t.Fatalf("gender update: %d %s", res.Code, res.Body.String())
	}
	changed := decodeResponse[patientRecord](t, res)
	if changed.Sex != "male" || changed.Notes != "Keep these notes" {
		t.Fatalf("gender update corrupted record: %+v", changed)
	}
	stale := a.request(http.MethodPut, path, map[string]any{"notes": "stale write", "version": record["version"]}, a.doctor)
	if stale.Code != 409 {
		t.Fatalf("stale write: %d", stale.Code)
	}
	cleared := a.request(http.MethodPut, path, map[string]any{"phone": nil, "dateOfBirth": nil, "tags": nil, "version": changed.Version}, a.nurse)
	if cleared.Code != 200 {
		t.Fatalf("clear nullable fields: %d %s", cleared.Code, cleared.Body.String())
	}
	final := decodeResponse[patientRecord](t, a.request(http.MethodGet, path, nil, a.doctor))
	if final.Phone != "" || final.DateOfBirth != "" || len(final.Tags) != 0 || final.Email != "alice@example.test" {
		t.Fatalf("nullable update: %+v", final)
	}
}

func TestPatientValidationConsistentForCreateAndUpdate(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.doctor, "Validation", "Patient")
	for _, invalid := range []map[string]any{{"sex": "invalid"}, {"dateOfBirth": "2026-02-31"}, {"dateOfBirth": "2999-01-01"}, {"email": "invalid"}, {"firstName": "   "}} {
		create := map[string]any{"firstName": "Valid", "lastName": "Patient"}
		update := map[string]any{"version": p.Version}
		for k, v := range invalid {
			create[k] = v
			update[k] = v
		}
		for _, tc := range []struct {
			method, path string
			body         map[string]any
		}{{http.MethodPost, "/api/v1/patients", create}, {http.MethodPut, "/api/v1/patients/" + p.ID, update}} {
			res := a.request(tc.method, tc.path, tc.body, a.doctor)
			if res.Code != 422 {
				t.Errorf("%s %v: %d %s", tc.method, invalid, res.Code, res.Body.String())
			}
		}
	}
	for _, body := range []map[string]any{{"version": p.Version, "sex": 12}, {"version": p.Version, "id": "override"}, {"sex": "male"}} {
		res := a.request(http.MethodPut, "/api/v1/patients/"+p.ID, body, a.doctor)
		if res.Code != 400 {
			t.Errorf("bad payload %v: %d", body, res.Code)
		}
	}
}
