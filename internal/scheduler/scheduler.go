// Package scheduler handles periodic diagnostic collection runs.
package scheduler

import (
	"fmt"
	"log/slog"
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
	collector         *collector.Collector
	store             storage.Store
	notifier          *notifier.Notifier
	metrics           *notifier.Metrics
	logger            *slog.Logger
	interval          time.Duration
	speedTestInterval time.Duration
	speedTestSchedule []string // specific HH:MM times, overrides interval when set
	speedTestDay      string   // "monday"-"sunday" or "1","15" for monthly
	speedTestFreq     string   // "24h", "weekly", "monthly" — only when schedule is set
	retention         RetentionConfig
	alerting          AlertingConfig
	serviceChecks     []internal.ServiceCheckConfig
	checker           *ServiceChecker
	retentionMgr      *RetentionManager

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
	store storage.Store,
	notif *notifier.Notifier,
	metrics *notifier.Metrics,
	logger *slog.Logger,
	interval time.Duration,
) *Scheduler {
	s := &Scheduler{
		collector:         col,
		store:             store,
		notifier:          notif,
		metrics:           metrics,
		logger:            logger,
		interval:          interval,
		speedTestInterval: 4 * time.Hour,
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
		stop:          make(chan struct{}),
		restart:       make(chan time.Duration, 1),
	}
	s.checker = NewServiceChecker(store, logger)
	// Opt into the per-type Details map on the scheduled path too —
	// HTTP status codes, resolved IPs, DNS records, Ping RTT, failure
	// stages are persisted into service_checks_history.details_json so
	// the /service-checks log UI can render the same rich context the
	// Test button already shows. Overhead is well under 1ms per check
	// (the runners already compute these values; we just stop
	// discarding them). See issue #182.
	s.checker.SetCollectDetails(true)
	s.retentionMgr = NewRetentionManager(store, store, logger)
	return s
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

	// Independent container & process stats loop — collects Docker metrics and
	// top processes every 5 minutes for chart history (full scans happen at the
	// configured interval which is too infrequent for granular charts).
	go func() {
		time.Sleep(30 * time.Second) // let first scan finish
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.collectContainerStats()
				s.collectProcessStats()
			case <-s.stop:
				return
			}
		}
	}()

	// Independent speed test loop — runs on interval or at scheduled times
	go func() {
		time.Sleep(2 * time.Minute)
		s.runSpeedTest()
		ticker := time.NewTicker(1 * time.Minute) // check every minute for schedule hits
		defer ticker.Stop()
		lastRun := time.Now()
		for {
			select {
			case <-ticker.C:
				s.mu.RLock()
				schedule := s.speedTestSchedule
				interval := s.speedTestInterval
				s.mu.RUnlock()
				now := time.Now()
				if len(schedule) > 0 {
					s.mu.RLock()
					day := s.speedTestDay
					freq := s.speedTestFreq
					s.mu.RUnlock()
					// Check if today matches the schedule day
					dayMatch := true
					if freq == "weekly" && day != "" {
						dayMatch = strings.EqualFold(now.Weekday().String(), day)
					} else if freq == "monthly" && day != "" {
						dayNum, _ := strconv.Atoi(day)
						dayMatch = dayNum > 0 && now.Day() == dayNum
					}
					// Scheduled mode: run at specific HH:MM times on matching days
					nowHHMM := now.Format("15:04")
					for _, t := range schedule {
						if dayMatch && nowHHMM == t && now.Sub(lastRun) > 5*time.Minute {
							s.runSpeedTest()
							lastRun = now
							break
						}
					}
				} else {
					// Interval mode
					if now.Sub(lastRun) >= interval {
						s.runSpeedTest()
						lastRun = now
					}
				}
			case <-s.stop:
				return
			}
		}
	}()
}

// UpdateInterval dynamically changes the scan interval without restarting.
// SetSpeedTestInterval updates how often the speed test runs.
func (s *Scheduler) SetSpeedTestInterval(d time.Duration) {
	if d < 5*time.Minute {
		d = 5 * time.Minute
	}
	s.mu.Lock()
	s.speedTestInterval = d
	s.mu.Unlock()
	s.logger.Info("speed test interval updated", "interval", d)
}

// SetSpeedTestSchedule sets specific times of day to run speed tests.
func (s *Scheduler) SetSpeedTestSchedule(times []string, day string, freq string) {
	s.mu.Lock()
	s.speedTestSchedule = times
	s.speedTestDay = day
	s.speedTestFreq = freq
	s.mu.Unlock()
	if len(times) > 0 {
		s.logger.Info("speed test schedule set", "times", times, "day", day, "freq", freq)
	}
}

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

	// Also persist process stats to the dedicated history table (same data
	// that the 5-minute loop collects, but captured during full scans too
	// so there are no gaps in the chart timeline).
	if len(snap.System.TopProcesses) > 0 {
		if err := s.store.SaveProcessStats(snap.System.TopProcesses); err != nil {
			s.logger.Warn("failed to save process stats during full scan", "error", err)
		}
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

// pruneData runs all data lifecycle maintenance tasks via RetentionManager.
func (s *Scheduler) pruneData() {
	s.mu.RLock()
	ret := s.retention
	s.mu.RUnlock()

	result := s.retentionMgr.RunRetention(RetentionManagerConfig{
		SnapshotMaxAge:     time.Duration(ret.SnapshotDays) * 24 * time.Hour,
		SnapshotKeepMin:    10,
		ServiceCheckMaxAge: time.Duration(ret.NotifyLogDays) * 24 * time.Hour,
		NotificationMaxAge: time.Duration(ret.NotifyLogDays) * 24 * time.Hour,
		AlertMaxAge:        time.Duration(ret.NotifyLogDays) * 24 * time.Hour,
		MaxDBSizeMB:        float64(ret.MaxDBSizeMB),
	})
	if result.SnapshotsPruned > 0 || result.ServiceChecksPruned > 0 || result.NotificationsPruned > 0 || result.AlertsPruned > 0 {
		s.logger.Info("retention complete",
			"snapshots", result.SnapshotsPruned,
			"service_checks", result.ServiceChecksPruned,
			"notifications", result.NotificationsPruned,
			"alerts", result.AlertsPruned,
			"orphans", result.OrphansPruned,
		)
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
// It also purges service_checks_history rows whose keys are no longer in the
// config — otherwise stale checks keep appearing on the /service-checks page
// (issue #133). Because this runs both on startup (from main.go) and on every
// settings save, it also cleans up orphans left over from upgraded installs.
func (s *Scheduler) UpdateServiceChecks(checks []internal.ServiceCheckConfig) {
	normalized := make([]internal.ServiceCheckConfig, 0, len(checks))
	for _, check := range checks {
		check.Type = strings.ToLower(strings.TrimSpace(check.Type))
		check.Name = strings.TrimSpace(check.Name)
		check.Target = strings.TrimSpace(check.Target)
		if check.Name == "" || check.Target == "" {
			continue
		}
		if !IsSupportedCheckType(check.Type) {
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

	// Purge orphaned history for any check that was removed from the config.
	if s.store != nil {
		keepKeys := make([]string, 0, len(normalized))
		for _, c := range normalized {
			keepKeys = append(keepKeys, CheckKey(c))
		}
		if pruned, err := s.store.DeleteServiceChecksNotIn(keepKeys); err != nil {
			s.logger.Warn("prune orphaned service check history", "error", err)
		} else if pruned > 0 {
			s.logger.Info("pruned orphaned service check history", "rows", pruned)
		}
	}

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

	result, err := s.retentionMgr.RunBackup(BackupManagerConfig{
		Enabled:   cfg.Enabled,
		Path:      cfg.Path,
		KeepCount: cfg.KeepCount,
		IntervalH: cfg.IntervalH,
	}, cfg.LastBackup, time.Now())
	if err != nil {
		s.logger.Warn("auto backup failed", "error", err)
		return
	}
	if result != nil {
		s.mu.Lock()
		s.backup.LastBackup = result.Timestamp
		s.mu.Unlock()
	}
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

func (s *Scheduler) runServiceChecks(now time.Time) ([]internal.ServiceCheckResult, error) {
	s.mu.RLock()
	checks := make([]internal.ServiceCheckConfig, len(s.serviceChecks))
	copy(checks, s.serviceChecks)
	s.mu.RUnlock()

	if len(checks) == 0 {
		return []internal.ServiceCheckResult{}, nil
	}

	// Filter to enabled-only for the full-scan path (RunCheck executes all passed checks).
	var enabled []internal.ServiceCheckConfig
	for _, c := range checks {
		if c.Enabled {
			enabled = append(enabled, c)
		}
	}
	if len(enabled) == 0 {
		return []internal.ServiceCheckResult{}, nil
	}

	results := make([]internal.ServiceCheckResult, 0, len(enabled))
	for _, check := range enabled {
		result := s.checker.RunCheck(check, now)

		// Track consecutive failures from store history.
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

// collectContainerStats runs a lightweight Docker stats collection and saves to DB.
// This runs every 5 minutes independently of the main scan (which runs every 6h)
// to provide enough data points for the container metrics charts.
func (s *Scheduler) collectContainerStats() {
	docker, err := s.collector.CollectDockerStats()
	if err != nil || docker == nil || !docker.Available {
		return
	}
	if err := s.store.SaveContainerStats(docker); err != nil {
		s.logger.Warn("failed to save container stats", "error", err)
		return
	}
	// Update the cached snapshot's Docker data so the dashboard shows fresh values
	s.mu.Lock()
	if s.latest != nil {
		s.latest.Docker = *docker
	}
	s.mu.Unlock()
}

// collectProcessStats runs a lightweight top-processes collection and saves to DB.
// This runs every 5 minutes alongside collectContainerStats to provide enough
// data points for the process resource usage charts.
func (s *Scheduler) collectProcessStats() {
	procs := s.collector.CollectTopProcesses(15)
	if len(procs) == 0 {
		return
	}

	// Enrich with container attribution from cached Docker data
	s.mu.RLock()
	var containers []internal.ContainerInfo
	if s.latest != nil && s.latest.Docker.Available {
		containers = s.latest.Docker.Containers
	}
	s.mu.RUnlock()

	if len(containers) > 0 {
		collector.EnrichProcessContainers(procs, containers, "")
	}

	// Persist to process_history table
	if err := s.store.SaveProcessStats(procs); err != nil {
		s.logger.Warn("failed to save process stats", "error", err)
		return
	}

	// Update cached snapshot with fresh process data
	s.mu.Lock()
	if s.latest != nil {
		s.latest.System.TopProcesses = procs
	}
	s.mu.Unlock()
}

// runSpeedTest executes a network speed test and stores the result.
func (s *Scheduler) runSpeedTest() {
	s.logger.Info("running speed test")
	result := collector.RunSpeedTest()
	if result == nil {
		s.logger.Info("speed test: no speedtest tool available (install speedtest or speedtest-cli)")
		return
	}
	s.logger.Info("speed test complete",
		"download", fmt.Sprintf("%.1f Mbps", result.DownloadMbps),
		"upload", fmt.Sprintf("%.1f Mbps", result.UploadMbps),
		"latency", fmt.Sprintf("%.1f ms", result.LatencyMs),
	)
	// Store in DB
	if err := s.store.SaveSpeedTest("speedtest-"+time.Now().Format("20060102-150405"), result); err != nil {
		s.logger.Warn("failed to save speed test result", "error", err)
	}
	// Update the cached snapshot's speed test field
	s.mu.Lock()
	if s.latest != nil {
		s.latest.SpeedTest = &internal.SpeedTestInfo{
			Available: true,
			Latest:    result,
		}
	}
	s.mu.Unlock()
}

func (s *Scheduler) runDueServiceChecks() {
	s.mu.RLock()
	checks := make([]internal.ServiceCheckConfig, len(s.serviceChecks))
	copy(checks, s.serviceChecks)
	s.mu.RUnlock()
	s.checker.RunDueChecks(checks, time.Now())
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
