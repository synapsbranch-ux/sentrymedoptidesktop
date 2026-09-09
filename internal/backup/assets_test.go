package backup

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
)

func TestAssetsRejectSymlinksAndUnsafeDestinations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires Windows privileges")
	}
	ctx := context.Background()
	data := t.TempDir()
	if err := os.Mkdir(filepath.Join(data, "documents"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private.txt"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private.txt"), filepath.Join(data, "documents", "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotAssets(ctx, data, filepath.Join(t.TempDir(), "bundle")); err == nil {
		t.Fatal("followed symlink during backup")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Join(data, "documents"), alias); err != nil {
		t.Fatal(err)
	}
	s := Service{DataDir: data}
	for _, dest := range []string{filepath.Join(data, "documents", "backup"), filepath.Join(alias, "backup")} {
		if _, err := s.ValidateDestination(ctx, dest); err == nil {
			t.Fatal("backup allowed inside documents")
		}
	}
}

func TestRetentionKeepsRecoveryGenerationsAndUnrelatedFiles(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	for _, dir := range []string{"database", "backups"} {
		if err := os.Mkdir(filepath.Join(data, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := database.Open(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Service{DB: db, DataDir: data}
	for i := 0; i < 5; i++ {
		r, err := s.Create(ctx, "automatic", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE backup_records SET created_at=? WHERE id=?", time.Now().AddDate(0, 0, -60+i).UTC().Format(time.RFC3339Nano), r.ID); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "sentrymed-automatic-unrelated.db")
	if err := os.WriteFile(outside, []byte("do not delete"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO backup_records(id,filename,path,size_bytes,checksum_sha256,kind,verified,created_at) VALUES('unrelated','unrelated',?,0,'','automatic',0,'2000-01-01')`, outside); err != nil {
		t.Fatal(err)
	}
	if err := s.pruneAutomatic(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM backup_records WHERE verified=1").Scan(&count); err != nil || count != 3 {
		t.Fatalf("recovery generations: %d %v", count, err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("retention deleted unrelated file")
	}
}
