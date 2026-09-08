package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

func (s *Server) registerClinicalRoutes(r chi.Router) {
	s.registerMacroRoutes(r)
	s.registerEyeDiagramRoutes(r)
	s.registerVisionTestRoutes(r)
	s.registerCodingRoutes(r)
	s.registerRecordingRoutes(r)
	r.Get("/patients", s.handlePatientsList)
	r.Get("/patients/filter-options", s.handlePatientFilterOptions)
	r.Post("/patients", s.handlePatientsCreate)
	r.Get("/patients/{id}", s.handlePatientGet)
	r.Put("/patients/{id}", s.handlePatientUpdate)
	r.Delete("/patients/{id}", s.handlePatientArchive)
	r.Get("/patients/{id}/history", s.handlePatientHistoryGet)
	r.Put("/patients/{id}/history", s.handlePatientHistoryUpdate)
	r.Get("/patients/{id}/timeline", s.handlePatientTimeline)
	r.Get("/patients/{id}/clinical-trends", s.handleClinicalTrends)
	r.Get("/encounters/{id}/delta", s.handleEncounterDelta)
	r.Get("/appointments", s.handleAppointmentsList)
	r.Post("/appointments", s.handleAppointmentsCreate)
	r.Put("/appointments/{id}", s.handleAppointmentUpdate)
	r.Patch("/appointments/{id}/status", s.handleAppointmentStatus)
	r.Get("/queue", s.handleQueueList)
	r.Post("/queue/check-in", s.handleQueueCheckIn)
	r.Post("/queue/walk-in", s.handleQueueWalkIn)
	r.Patch("/queue/{id}", s.handleQueueStage)
	r.Get("/encounters", s.handleEncountersList)
	r.Post("/encounters", s.handleEncounterCreate)
	r.Get("/encounters/{id}", s.handleEncounterGet)
	r.Put("/encounters/{id}", s.handleEncounterUpdate)
	r.Put("/encounters/{id}/pretest", s.handlePretestSave)
	r.Put("/encounters/{id}/sections/{section}", s.handleEncounterSectionSave)
	r.With(s.requireDoctor).Post("/encounters/{id}/diagnoses", s.handleDiagnosisCreate)
	r.With(s.requireDoctor).Post("/encounters/{id}/finalize", s.handleEncounterFinalize)
	r.With(s.requireDoctor).Post("/encounters/{id}/addenda", s.handleAddendumCreate)
	r.Get("/prescriptions", s.handlePrescriptionsList)
	r.With(s.requireDoctor).Post("/prescriptions", s.handlePrescriptionCreate)
	r.Get("/documents", s.handleDocumentsList)
	r.Post("/documents", s.handleDocumentUpload)
	r.Get("/documents/{id}/download", s.handleDocumentDownload)
	r.Get("/documents/{id}/content", s.handleDocumentContent)
	r.Delete("/documents/{id}", s.handleDocumentArchive)
}

type patientPayload struct {
	FirstName               string   `json:"firstName"`
	MiddleName              string   `json:"middleName"`
	LastName                string   `json:"lastName"`
	PreferredName           string   `json:"preferredName"`
	Sex                     string   `json:"sex"`
	DateOfBirth             string   `json:"dateOfBirth"`
	Phone                   string   `json:"phone"`
	AlternatePhone          string   `json:"alternatePhone"`
	Email                   string   `json:"email"`
	Address                 string   `json:"address"`
	City                    string   `json:"city"`
	Occupation              string   `json:"occupation"`
	Employer                string   `json:"employer"`
	PreferredLanguage       string   `json:"preferredLanguage"`
	CommunicationPreference string   `json:"communicationPreference"`
	ReferralSource          string   `json:"referralSource"`
	ReferringProvider       string   `json:"referringProvider"`
	Notes                   string   `json:"notes"`
	Tags                    []string `json:"tags"`
	Version                 int      `json:"version,omitempty"`
}

type patientRecord struct {
	ID                      string   `json:"id"`
	MedicalRecordNumber     string   `json:"medicalRecordNumber"`
	FirstName               string   `json:"firstName"`
	MiddleName              string   `json:"middleName"`
	LastName                string   `json:"lastName"`
	PreferredName           string   `json:"preferredName"`
	Sex                     string   `json:"sex"`
	DateOfBirth             string   `json:"dateOfBirth"`
	Phone                   string   `json:"phone"`
	AlternatePhone          string   `json:"alternatePhone"`
	Email                   string   `json:"email"`
	Address                 string   `json:"address"`
	City                    string   `json:"city"`
	Occupation              string   `json:"occupation"`
	Employer                string   `json:"employer"`
	PreferredLanguage       string   `json:"preferredLanguage"`
	CommunicationPreference string   `json:"communicationPreference"`
	ReferralSource          string   `json:"referralSource"`
	ReferringProvider       string   `json:"referringProvider"`
	Notes                   string   `json:"notes"`
	Tags                    []string `json:"tags"`
	Version                 int      `json:"version"`
	CreatedAt               string   `json:"createdAt"`
	UpdatedAt               string   `json:"updatedAt"`
	UpdatedBy               string   `json:"updatedBy"`
}

func scanPatient(scanner interface{ Scan(...any) error }) (patientRecord, error) {
	return scanPatientWithExtras(scanner)
}

// scanPatientWithExtras reads the standard patient columns and any additional
// values a caller selected after them, such as the derived last-visit date.
func scanPatientWithExtras(scanner interface{ Scan(...any) error }, extras ...any) (patientRecord, error) {
	var item patientRecord
	var tags string
	targets := []any{&item.ID, &item.MedicalRecordNumber, &item.FirstName, &item.MiddleName, &item.LastName, &item.PreferredName, &item.Sex, &item.DateOfBirth, &item.Phone, &item.AlternatePhone, &item.Email, &item.Address, &item.City, &item.Occupation, &item.Employer, &item.PreferredLanguage, &item.CommunicationPreference, &item.ReferralSource, &item.ReferringProvider, &item.Notes, &tags, &item.Version, &item.CreatedAt, &item.UpdatedAt, &item.UpdatedBy}
	err := scanner.Scan(append(targets, extras...)...)
	if err == nil {
		_ = json.Unmarshal([]byte(tags), &item.Tags)
		if item.Tags == nil {
			item.Tags = []string{}
		}
	}
	return item, err
}

// Qualified with the patients alias for queries that join the search index.
const prefixedPatientColumns = `p.id, p.medical_record_number, p.first_name, COALESCE(p.middle_name,''), p.last_name, COALESCE(p.preferred_name,''), COALESCE(p.sex,''), COALESCE(p.date_of_birth,''), COALESCE(p.phone,''), COALESCE(p.alternate_phone,''), COALESCE(p.email,''), COALESCE(p.address,''), COALESCE(p.city,''), COALESCE(p.occupation,''), COALESCE(p.employer,''), COALESCE(p.preferred_language,''), COALESCE(p.communication_preference,''), COALESCE(p.referral_source,''), COALESCE(p.referring_provider,''), COALESCE(p.notes,''), p.tags_json, p.version, p.created_at, p.updated_at, p.updated_by`

const patientColumns = `id, medical_record_number, first_name, COALESCE(middle_name,''), last_name, COALESCE(preferred_name,''), COALESCE(sex,''), COALESCE(date_of_birth,''), COALESCE(phone,''), COALESCE(alternate_phone,''), COALESCE(email,''), COALESCE(address,''), COALESCE(city,''), COALESCE(occupation,''), COALESCE(employer,''), COALESCE(preferred_language,''), COALESCE(communication_preference,''), COALESCE(referral_source,''), COALESCE(referring_provider,''), COALESCE(notes,''), tags_json, version, created_at, updated_at, updated_by`

func (s *Server) handlePatientsCreate(w http.ResponseWriter, r *http.Request) {
	var input patientPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input.FirstName, input.LastName = strings.TrimSpace(input.FirstName), strings.TrimSpace(input.LastName)
	if err := requireFields(map[string]string{"First name": input.FirstName, "Last name": input.LastName}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	if input.DateOfBirth != "" {
		if _, err := time.Parse("2006-01-02", input.DateOfBirth); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_DATE_OF_BIRTH", "Date of birth must use YYYY-MM-DD.")
			return
		}
	}
	var duplicateID, duplicateNumber string
	duplicateErr := s.db.QueryRowContext(r.Context(), `SELECT id, medical_record_number FROM patients WHERE archived_at IS NULL
		AND lower(first_name)=lower(?) AND lower(last_name)=lower(?) AND ((date_of_birth=? AND ?<>'') OR (phone=? AND ?<>'')) LIMIT 1`, input.FirstName, input.LastName, input.DateOfBirth, input.DateOfBirth, input.Phone, input.Phone).Scan(&duplicateID, &duplicateNumber)
	if duplicateErr == nil {
		writeJSON(w, http.StatusConflict, APIError{Code: "POSSIBLE_DUPLICATE_PATIENT", Message: "A possible duplicate patient already exists.", Details: map[string]any{"patientId": duplicateID, "medicalRecordNumber": duplicateNumber}})
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	tags, _ := json.Marshal(input.Tags)
	var number string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "patient", "PT", false)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO patients(id,medical_record_number,first_name,middle_name,last_name,preferred_name,sex,date_of_birth,phone,alternate_phone,email,address,city,occupation,employer,preferred_language,communication_preference,referral_source,referring_provider,notes,tags_json,created_at,updated_at,created_by,updated_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, number, input.FirstName, nilIfEmpty(input.MiddleName), input.LastName, nilIfEmpty(input.PreferredName), nilIfEmpty(input.Sex), nilIfEmpty(input.DateOfBirth), nilIfEmpty(input.Phone), nilIfEmpty(input.AlternatePhone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.City), nilIfEmpty(input.Occupation), nilIfEmpty(input.Employer), nilIfEmpty(input.PreferredLanguage), nilIfEmpty(input.CommunicationPreference), nilIfEmpty(input.ReferralSource), nilIfEmpty(input.ReferringProvider), nilIfEmpty(input.Notes), string(tags), now, now, user.ID, user.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), "INSERT INTO patient_histories(id,patient_id,created_at,updated_at,updated_by) VALUES(?,?,?,?,?)", uuid.NewString(), id, now, now, user.ID)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_CREATE_FAILED", "Could not create the patient.")
		return
	}
	s.audit(r.Context(), &user, "create", "patient", id, "Created patient "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "patient.created", EntityType: "patient", EntityID: id})
	item, err := scanPatient(s.db.QueryRowContext(r.Context(), "SELECT "+patientColumns+" FROM patients WHERE id=?", id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_LOAD_FAILED", "Patient was created but could not be reloaded.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handlePatientGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, err := scanPatient(s.db.QueryRowContext(r.Context(), "SELECT "+patientColumns+" FROM patients WHERE id=? AND archived_at IS NULL", id))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PATIENT_NOT_FOUND", "Patient was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_LOAD_FAILED", "Could not load the patient.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePatientUpdate(w http.ResponseWriter, r *http.Request) {
	var input patientPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Patient fields and the current version are required.")
		return
	}
	input.FirstName, input.LastName = strings.TrimSpace(input.FirstName), strings.TrimSpace(input.LastName)
	if err := requireFields(map[string]string{"First name": input.FirstName, "Last name": input.LastName}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	user, _ := userFromContext(r.Context())
	tags, _ := json.Marshal(input.Tags)
	result, err := s.db.ExecContext(r.Context(), `UPDATE patients SET first_name=?,middle_name=?,last_name=?,preferred_name=?,sex=?,date_of_birth=?,phone=?,alternate_phone=?,email=?,address=?,city=?,occupation=?,employer=?,preferred_language=?,communication_preference=?,referral_source=?,referring_provider=?,notes=?,tags_json=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, input.FirstName, nilIfEmpty(input.MiddleName), input.LastName, nilIfEmpty(input.PreferredName), nilIfEmpty(input.Sex), nilIfEmpty(input.DateOfBirth), nilIfEmpty(input.Phone), nilIfEmpty(input.AlternatePhone), nilIfEmpty(input.Email), nilIfEmpty(input.Address), nilIfEmpty(input.City), nilIfEmpty(input.Occupation), nilIfEmpty(input.Employer), nilIfEmpty(input.PreferredLanguage), nilIfEmpty(input.CommunicationPreference), nilIfEmpty(input.ReferralSource), nilIfEmpty(input.ReferringProvider), nilIfEmpty(input.Notes), string(tags), time.Now().UTC().Format(time.RFC3339Nano), user.ID, chi.URLParam(r, "id"), input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_UPDATE_FAILED", "Could not update the patient.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This patient record was modified by another user. Review the latest changes before saving.")
		return
	}
	id := chi.URLParam(r, "id")
	s.audit(r.Context(), &user, "update", "patient", id, "Updated patient demographics", "", "", r)
	s.broker.Publish(realtime.Event{Type: "patient.updated", EntityType: "patient", EntityID: id})
	item, _ := scanPatient(s.db.QueryRowContext(r.Context(), "SELECT "+patientColumns+" FROM patients WHERE id=?", id))
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePatientArchive(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	id := chi.URLParam(r, "id")
	result, err := s.db.ExecContext(r.Context(), "UPDATE patients SET archived_at=?, updated_at=?, updated_by=?, version=version+1 WHERE id=? AND archived_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_ARCHIVE_FAILED", "Could not archive the patient.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "PATIENT_NOT_FOUND", "Patient was not found.")
		return
	}
	s.audit(r.Context(), &user, "archive", "patient", id, "Archived patient record", "", "", r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePatientTimeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT 'appointment', id, starts_at, 'Appointment · ' || status FROM appointments WHERE patient_id=?
		UNION ALL SELECT 'encounter', id, created_at, 'Consultation · ' || status FROM encounters WHERE patient_id=?
		UNION ALL SELECT 'prescription', id, issued_at, 'Prescription · ' || type FROM prescriptions WHERE patient_id=?
		UNION ALL SELECT 'invoice', id, created_at, 'Invoice · ' || invoice_number FROM invoices WHERE patient_id=?
		UNION ALL SELECT 'payment', p.id, p.received_at, 'Payment · ' || p.receipt_number FROM payments p JOIN invoices i ON i.id=p.invoice_id WHERE i.patient_id=?
		UNION ALL SELECT 'lab_order', id, created_at, 'Lab order · ' || status FROM lab_orders WHERE patient_id=?
		UNION ALL SELECT 'document', id, created_at, 'Document · ' || display_name FROM documents WHERE patient_id=? AND archived_at IS NULL
		ORDER BY 3 DESC LIMIT 200`, id, id, id, id, id, id, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TIMELINE_FAILED", "Could not load patient timeline.")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var kind, entityID, at, title string
		if err := rows.Scan(&kind, &entityID, &at, &title); err != nil {
			writeError(w, http.StatusInternalServerError, "TIMELINE_FAILED", "Could not load patient timeline.")
			return
		}
		items = append(items, map[string]string{"type": kind, "id": entityID, "at": at, "title": title})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type appointmentPayload struct {
	PatientID      string `json:"patientId"`
	PractitionerID string `json:"practitionerId"`
	StartsAt       string `json:"startsAt"`
	Duration       int    `json:"durationMinutes"`
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Notes          string `json:"notes"`
	Version        int    `json:"version,omitempty"`
}

func (s *Server) handleAppointmentsList(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	where, args := "a.archived_at IS NULL", []any{}
	if date != "" {
		where += " AND substr(a.starts_at,1,10)=?"
		args = append(args, date)
	} else if from != "" || to != "" {
		if _, err := time.Parse(time.RFC3339, from); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "Appointment range start must be an RFC3339 timestamp.")
			return
		}
		if _, err := time.Parse(time.RFC3339, to); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "Appointment range end must be an RFC3339 timestamp.")
			return
		}
		where += " AND a.starts_at>=? AND a.starts_at<?"
		args = append(args, from, to)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT a.id,a.patient_id,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(a.practitioner_id,''),COALESCE(u.display_name,''),a.starts_at,a.duration_minutes,a.type,COALESCE(a.reason,''),COALESCE(a.notes,''),a.status,a.version
		FROM appointments a JOIN patients p ON p.id=a.patient_id LEFT JOIN users u ON u.id=a.practitioner_id WHERE `+where+` ORDER BY a.starts_at LIMIT 500`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "APPOINTMENT_LIST_FAILED", "Could not load appointments.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, patientID, mrn, patientName, practitionerID, practitionerName, startsAt, kind, reason, notes, status string
		var duration, version int
		if err := rows.Scan(&id, &patientID, &mrn, &patientName, &practitionerID, &practitionerName, &startsAt, &duration, &kind, &reason, &notes, &status, &version); err != nil {
			writeError(w, http.StatusInternalServerError, "APPOINTMENT_LIST_FAILED", "Could not load appointments.")
			return
		}
		items = append(items, map[string]any{"id": id, "patientId": patientID, "medicalRecordNumber": mrn, "patientName": patientName, "practitionerId": practitionerID, "practitionerName": practitionerName, "startsAt": startsAt, "durationMinutes": duration, "type": kind, "reason": reason, "notes": notes, "status": status, "version": version})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// appointmentConflict reports whether the given slot overlaps an existing active appointment.
// Appointments with no assigned practitioner share a single clinic-wide bucket (practitioner_id IS NULL)
// so that unassigned bookings still prevent double-booking the same time slot.
func (s *Server) appointmentConflict(r *http.Request, excludeID, practitionerID, startsAt string, duration int) (bool, error) {
	query := `SELECT COUNT(*) FROM appointments WHERE archived_at IS NULL AND status NOT IN ('cancelled','no_show','completed')
		AND datetime(starts_at) < datetime(?, '+'||?||' minutes') AND datetime(starts_at, '+'||duration_minutes||' minutes') > datetime(?)`
	args := []any{startsAt, duration, startsAt}
	if practitionerID != "" {
		query += " AND practitioner_id=?"
		args = append(args, practitionerID)
	} else {
		query += " AND practitioner_id IS NULL"
	}
	if excludeID != "" {
		query += " AND id<>?"
		args = append(args, excludeID)
	}
	var conflicts int
	if err := s.db.QueryRowContext(r.Context(), query, args...).Scan(&conflicts); err != nil {
		return false, err
	}
	return conflicts > 0, nil
}

func (s *Server) handleAppointmentsCreate(w http.ResponseWriter, r *http.Request) {
	var input appointmentPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if input.Duration <= 0 {
		input.Duration = 30
	}
	if err := requireFields(map[string]string{"Patient": input.PatientID, "Start time": input.StartsAt, "Appointment type": input.Type}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	if _, err := time.Parse(time.RFC3339, input.StartsAt); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_START_TIME", "Start time must be an RFC3339 timestamp.")
		return
	}
	if conflict, err := s.appointmentConflict(r, "", input.PractitionerID, input.StartsAt, input.Duration); err != nil {
		writeError(w, http.StatusInternalServerError, "APPOINTMENT_CREATE_FAILED", "Could not check for scheduling conflicts.")
		return
	} else if conflict {
		writeError(w, http.StatusConflict, "APPOINTMENT_CONFLICT", "This time slot is already booked.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO appointments(id,patient_id,practitioner_id,starts_at,duration_minutes,type,reason,notes,status,created_at,updated_at,created_by,updated_by)
		VALUES(?,?,?,?,?,?,?,?, 'scheduled',?,?,?,?)`, id, input.PatientID, nilIfEmpty(input.PractitionerID), input.StartsAt, input.Duration, input.Type, nilIfEmpty(input.Reason), nilIfEmpty(input.Notes), now, now, user.ID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "APPOINTMENT_CREATE_FAILED", "Could not create the appointment.")
		return
	}
	s.audit(r.Context(), &user, "create", "appointment", id, "Created appointment", "", "", r)
	s.broker.Publish(realtime.Event{Type: "appointment.created", EntityType: "appointment", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "scheduled", "version": 1})
}

func (s *Server) handleAppointmentUpdate(w http.ResponseWriter, r *http.Request) {
	var input appointmentPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Appointment fields and current version are required.")
		return
	}
	if input.Duration < 10 || input.Duration > 240 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_DURATION", "Appointment duration must be between 10 and 240 minutes.")
		return
	}
	if err := requireFields(map[string]string{"Patient": input.PatientID, "Start time": input.StartsAt, "Appointment type": input.Type}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	if _, err := time.Parse(time.RFC3339, input.StartsAt); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_START_TIME", "Start time must be an RFC3339 timestamp.")
		return
	}
	id := chi.URLParam(r, "id")
	if conflict, err := s.appointmentConflict(r, id, input.PractitionerID, input.StartsAt, input.Duration); err != nil {
		writeError(w, http.StatusInternalServerError, "APPOINTMENT_UPDATE_FAILED", "Could not check for scheduling conflicts.")
		return
	} else if conflict {
		writeError(w, http.StatusConflict, "APPOINTMENT_CONFLICT", "This time slot is already booked.")
		return
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE appointments SET patient_id=?,practitioner_id=?,starts_at=?,duration_minutes=?,type=?,reason=?,notes=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL AND status NOT IN ('completed','cancelled','no_show')`, input.PatientID, nilIfEmpty(input.PractitionerID), input.StartsAt, input.Duration, input.Type, nilIfEmpty(input.Reason), nilIfEmpty(input.Notes), now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "APPOINTMENT_UPDATE_FAILED", "Could not reschedule the appointment.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var status string
		if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM appointments WHERE id=?", id).Scan(&status); err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "APPOINTMENT_NOT_FOUND", "Appointment was not found.")
			return
		} else if status == "completed" || status == "cancelled" || status == "no_show" {
			writeError(w, http.StatusLocked, "APPOINTMENT_LOCKED", "Completed, cancelled or no-show appointments cannot be rescheduled.")
			return
		}
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The appointment changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "reschedule", "appointment", id, "Rescheduled appointment", "", marshalJSON(input), r)
	s.broker.Publish(realtime.Event{Type: "appointment.updated", EntityType: "appointment", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

type statusPayload struct {
	Status  string `json:"status"`
	Version int    `json:"version"`
}

func (s *Server) handleAppointmentStatus(w http.ResponseWriter, r *http.Request) {
	var input statusPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Status and current version are required.")
		return
	}
	valid := map[string]bool{"scheduled": true, "confirmed": true, "checked_in": true, "waiting": true, "in_consultation": true, "completed": true, "cancelled": true, "no_show": true}
	if !valid[input.Status] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_APPOINTMENT_STATUS", "Appointment status is invalid.")
		return
	}
	user, _ := userFromContext(r.Context())
	id := chi.URLParam(r, "id")
	result, err := s.db.ExecContext(r.Context(), "UPDATE appointments SET status=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND archived_at IS NULL", input.Status, time.Now().UTC().Format(time.RFC3339Nano), user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "APPOINTMENT_UPDATE_FAILED", "Could not update the appointment.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The appointment changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "status_change", "appointment", id, "Appointment moved to "+input.Status, "", "", r)
	s.broker.Publish(realtime.Event{Type: "appointment.updated", EntityType: "appointment", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": input.Status, "version": input.Version + 1})
}

func (s *Server) handleQueueList(w http.ResponseWriter, r *http.Request) {
	averages, err := s.queueStageAverages(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUEUE_FAILED", "Could not calculate waiting-time estimates.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT q.id,q.patient_id,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(q.appointment_id,''),COALESCE(q.encounter_id,''),COALESCE(q.assigned_doctor_id,''),COALESCE(u.display_name,''),q.arrived_at,q.stage,q.priority,q.source,q.version,q.updated_at,
		COALESCE((SELECT entered_at FROM queue_stage_events e WHERE e.queue_entry_id=q.id AND e.exited_at IS NULL ORDER BY e.id DESC LIMIT 1),q.arrived_at),
		COALESCE(q.visit_reason,(SELECT a.reason FROM appointments a WHERE a.id=q.appointment_id),''),
		COALESCE(p.phone,'')
		FROM queue_entries q JOIN patients p ON p.id=q.patient_id LEFT JOIN users u ON u.id=q.assigned_doctor_id WHERE q.completed_at IS NULL ORDER BY q.priority DESC,q.arrived_at`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUEUE_FAILED", "Could not load the waiting room.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, patientID, mrn, name, appointmentID, encounterID, doctorID, doctorName, arrivedAt, stage, source, updatedAt, stageEnteredAt, visitReason, phone string
		var priority, version int
		if err := rows.Scan(&id, &patientID, &mrn, &name, &appointmentID, &encounterID, &doctorID, &doctorName, &arrivedAt, &stage, &priority, &source, &version, &updatedAt, &stageEnteredAt, &visitReason, &phone); err != nil {
			writeError(w, http.StatusInternalServerError, "QUEUE_FAILED", "Could not load the waiting room.")
			return
		}
		estimate, samples := estimatedQueueWait(stage, stageEnteredAt, averages)
		items = append(items, map[string]any{"id": id, "patientId": patientID, "medicalRecordNumber": mrn, "patientName": name, "phone": phone, "visitReason": visitReason, "appointmentId": appointmentID, "encounterId": encounterID, "assignedDoctorId": doctorID, "assignedDoctorName": doctorName, "arrivedAt": arrivedAt, "stage": stage, "stageEnteredAt": stageEnteredAt, "estimatedWaitMinutes": estimate, "waitEstimateSamples": samples, "priority": priority, "source": source, "version": version, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type checkInPayload struct {
	PatientID      string `json:"patientId"`
	AppointmentID  string `json:"appointmentId"`
	AssignedDoctor string `json:"assignedDoctorId"`
	VisitReason    string `json:"visitReason"`
	Priority       int    `json:"priority"`
}

func (s *Server) handleQueueCheckIn(w http.ResponseWriter, r *http.Request) {
	var input checkInPayload
	if err := decodeJSON(r, &input); err != nil || input.PatientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A patient is required for check-in.")
		return
	}
	var active int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM queue_entries WHERE patient_id=? AND completed_at IS NULL", input.PatientID).Scan(&active)
	if active > 0 {
		writeError(w, http.StatusConflict, "PATIENT_ALREADY_IN_QUEUE", "This patient is already in the waiting room.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO queue_entries(id,patient_id,appointment_id,assigned_doctor_id,arrived_at,stage,priority,visit_reason,created_at,updated_at,updated_by)
			VALUES(?,?,?,?,?,'waiting_nurse',?,?,?,?,?)`, id, input.PatientID, nilIfEmpty(input.AppointmentID), nilIfEmpty(input.AssignedDoctor), now, input.Priority, nilIfEmpty(strings.TrimSpace(input.VisitReason)), now, now, user.ID); err != nil {
			return err
		}
		if input.AppointmentID != "" {
			_, err := tx.ExecContext(r.Context(), "UPDATE appointments SET status='checked_in',version=version+1,updated_at=?,updated_by=? WHERE id=?", now, user.ID, input.AppointmentID)
			return err
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CHECK_IN_FAILED", "Could not check in the patient.")
		return
	}
	s.audit(r.Context(), &user, "check_in", "queue_entry", id, "Patient checked in", "", "", r)
	s.broker.Publish(realtime.Event{Type: "queue.changed", EntityType: "queue_entry", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "stage": "waiting_nurse", "version": 1, "arrivedAt": now})
}

type queueStagePayload struct {
	Stage            string `json:"stage"`
	Priority         int    `json:"priority"`
	AssignedDoctorID string `json:"assignedDoctorId"`
	Version          int    `json:"version"`
}

func (s *Server) handleQueueStage(w http.ResponseWriter, r *http.Request) {
	var input queueStagePayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Queue stage and current version are required.")
		return
	}
	valid := map[string]bool{"checked_in": true, "waiting_nurse": true, "pre_test": true, "waiting_doctor": true, "in_consultation": true, "checkout": true, "completed": true}
	if !valid[input.Stage] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_QUEUE_STAGE", "Waiting room stage is invalid.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	completed := any(nil)
	if input.Stage == "completed" {
		completed = now
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE queue_entries SET stage=?,priority=?,assigned_doctor_id=?,completed_at=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND completed_at IS NULL`, input.Stage, input.Priority, nilIfEmpty(input.AssignedDoctorID), completed, now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUEUE_UPDATE_FAILED", "Could not update the waiting room.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This queue entry changed since it was opened.")
		return
	}
	s.audit(r.Context(), &user, "stage_change", "queue_entry", id, "Queue stage changed to "+input.Stage, "", "", r)
	s.broker.Publish(realtime.Event{Type: "queue.changed", EntityType: "queue_entry", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "stage": input.Stage, "version": input.Version + 1})
}
