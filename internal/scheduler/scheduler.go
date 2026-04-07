// Package scheduler handles periodic diagnostic collection runs.
package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
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

// AlertingConfig controls policy routing and suppression behavior.
type AlertingConfig struct {
	Policies           []AlertPolicy       `json:"policies,omitempty"`
	QuietHours         QuietHours          `json:"quiet_hours,omitempty"`
	MaintenanceWindows []MaintenanceWindow `json:"maintenance_windows,omitempty"`
	DefaultCooldownSec int                 `json:"default_cooldown_sec,omitempty"`
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
	alerting  AlertingConfig

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
		s.dispatchNotifications(notif, snap.Findings, hostname, snap.Timestamp)
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

func (s *Scheduler) dispatchNotifications(notif *notifier.Notifier, findings []internal.Finding, hostname string, now time.Time) {
	if notif == nil || len(findings) == 0 {
		return
	}

	s.mu.RLock()
	cfg := s.alerting
	s.mu.RUnlock()

	if cfg.DefaultCooldownSec <= 0 {
		cfg.DefaultCooldownSec = 900
	}

	if inMaintenanceWindow(cfg.MaintenanceWindows, hostname, now) {
		s.logSuppressed(notif, findings, hostname, cfg, "suppressed_maintenance")
		return
	}
	if inQuietHours(cfg.QuietHours, now) {
		s.logSuppressed(notif, findings, hostname, cfg, "suppressed_quiet_hours")
		return
	}

	if len(cfg.Policies) == 0 {
		s.dispatchLegacyNotifications(notif, findings, hostname, now, cfg.DefaultCooldownSec)
		return
	}

	webhooks := make(map[string]internal.WebhookConfig)
	for _, wh := range notif.Webhooks() {
		webhooks[strings.ToLower(strings.TrimSpace(wh.Name))] = wh
	}

	matchedPolicy := false
	for i, policy := range cfg.Policies {
		if !policy.Enabled {
			continue
		}
		if !matchesHostname(policy.Hostnames, hostname) {
			continue
		}

		whName := strings.ToLower(strings.TrimSpace(policy.WebhookName))
		if whName == "" {
			continue
		}
		wh, ok := webhooks[whName]
		if !ok || !wh.Enabled {
			s.logger.Warn("alert policy references unknown webhook", "policy", policy.Name, "webhook", policy.WebhookName)
			continue
		}

		filtered := filterFindingsForPolicy(findings, policy)
		if len(filtered) == 0 {
			continue
		}
		matchedPolicy = true

		routeKey := policyRouteKey(policy, i, wh.Name)
		cooldown := time.Duration(policy.CooldownSec) * time.Second
		if cooldown <= 0 {
			cooldown = time.Duration(cfg.DefaultCooldownSec) * time.Second
		}

		toSend := s.applyCooldown(filtered, routeKey, cooldown, now)
		if len(toSend) == 0 {
			_ = s.store.SaveNotificationLog(wh.Name, wh.Type, "suppressed_cooldown", len(filtered), "")
			continue
		}

		if err := notif.NotifyWebhook(wh, toSend, hostname); err != nil {
			continue
		}

		fingerprints := make([]string, 0, len(toSend))
		for _, f := range toSend {
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

	if !matchedPolicy {
		s.dispatchLegacyNotifications(notif, findings, hostname, now, cfg.DefaultCooldownSec)
	}
}

func (s *Scheduler) dispatchLegacyNotifications(notif *notifier.Notifier, findings []internal.Finding, hostname string, now time.Time, defaultCooldownSec int) {
	if defaultCooldownSec <= 0 {
		defaultCooldownSec = 900
	}
	for _, wh := range notif.Webhooks() {
		if !wh.Enabled {
			continue
		}
		filtered := filterBySeverity(findings, wh.MinLevel)
		if len(filtered) == 0 {
			continue
		}
		routeKey := "legacy:" + strings.ToLower(strings.TrimSpace(wh.Name))
		toSend := s.applyCooldown(filtered, routeKey, time.Duration(defaultCooldownSec)*time.Second, now)
		if len(toSend) == 0 {
			_ = s.store.SaveNotificationLog(wh.Name, wh.Type, "suppressed_cooldown", len(filtered), "")
			continue
		}
		if err := notif.NotifyWebhook(wh, toSend, hostname); err != nil {
			continue
		}
		fingerprints := make([]string, 0, len(toSend))
		for _, f := range toSend {
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
}

func (s *Scheduler) logSuppressed(notif *notifier.Notifier, findings []internal.Finding, hostname string, cfg AlertingConfig, status string) {
	if len(cfg.Policies) == 0 {
		for _, wh := range notif.Webhooks() {
			if !wh.Enabled {
				continue
			}
			filtered := filterBySeverity(findings, wh.MinLevel)
			if len(filtered) == 0 {
				continue
			}
			if err := s.store.SaveNotificationLog(wh.Name, wh.Type, status, len(filtered), ""); err != nil {
				s.logger.Warn("failed to save suppressed notification log", "error", err)
			}
		}
		s.logger.Info("notifications suppressed", "reason", status, "hostname", hostname)
		return
	}

	webhooks := make(map[string]internal.WebhookConfig)
	for _, wh := range notif.Webhooks() {
		webhooks[strings.ToLower(strings.TrimSpace(wh.Name))] = wh
	}
	for _, policy := range cfg.Policies {
		if !policy.Enabled || !matchesHostname(policy.Hostnames, hostname) {
			continue
		}
		wh, ok := webhooks[strings.ToLower(strings.TrimSpace(policy.WebhookName))]
		if !ok || !wh.Enabled {
			continue
		}
		filtered := filterFindingsForPolicy(findings, policy)
		if len(filtered) == 0 {
			continue
		}
		if err := s.store.SaveNotificationLog(wh.Name, wh.Type, status, len(filtered), ""); err != nil {
			s.logger.Warn("failed to save suppressed policy log", "error", err)
		}
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

func filterFindingsForPolicy(findings []internal.Finding, policy AlertPolicy) []internal.Finding {
	min := policy.MinSeverity
	if min == "" {
		min = internal.SeverityWarning
	}

	catSet := make(map[string]struct{}, len(policy.Categories))
	for _, c := range policy.Categories {
		catSet[strings.ToLower(string(c))] = struct{}{}
	}

	out := make([]internal.Finding, 0, len(findings))
	for _, f := range findings {
		if severityRank(f.Severity) < severityRank(min) {
			continue
		}
		if len(catSet) > 0 {
			if _, ok := catSet[strings.ToLower(string(f.Category))]; !ok {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

func filterBySeverity(findings []internal.Finding, minLevel internal.Severity) []internal.Finding {
	min := minLevel
	if min == "" {
		min = internal.SeverityWarning
	}
	out := make([]internal.Finding, 0, len(findings))
	for _, f := range findings {
		if severityRank(f.Severity) >= severityRank(min) {
			out = append(out, f)
		}
	}
	return out
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

func matchesHostname(policyHosts []string, hostname string) bool {
	if len(policyHosts) == 0 {
		return true
	}
	h := strings.ToLower(strings.TrimSpace(hostname))
	for _, candidate := range policyHosts {
		if strings.ToLower(strings.TrimSpace(candidate)) == h {
			return true
		}
	}
	return false
}

func policyRouteKey(policy AlertPolicy, idx int, webhookName string) string {
	if name := strings.TrimSpace(policy.Name); name != "" {
		return "policy:" + name
	}
	return fmt.Sprintf("policy:%d:%s", idx, strings.ToLower(strings.TrimSpace(webhookName)))
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
