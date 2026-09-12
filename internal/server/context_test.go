package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetupRequestAccessBoundary(t *testing.T) {
	t.Run("loopback HTTP request is allowed", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/complete", nil)
		request.RemoteAddr = "127.0.0.1:1234"
		request.Host = "127.0.0.1:8787"
		if !setupRequestAllowed(request) {
			t.Fatal("loopback setup request was rejected")
		}
	})

	t.Run("LAN HTTP request is rejected", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/complete", nil)
		request.RemoteAddr = "192.168.1.25:1234"
		if setupRequestAllowed(request) {
			t.Fatal("LAN setup request was accepted")
		}
	})

	t.Run("Wails in-process request is allowed", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/complete", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		marked := DesktopHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !setupRequestAllowed(r) {
				t.Error("marked desktop setup request was rejected")
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		marked.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("desktop marker handler status = %d", response.Code)
		}
	})
}

func TestSetupRejectsReboundOrProxiedHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://attacker.example/api/v1/setup/complete", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	if setupRequestAllowed(r) {
		t.Fatal("remote Host claimed local setup")
	}
}

// TestClientAttributionIPTrustsOnlyLoopbackProxy is the acceptance test for the
// Caddy/reverse-proxy deployment shape (see docs/DEPLOYMENT.md): once the
// backend binds 127.0.0.1:8787 behind a co-located proxy, every request's raw
// peer is loopback, so rate-limiting and audit logging must fall back to the
// proxy-supplied X-Forwarded-For value instead of collapsing every LAN device
// into one shared bucket -- but only when the peer is actually the trusted
// proxy, never when a LAN client reaches the backend (and hence this code)
// directly.
func TestClientAttributionIPTrustsOnlyLoopbackProxy(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		forwarded  []string // one entry per X-Forwarded-For header line to add
		want       string
	}{
		{"proxied LAN client", "127.0.0.1:5001", []string{"192.168.1.77"}, "192.168.1.77"},
		{"loopback, no header", "127.0.0.1:5001", nil, "127.0.0.1"},
		{"non-loopback peer forging the header", "192.168.1.77:5001", []string{"10.0.0.9"}, "192.168.1.77"},
		{"appended chain, take the last hop", "127.0.0.1:5001", []string{"1.2.3.4, 192.168.1.77"}, "192.168.1.77"},
		{"client pre-seeded a fake first hop", "127.0.0.1:5001", []string{"evil, 192.168.1.77"}, "192.168.1.77"},
		{"two header lines, take the last line", "127.0.0.1:5001", []string{"1.2.3.4", "192.168.1.77"}, "192.168.1.77"},
		{"malformed value falls back to peer", "127.0.0.1:5001", []string{"not-an-ip"}, "127.0.0.1"},
		{"empty value falls back to peer", "127.0.0.1:5001", []string{""}, "127.0.0.1"},
		{"hostname is never trusted", "127.0.0.1:5001", []string{"attacker.example"}, "127.0.0.1"},
		{"IPv6 bare value", "[::1]:5001", []string{"2001:db8::5"}, "2001:db8::5"},
		{"IPv6 bracketed with port", "127.0.0.1:5001", []string{"[2001:db8::5]:443"}, "2001:db8::5"},
		{"IPv6 loopback peer, no header", "[::1]:5001", nil, "::1"},
		{"IPv4-mapped IPv6 value canonicalizes to IPv4", "127.0.0.1:5001", []string{"::ffff:192.168.1.77"}, "192.168.1.77"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = testCase.remoteAddr
			for _, value := range testCase.forwarded {
				request.Header.Add(forwardedForHeader, value)
			}
			if got := clientAttributionIP(request); got != testCase.want {
				t.Fatalf("clientAttributionIP() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// requestWithForwardedFor mirrors testApp.request (server_test.go) but adds an
// X-Forwarded-For header, simulating a request that arrived through the
// co-located reverse proxy described in docs/DEPLOYMENT.md.
func (a *testApp) requestWithForwardedFor(method, path string, body any, forwardedFor string) *httptest.ResponseRecorder {
	a.t.Helper()
	var encoded *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		encoded = bytes.NewReader(data)
	} else {
		encoded = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, encoded)
	request.RemoteAddr = "127.0.0.1:1234"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set(forwardedForHeader, forwardedFor)
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

// TestKioskThrottleIsPerDeviceBehindProxy proves the fix end to end: without
// clientAttributionIP, every proxied kiosk terminal would share one rate-limit
// bucket (kioskThrottled has no identity fallback, unlike login), so patients
// checking in on different terminals within the same window would trip each
// other's limit. This test fires the limit from one simulated device and
// confirms a second device is unaffected.
func TestKioskThrottleIsPerDeviceBehindProxy(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < kioskLookupMaxAttempts; i++ {
		response := a.requestWithForwardedFor(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "000", "lastName": "Nobody"}, "192.168.1.10")
		if response.Code != http.StatusNotFound {
			t.Fatalf("device A attempt %d status=%d, want 404", i, response.Code)
		}
	}
	throttledA := a.requestWithForwardedFor(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "000", "lastName": "Nobody"}, "192.168.1.10")
	if throttledA.Code != http.StatusTooManyRequests {
		t.Fatalf("device A should be throttled: %d %s", throttledA.Code, throttledA.Body.String())
	}
	notThrottledB := a.requestWithForwardedFor(http.MethodPost, "/api/v1/public/kiosk/lookup", map[string]any{"phone": "000", "lastName": "Nobody"}, "192.168.1.11")
	if notThrottledB.Code != http.StatusNotFound {
		t.Fatalf("device B should not share device A's throttle bucket: %d %s", notThrottledB.Code, notThrottledB.Body.String())
	}
}
