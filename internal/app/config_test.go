package app

import "testing"

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
