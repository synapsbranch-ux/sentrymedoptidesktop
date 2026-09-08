package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func multipartDocumentRequest(t *testing.T, patientID, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"patientId": patientID, "category": "patient_document"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

var (
	pdfBytes        = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\ntrailer\n")
	pngBytes        = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	tiffLittleBytes = append([]byte("II\x2a\x00"), bytes.Repeat([]byte{0x11}, 32)...)
	tiffBigBytes    = append([]byte("MM\x00\x2a"), bytes.Repeat([]byte{0x11}, 32)...)
)

func TestDocumentsCanBeViewedInlineAndDownloaded(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Viewer", "Document")

	for _, upload := range []struct {
		filename  string
		content   []byte
		mediaType string
	}{
		{"scan.pdf", pdfBytes, "application/pdf"},
		{"photo.png", pngBytes, "image/png"},
		{"scan.tiff", tiffLittleBytes, "image/tiff"},
		{"scan.tif", tiffBigBytes, "image/tiff"},
	} {
		created := a.requestRaw(multipartDocumentRequest(t, patient.ID, upload.filename, upload.content), a.doctor)
		if created.Code != http.StatusCreated {
			t.Fatalf("upload %s: %d %s", upload.filename, created.Code, created.Body.String())
		}
		document := decodeResponse[map[string]any](t, created)
		if document["mediaType"] != upload.mediaType {
			t.Fatalf("upload %s media type = %v, want %s", upload.filename, document["mediaType"], upload.mediaType)
		}
		id := document["id"].(string)

		inline := a.request(http.MethodGet, "/api/v1/documents/"+id+"/content", nil, a.doctor)
		if inline.Code != http.StatusOK {
			t.Fatalf("inline %s: %d %s", upload.filename, inline.Code, inline.Body.String())
		}
		if disposition := inline.Header().Get("Content-Disposition"); disposition[:6] != "inline" {
			t.Fatalf("inline %s disposition = %q, want inline so the viewer can preview it", upload.filename, disposition)
		}
		if inline.Header().Get("Content-Type") != upload.mediaType {
			t.Fatalf("inline %s content type = %q", upload.filename, inline.Header().Get("Content-Type"))
		}
		if !bytes.Equal(inline.Body.Bytes(), upload.content) {
			t.Fatalf("inline %s returned altered bytes", upload.filename)
		}

		download := a.request(http.MethodGet, "/api/v1/documents/"+id+"/download", nil, a.doctor)
		if download.Code != http.StatusOK {
			t.Fatalf("download %s: %d", upload.filename, download.Code)
		}
		if disposition := download.Header().Get("Content-Disposition"); disposition[:10] != "attachment" {
			t.Fatalf("download %s disposition = %q, want attachment", upload.filename, disposition)
		}
	}
}

func TestDocumentContentRequiresAuthentication(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Private", "Document")
	created := a.requestRaw(multipartDocumentRequest(t, patient.ID, "scan.pdf", pdfBytes), a.doctor)
	if created.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", created.Code, created.Body.String())
	}
	id := decodeResponse[map[string]any](t, created)["id"].(string)
	anonymous := a.request(http.MethodGet, "/api/v1/documents/"+id+"/content", nil, nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous inline read status = %d, want 401", anonymous.Code)
	}
}

func TestDocumentUploadRejectsMismatchedContent(t *testing.T) {
	a := newTestApp(t)
	patient := a.createPatient(a.doctor, "Reject", "Document")
	// A .tiff extension over PNG bytes must not be accepted as a scan.
	mismatch := a.requestRaw(multipartDocumentRequest(t, patient.ID, "fake.tiff", pngBytes), a.doctor)
	if mismatch.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("mismatched upload status = %d, want 415", mismatch.Code)
	}
	executable := a.requestRaw(multipartDocumentRequest(t, patient.ID, "tool.exe", []byte("MZ\x90\x00")), a.doctor)
	if executable.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("executable upload status = %d, want 415", executable.Code)
	}
}
