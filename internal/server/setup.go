package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/backup"
)

type setupRequest struct {
	ClinicName      string `json:"clinicName"`
	Address         string `json:"address"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
	Timezone        string `json:"timezone"`
	Currency        string `json:"currency"`
	DoctorName      string `json:"doctorName"`
	DoctorUsername  string `json:"doctorUsername"`
	DoctorEmail     string `json:"doctorEmail"`
	Password        string `json:"password"`
	BackupDirectory string `json:"backupDirectory"`
	Language        string `json:"language"`
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "SETUP_STATUS_FAILED", "Could not determine setup status.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"required": count == 0})
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if !setupRequestAllowed(r) {
		writeError(w, http.StatusForbidden, "LOCAL_SETUP_REQUIRED", "Initial setup must be completed on the server computer.")
		return
	}
	var input setupRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	for key, value := range map[string]string{"Clinic name": input.ClinicName, "Doctor name": input.DoctorName, "Doctor username": input.DoctorUsername, "Timezone": input.Timezone, "Currency": input.Currency} {
		if strings.TrimSpace(value) == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", key+" is required.")
			return
		}
	}
	if !validPassword(input.Password) {
		writeError(w, http.StatusUnprocessableEntity, "WEAK_PASSWORD", "Use a password of at least 12 characters.")
		return
	}
	if input.Language == "" {
		input.Language = "en"
	}
	if !supportedLanguage(input.Language) {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_LANGUAGE", "Choose a supported application language.")
		return
	}
	var existing int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&existing); err != nil || existing > 0 {
		writeError(w, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "Initial setup has already been completed.")
		return
	}
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "Could not secure the password.")
		return
	}
	backupDirectory, err := (backup.Service{DB: s.db, DataDir: s.config.DataDir}).ValidateDestination(r.Context(), input.BackupDirectory)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "BACKUP_DESTINATION_UNAVAILABLE", err.Error())
		return
	}
	userID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	clinic, _ := json.Marshal(map[string]any{"name": input.ClinicName, "address": input.Address, "phone": input.Phone, "email": input.Email, "timezone": input.Timezone, "currency": input.Currency})
	err = s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO users(id, username, email, password_hash, display_name, role, created_at, updated_at)
			VALUES(?, ?, NULLIF(?,''), ?, ?, 'doctor', ?, ?)`, userID, strings.TrimSpace(input.DoctorUsername), strings.TrimSpace(input.DoctorEmail), passwordHash, strings.TrimSpace(input.DoctorName), now, now); err != nil {
			return err
		}
		defaults := map[string]string{
			"clinic":    string(clinic),
			"financial": `{"currencies":["HTG","USD"],"baseCurrency":"` + strings.ToUpper(input.Currency) + `","exchangeRate":"1","taxRate":"0"}`,
			"clinical":  `{"appointmentDuration":30,"enabledSections":["visual_acuity","refraction","iop","anterior_segment","posterior_segment"]}`,
			"backup":    marshalJSON(map[string]any{"intervalHours": 4, "retentionDays": 30, "directory": backupDirectory}),
			"appearance": `{"baseColor":"zinc","accentColor":"zinc","mode":"light","radius":"medium"}`,
			"localization": marshalJSON(map[string]any{"language": input.Language}),
			"public_display": `{"enabled":false,"privacyMode":"ticket_only","showAppointments":true,"announcement":"Welcome. Please watch the screen for your queue number."}`,
		}
		for key, value := range defaults {
			if _, err := tx.ExecContext(r.Context(), "INSERT INTO settings(key, value_json, updated_at, updated_by) VALUES(?, ?, ?, ?)", key, value, now, userID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "IDENTITY_IN_USE", "That username or email is already in use.")
			return
		}
		writeError(w, http.StatusInternalServerError, "SETUP_FAILED", "Initial setup could not be completed.")
		return
	}
	s.audit(r.Context(), &AuthUser{ID: userID, Username: input.DoctorUsername, DisplayName: input.DoctorName, Role: "doctor"}, "setup_complete", "system", "", "Initial clinic setup completed", "", string(clinic), r)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ready"})
}
