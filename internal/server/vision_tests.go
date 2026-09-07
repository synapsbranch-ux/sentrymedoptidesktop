package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// visionTestState is the whole of what the exam-lane screen shows. The phone writes
// it, the screen reads it; nothing else about the pair needs coordinating.
type visionTestState struct {
	Mode       string             `json:"mode"`
	Eye        string             `json:"eye"`
	Correction string             `json:"correction"`
	LogMAR     float64            `json:"logMar"`
	Optotype   string             `json:"optotype"`
	Seed       int64              `json:"seed"`
	SingleLine bool               `json:"singleLine"`
	Plate      int                `json:"plate"`
	Display    visionDisplayInfo  `json:"display"`
	Results    []visionTestResult `json:"results"`
}

// visionDisplayInfo is reported by the screen itself. Calibration belongs to a
// physical monitor, not to the clinic, so the screen owns these numbers and the
// phone only reads them — a result must never be recorded off an uncalibrated screen.
type visionDisplayInfo struct {
	DistanceMm   float64 `json:"distanceMm"`
	PixelsPerMm  float64 `json:"pixelsPerMm"`
	CalibratedAt string  `json:"calibratedAt"`
	Label        string  `json:"label"`
}

type visionTestResult struct {
	Eye        string  `json:"eye"`
	Correction string  `json:"correction"`
	LogMAR     float64 `json:"logMar"`
	Snellen    string  `json:"snellen"`
	Test       string  `json:"test"`
	Detail     string  `json:"detail"`
	RecordedAt string  `json:"recordedAt"`
}

var (
	visionModes      = map[string]bool{"blank": true, "acuity": true, "colour": true, "amsler": true, "fixation": true}
	visionOptotypes  = map[string]bool{"sloan": true, "landolt": true, "tumblingE": true, "numbers": true}
	visionEyes       = map[string]bool{"OD": true, "OS": true, "OU": true}
	visionCorrection = map[string]bool{"uncorrected": true, "corrected": true, "pinhole": true}
)

// visionSnellenSteps are the standard chart lines, listed in logMAR so a measured
// value maps back to the notation clinicians actually write in the record.
var visionSnellenSteps = []struct {
	logMAR  float64
	snellen string
}{
	{-0.30, "20/10"}, {-0.20, "20/12.5"}, {-0.10, "20/16"}, {0.00, "20/20"}, {0.10, "20/25"},
	{0.20, "20/32"}, {0.30, "20/40"}, {0.40, "20/50"}, {0.50, "20/63"}, {0.60, "20/80"},
	{0.70, "20/100"}, {0.80, "20/125"}, {0.90, "20/160"}, {1.00, "20/200"}, {1.10, "20/250"},
	{1.20, "20/320"}, {1.30, "20/400"}, {1.40, "20/500"}, {1.50, "20/630"}, {1.60, "20/800"},
}

// snellenFromLogMAR names the nearest standard line. Anything past the chart's last
// line is reported in logMAR rather than invented as a Snellen fraction.
func snellenFromLogMAR(value float64) string {
	best, distance := "", math.MaxFloat64
	for _, step := range visionSnellenSteps {
		if gap := math.Abs(step.logMAR - value); gap < distance {
			best, distance = step.snellen, gap
		}
	}
	if distance > 0.06 {
		return fmt.Sprintf("logMAR %.2f", value)
	}
	return best
}

func defaultVisionState() visionTestState {
	return visionTestState{
		Mode: "blank", Eye: "OD", Correction: "uncorrected", LogMAR: 1.0,
		Optotype: "sloan", Seed: time.Now().UnixNano() % 1_000_000, SingleLine: false,
		Plate: 1, Results: []visionTestResult{},
	}
}

// sanitiseVisionState keeps the shared state inside the values both screens agree on,
// so a stale or malformed phone cannot leave the exam-lane display in a broken mode.
func sanitiseVisionState(state visionTestState) (visionTestState, string) {
	if !visionModes[state.Mode] {
		return state, "Mode must be blank, acuity, colour, amsler or fixation."
	}
	if !visionEyes[state.Eye] {
		return state, "Eye must be OD, OS or OU."
	}
	if !visionCorrection[state.Correction] {
		return state, "Correction must be uncorrected, corrected or pinhole."
	}
	if !visionOptotypes[state.Optotype] {
		return state, "Optotype must be sloan, landolt, tumblingE or numbers."
	}
	if state.LogMAR < -0.3 || state.LogMAR > 1.6 {
		return state, "Acuity level must be between -0.30 and 1.60 logMAR."
	}
	if state.Plate < 1 || state.Plate > 24 {
		return state, "Plate number must be between 1 and 24."
	}
	if state.Display.DistanceMm < 0 || state.Display.DistanceMm > 20000 {
		return state, "Test distance must be between 0 and 20000 mm."
	}
	if state.Display.PixelsPerMm < 0 || state.Display.PixelsPerMm > 200 {
		return state, "Screen calibration is out of range."
	}
	if state.Results == nil {
		state.Results = []visionTestResult{}
	}
	if len(state.Results) > 40 {
		return state, "A session holds at most 40 recorded results."
	}
	state.LogMAR = round2(state.LogMAR)
	for index, result := range state.Results {
		state.Results[index].LogMAR = round2(result.LogMAR)
		if strings.TrimSpace(result.Snellen) == "" {
			state.Results[index].Snellen = snellenFromLogMAR(result.LogMAR)
		}
	}
	return state, ""
}

func (s *Server) registerVisionTestRoutes(r chi.Router) {
	r.Get("/vision-tests", s.handleVisionTestList)
	r.Post("/vision-tests", s.handleVisionTestCreate)
	r.Get("/vision-tests/{id}", s.handleVisionTestGet)
	r.Put("/vision-tests/{id}/state", s.handleVisionTestStateSave)
	r.Post("/vision-tests/{id}/close", s.handleVisionTestClose)
	r.Post("/vision-tests/{id}/apply", s.handleVisionTestApply)
}

type visionSessionRow struct {
	ID          string          `json:"id"`
	Room        string          `json:"room"`
	EncounterID string          `json:"encounterId"`
	PatientName string          `json:"patientName"`
	State       visionTestState `json:"state"`
	Revision    int             `json:"revision"`
	UpdatedAt   string          `json:"updatedAt"`
}

const visionSessionSelect = `SELECT v.id,v.room,COALESCE(v.encounter_id,''),COALESCE(p.first_name||' '||p.last_name,''),v.state_json,v.revision,v.updated_at
	FROM vision_test_sessions v
	LEFT JOIN encounters e ON e.id=v.encounter_id
	LEFT JOIN patients p ON p.id=e.patient_id`

func scanVisionSession(scan func(...any) error) (visionSessionRow, error) {
	var row visionSessionRow
	var stateJSON string
	if err := scan(&row.ID, &row.Room, &row.EncounterID, &row.PatientName, &stateJSON, &row.Revision, &row.UpdatedAt); err != nil {
		return row, err
	}
	row.State = defaultVisionState()
	_ = json.Unmarshal([]byte(stateJSON), &row.State)
	if row.State.Results == nil {
		row.State.Results = []visionTestResult{}
	}
	return row, nil
}

func (s *Server) handleVisionTestList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), visionSessionSelect+" WHERE v.closed_at IS NULL ORDER BY v.updated_at DESC LIMIT 25")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_TEST_LIST_FAILED", "Could not load the vision test sessions.")
		return
	}
	defer rows.Close()
	items := []visionSessionRow{}
	for rows.Next() {
		row, err := scanVisionSession(rows.Scan)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "VISION_TEST_LIST_FAILED", "Could not read the vision test sessions.")
			return
		}
		items = append(items, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleVisionTestCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Room        string `json:"room"`
		EncounterID string `json:"encounterId"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A room name is required.")
		return
	}
	input.Room = strings.TrimSpace(input.Room)
	if input.Room == "" || len(input.Room) > 60 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_ROOM", "Room must be between 1 and 60 characters.")
		return
	}
	if input.EncounterID != "" {
		var status string
		if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=?", input.EncounterID).Scan(&status); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "ENCOUNTER_NOT_FOUND", "The linked consultation was not found.")
			return
		}
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := uuid.NewString()
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO vision_test_sessions(id,room,encounter_id,state_json,created_at,updated_at,created_by,updated_by) VALUES(?,?,?,?,?,?,?,?)`,
		id, input.Room, nilIfEmpty(input.EncounterID), marshalJSON(defaultVisionState()), now, now, user.ID, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_TEST_CREATE_FAILED", "Could not open the vision test session.")
		return
	}
	s.audit(r.Context(), &user, "create", "vision_test", id, "Opened vision test in "+input.Room, "", "", r)
	s.broker.Publish(realtime.Event{Type: "vision_test.updated", EntityType: "vision_test", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "room": input.Room, "state": defaultVisionState(), "revision": 1})
}

func (s *Server) loadVisionSession(r *http.Request, id string) (visionSessionRow, error) {
	return scanVisionSession(s.db.QueryRowContext(r.Context(), visionSessionSelect+" WHERE v.id=?", id).Scan)
}

func (s *Server) handleVisionTestGet(w http.ResponseWriter, r *http.Request) {
	row, err := s.loadVisionSession(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "VISION_TEST_NOT_FOUND", "The vision test session was not found.")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleVisionTestStateSave(w http.ResponseWriter, r *http.Request) {
	var input struct {
		State visionTestState `json:"state"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Vision test state is required.")
		return
	}
	state, problem := sanitiseVisionState(input.State)
	if problem != "" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_VISION_STATE", problem)
		return
	}
	id := chi.URLParam(r, "id")
	user, _ := userFromContext(r.Context())
	result, err := s.db.ExecContext(r.Context(), `UPDATE vision_test_sessions SET state_json=?,revision=revision+1,updated_at=?,updated_by=? WHERE id=? AND closed_at IS NULL`,
		marshalJSON(state), time.Now().UTC().Format(time.RFC3339Nano), user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_TEST_SAVE_FAILED", "Could not update the vision test session.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "VISION_TEST_NOT_FOUND", "The vision test session is closed or was not found.")
		return
	}
	s.broker.Publish(realtime.Event{Type: "vision_test.updated", EntityType: "vision_test", EntityID: id})
	row, err := s.loadVisionSession(r, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_TEST_SAVE_FAILED", "Could not read back the vision test session.")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleVisionTestClose(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, _ := userFromContext(r.Context())
	result, err := s.db.ExecContext(r.Context(), "UPDATE vision_test_sessions SET closed_at=?,revision=revision+1,updated_at=?,updated_by=? WHERE id=? AND closed_at IS NULL",
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_TEST_CLOSE_FAILED", "Could not close the vision test session.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "VISION_TEST_NOT_FOUND", "The vision test session is already closed.")
		return
	}
	s.audit(r.Context(), &user, "update", "vision_test", id, "Closed vision test session", "", "", r)
	s.broker.Publish(realtime.Event{Type: "vision_test.updated", EntityType: "vision_test", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "closed": true})
}

// visionAcuityField maps a measured result onto the pre-test acuity grid, whose keys
// are built from the same eye and field names the nurse sees on screen.
func visionAcuityField(eye, correction string) string {
	switch correction {
	case "corrected":
		return strings.ToLower(eye) + "correcteddistance"
	case "pinhole":
		return strings.ToLower(eye) + "pinhole"
	default:
		return strings.ToLower(eye) + "uncorrecteddistance"
	}
}

// handleVisionTestApply writes the measured acuity into the linked consultation's
// pre-test. Only the acuity fields this session measured are touched; everything else
// the nurse entered is read back and written out unchanged.
func (s *Server) handleVisionTestApply(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.loadVisionSession(r, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "VISION_TEST_NOT_FOUND", "The vision test session was not found.")
		return
	}
	if row.EncounterID == "" {
		writeError(w, http.StatusUnprocessableEntity, "NO_LINKED_ENCOUNTER", "Link this session to a consultation before recording results.")
		return
	}
	acuityResults := []visionTestResult{}
	for _, result := range row.State.Results {
		if result.Test == "acuity" && visionEyes[result.Eye] && result.Eye != "OU" {
			acuityResults = append(acuityResults, result)
		}
	}
	if len(acuityResults) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "NO_ACUITY_RESULTS", "Record at least one acuity result before writing it to the pre-test.")
		return
	}
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=?", row.EncounterID).Scan(&status); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "The linked consultation was not found.")
		return
	}
	if status == "finalized" {
		writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "The linked consultation is finalized and locked.")
		return
	}
	var acuityJSON string
	var version int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COALESCE(visual_acuity_json,'{}'),version FROM pretests WHERE encounter_id=?", row.EncounterID).Scan(&acuityJSON, &version); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusUnprocessableEntity, "PRETEST_MISSING", "The linked consultation has no pre-test section.")
			return
		}
		writeError(w, http.StatusInternalServerError, "VISION_TEST_APPLY_FAILED", "Could not read the pre-test.")
		return
	}
	acuity := map[string]string{}
	_ = json.Unmarshal([]byte(acuityJSON), &acuity)
	applied := []string{}
	for _, result := range acuityResults {
		field := visionAcuityField(result.Eye, result.Correction)
		acuity[field] = result.Snellen
		applied = append(applied, result.Eye+" "+result.Correction+" "+result.Snellen)
	}
	user, _ := userFromContext(r.Context())
	update, err := s.db.ExecContext(r.Context(), "UPDATE pretests SET visual_acuity_json=?,version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND version=?",
		marshalJSON(acuity), time.Now().UTC().Format(time.RFC3339Nano), user.ID, row.EncounterID, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_TEST_APPLY_FAILED", "Could not write the results to the pre-test.")
		return
	}
	if affected, _ := update.RowsAffected(); affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The pre-test changed while the test was running. Reopen it and try again.")
		return
	}
	s.audit(r.Context(), &user, "update", "pretest", row.EncounterID, "Recorded vision test acuity: "+strings.Join(applied, ", "), "", "", r)
	s.broker.Publish(realtime.Event{Type: "pretest.changed", EntityType: "encounter", EntityID: row.EncounterID})
	writeJSON(w, http.StatusOK, map[string]any{"encounterId": row.EncounterID, "applied": applied, "visualAcuity": acuity})
}
