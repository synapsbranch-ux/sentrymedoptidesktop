package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// A consultation moves through two working stages before it is signed. They are
// stored on the consultation itself rather than inferred from the waiting-room
// queue, because a consultation can be started for a patient who never queued —
// a walk-in seen straight away, or a record opened after the fact.
const (
	stagePreTest    = "pre_test"
	stageDoctorExam = "doctor_exam"
)

// pretestPolicy says whether the nurse pre-test may be skipped. "required" keeps
// the skip action out of the interface for clinics that always pre-test;
// "optional" lets the clinician decide case by case. Neither value blocks
// finalization — a pre-test is preparatory, not a gate.
func (s *Server) pretestPolicy(ctx context.Context) string {
	var raw string
	if s.db.QueryRowContext(ctx, "SELECT COALESCE(json_extract(value_json,'$.pretestPolicy'),'') FROM settings WHERE key='clinical'").Scan(&raw) != nil {
		return "optional"
	}
	if raw == "required" {
		return "required"
	}
	return "optional"
}

// advanceToDoctorExam routes a consultation out of the pre-test stage. It is
// deliberately one-way: a consultation that has already reached the doctor never
// falls back, so a late pre-test edit cannot pull the patient out of the room.
func advanceToDoctorExam(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, encounterID, userID, now string) error {
	if _, err := exec.ExecContext(ctx, "UPDATE encounters SET workflow_stage=? WHERE id=? AND workflow_stage=? AND status='draft'", stageDoctorExam, encounterID, stagePreTest); err != nil {
		return err
	}
	_, err := exec.ExecContext(ctx, "UPDATE queue_entries SET stage='waiting_doctor',version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND completed_at IS NULL AND stage IN ('checked_in','waiting_nurse','pre_test')", now, userID, encounterID)
	return err
}

// A pre-test can only be skipped while it is still open and at the version the
// caller read, so two people cannot skip and complete it at the same time.
var errPretestSkipConflict = errors.New("pretest skip conflict")

type pretestSkipPayload struct {
	Reason  string `json:"reason"`
	Version int    `json:"version"`
}

// handlePretestSkip records that the pre-test was deliberately not performed and
// routes the consultation straight to the doctor's exam.
func (s *Server) handlePretestSkip(w http.ResponseWriter, r *http.Request) {
	var input pretestSkipPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "The current pre-test version is required.")
		return
	}
	if len([]rune(input.Reason)) > 500 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SKIP_REASON", "The reason for skipping is too long.")
		return
	}
	if s.pretestPolicy(r.Context()) != "optional" {
		writeError(w, http.StatusForbidden, "PRETEST_REQUIRED", "This clinic requires a pre-test on every consultation. A doctor can change this under System → Clinical.")
		return
	}
	encounterID := chi.URLParam(r, "id")
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=? AND archived_at IS NULL", encounterID).Scan(&status); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	if status == "finalized" {
		writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "This consultation is finalized and locked.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		result, err := tx.ExecContext(r.Context(), "UPDATE pretests SET skipped_at=?,skipped_by=?,skip_reason=?,version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND version=? AND completed_at IS NULL", now, user.ID, nilIfEmpty(strings.TrimSpace(input.Reason)), now, user.ID, encounterID, input.Version)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errPretestSkipConflict
		}
		return advanceToDoctorExam(r.Context(), tx, encounterID, user.ID, now)
	})
	if err != nil {
		if errors.Is(err, errPretestSkipConflict) {
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The pre-test changed before it could be skipped. Reload and review it.")
			return
		}
		if strings.Contains(err.Error(), "ENCOUNTER_FINALIZED") {
			writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "This consultation is finalized and locked.")
			return
		}
		writeError(w, http.StatusInternalServerError, "PRETEST_SKIP_FAILED", "Could not skip the pre-test.")
		return
	}
	s.audit(r.Context(), &user, "skip", "pretest", encounterID, "Skipped the pre-test and moved the consultation to the doctor's exam", "", "", r)
	s.broker.Publish(realtime.Event{Type: "pretest.changed", EntityType: "encounter", EntityID: encounterID})
	writeJSON(w, http.StatusOK, map[string]any{"encounterId": encounterID, "version": input.Version + 1, "skipped": true, "skippedAt": now, "workflowStage": stageDoctorExam})
}

// finalizeWarnings lists what a doctor is signing off without. None of these
// stop a signature: the doctor decides when the record is complete, and the
// warnings are shown so the decision is a deliberate one.
func (s *Server) finalizeWarnings(ctx context.Context, encounterID string) []map[string]any {
	warnings := []map[string]any{}
	var diagnoses, prescriptions, sections int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnoses WHERE encounter_id=?", encounterID).Scan(&diagnoses)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prescriptions WHERE encounter_id=? AND archived_at IS NULL", encounterID).Scan(&prescriptions)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM encounter_sections WHERE encounter_id=?", encounterID).Scan(&sections)
	if diagnoses == 0 {
		warnings = append(warnings, map[string]any{"code": "NO_DIAGNOSIS", "message": "No diagnosis has been recorded for this consultation."})
	}
	if prescriptions == 0 {
		warnings = append(warnings, map[string]any{"code": "NO_PRESCRIPTION", "message": "No prescription has been issued from this consultation."})
	}
	if sections == 0 {
		warnings = append(warnings, map[string]any{"code": "NO_EXAMINATION_SECTION", "message": "No examination section has been filled in."})
	}
	// Only worth mentioning when the consultation is still parked in the pre-test
	// stage: a doctor who started the consultation themself never had one queued.
	var pretestOpen int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM encounters e JOIN pretests p ON p.encounter_id=e.id WHERE e.id=? AND e.workflow_stage=? AND p.completed_at IS NULL AND p.skipped_at IS NULL", encounterID, stagePreTest).Scan(&pretestOpen)
	if pretestOpen > 0 {
		warnings = append(warnings, map[string]any{"code": "PRETEST_OPEN", "message": "The pre-test was neither completed nor skipped."})
	}
	return warnings
}

func warningCodes(warnings []map[string]any) []string {
	codes := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		codes = append(codes, warning["code"].(string))
	}
	return codes
}

func encodeWarnings(warnings []map[string]any) string {
	raw, err := json.Marshal(warningCodes(warnings))
	if err != nil {
		return "[]"
	}
	return string(raw)
}
