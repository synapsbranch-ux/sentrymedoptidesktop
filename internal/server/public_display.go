package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type publicDisplaySettings struct {
	Enabled          bool   `json:"enabled"`
	PrivacyMode      string `json:"privacyMode"`
	ShowAppointments bool   `json:"showAppointments"`
	Announcement     string `json:"announcement"`
}

type publicClinic struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

func (s *Server) publicDisplayConfiguration(ctx context.Context) (publicDisplaySettings, publicClinic, error) {
	settings := publicDisplaySettings{PrivacyMode: "ticket_only", ShowAppointments: true, Announcement: "Welcome. Please watch the screen for your queue number."}
	clinic := publicClinic{Name: DefaultClinicName, Timezone: "America/Port-au-Prince"}
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='public_display'").Scan(&raw); err != nil && err != sql.ErrNoRows {
		return settings, clinic, err
	} else if err == nil {
		_ = json.Unmarshal([]byte(raw), &settings)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='clinic'").Scan(&raw); err != nil && err != sql.ErrNoRows {
		return settings, clinic, err
	} else if err == nil {
		_ = json.Unmarshal([]byte(raw), &clinic)
	}
	if settings.PrivacyMode != "initials" && settings.PrivacyMode != "first_name" && settings.PrivacyMode != "ticket_only" {
		settings.PrivacyMode = "ticket_only"
	}
	return settings, clinic, nil
}

func publicDisplayCode(prefix, id string) string {
	clean := strings.ToUpper(strings.ReplaceAll(id, "-", ""))
	if len(clean) > 5 {
		clean = clean[len(clean)-5:]
	}
	return prefix + "-" + clean
}

func publicPatientLabel(mode, firstName, lastName, code string) string {
	switch mode {
	case "first_name":
		if strings.TrimSpace(firstName) != "" {
			return strings.TrimSpace(firstName) + " · " + code
		}
	case "initials":
		parts := []string{}
		for _, value := range []string{firstName, lastName} {
			value = strings.TrimSpace(value)
			letters := []rune(value)
			if len(letters) > 0 {
				parts = append(parts, strings.ToUpper(string(letters[0]))+".")
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ") + " · " + code
		}
	}
	return code
}

func (s *Server) handlePublicDisplay(w http.ResponseWriter, r *http.Request) {
	settings, clinic, err := s.publicDisplayConfiguration(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PUBLIC_DISPLAY_FAILED", "Could not load the public display.")
		return
	}
	if !settings.Enabled {
		writeError(w, http.StatusNotFound, "PUBLIC_DISPLAY_DISABLED", "The clinic public display is not enabled.")
		return
	}
	averages, err := s.queueStageAverages(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PUBLIC_DISPLAY_FAILED", "Could not calculate waiting-time estimates.")
		return
	}
	queueRows, err := s.db.QueryContext(r.Context(), `SELECT q.id,p.first_name,p.last_name,q.stage,q.arrived_at,COALESCE(u.display_name,''),q.priority,
		COALESCE((SELECT entered_at FROM queue_stage_events e WHERE e.queue_entry_id=q.id AND e.exited_at IS NULL ORDER BY e.id DESC LIMIT 1),q.arrived_at)
		FROM queue_entries q JOIN patients p ON p.id=q.patient_id LEFT JOIN users u ON u.id=q.assigned_doctor_id
		WHERE q.completed_at IS NULL ORDER BY q.priority DESC,q.arrived_at`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PUBLIC_DISPLAY_FAILED", "Could not load the waiting room.")
		return
	}
	queue := []map[string]any{}
	for queueRows.Next() {
		var id, firstName, lastName, stage, arrivedAt, doctor, stageEnteredAt string
		var priority int
		if err := queueRows.Scan(&id, &firstName, &lastName, &stage, &arrivedAt, &doctor, &priority, &stageEnteredAt); err != nil {
			queueRows.Close()
			writeError(w, http.StatusInternalServerError, "PUBLIC_DISPLAY_FAILED", "Could not read the waiting room.")
			return
		}
		code := publicDisplayCode("Q", id)
		estimate, samples := estimatedQueueWait(stage, stageEnteredAt, averages)
		queue = append(queue, map[string]any{"code": code, "patientLabel": publicPatientLabel(settings.PrivacyMode, firstName, lastName, code), "stage": stage, "arrivedAt": arrivedAt, "doctor": doctor, "priority": priority, "estimatedWaitMinutes": estimate, "waitEstimateSamples": samples})
	}
	queueRows.Close()

	appointments := []map[string]string{}
	if settings.ShowAppointments {
		location, locationErr := time.LoadLocation(clinic.Timezone)
		if locationErr != nil {
			location = time.Local
		}
		now := time.Now().In(location)
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).UTC().Format(time.RFC3339Nano)
		end := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location).UTC().Format(time.RFC3339Nano)
		rows, queryErr := s.db.QueryContext(r.Context(), `SELECT a.id,p.first_name,p.last_name,a.starts_at,a.status
			FROM appointments a JOIN patients p ON p.id=a.patient_id
			WHERE a.starts_at>=? AND a.starts_at<? AND a.archived_at IS NULL AND a.status IN ('scheduled','confirmed')
			ORDER BY a.starts_at LIMIT 30`, start, end)
		if queryErr != nil {
			writeError(w, http.StatusInternalServerError, "PUBLIC_DISPLAY_FAILED", "Could not load today's appointments.")
			return
		}
		for rows.Next() {
			var id, firstName, lastName, startsAt, status string
			if err := rows.Scan(&id, &firstName, &lastName, &startsAt, &status); err != nil {
				rows.Close()
				writeError(w, http.StatusInternalServerError, "PUBLIC_DISPLAY_FAILED", "Could not read today's appointments.")
				return
			}
			code := publicDisplayCode("A", id)
			appointments = append(appointments, map[string]string{"code": code, "patientLabel": publicPatientLabel(settings.PrivacyMode, firstName, lastName, code), "startsAt": startsAt, "status": status})
		}
		rows.Close()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"clinic": clinic, "settings": settings, "queue": queue, "appointments": appointments, "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "logoUrl": "/api/v1/public/branding/logo"})
}

func (s *Server) handlePublicEvents(w http.ResponseWriter, r *http.Request) {
	s.maintenance.RLock()
	settings, _, err := s.publicDisplayConfiguration(r.Context())
	s.maintenance.RUnlock()
	if err != nil || !settings.Enabled {
		writeError(w, http.StatusNotFound, "PUBLIC_DISPLAY_DISABLED", "The clinic public display is not enabled.")
		return
	}
	s.handleEvents(w, r)
}
