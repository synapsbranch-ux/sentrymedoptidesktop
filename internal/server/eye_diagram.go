package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// eyeDiagramMark is one annotation placed on the schematic eye drawing, in
// percentage coordinates (0-100) so the diagram scales to any viewport.
type eyeDiagramMark struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Shape   string  `json:"shape"`
	Color   string  `json:"color"`
	Label   string  `json:"label"`
	Structure string `json:"structure"`
}

type eyeDiagramPayload struct {
	Annotations []eyeDiagramMark `json:"annotations"`
	Notes       string           `json:"notes"`
	Version     int              `json:"version"`
}

var validEyes = map[string]bool{"OD": true, "OS": true}

func (s *Server) registerEyeDiagramRoutes(r chi.Router) {
	r.Get("/encounters/{id}/eye-diagrams", s.handleEyeDiagramsGet)
	r.Put("/encounters/{id}/eye-diagrams/{eye}", s.handleEyeDiagramSave)
}

func (s *Server) handleEyeDiagramsGet(w http.ResponseWriter, r *http.Request) {
	encounterID := chi.URLParam(r, "id")
	result := map[string]any{}
	for eye := range validEyes {
		var annotationsJSON, notes string
		var version int
		err := s.db.QueryRowContext(r.Context(), "SELECT annotations_json,COALESCE(notes,''),version FROM eye_diagrams WHERE encounter_id=? AND eye=?", encounterID, eye).Scan(&annotationsJSON, &notes, &version)
		annotations := []eyeDiagramMark{}
		if err == nil {
			_ = json.Unmarshal([]byte(annotationsJSON), &annotations)
			result[eye] = map[string]any{"annotations": annotations, "notes": notes, "version": version}
		} else {
			result[eye] = map[string]any{"annotations": annotations, "notes": "", "version": 0}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleEyeDiagramSave(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if user.Role != "doctor" {
		writeError(w, http.StatusForbidden, "DOCTOR_ACCESS_REQUIRED", "Only a doctor may update the annotated eye diagram.")
		return
	}
	eye := chi.URLParam(r, "eye")
	if !validEyes[eye] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_EYE", "Eye must be OD or OS.")
		return
	}
	var input eyeDiagramPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Diagram data and version are required.")
		return
	}
	if input.Annotations == nil {
		input.Annotations = []eyeDiagramMark{}
	}
	encounterID, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=?", encounterID).Scan(&status); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	if status == "finalized" {
		writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "This consultation is finalized and locked.")
		return
	}
	annotationsJSON := marshalJSON(input.Annotations)
	if input.Version == 0 {
		_, err := s.db.ExecContext(r.Context(), `INSERT INTO eye_diagrams(id,encounter_id,eye,annotations_json,notes,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?)`, uuid.NewString(), encounterID, eye, annotationsJSON, nilIfEmpty(input.Notes), now, now, user.ID, user.ID)
		if err != nil {
			if isUniqueViolation(err) {
				writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This eye diagram was created by another user. Reload before saving.")
				return
			}
			writeError(w, http.StatusInternalServerError, "EYE_DIAGRAM_SAVE_FAILED", "Could not save the eye diagram.")
			return
		}
		s.audit(r.Context(), &user, "create", "eye_diagram", encounterID, "Recorded "+eye+" eye diagram", "", "", r)
		s.broker.Publish(realtime.Event{Type: "encounter.updated", EntityType: "encounter", EntityID: encounterID})
		writeJSON(w, http.StatusCreated, map[string]any{"version": 1})
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE eye_diagrams SET annotations_json=?,notes=?,version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND eye=? AND version=?`, annotationsJSON, nilIfEmpty(input.Notes), now, user.ID, encounterID, eye, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EYE_DIAGRAM_SAVE_FAILED", "Could not save the eye diagram.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This eye diagram changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "eye_diagram", encounterID, "Updated "+eye+" eye diagram", "", "", r)
	s.broker.Publish(realtime.Event{Type: "encounter.updated", EntityType: "encounter", EntityID: encounterID})
	writeJSON(w, http.StatusOK, map[string]any{"version": input.Version + 1})
}
