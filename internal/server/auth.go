package server

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const sessionCookie = "sentrymed_session"

type loginRequest struct {
	Identity string `json:"identity"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input.Identity = strings.TrimSpace(input.Identity)
	if len(input.Identity) > 254 || len(input.Password) > 256 || input.Identity == "" || input.Password == "" {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The username/email or password is incorrect.")
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		writeError(w, http.StatusTooManyRequests, "LOGIN_BUSY", "Sign-in is busy. Try again shortly.")
		return
	}
	ip := requestIP(r)
	cutoff := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339Nano)
	var failures int
	err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM login_attempts
		WHERE (ip_address = ? OR identity = ? COLLATE NOCASE) AND successful = 0 AND attempted_at > ?`, ip, input.Identity, cutoff).Scan(&failures)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "LOGIN_UNAVAILABLE", "Sign-in is temporarily unavailable.")
		return
	}
	if failures >= 8 {
		writeError(w, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "Too many failed sign-in attempts. Try again later.")
		return
	}
	var user AuthUser
	var passwordHash string
	err = s.db.QueryRowContext(r.Context(), `SELECT id, username, COALESCE(email,''), display_name, role, password_hash
		FROM users WHERE active = 1 AND archived_at IS NULL AND (username = ? COLLATE NOCASE OR email = ? COLLATE NOCASE)`, input.Identity, input.Identity).
		Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &passwordHash)
	// Equalize the expensive hash check for unknown and inactive identities.
	if err != nil {
		passwordHash = dummyPasswordHash
	}
	verified := verifyPassword(input.Password, passwordHash)
	success := err == nil && verified
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(r.Context(), "INSERT INTO login_attempts(identity, ip_address, successful, attempted_at) VALUES(?, ?, ?, ?)", input.Identity, ip, boolInt(success), now.Format(time.RFC3339Nano))
	if !success {
		if err != nil && err != sql.ErrNoRows {
			s.logger.Error("login query failed", "error", err)
		}
		s.audit(r.Context(), nil, "login_failed", "session", "", "Failed sign-in for "+input.Identity, "", "", r)
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The username/email or password is incorrect.")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "Could not create a secure session.")
		return
	}
	expires := now.Add(12 * time.Hour)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO sessions(id, user_id, token_hash, ip_address, user_agent, expires_at, created_at, last_seen_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), user.ID, tokenHash(token), ip, r.UserAgent(), expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SESSION_CREATE_FAILED", "Could not create a secure session.")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), "UPDATE users SET last_login_at = ? WHERE id = ?", now.Format(time.RFC3339Nano), user.ID)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.secureRequest(r), SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int((12 * time.Hour).Seconds())})
	s.audit(r.Context(), &user, "login", "session", "", "User signed in", "", "", r)
	response := map[string]any{"user": user, "expiresAt": expires}
	if isDesktopRequest(r) {
		// WebKit custom URI schemes do not consistently persist Set-Cookie.
		// This token is returned only through Wails' in-process handler.
		response["desktopSessionToken"] = token
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamLocked := r.URL.Path == "/api/v1/events"
		if streamLocked {
			s.maintenance.RLock()
		}
		defer func() {
			if streamLocked {
				s.maintenance.RUnlock()
			}
		}()
		token := sessionTokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Sign in to continue.")
			return
		}
		var user AuthUser
		var expires, lastSeen string
		err := s.db.QueryRowContext(r.Context(), `SELECT u.id, u.username, COALESCE(u.email,''), u.display_name, u.role, s.expires_at, s.last_seen_at
			FROM sessions s JOIN users u ON u.id = s.user_id
			WHERE s.token_hash = ? AND s.invalidated_at IS NULL AND u.active = 1 AND u.archived_at IS NULL`, tokenHash(token)).
			Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &expires, &lastSeen)
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, expires)
		if err != nil || parseErr != nil || time.Now().UTC().After(expiresAt) {
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
			writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Your session has expired. Sign in again.")
			return
		}
		seenAt, _ := time.Parse(time.RFC3339Nano, lastSeen)
		if time.Since(seenAt) >= 5*time.Minute {
			_, _ = s.db.ExecContext(r.Context(), "UPDATE sessions SET last_seen_at = ? WHERE token_hash = ?", time.Now().UTC().Format(time.RFC3339Nano), tokenHash(token))
		}
		if streamLocked {
			s.maintenance.RUnlock()
			streamLocked = false
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) requireDoctor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r.Context())
		if !ok || user.Role != "doctor" {
			writeError(w, http.StatusForbidden, "DOCTOR_ACCESS_REQUIRED", "This action requires doctor access.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if token := sessionTokenFromRequest(r); token != "" {
		if _, err := s.db.ExecContext(r.Context(), "UPDATE sessions SET invalidated_at = ? WHERE token_hash = ?", time.Now().UTC().Format(time.RFC3339Nano), tokenHash(token)); err != nil {
			writeError(w, http.StatusServiceUnavailable, "LOGOUT_FAILED", "Could not sign out. Please try again.")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	s.audit(r.Context(), &user, "logout", "session", "", "User signed out", "", "", r)
	w.WriteHeader(http.StatusNoContent)
}

func sessionTokenFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	if isDesktopRequest(r) {
		const scheme = "SentryMed "
		if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, scheme) {
			return strings.TrimSpace(strings.TrimPrefix(authorization, scheme))
		}
	}
	return ""
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Long-lived streams lose access after logout, expiry, deactivation or restore.
func (s *Server) eventSessionValid(r *http.Request) bool {
	s.maintenance.RLock()
	defer s.maintenance.RUnlock()
	if r.URL.Path == "/api/v1/public/events" {
		settings, _, err := s.publicDisplayConfiguration(r.Context())
		return err == nil && settings.Enabled
	}
	var expires string
	err := s.db.QueryRowContext(r.Context(), `SELECT s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.invalidated_at IS NULL AND u.active=1 AND u.archived_at IS NULL`, tokenHash(sessionTokenFromRequest(r))).Scan(&expires)
	at, parseErr := time.Parse(time.RFC3339Nano, expires)
	return err == nil && parseErr == nil && time.Now().Before(at)
}
