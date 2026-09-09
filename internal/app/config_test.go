package app

import (
	"os"
	"testing"
)

func TestConfigURLs(t *testing.T) {
	plain := Config{Address: ":8787"}
	if plain.Port() != "8787" || plain.LoopbackURL() != "http://127.0.0.1:8787" {
		t.Fatalf("unexpected plain URL: %s", plain.LoopbackURL())
	}
	secure := Config{Address: "127.0.0.1:9443", TLSCert: "clinic.crt", PublicURL: "https://sentrymed.clinic.local"}
	if secure.Port() != "9443" || secure.MobileURL("192.168.1.4") != secure.PublicURL {
		t.Fatalf("unexpected secure URL: %s", secure.MobileURL("192.168.1.4"))
	}
}

func TestLoadConfigRejectsIncompleteTLS(t *testing.T) {
	t.Setenv("SENTRYMED_DATA_DIR", t.TempDir())
	t.Setenv("SENTRYMED_TLS_CERT", "clinic.crt")
	t.Setenv("SENTRYMED_TLS_KEY", "")
	if _, err := LoadConfig(false); err == nil {
		t.Fatal("expected incomplete TLS configuration to fail")
	}
}

func TestLoadConfigCreatesPersistentLocalTLSForProduction(t *testing.T) {
	t.Setenv("SENTRYMED_DATA_DIR", t.TempDir())
	t.Setenv("SENTRYMED_TLS_CERT", "")
	t.Setenv("SENTRYMED_TLS_KEY", "")
	t.Setenv("SENTRYMED_TLS_CA", "")
	t.Setenv("SENTRYMED_AUTO_TLS", "true")
	config, err := LoadConfig(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{config.TLSCert, config.TLSKey, config.TLSCA} {
		if path == "" {
			t.Fatal("expected generated TLS path")
		}
		if stat, err := os.Stat(path); err != nil || stat.Size() == 0 {
			t.Fatalf("generated TLS file %q invalid: %v", path, err)
		}
	}
	if config.Scheme() != "https" {
		t.Fatalf("scheme=%s, want https", config.Scheme())
	}
}

func TestProductionRejectsUnencryptedLAN(t *testing.T) {
	t.Setenv("SENTRYMED_DATA_DIR", t.TempDir())
	t.Setenv("SENTRYMED_TLS_CERT", "")
	t.Setenv("SENTRYMED_TLS_KEY", "")
	t.Setenv("SENTRYMED_AUTO_TLS", "false")
	for _, address := range []string{":8787", "0.0.0.0:8787", "192.168.1.4:8787", "[::]:8787"} {
		t.Setenv("SENTRYMED_ADDRESS", address)
		if _, err := LoadConfig(false); err == nil {
			t.Fatalf("allowed plaintext LAN: %s", address)
		}
	}
	t.Setenv("SENTRYMED_ADDRESS", "127.0.0.1:8787")
	if _, err := LoadConfig(false); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"http://clinic.local", "https://user:password@clinic.local", "https://clinic.local/path", "https://clinic.local?token=secret"} {
		t.Setenv("SENTRYMED_PUBLIC_URL", origin)
		if _, err := LoadConfig(false); err == nil {
			t.Fatalf("allowed unsafe public origin: %s", origin)
		}
	}
}
