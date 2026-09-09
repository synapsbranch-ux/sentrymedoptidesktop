package server

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRestoresDocumentsSignaturesAndRetainsRecoveryCatalog(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.doctor, "Backup", "Files")
	upload := a.requestRaw(multipartDocumentRequest(t, p.ID, "scan.pdf", pdfBytes), a.doctor)
	if upload.Code != 201 {
		t.Fatal(upload.Body.String())
	}
	docID := decodeResponse[map[string]any](t, upload)["id"].(string)
	if signature := a.requestRaw(signatureRequest(t, "drawn", signaturePNG, "signature.png"), a.doctor); signature.Code != 200 {
		t.Fatal(signature.Body.String())
	}
	snapshot := a.request("POST", "/api/v1/backups", map[string]any{}, a.doctor)
	if snapshot.Code != 201 {
		t.Fatal(snapshot.Body.String())
	}
	record := decodeResponse[map[string]any](t, snapshot)
	if record["assetsChecksumSha256"] == "" {
		t.Fatal("no verified assets checksum")
	}
	// Simulate missing working files. The verified generation contains both.
	if err := os.RemoveAll(filepath.Join(a.server.config.DataDir, "documents")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(a.server.config.DataDir, "signatures")); err != nil {
		t.Fatal(err)
	}
	restore := a.request("POST", "/api/v1/backups/restore", map[string]any{"backupId": record["id"]}, a.doctor)
	if restore.Code != 200 {
		t.Fatal(restore.Body.String())
	}
	if res := a.request("GET", "/api/v1/auth/me", nil, a.doctor); res.Code != 401 {
		t.Fatal("old session survived")
	}
	a.doctor = a.login("doctor.dev", "Doctor-Development-Only-2026")
	doc := a.request("GET", "/api/v1/documents/"+docID+"/content", nil, a.doctor)
	if doc.Code != 200 || !bytes.Equal(doc.Body.Bytes(), pdfBytes) {
		t.Fatalf("document restore: %d", doc.Code)
	}
	signature := a.request("GET", "/api/v1/me/signature/image", nil, a.doctor)
	if signature.Code != 200 || !bytes.Equal(signature.Body.Bytes(), signaturePNG) {
		t.Fatalf("signature restore: %d", signature.Code)
	}
	var count int
	if err := a.server.db.QueryRow("SELECT count(*) FROM backup_records WHERE kind='pre_restore'").Scan(&count); err != nil || count < 1 {
		t.Fatal("safety generation disappeared from catalog")
	}
}

func TestTamperedBackupAssetCannotReplaceLiveData(t *testing.T) {
	a := newTestApp(t)
	p := a.createPatient(a.doctor, "Tamper", "Guard")
	upload := a.requestRaw(multipartDocumentRequest(t, p.ID, "scan.pdf", pdfBytes), a.doctor)
	if upload.Code != 201 {
		t.Fatal(upload.Body.String())
	}
	snapshot := a.request("POST", "/api/v1/backups", map[string]any{}, a.doctor)
	if snapshot.Code != 201 {
		t.Fatal(snapshot.Body.String())
	}
	record := decodeResponse[map[string]any](t, snapshot)
	files, err := filepath.Glob(record["path"].(string) + ".files/documents/*.pdf")
	if err != nil || len(files) != 1 {
		t.Fatalf("snapshot files: %v %v", files, err)
	}
	if err := os.WriteFile(files[0], []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	restore := a.request("POST", "/api/v1/backups/restore", map[string]any{"backupId": record["id"]}, a.doctor)
	if restore.Code != http.StatusInternalServerError {
		t.Fatalf("tampered restore: %d", restore.Code)
	}
	if res := a.request("GET", "/api/v1/patients/"+p.ID, nil, a.doctor); res.Code != 200 {
		t.Fatal("live data lost after rejected restore")
	}
}
