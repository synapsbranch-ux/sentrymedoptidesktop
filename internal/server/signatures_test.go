package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A 1x1 transparent PNG.
var signaturePNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
	0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

func signatureRequest(t *testing.T, method string, content []byte, filename string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("method", method); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("signature", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/me/signature", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

func TestSignatureCanBeUploadedOrDrawnAndBelongsToItsOwner(t *testing.T) {
	a := newTestApp(t)
	for _, method := range []string{"uploaded", "drawn"} {
		saved := a.requestRaw(signatureRequest(t, method, signaturePNG, "signature.png"), a.doctor)
		if saved.Code != http.StatusOK {
			t.Fatalf("save %s: %d %s", method, saved.Code, saved.Body.String())
		}
		record := decodeResponse[map[string]any](t, saved)
		signature, _ := record["signature"].(map[string]any)
		if signature["method"] != method || signature["mediaType"] != "image/png" {
			t.Fatalf("stored %+v", signature)
		}
	}

	// The nurse's own signature slot is separate and starts empty, so a saved
	// signature is never visible as somebody else's.
	nurse := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/me/signature", nil, a.nurse))
	if nurse["present"] != false {
		t.Fatalf("the nurse sees a signature that is not theirs: %v", nurse)
	}
	doctor := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/me/signature", nil, a.doctor))
	if doctor["present"] != true {
		t.Fatalf("the doctor's own signature is missing: %v", doctor)
	}

	image := a.request(http.MethodGet, "/api/v1/me/signature/image", nil, a.doctor)
	if image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("signature image: %d %s", image.Code, image.Header().Get("Content-Type"))
	}
	if image.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("signature image cache header = %q", image.Header().Get("Cache-Control"))
	}
	if anonymous := a.request(http.MethodGet, "/api/v1/me/signature/image", nil, nil); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous signature read = %d, want 401", anonymous.Code)
	}
}

func TestSignatureRejectsNonImagesAndUnknownMethods(t *testing.T) {
	a := newTestApp(t)
	if response := a.requestRaw(signatureRequest(t, "uploaded", []byte("%PDF-1.7 not an image"), "signature.pdf"), a.doctor); response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("pdf signature status = %d, want 415", response.Code)
	}
	if response := a.requestRaw(signatureRequest(t, "forged", signaturePNG, "signature.png"), a.doctor); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown method status = %d, want 422", response.Code)
	}
}

func TestPrescriptionRecordsWhoSignedItAndWhen(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Signed", "Prescription")

	unsigned := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "medication", "details": map[string]string{"medication": "Timolol", "dosage": "1 drop"},
	}, a.doctor)
	if unsigned.Code != http.StatusCreated {
		t.Fatalf("issue without a signature: %d %s", unsigned.Code, unsigned.Body.String())
	}
	if decodeResponse[map[string]any](t, unsigned)["signed"] != false {
		t.Fatal("a prescription issued before a signature exists must not claim to be signed")
	}
	unsignedID := decodeResponse[map[string]any](t, unsigned)["id"].(string)
	if image := a.request(http.MethodGet, "/api/v1/prescriptions/"+unsignedID+"/signature", nil, a.doctor); image.Code != http.StatusNotFound {
		t.Fatalf("unsigned prescription signature = %d, want 404", image.Code)
	}

	if saved := a.requestRaw(signatureRequest(t, "drawn", signaturePNG, "signature.png"), a.doctor); saved.Code != http.StatusOK {
		t.Fatalf("save signature: %d %s", saved.Code, saved.Body.String())
	}
	signed := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "medication", "details": map[string]string{"medication": "Latanoprost", "dosage": "1 drop"},
	}, a.doctor)
	if signed.Code != http.StatusCreated {
		t.Fatalf("issue with a signature: %d %s", signed.Code, signed.Body.String())
	}
	created := decodeResponse[map[string]any](t, signed)
	if created["signed"] != true || created["signedAt"] == "" {
		t.Fatalf("issued prescription did not record the signature: %v", created)
	}
	signedID := created["id"].(string)
	if image := a.request(http.MethodGet, "/api/v1/prescriptions/"+signedID+"/signature", nil, a.doctor); image.Code != http.StatusOK {
		t.Fatalf("signed prescription signature = %d", image.Code)
	}

	list := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/prescriptions?patientId="+patient.ID, nil, a.doctor))
	items, _ := list["items"].([]any)
	for _, item := range items {
		record, _ := item.(map[string]any)
		if record["id"] == signedID {
			if record["signed"] != true || record["signedBy"] == "" || record["signedAt"] == "" {
				t.Fatalf("list omitted who signed and when: %v", record)
			}
		}
	}
}

func TestReplacingASignatureDoesNotAlterAnAlreadyIssuedPrescription(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Snapshot", "Signature")
	if saved := a.requestRaw(signatureRequest(t, "drawn", signaturePNG, "signature.png"), a.doctor); saved.Code != http.StatusOK {
		t.Fatalf("save first signature: %d %s", saved.Code, saved.Body.String())
	}
	issued := a.request(http.MethodPost, "/api/v1/prescriptions", map[string]any{
		"patientId": patient.ID, "type": "medication", "details": map[string]string{"medication": "Timolol", "dosage": "1 drop"},
	}, a.doctor)
	if issued.Code != http.StatusCreated {
		t.Fatalf("issue: %d %s", issued.Code, issued.Body.String())
	}
	id := decodeResponse[map[string]any](t, issued)["id"].(string)

	// Replace, then delete, the doctor's current signature.
	if saved := a.requestRaw(signatureRequest(t, "uploaded", signaturePNG, "new.png"), a.doctor); saved.Code != http.StatusOK {
		t.Fatalf("replace signature: %d %s", saved.Code, saved.Body.String())
	}
	if removed := a.request(http.MethodDelete, "/api/v1/me/signature", nil, a.doctor); removed.Code != http.StatusNoContent {
		t.Fatalf("delete signature: %d", removed.Code)
	}
	// The issued document keeps the signature that was applied to it.
	if image := a.request(http.MethodGet, "/api/v1/prescriptions/"+id+"/signature", nil, a.doctor); image.Code != http.StatusOK {
		t.Fatalf("issued prescription lost its signature after the doctor changed theirs: %d", image.Code)
	}
	if current := decodeResponse[map[string]any](t, a.request(http.MethodGet, "/api/v1/me/signature", nil, a.doctor)); current["present"] != false {
		t.Fatalf("signature was not removed: %v", current)
	}
}

// Regression: the client's api.put() helper used to JSON.stringify() every
// body unconditionally, including a FormData signature upload — which
// serializes to "{}" and is sent as application/json, never reaching the
// server as multipart at all. ParseMultipartForm's error for "this isn't
// multipart" and its error for "the body exceeded the byte limit" used to be
// handled identically, so a doctor drawing a normal, small signature was told
// their (nonexistent) file was over 2 MB — a message that sent them looking
// for a problem that did not exist, while masking the real one (a client bug
// that meant no signature was ever actually sent). This pins both the fixed
// client behavior (api.test.ts) and this server-side distinction.
func TestSignatureSaveDistinguishesWrongContentTypeFromTooLarge(t *testing.T) {
	a := newTestApp(t)

	// Exactly what the pre-fix client bug produced: a PUT with a JSON body
	// instead of multipart/form-data, for an ordinary, small signature.
	jsonRequest := httptest.NewRequest(http.MethodPut, "/api/v1/me/signature", bytes.NewBufferString("{}"))
	jsonRequest.Header.Set("Content-Type", "application/json")
	jsonRequest.RemoteAddr = "127.0.0.1:1234"
	response := a.requestRaw(jsonRequest, a.doctor)

	if response.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("a non-multipart request was reported as too large, which is not what happened: %d %s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("non-multipart signature save = %d, want 400: %s", response.Code, response.Body.String())
	}
	body := decodeResponse[map[string]any](t, response)
	if body["code"] == "SIGNATURE_TOO_LARGE" {
		t.Fatalf("error code still misreports content-type mismatch as size: %v", body)
	}

	// A genuinely oversized multipart upload must still be reported as too large.
	oversized := append(append([]byte{}, signaturePNG...), make([]byte, maxSignatureBytes+1024)...)
	tooLarge := a.requestRaw(signatureRequest(t, "uploaded", oversized, "signature.png"), a.doctor)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("genuinely oversized signature = %d, want 413", tooLarge.Code)
	}
	if decodeResponse[map[string]any](t, tooLarge)["code"] != "SIGNATURE_TOO_LARGE" {
		t.Fatalf("oversized signature did not report SIGNATURE_TOO_LARGE: %s", tooLarge.Body.String())
	}

	// And a normal, small signature must still save.
	if saved := a.requestRaw(signatureRequest(t, "drawn", signaturePNG, "signature.png"), a.doctor); saved.Code != http.StatusOK {
		t.Fatalf("ordinary signature save: %d %s", saved.Code, saved.Body.String())
	}
}
