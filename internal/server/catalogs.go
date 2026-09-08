package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// D2: appointment reasons and prescription items are clinic-managed lists. They
// are edited from System → Catalogs and take effect immediately, with no
// deployment. Entries are deactivated rather than deleted so a record that
// already refers to one keeps its meaning.

var catalogNames = map[string]bool{"appointment_reason": true, "prescription_item": true}

type catalogPayload struct {
	Label     string            `json:"label"`
	Details   map[string]string `json:"details"`
	SortOrder int               `json:"sortOrder"`
	Active    *bool             `json:"active"`
	Version   int               `json:"version"`
}

type catalogEntry struct {
	ID        string            `json:"id"`
	Catalog   string            `json:"catalog"`
	Label     string            `json:"label"`
	Details   map[string]string `json:"details"`
	SortOrder int               `json:"sortOrder"`
	Active    bool              `json:"active"`
	Version   int               `json:"version"`
	UpdatedAt string            `json:"updatedAt"`
}

func (s *Server) registerCatalogRoutes(r chi.Router) {
	r.Get("/catalogs/{catalog}", s.handleCatalogList)
	r.With(s.requireDoctor).Post("/catalogs/{catalog}", s.handleCatalogCreate)
	r.With(s.requireDoctor).Put("/catalogs/{catalog}/{id}", s.handleCatalogUpdate)
	r.With(s.requireDoctor).Delete("/catalogs/{catalog}/{id}", s.handleCatalogDeactivate)
}

func catalogFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := chi.URLParam(r, "catalog")
	if !catalogNames[name] {
		writeError(w, http.StatusNotFound, "CATALOG_NOT_FOUND", "That catalog does not exist.")
		return "", false
	}
	return name, true
}

func validateCatalogEntry(input *catalogPayload) *APIError {
	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" {
		return &APIError{Code: "VALIDATION_ERROR", Message: "An entry needs a label."}
	}
	if len(input.Label) > 120 {
		return &APIError{Code: "VALIDATION_ERROR", Message: "A label must be 120 characters or fewer."}
	}
	cleaned := map[string]string{}
	for key, value := range input.Details {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			cleaned[strings.TrimSpace(key)] = trimmed
		}
	}
	input.Details = cleaned
	return nil
}

func (s *Server) handleCatalogList(w http.ResponseWriter, r *http.Request) {
	catalog, ok := catalogFromRequest(w, r)
	if !ok {
		return
	}
	// Clinical screens want only what is in use; the admin screen asks for all.
	where := "catalog=? AND active=1"
	if r.URL.Query().Get("includeInactive") == "true" {
		where = "catalog=?"
	}
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id,catalog,label,details_json,sort_order,active,version,updated_at FROM catalog_entries WHERE `+where+` ORDER BY sort_order, label COLLATE NOCASE`, catalog)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CATALOG_LIST_FAILED", "Could not load the catalog.")
		return
	}
	defer rows.Close()
	items := []catalogEntry{}
	for rows.Next() {
		var entry catalogEntry
		var details string
		var active int
		if err := rows.Scan(&entry.ID, &entry.Catalog, &entry.Label, &details, &entry.SortOrder, &active, &entry.Version, &entry.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "CATALOG_LIST_FAILED", "Could not load the catalog.")
			return
		}
		entry.Active = active == 1
		if json.Unmarshal([]byte(details), &entry.Details) != nil || entry.Details == nil {
			entry.Details = map[string]string{}
		}
		items = append(items, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"catalog": catalog, "items": items})
}

func (s *Server) handleCatalogCreate(w http.ResponseWriter, r *http.Request) {
	catalog, ok := catalogFromRequest(w, r)
	if !ok {
		return
	}
	var input catalogPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if apiErr := validateCatalogEntry(&input); apiErr != nil {
		writeError(w, http.StatusUnprocessableEntity, apiErr.Code, apiErr.Message)
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(),
		`INSERT INTO catalog_entries(id,catalog,label,details_json,sort_order,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?)`,
		id, catalog, input.Label, marshalJSON(input.Details), input.SortOrder, now, now, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "CATALOG_ENTRY_EXISTS", "That entry already exists in this catalog.")
			return
		}
		writeError(w, http.StatusInternalServerError, "CATALOG_CREATE_FAILED", "Could not add the catalog entry.")
		return
	}
	s.audit(r.Context(), &user, "create", "catalog_entry", id, "Added "+catalog+" entry "+input.Label, "", "", r)
	s.broker.Publish(realtime.Event{Type: "catalog.changed", EntityType: "catalog_entry", EntityID: id})
	writeJSON(w, http.StatusCreated, catalogEntry{ID: id, Catalog: catalog, Label: input.Label, Details: input.Details, SortOrder: input.SortOrder, Active: true, Version: 1, UpdatedAt: now})
}

func (s *Server) handleCatalogUpdate(w http.ResponseWriter, r *http.Request) {
	catalog, ok := catalogFromRequest(w, r)
	if !ok {
		return
	}
	var input catalogPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Catalog entry fields and the current version are required.")
		return
	}
	if apiErr := validateCatalogEntry(&input); apiErr != nil {
		writeError(w, http.StatusUnprocessableEntity, apiErr.Code, apiErr.Message)
		return
	}
	active := 1
	if input.Active != nil && !*input.Active {
		active = 0
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(),
		`UPDATE catalog_entries SET label=?,details_json=?,sort_order=?,active=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND catalog=? AND version=?`,
		input.Label, marshalJSON(input.Details), input.SortOrder, active, now, user.ID, id, catalog, input.Version)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "CATALOG_ENTRY_EXISTS", "Another entry in this catalog already uses that label.")
			return
		}
		writeError(w, http.StatusInternalServerError, "CATALOG_UPDATE_FAILED", "Could not update the catalog entry.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This catalog entry changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "update", "catalog_entry", id, "Updated "+catalog+" entry "+input.Label, "", "", r)
	s.broker.Publish(realtime.Event{Type: "catalog.changed", EntityType: "catalog_entry", EntityID: id})
	writeJSON(w, http.StatusOK, catalogEntry{ID: id, Catalog: catalog, Label: input.Label, Details: input.Details, SortOrder: input.SortOrder, Active: active == 1, Version: input.Version + 1, UpdatedAt: now})
}

func (s *Server) handleCatalogDeactivate(w http.ResponseWriter, r *http.Request) {
	catalog, ok := catalogFromRequest(w, r)
	if !ok {
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(),
		"UPDATE catalog_entries SET active=0,version=version+1,updated_at=?,updated_by=? WHERE id=? AND catalog=? AND active=1", now, user.ID, id, catalog)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CATALOG_DEACTIVATE_FAILED", "Could not retire the catalog entry.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "CATALOG_ENTRY_NOT_FOUND", "That catalog entry was not found.")
		return
	}
	s.audit(r.Context(), &user, "archive", "catalog_entry", id, "Retired a "+catalog+" entry", "", "", r)
	s.broker.Publish(realtime.Event{Type: "catalog.changed", EntityType: "catalog_entry", EntityID: id})
	w.WriteHeader(http.StatusNoContent)
}
