// Package scheduler handles periodic diagnostic collection runs.
package scheduler

import (
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/analyzer"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// RetentionConfig holds configurable data lifecycle settings.
type RetentionConfig struct {
	SnapshotDays  int // days to keep snapshots (default 90)
	MaxDBSizeMB   int // hard cap on DB file size (default 500)
	NotifyLogDays int // days to keep notification logs (default 30)
}

// BackupConfig holds backup scheduling settings.
type BackupConfig struct {
	Enabled    bool
	Path       string // backup directory
	KeepCount  int
	IntervalH  int
	LastBackup time.Time
}

// Scheduler periodically runs diagnostic collections and analysis.
type Scheduler struct {
	collector *collector.Collector
	store     *storage.DB
	notifier  *notifier.Notifier
	metrics   *notifier.Metrics
	logger    *slog.Logger
	interval  time.Duration
	retention RetentionConfig

	mu      sync.RWMutex
	latest  *internal.Snapshot
	running bool
	stop    chan struct{}
	restart chan time.Duration // signal to update interval
	backup  BackupConfig
}

// New creates a new Scheduler.
func New(
	col *collector.Collector,
	store *storage.DB,
	notif *notifier.Notifier,
	metrics *notifier.Metrics,
	logger *slog.Logger,
	interval time.Duration,
) *Scheduler {
	return &Scheduler{
		collector: col,
		store:     store,
		notifier:  notif,
		metrics:   metrics,
		logger:    logger,
		interval:  interval,
		retention: RetentionConfig{
			SnapshotDays:  90,
			MaxDBSizeMB:   500,
			NotifyLogDays: 30,
		},
		stop:    make(chan struct{}),
		restart: make(chan time.Duration, 1),
	}
}

// Start begins the periodic collection loop. It runs the first collection
// immediately, then repeats at the configured interval.
func (s *Scheduler) Start() {
	s.logger.Info("scheduler starting", "interval", s.interval)
	go func() {
		// Run immediately on startup
		s.RunOnce()

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.RunOnce()
			case newInterval := <-s.restart:
				ticker.Stop()
				s.mu.Lock()
				s.interval = newInterval
				s.mu.Unlock()
				ticker = time.NewTicker(newInterval)
				s.logger.Info("scheduler interval updated", "new_interval", newInterval)
			case <-s.stop:
				s.logger.Info("scheduler stopped")
				return
			}
		}
	}()
}

// UpdateInterval dynamically changes the scan interval without restarting.
func (s *Scheduler) UpdateInterval(d time.Duration) {
	if d < 1*time.Second {
		d = 1 * time.Second // minimum 1 second
	}
	select {
	case s.restart <- d:
	default:
		// channel full, skip (a previous update is pending)
	}
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	close(s.stop)
}

// RunOnce performs a single collection + analysis + notify cycle.
func (s *Scheduler) RunOnce() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		s.logger.Warn("collection already in progress, skipping")
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	s.logger.Info("starting diagnostic collection")

	// Collect
	snap, err := s.collector.Collect()
	if err != nil {
		s.logger.Error("collection failed", "error", err)
		return
	}

	// Analyze
	snap.Findings = analyzer.Analyze(snap)
	// Stamp findings with detection timestamp
	ts := snap.Timestamp.Format(time.RFC3339)
	for i := range snap.Findings {
		snap.Findings[i].DetectedAt = ts
	}
	s.logger.Info("analysis complete",
		"critical", countSeverity(snap.Findings, internal.SeverityCritical),
		"warnings", countSeverity(snap.Findings, internal.SeverityWarning),
		"info", countSeverity(snap.Findings, internal.SeverityInfo),
	)

	// Store
	if err := s.store.SaveSnapshot(snap); err != nil {
		s.logger.Error("failed to save snapshot", "error", err)
	}

	// Update Prometheus metrics
	if s.metrics != nil {
		s.metrics.Update(snap)
	}

	// Cache latest
	s.mu.Lock()
	s.latest = snap
	s.mu.Unlock()

	// Notify
	if s.notifier != nil {
		hostname := snap.System.Hostname
		if hostname == "" {
			hostname = "Unknown"
		}
		s.notifier.NotifyFindings(snap.Findings, hostname)
	}

	// Data lifecycle: prune old data
	s.pruneData()

	// Auto backup check
	s.checkBackup()
}

// Latest returns the most recent snapshot from the cache.
func (s *Scheduler) Latest() *internal.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

// SetLatest injects a snapshot into the scheduler's cache (used by demo mode).
func (s *Scheduler) SetLatest(snap *internal.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = snap
}

// UpdateRetention updates the data lifecycle configuration.
func (s *Scheduler) UpdateRetention(cfg RetentionConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.SnapshotDays > 0 {
		s.retention.SnapshotDays = cfg.SnapshotDays
	}
	if cfg.MaxDBSizeMB > 0 {
		s.retention.MaxDBSizeMB = cfg.MaxDBSizeMB
	}
	if cfg.NotifyLogDays > 0 {
		s.retention.NotifyLogDays = cfg.NotifyLogDays
	}
	s.logger.Info("retention config updated",
		"snapshot_days", s.retention.SnapshotDays,
		"max_db_mb", s.retention.MaxDBSizeMB,
		"notify_log_days", s.retention.NotifyLogDays,
	)
}

// pruneData runs all data lifecycle maintenance tasks.
func (s *Scheduler) pruneData() {
	s.mu.RLock()
	ret := s.retention
	s.mu.RUnlock()

	snapshotAge := time.Duration(ret.SnapshotDays) * 24 * time.Hour
	notifyAge := time.Duration(ret.NotifyLogDays) * 24 * time.Hour
	needsVacuum := false

	// 1. Prune old snapshots (cascades to smart_history, system_history)
	if pruned, err := s.store.PruneSnapshots(snapshotAge, 10); err != nil {
		s.logger.Warn("prune snapshots failed", "error", err)
	} else if pruned > 0 {
		s.logger.Info("pruned old snapshots", "count", pruned, "retention_days", ret.SnapshotDays)
		needsVacuum = true
	}

	// 2. Prune orphaned findings (safety net)
	if pruned, err := s.store.PruneOrphanedFindings(); err != nil {
		s.logger.Warn("prune orphaned findings failed", "error", err)
	} else if pruned > 0 {
		s.logger.Info("pruned orphaned findings", "count", pruned)
		needsVacuum = true
	}

	// 3. Prune notification log
	if pruned, err := s.store.PruneNotificationLog(notifyAge); err != nil {
		s.logger.Warn("prune notification log failed", "error", err)
	} else if pruned > 0 {
		s.logger.Info("pruned notification log", "count", pruned)
		needsVacuum = true
	}

	// 4. Prune resolved alerts (same retention as notifications)
	if pruned, err := s.store.PruneAlerts(notifyAge); err != nil {
		s.logger.Warn("prune alerts failed", "error", err)
	} else if pruned > 0 {
		s.logger.Info("pruned old alerts", "count", pruned)
		needsVacuum = true
	}

	// 5. Check DB size cap — if over the limit, aggressively delete oldest data
	if ret.MaxDBSizeMB > 0 {
		if pruned, err := s.store.PruneToSizeMB(float64(ret.MaxDBSizeMB)); err != nil {
			s.logger.Warn("prune to size failed", "error", err)
		} else if pruned > 0 {
			s.logger.Warn("DB size exceeded cap, pruned snapshots",
				"pruned", pruned, "cap_mb", ret.MaxDBSizeMB)
			needsVacuum = false // PruneToSizeMB already vacuums
		}
	}

	// 6. VACUUM to reclaim space (only if we pruned and didn't already vacuum)
	if needsVacuum {
		if err := s.store.Vacuum(); err != nil {
			s.logger.Warn("vacuum failed", "error", err)
		}
	}
}

// IsRunning returns true if a collection is currently in progress.
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Interval returns the current scan interval.
func (s *Scheduler) Interval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interval
}

// UpdateBackup updates the backup configuration.
func (s *Scheduler) UpdateBackup(cfg BackupConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backup = cfg
	s.logger.Info("backup config updated",
		"enabled", cfg.Enabled,
		"path", cfg.Path,
		"keep", cfg.KeepCount,
		"interval_h", cfg.IntervalH,
	)
}

// checkBackup runs a backup if enough time has elapsed since the last one.
func (s *Scheduler) checkBackup() {
	s.mu.RLock()
	cfg := s.backup
	s.mu.RUnlock()

	if !cfg.Enabled {
		return
	}

	intervalH := cfg.IntervalH
	if intervalH <= 0 {
		intervalH = 168 // weekly
	}

	if !cfg.LastBackup.IsZero() && time.Since(cfg.LastBackup) < time.Duration(intervalH)*time.Hour {
		return // not time yet
	}

	result, err := s.store.CreateBackup(cfg.Path, s.logger)
	if err != nil {
		s.logger.Warn("auto backup failed", "error", err)
		return
	}

	// Prune old backups
	backupDir := cfg.Path
	if backupDir == "" {
		// Extract directory from result path
		backupDir = filepath.Dir(result.Path)
	}

	keepCount := cfg.KeepCount
	if keepCount <= 0 {
		keepCount = 4
	}
	if pruned, err := storage.PruneBackups(backupDir, keepCount, s.logger); err != nil {
		s.logger.Warn("backup prune failed", "error", err)
	} else if pruned > 0 {
		s.logger.Info("pruned old backups", "count", pruned)
	}

	s.mu.Lock()
	s.backup.LastBackup = result.Timestamp
	s.mu.Unlock()
}

func countSeverity(findings []internal.Finding, sev internal.Severity) int {
	count := 0
	for _, f := range findings {
		if f.Severity == sev {
			count++
		}
	}
	return count
}
