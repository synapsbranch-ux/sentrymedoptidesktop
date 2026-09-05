package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
	r.Get("/settings", s.handleSettingsList)
	r.With(s.requireDoctor).Put("/settings/{key}", s.handleSettingsUpdate)
	r.With(s.requireDoctor).Get("/users", s.handleUsersList)
	r.With(s.requireDoctor).Post("/users", s.handleUsersCreate)
	r.With(s.requireDoctor).Patch("/users/{id}", s.handleUsersUpdate)
	r.With(s.requireDoctor).Get("/audit", s.handleAuditList)
	r.With(s.requireDoctor).Get("/backups", s.handleBackupsList)
	r.With(s.requireDoctor).Post("/backups", s.handleBackupCreate)
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
	response := map[string]any{"today": metrics, "role": user.Role}
	if user.Role == "doctor" {
		var baseCurrency string
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(value_json,'$.currency'),'HTG') FROM settings WHERE key='clinic'`).Scan(&baseCurrency)
		if baseCurrency == "" {
			baseCurrency = "HTG"
		}
		var todayRevenue, monthRevenue, outstanding, expenses int64
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,10)=?", today).Scan(&todayRevenue)
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM payments WHERE substr(received_at,1,7)=?", month).Scan(&monthRevenue)
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(CAST(ROUND((i.total_minor-COALESCE(p.paid,0))*CAST(i.exchange_rate AS REAL)) AS INTEGER)),0) FROM invoices i
			LEFT JOIN (SELECT invoice_id,SUM(amount_minor) paid FROM payments GROUP BY invoice_id) p ON p.invoice_id=i.id
			WHERE i.status IN ('issued','partially_paid','overdue')`).Scan(&outstanding)
		_ = s.db.QueryRowContext(r.Context(), "SELECT COALESCE(SUM(CAST(ROUND(amount_minor*CAST(exchange_rate AS REAL)) AS INTEGER)),0) FROM expenses WHERE substr(expense_date,1,7)=?", month).Scan(&expenses)
		response["finance"] = map[string]any{"baseCurrency": baseCurrency, "todayRevenueMinor": todayRevenue, "monthRevenueMinor": monthRevenue, "outstandingMinor": outstanding, "monthExpensesMinor": expenses}
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
	writeJSON(w, http.StatusOK, map[string]any{"running": true, "addresses": addresses, "url": s.config.MobileURL(primary), "tls": s.config.TLSCert != "", "connectedDevices": s.broker.Connected()})
}

func (s *Server) handleSettingsList(w http.ResponseWriter, r *http.Request) {
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
