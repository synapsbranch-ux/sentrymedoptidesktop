package server

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type contextKey string

const (
	userContextKey           contextKey = "user"
	desktopRequestContextKey contextKey = "desktop-request"
)

type AuthUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

func userFromContext(ctx context.Context) (AuthUser, bool) {
	user, ok := ctx.Value(userContextKey).(AuthUser)
	return user, ok
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// DesktopHandler marks requests that originate from Wails' in-process asset
// server. The LAN HTTP server deliberately uses Handler directly, so remote
// clients cannot manufacture this marker with an HTTP header.
func DesktopHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), desktopRequestContextKey, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isDesktopRequest(r *http.Request) bool {
	desktop, _ := r.Context().Value(desktopRequestContextKey).(bool)
	return desktop
}

func setupRequestAllowed(r *http.Request) bool {
	if isDesktopRequest(r) {
		return true
	}
	ip := net.ParseIP(requestIP(r))
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	// A loopback peer alone is insufficient: a local proxy or DNS rebinding
	// page can otherwise present an arbitrary Host as first-run setup.
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	hostIP := net.ParseIP(strings.Trim(host, "[]"))
	return hostIP != nil && hostIP.IsLoopback()
}
