package server

import (
	"net/http"
	"strings"
	"testing"
)

// Security properties of the surfaces added during the stabilisation round.
// These assert behaviour the brief requires ("a signature must never be
// applicable by anyone other than its owner") and the ordinary guards around
// newly exposed routes, so a later refactor cannot quietly relax them.

func TestSignatureOfOneUserIsNeverReachableAsAnother(t *testing.T) {
	a := newTestApp(t)
	if saved := a.requestRaw(signatureRequest(t, "drawn", signaturePNG, "signature.png"), a.doctor); saved.Code != http.StatusOK {
		t.Fatalf("doctor saves a signature: %d %s", saved.Code, saved.Body.String())
	}

	// The nurse's own slot stays empty and their image read 404s: /me is bound
	// to the session, so there is no parameter to point at another user.
	if state := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/me/signature", nil, a.nurse)); state["present"] != false {
		t.Fatalf("nurse sees a signature that is not theirs: %v", state)
	}
	if image := a.request(http.MethodGet, "/api/v1/me/signature/image", nil, a.nurse); image.Code != http.StatusNotFound {
		t.Fatalf("nurse reading a signature image = %d, want 404", image.Code)
	}

	// The nurse saving one must not touch the doctor's.
	if saved := a.requestRaw(signatureRequest(t, "uploaded", signaturePNG, "nurse.png"), a.nurse); saved.Code != http.StatusOK {
		t.Fatalf("nurse saves their own signature: %d %s", saved.Code, saved.Body.String())
	}
	doctor := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/me/signature", nil, a.doctor))
	signature, _ := doctor["signature"].(map[string]any)
	if signature["method"] != "drawn" {
		t.Fatalf("the doctor's signature changed when the nurse saved theirs: %v", signature)
	}

	// And the nurse deleting theirs must leave the doctor's in place.
	if removed := a.request(http.MethodDelete, "/api/v1/me/signature", nil, a.nurse); removed.Code != http.StatusNoContent {
		t.Fatalf("nurse deletes their own signature: %d", removed.Code)
	}
	if state := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/me/signature", nil, a.doctor)); state["present"] != true {
		t.Fatalf("the doctor's signature was removed by the nurse's delete: %v", state)
	}
}

func TestPrescriptionCarriesTheIssuersOwnSignatureOnly(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Signature", "Attribution")
	if saved := a.requestRaw(signatureRequest(t, "drawn", signaturePNG, "signature.png"), a.doctor); saved.Code != http.StatusOK {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	// A nurse cannot issue a prescription at all, so there is no path by which
	// the doctor's signature could be applied by somebody else.
	if response := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "medication", "details": map[string]string{"medication": "Timolol", "dosage": "1 drop"},
	}, a.nurse); response.Code != http.StatusForbidden {
		t.Fatalf("nurse issuing a prescription = %d, want 403", response.Code)
	}

	issued := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "medication", "details": map[string]string{"medication": "Timolol", "dosage": "1 drop"},
	}, a.doctor)
	if issued.Code != http.StatusCreated {
		t.Fatalf("issue: %d %s", issued.Code, issued.Body.String())
	}
	if signedBy := decodeResponse[map[string]any](t, issued)["signedBy"]; signedBy == "" {
		t.Fatal("the issued prescription did not record who signed it")
	}
}

func TestNewRoutesRefuseAnonymousAndUnprivilegedCallers(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Route", "Guards")

	anonymous := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/patients/filter-options"},
		{http.MethodGet, "/api/v1/catalogs/appointment_reason"},
		{http.MethodPost, "/api/v1/queue/walk-in"},
		{http.MethodGet, "/api/v1/me/signature"},
		{http.MethodGet, "/api/v1/me/signature/image"},
		{http.MethodPut, "/api/v1/me/signature"},
		{http.MethodDelete, "/api/v1/me/signature"},
		{http.MethodGet, "/api/v1/documents/any/content"},
	}
	for _, route := range anonymous {
		if response := a.request(route.method, route.path, map[string]any{}, nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s = %d, want 401", route.method, route.path, response.Code)
		}
	}

	// Writes a nurse must not perform on the new clinic-managed catalogs.
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/catalogs/appointment_reason"},
		{http.MethodPut, "/api/v1/catalogs/appointment_reason/seed-reason-eye-exam"},
		{http.MethodDelete, "/api/v1/catalogs/appointment_reason/seed-reason-eye-exam"},
	} {
		response := a.request(route.method, route.path, map[string]any{"label": "Nurse edit", "version": 1}, a.nurse)
		if response.Code != http.StatusForbidden {
			t.Fatalf("nurse %s %s = %d, want 403", route.method, route.path, response.Code)
		}
	}

	// A nurse legitimately registers walk-ins and reads reasons.
	if response := a.request(http.MethodPost, "/api/v1/queue/walk-in", map[string]any{"patientId": patient.ID}, a.nurse); response.Code != http.StatusCreated {
		t.Fatalf("nurse registering a walk-in = %d, want 201", response.Code)
	}
}

func TestPatientSearchSurvivesAdversarialInput(t *testing.T) {
	a := newTestApp(t)
	a.createPatient(a.doctor, "Robust", "Search")

	// FTS5 parses the MATCH argument as a query expression. Terms are quoted so
	// operators typed by a user are matched literally rather than executed, and
	// nothing here may reach the database as SQL.
	for _, query := range []string{
		`' OR 1=1 --`,
		`"; DROP TABLE patients; --`,
		`AND OR NOT NEAR`,
		`*`, `^`, `:`, `-`, `""`, `"`, `((((`, `a AND (b OR`,
		`patients MATCH 'x'`,
		strings.Repeat("a", 5000),
		strings.Repeat("term ", 200),
		"%_%", "\\", "\x00nul",
	} {
		response := a.request(http.MethodGet, "/api/v1/patients?q="+urlEscape(query), nil, a.doctor)
		if response.Code != http.StatusOK {
			t.Fatalf("search %q = %d %s; adversarial input must return results, not an error", query, response.Code, response.Body.String())
		}
	}

	// The table is still there and the ordinary search still works.
	if names := patientNames(a.searchPatients(t, "q=Robust")); len(names) != 1 {
		t.Fatalf("after adversarial input the search returned %v", names)
	}
}

func TestUploadsRefuseDisguisedAndOversizedContent(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Upload", "Guards")

	// A script renamed to an allowed extension must be rejected on its content.
	for _, upload := range []struct {
		filename string
		content  []byte
	}{
		{"payload.png", []byte("<?php system($_GET['c']); ?>")},
		{"payload.pdf", []byte("<html><script>alert(1)</script></html>")},
		{"payload.jpg", []byte("#!/bin/sh\nrm -rf /\n")},
	} {
		response := a.requestRaw(multipartDocumentRequest(t, patient.ID, upload.filename, upload.content), a.doctor)
		if response.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("document %s = %d, want 415", upload.filename, response.Code)
		}
	}
	// The same for a signature, which is rendered onto a printed prescription.
	if response := a.requestRaw(signatureRequest(t, "uploaded", []byte("<svg onload=alert(1)>"), "signature.png"), a.doctor); response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("SVG signature = %d, want 415: an inline SVG can carry script", response.Code)
	}

	// Size caps hold, so neither route can be used to fill the clinic's disk.
	oversizedSignature := append(append([]byte{}, signaturePNG...), make([]byte, maxSignatureBytes+1024)...)
	if response := a.requestRaw(signatureRequest(t, "uploaded", oversizedSignature, "signature.png"), a.doctor); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized signature = %d, want 413", response.Code)
	}
	oversizedDocument := append(append([]byte{}, pdfBytes...), make([]byte, (26<<20)+1024)...)
	if response := a.requestRaw(multipartDocumentRequest(t, patient.ID, "huge.pdf", oversizedDocument), a.doctor); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized document = %d, want 413", response.Code)
	}
}

func TestStoredFilesAreServedWithNosniffAndWithoutCaching(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Header", "Guards")
	created := a.requestRaw(multipartDocumentRequest(t, patient.ID, "scan.pdf", pdfBytes), a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)

	for _, path := range []string{"/api/v1/documents/" + id + "/content", "/api/v1/documents/" + id + "/download"} {
		response := a.request(http.MethodGet, path, nil, a.doctor)
		if response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s is missing nosniff; a browser could re-interpret patient content", path)
		}
		// Patient content must not be left in a shared cache.
		if response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s Cache-Control = %q, want private, no-store", path, response.Header().Get("Cache-Control"))
		}
	}
}

func urlEscape(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25", " ", "%20", "\"", "%22", "'", "%27", "#", "%23", "&", "%26",
		"+", "%2B", "\\", "%5C", "\x00", "%00", ";", "%3B", "?", "%3F",
	)
	return replacer.Replace(value)
}
