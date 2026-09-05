package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

type patientHistory struct {
	ChronicDiseases       []string `json:"chronicDiseases"`
	Diabetes              *bool    `json:"diabetes"`
	Hypertension          *bool    `json:"hypertension"`
	CardiovascularNotes   string   `json:"cardiovascularNotes"`
	NeurologicalNotes     string   `json:"neurologicalNotes"`
	Surgeries             string   `json:"surgeries"`
	PregnancyNotes        string   `json:"pregnancyNotes"`
	TobaccoUse            string   `json:"tobaccoUse"`
	FamilyMedicalHistory  string   `json:"familyMedicalHistory"`
	FamilyOcularHistory   string   `json:"familyOcularHistory"`
	PreviousEyeSurgery    string   `json:"previousEyeSurgery"`
	OcularTrauma          string   `json:"ocularTrauma"`
	GlaucomaHistory       string   `json:"glaucomaHistory"`
	CataractHistory       string   `json:"cataractHistory"`
	RetinalDisease        string   `json:"retinalDisease"`
	PreviousGlasses       string   `json:"previousGlasses"`
	PreviousContactLenses string   `json:"previousContactLenses"`
	Version               int      `json:"version"`
	UpdatedAt             string   `json:"updatedAt"`
}

func scanPatientHistory(row interface{ Scan(...any) error }) (patientHistory, error) {
	var item patientHistory
	var chronic string
	var diabetes, hypertension sql.NullBool
	err := row.Scan(&chronic, &diabetes, &hypertension, &item.CardiovascularNotes, &item.NeurologicalNotes, &item.Surgeries, &item.PregnancyNotes, &item.TobaccoUse, &item.FamilyMedicalHistory, &item.FamilyOcularHistory, &item.PreviousEyeSurgery, &item.OcularTrauma, &item.GlaucomaHistory, &item.CataractHistory, &item.RetinalDisease, &item.PreviousGlasses, &item.PreviousContactLenses, &item.Version, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(chronic), &item.ChronicDiseases)
	if item.ChronicDiseases == nil {
		item.ChronicDiseases = []string{}
	}
	if diabetes.Valid {
		value := diabetes.Bool
		item.Diabetes = &value
	}
	if hypertension.Valid {
		value := hypertension.Bool
		item.Hypertension = &value
	}
	return item, nil
}

const patientHistoryColumns = `chronic_diseases_json,diabetes,hypertension,COALESCE(cardiovascular_notes,''),COALESCE(neurological_notes,''),COALESCE(surgeries,''),COALESCE(pregnancy_notes,''),COALESCE(tobacco_use,''),COALESCE(family_medical_history,''),COALESCE(family_ocular_history,''),COALESCE(previous_eye_surgery,''),COALESCE(ocular_trauma,''),COALESCE(glaucoma_history,''),COALESCE(cataract_history,''),COALESCE(retinal_disease,''),COALESCE(previous_glasses,''),COALESCE(previous_contact_lenses,''),version,updated_at`

func (s *Server) handlePatientHistoryGet(w http.ResponseWriter, r *http.Request) {
	item, err := scanPatientHistory(s.db.QueryRowContext(r.Context(), "SELECT "+patientHistoryColumns+" FROM patient_histories WHERE patient_id=?", chi.URLParam(r, "id")))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "PATIENT_NOT_FOUND", "Patient medical history was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_HISTORY_FAILED", "Could not load medical history.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePatientHistoryUpdate(w http.ResponseWriter, r *http.Request) {
	var input patientHistory
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Medical history and its current version are required.")
		return
	}
	patientID := chi.URLParam(r, "id")
	before, err := scanPatientHistory(s.db.QueryRowContext(r.Context(), "SELECT "+patientHistoryColumns+" FROM patient_histories WHERE patient_id=?", patientID))
	if err != nil {
		writeError(w, http.StatusNotFound, "PATIENT_NOT_FOUND", "Patient medical history was not found.")
		return
	}
	chronic, _ := json.Marshal(input.ChronicDiseases)
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE patient_histories SET chronic_diseases_json=?,diabetes=?,hypertension=?,cardiovascular_notes=?,neurological_notes=?,surgeries=?,pregnancy_notes=?,tobacco_use=?,family_medical_history=?,family_ocular_history=?,previous_eye_surgery=?,ocular_trauma=?,glaucoma_history=?,cataract_history=?,retinal_disease=?,previous_glasses=?,previous_contact_lenses=?,version=version+1,updated_at=?,updated_by=? WHERE patient_id=? AND version=?`, string(chronic), nullableBool(input.Diabetes), nullableBool(input.Hypertension), nilIfEmpty(input.CardiovascularNotes), nilIfEmpty(input.NeurologicalNotes), nilIfEmpty(input.Surgeries), nilIfEmpty(input.PregnancyNotes), nilIfEmpty(input.TobaccoUse), nilIfEmpty(input.FamilyMedicalHistory), nilIfEmpty(input.FamilyOcularHistory), nilIfEmpty(input.PreviousEyeSurgery), nilIfEmpty(input.OcularTrauma), nilIfEmpty(input.GlaucomaHistory), nilIfEmpty(input.CataractHistory), nilIfEmpty(input.RetinalDisease), nilIfEmpty(input.PreviousGlasses), nilIfEmpty(input.PreviousContactLenses), now, user.ID, patientID, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PATIENT_HISTORY_UPDATE_FAILED", "Could not update medical history.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "This medical history changed on another device. Reload before saving.")
		return
	}
	after, _ := scanPatientHistory(s.db.QueryRowContext(r.Context(), "SELECT "+patientHistoryColumns+" FROM patient_histories WHERE patient_id=?", patientID))
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	s.audit(r.Context(), &user, "update", "patient_history", patientID, "Updated structured medical and ocular history", string(beforeJSON), string(afterJSON), r)
	s.broker.Publish(realtime.Event{Type: "patient.history.updated", EntityType: "patient", EntityID: patientID})
	writeJSON(w, http.StatusOK, after)
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}
