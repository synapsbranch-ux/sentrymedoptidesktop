package server

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// requestIP and its trust-decision callers (setupRequestAllowed below, and
// secureRequest in network_security.go) intentionally use the raw TCP peer,
// never clientAttributionIP's forwarded-header value: they are deciding
// whether to trust a claim (loopback setup, a co-located proxy's public
// origin), not who to blame an action on. Do not "fix" them to match
// clientAttributionIP.

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

// forwardedForHeader is written by the co-located reverse proxy documented in
// docs/DEPLOYMENT.md (deploy/caddy/Caddyfile), which overwrites rather than
// appends it so the value below always reflects the proxy's own observed peer.
const forwardedForHeader = "X-Forwarded-For"

// clientAttributionIP returns the address a request should be attributed to
// for rate-limiting and audit logging. This is a different question from the
// one requestIP answers for secureRequest and setupRequestAllowed: those need
// the raw TCP peer to decide whether to TRUST a claimed public origin or a
// loopback setup request, and must keep using requestIP directly.
//
// Once a reverse proxy fronts the backend (SENTRYMED_ADDRESS=127.0.0.1:8787),
// every request's raw peer is loopback, which is correct for that trust
// decision but wrong for attribution: it would collapse every LAN device into
// one shared rate-limit bucket and erase per-device forensic value from the
// audit log. The forwarded header is honored only when the peer is loopback,
// because in this deployment shape only the trusted co-located proxy can ever
// be that peer -- a device reaching the backend directly is never loopback,
// so it cannot forge its own attribution by sending the header itself.
func clientAttributionIP(r *http.Request) string {
	peer := requestIP(r)
	if ip := net.ParseIP(peer); ip == nil || !ip.IsLoopback() {
		return peer
	}
	values := r.Header.Values(forwardedForHeader)
	if len(values) == 0 {
		return peer
	}
	// Read the LAST hop of the LAST header line: a well-behaved proxy appends
	// (or, per our Caddyfile, overwrites with) the peer it observed, so the
	// final element is the only one the proxy itself vouched for. Taking the
	// first element instead would let a client that reaches the proxy pick its
	// own attribution by pre-seeding the header.
	hops := strings.Split(values[len(values)-1], ",")
	if forwarded := parseForwardedIP(hops[len(hops)-1]); forwarded != "" {
		return forwarded
	}
	return peer
}

// parseForwardedIP accepts a bare address, a bracketed IPv6 literal, or an
// address:port pair, and returns "" for anything it cannot canonicalize (a
// hostname, garbage text, or an empty value) so the caller falls back to the
// raw peer instead of storing an unvalidated string in a rate-limit key or an
// audit row.
func parseForwardedIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
			return ip.String()
		}
	}
	return ""
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
