package server

import (
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
