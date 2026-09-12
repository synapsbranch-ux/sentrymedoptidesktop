package server

import (
	"bytes"
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

// An image is at most 8 MB: enough for a phone photograph of a frame, small
// enough that a gallery still loads over the clinic's own network.
const maxImageBytes = 8 << 20

// imageOwners maps what an image can be attached to onto the query that proves
// the thing exists. Anything not named here cannot be given an image at all.
var imageOwners = map[string]string{
	"inventory_item": "SELECT COUNT(*) FROM inventory_items WHERE id=? AND archived_at IS NULL",
	"lab_order":      "SELECT COUNT(*) FROM lab_orders WHERE id=?",
	// A provider's logo/photos reuse this same gallery rather than a
	// dedicated single-logo column — one fewer upload path to maintain.
	"payer": "SELECT COUNT(*) FROM payers WHERE id=?",
}

func (s *Server) registerImageRoutes(r chi.Router) {
	r.Get("/images", s.handleImagesList)
	r.Post("/images", s.handleImageUpload)
	r.Get("/images/{id}/content", s.handleImageContent)
	r.Delete("/images/{id}", s.handleImageDelete)
}

func (s *Server) imageOwnerExists(r *http.Request, entityType, entityID string) (bool, error) {
	query, known := imageOwners[entityType]
	if !known || strings.TrimSpace(entityID) == "" {
		return false, nil
	}
	var count int
	if err := s.db.QueryRowContext(r.Context(), query, entityID).Scan(&count); err != nil {
		return false, err
	}
	return count == 1, nil
}

func (s *Server) handleImagesList(w http.ResponseWriter, r *http.Request) {
	entityType, entityID := r.URL.Query().Get("entityType"), r.URL.Query().Get("entityId")
	if _, known := imageOwners[entityType]; !known || strings.TrimSpace(entityID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "An entity type and id are required.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT i.id,i.display_name,i.media_type,i.size_bytes,COALESCE(i.caption,''),i.sort_order,i.created_at,u.display_name
		FROM entity_images i JOIN users u ON u.id=i.created_by WHERE i.entity_type=? AND i.entity_id=? ORDER BY i.sort_order, i.created_at`, entityType, entityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_LIST_FAILED", "Could not load images.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, displayName, mediaType, caption, createdAt, uploadedBy string
		var size int64
		var order int
		if err := rows.Scan(&id, &displayName, &mediaType, &size, &caption, &order, &createdAt, &uploadedBy); err != nil {
			writeError(w, http.StatusInternalServerError, "IMAGE_LIST_FAILED", "Could not load images.")
			return
		}
		items = append(items, map[string]any{"id": id, "displayName": displayName, "mediaType": mediaType, "sizeBytes": size, "caption": caption, "sortOrder": order, "createdAt": createdAt, "uploadedBy": uploadedBy})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleImageUpload stores one picture against a stock item or a lab order. The
// media type is taken from the file's own bytes, never from the name or the
// header the client sent, so a renamed executable cannot be stored as a JPEG.
func (s *Server) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxImageBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", "An image must be 8 MB or smaller.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	entityType, entityID := r.FormValue("entityType"), r.FormValue("entityId")
	owned, err := s.imageOwnerExists(r, entityType, entityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_OWNER_CHECK_FAILED", "Could not check what this image belongs to.")
		return
	}
	if !owned {
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_OWNER_NOT_FOUND", "Attach the image to an existing stock item or lab order.")
		return
	}
	var existing int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM entity_images WHERE entity_type=? AND entity_id=?", entityType, entityID).Scan(&existing)
	if existing >= 12 {
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_LIMIT_REACHED", "This record already has 12 images. Remove one before adding another.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "FILE_REQUIRED", "Choose an image to upload.")
		return
	}
	defer file.Close()
	prefix := make([]byte, 512)
	prefixLength, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "The image could not be read.")
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
	path := filepath.Join(s.config.DataDir, "images", storageName)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_STORE_FAILED", "Could not store the image.")
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(io.MultiReader(bytes.NewReader(prefix), file), maxImageBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > maxImageBytes {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "IMAGE_STORE_FAILED", "Could not store the image or it exceeds 8 MB.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	caption := strings.TrimSpace(r.FormValue("caption"))
	if len([]rune(caption)) > 200 {
		caption = string([]rune(caption)[:200])
	}
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO entity_images(id,entity_type,entity_id,storage_name,display_name,media_type,size_bytes,checksum_sha256,caption,sort_order,created_at,created_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, entityType, entityID, storageName, filepath.Base(header.Filename), detected, size, hex.EncodeToString(hash.Sum(nil)), nilIfEmpty(caption), existing, now, user.ID)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "IMAGE_METADATA_FAILED", "The image was not attached to the record.")
		return
	}
	s.audit(r.Context(), &user, "upload", "image", id, "Attached an image to a "+strings.ReplaceAll(entityType, "_", " "), "", "", r)
	s.broker.Publish(realtime.Event{Type: "image.attached", EntityType: entityType, EntityID: entityID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "displayName": filepath.Base(header.Filename), "mediaType": detected, "sizeBytes": size, "caption": caption, "createdAt": now})
}

func (s *Server) handleImageContent(w http.ResponseWriter, r *http.Request) {
	var storageName, mediaType string
	err := s.db.QueryRowContext(r.Context(), "SELECT storage_name,media_type FROM entity_images WHERE id=?", chi.URLParam(r, "id")).Scan(&storageName, &mediaType)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "IMAGE_NOT_FOUND", "That image was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_LOAD_FAILED", "Could not read the image.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	serveStoredFile(w, r, filepath.Join(s.config.DataDir, "images"), storageName)
}

// handleImageDelete removes the row and then the file. If the file is already
// gone the record still clears, so a missing file cannot leave an image nobody
// can remove.
func (s *Server) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var storageName, entityType, entityID string
	err := s.db.QueryRowContext(r.Context(), "SELECT storage_name,entity_type,entity_id FROM entity_images WHERE id=?", id).Scan(&storageName, &entityType, &entityID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "IMAGE_NOT_FOUND", "That image was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_DELETE_FAILED", "Could not remove the image.")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "DELETE FROM entity_images WHERE id=?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "IMAGE_DELETE_FAILED", "Could not remove the image.")
		return
	}
	_ = os.Remove(filepath.Join(s.config.DataDir, "images", storageName))
	user, _ := userFromContext(r.Context())
	s.audit(r.Context(), &user, "delete", "image", id, "Removed an image", "", "", r)
	s.broker.Publish(realtime.Event{Type: "image.attached", EntityType: entityType, EntityID: entityID})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}
