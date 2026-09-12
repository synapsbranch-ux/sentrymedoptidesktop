package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

// kioskActor attributes self-service kiosk actions in the queue/audit tables to a
// permanently inactive placeholder account, lazily created on first use (never at
// server boot) so it can never be mistaken for a real account by the first-run setup
// wizard, which gates on `COUNT(*) FROM users == 0`. active=0 excludes it from
// handleLogin, so it can never sign in even if somehow guessed.
var kioskActor = AuthUser{ID: "system-kiosk", Username: "system.kiosk", DisplayName: "Self-service kiosk", Role: "nurse"}

func (s *Server) ensureKioskActor(r *http.Request, tx *sql.Tx) error {
	_, err := tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO users(id,username,email,password_hash,display_name,role,active,created_at,updated_at)
		VALUES(?,?,NULL,'disabled',?,?,0,?,?)`, kioskActor.ID, kioskActor.Username, kioskActor.DisplayName, kioskActor.Role, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

const kioskLookupWindow = 15 * time.Minute
const kioskLookupMaxAttempts = 10

func normalizeDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Server) registerKioskRoutes(api chi.Router) {
	api.Post("/public/kiosk/lookup", s.handleKioskLookup)
	api.Post("/public/kiosk/checkin", s.handleKioskCheckIn)
}

func (s *Server) kioskThrottled(r *http.Request) bool {
	ip := clientAttributionIP(r)
	cutoff := time.Now().UTC().Add(-kioskLookupWindow).Format(time.RFC3339Nano)
	var attempts int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM kiosk_lookup_attempts WHERE ip_address=? AND attempted_at>?", ip, cutoff).Scan(&attempts)
	return attempts >= kioskLookupMaxAttempts
}

func (s *Server) recordKioskAttempt(r *http.Request) {
	_, _ = s.db.ExecContext(r.Context(), "INSERT INTO kiosk_lookup_attempts(id,ip_address,attempted_at) VALUES(?,?,?)", uuid.NewString(), clientAttributionIP(r), time.Now().UTC().Format(time.RFC3339Nano))
}

// findKioskPatient matches on last name (exact, case-insensitive) plus a digits-only
// comparison of phone/alternate phone, so formatting differences (spaces, dashes,
// country-code prefixes typed differently) do not block a legitimate match. Both fields
// must be supplied and non-trivial: this is the only identity check standing between an
// anonymous kiosk visitor and checking a patient into the waiting room.
func (s *Server) findKioskPatient(r *http.Request, phone, lastName string) (id, firstName, lastInitial string, found bool) {
	digits := normalizeDigits(phone)
	if digits == "" || strings.TrimSpace(lastName) == "" {
		return "", "", "", false
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,first_name,last_name,COALESCE(phone,''),COALESCE(alternate_phone,'') FROM patients WHERE last_name=? COLLATE NOCASE AND archived_at IS NULL", strings.TrimSpace(lastName))
	if err != nil {
		return "", "", "", false
	}
	defer rows.Close()
	for rows.Next() {
		var candidateID, first, last, candidatePhone, candidateAlt string
		if rows.Scan(&candidateID, &first, &last, &candidatePhone, &candidateAlt) != nil {
			continue
		}
		if normalizeDigits(candidatePhone) == digits || (candidateAlt != "" && normalizeDigits(candidateAlt) == digits) {
			initial := ""
			if len(last) > 0 {
				initial = strings.ToUpper(last[:1]) + "."
			}
			return candidateID, first, initial, true
		}
	}
	return "", "", "", false
}

func (s *Server) handleKioskLookup(w http.ResponseWriter, r *http.Request) {
	if s.kioskThrottled(r) {
		writeError(w, http.StatusTooManyRequests, "KIOSK_RATE_LIMITED", "Too many attempts. Please ask the front desk for help.")
		return
	}
	var input struct {
		Phone    string `json:"phone"`
		LastName string `json:"lastName"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	s.recordKioskAttempt(r)
	patientID, firstName, lastInitial, found := s.findKioskPatient(r, input.Phone, input.LastName)
	if !found {
		writeError(w, http.StatusNotFound, "KIOSK_NO_MATCH", "We could not find a matching record. Check your phone number and last name, or ask the front desk for help.")
		return
	}
	var alreadyWaiting int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM queue_entries WHERE patient_id=? AND completed_at IS NULL", patientID).Scan(&alreadyWaiting)
	today := time.Now().UTC().Format("2006-01-02")
	appointments := []map[string]any{}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,starts_at,COALESCE(reason,''),status FROM appointments
		WHERE patient_id=? AND substr(starts_at,1,10)=? AND archived_at IS NULL AND status NOT IN ('cancelled','no_show','completed') ORDER BY starts_at`, patientID, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "KIOSK_LOOKUP_FAILED", "Could not look up today's appointments.")
		return
	}
	{
		defer rows.Close()
		for rows.Next() {
			var id, startsAt, reason, status string
			if err := rows.Scan(&id, &startsAt, &reason, &status); err != nil {
				writeError(w, http.StatusInternalServerError, "KIOSK_LOOKUP_FAILED", "Could not look up today's appointments.")
				return
			}
			{
				appointments = append(appointments, map[string]any{"id": id, "startsAt": startsAt, "reason": reason, "status": status})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"patientId": patientID, "firstName": firstName, "lastInitial": lastInitial,
		"appointments": appointments, "alreadyCheckedIn": alreadyWaiting > 0,
	})
}

func (s *Server) handleKioskCheckIn(w http.ResponseWriter, r *http.Request) {
	if s.kioskThrottled(r) {
		writeError(w, http.StatusTooManyRequests, "KIOSK_RATE_LIMITED", "Too many attempts. Please ask the front desk for help.")
		return
	}
	var input struct {
		PatientID     string `json:"patientId"`
		Phone         string `json:"phone"`
		LastName      string `json:"lastName"`
		AppointmentID string `json:"appointmentId"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	s.recordKioskAttempt(r)
	// Re-validate phone+last name server-side rather than trusting the client-supplied
	// patientId: a kiosk terminal is unauthenticated, so the identity check must be
	// repeated on every state-changing call, not just the initial lookup.
	confirmedID, _, _, found := s.findKioskPatient(r, input.Phone, input.LastName)
	if !found || confirmedID != input.PatientID {
		writeError(w, http.StatusForbidden, "KIOSK_IDENTITY_MISMATCH", "We could not confirm your identity. Please ask the front desk for help.")
		return
	}
	var active int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM queue_entries WHERE patient_id=? AND completed_at IS NULL", confirmedID).Scan(&active)
	if active > 0 {
		writeError(w, http.StatusConflict, "PATIENT_ALREADY_IN_QUEUE", "You are already checked in. Please have a seat in the waiting room.")
		return
	}
	if input.AppointmentID != "" {
		var appointmentPatient string
		if err := s.db.QueryRowContext(r.Context(), "SELECT patient_id FROM appointments WHERE id=?", input.AppointmentID).Scan(&appointmentPatient); err != nil || appointmentPatient != confirmedID {
			writeError(w, http.StatusUnprocessableEntity, "APPOINTMENT_PATIENT_MISMATCH", "That appointment does not belong to you.")
			return
		}
	}
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if err := s.ensureKioskActor(r, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO queue_entries(id,patient_id,appointment_id,assigned_doctor_id,arrived_at,stage,priority,source,created_at,updated_at,updated_by)
			VALUES(?,?,?,NULL,?,'waiting_nurse',0,'kiosk',?,?,?)`, id, confirmedID, nilIfEmpty(input.AppointmentID), now, now, now, kioskActor.ID); err != nil {
			return err
		}
		if input.AppointmentID != "" {
			_, err := tx.ExecContext(r.Context(), "UPDATE appointments SET status='checked_in',version=version+1,updated_at=?,updated_by=? WHERE id=?", now, kioskActor.ID, input.AppointmentID)
			return err
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CHECK_IN_FAILED", "Could not check in. Please ask the front desk for help.")
		return
	}
	s.audit(r.Context(), &kioskActor, "check_in", "queue_entry", id, "Patient self-checked-in at the kiosk", "", "", r)
	s.broker.Publish(realtime.Event{Type: "queue.changed", EntityType: "queue_entry", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "stage": "waiting_nurse"})
}
