package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Legacy coordinates remain exactly as recorded. No depth or physical unit is inferred.
func identifyLegacyMarks(chartID string, marks []chartMark) {
	for index := range marks {
		mark := &marks[index]
		if mark.ID == "" {
			mark.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("chart:%s:%d", chartID, index))).String()
		}
		if mark.TrackingID == "" {
			mark.TrackingID = mark.ID
		}
		if mark.CoordinateSystem == "" {
			mark.CoordinateSystem = "legacy-svg-percent-v1"
		}
	}
}

func prepareChartIdentities(marks []chartMark) error {
	seen := map[string]bool{}
	for index := range marks {
		mark := &marks[index]
		if mark.ID == "" {
			mark.ID = uuid.NewString()
		}
		if _, err := uuid.Parse(mark.ID); err != nil || seen[mark.ID] {
			return fmt.Errorf("annotation IDs must be unique UUIDs")
		}
		seen[mark.ID] = true
		if mark.TrackingID == "" {
			mark.TrackingID = mark.ID
		}
		if _, err := uuid.Parse(mark.TrackingID); err != nil {
			return fmt.Errorf("tracking IDs must be UUIDs")
		}
		if mark.CoordinateSystem == "" {
			mark.CoordinateSystem = "legacy-svg-percent-v1"
		}
		if mark.CoordinateSystem != "legacy-svg-percent-v1" {
			return fmt.Errorf("unsupported coordinate system; no anatomical projection is available yet")
		}
	}
	return nil
}

func chartWriteLocked(w http.ResponseWriter, err error) bool {
	if !strings.Contains(err.Error(), "CHART_ENCOUNTER_LOCKED") {
		return false
	}
	writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "This consultation is finalized or archived. The chart was not changed.")
	return true
}

func (s *Server) validateChartTracking(ctx context.Context, encounterID, chartType, eye string, marks []chartMark) error {
	needsHistory := false
	for _, mark := range marks {
		if mark.TrackingID != mark.ID {
			needsHistory = true
		}
	}
	if !needsHistory {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,h.annotations_json FROM eye_diagrams d JOIN clinical_chart_revisions h ON h.chart_id=d.id JOIN encounters e ON e.id=d.encounter_id
		WHERE e.patient_id=(SELECT patient_id FROM encounters WHERE id=?) AND d.chart_type=? AND d.eye=?
		AND e.created_at <= (SELECT created_at FROM encounters WHERE id=?) AND e.archived_at IS NULL`, encounterID, chartType, eye, encounterID)
	if err != nil {
		return err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var previous []chartMark
		if err := json.Unmarshal([]byte(raw), &previous); err != nil {
			return err
		}
		identifyLegacyMarks(id, previous)
		for _, mark := range previous {
			known[mark.TrackingID] = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, mark := range marks {
		if mark.ID != mark.TrackingID && !known[mark.TrackingID] {
			return fmt.Errorf("unknown finding")
		}
	}
	return nil
}

func (s *Server) previousChartsByEye(ctx context.Context, encounterID string) (map[string]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,chart_type,eye,annotations_json,COALESCE(notes,''),version,exam_status,encounter_id,encounter_number,visit_date
		FROM (SELECT d.*,e.encounter_number,e.created_at AS visit_date,
		ROW_NUMBER() OVER (PARTITION BY d.chart_type,d.eye ORDER BY e.created_at DESC,e.id DESC) AS position
		FROM eye_diagrams d JOIN encounters e ON e.id=d.encounter_id
		WHERE e.patient_id=(SELECT patient_id FROM encounters WHERE id=?) AND e.archived_at IS NULL
		AND e.created_at<(SELECT created_at FROM encounters WHERE id=?)) WHERE position=1`, encounterID, encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]map[string]any{}
	for rows.Next() {
		var id, chartType, eye, raw, notes, status, visitID, number, date string
		var version int
		if err := rows.Scan(&id, &chartType, &eye, &raw, &notes, &version, &status, &visitID, &number, &date); err != nil {
			return nil, err
		}
		var marks []chartMark
		if err := json.Unmarshal([]byte(raw), &marks); err != nil {
			return nil, err
		}
		identifyLegacyMarks(id, marks)
		if result[chartType] == nil {
			result[chartType] = map[string]any{}
		}
		result[chartType][eye] = map[string]any{"encounterId": visitID, "encounterNumber": number, "date": date,
			"chart": map[string]any{"annotations": marks, "notes": notes, "version": version, "examStatus": status, "schemaVersion": 2}}
	}
	return result, rows.Err()
}

func (s *Server) handleChartHistory(w http.ResponseWriter, r *http.Request) {
	chartType, eye, id := chi.URLParam(r, "chartType"), chi.URLParam(r, "eye"), chi.URLParam(r, "id")
	if !chartAcceptsEye(chartType, eye) {
		writeError(w, 422, "INVALID_CHART_TYPE", "Invalid chart or eye.")
		return
	}
	var patientID string
	if err := s.db.QueryRowContext(r.Context(), "SELECT patient_id FROM encounters WHERE id=? AND archived_at IS NULL", id).Scan(&patientID); err != nil {
		writeError(w, 404, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100000 {
			writeError(w, 400, "INVALID_OFFSET", "Invalid history offset.")
			return
		}
		offset = value
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT h.chart_id,h.version,h.annotations_json,COALESCE(h.notes,''),h.exam_status,h.recorded_at,u.display_name,h.baseline,e.id,e.encounter_number
		FROM clinical_chart_revisions h JOIN eye_diagrams d ON d.id=h.chart_id JOIN encounters e ON e.id=d.encounter_id JOIN users u ON u.id=h.recorded_by
		WHERE e.patient_id=? AND d.chart_type=? AND d.eye=? AND e.archived_at IS NULL AND e.created_at<=(SELECT created_at FROM encounters WHERE id=?)
		ORDER BY e.created_at DESC,e.id DESC,h.version DESC LIMIT 21 OFFSET ?`, patientID, chartType, eye, id, offset)
	if err != nil {
		writeError(w, 500, "CHART_HISTORY_FAILED", "Could not load chart history.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var chartID, raw, notes, status, at, author, visitID, number string
		var version, baseline int
		if err := rows.Scan(&chartID, &version, &raw, &notes, &status, &at, &author, &baseline, &visitID, &number); err != nil {
			writeError(w, 500, "CHART_HISTORY_FAILED", "Could not read chart history.")
			return
		}
		var marks []chartMark
		if err := json.Unmarshal([]byte(raw), &marks); err != nil {
			writeError(w, 500, "CHART_HISTORY_FAILED", "Could not read chart history.")
			return
		}
		identifyLegacyMarks(chartID, marks)
		items = append(items, map[string]any{"encounterId": visitID, "encounterNumber": number, "version": version, "annotations": marks, "notes": notes, "examStatus": status, "schemaVersion": 2, "recordedAt": at, "recordedBy": author, "baseline": baseline == 1})
	}
	if rows.Err() != nil {
		writeError(w, 500, "CHART_HISTORY_FAILED", "Could not read chart history.")
		return
	}
	var next any
	if len(items) > 20 {
		items = items[:20]
		next = offset + 20
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"items": items, "nextOffset": next})
}
