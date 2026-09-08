package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// B2: registering a walk-in used to mean creating the patient in one dialog and
// then checking them in from another, with a patient dropdown in between. This
// endpoint does both in one request so the front desk never leaves the form.

type walkInPayload struct {
	// Set when the front desk picked an existing record from the live search.
	PatientID   string `json:"patientId"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Phone       string `json:"phone"`
	VisitReason string `json:"reason"`
	Priority    int    `json:"priority"`
}

func (s *Server) handleQueueWalkIn(w http.ResponseWriter, r *http.Request) {
	var input walkInPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Phone = strings.TrimSpace(input.Phone)
	input.VisitReason = strings.TrimSpace(input.VisitReason)

	patientID := strings.TrimSpace(input.PatientID)
	created := false
	if patientID == "" {
		if err := requireFields(map[string]string{"First name": input.FirstName, "Last name": input.LastName}); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		// The form searches as the user types, so a duplicate here means the
		// match was not noticed. Return it rather than creating a second record.
		var duplicateID, duplicateNumber string
		duplicate := s.db.QueryRowContext(r.Context(), `SELECT id, medical_record_number FROM patients WHERE archived_at IS NULL
			AND lower(first_name)=lower(?) AND lower(last_name)=lower(?) AND ((phone=? AND ?<>'') OR ?='') LIMIT 1`,
			input.FirstName, input.LastName, input.Phone, input.Phone, input.Phone).Scan(&duplicateID, &duplicateNumber)
		if duplicate == nil {
			writeJSON(w, http.StatusConflict, APIError{Code: "POSSIBLE_DUPLICATE_PATIENT", Message: "A patient with this name is already registered. Check them in instead of creating a second record.", Details: map[string]any{"patientId": duplicateID, "medicalRecordNumber": duplicateNumber}})
			return
		}
		created = true
	}

	var active int
	if patientID != "" {
		_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM queue_entries WHERE patient_id=? AND completed_at IS NULL", patientID).Scan(&active)
		if active > 0 {
			writeError(w, http.StatusConflict, "PATIENT_ALREADY_IN_QUEUE", "This patient is already in the waiting room.")
			return
		}
	}

	user, _ := userFromContext(r.Context())
	queueID, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var medicalRecordNumber string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if created {
			newID, err := uuid.NewRandom()
			if err != nil {
				return err
			}
			patientID = newID.String()
			medicalRecordNumber, err = s.nextNumber(r.Context(), tx, "patient", "PT", false)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO patients(id,medical_record_number,first_name,last_name,phone,tags_json,created_at,updated_at,created_by,updated_by)
				VALUES(?,?,?,?,?,'[]',?,?,?,?)`, patientID, medicalRecordNumber, input.FirstName, input.LastName, nilIfEmpty(input.Phone), now, now, user.ID, user.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), "INSERT INTO patient_histories(id,patient_id,created_at,updated_at,updated_by) VALUES(?,?,?,?,?)", uuid.NewString(), patientID, now, now, user.ID); err != nil {
				return err
			}
		}
		// `source` records who entered the arrival (staff or the patient kiosk).
		// A walk-in is identified by having no appointment, not by a new source
		// value, which the column's CHECK constraint does not allow.
		_, err := tx.ExecContext(r.Context(), `INSERT INTO queue_entries(id,patient_id,arrived_at,stage,priority,source,visit_reason,created_at,updated_at,updated_by)
			VALUES(?,?,?,'waiting_nurse',?,'staff',?,?,?,?)`, queueID, patientID, now, input.Priority, nilIfEmpty(input.VisitReason), now, now, user.ID)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "WALK_IN_FAILED", "Could not register the walk-in patient.")
		return
	}
	if created {
		s.audit(r.Context(), &user, "create", "patient", patientID, "Registered walk-in patient "+medicalRecordNumber, "", "", r)
		s.broker.Publish(realtime.Event{Type: "patient.created", EntityType: "patient", EntityID: patientID})
	}
	s.audit(r.Context(), &user, "check_in", "queue_entry", queueID, "Walk-in added to the waiting room", "", "", r)
	s.broker.Publish(realtime.Event{Type: "queue.changed", EntityType: "queue_entry", EntityID: queueID})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": queueID, "patientId": patientID, "medicalRecordNumber": medicalRecordNumber,
		"patientCreated": created, "stage": "waiting_nurse", "version": 1, "arrivedAt": now,
	})
}
