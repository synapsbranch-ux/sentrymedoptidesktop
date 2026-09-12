package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Use Go's maintained Origin/Fetch-Metadata checks, including login and setup.
// Wails requests are marked in-process; a LAN header cannot claim this exemption.
func (s *Server) protectBrowserRequests(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "CROSS_ORIGIN_REQUEST", "Open SentryMed directly to perform this action.")
	}))
	guarded := protection.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDesktopRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		// Streaming GET endpoints also validate origins; they can remain open for hours.
		if strings.HasSuffix(r.URL.Path, "/events") && r.Header.Get("Origin") != "" {
			origin, err := url.Parse(r.Header.Get("Origin"))
			scheme := "http"
			if s.secureRequest(r) {
				scheme = "https"
			}
			if err != nil || origin.Scheme != scheme || !strings.EqualFold(origin.Host, r.Host) {
				writeError(w, http.StatusForbidden, "CROSS_ORIGIN_REQUEST", "Open SentryMed directly to connect.")
				return
			}
		}
		guarded.ServeHTTP(w, r)
	})
}

// Snapshot/restore replace the database handle and must exclude every short
// request, authentication included. Streams lock only around their DB reads.
func (s *Server) guardDatabase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events" || r.URL.Path == "/api/v1/public/events" {
			next.ServeHTTP(w, r)
			return
		}
		exclusive := r.Method == http.MethodPost && (r.URL.Path == "/api/v1/backups" || r.URL.Path == "/api/v1/backups/restore")
		if exclusive {
			s.maintenance.Lock()
			defer s.maintenance.Unlock()
		} else {
			s.maintenance.RLock()
			defer s.maintenance.RUnlock()
		}
		next.ServeHTTP(w, r)
	})
}

// Support a local TLS proxy only when the configured public HTTPS origin and
// preserved Host match. Untrusted forwarding headers never grant this status.
// This intentionally uses requestIP's raw peer, not clientAttributionIP's
// forwarded-header value (context.go): this is a trust decision, not
// attribution, and must not honor a header the peer itself could set.
func (s *Server) secureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	peer := net.ParseIP(requestIP(r))
	public, err := url.Parse(s.config.PublicURL)
	return err == nil && public.Scheme == "https" && public.Host != "" && strings.EqualFold(public.Host, r.Host) && peer != nil && peer.IsLoopback()
}
