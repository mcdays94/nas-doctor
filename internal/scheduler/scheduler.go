// Package scheduler handles periodic diagnostic collection runs.
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/analyzer"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/logfwd"
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

// AlertPolicy routes matching findings to a target webhook.
type AlertPolicy struct {
	Name        string              `json:"name"`
	Enabled     bool                `json:"enabled"`
	WebhookName string              `json:"webhook_name"`
	MinSeverity internal.Severity   `json:"min_severity"`
	Categories  []internal.Category `json:"categories,omitempty"`
	Hostnames   []string            `json:"hostnames,omitempty"`
	CooldownSec int                 `json:"cooldown_sec"`
}

// QuietHours suppresses notifications in a daily local time window.
type QuietHours struct {
	Enabled   bool   `json:"enabled"`
	Timezone  string `json:"timezone"`
	StartHHMM string `json:"start_hhmm"`
	EndHHMM   string `json:"end_hhmm"`
}

// MaintenanceWindow suppresses notifications during an explicit time range.
type MaintenanceWindow struct {
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	StartISO  string   `json:"start_iso"`
	EndISO    string   `json:"end_iso"`
	Hostnames []string `json:"hostnames,omitempty"`
}

// AlertingConfig controls notification rules and suppression behavior.
type AlertingConfig struct {
	Rules              []internal.NotificationRule `json:"rules,omitempty"`
	Policies           []AlertPolicy               `json:"policies,omitempty"` // legacy
	QuietHours         QuietHours                  `json:"quiet_hours,omitempty"`
	MaintenanceWindows []MaintenanceWindow         `json:"maintenance_windows,omitempty"`
	DefaultCooldownSec int                         `json:"default_cooldown_sec,omitempty"`
}

// Scheduler periodically runs diagnostic collections and analysis.
type Scheduler struct {
	collector     *collector.Collector
	store         *storage.DB
	notifier      *notifier.Notifier
	metrics       *notifier.Metrics
	logger        *slog.Logger
	interval      time.Duration
	retention     RetentionConfig
	alerting      AlertingConfig
	serviceChecks []internal.ServiceCheckConfig
	lastCheckRun  map[string]time.Time // per-check last execution time

	logForwarder *logfwd.Forwarder

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
		alerting: AlertingConfig{
			Policies:           []AlertPolicy{},
			MaintenanceWindows: []MaintenanceWindow{},
			DefaultCooldownSec: 900,
		},
		serviceChecks: []internal.ServiceCheckConfig{},
		lastCheckRun:  make(map[string]time.Time),
		stop:          make(chan struct{}),
		restart:       make(chan time.Duration, 1),
	}
}

// Start begins the periodic collection loop. It runs the first collection
// immediately, then repeats at the configured interval.
// Also starts an independent service check loop (30s tick) that respects
// per-check intervals.
func (s *Scheduler) Start() {
	s.logger.Info("scheduler starting", "interval", s.interval)
	// Main diagnostic collection loop
	go func() {
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
	// Independent service check loop — ticks every 30s, runs due checks
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runDueServiceChecks()
			case <-s.stop:
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

	serviceResults, err := s.runServiceChecks(snap.Timestamp)
	if err != nil {
		s.logger.Warn("service checks partial failure", "error", err)
	}
	snap.Services = serviceResults

	// Analyze
	snap.Findings = analyzer.Analyze(snap)
	snap.Findings = append(snap.Findings, s.buildSMARTTrendFindings(snap)...)
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

	stateFindings := make([]storage.AlertStateFinding, 0, len(snap.Findings))
	for _, f := range snap.Findings {
		stateFindings = append(stateFindings, storage.AlertStateFinding{
			Fingerprint: findingFingerprint(f),
			FindingID:   f.ID,
			Severity:    string(f.Severity),
			Title:       f.Title,
		})
	}
	if err := s.store.SyncAlertStates(snap.ID, stateFindings, snap.Timestamp); err != nil {
		s.logger.Warn("sync alert states failed", "error", err)
	}

	// Notify
	s.mu.RLock()
	notif := s.notifier
	s.mu.RUnlock()
	if notif != nil {
		hostname := snap.System.Hostname
		if hostname == "" {
			hostname = "Unknown"
		}
		s.dispatchNotifications(notif, snap, hostname, snap.Timestamp)
	}

	// Log forwarding
	s.mu.RLock()
	fwd := s.logForwarder
	s.mu.RUnlock()
	if fwd != nil {
		hostname := snap.System.Hostname
		if hostname == "" {
			hostname = "Unknown"
		}
		fwd.Forward(snap, hostname)
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

	// 3b. Prune service check history
	if pruned, err := s.store.PruneServiceCheckHistory(notifyAge); err != nil {
		s.logger.Warn("prune service check history failed", "error", err)
	} else if pruned > 0 {
		s.logger.Info("pruned service check history", "count", pruned)
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

// UpdateLogForwarder sets the log forwarding destinations.
func (s *Scheduler) UpdateLogForwarder(dests []logfwd.Destination) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logForwarder == nil {
		s.logForwarder = logfwd.New(s.logger)
	}
	s.logForwarder.SetDestinations(dests)
	s.logger.Info("log forwarder updated", "destinations", len(dests))
}

// UpdateAlerting updates policy routing and suppression configuration.
func (s *Scheduler) UpdateAlerting(cfg AlertingConfig) {
	if cfg.Policies == nil {
		cfg.Policies = []AlertPolicy{}
	}
	if cfg.MaintenanceWindows == nil {
		cfg.MaintenanceWindows = []MaintenanceWindow{}
	}
	if cfg.DefaultCooldownSec <= 0 {
		cfg.DefaultCooldownSec = 900
	}
	if cfg.QuietHours.Timezone == "" {
		cfg.QuietHours.Timezone = "UTC"
	}

	s.mu.Lock()
	s.alerting = cfg
	s.mu.Unlock()

	s.logger.Info("alerting config updated",
		"policies", len(cfg.Policies),
		"maintenance_windows", len(cfg.MaintenanceWindows),
		"quiet_hours_enabled", cfg.QuietHours.Enabled,
	)
}

// UpdateServiceChecks replaces service check configuration used in each run.
func (s *Scheduler) UpdateServiceChecks(checks []internal.ServiceCheckConfig) {
	normalized := make([]internal.ServiceCheckConfig, 0, len(checks))
	for _, check := range checks {
		check.Type = strings.ToLower(strings.TrimSpace(check.Type))
		check.Name = strings.TrimSpace(check.Name)
		check.Target = strings.TrimSpace(check.Target)
		if check.Name == "" || check.Target == "" {
			continue
		}
		if !isSupportedServiceCheckType(check.Type) {
			continue
		}
		if check.TimeoutSec <= 0 {
			check.TimeoutSec = 5
		}
		if check.FailureThreshold <= 0 {
			check.FailureThreshold = 1
		}
		if check.FailureSeverity == "" {
			check.FailureSeverity = internal.SeverityWarning
		}
		normalized = append(normalized, check)
	}

	s.mu.Lock()
	s.serviceChecks = normalized
	s.mu.Unlock()

	s.logger.Info("service check config updated", "checks", len(normalized))
}

// RunServiceChecksNow executes configured service checks immediately and persists results.
func (s *Scheduler) RunServiceChecksNow() ([]internal.ServiceCheckResult, error) {
	return s.runServiceChecks(time.Now().UTC())
}

// UpdateNotifier swaps the notifier used for delivery.
func (s *Scheduler) UpdateNotifier(notif *notifier.Notifier) {
	s.mu.Lock()
	s.notifier = notif
	s.mu.Unlock()
	if notif == nil {
		s.logger.Info("notifier updated", "enabled", false)
		return
	}
	s.logger.Info("notifier updated", "enabled", true)
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

func (s *Scheduler) buildSMARTTrendFindings(snap *internal.Snapshot) []internal.Finding {
	if snap == nil || len(snap.SMART) == 0 {
		return nil
	}

	findings := make([]internal.Finding, 0)
	for _, drive := range snap.SMART {
		if strings.TrimSpace(drive.Serial) == "" {
			continue
		}

		history, err := s.store.GetDiskHistory(drive.Serial, 30)
		if err != nil {
			s.logger.Warn("failed to load disk history for trend analysis", "serial", drive.Serial, "error", err)
			continue
		}
		if len(history) < 2 {
			continue
		}

		first := history[0]
		last := history[len(history)-1]
		days := last.Timestamp.Sub(first.Timestamp).Hours() / 24.0
		if days < 1 {
			days = 1
		}

		reallocDelta := last.Reallocated - first.Reallocated
		pendingDelta := last.Pending - first.Pending
		crcDelta := last.UDMACRC - first.UDMACRC
		tempDelta := last.Temperature - first.Temperature

		worsening := reallocDelta > 0 || pendingDelta > 0 || crcDelta > 0 || (tempDelta >= 4 && last.Temperature >= 45)
		if !worsening {
			continue
		}

		riskScore := 0
		if last.Pending > 0 {
			riskScore += 40
		}
		if pendingDelta > 0 {
			riskScore += 25
		}
		if reallocDelta > 0 {
			riskScore += int(minInt64(reallocDelta, 20))
		}
		if crcDelta > 5 {
			riskScore += 10
		}
		if last.Temperature >= 50 {
			riskScore += 15
		} else if last.Temperature >= 45 {
			riskScore += 8
		}
		if tempDelta >= 4 {
			riskScore += 10
		}

		severity := internal.SeverityInfo
		urgency := "monitor"
		if riskScore >= 70 {
			severity = internal.SeverityCritical
			urgency = "immediate"
		} else if riskScore >= 40 {
			severity = internal.SeverityWarning
			urgency = "short-term"
		}

		confidence := "low"
		if len(history) >= 10 && days >= 7 {
			confidence = "high"
		} else if len(history) >= 5 && days >= 3 {
			confidence = "medium"
		}

		title := fmt.Sprintf("Worsening SMART Trend: %s (%s)", drive.Device, drive.Model)
		description := fmt.Sprintf(
			"SMART metrics are worsening over %.1f days (realloc %+d, pending %+d, CRC %+d, temp %+dC). Risk score %d/100.",
			days, reallocDelta, pendingDelta, crcDelta, tempDelta, riskScore,
		)
		evidence := []string{
			fmt.Sprintf("Current: temp=%dC realloc=%d pending=%d crc=%d", last.Temperature, last.Reallocated, last.Pending, last.UDMACRC),
			fmt.Sprintf("Delta: realloc %+d (%.2f/day), pending %+d (%.2f/day), crc %+d (%.2f/day)", reallocDelta, float64(reallocDelta)/days, pendingDelta, float64(pendingDelta)/days, crcDelta, float64(crcDelta)/days),
			fmt.Sprintf("Guidance: urgency=%s confidence=%s", urgency, confidence),
		}

		action := "Review trend trajectory, verify recent SMART test output, and plan replacement if counters continue rising."
		if urgency == "immediate" {
			action = "Prepare replacement immediately and verify backups now. Rising pending/reallocated trends indicate elevated near-term failure risk."
		}

		findings = append(findings, internal.Finding{
			Severity:    severity,
			Category:    internal.CategorySMART,
			Title:       title,
			Description: description,
			Evidence:    evidence,
			Impact:      "Increased probability of uncorrectable read/write failures if trend continues.",
			Action:      action,
			Priority:    urgency,
			Cost:        estimateTrendCost(urgency),
			RelatedDisk: drive.ArraySlot,
		})
	}

	return findings
}

func estimateTrendCost(urgency string) string {
	switch urgency {
	case "immediate":
		return "$80-350 for replacement drive"
	case "short-term":
		return "$5-15 (cable) or drive replacement planning"
	default:
		return "none"
	}
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (s *Scheduler) runServiceChecks(now time.Time) ([]internal.ServiceCheckResult, error) {
	s.mu.RLock()
	checks := make([]internal.ServiceCheckConfig, len(s.serviceChecks))
	copy(checks, s.serviceChecks)
	s.mu.RUnlock()

	if len(checks) == 0 {
		return []internal.ServiceCheckResult{}, nil
	}

	results := make([]internal.ServiceCheckResult, 0, len(checks))
	for _, check := range checks {
		if !check.Enabled {
			continue
		}

		result := executeServiceCheck(check, now)

		state, ok, err := s.store.GetLatestServiceCheckState(result.Key)
		if err != nil {
			s.logger.Warn("failed to read previous service check state", "check", result.Name, "error", err)
		}
		if result.Status == "down" {
			if ok && state.Status == "down" {
				result.ConsecutiveFailures = state.ConsecutiveFailures + 1
			} else {
				result.ConsecutiveFailures = 1
			}
		}

		results = append(results, result)
	}

	if len(results) == 0 {
		return results, nil
	}
	if err := s.store.SaveServiceCheckResults(results); err != nil {
		return results, err
	}
	return results, nil
}

const defaultCheckIntervalSec = 300 // 5 minutes

// runDueServiceChecks is called every 30s by the independent service check
// loop. It checks each configured check's per-check interval and only
// executes those whose interval has elapsed since their last run.
func (s *Scheduler) runDueServiceChecks() {
	s.mu.RLock()
	checks := make([]internal.ServiceCheckConfig, len(s.serviceChecks))
	copy(checks, s.serviceChecks)
	s.mu.RUnlock()

	now := time.Now()
	var due []internal.ServiceCheckConfig
	for _, check := range checks {
		if !check.Enabled {
			continue
		}
		key := serviceCheckKey(check)
		interval := check.IntervalSec
		if interval <= 0 {
			interval = defaultCheckIntervalSec
		}
		s.mu.RLock()
		last, exists := s.lastCheckRun[key]
		s.mu.RUnlock()
		if !exists || now.Sub(last) >= time.Duration(interval)*time.Second {
			due = append(due, check)
		}
	}
	if len(due) == 0 {
		return
	}

	results := make([]internal.ServiceCheckResult, 0, len(due))
	for _, check := range due {
		result := executeServiceCheck(check, now)
		key := result.Key

		state, ok, err := s.store.GetLatestServiceCheckState(key)
		if err != nil {
			s.logger.Warn("service check state read failed", "check", result.Name, "error", err)
		}
		if result.Status == "down" {
			if ok && state.Status == "down" {
				result.ConsecutiveFailures = state.ConsecutiveFailures + 1
			} else {
				result.ConsecutiveFailures = 1
			}
		}
		results = append(results, result)

		s.mu.Lock()
		s.lastCheckRun[key] = now
		s.mu.Unlock()
	}

	if err := s.store.SaveServiceCheckResults(results); err != nil {
		s.logger.Warn("failed to save service check results", "error", err)
	}
}

func executeServiceCheck(check internal.ServiceCheckConfig, now time.Time) internal.ServiceCheckResult {
	typeName := strings.ToLower(strings.TrimSpace(check.Type))
	timeoutSec := check.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	threshold := check.FailureThreshold
	if threshold <= 0 {
		threshold = 1
	}
	severity := check.FailureSeverity
	if severity == "" {
		severity = internal.SeverityWarning
	}

	result := internal.ServiceCheckResult{
		Key:              serviceCheckKey(check),
		Name:             strings.TrimSpace(check.Name),
		Type:             typeName,
		Target:           strings.TrimSpace(check.Target),
		Status:           "down",
		CheckedAt:        now.UTC().Format(time.RFC3339),
		FailureThreshold: threshold,
		FailureSeverity:  severity,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	switch typeName {
	case internal.ServiceCheckHTTP:
		urlValue := normalizeHTTPURL(check.Target)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlValue, nil)
		if err != nil {
			result.Error = err.Error()
			break
		}
		resp, err := (&http.Client{Timeout: time.Duration(timeoutSec) * time.Second}).Do(req)
		result.ResponseMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Error = err.Error()
			break
		}
		_ = resp.Body.Close()
		minStatus := check.ExpectedMin
		maxStatus := check.ExpectedMax
		if minStatus <= 0 {
			minStatus = 200
		}
		if maxStatus <= 0 {
			maxStatus = 399
		}
		if maxStatus < minStatus {
			maxStatus = minStatus
		}
		if resp.StatusCode < minStatus || resp.StatusCode > maxStatus {
			result.Error = fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode)
			break
		}
		result.Status = "up"

	case internal.ServiceCheckDNS:
		host := normalizeDNSHost(check.Target)
		if host == "" {
			result.Error = "empty DNS target"
			break
		}
		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		result.ResponseMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Error = err.Error()
			break
		}
		if len(addrs) == 0 {
			result.Error = "no DNS records found"
			break
		}
		result.Status = "up"

	case internal.ServiceCheckTCP, internal.ServiceCheckSMB, internal.ServiceCheckNFS:
		addr, err := normalizeTCPAddress(check)
		if err != nil {
			result.Error = err.Error()
			break
		}
		dialer := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		result.ResponseMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Error = err.Error()
			break
		}
		_ = conn.Close()
		result.Status = "up"

	case internal.ServiceCheckPing:
		host := normalizeDNSHost(check.Target)
		if host == "" {
			result.Error = "empty ping target"
			break
		}
		countArg := "-c"
		timeoutArg := "-W"
		timeoutVal := fmt.Sprintf("%d", timeoutSec)
		if runtime.GOOS == "darwin" {
			timeoutArg = "-t"
		}
		cmd := exec.CommandContext(ctx, "ping", countArg, "1", timeoutArg, timeoutVal, host)
		out, err := cmd.CombinedOutput()
		result.ResponseMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Error = "host unreachable"
			break
		}
		// Parse round-trip time from ping output if available
		outStr := string(out)
		if idx := strings.Index(outStr, "time="); idx >= 0 {
			sub := outStr[idx+5:]
			if sp := strings.IndexAny(sub, " m\n"); sp > 0 {
				if ms, parseErr := strconv.ParseFloat(sub[:sp], 64); parseErr == nil {
					result.ResponseMS = int64(ms)
				}
			}
		}
		result.Status = "up"

	default:
		result.Error = "unsupported service check type"
	}

	if result.ResponseMS == 0 {
		result.ResponseMS = time.Since(start).Milliseconds()
	}
	if result.Status == "up" {
		result.Error = ""
	}
	return result
}

func isSupportedServiceCheckType(checkType string) bool {
	switch strings.ToLower(strings.TrimSpace(checkType)) {
	case internal.ServiceCheckHTTP, internal.ServiceCheckTCP, internal.ServiceCheckDNS, internal.ServiceCheckSMB, internal.ServiceCheckNFS, internal.ServiceCheckPing:
		return true
	default:
		return false
	}
}

func normalizeHTTPURL(rawTarget string) string {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return target
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	return "http://" + target
}

func normalizeDNSHost(rawTarget string) string {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return ""
	}
	if strings.Contains(target, "://") {
		if parsed, err := url.Parse(target); err == nil {
			if host := strings.TrimSpace(parsed.Hostname()); host != "" {
				return host
			}
		}
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		return host
	}
	return target
}

func normalizeTCPAddress(check internal.ServiceCheckConfig) (string, error) {
	target := strings.TrimSpace(check.Target)
	if target == "" {
		return "", fmt.Errorf("empty target")
	}
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err == nil && parsed.Host != "" {
			target = parsed.Host
		}
	}

	if _, _, err := net.SplitHostPort(target); err == nil {
		return target, nil
	}

	port := check.Port
	if port <= 0 {
		switch strings.ToLower(strings.TrimSpace(check.Type)) {
		case internal.ServiceCheckSMB:
			port = 445
		case internal.ServiceCheckNFS:
			port = 2049
		}
	}
	if port <= 0 {
		return "", fmt.Errorf("missing port")
	}
	host := normalizeDNSHost(target)
	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

func serviceCheckKey(check internal.ServiceCheckConfig) string {
	raw := strings.ToLower(strings.TrimSpace(check.Name)) + "|" + strings.ToLower(strings.TrimSpace(check.Type)) + "|" + strings.ToLower(strings.TrimSpace(check.Target)) + "|" + fmt.Sprintf("%d", check.Port)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Scheduler) dispatchNotifications(notif *notifier.Notifier, snap *internal.Snapshot, hostname string, now time.Time) {
	if notif == nil {
		return
	}

	s.mu.RLock()
	cfg := s.alerting
	s.mu.RUnlock()

	if cfg.DefaultCooldownSec <= 0 {
		cfg.DefaultCooldownSec = 900
	}

	if inMaintenanceWindow(cfg.MaintenanceWindows, hostname, now) {
		s.logSuppressed(notif, snap.Findings, hostname, cfg, "suppressed_maintenance")
		return
	}
	if inQuietHours(cfg.QuietHours, now) {
		s.logSuppressed(notif, snap.Findings, hostname, cfg, "suppressed_quiet_hours")
		return
	}

	// Build webhook lookup
	webhooks := make(map[string]internal.WebhookConfig)
	for _, wh := range notif.Webhooks() {
		webhooks[strings.ToLower(strings.TrimSpace(wh.Name))] = wh
	}

	if len(cfg.Rules) == 0 {
		// No rules configured — send all findings to all enabled webhooks (legacy)
		if len(snap.Findings) > 0 {
			for _, wh := range notif.Webhooks() {
				if !wh.Enabled {
					continue
				}
				routeKey := "legacy:" + strings.ToLower(strings.TrimSpace(wh.Name))
				toSend := s.applyCooldown(snap.Findings, routeKey, time.Duration(cfg.DefaultCooldownSec)*time.Second, now)
				if len(toSend) == 0 {
					continue
				}
				if err := notif.NotifyWebhook(wh, toSend, hostname); err != nil {
					continue
				}
				s.recordSent(toSend, routeKey, now)
			}
		}
		return
	}

	// ── Rule-based dispatch ──
	for _, rule := range cfg.Rules {
		if !rule.Enabled {
			continue
		}
		whName := strings.ToLower(strings.TrimSpace(rule.Webhook))
		wh, ok := webhooks[whName]
		if !ok || !wh.Enabled {
			continue
		}

		matched := evaluateRule(rule, snap)
		if len(matched) == 0 {
			continue
		}

		cooldown := time.Duration(rule.CooldownSec) * time.Second
		if cooldown <= 0 {
			cooldown = time.Duration(cfg.DefaultCooldownSec) * time.Second
		}
		routeKey := "rule:" + rule.ID
		toSend := s.applyCooldown(matched, routeKey, cooldown, now)
		if len(toSend) == 0 {
			_ = s.store.SaveNotificationLog(wh.Name, wh.Type, "suppressed_cooldown", len(matched), "")
			continue
		}

		if err := notif.NotifyWebhook(wh, toSend, hostname); err != nil {
			continue
		}
		s.recordSent(toSend, routeKey, now)
	}
}

func (s *Scheduler) recordSent(findings []internal.Finding, routeKey string, now time.Time) {
	fingerprints := make([]string, 0, len(findings))
	for _, f := range findings {
		fp := findingFingerprint(f)
		fingerprints = append(fingerprints, fp)
		if err := s.store.SaveNotificationState(fp, routeKey, "sent", now); err != nil {
			s.logger.Warn("failed to save notification state", "fingerprint", fp, "route", routeKey, "error", err)
		}
	}
	if err := s.store.MarkAlertsNotifiedByFingerprint(fingerprints, now); err != nil {
		s.logger.Warn("failed to mark alerts notified", "error", err)
	}
}

// evaluateRule checks a single notification rule against the snapshot and
// returns matching findings (real or synthetic).
func evaluateRule(rule internal.NotificationRule, snap *internal.Snapshot) []internal.Finding {
	cat := strings.ToLower(rule.Category)
	cond := strings.ToLower(rule.Condition)
	target := strings.ToLower(strings.TrimSpace(rule.Target))
	val := parseFloat(rule.Value)

	switch cat {
	case "findings":
		return evalFindings(cond, target, snap.Findings)
	case "disk_space":
		return evalDiskSpace(cond, target, val, snap.Disks)
	case "disk_temp":
		return evalDiskTemp(cond, target, int(val), snap.SMART)
	case "smart":
		return evalSMART(cond, target, int64(val), snap.SMART)
	case "service":
		return evalService(cond, target, val, snap.Services)
	case "parity":
		return evalParity(cond, val, snap.Parity)
	case "ups":
		return evalUPS(cond, val, snap.UPS)
	case "docker":
		return evalDocker(cond, target, snap.Docker)
	case "system":
		return evalSystem(cond, val, snap.System)
	case "zfs":
		return evalZFS(cond, target, val, snap.ZFS)
	case "tunnels":
		return evalTunnels(cond, target, snap.Tunnels)
	case "update":
		return evalUpdate(snap.Update)
	}
	return nil
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func synth(id string, sev internal.Severity, cat internal.Category, title, desc string) internal.Finding {
	return internal.Finding{ID: id, Severity: sev, Category: cat, Title: title, Description: desc}
}

func matchTarget(target, candidate string) bool {
	if target == "" {
		return true
	}
	return strings.EqualFold(target, strings.TrimSpace(candidate))
}

// ── Category evaluators ──

func evalFindings(cond, target string, findings []internal.Finding) []internal.Finding {
	var out []internal.Finding
	for _, f := range findings {
		switch cond {
		case "critical":
			if f.Severity == internal.SeverityCritical {
				out = append(out, f)
			}
		case "warning":
			if f.Severity == internal.SeverityWarning || f.Severity == internal.SeverityCritical {
				out = append(out, f)
			}
		case "category":
			if strings.EqualFold(string(f.Category), target) {
				out = append(out, f)
			}
		case "any":
			out = append(out, f)
		}
	}
	return out
}

func evalDiskSpace(cond, target string, val float64, disks []internal.DiskInfo) []internal.Finding {
	if val <= 0 {
		return nil
	}
	var out []internal.Finding
	for _, d := range disks {
		if !matchTarget(target, d.MountPoint) && !matchTarget(target, d.Label) {
			continue
		}
		free := 100.0 - d.UsedPct
		if free < val {
			out = append(out, synth("rule:disk-space:"+d.MountPoint, internal.SeverityWarning, internal.CategoryDisk,
				"Disk space low: "+d.MountPoint, fmt.Sprintf("Free space %.1f%% is below threshold %.0f%%", free, val)))
		}
	}
	return out
}

func evalDiskTemp(cond, target string, val int, smart []internal.SMARTInfo) []internal.Finding {
	if val <= 0 {
		return nil
	}
	var out []internal.Finding
	switch cond {
	case "above", "single_above":
		for _, s := range smart {
			if !matchTarget(target, s.Serial) && !matchTarget(target, s.Device) {
				continue
			}
			if s.Temperature > val {
				out = append(out, synth("rule:disk-temp:"+s.Serial, internal.SeverityWarning, internal.CategoryThermal,
					"Disk temperature high: "+s.Device, fmt.Sprintf("%d°C exceeds threshold %d°C", s.Temperature, val)))
			}
		}
	case "avg_above":
		if len(smart) == 0 {
			return nil
		}
		sum := 0
		for _, s := range smart {
			sum += s.Temperature
		}
		avg := sum / len(smart)
		if avg > val {
			out = append(out, synth("rule:avg-disk-temp", internal.SeverityWarning, internal.CategoryThermal,
				"Average disk temperature high", fmt.Sprintf("Average %d°C exceeds threshold %d°C", avg, val)))
		}
	}
	return out
}

func evalSMART(cond, target string, val int64, smart []internal.SMARTInfo) []internal.Finding {
	var out []internal.Finding
	for _, s := range smart {
		if !matchTarget(target, s.Serial) && !matchTarget(target, s.Device) {
			continue
		}
		switch cond {
		case "health_fails":
			if !s.HealthPassed {
				out = append(out, synth("rule:smart-fail:"+s.Serial, internal.SeverityCritical, internal.CategorySMART,
					"SMART health failed: "+s.Device, s.Model+" (S/N: "+s.Serial+")"))
			}
		case "reallocated_above":
			if val > 0 && s.Reallocated > val {
				out = append(out, synth("rule:smart-realloc:"+s.Serial, internal.SeverityCritical, internal.CategorySMART,
					"Reallocated sectors high: "+s.Device, fmt.Sprintf("%d exceeds threshold %d", s.Reallocated, val)))
			}
		case "pending_above":
			if val > 0 && s.Pending > val {
				out = append(out, synth("rule:smart-pending:"+s.Serial, internal.SeverityWarning, internal.CategorySMART,
					"Pending sectors: "+s.Device, fmt.Sprintf("%d exceeds threshold %d", s.Pending, val)))
			}
		case "crc_above":
			if val > 0 && s.UDMACRC > val {
				out = append(out, synth("rule:smart-crc:"+s.Serial, internal.SeverityWarning, internal.CategorySMART,
					"UDMA CRC errors: "+s.Device, fmt.Sprintf("%d exceeds threshold %d", s.UDMACRC, val)))
			}
		case "power_hours_above":
			if val > 0 && s.PowerOnHours > val {
				out = append(out, synth("rule:smart-age:"+s.Serial, internal.SeverityInfo, internal.CategorySMART,
					"Drive age warning: "+s.Device, fmt.Sprintf("%.1f years (%.0f hours threshold)", float64(s.PowerOnHours)/8766, float64(val))))
			}
		}
	}
	return out
}

func evalService(cond, target string, val float64, services []internal.ServiceCheckResult) []internal.Finding {
	var out []internal.Finding
	for _, sc := range services {
		if !matchTarget(target, sc.Name) && !matchTarget(target, sc.Key) {
			continue
		}
		switch cond {
		case "down":
			if sc.Status == "down" {
				out = append(out, synth("rule:svc-down:"+sc.Key, internal.SeverityWarning, internal.CategoryService,
					"Service down: "+sc.Name, fmt.Sprintf("%s (%s) — %s", sc.Target, sc.Type, sc.Error)))
			}
		case "latency_above":
			if val > 0 && float64(sc.ResponseMS) > val {
				out = append(out, synth("rule:svc-latency:"+sc.Key, internal.SeverityWarning, internal.CategoryService,
					"Service latency high: "+sc.Name, fmt.Sprintf("%dms exceeds threshold %.0fms", sc.ResponseMS, val)))
			}
		}
	}
	return out
}

func evalParity(cond string, val float64, parity *internal.ParityInfo) []internal.Finding {
	if parity == nil || len(parity.History) == 0 {
		return nil
	}
	last := parity.History[len(parity.History)-1]
	switch cond {
	case "errors":
		if last.Errors > 0 {
			return []internal.Finding{synth("rule:parity-err:"+last.Date, internal.SeverityCritical, internal.CategoryParity,
				"Parity check errors: "+last.Date, fmt.Sprintf("%d errors found", last.Errors))}
		}
	case "speed_below":
		if val > 0 && last.SpeedMBs < val {
			return []internal.Finding{synth("rule:parity-slow:"+last.Date, internal.SeverityWarning, internal.CategoryParity,
				"Parity check slow", fmt.Sprintf("%.1f MB/s below threshold %.0f MB/s", last.SpeedMBs, val))}
		}
	}
	return nil
}

func evalUPS(cond string, val float64, ups *internal.UPSInfo) []internal.Finding {
	if ups == nil || !ups.Available {
		return nil
	}
	switch cond {
	case "on_battery":
		if ups.OnBattery {
			return []internal.Finding{synth("rule:ups-battery", internal.SeverityCritical, internal.CategoryUPS,
				"UPS on battery", fmt.Sprintf("Battery %.0f%%, runtime %.0f min", ups.BatteryPct, ups.RuntimeMins))}
		}
	case "battery_below":
		if val > 0 && ups.BatteryPct < val {
			return []internal.Finding{synth("rule:ups-low", internal.SeverityCritical, internal.CategoryUPS,
				"UPS battery low", fmt.Sprintf("%.0f%% below threshold %.0f%%", ups.BatteryPct, val))}
		}
	case "load_above":
		if val > 0 && ups.LoadPct > val {
			return []internal.Finding{synth("rule:ups-load", internal.SeverityWarning, internal.CategoryUPS,
				"UPS load high", fmt.Sprintf("%.0f%% exceeds threshold %.0f%%", ups.LoadPct, val))}
		}
	}
	return nil
}

func evalDocker(cond, target string, docker internal.DockerInfo) []internal.Finding {
	var out []internal.Finding
	for _, c := range docker.Containers {
		if !matchTarget(target, c.Name) {
			continue
		}
		switch cond {
		case "stopped":
			if c.State != "running" {
				out = append(out, synth("rule:docker-stop:"+c.Name, internal.SeverityWarning, internal.CategoryDocker,
					"Container stopped: "+c.Name, c.Image+" — state: "+c.State))
			}
		}
	}
	return out
}

func evalSystem(cond string, val float64, sys internal.SystemInfo) []internal.Finding {
	if val <= 0 {
		return nil
	}
	switch cond {
	case "cpu_above":
		if sys.CPUUsage > val {
			return []internal.Finding{synth("rule:sys-cpu", internal.SeverityWarning, internal.CategorySystem,
				"CPU usage high", fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", sys.CPUUsage, val))}
		}
	case "mem_above":
		if sys.MemPercent > val {
			return []internal.Finding{synth("rule:sys-mem", internal.SeverityWarning, internal.CategorySystem,
				"Memory usage high", fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", sys.MemPercent, val))}
		}
	case "load_above":
		if sys.LoadAvg1 > val {
			return []internal.Finding{synth("rule:sys-load", internal.SeverityWarning, internal.CategorySystem,
				"Load average high", fmt.Sprintf("%.2f exceeds threshold %.0f", sys.LoadAvg1, val))}
		}
	case "iowait_above":
		if sys.IOWait > val {
			return []internal.Finding{synth("rule:sys-iowait", internal.SeverityWarning, internal.CategorySystem,
				"I/O wait high", fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", sys.IOWait, val))}
		}
	}
	return nil
}

func evalZFS(cond, target string, val float64, zfs *internal.ZFSInfo) []internal.Finding {
	if zfs == nil || !zfs.Available {
		return nil
	}
	var out []internal.Finding
	for _, pool := range zfs.Pools {
		if !matchTarget(target, pool.Name) {
			continue
		}
		switch cond {
		case "degraded":
			if !strings.EqualFold(pool.State, "ONLINE") {
				out = append(out, synth("rule:zfs-degraded:"+pool.Name, internal.SeverityCritical, internal.CategoryZFS,
					"ZFS pool degraded: "+pool.Name, "State: "+pool.State))
			}
		case "scrub_errors":
			if pool.ScanErrors > 0 {
				out = append(out, synth("rule:zfs-scrub-err:"+pool.Name, internal.SeverityCritical, internal.CategoryZFS,
					"ZFS scrub errors: "+pool.Name, fmt.Sprintf("%d errors found", pool.ScanErrors)))
			}
		case "usage_above":
			if val > 0 && pool.UsedPct > val {
				out = append(out, synth("rule:zfs-usage:"+pool.Name, internal.SeverityWarning, internal.CategoryZFS,
					"ZFS pool usage high: "+pool.Name, fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", pool.UsedPct, val)))
			}
		}
	}
	return out
}

func evalTunnels(cond, target string, tunnels *internal.TunnelInfo) []internal.Finding {
	if tunnels == nil {
		return nil
	}
	var out []internal.Finding
	switch cond {
	case "cloudflared_down":
		if tunnels.Cloudflared != nil {
			for _, t := range tunnels.Cloudflared.Tunnels {
				if !matchTarget(target, t.Name) {
					continue
				}
				if t.Status != "healthy" {
					out = append(out, synth("rule:cf-down:"+t.Name, internal.SeverityWarning, internal.CategoryNetwork,
						"Cloudflared tunnel down: "+t.Name, "Status: "+t.Status))
				}
			}
		}
	case "tailscale_offline":
		if tunnels.Tailscale != nil {
			all := tunnels.Tailscale.Peers
			for _, nd := range all {
				if !matchTarget(target, nd.Name) {
					continue
				}
				if !nd.Online {
					out = append(out, synth("rule:ts-offline:"+nd.Name, internal.SeverityWarning, internal.CategoryNetwork,
						"Tailscale node offline: "+nd.Name, "IP: "+nd.IP))
				}
			}
		}
	}
	return out
}

func evalUpdate(update *internal.UpdateInfo) []internal.Finding {
	if update != nil && update.UpdateAvailable {
		return []internal.Finding{synth("rule:update", internal.SeverityInfo, internal.CategorySystem,
			"Platform update available", fmt.Sprintf("%s → %s", update.InstalledVersion, update.LatestVersion))}
	}
	return nil
}

func (s *Scheduler) logSuppressed(notif *notifier.Notifier, findings []internal.Finding, hostname string, cfg AlertingConfig, status string) {
	for _, wh := range notif.Webhooks() {
		if !wh.Enabled || len(findings) == 0 {
			continue
		}
		_ = s.store.SaveNotificationLog(wh.Name, wh.Type, status, len(findings), "")
	}
	s.logger.Info("notifications suppressed", "reason", status, "hostname", hostname)
}

func (s *Scheduler) applyCooldown(findings []internal.Finding, routeKey string, cooldown time.Duration, now time.Time) []internal.Finding {
	seen := map[string]struct{}{}
	out := make([]internal.Finding, 0, len(findings))
	for _, f := range findings {
		fp := findingFingerprint(f)
		if fp == "" {
			out = append(out, f)
			continue
		}
		if _, exists := seen[fp]; exists {
			continue
		}
		seen[fp] = struct{}{}

		suppressed, _, err := s.store.IsAlertSuppressed(fp, now)
		if err != nil {
			s.logger.Warn("alert suppression check failed", "fingerprint", fp, "error", err)
		} else if suppressed {
			continue
		}

		allowed, err := s.store.CanSendNotification(fp, routeKey, cooldown, now)
		if err != nil {
			s.logger.Warn("cooldown check failed; allowing notification", "route", routeKey, "fingerprint", fp, "error", err)
			out = append(out, f)
			continue
		}
		if allowed {
			out = append(out, f)
		}
	}
	return out
}

func matchesHostname(hosts []string, hostname string) bool {
	if len(hosts) == 0 {
		return true
	}
	h := strings.ToLower(strings.TrimSpace(hostname))
	for _, c := range hosts {
		if strings.ToLower(strings.TrimSpace(c)) == h {
			return true
		}
	}
	return false
}

func severityRank(sev internal.Severity) int {
	switch sev {
	case internal.SeverityCritical:
		return 3
	case internal.SeverityWarning:
		return 2
	case internal.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func inQuietHours(cfg QuietHours, now time.Time) bool {
	if !cfg.Enabled {
		return false
	}
	start, err := parseHHMM(cfg.StartHHMM)
	if err != nil {
		return false
	}
	end, err := parseHHMM(cfg.EndHHMM)
	if err != nil {
		return false
	}
	if start == end {
		return false
	}

	loc := time.UTC
	if cfg.Timezone != "" {
		if loaded, err := time.LoadLocation(cfg.Timezone); err == nil {
			loc = loaded
		}
	}
	localNow := now.In(loc)
	mins := localNow.Hour()*60 + localNow.Minute()

	if start < end {
		return mins >= start && mins < end
	}
	return mins >= start || mins < end
}

func parseHHMM(v string) (int, error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time")
	}
	h, err := time.Parse("15:04", fmt.Sprintf("%s:%s", parts[0], parts[1]))
	if err != nil {
		return 0, err
	}
	return h.Hour()*60 + h.Minute(), nil
}

func inMaintenanceWindow(windows []MaintenanceWindow, hostname string, now time.Time) bool {
	for _, w := range windows {
		if !w.Enabled {
			continue
		}
		if !matchesHostname(w.Hostnames, hostname) {
			continue
		}
		start, err1 := time.Parse(time.RFC3339, strings.TrimSpace(w.StartISO))
		end, err2 := time.Parse(time.RFC3339, strings.TrimSpace(w.EndISO))
		if err1 != nil || err2 != nil || !end.After(start) {
			continue
		}
		if (now.Equal(start) || now.After(start)) && now.Before(end) {
			return true
		}
	}
	return false
}

func findingFingerprint(f internal.Finding) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(string(f.Category))),
		strings.ToLower(strings.TrimSpace(f.Title)),
		strings.ToLower(strings.TrimSpace(f.RelatedDisk)),
	}
	raw := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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
