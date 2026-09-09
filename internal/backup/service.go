package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/synapsbranch-ux/sentrymedoptidesktop/internal/database"
	_ "modernc.org/sqlite"
)

type Record struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	Path           string `json:"path"`
	SizeBytes      int64  `json:"sizeBytes"`
	Checksum       string `json:"checksumSha256"`
	Kind           string `json:"kind"`
	Verified       bool   `json:"verified"`
	CreatedAt      string `json:"createdAt"`
	AssetsChecksum string `json:"assetsChecksumSha256"`
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
	for _, protected := range append([]string{"database"}, assetDirectories...) {
		root := filepath.Join(s.DataDir, protected)
		rel, err := filepath.Rel(root, directory)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return "", fmt.Errorf("backup destination cannot be inside live clinic storage")
		}
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", fmt.Errorf("create backup destination: %w", err)
	}
	// Resolve existing links as well as lexical paths to reject aliases back
	// into the files being snapshotted (which would recursively back up itself).
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	for _, protected := range append([]string{"database"}, assetDirectories...) {
		root, err := filepath.EvalSymlinks(filepath.Join(s.DataDir, protected))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, resolved)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return "", fmt.Errorf("backup destination cannot resolve inside live clinic storage")
		}
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
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return Record{}, fmt.Errorf("create consistent SQLite snapshot: %w", err)
	}
	_ = os.Chmod(path, 0600)
	verified, checksum, size, err := validateFile(ctx, path)
	if err != nil || !verified {
		_ = os.Remove(path)
		if err == nil {
			err = fmt.Errorf("backup integrity check failed")
		}
		return Record{}, err
	}
	assetsChecksum, err := snapshotAssets(ctx, s.DataDir, path+".files")
	if err == nil {
		_, err = validateAssets(ctx, path+".files", assetsChecksum)
	}
	if err != nil {
		_ = os.Remove(path)
		_ = os.RemoveAll(path + ".files")
		return Record{}, err
	}
	record := Record{AssetsChecksum: assetsChecksum, ID: uuid.NewString(), Filename: name, Path: path, SizeBytes: size, Checksum: checksum, Kind: kind, Verified: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO backup_records(id, filename, path, size_bytes, checksum_sha256, kind, verified, created_at, created_by, assets_checksum)
			VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, record.ID, record.Filename, record.Path, record.SizeBytes, record.Checksum, record.Kind, record.CreatedAt, userID, assetsChecksum); err != nil {
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
		_ = os.RemoveAll(path + ".files")
	}
	return record, err
}

func (s Service) List(ctx context.Context) ([]Record, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, filename, path, size_bytes, checksum_sha256, kind, verified, created_at, assets_checksum
		FROM backup_records ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Record{}
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.Filename, &item.Path, &item.SizeBytes, &item.Checksum, &item.Kind, &item.Verified, &item.CreatedAt, &item.AssetsChecksum); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s Service) Restore(ctx context.Context, backupID string, userID string) (restoreErr error) {
	var source, expectedChecksum, assetsChecksum string
	if err := s.DB.QueryRowContext(ctx, "SELECT path, checksum_sha256, assets_checksum FROM backup_records WHERE id = ? AND verified = 1", backupID).Scan(&source, &expectedChecksum, &assetsChecksum); err != nil {
		return fmt.Errorf("find verified backup: %w", err)
	}
	verified, checksum, _, err := validateFile(ctx, source)
	if err != nil || !verified || checksum != expectedChecksum {
		return fmt.Errorf("backup validation failed")
	}
	applyAssets, rollbackAssets, finishAssets, err := prepareAssetsRestore(ctx, s.DataDir, source+".files", assetsChecksum)
	if err != nil {
		return fmt.Errorf("backup assets validation failed: %w", err)
	}
	assetsCommitted := false
	defer func() {
		if assetsCommitted {
			finishAssets()
		} else {
			if err := rollbackAssets(); err != nil {
				restoreErr = fmt.Errorf("%v; asset rollback needs administrator recovery: %w", restoreErr, err)
			}
		}
	}()
	if _, err := s.Create(ctx, "pre_restore", &userID); err != nil {
		return fmt.Errorf("create pre-restore safety snapshot: %w", err)
	}
	preserved, err := s.List(ctx)
	if err != nil {
		return err
	}
	// Flush and remove the WAL before closing so replacement is portable on
	// Windows as well as Unix-like systems.
	if _, err := s.DB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint database before restore: %w", err)
	}
	if err := s.DB.Close(); err != nil {
		return fmt.Errorf("stop database writes: %w", err)
	}
	temporary := s.DB.Path + ".restore-" + uuid.NewString()
	defer os.Remove(temporary)
	if err := copyFile(source, temporary); err != nil {
		_ = s.reopen(context.WithoutCancel(ctx))
		return err
	}
	previous := s.DB.Path + ".before-restore-" + uuid.NewString()
	if err := os.Rename(s.DB.Path, previous); err != nil {
		_ = os.Remove(temporary)
		_ = s.reopen(context.WithoutCancel(ctx))
		return fmt.Errorf("preserve current database before replacement: %w", err)
	}
	if err := os.Rename(temporary, s.DB.Path); err != nil {
		_ = os.Rename(previous, s.DB.Path)
		_ = s.reopen(context.WithoutCancel(ctx))
		return fmt.Errorf("replace database: %w", err)
	}
	if err := applyAssets(); err != nil {
		_ = os.Remove(s.DB.Path)
		_ = os.Remove(s.DB.Path + "-wal")
		_ = os.Remove(s.DB.Path + "-shm")
		if err := os.Rename(previous, s.DB.Path); err != nil {
			return fmt.Errorf("database rollback needs administrator recovery (%s): %w", previous, err)
		}
		_ = s.reopen(context.WithoutCancel(ctx))
		return fmt.Errorf("restore assets: %w", err)
	}
	if err := s.reopen(ctx); err != nil {
		_ = os.Remove(s.DB.Path)
		_ = os.Remove(s.DB.Path + "-wal")
		_ = os.Remove(s.DB.Path + "-shm")
		if err := os.Rename(previous, s.DB.Path); err != nil {
			return fmt.Errorf("database rollback needs administrator recovery (%s): %w", previous, err)
		}
		if rollbackErr := s.reopen(context.WithoutCancel(ctx)); rollbackErr != nil {
			return fmt.Errorf("restored database failed to open (%v), and rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("restored database failed to open; previous database was recovered: %w", err)
	}
	if err := s.DB.IntegrityCheck(ctx); err != nil {
		_ = s.DB.Close()
		_ = os.Remove(s.DB.Path)
		_ = os.Remove(s.DB.Path + "-wal")
		_ = os.Remove(s.DB.Path + "-shm")
		if err := os.Rename(previous, s.DB.Path); err != nil {
			return fmt.Errorf("database rollback needs administrator recovery (%s): %w", previous, err)
		}
		if rollbackErr := s.reopen(context.WithoutCancel(ctx)); rollbackErr != nil {
			return fmt.Errorf("restored database failed integrity check (%v), and rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("restored database failed integrity check; previous database was recovered: %w", err)
	}
	// Never resurrect a previously logged-out session from a snapshot. Preserve
	// the backup catalog so the pre-restore recovery generation stays selectable.
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE sessions SET invalidated_at=?", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		for _, record := range preserved {
			_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO backup_records(id,filename,path,size_bytes,checksum_sha256,kind,verified,created_at,assets_checksum) VALUES(?,?,?,?,?,?,?,?,?)`, record.ID, record.Filename, record.Path, record.SizeBytes, record.Checksum, record.Kind, boolToInt(record.Verified), record.CreatedAt, record.AssetsChecksum)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = s.DB.Close()
		_ = os.Remove(s.DB.Path)
		_ = os.Remove(s.DB.Path + "-wal")
		_ = os.Remove(s.DB.Path + "-shm")
		if err := os.Rename(previous, s.DB.Path); err != nil {
			return fmt.Errorf("database rollback needs administrator recovery (%s): %w", previous, err)
		}
		_ = s.reopen(context.WithoutCancel(ctx))
		return fmt.Errorf("secure restored sessions and catalog: %w", err)
	}
	assetsCommitted = true
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
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false, "", 0, fmt.Errorf("backup is not a regular file")
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()+"?mode=ro")
	if err != nil {
		return false, "", 0, err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return false, "", 0, err
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return false, "", 0, err
	}
	badForeignKeys := rows.Next()
	rowErr := rows.Err()
	rows.Close()
	if badForeignKeys || rowErr != nil {
		return false, "", 0, fmt.Errorf("backup foreign key check failed")
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
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
