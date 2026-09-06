package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
)

// SeedDevelopment is intentionally opt-in. Production initialization always starts clean.
func SeedDevelopment(ctx context.Context, db *database.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("development seed requires an empty database")
	}
	doctorHash, err := hashPassword("Doctor-Development-Only-2026")
	if err != nil {
		return err
	}
	nurseHash, err := hashPassword("Nurse-Development-Only-2026")
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	doctorID, nurseID := uuid.NewString(), uuid.NewString()
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, values := range [][]any{
			{doctorID, "doctor.dev", "doctor@example.invalid", doctorHash, "Dr. Development", "doctor"},
			{nurseID, "nurse.dev", "nurse@example.invalid", nurseHash, "Nurse Development", "nurse"},
		} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,username,email,password_hash,display_name,role,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, append(values, now, now)...); err != nil {
				return err
			}
		}
		clinic, _ := json.Marshal(map[string]any{"name": "Development Eye Clinic", "address": "Local development only", "phone": "", "email": "", "timezone": "America/Port-au-Prince", "currency": "HTG"})
		settings := map[string]string{
			"clinic":    string(clinic),
			"financial": `{"currencies":["HTG","USD"],"baseCurrency":"HTG","exchangeRate":"1","taxRate":"0"}`,
			"clinical":  `{"appointmentDuration":30,"enabledSections":["visual_acuity","refraction","iop","anterior_segment","posterior_segment"]}`,
			"backup":    `{"intervalHours":4,"retentionDays":30}`,
			"appearance": `{"baseColor":"zinc","accentColor":"zinc","mode":"light","radius":"medium"}`,
			"public_display": `{"enabled":false,"privacyMode":"ticket_only","showAppointments":true,"announcement":"Welcome. Please watch the screen for your queue number."}`,
		}
		for key, value := range settings {
			if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value_json,updated_at,updated_by) VALUES(?,?,?,?)", key, value, now, doctorID); err != nil {
				return err
			}
		}
		return nil
	})
}
