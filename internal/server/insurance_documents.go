package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// Insurance documents cover two related workflows with one table: a
// provider's own required forms (payer_id set, nothing else) and a specific
// request to a specific patient or claim (patient_id/claim_id set). Both
// move through the same status lifecycle. See migration 029.
var insuranceDocumentStatuses = map[string]bool{"required": true, "requested": true, "received": true, "completed": true, "submitted": true, "rejected": true, "expired": true}

// A scanned multi-page form or a phone photo of a physical card/form.
const maxInsuranceDocumentBytes = 15 << 20

var insuranceDocumentExtensions = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "application/pdf": ".pdf"}

const insuranceDocumentColumns = `id,COALESCE(payer_id,''),COALESCE(patient_id,''),COALESCE(claim_id,''),COALESCE(patient_insurance_id,''),document_type,status,COALESCE(storage_name,''),COALESCE(display_name,''),COALESCE(media_type,''),COALESCE(size_bytes,0),COALESCE(existing_document_id,''),COALESCE(requested_by,''),COALESCE(requested_at,''),COALESCE(received_at,''),COALESCE(expiration_date,''),COALESCE(notes,''),version,created_at,updated_at`

func scanInsuranceDocument(row interface{ Scan(...any) error }) (map[string]any, error) {
	var id, payerID, patientID, claimID, patientInsuranceID, documentType, status, storageName, displayName, mediaType, existingDocumentID, requestedBy, requestedAt, receivedAt, expiration, notes, created, updated string
	var sizeBytes int64
	var version int
	if err := row.Scan(&id, &payerID, &patientID, &claimID, &patientInsuranceID, &documentType, &status, &storageName, &displayName, &mediaType, &sizeBytes, &existingDocumentID, &requestedBy, &requestedAt, &receivedAt, &expiration, &notes, &version, &created, &updated); err != nil {
		return nil, err
	}
	expired := expiration != "" && expiration < time.Now().UTC().Format("2006-01-02") && status != "submitted" && status != "completed" && status != "rejected"
	displayStatus := status
	if expired {
		displayStatus = "expired"
	}
	return map[string]any{
		"id": id, "payerId": payerID, "patientId": patientID, "claimId": claimID, "patientInsuranceId": patientInsuranceID,
		"documentType": documentType, "status": displayStatus, "hasFile": storageName != "" || existingDocumentID != "", "displayName": displayName, "mediaType": mediaType, "sizeBytes": sizeBytes,
		"existingDocumentId": existingDocumentID, "requestedBy": requestedBy, "requestedAt": requestedAt, "receivedAt": receivedAt,
		"expirationDate": expiration, "notes": notes, "version": version, "createdAt": created, "updatedAt": updated,
	}, nil
}

// listInsuranceDocuments is used both by the general list endpoint and by
// nested reads (a claim's own documents on its detail view). filterColumn is
// always a literal from this file, never user input.
func (s *Server) listInsuranceDocuments(ctx context.Context, filterColumn, filterValue string) ([]map[string]any, error) {
	if filterValue == "" {
		return []map[string]any{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+insuranceDocumentColumns+` FROM insurance_documents WHERE `+filterColumn+`=? AND archived_at IS NULL ORDER BY created_at DESC`, filterValue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item, err := scanInsuranceDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) handleInsuranceDocumentsList(w http.ResponseWriter, r *http.Request) {
	where, args := "archived_at IS NULL", []any{}
	for column, param := range map[string]string{"payer_id": "payerId", "patient_id": "patientId", "claim_id": "claimId", "patient_insurance_id": "patientInsuranceId", "status": "status"} {
		if value := strings.TrimSpace(r.URL.Query().Get(param)); value != "" {
			where += " AND " + column + "=?"
			args = append(args, value)
		}
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+insuranceDocumentColumns+` FROM insurance_documents WHERE `+where+` ORDER BY created_at DESC LIMIT 500`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENTS_FAILED", "Could not load insurance documents.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item, err := scanInsuranceDocument(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENTS_FAILED", "Could not load insurance documents.")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type insuranceDocumentPayload struct {
	PayerID, PatientID, ClaimID, PatientInsuranceID string
	DocumentType                                    string
	Status                                          string
	ExpirationDate, Notes                           string
	ExistingDocumentID                              string
}

// handleInsuranceDocumentCreate records a requirement or a request — "we
// need this form from this patient" — before any file necessarily exists.
// An existing document already on file can be associated directly via
// ExistingDocumentID instead of asking staff to upload a second copy.
func (s *Server) handleInsuranceDocumentCreate(w http.ResponseWriter, r *http.Request) {
	var in insuranceDocumentPayload
	if decodeJSON(r, &in) != nil || strings.TrimSpace(in.DocumentType) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A document type is required.")
		return
	}
	if in.Status == "" {
		in.Status = "required"
	}
	if !insuranceDocumentStatuses[in.Status] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_STATUS", "That is not a valid document status.")
		return
	}
	if in.PayerID == "" && in.PatientID == "" && in.ClaimID == "" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_REQUEST", "An insurance document needs a provider, a patient or a claim.")
		return
	}
	for table, id := range map[string]string{"payers": in.PayerID, "patients": in.PatientID, "insurance_claims": in.ClaimID, "patient_insurance": in.PatientInsuranceID} {
		if id == "" {
			continue
		}
		var exists int
		if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM "+table+" WHERE id=?", id).Scan(&exists); err != nil || exists != 1 {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_REFERENCE", "One of the referenced records was not found.")
			return
		}
	}
	if in.ExistingDocumentID != "" {
		var exists int
		if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM documents WHERE id=? AND archived_at IS NULL", in.ExistingDocumentID).Scan(&exists); err != nil || exists != 1 {
			writeError(w, http.StatusUnprocessableEntity, "DOCUMENT_NOT_FOUND", "That existing document was not found.")
			return
		}
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var requestedAt any
	if in.Status == "requested" {
		requestedAt = now
	}
	receivedAt := ""
	if in.ExistingDocumentID != "" && in.Status == "required" {
		// Associating an existing document is itself receiving it.
		in.Status = "received"
	}
	if in.Status == "received" {
		receivedAt = now
	}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO insurance_documents(id,payer_id,patient_id,claim_id,patient_insurance_id,document_type,status,existing_document_id,requested_by,requested_at,received_at,expiration_date,notes,version,created_at,updated_at,created_by,updated_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?)`,
		id, nilIfEmpty(in.PayerID), nilIfEmpty(in.PatientID), nilIfEmpty(in.ClaimID), nilIfEmpty(in.PatientInsuranceID), strings.TrimSpace(in.DocumentType), in.Status, nilIfEmpty(in.ExistingDocumentID), user.ID, requestedAt, nilIfEmpty(receivedAt), nilIfEmpty(in.ExpirationDate), nilIfEmpty(in.Notes), now, now, user.ID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENT_CREATE_FAILED", "Could not record the document.")
		return
	}
	s.audit(r.Context(), &user, "create", "insurance_document", id, "Recorded insurance document requirement: "+in.DocumentType, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_document", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": in.Status, "version": 1})
}

// handleInsuranceDocumentUpload attaches (or replaces) the file for a
// document row that is required or requested — moving it to RECEIVED — using
// the same content-sniffed, checksummed, UUID-named storage pattern as
// images.go and the patient documents feature.
func (s *Server) handleInsuranceDocumentUpload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var previousStorage string
	if err := s.db.QueryRowContext(r.Context(), "SELECT COALESCE(storage_name,'') FROM insurance_documents WHERE id=? AND archived_at IS NULL", id).Scan(&previousStorage); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "INSURANCE_DOCUMENT_NOT_FOUND", "That document was not found.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENT_UPLOAD_FAILED", "Could not read the document.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxInsuranceDocumentBytes+(1<<20))
	if err := r.ParseMultipartForm(maxInsuranceDocumentBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "A form or scan must be 15 MB or smaller.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "FILE_REQUIRED", "Choose a file to upload.")
		return
	}
	defer file.Close()
	prefix := make([]byte, 512)
	prefixLength, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "The file could not be read.")
		return
	}
	prefix = prefix[:prefixLength]
	detected := http.DetectContentType(prefix)
	extension, supported := insuranceDocumentExtensions[detected]
	if !supported {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE_TYPE", "Supported formats are JPEG, PNG, WebP and PDF.")
		return
	}
	storageName := uuid.NewString() + extension
	path := filepath.Join(s.config.DataDir, "insurance-documents", storageName)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "FILE_STORE_FAILED", "Could not store the file.")
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(io.MultiReader(bytes.NewReader(prefix), file), maxInsuranceDocumentBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > maxInsuranceDocumentBytes {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "FILE_STORE_FAILED", "Could not store the file or it exceeds 15 MB.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), `UPDATE insurance_documents SET storage_name=?,display_name=?,media_type=?,size_bytes=?,checksum_sha256=?,status='received',received_at=COALESCE(received_at,?),version=version+1,updated_at=?,updated_by=? WHERE id=? AND archived_at IS NULL`,
		storageName, filepath.Base(header.Filename), detected, size, hex.EncodeToString(hash.Sum(nil)), now, now, user.ID, id)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENT_UPLOAD_FAILED", "Could not attach the file.")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		_ = os.Remove(path)
		writeError(w, http.StatusNotFound, "INSURANCE_DOCUMENT_NOT_FOUND", "That document was not found.")
		return
	}
	if previousStorage != "" {
		_ = os.Remove(filepath.Join(s.config.DataDir, "insurance-documents", previousStorage))
	}
	action := "upload"
	if previousStorage != "" {
		action = "replace"
	}
	s.audit(r.Context(), &user, action, "insurance_document", id, "Attached a file to an insurance document", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_document", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "received"})
}

func (s *Server) handleInsuranceDocumentContent(w http.ResponseWriter, r *http.Request) {
	var storageName, mediaType, displayName string
	err := s.db.QueryRowContext(r.Context(), "SELECT COALESCE(storage_name,''),COALESCE(media_type,''),COALESCE(display_name,'file') FROM insurance_documents WHERE id=?", chi.URLParam(r, "id")).Scan(&storageName, &mediaType, &displayName)
	if err == sql.ErrNoRows || storageName == "" {
		writeError(w, http.StatusNotFound, "FILE_NOT_FOUND", "That file is unavailable.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "FILE_LOAD_FAILED", "Could not read the file.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(displayName, `"`, "")+`"`)
	}
	serveStoredFile(w, r, filepath.Join(s.config.DataDir, "insurance-documents"), storageName)
}

type insuranceDocumentStatusPayload struct {
	Status  string
	Notes   string
	Version int
}

func (s *Server) handleInsuranceDocumentStatus(w http.ResponseWriter, r *http.Request) {
	var in insuranceDocumentStatusPayload
	if decodeJSON(r, &in) != nil || !insuranceDocumentStatuses[in.Status] || in.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid status and current version are required.")
		return
	}
	id := chi.URLParam(r, "id")
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	notesClause, args := "", []any{in.Status}
	if strings.TrimSpace(in.Notes) != "" {
		notesClause = ",notes=?"
		args = append(args, in.Notes)
	}
	args = append(args, now, user.ID, id, in.Version)
	res, err := s.db.ExecContext(r.Context(), `UPDATE insurance_documents SET status=?`+notesClause+`,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENT_UPDATE_FAILED", "Could not update the document.")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This document changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "status", "insurance_document", id, "Changed insurance document status to "+in.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_document", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": in.Status, "version": in.Version + 1})
}

// handleInsuranceDocumentDelete archives the row and removes its own file
// (an associated existing document is never touched — it is owned by the
// patient's document record, not by this reference to it).
func (s *Server) handleInsuranceDocumentDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := r.URL.Query().Get("version")
	if version == "" {
		writeError(w, http.StatusBadRequest, "VERSION_REQUIRED", "Current document version is required.")
		return
	}
	var storageName string
	_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(storage_name,'') FROM insurance_documents WHERE id=?", id).Scan(&storageName)
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(r.Context(), "UPDATE insurance_documents SET archived_at=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL", now, now, user.ID, id, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INSURANCE_DOCUMENT_DELETE_FAILED", "Could not remove the document.")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This document changed since it was opened.")
		return
	}
	if storageName != "" {
		_ = os.Remove(filepath.Join(s.config.DataDir, "insurance-documents", storageName))
	}
	s.audit(r.Context(), &user, "delete", "insurance_document", id, "Removed an insurance document", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "insurance_document", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}

// ---- Insurance cards --------------------------------------------------------

var insuranceCardSides = map[string]bool{"front": true, "back": true}

func (s *Server) handleInsuranceCardsList(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,side,media_type,size_bytes,uploaded_at,uploaded_by FROM patient_insurance_cards WHERE patient_insurance_id=?", policyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INSURANCE_CARDS_FAILED", "Could not load insurance cards.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, side, mediaType, uploadedAt, uploadedBy string
		var size int64
		if err := rows.Scan(&id, &side, &mediaType, &size, &uploadedAt, &uploadedBy); err != nil {
			writeError(w, http.StatusInternalServerError, "INSURANCE_CARDS_FAILED", "Could not load insurance cards.")
			return
		}
		items = append(items, map[string]any{"id": id, "side": side, "mediaType": mediaType, "sizeBytes": size, "uploadedAt": uploadedAt, "uploadedBy": uploadedBy})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleInsuranceCardUpload uploads or replaces one named side of a policy's
// card in one step: the previous image and row for that side, if any, are
// removed inside the same transaction as the new one is written, so a
// replace can never leave two images claiming to be the same side.
func (s *Server) handleInsuranceCardUpload(w http.ResponseWriter, r *http.Request) {
	policyID, side := chi.URLParam(r, "id"), chi.URLParam(r, "side")
	if !insuranceCardSides[side] {
		writeError(w, http.StatusBadRequest, "INVALID_SIDE", "A card side must be front or back.")
		return
	}
	var policyExists int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM patient_insurance WHERE id=?", policyID).Scan(&policyExists); err != nil || policyExists != 1 {
		writeError(w, http.StatusUnprocessableEntity, "POLICY_NOT_FOUND", "That insurance policy was not found.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxInsuranceDocumentBytes+(1<<20))
	if err := r.ParseMultipartForm(maxInsuranceDocumentBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "An insurance card photo must be 15 MB or smaller.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "FILE_REQUIRED", "Choose or capture a photo of the card.")
		return
	}
	defer file.Close()
	prefix := make([]byte, 512)
	prefixLength, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "The image could not be read.")
		return
	}
	prefix = prefix[:prefixLength]
	extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	detected := http.DetectContentType(prefix)
	extension, supported := extensions[detected]
	if !supported {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_IMAGE_TYPE", "Supported image formats are JPEG, PNG and WebP.")
		return
	}
	storageName := uuid.NewString() + extension
	path := filepath.Join(s.config.DataDir, "insurance-cards", storageName)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_STORE_FAILED", "Could not store the image.")
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(io.MultiReader(bytes.NewReader(prefix), file), maxInsuranceDocumentBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > maxInsuranceDocumentBytes {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "IMAGE_STORE_FAILED", "Could not store the image or it exceeds 15 MB.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var previousStorage string
	var isReplace bool
	err = s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(r.Context(), "SELECT storage_name FROM patient_insurance_cards WHERE patient_insurance_id=? AND side=?", policyID, side).Scan(&previousStorage); err == nil {
			isReplace = true
			if _, err := tx.ExecContext(r.Context(), "DELETE FROM patient_insurance_cards WHERE patient_insurance_id=? AND side=?", policyID, side); err != nil {
				return err
			}
		} else if err != sql.ErrNoRows {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `INSERT INTO patient_insurance_cards(id,patient_insurance_id,side,storage_name,media_type,size_bytes,checksum_sha256,uploaded_at,uploaded_by) VALUES(?,?,?,?,?,?,?,?,?)`,
			id, policyID, side, storageName, detected, size, hex.EncodeToString(hash.Sum(nil)), now, user.ID)
		return err
	})
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "IMAGE_STORE_FAILED", "Could not save the insurance card.")
		return
	}
	if previousStorage != "" {
		_ = os.Remove(filepath.Join(s.config.DataDir, "insurance-cards", previousStorage))
	}
	action, description := "upload", "Uploaded "+side+" insurance card"
	if isReplace {
		action, description = "replace", "Replaced "+side+" insurance card"
	}
	s.audit(r.Context(), &user, action, "patient_insurance_card", policyID, description, "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: policyID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "side": side})
}

func (s *Server) handleInsuranceCardContent(w http.ResponseWriter, r *http.Request) {
	policyID, side := chi.URLParam(r, "id"), chi.URLParam(r, "side")
	var storageName, mediaType string
	err := s.db.QueryRowContext(r.Context(), "SELECT storage_name,media_type FROM patient_insurance_cards WHERE patient_insurance_id=? AND side=?", policyID, side).Scan(&storageName, &mediaType)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "CARD_NOT_FOUND", "That side of the card has not been uploaded.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CARD_LOAD_FAILED", "Could not read the insurance card.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Disposition", `attachment; filename="insurance-card-`+side+filepath.Ext(storageName)+`"`)
	}
	serveStoredFile(w, r, filepath.Join(s.config.DataDir, "insurance-cards"), storageName)
}

// handleInsuranceCardDelete removes one side of a card. This is doctor-only
// at the route level: an insurance card is a sensitive patient document, and
// its deletion (unlike a routine replace) should not be a normal-staff action.
func (s *Server) handleInsuranceCardDelete(w http.ResponseWriter, r *http.Request) {
	policyID, side := chi.URLParam(r, "id"), chi.URLParam(r, "side")
	var storageName string
	if err := s.db.QueryRowContext(r.Context(), "SELECT storage_name FROM patient_insurance_cards WHERE patient_insurance_id=? AND side=?", policyID, side).Scan(&storageName); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "CARD_NOT_FOUND", "That side of the card has not been uploaded.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "CARD_DELETE_FAILED", "Could not remove the insurance card.")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "DELETE FROM patient_insurance_cards WHERE patient_insurance_id=? AND side=?", policyID, side); err != nil {
		writeError(w, http.StatusInternalServerError, "CARD_DELETE_FAILED", "Could not remove the insurance card.")
		return
	}
	_ = os.Remove(filepath.Join(s.config.DataDir, "insurance-cards", storageName))
	user, _ := userFromContext(r.Context())
	s.audit(r.Context(), &user, "delete", "patient_insurance_card", policyID, "Removed "+side+" insurance card", "", "", r)
	s.broker.Publish(realtime.Event{Type: "insurance.changed", EntityType: "patient_insurance", EntityID: policyID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) registerInsuranceDocumentRoutes(r chi.Router) {
	r.Get("/insurance/documents", s.handleInsuranceDocumentsList)
	r.Post("/insurance/documents", s.handleInsuranceDocumentCreate)
	r.Post("/insurance/documents/{id}/upload", s.handleInsuranceDocumentUpload)
	r.Get("/insurance/documents/{id}/content", s.handleInsuranceDocumentContent)
	r.Patch("/insurance/documents/{id}/status", s.handleInsuranceDocumentStatus)
	r.With(s.requireDoctor).Delete("/insurance/documents/{id}", s.handleInsuranceDocumentDelete)
	r.Get("/patient-insurance/{id}/cards", s.handleInsuranceCardsList)
	r.Post("/patient-insurance/{id}/cards/{side}", s.handleInsuranceCardUpload)
	r.Get("/patient-insurance/{id}/cards/{side}/content", s.handleInsuranceCardContent)
	r.With(s.requireDoctor).Delete("/patient-insurance/{id}/cards/{side}", s.handleInsuranceCardDelete)
}
