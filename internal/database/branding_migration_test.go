package database

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestClinicBrandingUpgradePreservesCustomNameAndLogo(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "database"), 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const (
		customName = "Cabinet Vision du Docteur"
		logoName   = "clinic-logo-custom.webp"
	)
	logoBytes := []byte("custom-logo-file-that-must-survive-the-upgrade")
	brandingDir := filepath.Join(dataDir, "branding")
	if err := os.MkdirAll(brandingDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brandingDir, logoName), logoBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	now := "2026-09-11T00:00:00Z"
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,username,password_hash,display_name,role,created_at,updated_at) VALUES('doctor-upgrade','doctor-upgrade','hash','Doctor','doctor',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings(key,value_json,updated_at,updated_by) VALUES('clinic',json_object('name',?,'currency','HTG'),?,'doctor-upgrade')`, customName, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO branding_assets(key,filename,media_type,size_bytes,checksum_sha256,updated_at,updated_by) VALUES('clinic_logo',?,'image/webp',?,'custom-checksum',?,'doctor-upgrade')`, logoName, len(logoBytes), now); err != nil {
		t.Fatal(err)
	}

	// Re-run migration 025 as if this data directory came from release 024.
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=25`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var name, filename, checksum string
	if err := db.QueryRowContext(ctx, `SELECT json_extract(value_json,'$.name') FROM settings WHERE key='clinic'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != customName {
		t.Fatalf("custom clinic name changed during upgrade: got %q, want %q", name, customName)
	}
	if err := db.QueryRowContext(ctx, `SELECT filename,checksum_sha256 FROM branding_assets WHERE key='clinic_logo'`).Scan(&filename, &checksum); err != nil {
		t.Fatal(err)
	}
	if filename != logoName || checksum != "custom-checksum" {
		t.Fatalf("custom logo metadata changed during upgrade: filename=%q checksum=%q", filename, checksum)
	}
	storedLogo, err := os.ReadFile(filepath.Join(brandingDir, filename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(storedLogo, logoBytes) {
		t.Fatal("custom logo file changed during upgrade")
	}
}

func TestClinicBrandingUpgradeReplacesOnlyPlaceholderName(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "database"), 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `INSERT INTO settings(key,value_json,updated_at) VALUES('clinic',json_object('name','SentryMed Opti'),'2026-09-11T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=25`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var name string
	if err := db.QueryRowContext(ctx, `SELECT json_extract(value_json,'$.name') FROM settings WHERE key='clinic'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Clinique Le Bon Spécialiste" {
		t.Fatalf("placeholder clinic name was not upgraded: got %q", name)
	}
}
