package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
	_ "modernc.org/sqlite"
)

type Record struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Checksum  string `json:"checksumSha256"`
	Kind      string `json:"kind"`
	Verified  bool   `json:"verified"`
	CreatedAt string `json:"createdAt"`
}

type Service struct {
	DB      *database.DB
	DataDir string
}

type destinationSettings struct {
	Directory string `json:"directory"`
}

func (s Service) Destination(ctx context.Context) (string, error) {
	directory := filepath.Join(s.DataDir, "backups")
	var raw string
	if err := s.DB.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='backup'").Scan(&raw); err == nil {
		var settings destinationSettings
		if json.Unmarshal([]byte(raw), &settings) == nil && strings.TrimSpace(settings.Directory) != "" {
			directory = strings.TrimSpace(settings.Directory)
		}
	} else if err != sql.ErrNoRows {
		return "", err
	}
	if !filepath.IsAbs(directory) {
		return "", fmt.Errorf("backup destination must be an absolute path")
	}
	return filepath.Clean(directory), nil
}

func (s Service) ValidateDestination(ctx context.Context, directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		directory = filepath.Join(s.DataDir, "backups")
	}
	if !filepath.IsAbs(directory) {
		return "", fmt.Errorf("backup destination must be an absolute path")
	}
	directory = filepath.Clean(directory)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", fmt.Errorf("create backup destination: %w", err)
	}
	probe, err := os.CreateTemp(directory, ".sentrymed-permission-*")
	if err != nil {
		return "", fmt.Errorf("backup destination is not writable: %w", err)
	}
	name := probe.Name()
	if _, err := probe.WriteString("SentryMed backup permission check"); err != nil {
		_ = probe.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("backup destination is not writable: %w", err)
	}
	if err := probe.Sync(); err != nil {
		_ = probe.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("backup destination cannot be synchronized: %w", err)
	}
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return directory, nil
}

func (s Service) Create(ctx context.Context, kind string, userID *string) (Record, error) {
	if kind != "manual" && kind != "automatic" && kind != "pre_restore" {
		return Record{}, fmt.Errorf("invalid backup kind")
	}
	backupDir, err := s.Destination(ctx)
	if err != nil {
		return Record{}, err
	}
	if _, err := s.ValidateDestination(ctx, backupDir); err != nil {
		return Record{}, err
	}
	name := fmt.Sprintf("sentrymed-%s-%s.db", kind, time.Now().UTC().Format("20060102T150405.000000000Z"))
	path := filepath.Join(backupDir, name)
	// VACUUM INTO is an online, transactionally consistent SQLite snapshot and includes committed WAL data.
	escaped := strings.ReplaceAll(filepath.ToSlash(path), "'", "''")
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return Record{}, fmt.Errorf("create consistent SQLite snapshot: %w", err)
	}
	verified, checksum, size, err := validateFile(ctx, path)
	if err != nil || !verified {
		_ = os.Remove(path)
		if err == nil {
			err = fmt.Errorf("backup integrity check failed")
		}
		return Record{}, err
	}
	record := Record{ID: uuid.NewString(), Filename: name, Path: path, SizeBytes: size, Checksum: checksum, Kind: kind, Verified: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO backup_records(id, filename, path, size_bytes, checksum_sha256, kind, verified, created_at, created_by)
			VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?)`, record.ID, record.Filename, record.Path, record.SizeBytes, record.Checksum, record.Kind, record.CreatedAt, userID); err != nil {
			return err
		}
		if kind == "automatic" {
			_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(id,user_id,action,entity_type,entity_id,summary,created_at)
				VALUES(?,NULL,'create','backup',?,'Created and verified automatic backup',?)`, uuid.NewString(), record.ID, record.CreatedAt)
			return err
		}
		return nil
	})
	if err != nil {
		_ = os.Remove(path)
	}
	return record, err
}

func (s Service) List(ctx context.Context) ([]Record, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, filename, path, size_bytes, checksum_sha256, kind, verified, created_at
		FROM backup_records ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Record{}
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.Filename, &item.Path, &item.SizeBytes, &item.Checksum, &item.Kind, &item.Verified, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s Service) Restore(ctx context.Context, backupID string, userID string) error {
	var source, expectedChecksum string
	if err := s.DB.QueryRowContext(ctx, "SELECT path, checksum_sha256 FROM backup_records WHERE id = ? AND verified = 1", backupID).Scan(&source, &expectedChecksum); err != nil {
		return fmt.Errorf("find verified backup: %w", err)
	}
	verified, checksum, _, err := validateFile(ctx, source)
	if err != nil || !verified || checksum != expectedChecksum {
		return fmt.Errorf("backup validation failed")
	}
	if _, err := s.Create(ctx, "pre_restore", &userID); err != nil {
		return fmt.Errorf("create pre-restore safety snapshot: %w", err)
	}
	// Flush and remove the WAL before closing so replacement is portable on
	// Windows as well as Unix-like systems.
	if _, err := s.DB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint database before restore: %w", err)
	}
	if err := s.DB.Close(); err != nil {
		return fmt.Errorf("stop database writes: %w", err)
	}
	temporary := s.DB.Path + ".restore"
	if err := copyFile(source, temporary); err != nil {
		_ = s.reopen(ctx)
		return err
	}
	previous := s.DB.Path + ".before-restore"
	_ = os.Remove(previous)
	if err := os.Rename(s.DB.Path, previous); err != nil {
		_ = os.Remove(temporary)
		_ = s.reopen(ctx)
		return fmt.Errorf("preserve current database before replacement: %w", err)
	}
	if err := os.Rename(temporary, s.DB.Path); err != nil {
		_ = os.Rename(previous, s.DB.Path)
		_ = s.reopen(ctx)
		return fmt.Errorf("replace database: %w", err)
	}
	if err := s.reopen(ctx); err != nil {
		_ = os.Remove(s.DB.Path)
		_ = os.Rename(previous, s.DB.Path)
		if rollbackErr := s.reopen(ctx); rollbackErr != nil {
			return fmt.Errorf("restored database failed to open (%v), and rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("restored database failed to open; previous database was recovered: %w", err)
	}
	if err := s.DB.IntegrityCheck(ctx); err != nil {
		_ = s.DB.Close()
		_ = os.Remove(s.DB.Path)
		_ = os.Rename(previous, s.DB.Path)
		if rollbackErr := s.reopen(ctx); rollbackErr != nil {
			return fmt.Errorf("restored database failed integrity check (%v), and rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("restored database failed integrity check; previous database was recovered: %w", err)
	}
	_ = os.Remove(previous)
	return nil
}

func (s Service) reopen(ctx context.Context) error {
	reopened, err := database.Open(ctx, s.DataDir)
	if err != nil {
		return err
	}
	s.DB.DB = reopened.DB
	s.DB.Path = reopened.Path
	return nil
}

func validateFile(ctx context.Context, path string) (bool, string, int64, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return false, "", 0, err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return false, "", 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return false, "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, "", 0, err
	}
	stat, err := file.Stat()
	if err != nil {
		return false, "", 0, err
	}
	return integrity == "ok", hex.EncodeToString(hash.Sum(nil)), stat.Size(), nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
