package server

import (
	"net/http"
	"testing"
)

// startNurseEncounter opens a consultation the way the waiting room does: the
// nurse starts it, so it begins in the pre-test stage.
func (a *testApp) startNurseEncounter(patientID string) (string, int) {
	a.t.Helper()
	response := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patientID, "visitReason": "Eye examination"}, a.nurse)
	if response.Code != http.StatusCreated {
		a.t.Fatalf("start consultation: %d %s", response.Code, response.Body.String())
	}
	created := decodeResponse[map[string]any](a.t, response)
	if created["workflowStage"] != stagePreTest {
		a.t.Fatalf("a nurse-started consultation begins at %v, want %s", created["workflowStage"], stagePreTest)
	}
	return created["id"].(string), int(created["version"].(float64))
}

func (a *testApp) encounter(id string) map[string]any {
	a.t.Helper()
	response := a.request(http.MethodGet, "/api/v1/encounters/"+id, nil, a.doctor)
	if response.Code != http.StatusOK {
		a.t.Fatalf("load consultation: %d %s", response.Code, response.Body.String())
	}
	return decodeResponse[map[string]any](a.t, response)
}

func TestCompletingThePretestRoutesIntoTheDoctorExam(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Routed", "Forward")
	encounterID, _ := a.startNurseEncounter(patient.ID)

	saved := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/pretest", map[string]any{
		"chiefComplaint": "Blurred vision", "visualAcuity": map[string]any{"odDistanceVA": "20/40"}, "complete": true, "version": 1,
	}, a.nurse)
	if saved.Code != http.StatusOK {
		t.Fatalf("complete pre-test: %d %s", saved.Code, saved.Body.String())
	}
	if decodeResponse[map[string]any](t, saved)["workflowStage"] != stageDoctorExam {
		t.Fatal("completing the pre-test did not report the doctor's exam as the next stage")
	}
	if stage := a.encounter(encounterID)["workflowStage"]; stage != stageDoctorExam {
		t.Fatalf("consultation stage after pre-test = %v, want %s", stage, stageDoctorExam)
	}
}

func TestPretestCanBeSkippedWhenTheClinicAllowsIt(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Skipped", "Pretest")
	encounterID, _ := a.startNurseEncounter(patient.ID)

	skipped := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/pretest/skip", map[string]any{"reason": "Patient seen immediately", "version": 1}, a.nurse)
	if skipped.Code != http.StatusOK {
		t.Fatalf("skip pre-test: %d %s", skipped.Code, skipped.Body.String())
	}
	detail := a.encounter(encounterID)
	if detail["workflowStage"] != stageDoctorExam {
		t.Fatalf("skipping did not route to the doctor's exam: %v", detail["workflowStage"])
	}
	pretest, _ := detail["pretest"].(map[string]any)
	if pretest["skippedAt"] == "" || pretest["skipReason"] != "Patient seen immediately" {
		t.Fatalf("the skip was not recorded on the pre-test: %v", pretest)
	}
}

func TestPretestSkipIsRefusedWhenTheClinicRequiresIt(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.server.db.Exec(`UPDATE settings SET value_json=json_set(value_json,'$.pretestPolicy','required') WHERE key='clinical'`); err != nil {
		t.Fatal(err)
	}
	patient := a.createPatient(a.doctor, "Required", "Pretest")
	encounterID, _ := a.startNurseEncounter(patient.ID)

	skipped := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/pretest/skip", map[string]any{"version": 1}, a.nurse)
	if skipped.Code != http.StatusForbidden {
		t.Fatalf("skip under a required policy = %d, want 403", skipped.Code)
	}
	if stage := a.encounter(encounterID)["workflowStage"]; stage != stagePreTest {
		t.Fatalf("a refused skip moved the consultation to %v", stage)
	}
}

func TestPretestSkipIsRejectedOnAStaleVersion(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Stale", "Skip")
	encounterID, _ := a.startNurseEncounter(patient.ID)
	first := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/pretest/skip", map[string]any{"version": 1}, a.nurse)
	if first.Code != http.StatusOK {
		t.Fatalf("first skip: %d %s", first.Code, first.Body.String())
	}
	second := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/pretest/skip", map[string]any{"version": 1}, a.nurse)
	if second.Code != http.StatusConflict {
		t.Fatalf("repeat skip at a stale version = %d, want 409", second.Code)
	}
}

func TestFinalizeWarnsAboutAnEmptyRecordButDoesNotBlockIt(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Warned", "Finalize")
	response := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Exam"}, a.doctor)
	if response.Code != http.StatusCreated {
		t.Fatalf("start consultation: %d %s", response.Code, response.Body.String())
	}
	encounterID := decodeResponse[map[string]any](t, response)["id"].(string)

	warned := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if warned.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unconfirmed finalize = %d, want 422", warned.Code)
	}
	body := decodeResponse[map[string]any](t, warned)
	if body["code"] != "FINALIZE_WARNINGS" {
		t.Fatalf("unconfirmed finalize returned %v, want FINALIZE_WARNINGS", body["code"])
	}
	details, _ := body["details"].(map[string]any)
	warnings, _ := details["warnings"].([]any)
	found := map[string]bool{}
	for _, item := range warnings {
		warning, _ := item.(map[string]any)
		found[warning["code"].(string)] = true
	}
	for _, code := range []string{"NO_DIAGNOSIS", "NO_PRESCRIPTION", "NO_EXAMINATION_SECTION"} {
		if !found[code] {
			t.Fatalf("warning %s is missing from %v", code, warnings)
		}
	}

	// The doctor confirms and the consultation signs, with no diagnosis entered.
	confirmed := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/finalize", map[string]any{"version": 1, "acknowledgeWarnings": true}, a.doctor)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirmed finalize = %d %s", confirmed.Code, confirmed.Body.String())
	}
	if status := a.encounter(encounterID)["status"]; status != "finalized" {
		t.Fatalf("consultation status after confirmed finalize = %v", status)
	}
}

func TestFinalizeNeedsNoConfirmationWhenNothingIsMissing(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Complete", "Finalize")
	response := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": patient.ID, "visitReason": "Exam"}, a.doctor)
	encounterID := decodeResponse[map[string]any](t, response)["id"].(string)
	if code := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/diagnoses", map[string]any{"diagnosis": "Myopia, bilateral", "code": "H52.13", "laterality": "OU", "primary": true}, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("add diagnosis: %d", code)
	}
	if code := a.request(http.MethodPut, "/api/v1/encounters/"+encounterID+"/sections/subjective_refraction", map[string]any{"data": map[string]any{"odSphere": "-1.00"}, "version": 0}, a.doctor).Code; code != http.StatusOK {
		t.Fatalf("save section: %d", code)
	}
	if code := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{"patientId": patient.ID, "encounterId": encounterID, "type": "spectacle", "od": map[string]string{"sphere": "-1.00"}}, a.doctor).Code; code != http.StatusCreated {
		t.Fatalf("issue prescription: %d", code)
	}
	detail := a.encounter(encounterID)
	if warnings, _ := detail["finalizeWarnings"].([]any); len(warnings) != 0 {
		t.Fatalf("a complete consultation should raise no warnings, got %v", warnings)
	}
	finalized := a.request(http.MethodPost, "/api/v1/encounters/"+encounterID+"/finalize", map[string]any{"version": 1}, a.doctor)
	if finalized.Code != http.StatusOK {
		t.Fatalf("finalize without confirmation = %d %s", finalized.Code, finalized.Body.String())
	}
}
