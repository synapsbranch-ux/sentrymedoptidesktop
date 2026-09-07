package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/backup"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/realtime"
)

func (s *Server) registerSystemRoutes(r chi.Router) {
	r.Get("/dashboard", s.handleDashboard)
	r.Get("/search", s.handleSearch)
	r.Get("/network", s.handleNetwork)
	r.Get("/network/local-ca", s.handleLocalCADownload)
	r.Get("/settings", s.handleSettingsList)
	r.With(s.requireDoctor).Put("/settings/{key}", s.handleSettingsUpdate)
	r.Get("/branding/logo", s.handleClinicLogoGet)
	r.With(s.requireDoctor).Post("/branding/logo", s.handleClinicLogoUpload)
	r.With(s.requireDoctor).Get("/users", s.handleUsersList)
	r.With(s.requireDoctor).Post("/users", s.handleUsersCreate)
	r.With(s.requireDoctor).Patch("/users/{id}", s.handleUsersUpdate)
	r.With(s.requireDoctor).Post("/users/{id}/password", s.handleUserPasswordReset)
	r.With(s.requireDoctor).Get("/audit", s.handleAuditList)
	r.With(s.requireDoctor).Get("/backups", s.handleBackupsList)
	r.With(s.requireDoctor).Post("/backups", s.handleBackupCreate)
	r.With(s.requireDoctor).Post("/backups/validate-destination", s.handleBackupDestinationValidate)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusNotImplemented, "STREAMING_UNAVAILABLE", "Realtime events are unavailable on this connection.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	channel, unsubscribe := s.broker.Subscribe()
	defer unsubscribe()
	_, _ = fmt.Fprint(w, "event: connected\ndata: {\"type\":\"connected\"}\n\n")
	flusher.Flush()
	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case data, open := <-channel:
			if !open {
				return
			}
			_, _ = fmt.Fprintf(w, "event: update\ndata: %s\n\n", data)
			flusher.Flush()
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleEventRevision(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]uint64{"revision": s.broker.Revision()})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	today := time.Now().UTC().Format("2006-01-02")
	month := today[:7]
	queries := map[string]string{
		"appointmentsToday":      "SELECT COUNT(*) FROM appointments WHERE substr(starts_at,1,10)=? AND archived_at IS NULL",
		"patientsWaiting":        "SELECT COUNT(*) FROM queue_entries WHERE completed_at IS NULL",
		"consultationsCompleted": "SELECT COUNT(*) FROM encounters WHERE status='finalized' AND substr(finalized_at,1,10)=?",
		"newPatients":            "SELECT COUNT(*) FROM patients WHERE substr(created_at,1,10)=? AND archived_at IS NULL",
		"pendingLabOrders":       "SELECT COUNT(*) FROM lab_orders WHERE status NOT IN ('delivered','cancelled')",
		"readyGlasses":           "SELECT COUNT(*) FROM lab_orders WHERE status='ready'",
		"lowStockItems":          "SELECT COUNT(*) FROM inventory_items WHERE track_stock=1 AND quantity<=reorder_level AND archived_at IS NULL",
		"unpaidInvoices":         "SELECT COUNT(*) FROM invoices WHERE status IN ('issued','partially_paid','overdue') AND archived_at IS NULL",
	}
	metrics := map[string]int64{}
	for key, query := range queries {
		argument := today
		if key == "patientsWaiting" || key == "pendingLabOrders" || key == "readyGlasses" || key == "lowStockItems" || key == "unpaidInvoices" {
			argument = ""
		}
		var count int64
		var err error
		if argument == "" {
			err = s.db.QueryRowContext(r.Context(), query).Scan(&count)
		} else {
			err = s.db.QueryRowContext(r.Context(), query, argument).Scan(&count)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "DASHBOARD_FAILED", "Could not load dashboard metrics.")
			return
		}
		metrics[key] = count
	}
	user, _ := userFromContext(r.Context())
	visits := []map[string]any{}
	for offset := 6; offset >= 0; offset-- {
		day := time.Now().UTC().AddDate(0, 0, -offset).Format("2006-01-02")
		var count int64
		_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM encounters WHERE substr(created_at,1,10)=? AND archived_at IS NULL", day).Scan(&count)
		visits = append(visits, map[string]any{"label": day, "value": count})
	}
	appointmentStatuses := []map[string]any{}
	statusRows, _ := s.db.QueryContext(r.Context(), "SELECT status,COUNT(*) FROM appointments WHERE substr(starts_at,1,10)=? AND archived_at IS NULL GROUP BY status ORDER BY status", today)
	if statusRows != nil {
		for statusRows.Next() {
			var label string
			var value int64
			if statusRows.Scan(&label, &value) == nil {
				appointmentStatuses = append(appointmentStatuses, map[string]any{"label": label, "value": value})
			}
		}
		_ = statusRows.Close()
	}
	charts := map[string]any{"visits": visits, "appointmentStatus": appointmentStatuses}
	response := map[string]any{"today": metrics, "role": user.Role, "charts": charts}
	if user.Role == "doctor" {
		var baseCurrency string
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.currency'),'HTG') FROM settings WHERE key='clinic'`).Scan(&baseCurrency)
		if baseCurrency == "" {
			baseCurrency = "HTG"
		}
		var todayPayments, todayRefunds, monthPayments, monthRefunds, outstanding, expenses int64
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,10)=?", today).Scan(&todayPayments)
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(r.amount_minor*CAST(p.exchange_rate AS REAL)) AS INTEGER)),0) FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE substr(r.refunded_at,1,10)=?", today).Scan(&todayRefunds)
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,7)=?", month).Scan(&monthPayments)
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(r.amount_minor*CAST(p.exchange_rate AS REAL)) AS INTEGER)),0) FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE substr(r.refunded_at,1,7)=?", month).Scan(&monthRefunds)
		todayRevenue, monthRevenue := todayPayments-todayRefunds, monthPayments-monthRefunds
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(CAST(ROUND((i.total_minor-COALESCE(p.paid,0)+COALESCE(ref.refunded,0))*CAST(i.exchange_rate AS REAL)) AS INTEGER)),0) FROM invoices i
			LEFT JOIN (SELECT invoice_id,SUM(amount_minor) paid FROM payments GROUP BY invoice_id) p ON p.invoice_id=i.id
			LEFT JOIN (SELECT p.invoice_id,SUM(r.amount_minor) refunded FROM refunds r JOIN payments p ON p.id=r.payment_id GROUP BY p.invoice_id) ref ON ref.invoice_id=i.id
			WHERE i.status IN ('issued','partially_paid','overdue')`).Scan(&outstanding)
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM expenses WHERE substr(expense_date,1,7)=?", month).Scan(&expenses)
		response["finance"] = map[string]any{"baseCurrency": baseCurrency, "todayRevenueMinor": todayRevenue, "monthRevenueMinor": monthRevenue, "monthRefundsMinor": monthRefunds, "outstandingMinor": outstanding, "monthExpensesMinor": expenses}
		dailyRevenue := []map[string]any{}
		for offset := 6; offset >= 0; offset-- {
			day := time.Now().UTC().AddDate(0, 0, -offset).Format("2006-01-02")
			var payments, refunds int64
			_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,10)=?", day).Scan(&payments)
			_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(r.amount_minor*CAST(p.exchange_rate AS REAL)) AS INTEGER)),0) FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE substr(r.refunded_at,1,10)=?", day).Scan(&refunds)
			dailyRevenue = append(dailyRevenue, map[string]any{"label": day, "value": payments - refunds})
		}
		monthlyRevenue := []map[string]any{}
		for offset := 5; offset >= 0; offset-- {
			period := time.Now().UTC().AddDate(0, -offset, 0).Format("2006-01")
			var payments, refunds int64
			_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,7)=?", period).Scan(&payments)
			_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(r.amount_minor*CAST(p.exchange_rate AS REAL)) AS INTEGER)),0) FROM refunds r JOIN payments p ON p.id=r.payment_id WHERE substr(r.refunded_at,1,7)=?", period).Scan(&refunds)
			monthlyRevenue = append(monthlyRevenue, map[string]any{"label": period, "value": payments - refunds})
		}
		salesDistribution := []map[string]any{}
		salesRows, _ := s.db.QueryContext(r.Context(), `SELECT COALESCE(ii.category,'service'),COALESCE(SUM(li.line_total_minor),0) FROM invoice_items li JOIN invoices inv ON inv.id=li.invoice_id LEFT JOIN inventory_items ii ON ii.id=li.inventory_item_id WHERE inv.status NOT IN ('cancelled','refunded') AND substr(inv.created_at,1,7)=? GROUP BY COALESCE(ii.category,'service') ORDER BY 2 DESC`, month)
		if salesRows != nil {
			for salesRows.Next() {
				var label string
				var value int64
				if salesRows.Scan(&label, &value) == nil {
					salesDistribution = append(salesDistribution, map[string]any{"label": label, "value": value})
				}
			}
			_ = salesRows.Close()
		}
		charts["dailyRevenue"] = dailyRevenue
		charts["monthlyRevenue"] = monthlyRevenue
		charts["salesDistribution"] = salesDistribution
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	like := "%" + query + "%"
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT 'patient', id, medical_record_number, first_name || ' ' || last_name FROM patients
		 WHERE archived_at IS NULL AND (medical_record_number LIKE ? OR first_name LIKE ? OR last_name LIKE ? OR phone LIKE ?)
		UNION ALL SELECT 'invoice', id, invoice_number, status FROM invoices WHERE archived_at IS NULL AND invoice_number LIKE ?
		UNION ALL SELECT 'lab_order', id, order_number, status FROM lab_orders WHERE order_number LIKE ?
		UNION ALL SELECT 'inventory', id, sku, name FROM inventory_items WHERE archived_at IS NULL AND (sku LIKE ? OR barcode LIKE ? OR name LIKE ?)
		LIMIT 30`, like, like, like, like, like, like, like, like, like)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SEARCH_FAILED", "Clinic search failed.")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		item := map[string]string{}
		var kind, id, primary, secondary string
		if err := rows.Scan(&kind, &id, &primary, &secondary); err != nil {
			writeError(w, http.StatusInternalServerError, "SEARCH_FAILED", "Clinic search failed.")
			return
		}
		item["type"], item["id"], item["primary"], item["secondary"] = kind, id, primary, secondary
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	addresses := []string{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		list, _ := iface.Addrs()
		for _, address := range list {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() {
				addresses = append(addresses, ip.String())
			}
		}
	}
	primary := "127.0.0.1"
	if len(addresses) > 0 {
		primary = addresses[0]
	}
	if host, _, err := net.SplitHostPort(s.config.Address); err == nil && host != "" && host != "0.0.0.0" && host != "::" {
		primary = host
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": true, "addresses": addresses, "url": s.config.MobileURL(primary), "tls": s.config.TLSCert != "", "localCAAvailable": s.config.TLSCA != "", "connectedDevices": s.broker.Connected()})
}

func (s *Server) handleLocalCADownload(w http.ResponseWriter, r *http.Request) {
	if s.config.TLSCA == "" {
		writeError(w, http.StatusNotFound, "LOCAL_CA_NOT_AVAILABLE", "This server uses an externally managed certificate.")
		return
	}
	if _, err := os.Stat(s.config.TLSCA); err != nil {
		writeError(w, http.StatusNotFound, "LOCAL_CA_NOT_AVAILABLE", "Local certificate authority file was not found.")
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="sentrymed-local-ca.crt"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, s.config.TLSCA)
}

func (s *Server) handleSettingsList(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	defaults := map[string]string{
		"appearance":     `{"baseColor":"zinc","accentColor":"zinc","mode":"light","radius":"medium"}`,
		"localization":   `{"language":"en"}`,
		"public_display": `{"enabled":false,"privacyMode":"ticket_only","showAppointments":true,"announcement":"Welcome. Please watch the screen for your queue number."}`,
	}
	for key, value := range defaults {
		_, _ = s.db.ExecContext(r.Context(), "INSERT OR IGNORE INTO settings(key,value_json,updated_at) VALUES(?,?,?)", key, value, now)
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT key, value_json, version, updated_at FROM settings ORDER BY key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_FAILED", "Could not load settings.")
		return
	}
	defer rows.Close()
	items := map[string]any{}
	versions := map[string]int{}
	for rows.Next() {
		var key, raw, updated string
		var version int
		if err := rows.Scan(&key, &raw, &version, &updated); err != nil {
			writeError(w, http.StatusInternalServerError, "SETTINGS_FAILED", "Could not load settings.")
			return
		}
		var value any
		if json.Unmarshal([]byte(raw), &value) != nil {
			value = raw
		}
		items[key] = value
		versions[key] = version
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": items, "versions": versions})
}

type settingsUpdateRequest struct {
	Value   any `json:"value"`
	Version int `json:"version"`
}

func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	allowed := map[string]bool{"clinic": true, "financial": true, "clinical": true, "backup": true, "appearance": true, "localization": true, "public_display": true}
	if !allowed[key] {
		writeError(w, http.StatusNotFound, "SETTING_NOT_FOUND", "This setting cannot be changed.")
		return
	}
	var input settingsUpdateRequest
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A value and current version are required.")
		return
	}
	raw, err := json.Marshal(input.Value)
	if err != nil || len(raw) > 256*1024 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SETTING", "The setting value is invalid or too large.")
		return
	}
	if key == "appearance" {
		var value struct {
			BaseColor   string `json:"baseColor"`
			AccentColor string `json:"accentColor"`
			Mode        string `json:"mode"`
			Radius      string `json:"radius"`
		}
		baseColors := map[string]bool{"neutral": true, "zinc": true, "stone": true, "mauve": true, "olive": true, "mist": true, "taupe": true}
		accentColors := map[string]bool{"zinc": true, "red": true, "orange": true, "amber": true, "green": true, "teal": true, "blue": true, "violet": true, "rose": true}
		modes := map[string]bool{"light": true, "dark": true, "system": true}
		radii := map[string]bool{"none": true, "small": true, "medium": true, "large": true}
		if json.Unmarshal(raw, &value) != nil || !baseColors[value.BaseColor] || !accentColors[value.AccentColor] || !modes[value.Mode] || !radii[value.Radius] {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_APPEARANCE", "Choose a supported base palette, accent, mode and corner radius.")
			return
		}
	}
	if key == "localization" {
		var value struct {
			Language string `json:"language"`
		}
		if json.Unmarshal(raw, &value) != nil || !supportedLanguage(value.Language) {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_LANGUAGE", "Choose a supported application language.")
			return
		}
	}
	if key == "public_display" {
		var value publicDisplaySettings
		if json.Unmarshal(raw, &value) != nil || (value.PrivacyMode != "ticket_only" && value.PrivacyMode != "initials" && value.PrivacyMode != "first_name") || len([]rune(value.Announcement)) > 180 {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_PUBLIC_DISPLAY", "Public display privacy and announcement settings are invalid.")
			return
		}
	}
	if key == "clinical" {
		var value struct {
			TranscriptionEnabled bool                       `json:"transcriptionEnabled"`
			TranscriptionCommand string                     `json:"transcriptionCommand"`
			Analytics            *clinicalAnalyticsSettings `json:"analytics"`
		}
		if json.Unmarshal(raw, &value) != nil || len(value.TranscriptionCommand) > 1000 {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_CLINICAL_SETTING", "Clinical settings are invalid.")
			return
		}
		if value.TranscriptionEnabled && strings.TrimSpace(value.TranscriptionCommand) == "" {
			writeError(w, http.StatusUnprocessableEntity, "TRANSCRIPTION_COMMAND_REQUIRED", "Set a transcription command before enabling transcription.")
			return
		}
		if value.Analytics != nil {
			a := value.Analytics
			validThresholds := a.RapidMyopicShiftDPerYear >= 0.1 && a.RapidMyopicShiftDPerYear <= 5 &&
				a.SignificantAcuityLoss >= 1 && a.SignificantAcuityLoss <= 10 &&
				a.ElevatedIOP >= 10 && a.ElevatedIOP <= 50 && a.IOPAsymmetry >= 1 && a.IOPAsymmetry <= 20 &&
				a.ThinCornea >= 350 && a.ThinCornea <= 700 && a.ThickCornea >= 350 && a.ThickCornea <= 700 && a.ThinCornea < a.ThickCornea &&
				a.CCTReference >= 350 && a.CCTReference <= 700 && a.CCTMicronsPerMmHg >= 0 && a.CCTMicronsPerMmHg <= 100
			if !validThresholds || (a.CCTCorrectionEnabled && a.CCTMicronsPerMmHg == 0) {
				writeError(w, http.StatusUnprocessableEntity, "INVALID_CLINICAL_THRESHOLDS", "Clinical alert thresholds or the optional CCT correction coefficient are invalid.")
				return
			}
		}
	}
	if key == "backup" {
		var value struct {
			Directory string `json:"directory"`
		}
		if json.Unmarshal(raw, &value) != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_BACKUP_SETTING", "Backup settings are invalid.")
			return
		}
		if _, err := (backup.Service{DB: s.db, DataDir: s.config.DataDir}).ValidateDestination(r.Context(), value.Directory); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "BACKUP_DESTINATION_UNAVAILABLE", err.Error())
			return
		}
	}
	user, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(r.Context(), `UPDATE settings SET value_json=?, version=version+1, updated_at=?, updated_by=? WHERE key=? AND version=?`, string(raw), now, user.ID, key, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTING_UPDATE_FAILED", "Could not update the setting.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The setting changed since it was opened. Reload before saving.")
		return
	}
	s.audit(r.Context(), &user, "update", "setting", key, "Updated system setting", "", string(raw), r)
	s.broker.Publish(realtime.Event{Type: "settings.updated", EntityType: "setting", EntityID: key})
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "version": input.Version + 1, "value": input.Value, "updatedAt": now})
}

func supportedLanguage(language string) bool {
	supported := map[string]bool{
		"en": true, "fr": true, "ht": true, "pt": true, "es": true,
		"de": true, "zh-CN": true, "ru": true, "ja": true, "ko": true, "id": true,
	}
	return supported[language]
}

func (s *Server) handlePublicLocalization(w http.ResponseWriter, r *http.Request) {
	language := "en"
	var raw string
	if err := s.db.QueryRowContext(r.Context(), "SELECT value_json FROM settings WHERE key='localization'").Scan(&raw); err == nil {
		var value struct {
			Language string `json:"language"`
		}
		if json.Unmarshal([]byte(raw), &value) == nil && supportedLanguage(value.Language) {
			language = value.Language
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"language": language})
}

type createUserRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

func (s *Server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, username, COALESCE(email,''), display_name, role, active, version, COALESCE(last_login_at,''), created_at
		FROM users WHERE archived_at IS NULL ORDER BY display_name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USERS_FAILED", "Could not load users.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, username, email, name, role, lastLogin, created string
		var active bool
		var version int
		if err := rows.Scan(&id, &username, &email, &name, &role, &active, &version, &lastLogin, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "USERS_FAILED", "Could not load users.")
			return
		}
		items = append(items, map[string]any{"id": id, "username": username, "email": email, "displayName": name, "role": role, "active": active, "version": version, "lastLoginAt": lastLogin, "createdAt": created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUsersCreate(w http.ResponseWriter, r *http.Request) {
	var input createUserRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := requireFields(map[string]string{"Username": strings.TrimSpace(input.Username), "Display name": strings.TrimSpace(input.DisplayName), "Role": input.Role}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	if input.Role != "doctor" && input.Role != "nurse" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_ROLE", "Role must be doctor or nurse.")
		return
	}
	if !validPassword(input.Password) {
		writeError(w, http.StatusUnprocessableEntity, "WEAK_PASSWORD", "Use a password of at least 12 characters.")
		return
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "Could not secure the password.")
		return
	}
	actor, _ := userFromContext(r.Context())
	id, now := uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO users(id,username,email,password_hash,display_name,role,created_at,updated_at,updated_by)
		VALUES(?,?,NULLIF(?,''),?,?,?,?,?,?)`, id, strings.TrimSpace(input.Username), strings.TrimSpace(input.Email), hash, strings.TrimSpace(input.DisplayName), input.Role, now, now, actor.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "IDENTITY_IN_USE", "That username or email is already in use.")
			return
		}
		writeError(w, http.StatusInternalServerError, "USER_CREATE_FAILED", "Could not create the user.")
		return
	}
	s.audit(r.Context(), &actor, "create", "user", id, "Created "+input.Role+" user", "", "", r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "username": input.Username, "displayName": input.DisplayName, "role": input.Role, "version": 1})
}

type updateUserRequest struct {
	DisplayName string `json:"displayName"`
	Active      bool   `json:"active"`
	Version     int    `json:"version"`
}

func (s *Server) handleUsersUpdate(w http.ResponseWriter, r *http.Request) {
	var input updateUserRequest
	if err := decodeJSON(r, &input); err != nil || input.Version < 1 || strings.TrimSpace(input.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Display name, active status, and current version are required.")
		return
	}
	actor, _ := userFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if id == actor.ID && !input.Active {
		writeError(w, http.StatusUnprocessableEntity, "CANNOT_DISABLE_SELF", "You cannot disable your own active session.")
		return
	}
	if !input.Active {
		var role string
		var activeDoctors int
		if err := s.db.QueryRowContext(r.Context(), "SELECT role FROM users WHERE id=? AND archived_at IS NULL", id).Scan(&role); err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "USER_NOT_FOUND", "User was not found.")
			return
		}
		if role == "doctor" {
			_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users WHERE role='doctor' AND active=1 AND archived_at IS NULL").Scan(&activeDoctors)
			if activeDoctors <= 1 {
				writeError(w, http.StatusUnprocessableEntity, "LAST_DOCTOR_REQUIRED", "The clinic must retain at least one active doctor account.")
				return
			}
		}
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE users SET display_name=?, active=?, version=version+1, updated_at=?, updated_by=? WHERE id=? AND version=? AND archived_at IS NULL`, strings.TrimSpace(input.DisplayName), boolInt(input.Active), time.Now().UTC().Format(time.RFC3339Nano), actor.ID, id, input.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "USER_UPDATE_FAILED", "Could not update the user.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "The user changed since it was opened.")
		return
	}
	if !input.Active {
		_, _ = s.db.ExecContext(r.Context(), "UPDATE sessions SET invalidated_at=? WHERE user_id=? AND invalidated_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), id)
	}
	s.audit(r.Context(), &actor, "update", "user", id, "Updated user account", "", "", r)
	w.WriteHeader(http.StatusNoContent)
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleUserPasswordReset(w http.ResponseWriter, r *http.Request) {
	var input resetPasswordRequest
	if err := decodeJSON(r, &input); err != nil || !validPassword(input.Password) {
		writeError(w, http.StatusUnprocessableEntity, "WEAK_PASSWORD", "Use a password of at least 12 characters.")
		return
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "Could not secure the password.")
		return
	}
	actor, _ := userFromContext(r.Context())
	id, now := chi.URLParam(r, "id"), time.Now().UTC().Format(time.RFC3339Nano)
	err = s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		result, err := tx.ExecContext(r.Context(), "UPDATE users SET password_hash=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND archived_at IS NULL", hash, now, actor.ID, id)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return sql.ErrNoRows
		}
		_, err = tx.ExecContext(r.Context(), "UPDATE sessions SET invalidated_at=? WHERE user_id=? AND invalidated_at IS NULL", now, id)
		return err
	})
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "USER_NOT_FOUND", "User was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PASSWORD_RESET_FAILED", "Could not reset the password.")
		return
	}
	s.audit(r.Context(), &actor, "password_reset", "user", id, "Reset clinic user password and invalidated sessions", "", "", r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

func (s *Server) handleClinicLogoUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+1024)
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "LOGO_TOO_LARGE", "Clinic logo must be 3 MB or smaller.")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "LOGO_REQUIRED", "Choose a PNG or JPEG logo.")
		return
	}
	defer file.Close()
	prefix := make([]byte, 512)
	count, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "INVALID_LOGO", "Logo file could not be read.")
		return
	}
	prefix = prefix[:count]
	mediaType := http.DetectContentType(prefix)
	extensions := map[string]string{"image/png": ".png", "image/jpeg": ".jpg"}
	extension, allowed := extensions[mediaType]
	if !allowed {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_LOGO_TYPE", "Clinic logo must be PNG or JPEG.")
		return
	}
	directory := filepath.Join(s.config.DataDir, "branding")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "LOGO_STORE_FAILED", "Could not prepare branding storage.")
		return
	}
	filename := "clinic-logo-" + uuid.NewString() + extension
	path := filepath.Join(directory, filename)
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LOGO_STORE_FAILED", "Could not store clinic logo.")
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(io.MultiReader(strings.NewReader(string(prefix)), file), (3<<20)+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || size > 3<<20 {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, "LOGO_STORE_FAILED", "Could not store logo or it exceeds 3 MB.")
		return
	}
	actor, _ := userFromContext(r.Context())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var previous string
	_ = s.db.QueryRowContext(r.Context(), "SELECT filename FROM branding_assets WHERE key='clinic_logo'").Scan(&previous)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO branding_assets(key,filename,media_type,size_bytes,checksum_sha256,updated_at,updated_by) VALUES('clinic_logo',?,?,?,?,?,?) ON CONFLICT(key) DO UPDATE SET filename=excluded.filename,media_type=excluded.media_type,size_bytes=excluded.size_bytes,checksum_sha256=excluded.checksum_sha256,version=branding_assets.version+1,updated_at=excluded.updated_at,updated_by=excluded.updated_by`, filename, mediaType, size, hex.EncodeToString(hash.Sum(nil)), now, actor.ID)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "LOGO_METADATA_FAILED", "Logo was not saved to clinic settings.")
		return
	}
	if previous != "" && previous != filename {
		_ = os.Remove(filepath.Join(directory, filepath.Base(previous)))
	}
	s.audit(r.Context(), &actor, "update", "branding", "clinic_logo", "Updated clinic logo", "", header.Filename, r)
	s.broker.Publish(realtime.Event{Type: "settings.updated", EntityType: "branding", EntityID: "clinic_logo"})
	writeJSON(w, http.StatusOK, map[string]any{"url": "/api/v1/branding/logo?v=" + now, "mediaType": mediaType, "sizeBytes": size})
}

func (s *Server) handleClinicLogoGet(w http.ResponseWriter, r *http.Request) {
	var filename, mediaType string
	if err := s.db.QueryRowContext(r.Context(), "SELECT filename,media_type FROM branding_assets WHERE key='clinic_logo'").Scan(&filename, &mediaType); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "LOGO_NOT_FOUND", "No clinic logo has been uploaded.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "LOGO_LOAD_FAILED", "Could not load clinic logo.")
		return
	}
	path := filepath.Join(s.config.DataDir, "branding", filepath.Base(filename))
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, path)
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if requested, _ := strconv.Atoi(r.URL.Query().Get("limit")); requested > 0 && requested <= 500 {
		limit = requested
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT a.id, COALESCE(u.display_name,'System'), a.action, a.entity_type, COALESCE(a.entity_id,''), a.summary, COALESCE(a.ip_address,''), a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id=a.user_id ORDER BY a.created_at DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUDIT_FAILED", "Could not load the audit log.")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, user, action, entityType, entityID, summary, ip, created string
		if err := rows.Scan(&id, &user, &action, &entityType, &entityID, &summary, &ip, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "AUDIT_FAILED", "Could not load the audit log.")
			return
		}
		items = append(items, map[string]string{"id": id, "user": user, "action": action, "entityType": entityType, "entityId": entityID, "summary": summary, "ipAddress": ip, "createdAt": created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBackupsList(w http.ResponseWriter, r *http.Request) {
	items, err := (backup.Service{DB: s.db, DataDir: s.config.DataDir}).List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "BACKUP_LIST_FAILED", "Could not load backups.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	record, err := (backup.Service{DB: s.db, DataDir: s.config.DataDir}).Create(r.Context(), "manual", &user.ID)
	if err != nil {
		s.logger.Error("backup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "BACKUP_FAILED", "Could not create a verified backup.")
		return
	}
	s.audit(r.Context(), &user, "create", "backup", record.ID, "Created and verified manual backup", "", "", r)
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleBackupDestinationValidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Directory string `json:"directory"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	directory, err := (backup.Service{DB: s.db, DataDir: s.config.DataDir}).ValidateDestination(r.Context(), input.Directory)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "BACKUP_DESTINATION_UNAVAILABLE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"directory": directory, "writable": true})
}

type restoreRequest struct {
	BackupID string `json:"backupId"`
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	var input restoreRequest
	if err := decodeJSON(r, &input); err != nil || input.BackupID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A backup ID is required.")
		return
	}
	user, _ := userFromContext(r.Context())
	s.maintenance.Lock()
	defer s.maintenance.Unlock()
	if err := (backup.Service{DB: s.db, DataDir: s.config.DataDir}).Restore(r.Context(), input.BackupID, user.ID); err != nil {
		s.logger.Error("restore failed", "error", err)
		writeError(w, http.StatusInternalServerError, "RESTORE_FAILED", "The backup could not be safely restored; the current database was preserved.")
		return
	}
	s.audit(r.Context(), &user, "restore", "backup", input.BackupID, "Restored a verified database backup", "", "", r)
	s.broker.Publish(realtime.Event{Type: "system.restored", EntityType: "backup", EntityID: input.BackupID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

var _ = sql.ErrNoRows
