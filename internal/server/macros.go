package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

var macroCategories = map[string]bool{
	"general": true, "chief_complaint": true, "hpi": true,
	"assessment": true, "treatment_plan": true, "follow_up": true,
}

type macroPayload struct {
	Label    string `json:"label"`
	Category string `json:"category"`
	Body     string `json:"body"`
	Version  int    `json:"version"`
}

func (s *Server) registerMacroRoutes(r chi.Router) {
	r.Get("/macros", s.handleMacrosList)
	r.Post("/macros", s.handleMacroCreate)
	r.Put("/macros/{id}", s.handleMacroUpdate)
	r.Delete("/macros/{id}", s.handleMacroArchive)
}

func validateMacro(input *macroPayload) *APIError {
	input.Label = strings.TrimSpace(input.Label)
	input.Body = strings.TrimSpace(input.Body)
	if input.Category == "" {
		input.Category = "general"
	}
	if input.Label == "" || input.Body == "" {
		return &APIError{Code: "VALIDATION_ERROR", Message: "A template needs both a label and a body."}
	}
	if !macroCategories[input.Category] {
		return &APIError{Code: "INVALID_CATEGORY", Message: "Unknown template category."}
	}
	return nil
}

func (s *Server) handleMacrosList(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	query := "SELECT id,label,category,body,version,updated_at FROM clinical_macros WHERE archived_at IS NULL"
	args := []any{}
	if category != "" {
		query += " AND category=?"
		args = append(args, category)
	}
	query += " ORDER BY category,label COLLATE NOCASE"
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MACRO_LIST_FAILED", "Could not load templates.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, label, category, body, updatedAt string
		var version int
		if rows.Scan(&id, &label, &category, &body, &version, &updatedAt) == nil {
			items = append(items, map[string]any{"id": id, "label": label, "category": category, "body": body, "version": version, "updatedAt": updatedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleMacroCreate(w http.ResponseWriter, r *http.Request) {
	var input macroPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if apiErr := validateMacro(&input); apiErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, apiErr)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO clinical_macros(id,label,category,body,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?)`, id, input.Label, input.Category, input.Body, now, now, user.ID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MACRO_CREATE_FAILED", "Could not create template.")
		return
	}
	s.audit(r.Context(), &user, "create", "clinical_macro", id, "Created charting template "+input.Label, "", "", r)
	s.broker.Publish(realtime.Event{Type: "macro.changed", EntityType: "clinical_macro", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "version": 1})
}

func (s *Server) handleMacroUpdate(w http.ResponseWriter, r *http.Request) {
	var input macroPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Template version is required.")
		return
	}
	if apiErr := validateMacro(&input); apiErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, apiErr)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE clinical_macros SET label=?,category=?,body=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, input.Label, input.Category, input.Body, now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MACRO_UPDATE_FAILED", "Could not update template.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Template changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "clinical_macro", id, "Updated charting template "+input.Label, "", "", r)
	s.broker.Publish(realtime.Event{Type: "macro.changed", EntityType: "clinical_macro", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

func (s *Server) handleMacroArchive(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if version == "" {
		writeError(w, http.StatusBadRequest, "VERSION_REQUIRED", "Template version is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE clinical_macros SET archived_at=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, now, now, user.ID, id, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MACRO_ARCHIVE_FAILED", "Could not delete template.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Template changed since it was opened. Reload before deleting.")
		return
	}
	s.audit(r.Context(), &user, "archive", "clinical_macro", id, "Deleted charting template", "", "", r)
	s.broker.Publish(realtime.Event{Type: "macro.changed", EntityType: "clinical_macro", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}
