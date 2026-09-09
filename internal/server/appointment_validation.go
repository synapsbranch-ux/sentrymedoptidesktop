package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) validateAppointment(r *http.Request, input *appointmentPayload) *APIError {
	if input.Duration < 10 || input.Duration > 240 {
		return &APIError{Code: "INVALID_DURATION", Message: "Appointment duration must be between 10 and 240 minutes."}
	}
	input.PatientID = strings.TrimSpace(input.PatientID)
	input.PractitionerID = strings.TrimSpace(input.PractitionerID)
	// A booking made from one of the clinic's own services carries that service's
	// name as its type, so the fixed list only governs the legacy slugs a client
	// can still send when no service is named.
	if input.ServiceItemID == "" && (!allowedValue(input.Type, []string{"eye_exam", "follow_up", "contact_lens", "optical_delivery"}) || input.Type == "") {
		return &APIError{Code: "INVALID_APPOINTMENT_TYPE", Message: "Choose a supported appointment type, or book one of the clinic's own services."}
	}
	at, err := time.Parse(time.RFC3339, input.StartsAt)
	if err != nil {
		return &APIError{Code: "INVALID_START_TIME", Message: "Start time must be an RFC3339 timestamp."}
	}
	input.StartsAt = at.UTC().Format(time.RFC3339)
	var count int
	if err := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM patients WHERE id=? AND archived_at IS NULL", input.PatientID).Scan(&count); err != nil || count != 1 {
		return &APIError{Code: "INVALID_PATIENT", Message: "Select an existing active patient."}
	}
	if input.PractitionerID != "" {
		if err := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users WHERE id=? AND role='doctor' AND active=1 AND archived_at IS NULL", input.PractitionerID).Scan(&count); err != nil || count != 1 {
			return &APIError{Code: "INVALID_PRACTITIONER", Message: "Select an active doctor."}
		}
	}
	return nil
}
