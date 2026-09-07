package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// chartMark is one annotation placed on a clinical chart, in percentage
// coordinates (0-100) so every drawing scales to any viewport. Free-form charts
// (anterior segment, fundus) use the label and structure; grid charts (visual
// field, motility) additionally carry the cell they belong to and its grade.
type chartMark struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Shape     string  `json:"shape"`
	Color     string  `json:"color"`
	Label     string  `json:"label"`
	Structure string  `json:"structure"`
	Cell      string  `json:"cell,omitempty"`
	Grade     string  `json:"grade,omitempty"`
}

type chartPayload struct {
	Annotations []chartMark `json:"annotations"`
	Notes       string      `json:"notes"`
	Version     int         `json:"version"`
}

// chartEyes declares which eyes each chart type is recorded for. Ocular motility
// and cover testing describe how the two eyes work together, so they are stored
// once as OU rather than duplicated per eye.
var chartEyes = map[string][]string{
	"anterior": {"OD", "OS"},
	"fundus":   {"OD", "OS"},
	"field":    {"OD", "OS"},
	"motility": {"OU"},
}

// maxChartMarks bounds a single chart so a malformed client cannot store an
// unbounded blob inside the consultation record.
const maxChartMarks = 200

func chartAcceptsEye(chartType, eye string) bool {
	for _, candidate := range chartEyes[chartType] {
		if candidate == eye {
			return true
		}
	}
	return false
}

func (s *Server) registerEyeDiagramRoutes(r chi.Router) {
	r.Get("/encounters/{id}/eye-diagrams", s.handleEyeDiagramsGet)
	r.Put("/encounters/{id}/eye-diagrams/{chartType}/{eye}", s.handleEyeDiagramSave)
}

// loadEncounterCharts returns every chart of an encounter, pre-seeded with an
// empty entry for each declared chart/eye pair so the client can render the full
// set without special-casing charts that were never opened.
func (s *Server) loadEncounterCharts(ctx context.Context, encounterID string) (map[string]map[string]any, error) {
	charts := map[string]map[string]any{}
	for chartType, eyes := range chartEyes {
		charts[chartType] = map[string]any{}
		for _, eye := range eyes {
			charts[chartType][eye] = map[string]any{"annotations": []chartMark{}, "notes": "", "version": 0}
		}
	}
	rows, err := s.db.QueryContext(ctx, "SELECT chart_type,eye,annotations_json,COALESCE(notes,''),version FROM eye_diagrams WHERE encounter_id=?", encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var chartType, eye, annotationsJSON, notes string
		var version int
		if err := rows.Scan(&chartType, &eye, &annotationsJSON, &notes, &version); err != nil {
			return nil, err
		}
		if charts[chartType] == nil {
			continue
		}
		annotations := []chartMark{}
		_ = json.Unmarshal([]byte(annotationsJSON), &annotations)
		charts[chartType][eye] = map[string]any{"annotations": annotations, "notes": notes, "version": version}
	}
	return charts, rows.Err()
}

func (s *Server) handleEyeDiagramsGet(w http.ResponseWriter, r *http.Request) {
	encounterID := chi.URLParam(r, "id")
	charts, err := s.loadEncounterCharts(r.Context(), encounterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EYE_DIAGRAM_LOAD_FAILED", "Could not load the clinical charts.")
		return
	}
	response := map[string]any{"charts": charts, "previous": nil}

	// The previous charted visit is returned alongside so the visual field grid
	// can overlay the last relevant exam and show what moved since then.
	var previousID, previousNumber, previousDate string
	err = s.db.QueryRowContext(r.Context(), `
		SELECT e.id,e.encounter_number,e.created_at FROM encounters e
		WHERE e.patient_id=(SELECT patient_id FROM encounters WHERE id=?)
		  AND e.archived_at IS NULL
		  AND e.created_at<(SELECT created_at FROM encounters WHERE id=?)
		  AND EXISTS (SELECT 1 FROM eye_diagrams d WHERE d.encounter_id=e.id)
		ORDER BY e.created_at DESC LIMIT 1`, encounterID, encounterID).Scan(&previousID, &previousNumber, &previousDate)
	if err == nil {
		if previousCharts, loadErr := s.loadEncounterCharts(r.Context(), previousID); loadErr == nil {
			response["previous"] = map[string]any{"encounterNumber": previousNumber, "date": previousDate, "charts": previousCharts}
		}
	} else if err != sql.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "EYE_DIAGRAM_LOAD_FAILED", "Could not load the clinical charts.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleEyeDiagramSave(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if user.Role != "doctor" {
		writeError(w, http.StatusForbidden, "DOCTOR_ACCESS_REQUIRED", "Only a doctor may update the clinical charts.")
		return
	}
	chartType, eye := chi.URLParam(r, "chartType"), chi.URLParam(r, "eye")
	if chartEyes[chartType] == nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_CHART_TYPE", "Chart must be anterior, fundus, field or motility.")
		return
	}
	if !chartAcceptsEye(chartType, eye) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_EYE", "This chart is not recorded for that eye.")
		return
	}
	var input chartPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Chart data and version are required.")
		return
	}
	if input.Annotations == nil {
		input.Annotations = []chartMark{}
	}
	if len(input.Annotations) > maxChartMarks {
		writeError(w, http.StatusUnprocessableEntity, "TOO_MANY_MARKS", "A chart holds at most 200 marks.")
		return
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
		_, err := s.db.ExecContext(r.Context(), `INSERT INTO eye_diagrams(id,encounter_id,chart_type,eye,annotations_json,notes,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), encounterID, chartType, eye, annotationsJSON, nilIfEmpty(input.Notes), now, now, user.ID, user.ID)
		if err != nil {
			if isUniqueViolation(err) {
				writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This chart was created by another user. Reload before saving.")
				return
			}
			writeError(w, http.StatusInternalServerError, "EYE_DIAGRAM_SAVE_FAILED", "Could not save the clinical chart.")
			return
		}
		s.audit(r.Context(), &user, "create", "eye_diagram", encounterID, "Recorded "+eye+" "+chartType+" chart", "", "", r)
		s.broker.Publish(realtime.Event{Type: "encounter.updated", EntityType: "encounter", EntityID: encounterID})
		writeJSON(w, http.StatusCreated, map[string]any{"version": 1})
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE eye_diagrams SET annotations_json=?,notes=?,version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND chart_type=? AND eye=? AND version=?`, annotationsJSON, nilIfEmpty(input.Notes), now, user.ID, encounterID, chartType, eye, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EYE_DIAGRAM_SAVE_FAILED", "Could not save the clinical chart.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This chart changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "eye_diagram", encounterID, "Updated "+eye+" "+chartType+" chart", "", "", r)
	s.broker.Publish(realtime.Event{Type: "encounter.updated", EntityType: "encounter", EntityID: encounterID})
	writeJSON(w, http.StatusOK, map[string]any{"version": input.Version + 1})
}
