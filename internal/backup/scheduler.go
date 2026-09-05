package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type scheduleSettings struct {
	IntervalHours int `json:"intervalHours"`
	RetentionDays int `json:"retentionDays"`
}

// RunScheduler checks the database-backed backup policy once a minute. The
// policy is deliberately read on each check so a Settings change does not
// require restarting the clinic server.
func (s Service) RunScheduler(ctx context.Context, logger *slog.Logger, guard sync.Locker) {
	check := func() {
		if guard != nil {
			guard.Lock()
			defer guard.Unlock()
		}
		if err := s.runScheduledCheck(ctx); err != nil && ctx.Err() == nil {
			logger.Error("automatic backup check failed", "error", err)
		}
	}
	check()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

func (s Service) runScheduledCheck(ctx context.Context) error {
	var raw string
	if err := s.DB.QueryRowContext(ctx, "SELECT value_json FROM settings WHERE key='backup'").Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	settings := scheduleSettings{IntervalHours: 4, RetentionDays: 30}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return err
	}
	if settings.IntervalHours <= 0 {
		return nil
	}
	var last sql.NullString
	if err := s.DB.QueryRowContext(ctx, "SELECT MAX(created_at) FROM backup_records WHERE kind='automatic'").Scan(&last); err != nil {
		return err
	}
	due := true
	if last.Valid {
		if parsed, err := time.Parse(time.RFC3339Nano, last.String); err == nil {
			due = time.Since(parsed) >= time.Duration(settings.IntervalHours)*time.Hour
		}
	}
	if due {
		if _, err := s.Create(ctx, "automatic", nil); err != nil {
			return err
		}
	}
	if settings.RetentionDays > 0 {
		return s.pruneAutomatic(ctx, time.Now().UTC().Add(-time.Duration(settings.RetentionDays)*24*time.Hour))
	}
	return nil
}

func (s Service) pruneAutomatic(ctx context.Context, before time.Time) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, path FROM backup_records WHERE kind='automatic' AND created_at < ?", before.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	type expired struct{ id, path string }
	var items []expired
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.path); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(s.DataDir, "backups"))
	if err != nil {
		return err
	}
	for _, item := range items {
		path, err := filepath.Abs(item.path)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." || relative == ".." || len(relative) >= 3 && relative[:3] == ".."+string(os.PathSeparator) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, "DELETE FROM backup_records WHERE id=? AND kind='automatic'", item.id); err != nil {
			return err
		}
	}
	return nil
}
