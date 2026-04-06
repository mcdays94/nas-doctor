// Automatic database backup management.
package storage

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupConfig holds backup configuration.
type BackupConfig struct {
	Enabled   bool
	Path      string // directory for backups
	KeepCount int    // max backups to retain
	IntervalH int    // hours between backups
}

// BackupResult holds info about a completed backup.
type BackupResult struct {
	Path      string    `json:"path"`
	SizeMB    float64   `json:"size_mb"`
	Timestamp time.Time `json:"timestamp"`
}

// BackupInfo lists existing backups.
type BackupInfo struct {
	Backups    []BackupResult `json:"backups"`
	NextBackup string         `json:"next_backup,omitempty"`
}

// CreateBackup copies the database file to a gzip-compressed backup.
// Returns the backup file path and size.
func (d *DB) CreateBackup(backupDir string, logger *slog.Logger) (*BackupResult, error) {
	if backupDir == "" {
		backupDir = filepath.Join(filepath.Dir(d.path), "backups")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	// Generate backup filename with timestamp
	ts := time.Now()
	filename := fmt.Sprintf("nas-doctor-%s.db.gz", ts.Format("20060102-150405"))
	backupPath := filepath.Join(backupDir, filename)

	// Use SQLite's backup API via a checkpoint + file copy
	// First, force a WAL checkpoint to ensure all data is in the main DB file
	if _, err := d.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		logger.Warn("WAL checkpoint failed, proceeding with copy", "error", err)
	}

	// Open source DB file
	src, err := os.Open(d.path)
	if err != nil {
		return nil, fmt.Errorf("open db for backup: %w", err)
	}
	defer src.Close()

	// Create gzip-compressed backup
	dst, err := os.Create(backupPath)
	if err != nil {
		return nil, fmt.Errorf("create backup file: %w", err)
	}
	defer dst.Close()

	gz := gzip.NewWriter(dst)
	gz.Name = "nas-doctor.db"
	gz.ModTime = ts

	if _, err := io.Copy(gz, src); err != nil {
		os.Remove(backupPath)
		return nil, fmt.Errorf("write backup: %w", err)
	}

	if err := gz.Close(); err != nil {
		os.Remove(backupPath)
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	// Get backup size
	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, err
	}

	result := &BackupResult{
		Path:      backupPath,
		SizeMB:    float64(info.Size()) / (1024 * 1024),
		Timestamp: ts,
	}

	logger.Info("backup created", "path", backupPath, "size_mb", fmt.Sprintf("%.1f", result.SizeMB))
	return result, nil
}

// PruneBackups removes old backups, keeping only the most recent `keep` files.
func PruneBackups(backupDir string, keep int, logger *slog.Logger) (int, error) {
	if keep <= 0 {
		keep = 4
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0, err
	}

	// Filter to only our backup files
	var backups []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "nas-doctor-") && strings.HasSuffix(e.Name(), ".db.gz") {
			backups = append(backups, e)
		}
	}

	if len(backups) <= keep {
		return 0, nil
	}

	// Sort by name (which includes timestamp, so lexicographic = chronological)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name() < backups[j].Name()
	})

	// Delete oldest until we're at `keep`
	pruned := 0
	toDelete := len(backups) - keep
	for i := 0; i < toDelete; i++ {
		path := filepath.Join(backupDir, backups[i].Name())
		if err := os.Remove(path); err != nil {
			logger.Warn("failed to remove old backup", "path", path, "error", err)
		} else {
			pruned++
			logger.Info("pruned old backup", "path", path)
		}
	}
	return pruned, nil
}

// ListBackups returns info about existing backups in the directory.
func ListBackups(backupDir string) ([]BackupResult, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []BackupResult
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "nas-doctor-") || !strings.HasSuffix(e.Name(), ".db.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		results = append(results, BackupResult{
			Path:      filepath.Join(backupDir, e.Name()),
			SizeMB:    float64(info.Size()) / (1024 * 1024),
			Timestamp: info.ModTime(),
		})
	}

	// Sort newest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results, nil
}
