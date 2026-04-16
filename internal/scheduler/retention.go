// Package scheduler — retention.go implements the RetentionManager, a standalone
// module that owns all data lifecycle operations (pruning, vacuuming, backups).
//
// This module is additive: the existing pruneData() and checkBackup() in
// scheduler.go remain untouched. Wiring happens in a follow-up issue (#94).
package scheduler

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// RetentionManagerConfig holds configurable data lifecycle settings.
// Unlike the scheduler's RetentionConfig (which uses int days), this uses
// time.Duration for precision and testability.
type RetentionManagerConfig struct {
	SnapshotMaxAge     time.Duration
	SnapshotKeepMin    int
	ServiceCheckMaxAge time.Duration
	NotificationMaxAge time.Duration
	AlertMaxAge        time.Duration
	MaxDBSizeMB        float64
}

// RetentionResult summarizes what a single RunRetention call pruned.
type RetentionResult struct {
	SnapshotsPruned     int
	ServiceChecksPruned int
	NotificationsPruned int
	AlertsPruned        int
	OrphansPruned       int
	SizePruned          int
	Vacuumed            bool
}

// RetentionManager owns all data lifecycle operations. It depends only on
// storage.LifecycleStore (for pruning/vacuum) and storage.ServiceCheckStore
// (for service check history pruning), keeping it decoupled from the
// full Scheduler.
type RetentionManager struct {
	store  storage.LifecycleStore
	svc    storage.ServiceCheckStore
	logger *slog.Logger
}

// NewRetentionManager creates a RetentionManager.
// If svc is nil, service check pruning is skipped.
func NewRetentionManager(store storage.LifecycleStore, svc storage.ServiceCheckStore, logger *slog.Logger) *RetentionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionManager{
		store:  store,
		svc:    svc,
		logger: logger,
	}
}

// RunRetention executes all pruning operations in the same order as the
// existing scheduler.pruneData() method:
//
//  1. Prune old snapshots (respects keep-minimum)
//  2. Prune orphaned findings
//  3. Prune notification log
//  4. Prune service check history (3b)
//  5. Prune resolved alerts
//  6. DB size cap enforcement
//  7. VACUUM (if anything was pruned and PruneToSizeMB didn't already vacuum)
func (rm *RetentionManager) RunRetention(cfg RetentionManagerConfig) RetentionResult {
	var result RetentionResult
	needsVacuum := false

	// 1. Prune old snapshots
	if pruned, err := rm.store.PruneSnapshots(cfg.SnapshotMaxAge, cfg.SnapshotKeepMin); err != nil {
		rm.logger.Warn("prune snapshots failed", "error", err)
	} else if pruned > 0 {
		rm.logger.Info("pruned old snapshots", "count", pruned)
		result.SnapshotsPruned = pruned
		needsVacuum = true
	}

	// 2. Prune orphaned findings
	if pruned, err := rm.store.PruneOrphanedFindings(); err != nil {
		rm.logger.Warn("prune orphaned findings failed", "error", err)
	} else if pruned > 0 {
		rm.logger.Info("pruned orphaned findings", "count", pruned)
		result.OrphansPruned = pruned
		needsVacuum = true
	}

	// 3. Prune notification log
	if pruned, err := rm.store.PruneNotificationLog(cfg.NotificationMaxAge); err != nil {
		rm.logger.Warn("prune notification log failed", "error", err)
	} else if pruned > 0 {
		rm.logger.Info("pruned notification log", "count", pruned)
		result.NotificationsPruned = pruned
		needsVacuum = true
	}

	// 3b. Prune service check history
	if rm.svc != nil {
		if pruned, err := rm.svc.PruneServiceCheckHistory(cfg.ServiceCheckMaxAge); err != nil {
			rm.logger.Warn("prune service check history failed", "error", err)
		} else if pruned > 0 {
			rm.logger.Info("pruned service check history", "count", pruned)
			result.ServiceChecksPruned = pruned
			needsVacuum = true
		}
	}

	// 4. Prune resolved alerts
	if pruned, err := rm.store.PruneAlerts(cfg.AlertMaxAge); err != nil {
		rm.logger.Warn("prune alerts failed", "error", err)
	} else if pruned > 0 {
		rm.logger.Info("pruned old alerts", "count", pruned)
		result.AlertsPruned = pruned
		needsVacuum = true
	}

	// 5. DB size cap
	if cfg.MaxDBSizeMB > 0 {
		if pruned, err := rm.store.PruneToSizeMB(cfg.MaxDBSizeMB); err != nil {
			rm.logger.Warn("prune to size failed", "error", err)
		} else if pruned > 0 {
			rm.logger.Warn("DB size exceeded cap, pruned snapshots",
				"pruned", pruned, "cap_mb", cfg.MaxDBSizeMB)
			result.SizePruned = pruned
			needsVacuum = false // PruneToSizeMB already vacuums
		}
	}

	// 6. VACUUM to reclaim space
	if needsVacuum {
		if err := rm.store.Vacuum(); err != nil {
			rm.logger.Warn("vacuum failed", "error", err)
		} else {
			result.Vacuumed = true
		}
	}

	return result
}

// BackupManagerConfig holds backup scheduling settings for RunBackup.
type BackupManagerConfig struct {
	Enabled   bool
	Path      string // backup directory
	KeepCount int
	IntervalH int
}

// RunBackup creates a database backup if due (based on lastBackup + interval),
// then prunes old backups to keep KeepCount. Returns the backup result or nil
// if not due / disabled.
func (rm *RetentionManager) RunBackup(cfg BackupManagerConfig, lastBackup time.Time, now time.Time) (*storage.BackupResult, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	intervalH := cfg.IntervalH
	if intervalH <= 0 {
		intervalH = 168 // weekly default
	}

	if !lastBackup.IsZero() && now.Sub(lastBackup) < time.Duration(intervalH)*time.Hour {
		return nil, nil // not time yet
	}

	result, err := rm.store.CreateBackup(cfg.Path, rm.logger)
	if err != nil {
		return nil, err
	}

	// Prune old backups
	backupDir := cfg.Path
	if backupDir == "" {
		backupDir = filepath.Dir(result.Path)
	}

	keepCount := cfg.KeepCount
	if keepCount <= 0 {
		keepCount = 4
	}
	if pruned, err := storage.PruneBackups(backupDir, keepCount, rm.logger); err != nil {
		rm.logger.Warn("backup prune failed", "error", err)
	} else if pruned > 0 {
		rm.logger.Info("pruned old backups", "count", pruned)
	}

	return result, nil
}
