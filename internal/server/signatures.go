package server

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// D3: a doctor's signature. It belongs to exactly one user: it can only be set,
// replaced or removed through /me/signature, which resolves the owner from the
// session, and there is no route that applies one user's signature on behalf of
// another. A prescription stores a snapshot of the signature that was applied to
// it, so replacing a signature never alters an already-issued document.

const maxSignatureBytes = 2 << 20
const maxSignatureJSONBytes = (maxSignatureBytes*4)/3 + 4096

func signatureDir(dataDir string) string { return filepath.Join(dataDir, "signatures") }

func (s *Server) registerSignatureRoutes(r chi.Router) {
	r.Get("/me/signature", s.handleSignatureGet)
	r.Get("/me/signature/image", s.handleSignatureImage)
	r.Put("/me/signature", s.handleSignatureSave)
	r.Delete("/me/signature", s.handleSignatureDelete)
	r.Get("/prescriptions/{id}/signature", s.handlePrescriptionSignatureImage)
}

type signatureRecord struct {
	Method    string `json:"method"`
	MediaType string `json:"mediaType"`
	SizeBytes int64  `json:"sizeBytes"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *Server) loadSignature(r *http.Request, userID string) (signatureRecord, string, error) {
	var record signatureRecord
	var storage string
	err := s.db.QueryRowContext(r.Context(),
		"SELECT method,media_type,size_bytes,version,updated_at,storage_name FROM user_signatures WHERE user_id=?", userID).
		Scan(&record.Method, &record.MediaType, &record.SizeBytes, &record.Version, &record.UpdatedAt, &storage)
	return record, storage, err
}

func (s *Server) handleSignatureGet(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	record, _, err := s.loadSignature(r, user.ID)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"present": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_LOAD_FAILED", "Could not load your signature.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"present": true, "signature": record})
}

func (s *Server) handleSignatureImage(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	record, storage, err := s.loadSignature(r, user.ID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "SIGNATURE_NOT_FOUND", "You have not added a signature yet.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_LOAD_FAILED", "Could not load your signature.")
		return
	}
	serveSignatureFile(w, r, s.config.DataDir, storage, record.MediaType)
}

// The signature shown on an issued prescription is the snapshot recorded with
// that prescription. There is deliberately no route that returns an arbitrary
// user's current signature.
func (s *Server) handlePrescriptionSignatureImage(w http.ResponseWriter, r *http.Request) {
	var storage, mediaType string
	err := s.db.QueryRowContext(r.Context(),
		"SELECT COALESCE(signature_storage_name,''),COALESCE(signature_media_type,'') FROM prescriptions WHERE id=? AND archived_at IS NULL", chi.URLParam(r, "id")).
		Scan(&storage, &mediaType)
	if err == sql.ErrNoRows || (err == nil && storage == "") {
		writeError(w, http.StatusNotFound, "SIGNATURE_NOT_FOUND", "This prescription has no signature on file.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_LOAD_FAILED", "Could not load the signature.")
		return
	}
	serveSignatureFile(w, r, s.config.DataDir, storage, mediaType)
}

func serveSignatureFile(w http.ResponseWriter, r *http.Request, dataDir, storage, mediaType string) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, no-store")
	serveStoredFile(w, r, signatureDir(dataDir), storage)
}

// handleSignatureSave accepts both an uploaded image file and the PNG produced
// by the on-screen pad; the pad posts the same multipart form with method=drawn.
func (s *Server) handleSignatureSave(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		s.handleSignatureJSONSave(w, r, user)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSignatureBytes)
	if err := r.ParseMultipartForm(maxSignatureBytes); err != nil {
		// A request that actually exceeds the limit surfaces a *http.MaxBytesError
		// (Go 1.19+); anything else — most commonly a request that never reached
		// us as multipart/form-data at all — is a different failure and must not
		// be reported as "too large", which sends staff hunting for a smaller
		// file when the real problem is the request itself.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "SIGNATURE_TOO_LARGE", "A signature image must be 2 MB or smaller.")
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_SIGNATURE_REQUEST", "Could not read the signature upload.")
		return
	}
	method := r.FormValue("method")
	if method != "uploaded" && method != "drawn" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SIGNATURE_METHOD", "A signature is either uploaded or drawn.")
		return
	}
	file, _, err := r.FormFile("signature")
	if err != nil {
		writeError(w, http.StatusBadRequest, "SIGNATURE_REQUIRED", "Choose or draw a signature image.")
		return
	}
	defer file.Close()

	prefix := make([]byte, 512)
	length, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_SIGNATURE", "The signature image could not be read.")
		return
	}
	prefix = prefix[:length]
	// PNG keeps the transparent background the clinic wants on a printed form;
	// JPEG is accepted because that is what a phone camera produces.
	detected := http.DetectContentType(prefix)
	if detected != "image/png" && detected != "image/jpeg" {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_SIGNATURE_TYPE", "A signature must be a PNG (preferred, keeps a transparent background) or a JPEG.")
		return
	}

	content, err := io.ReadAll(io.LimitReader(io.MultiReader(bytes.NewReader(prefix), file), maxSignatureBytes+1))
	if err != nil || len(content) > maxSignatureBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "SIGNATURE_STORE_FAILED", "Could not store the signature or it exceeds 2 MB.")
		return
	}
	s.storeSignature(w, r, user, method, content, detected)
}

func (s *Server) handleSignatureJSONSave(w http.ResponseWriter, r *http.Request, user AuthUser) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSignatureJSONBytes)
	var input struct {
		Method string `json:"method"`
		Data   string `json:"data"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Method == "" || input.Data == "" {
		writeError(w, http.StatusBadRequest, "INVALID_SIGNATURE_REQUEST", "Could not read the signature upload.")
		return
	}
	if input.Method != "uploaded" && input.Method != "drawn" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SIGNATURE_METHOD", "A signature is either uploaded or drawn.")
		return
	}
	content, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SIGNATURE", "The signature image could not be read.")
		return
	}
	if len(content) > maxSignatureBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "SIGNATURE_TOO_LARGE", "A signature image must be 2 MB or smaller.")
		return
	}
	detected := http.DetectContentType(content)
	if detected != "image/png" && detected != "image/jpeg" {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_SIGNATURE_TYPE", "A signature must be a PNG (preferred, keeps a transparent background) or a JPEG.")
		return
	}
	s.storeSignature(w, r, user, input.Method, content, detected)
}

func (s *Server) storeSignature(w http.ResponseWriter, r *http.Request, user AuthUser, method string, content []byte, detected string) {
	if err := os.MkdirAll(signatureDir(s.config.DataDir), 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_STORE_FAILED", "Could not prepare signature storage.")
		return
	}
	extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg"}[detected]
	storageName := uuid.NewString() + extension
	path := filepath.Join(signatureDir(s.config.DataDir), storageName)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_STORE_FAILED", "Could not store the signature.")
		return
	}
	hash := sha256.Sum256(content)
	size, copyErr := output.Write(content)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > maxSignatureBytes {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "SIGNATURE_STORE_FAILED", "Could not store the signature or it exceeds 2 MB.")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var previous string
	_ = s.db.QueryRowContext(r.Context(), "SELECT storage_name FROM user_signatures WHERE user_id=?", user.ID).Scan(&previous)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO user_signatures(user_id,method,storage_name,media_type,size_bytes,checksum_sha256,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET method=excluded.method,storage_name=excluded.storage_name,media_type=excluded.media_type,size_bytes=excluded.size_bytes,checksum_sha256=excluded.checksum_sha256,version=user_signatures.version+1,updated_at=excluded.updated_at`,
		user.ID, method, storageName, detected, size, hex.EncodeToString(hash[:]), now, now)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "SIGNATURE_STORE_FAILED", "Could not save the signature.")
		return
	}
	// The old file is only removed once no issued prescription still points at it.
	s.removeUnreferencedSignature(r, previous, storageName)
	s.audit(r.Context(), &user, "update", "signature", user.ID, "Saved a "+method+" signature", "", "", r)
	record, _, _ := s.loadSignature(r, user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"present": true, "signature": record})
}

func (s *Server) handleSignatureDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var previous string
	err := s.db.QueryRowContext(r.Context(), "SELECT storage_name FROM user_signatures WHERE user_id=?", user.ID).Scan(&previous)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_DELETE_FAILED", "Could not remove the signature.")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "DELETE FROM user_signatures WHERE user_id=?", user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "SIGNATURE_DELETE_FAILED", "Could not remove the signature.")
		return
	}
	s.removeUnreferencedSignature(r, previous, "")
	s.audit(r.Context(), &user, "delete", "signature", user.ID, "Removed the stored signature", "", "", r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeUnreferencedSignature(r *http.Request, storage, replacement string) {
	if storage == "" || storage == replacement {
		return
	}
	var referenced int
	if s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM prescriptions WHERE signature_storage_name=?", storage).Scan(&referenced) != nil || referenced > 0 {
		return
	}
	var stillCurrent int
	if s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM user_signatures WHERE storage_name=?", storage).Scan(&stillCurrent) != nil || stillCurrent > 0 {
		return
	}
	_ = os.Remove(filepath.Join(signatureDir(s.config.DataDir), filepath.Base(storage)))
}

// signatureForIssuer returns the storage name and media type to stamp onto a
// prescription. It is called only with the signed-in doctor's own id, which is
// what keeps a signature from being applied by anyone but its owner.
func (s *Server) signatureForIssuer(r *http.Request, userID string) (string, string) {
	var storage, mediaType string
	if s.db.QueryRowContext(r.Context(), "SELECT storage_name,media_type FROM user_signatures WHERE user_id=?", userID).Scan(&storage, &mediaType) != nil {
		return "", ""
	}
	return storage, mediaType
}
