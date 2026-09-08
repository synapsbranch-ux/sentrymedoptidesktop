package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestChartRevisionsTrackingAndPreviousByEye(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.nurse, "Chart", "History")
	createVisit := func(date string) string {
		response := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": p.ID, "visitReason": "Review"}, a.doctor)
		if response.Code != 201 {
			t.Fatalf("create: %d %s", response.Code, response.Body.String())
		}
		id := decodeResponse[map[string]any](t, response)["id"].(string)
		if _, err := a.server.db.ExecContext(context.Background(), "UPDATE encounters SET created_at=? WHERE id=?", date, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := createVisit("2025-01-01T10:00:00Z")
	middle := createVisit("2025-02-01T10:00:00Z")
	current := createVisit("2025-03-01T10:00:00Z")
	mark := chartMark{X: 20, Y: 30, Shape: "dot", Color: "#dc2626", Label: "Observation", Structure: "retina"}
	save := func(id, eye string, version int, marks []chartMark) []chartMark {
		response := a.request(http.MethodPut, "/api/v1/encounters/"+id+"/eye-diagrams/fundus/"+eye, map[string]any{"annotations": marks, "version": version, "schemaVersion": 2}, a.doctor)
		if response.Code != 200 && response.Code != 201 {
			t.Fatalf("save: %d %s", response.Code, response.Body.String())
		}
		return decodeResponse[struct {
			Annotations []chartMark `json:"annotations"`
		}](t, response).Annotations
	}
	marks := save(first, "OD", 0, []chartMark{mark})
	if marks[0].ID == "" || marks[0].TrackingID != marks[0].ID || marks[0].CoordinateSystem != "legacy-svg-percent-v1" {
		t.Fatalf("identity: %+v", marks[0])
	}
	marks[0].Label = "Reviewed"
	save(first, "OD", 1, marks)
	save(middle, "OS", 0, []chartMark{mark})
	response := a.request(http.MethodGet, "/api/v1/encounters/"+current+"/eye-diagrams", nil, a.nurse)
	if response.Code != 200 {
		t.Fatalf("get: %s", response.Body.String())
	}
	data := decodeResponse[struct {
		PreviousByChart map[string]map[string]struct {
			EncounterID string `json:"encounterId"`
			Date        string `json:"date"`
		} `json:"previousByChart"`
	}](t, response)
	if data.PreviousByChart["fundus"]["OD"].EncounterID != first || data.PreviousByChart["fundus"]["OS"].EncounterID != middle || data.PreviousByChart["fundus"]["OD"].Date != "2025-01-01T10:00:00Z" {
		t.Fatalf("previous: %+v", data)
	}
	tracked := marks[0]
	tracked.ID = uuid.NewString()
	save(current, "OD", 0, []chartMark{tracked})
	rejected := a.request(http.MethodPut, "/api/v1/encounters/"+current+"/eye-diagrams/fundus/OS", map[string]any{"annotations": []chartMark{tracked}, "version": 0}, a.doctor)
	if rejected.Code != 422 {
		t.Fatalf("wrong eye tracking: %d", rejected.Code)
	}
	stale := a.request(http.MethodPut, "/api/v1/encounters/"+first+"/eye-diagrams/fundus/OD", map[string]any{"annotations": marks, "version": 1}, a.doctor)
	if stale.Code != 409 {
		t.Fatalf("stale: %d", stale.Code)
	}
	history := a.request(http.MethodGet, "/api/v1/encounters/"+current+"/eye-diagrams/fundus/OD/history", nil, a.nurse)
	if history.Code != 200 {
		t.Fatalf("history: %d %s", history.Code, history.Body.String())
	}
	h := decodeResponse[struct {
		Items []struct {
			Version     int         `json:"version"`
			Annotations []chartMark `json:"annotations"`
		} `json:"items"`
	}](t, history)
	if len(h.Items) != 3 || h.Items[2].Annotations[0].Label != "Observation" || h.Items[1].Annotations[0].ID != h.Items[2].Annotations[0].ID {
		t.Fatalf("history: %s", history.Body.String())
	}
	if history.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("history must not be cached")
	}
	if unauth := a.request(http.MethodGet, "/api/v1/encounters/"+current+"/eye-diagrams/fundus/OD/history", nil, nil); unauth.Code != 401 {
		t.Fatal("history requires authentication")
	}
	if _, err := a.server.db.ExecContext(context.Background(), "UPDATE encounters SET status='finalized' WHERE id=?", current); err != nil {
		t.Fatal(err)
	}
	locked := a.request(http.MethodPut, "/api/v1/encounters/"+current+"/eye-diagrams/fundus/OD", map[string]any{"annotations": []chartMark{tracked}, "version": 1}, a.doctor)
	if locked.Code != 423 {
		t.Fatalf("locked: %d", locked.Code)
	}
	// The DB itself rejects writes, even if finalization occurs after a handler's read.
	if _, err := a.server.db.ExecContext(context.Background(), "UPDATE eye_diagrams SET notes='unsafe',version=version+1 WHERE encounter_id=?", current); err == nil {
		t.Fatal("finalized DB write was allowed")
	}
	if _, err := a.server.db.ExecContext(context.Background(), "UPDATE clinical_chart_revisions SET notes='unsafe'"); err == nil {
		t.Fatal("revision mutation was allowed")
	}
	if _, err := a.server.db.ExecContext(context.Background(), "DELETE FROM clinical_chart_revisions"); err == nil {
		t.Fatal("revision deletion was allowed")
	}
}

func TestChartIdentityAndStatusValidation(t *testing.T) {
	legacy := []chartMark{{X: 12.5, Y: 30.1}}
	identifyLegacyMarks("old-chart", legacy)
	again := []chartMark{{X: 12.5, Y: 30.1}}
	identifyLegacyMarks("old-chart", again)
	if legacy[0] != again[0] || legacy[0].X != 12.5 || legacy[0].Y != 30.1 {
		t.Fatal("legacy normalization must be stable and lossless")
	}
	if err := prepareChartIdentities([]chartMark{legacy[0], legacy[0]}); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	if err := prepareChartIdentities([]chartMark{{CoordinateSystem: "invented-3d-mm"}}); err == nil {
		t.Fatal("unsupported coordinates accepted")
	}
	a := newTestApp(t)
	p := a.createPatient(a.nurse, "Status", "Test")
	visit := a.request(http.MethodPost, "/api/v1/encounters", map[string]any{"patientId": p.ID}, a.doctor)
	id := decodeResponse[map[string]any](t, visit)["id"].(string)
	path := "/api/v1/encounters/" + id + "/eye-diagrams/anterior/OD"
	bad := a.request(http.MethodPut, path, map[string]any{"version": 0, "examStatus": "no_findings", "annotations": legacy}, a.doctor)
	if bad.Code != 422 {
		t.Fatalf("contradictory status: %d", bad.Code)
	}
	good := a.request(http.MethodPut, path, map[string]any{"version": 0, "examStatus": "not_examined", "annotations": []chartMark{}}, a.doctor)
	if good.Code != 201 {
		t.Fatalf("explicit status: %s", good.Body.String())
	}
	legacy[0].Shape = "dot"
	legacy[0].Color = "#dc2626"
	legacy[0].Structure = "cornea"
	updated := a.request(http.MethodPut, path, map[string]any{"version": 1, "annotations": legacy}, a.doctor)
	if updated.Code != 200 {
		t.Fatalf("legacy client: %s", updated.Body.String())
	}
	var status string
	if err := a.server.db.QueryRowContext(context.Background(), "SELECT exam_status FROM eye_diagrams WHERE encounter_id=?", id).Scan(&status); err != nil || status != "unspecified" {
		t.Fatalf("legacy client retained contradictory status: %s %v", status, err)
	}
}
