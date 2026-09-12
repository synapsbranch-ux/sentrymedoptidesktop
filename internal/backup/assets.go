package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var assetDirectories = []string{"documents", "branding", "signatures", "images", "insurance-cards", "insurance-documents"}

type assetEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Files are immutable UUID names. Snapshots run under the server maintenance
// lock, so a database row cannot race file replacement, deletion or upload.
func snapshotAssets(ctx context.Context, dataDir, destination string) (string, error) {
	if err := os.Mkdir(destination, 0700); err != nil {
		return "", err
	}
	source, err := os.OpenRoot(dataDir)
	if err != nil {
		return "", err
	}
	defer source.Close()
	output, err := os.OpenRoot(destination)
	if err != nil {
		return "", err
	}
	defer output.Close()
	entries := []assetEntry{}
	for _, directory := range assetDirectories {
		if _, err := source.Stat(directory); os.IsNotExist(err) {
			continue
		}
		err = fs.WalkDir(source.FS(), directory, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("backup refuses symbolic links")
			}
			if entry.IsDir() {
				return output.MkdirAll(name, 0700)
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("backup refuses non-regular files")
			}
			in, err := source.Open(name)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := output.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(io.MultiWriter(out, hash), in)
			syncErr := out.Sync()
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if syncErr != nil {
				return syncErr
			}
			if closeErr != nil {
				return closeErr
			}
			entries = append(entries, assetEntry{name, hex.EncodeToString(hash.Sum(nil))})
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	body, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	if err := output.WriteFile("manifest.json", body, 0600); err != nil {
		return "", err
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:]), nil
}

func validateAssets(ctx context.Context, directory, expected string) ([]assetEntry, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	body, err := root.ReadFile("manifest.json")
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != expected {
		return nil, fmt.Errorf("backup asset manifest checksum mismatch")
	}
	var entries []assetEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		valid := false
		for _, dir := range assetDirectories {
			if strings.HasPrefix(entry.Path, dir+"/") {
				valid = true
			}
		}
		if !valid || !fs.ValidPath(entry.Path) || seen[entry.Path] {
			return nil, fmt.Errorf("invalid backup asset path")
		}
		seen[entry.Path] = true
		info, err := root.Lstat(entry.Path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("backup asset is not a regular file")
		}
		file, err := root.Open(entry.Path)
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		file.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return nil, fmt.Errorf("backup asset checksum mismatch")
		}
	}
	return entries, nil
}

// Stage assets and return commit/rollback functions. Old directories are kept
// until the replacement database has reopened and passed integrity checks.
func prepareAssetsRestore(ctx context.Context, dataDir, source, checksum string) (apply func() error, rollback func() error, finish func(), err error) {
	noop := func() {}
	if checksum == "" {
		return func() error { return nil }, func() error { return nil }, noop, nil
	} // legacy SQLite-only snapshot
	if _, err = validateAssets(ctx, source, checksum); err != nil {
		return nil, nil, nil, err
	}
	stage, err := os.MkdirTemp(dataDir, "restore-assets-")
	if err != nil {
		return nil, nil, nil, err
	}
	// Copy from the verified bundle; snapshotAssets re-hashes every copied file.
	prepared := filepath.Join(stage, "new")
	copied, err := snapshotAssets(ctx, source, prepared)
	if err != nil || copied != checksum {
		os.RemoveAll(stage)
		return nil, nil, nil, fmt.Errorf("could not stage verified backup assets")
	}
	swapped := []string{}
	old := map[string]bool{}
	rollback = func() error {
		for i := len(swapped) - 1; i >= 0; i-- {
			dir := swapped[i]
			if err := os.RemoveAll(filepath.Join(dataDir, dir)); err != nil {
				return fmt.Errorf("preserved assets at %s: %w", stage, err)
			}
			if old[dir] {
				if err := os.Rename(filepath.Join(stage, "old-"+dir), filepath.Join(dataDir, dir)); err != nil {
					return fmt.Errorf("preserved assets at %s: %w", stage, err)
				}
			}
		}
		return os.RemoveAll(stage)
	}
	apply = func() error {
		for _, dir := range assetDirectories {
			current := filepath.Join(dataDir, dir)
			if _, e := os.Lstat(current); e == nil {
				if e = os.Rename(current, filepath.Join(stage, "old-"+dir)); e != nil {
					return e
				}
				old[dir] = true
			} else if !os.IsNotExist(e) {
				return e
			}
			swapped = append(swapped, dir)
			if _, e := os.Stat(filepath.Join(prepared, dir)); e == nil {
				if e = os.Rename(filepath.Join(prepared, dir), current); e != nil {
					return e
				}
			} else if !os.IsNotExist(e) {
				return e
			}
		}
		return nil
	}
	finish = func() { _ = os.RemoveAll(stage) }
	return apply, rollback, finish, nil
}
