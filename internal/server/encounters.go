package server

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

type encounterPayload struct {
	PatientID      string `json:"patientId"`
	AppointmentID  string `json:"appointmentId"`
	VisitReason    string `json:"visitReason"`
	ChiefComplaint string `json:"chiefComplaint"`
	HPI            string `json:"hpi"`
	Assessment     string `json:"assessment"`
	TreatmentPlan  string `json:"treatmentPlan"`
	FollowUp       string `json:"followUp"`
	Version        int    `json:"version,omitempty"`
}

func (s *Server) handleEncountersList(w http.ResponseWriter, r *http.Request) {
	patientID := r.URL.Query().Get("patientId")
	where, args := "e.archived_at IS NULL", []any{}
	if patientID != "" {
		where += " AND e.patient_id=?"
		args = append(args, patientID)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT e.id,e.encounter_number,e.patient_id,p.medical_record_number,p.first_name||' '||p.last_name,COALESCE(e.appointment_id,''),COALESCE(e.doctor_id,''),COALESCE(u.display_name,''),COALESCE(e.visit_reason,''),COALESCE(e.chief_complaint,''),COALESCE(e.assessment,''),e.status,COALESCE(e.finalized_at,''),e.version,e.created_at,e.updated_at
		FROM encounters e JOIN patients p ON p.id=e.patient_id LEFT JOIN users u ON u.id=e.doctor_id WHERE `+where+` ORDER BY e.created_at DESC LIMIT 500`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_LIST_FAILED", "Could not load consultations.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, patientID, mrn, patientName, appointmentID, doctorID, doctorName, reason, complaint, assessment, status, finalizedAt, createdAt, updatedAt string
		var version int
		if err := rows.Scan(&id, &number, &patientID, &mrn, &patientName, &appointmentID, &doctorID, &doctorName, &reason, &complaint, &assessment, &status, &finalizedAt, &version, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "ENCOUNTER_LIST_FAILED", "Could not load consultations.")
			return
		}
		items = append(items, map[string]any{"id": id, "encounterNumber": number, "patientId": patientID, "medicalRecordNumber": mrn, "patientName": patientName, "appointmentId": appointmentID, "doctorId": doctorID, "doctorName": doctorName, "visitReason": reason, "chiefComplaint": complaint, "assessment": assessment, "status": status, "finalizedAt": finalizedAt, "version": version, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleEncounterCreate(w http.ResponseWriter, r *http.Request) {
	var input encounterPayload
	if err := decodeJSON(r, &input); err != nil || input.PatientID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A patient is required to start a consultation.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, pretestID, now := uuid.NewString(), uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	doctorID := any(nil)
	if user.Role == "doctor" {
		doctorID = user.ID
	}
	var number string
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "encounter", "ENC", true)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO encounters(id,encounter_number,patient_id,appointment_id,doctor_id,visit_reason,chief_complaint,hpi,assessment,treatment_plan,follow_up,status,created_at,updated_at,created_by,updated_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,'draft',?,?,?,?)`, id, number, input.PatientID, nilIfEmpty(input.AppointmentID), doctorID, nilIfEmpty(input.VisitReason), nilIfEmpty(input.ChiefComplaint), nilIfEmpty(input.HPI), nilIfEmpty(input.Assessment), nilIfEmpty(input.TreatmentPlan), nilIfEmpty(input.FollowUp), now, now, user.ID, user.ID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), "INSERT INTO pretests(id,encounter_id,chief_complaint,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?)", pretestID, id, nilIfEmpty(input.ChiefComplaint), now, now, user.ID); err != nil {
			return err
		}
		if input.AppointmentID != "" {
			_, _ = tx.ExecContext(r.Context(), "UPDATE appointments SET status='in_consultation',version=version+1,updated_at=?,updated_by=? WHERE id=?", now, user.ID, input.AppointmentID)
		}
		_, _ = tx.ExecContext(r.Context(), "UPDATE queue_entries SET encounter_id=?,stage='in_consultation',version=version+1,updated_at=?,updated_by=? WHERE patient_id=? AND completed_at IS NULL", id, now, user.ID, input.PatientID)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_CREATE_FAILED", "Could not start the consultation.")
		return
	}
	s.audit(r.Context(), &user, "create", "encounter", id, "Started consultation "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "consultation.created", EntityType: "encounter", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "encounterNumber": number, "status": "draft", "version": 1, "pretestVersion": 1})
}

func (s *Server) handleEncounterGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var encounter struct {
		ID, Number, PatientID, PatientName, AppointmentID, DoctorID, DoctorName, VisitReason, ChiefComplaint, HPI, Assessment, TreatmentPlan, FollowUp, Status, FinalizedAt, CreatedAt, UpdatedAt string
		Version                                                                                                                                                                                   int
	}
	err := s.db.QueryRowContext(r.Context(), `SELECT e.id,e.encounter_number,e.patient_id,p.first_name||' '||p.last_name,COALESCE(e.appointment_id,''),COALESCE(e.doctor_id,''),COALESCE(u.display_name,''),COALESCE(e.visit_reason,''),COALESCE(e.chief_complaint,''),COALESCE(e.hpi,''),COALESCE(e.assessment,''),COALESCE(e.treatment_plan,''),COALESCE(e.follow_up,''),e.status,COALESCE(e.finalized_at,''),e.version,e.created_at,e.updated_at
		FROM encounters e JOIN patients p ON p.id=e.patient_id LEFT JOIN users u ON u.id=e.doctor_id WHERE e.id=? AND e.archived_at IS NULL`, id).
		Scan(&encounter.ID, &encounter.Number, &encounter.PatientID, &encounter.PatientName, &encounter.AppointmentID, &encounter.DoctorID, &encounter.DoctorName, &encounter.VisitReason, &encounter.ChiefComplaint, &encounter.HPI, &encounter.Assessment, &encounter.TreatmentPlan, &encounter.FollowUp, &encounter.Status, &encounter.FinalizedAt, &encounter.Version, &encounter.CreatedAt, &encounter.UpdatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_LOAD_FAILED", "Could not load the consultation.")
		return
	}
	response := map[string]any{
		"id": encounter.ID, "encounterNumber": encounter.Number, "patientId": encounter.PatientID, "patientName": encounter.PatientName,
		"appointmentId": encounter.AppointmentID, "doctorId": encounter.DoctorID, "doctorName": encounter.DoctorName, "visitReason": encounter.VisitReason,
		"chiefComplaint": encounter.ChiefComplaint, "hpi": encounter.HPI, "assessment": encounter.Assessment, "treatmentPlan": encounter.TreatmentPlan,
		"followUp": encounter.FollowUp, "status": encounter.Status, "finalizedAt": encounter.FinalizedAt, "version": encounter.Version,
		"createdAt": encounter.CreatedAt, "updatedAt": encounter.UpdatedAt,
	}
	var pretestID, complaint, vitals, acuity, autoRefraction, keratometry, iop, pupils, eom, cover, fields, color, stereo, pachy, lensometry, completedAt, updatedAt string
	var pretestVersion int
	if err := s.db.QueryRowContext(r.Context(), `SELECT id,COALESCE(chief_complaint,''),vitals_json,visual_acuity_json,autorefraction_json,keratometry_json,iop_json,COALESCE(pupils,''),COALESCE(eom,''),COALESCE(cover_test,''),COALESCE(confrontation_fields,''),COALESCE(color_vision,''),COALESCE(stereopsis,''),pachymetry_json,lensometry_json,COALESCE(completed_at,''),version,updated_at FROM pretests WHERE encounter_id=?`, id).
		Scan(&pretestID, &complaint, &vitals, &acuity, &autoRefraction, &keratometry, &iop, &pupils, &eom, &cover, &fields, &color, &stereo, &pachy, &lensometry, &completedAt, &pretestVersion, &updatedAt); err == nil {
		response["pretest"] = map[string]any{"id": pretestID, "chiefComplaint": complaint, "vitals": rawJSON(vitals), "visualAcuity": rawJSON(acuity), "autorefraction": rawJSON(autoRefraction), "keratometry": rawJSON(keratometry), "iop": rawJSON(iop), "pupils": pupils, "eom": eom, "coverTest": cover, "confrontationFields": fields, "colorVision": color, "stereopsis": stereo, "pachymetry": rawJSON(pachy), "lensometry": rawJSON(lensometry), "completedAt": completedAt, "version": pretestVersion, "updatedAt": updatedAt}
	}
	sections := map[string]any{}
	rows, rowsErr := s.db.QueryContext(r.Context(), "SELECT section_type,data_json,version,updated_at FROM encounter_sections WHERE encounter_id=?", id)
	if rowsErr != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_LOAD_FAILED", "Could not load the consultation.")
		return
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var kind, raw, at string
			var version int
			if err := rows.Scan(&kind, &raw, &version, &at); err != nil {
				writeError(w, http.StatusInternalServerError, "ENCOUNTER_LOAD_FAILED", "Could not load the consultation.")
				return
			}
			sections[kind] = map[string]any{"data": rawJSON(raw), "version": version, "updatedAt": at}
		}
	}
	response["sections"] = sections
	diagnoses, err := s.loadDiagnoses(r, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_LOAD_FAILED", "Could not load the consultation.")
		return
	}
	addenda, err := s.loadAddenda(r, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_LOAD_FAILED", "Could not load the consultation.")
		return
	}
	response["diagnoses"] = diagnoses
	response["addenda"] = addenda
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleEncounterUpdate(w http.ResponseWriter, r *http.Request) {
	var input encounterPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Consultation fields and current version are required.")
		return
	}
	user, _ := userFromContext(r.Context())
	if user.Role != "doctor" && (input.Assessment != "" || input.TreatmentPlan != "") {
		writeError(w, http.StatusForbidden, "CLINICAL_AUTHORITY_REQUIRED", "Only a doctor may record the final assessment or treatment plan.")
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.db.ExecContext(r.Context(), `UPDATE encounters SET visit_reason=?,chief_complaint=?,hpi=?,assessment=?,treatment_plan=?,follow_up=?,doctor_id=COALESCE(doctor_id,?),version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status='draft' AND archived_at IS NULL`, nilIfEmpty(input.VisitReason), nilIfEmpty(input.ChiefComplaint), nilIfEmpty(input.HPI), nilIfEmpty(input.Assessment), nilIfEmpty(input.TreatmentPlan), nilIfEmpty(input.FollowUp), doctorIDFor(user), time.Now().UTC().Format(time.RFC3339Nano), user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENCOUNTER_UPDATE_FAILED", "Could not update the consultation.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var status, version string
		err := s.db.QueryRowContext(r.Context(), "SELECT status, CAST(version AS TEXT) FROM encounters WHERE id=?", id).Scan(&status, &version)
		if err == nil && status == "finalized" {
			writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "This consultation is finalized and locked. Add an addendum instead.")
			return
		}
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This consultation was modified by another user. Review the latest changes before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "encounter", id, "Updated consultation", "", "", r)
	s.broker.Publish(realtime.Event{Type: "consultation.changed", EntityType: "encounter", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": input.Version + 1})
}

type pretestPayload struct {
	ChiefComplaint      string         `json:"chiefComplaint"`
	Vitals              map[string]any `json:"vitals"`
	VisualAcuity        map[string]any `json:"visualAcuity"`
	Autorefraction      map[string]any `json:"autorefraction"`
	Keratometry         map[string]any `json:"keratometry"`
	IOP                 map[string]any `json:"iop"`
	Pupils              string         `json:"pupils"`
	EOM                 string         `json:"eom"`
	CoverTest           string         `json:"coverTest"`
	ConfrontationFields string         `json:"confrontationFields"`
	ColorVision         string         `json:"colorVision"`
	Stereopsis          string         `json:"stereopsis"`
	Pachymetry          map[string]any `json:"pachymetry"`
	Lensometry          map[string]any `json:"lensometry"`
	Complete            bool           `json:"complete"`
	Version             int            `json:"version"`
}

func (s *Server) handlePretestSave(w http.ResponseWriter, r *http.Request) {
	var input pretestPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Pre-test data and current version are required.")
		return
	}
	encounterID := chi.URLParam(r, "id")
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=?", encounterID).Scan(&status); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	if status == "finalized" {
		writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "Pre-test data cannot be changed after consultation finalization.")
		return
	}
	user, _ := userFromContext(r.Context())
	completed := any(nil)
	if input.Complete {
		completed = time.Now().UTC().Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE pretests SET chief_complaint=?,vitals_json=?,visual_acuity_json=?,autorefraction_json=?,keratometry_json=?,iop_json=?,pupils=?,eom=?,cover_test=?,confrontation_fields=?,color_vision=?,stereopsis=?,pachymetry_json=?,lensometry_json=?,completed_at=COALESCE(?,completed_at),version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND version=?`, nilIfEmpty(input.ChiefComplaint), marshalJSON(input.Vitals), marshalJSON(input.VisualAcuity), marshalJSON(input.Autorefraction), marshalJSON(input.Keratometry), marshalJSON(input.IOP), nilIfEmpty(input.Pupils), nilIfEmpty(input.EOM), nilIfEmpty(input.CoverTest), nilIfEmpty(input.ConfrontationFields), nilIfEmpty(input.ColorVision), nilIfEmpty(input.Stereopsis), marshalJSON(input.Pachymetry), marshalJSON(input.Lensometry), completed, time.Now().UTC().Format(time.RFC3339Nano), user.ID, encounterID, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PRETEST_UPDATE_FAILED", "Could not save pre-test data.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The pre-test was modified by another user. Review the latest changes before saving.")
		return
	}
	if input.Complete {
		_, _ = s.db.ExecContext(r.Context(), "UPDATE queue_entries SET stage='waiting_doctor',version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND completed_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), user.ID, encounterID)
	}
	s.audit(r.Context(), &user, "update", "pretest", encounterID, "Updated ophthalmic pre-test", "", "", r)
	s.broker.Publish(realtime.Event{Type: "pretest.changed", EntityType: "encounter", EntityID: encounterID})
	writeJSON(w, http.StatusOK, map[string]any{"encounterId": encounterID, "version": input.Version + 1, "complete": input.Complete})
}

type sectionPayload struct {
	Data    map[string]any `json:"data"`
	Version int            `json:"version"`
}

func (s *Server) handleEncounterSectionSave(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if user.Role != "doctor" {
		writeError(w, http.StatusForbidden, "DOCTOR_ACCESS_REQUIRED", "Only a doctor may update examination findings.")
		return
	}
	section := chi.URLParam(r, "section")
	valid := map[string]bool{"history_review": true, "current_correction": true, "objective_refraction": true, "subjective_refraction": true, "cycloplegic_refraction": true, "anterior_segment": true, "posterior_segment": true, "other_exam": true}
	if !valid[section] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_ENCOUNTER_SECTION", "Clinical section is invalid.")
		return
	}
	var input sectionPayload
	if err := decodeJSON(r, &input); err != nil || input.Version < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Section data and version are required.")
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
	if input.Version == 0 {
		_, err := s.db.ExecContext(r.Context(), `INSERT INTO encounter_sections(id,encounter_id,section_type,data_json,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?)`, uuid.NewString(), encounterID, section, marshalJSON(input.Data), now, now, user.ID)
		if err != nil {
			if isUniqueViolation(err) {
				writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This clinical section was created by another user. Reload before saving.")
				return
			}
			writeError(w, http.StatusInternalServerError, "SECTION_UPDATE_FAILED", "Could not save the clinical section.")
			return
		}
	} else {
		result, err := s.db.ExecContext(r.Context(), `UPDATE encounter_sections SET data_json=?,version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND section_type=? AND version=?`, marshalJSON(input.Data), now, user.ID, encounterID, section, input.Version)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "SECTION_UPDATE_FAILED", "Could not save the clinical section.")
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This clinical section was modified by another user. Reload before saving.")
			return
		}
	}
	s.audit(r.Context(), &user, "update", "encounter_section", encounterID+":"+section, "Updated "+section+" findings", "", "", r)
	s.broker.Publish(realtime.Event{Type: "consultation.changed", EntityType: "encounter", EntityID: encounterID})
	writeJSON(w, http.StatusOK, map[string]any{"encounterId": encounterID, "section": section, "version": input.Version + 1})
}

type diagnosisPayload struct {
	Diagnosis  string `json:"diagnosis"`
	Code       string `json:"code"`
	Laterality string `json:"laterality"`
	Notes      string `json:"notes"`
	Primary    bool   `json:"primary"`
}

func (s *Server) handleDiagnosisCreate(w http.ResponseWriter, r *http.Request) {
	var input diagnosisPayload
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.Diagnosis) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A diagnosis is required.")
		return
	}
	encounterID := chi.URLParam(r, "id")
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=?", encounterID).Scan(&status); err != nil {
		writeError(w, http.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Consultation was not found.")
		return
	}
	if status != "draft" {
		writeError(w, http.StatusLocked, "ENCOUNTER_FINALIZED", "A finalized consultation cannot be changed.")
		return
	}
	user, _ := userFromContext(r.Context())
	id := uuid.NewString()
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO diagnoses(id,encounter_id,diagnosis,code,laterality,notes,is_primary,created_at,created_by) VALUES(?,?,?,?,?,?,?,?,?)`, id, encounterID, strings.TrimSpace(input.Diagnosis), nilIfEmpty(input.Code), nilIfEmpty(input.Laterality), nilIfEmpty(input.Notes), boolInt(input.Primary), time.Now().UTC().Format(time.RFC3339Nano), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DIAGNOSIS_CREATE_FAILED", "Could not add the diagnosis.")
		return
	}
	s.audit(r.Context(), &user, "create", "diagnosis", id, "Added diagnosis to consultation", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleEncounterFinalize(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version int `json:"version"`
	}
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "The current consultation version is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	var diagnosisCount int
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM diagnoses WHERE encounter_id=?", id).Scan(&diagnosisCount)
	if diagnosisCount == 0 {
		writeError(w, http.StatusUnprocessableEntity, "DIAGNOSIS_REQUIRED", "Add at least one diagnosis before finalizing.")
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE encounters SET status='finalized',finalized_at=?,finalized_by=?,doctor_id=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status='draft'`, now, user.ID, user.ID, now, user.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "FINALIZE_FAILED", "Could not finalize the consultation.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The consultation changed before it could be finalized. Reload and review it.")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), "UPDATE queue_entries SET stage='checkout',version=version+1,updated_at=?,updated_by=? WHERE encounter_id=? AND completed_at IS NULL", now, user.ID, id)
	s.audit(r.Context(), &user, "finalize", "encounter", id, "Digitally signed and locked consultation", "", "", r)
	s.broker.Publish(realtime.Event{Type: "consultation.finalized", EntityType: "encounter", EntityID: id})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "finalized", "finalizedAt": now, "version": input.Version + 1})
}

func (s *Server) handleAddendumCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.Body) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Addendum text is required.")
		return
	}
	encounterID := chi.URLParam(r, "id")
	var status string
	if err := s.db.QueryRowContext(r.Context(), "SELECT status FROM encounters WHERE id=?", encounterID).Scan(&status); err != nil || status != "finalized" {
		writeError(w, http.StatusUnprocessableEntity, "FINALIZED_ENCOUNTER_REQUIRED", "Addenda can only be appended to a finalized consultation.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(r.Context(), "INSERT INTO encounter_addenda(id,encounter_id,body,created_at,created_by) VALUES(?,?,?,?,?)", id, encounterID, strings.TrimSpace(input.Body), now, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ADDENDUM_CREATE_FAILED", "Could not append the addendum.")
		return
	}
	s.audit(r.Context(), &user, "addendum", "encounter", encounterID, "Appended a signed consultation addendum", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "createdAt": now})
}

func (s *Server) loadDiagnoses(r *http.Request, encounterID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,diagnosis,COALESCE(code,''),COALESCE(laterality,''),COALESCE(notes,''),is_primary,created_at FROM diagnoses WHERE encounter_id=? ORDER BY is_primary DESC,created_at", encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, diagnosis, code, laterality, notes, createdAt string
		var primary bool
		if err := rows.Scan(&id, &diagnosis, &code, &laterality, &notes, &primary, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "diagnosis": diagnosis, "code": code, "laterality": laterality, "notes": notes, "primary": primary, "createdAt": createdAt})
	}
	return items, rows.Err()
}

func (s *Server) loadAddenda(r *http.Request, encounterID string) ([]map[string]string, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT a.id,a.body,a.created_at,u.display_name FROM encounter_addenda a JOIN users u ON u.id=a.created_by WHERE a.encounter_id=? ORDER BY a.created_at`, encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, body, createdAt, author string
		if err := rows.Scan(&id, &body, &createdAt, &author); err != nil {
			return nil, err
		}
		items = append(items, map[string]string{"id": id, "body": body, "createdAt": createdAt, "author": author})
	}
	return items, rows.Err()
}

func doctorIDFor(user AuthUser) any {
	if user.Role == "doctor" {
		return user.ID
	}
	return nil
}

func marshalJSON(value any) string {
	if value == nil {
		return "{}"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func rawJSON(value string) any {
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) != nil {
		return map[string]any{}
	}
	return decoded
}

type prescriptionPayload struct {
	PatientID   string         `json:"patientId"`
	EncounterID string         `json:"encounterId"`
	Type        string         `json:"type"`
	OD          map[string]any `json:"od"`
	OS          map[string]any `json:"os"`
	Details     map[string]any `json:"details"`
	Notes       string         `json:"notes"`
	ExpiresAt   string         `json:"expiresAt"`
}

func (s *Server) handlePrescriptionsList(w http.ResponseWriter, r *http.Request) {
	patientID := r.URL.Query().Get("patientId")
	where, args := "p.archived_at IS NULL", []any{}
	if patientID != "" {
		where += " AND p.patient_id=?"
		args = append(args, patientID)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.prescription_number,p.patient_id,pt.first_name||' '||pt.last_name,COALESCE(p.encounter_id,''),p.type,p.od_json,p.os_json,p.details_json,COALESCE(p.notes,''),p.issued_at,COALESCE(p.expires_at,''),p.status,p.version,u.display_name,COALESCE(signer.display_name,''),COALESCE(p.signed_at,''),CASE WHEN COALESCE(p.signature_storage_name,'')='' THEN 0 ELSE 1 END FROM prescriptions p JOIN patients pt ON pt.id=p.patient_id JOIN users u ON u.id=p.doctor_id LEFT JOIN users signer ON signer.id=p.signed_by WHERE `+where+` ORDER BY p.issued_at DESC`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PRESCRIPTION_LIST_FAILED", "Could not load prescriptions.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, patientID, patientName, encounterID, kind, od, osValue, details, notes, issuedAt, expiresAt, status, doctor, signedBy, signedAt string
		var version, signed int
		if err := rows.Scan(&id, &number, &patientID, &patientName, &encounterID, &kind, &od, &osValue, &details, &notes, &issuedAt, &expiresAt, &status, &version, &doctor, &signedBy, &signedAt, &signed); err != nil {
			writeError(w, http.StatusInternalServerError, "PRESCRIPTION_LIST_FAILED", "Could not load prescriptions.")
			return
		}
		items = append(items, map[string]any{"id": id, "prescriptionNumber": number, "patientId": patientID, "patientName": patientName, "encounterId": encounterID, "type": kind, "od": rawJSON(od), "os": rawJSON(osValue), "details": rawJSON(details), "notes": notes, "issuedAt": issuedAt, "expiresAt": expiresAt, "status": status, "version": version, "doctor": doctor, "signedBy": signedBy, "signedAt": signedAt, "signed": signed == 1, "standalone": encounterID == ""})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePrescriptionCreate(w http.ResponseWriter, r *http.Request) {
	var input prescriptionPayload
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	validTypes := map[string]bool{"spectacle": true, "contact_lens": true, "medication": true}
	if input.PatientID == "" || !validTypes[input.Type] {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PRESCRIPTION", "Patient and a valid prescription type are required.")
		return
	}
	if (input.Type == "spectacle" || input.Type == "contact_lens") && len(input.OD) == 0 && len(input.OS) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "PRESCRIPTION_VALUES_REQUIRED", "At least one OD or OS value is required.")
		return
	}
	if input.Type == "medication" {
		medication, medicationOK := input.Details["medication"].(string)
		dosage, dosageOK := input.Details["dosage"].(string)
		if !medicationOK || !dosageOK || strings.TrimSpace(medication) == "" || strings.TrimSpace(dosage) == "" {
			writeError(w, http.StatusUnprocessableEntity, "MEDICATION_DETAILS_REQUIRED", "Medication name and dosage are required.")
			return
		}
	}
	if input.ExpiresAt != "" {
		if _, err := time.Parse("2006-01-02", input.ExpiresAt); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_EXPIRATION_DATE", "Prescription expiration must be a valid date.")
			return
		}
	}
	// D4: a prescription may be issued outside a consultation, in which case it
	// simply has no encounter. When one is named it must belong to this patient,
	// or the document would be filed against somebody else's visit.
	if input.EncounterID != "" {
		var owner string
		switch err := s.db.QueryRowContext(r.Context(), "SELECT patient_id FROM encounters WHERE id=? AND archived_at IS NULL", input.EncounterID).Scan(&owner); {
		case err == sql.ErrNoRows:
			writeError(w, http.StatusUnprocessableEntity, "ENCOUNTER_NOT_FOUND", "That consultation was not found.")
			return
		case err != nil:
			writeError(w, http.StatusInternalServerError, "PRESCRIPTION_CREATE_FAILED", "Could not issue the prescription.")
			return
		case owner != input.PatientID:
			writeError(w, http.StatusUnprocessableEntity, "ENCOUNTER_PATIENT_MISMATCH", "That consultation belongs to a different patient.")
			return
		}
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	var number string
	signatureStorage, signatureMediaType := s.signatureForIssuer(r, user.ID)
	err := s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		number, err = s.nextNumber(r.Context(), tx, "prescription", "RX", true)
		if err != nil {
			return err
		}
		// D3: the issuing doctor's own signature is stamped onto the document,
		// together with who signed and when. It is looked up by the session's
		// user id, so no one can apply another clinician's signature.
		_, err = tx.ExecContext(r.Context(), `INSERT INTO prescriptions(id,prescription_number,patient_id,encounter_id,doctor_id,type,od_json,os_json,details_json,notes,issued_at,expires_at,status,signed_by,signed_at,signature_storage_name,signature_media_type,created_at,updated_at,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?, 'final',?,?,?,?,?,?,?)`, id, number, input.PatientID, nilIfEmpty(input.EncounterID), user.ID, input.Type, marshalJSON(input.OD), marshalJSON(input.OS), marshalJSON(input.Details), nilIfEmpty(input.Notes), now, nilIfEmpty(input.ExpiresAt), user.ID, now, nilIfEmpty(signatureStorage), nilIfEmpty(signatureMediaType), now, now, user.ID)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PRESCRIPTION_CREATE_FAILED", "Could not issue the prescription.")
		return
	}
	s.audit(r.Context(), &user, "issue", "prescription", id, "Issued "+input.Type+" prescription "+number, "", "", r)
	s.broker.Publish(realtime.Event{Type: "prescription.created", EntityType: "prescription", EntityID: id})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "prescriptionNumber": number, "status": "final", "version": 1, "signed": signatureStorage != "", "signedAt": now, "signedBy": user.DisplayName, "encounterId": input.EncounterID, "standalone": input.EncounterID == ""})
}

func (s *Server) handleDocumentsList(w http.ResponseWriter, r *http.Request) {
	patientID := r.URL.Query().Get("patientId")
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,COALESCE(patient_id,''),COALESCE(encounter_id,''),category,display_name,media_type,size_bytes,created_at FROM documents WHERE archived_at IS NULL AND (?='' OR patient_id=?) ORDER BY created_at DESC`, patientID, patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DOCUMENT_LIST_FAILED", "Could not load documents.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, patient, encounter, category, name, mediaType, createdAt string
		var size int64
		if err := rows.Scan(&id, &patient, &encounter, &category, &name, &mediaType, &size, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DOCUMENT_LIST_FAILED", "Could not load documents.")
			return
		}
		items = append(items, map[string]any{"id": id, "patientId": patient, "encounterId": encounter, "category": category, "displayName": name, "mediaType": mediaType, "sizeBytes": size, "createdAt": createdAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleDocumentUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 26<<20)
	if err := r.ParseMultipartForm(26 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "DOCUMENT_TOO_LARGE", "Document must be 25 MB or smaller.")
		return
	}
	patientID, encounterID, category := r.FormValue("patientId"), r.FormValue("encounterId"), strings.TrimSpace(r.FormValue("category"))
	if category == "" || (patientID == "" && encounterID == "") {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_DOCUMENT", "A category and patient or consultation are required.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "FILE_REQUIRED", "Choose a document to upload.")
		return
	}
	defer file.Close()
	extension := strings.ToLower(filepath.Ext(header.Filename))
	prefix := make([]byte, 512)
	prefixLength, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_DOCUMENT", "The document could not be read.")
		return
	}
	prefix = prefix[:prefixLength]
	detected := http.DetectContentType(prefix)
	if isTIFF(prefix) {
		// Go's content sniffer has no TIFF entry and reports application/octet-stream,
		// so scanners' TIFF output is recognised from its own magic number here.
		detected = tiffMediaType
	}
	allowed := map[string]map[string]bool{
		".pdf":  {"application/pdf": true},
		".jpg":  {"image/jpeg": true},
		".jpeg": {"image/jpeg": true},
		".png":  {"image/png": true},
		".tif":  {tiffMediaType: true},
		".tiff": {tiffMediaType: true},
		".doc":  {"application/octet-stream": true, "application/x-ole-storage": true},
		".docx": {"application/zip": true},
	}
	if !allowed[extension][detected] {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_DOCUMENT_TYPE", "Supported formats are PDF, JPG, PNG, TIFF, DOC, and DOCX.")
		return
	}
	mediaType := detected
	if extension == ".doc" {
		mediaType = "application/msword"
	} else if extension == ".docx" {
		mediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	storageName := uuid.NewString() + extension
	path := filepath.Join(s.config.DataDir, "documents", storageName)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DOCUMENT_STORE_FAILED", "Could not store the document.")
		return
	}
	hash := sha256.New()
	content := io.MultiReader(bytes.NewReader(prefix), file)
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(content, (25<<20)+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > 25<<20 {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "DOCUMENT_STORE_FAILED", "Could not store the document or it exceeds 25 MB.")
		return
	}
	user, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO documents(id,patient_id,encounter_id,category,display_name,storage_name,media_type,size_bytes,checksum_sha256,created_at,created_by) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, nilIfEmpty(patientID), nilIfEmpty(encounterID), category, filepath.Base(header.Filename), storageName, mediaType, size, hex.EncodeToString(hash.Sum(nil)), now, user.ID)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "DOCUMENT_METADATA_FAILED", "Document file was not added to the patient record.")
		return
	}
	s.audit(r.Context(), &user, "upload", "document", id, "Uploaded patient document", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "displayName": filepath.Base(header.Filename), "category": category, "mediaType": mediaType, "sizeBytes": size, "createdAt": now})
}

const tiffMediaType = "image/tiff"

func isTIFF(prefix []byte) bool {
	return len(prefix) >= 4 && (string(prefix[:4]) == "II\x2a\x00" || string(prefix[:4]) == "MM\x00\x2a")
}

func (s *Server) handleDocumentDownload(w http.ResponseWriter, r *http.Request) {
	s.serveDocument(w, r, "attachment")
}

// The in-app viewer needs the file inline; a browser will not preview a response
// sent as an attachment. The route is otherwise identical and equally protected.
func (s *Server) handleDocumentContent(w http.ResponseWriter, r *http.Request) {
	s.serveDocument(w, r, "inline")
}

func (s *Server) serveDocument(w http.ResponseWriter, r *http.Request, disposition string) {
	var storage, name, mediaType string
	err := s.db.QueryRowContext(r.Context(), "SELECT storage_name,display_name,media_type FROM documents WHERE id=? AND archived_at IS NULL", chi.URLParam(r, "id")).Scan(&storage, &name, &mediaType)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DOCUMENT_LOAD_FAILED", "Could not load the document.")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filepath.Base(name)}))
	http.ServeFile(w, r, filepath.Join(s.config.DataDir, "documents", filepath.Base(storage)))
}

func (s *Server) handleDocumentArchive(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	id := chi.URLParam(r, "id")
	result, err := s.db.ExecContext(r.Context(), "UPDATE documents SET archived_at=? WHERE id=? AND archived_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DOCUMENT_ARCHIVE_FAILED", "Could not archive the document.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found.")
		return
	}
	s.audit(r.Context(), &user, "archive", "document", id, "Archived patient document", "", "", r)
	w.WriteHeader(http.StatusNoContent)
}
