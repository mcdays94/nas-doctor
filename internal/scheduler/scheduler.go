// Package scheduler handles periodic diagnostic collection runs.
package scheduler

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"context"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/analyzer"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
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

// SpeedTestIntervalDisabled is the sentinel value for the standalone
// speed-test loop's interval that means "do not run". When this is set,
// runSpeedTest skips the Ookla invocation and records status=disabled
// once in LastSpeedTestAttempt (no bandwidth consumption, no
// speedtest_history write, no churn on subsequent tick calls). Maps
// to the "Disabled" option in the settings UI (#180, for users on
// metered connections) and is read by the scheduled type=speed
// service check to report down with "speed test disabled in settings"
// rather than the misleading "no tool available" error (#210).
//
// A negative duration is chosen because it cannot collide with any
// real positive interval and explicitly survives the 5-minute minimum
// clamp in SetSpeedTestInterval. Zero is NOT used as the sentinel
// because zero-value Duration fields would accidentally opt users
// into the disabled state.
const SpeedTestIntervalDisabled time.Duration = -1

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
	// speedTestRunFn is the injectable speed-test runner. Production
	// uses collector.RunSpeedTest via the default wired in New(); tests
	// swap this to observe scheduler behaviour without spawning Ookla.
	// Matches the existing ServiceChecker.speedTestRunFn naming for
	// consistency across the scheduler package.
	speedTestRunFn SpeedTestRunner
	// dockerStatsFn is the injectable seam for the 5-minute container
	// stats loop. Production wires collector.CollectDockerStats via
	// New(); tests swap it for a deterministic stub to exercise the
	// three logging branches (error / unavailable / success) added in
	// issue #226. Read under s.mu.
	dockerStatsFn  func() (*internal.DockerInfo, error)
	retention      RetentionConfig
	alerting       AlertingConfig
	serviceChecks  []internal.ServiceCheckConfig
	checker        *ServiceChecker
	retentionMgr   *RetentionManager

	// smartMaxAgeDays is the Settings.SMART.MaxAgeDays value driving
	// the StaleSMARTChecker (issue #238). 0 disables the feature
	// entirely. Mutated by SetSMARTMaxAgeDays from the API handler
	// when the user changes the setting. Read under s.mu.
	smartMaxAgeDays int

	// dispatcher owns per-subsystem scan scheduling for the 6
	// configurable subsystems (issue #260 / PRD #239 slice 2b).
	// Non-nil after New(); safe for concurrent use. Settings saves
	// push new per-subsystem intervals via dispatcher.UpdateIntervals
	// from the API handler.
	dispatcher *ScanDispatcher

	// liveTestRegistry is the singleton-acquire registry that drives
	// the speed-test broadcast state machine (PRD #283 slice 2 /
	// issue #285). When non-nil, runSpeedTest routes through it and
	// the manual /api/v1/speedtest/run + cron-driven scheduled paths
	// share a single in-flight test (idempotency). When nil, the
	// legacy speedTestRunFn path is used (preserves test seams that
	// don't need the registry — see SetSpeedTestRunner).
	liveTestRegistry livetest.Registry

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
		speedTestRunFn:    collector.RunSpeedTest,
		dockerStatsFn:     col.CollectDockerStats,
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
		// Default matches defaultSettings().SMART.MaxAgeDays (#237).
		// Callers that want a different value (or zero to disable)
		// should call SetSMARTMaxAgeDays after construction.
		smartMaxAgeDays: 7,
	}
	// Dispatcher starts with all-subsystems-on-global; callers (the
	// API settings handler on startup) push the user's configured
	// per-subsystem intervals via SetDispatcherIntervals once
	// Settings.AdvancedScans is loaded.
	s.dispatcher = NewScanDispatcher(DispatcherIntervalsConfig{}, interval, nil)
	s.checker = NewServiceChecker(store, logger)
	// Opt into the per-type Details map on the scheduled path too —
	// HTTP status codes, resolved IPs, DNS records, Ping RTT, failure
	// stages are persisted into service_checks_history.details_json so
	// the /service-checks log UI can render the same rich context the
	// Test button already shows. Overhead is well under 1ms per check
	// (the runners already compute these values; we just stop
	// discarding them). See issue #182.
	s.checker.SetCollectDetails(true)
	// Wire the default mtr-based traceroute runner so scheduled
	// type=traceroute checks produce real results. cycles comes from
	// runTraceCheck's hardcoded scheduledCycles (5). See issue #189.
	s.checker.SetTraceRunner(collector.RunMTR)
	s.retentionMgr = NewRetentionManager(store, store, logger)
	return s
}

// Start begins the periodic collection loop. It runs the first collection
// immediately, then repeats at the dispatcher's FastestInterval — which
// is min(global scan_interval, all non-zero per-subsystem intervals)
// per issue #260 / PRD #239 slice 2b. Each tick the dispatcher decides
// which of the 6 configurable subsystems to run; the 9 non-configurable
// subsystems always run every tick (cheap enough and the dispatcher is
// not responsible for them).
//
// Also starts an independent service check loop (30s tick) that respects
// per-check intervals.
func (s *Scheduler) Start() {
	tickInterval := s.dispatcher.FastestInterval()
	s.logger.Info("scheduler starting",
		"global_interval", s.interval,
		"tick_interval", tickInterval,
	)
	// Main diagnostic collection loop
	go func() {
		s.RunOnce()
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.RunOnce()
			case newInterval := <-s.restart:
				ticker.Stop()
				// The `restart` channel carries a GLOBAL scan_interval
				// change (from UpdateInterval) OR a dispatcher-driven
				// FastestInterval refresh (from SetDispatcherIntervals
				// — in that case the dispatcher has already been
				// updated and we just need to rebuild the ticker).
				// We distinguish by comparing against the dispatcher's
				// current FastestInterval: if it equals newInterval,
				// it's the dispatcher path (global unchanged); else
				// it's a user global update and we push the new
				// global through to the dispatcher.
				s.mu.Lock()
				if newInterval != s.dispatcher.FastestInterval() {
					s.interval = newInterval
					// Global changed → push through to dispatcher so
					// "use global" subsystems pick it up.
					s.dispatcher.SetGlobal(newInterval)
				}
				s.mu.Unlock()
				fresh := s.dispatcher.FastestInterval()
				ticker = time.NewTicker(fresh)
				s.logger.Info("scheduler interval updated",
					"global_interval", s.interval,
					"tick_interval", fresh,
				)
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
//
// The SpeedTestIntervalDisabled sentinel (#180/#210) short-circuits
// the 5-minute minimum clamp and is preserved as-is so users on
// metered connections can turn the loop off entirely. All other
// values are clamped to 5 minutes to protect bandwidth. The
// runSpeedTest branch handles the sentinel by skipping the Ookla
// invocation and recording status=disabled in LastSpeedTestAttempt.
func (s *Scheduler) SetSpeedTestInterval(d time.Duration) {
	if d != SpeedTestIntervalDisabled && d < 5*time.Minute {
		d = 5 * time.Minute
	}
	s.mu.Lock()
	s.speedTestInterval = d
	s.mu.Unlock()
	if d == SpeedTestIntervalDisabled {
		s.logger.Info("speed test loop disabled by user setting (issue #180)")
	} else {
		s.logger.Info("speed test interval updated", "interval", d)
	}
}

// SetSpeedTestRunner injects the function used by runSpeedTest to
// execute the actual network test. Production wires the default to
// collector.RunSpeedTest via the constructor; tests swap it for a
// deterministic stub so runSpeedTest can record attempt state
// (success / failed / pending / disabled) without spawning Ookla.
//
// Matches the existing ServiceChecker.SetSpeedTestRunner naming for
// consistency across the scheduler package. Originally introduced in
// #180 (for the disabled-loop test coverage); extended in #210 to
// cover all four attempt-state branches.
func (s *Scheduler) SetSpeedTestRunner(fn SpeedTestRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speedTestRunFn = fn
}

// SetLiveTestRegistry wires the registry that drives speed-test
// broadcast fan-out. Production wiring (cmd/nas-doctor/main.go)
// constructs a livetest.Manager around collector.DefaultSpeedTestRunner
// and passes it here. Tests typically leave the registry nil and use
// SetSpeedTestRunner to inject the legacy result-only stub — that
// path is preserved so existing scheduler tests (#180, #210) keep
// working unchanged. PRD #283 slice 2 / issue #285.
func (s *Scheduler) SetLiveTestRegistry(reg livetest.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.liveTestRegistry = reg
}

// LiveTestRegistry returns the wired registry. Used by the API layer
// to route POST /api/v1/speedtest/run + GET /api/v1/speedtest/stream/{id}
// requests through the same singleton lock that the scheduler uses.
func (s *Scheduler) LiveTestRegistry() livetest.Registry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liveTestRegistry
}

// SetDockerStatsFn injects the function used by collectContainerStats
// to collect the 5-minute Docker stats snapshot. Production wires the
// default to collector.CollectDockerStats via the constructor; tests
// swap it for a stub so they can exercise the error / unavailable /
// success logging branches without a live Docker daemon (issue #226).
func (s *Scheduler) SetDockerStatsFn(fn func() (*internal.DockerInfo, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dockerStatsFn = fn
}

// SetSMARTMaxAgeDays updates the max-age threshold driving the
// StaleSMARTChecker (issue #238). 0 disables the feature entirely —
// in which case RunOnce skips the Check+Apply pass completely and
// the scheduler behaves exactly like v0.9.5 (user story 5).
//
// Called by the API handler when the user edits
// Settings.SMART.MaxAgeDays; also by main.go on startup to apply the
// persisted preference.
func (s *Scheduler) SetSMARTMaxAgeDays(days int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.smartMaxAgeDays = days
}

// SetDispatcherIntervals pushes updated per-subsystem intervals into
// the ScanDispatcher. Called by the API settings handler on save so
// the new cadences take effect without restarting the scheduler
// (issue #260 user story 1-6). Also restarts the main scan loop
// ticker at the new FastestInterval via UpdateInterval, since a
// newly-faster subsystem may require the ticker to fire more often.
//
// global is the canonical scan_interval the caller just parsed from
// settings; passed explicitly (rather than read from s.interval) to
// avoid a race with a concurrent UpdateInterval that may not have
// been consumed by the main loop yet. In practice the API handler
// calls UpdateInterval then SetDispatcherIntervals in sequence on
// the same goroutine — passing global through makes that ordering
// irrelevant.
func (s *Scheduler) SetDispatcherIntervals(cfg DispatcherIntervalsConfig, global time.Duration) {
	if global <= 0 {
		// Defensive: if caller didn't know the global (or passed a
		// sentinel), fall back to whatever the scheduler has cached.
		s.mu.RLock()
		global = s.interval
		s.mu.RUnlock()
	} else {
		// Caller supplied a fresh global — also update s.interval so
		// subsequent reads see it. If a concurrent UpdateInterval is
		// already propagating the same value, the main loop will
		// simply re-tick at the same interval (harmless).
		s.mu.Lock()
		s.interval = global
		s.mu.Unlock()
	}
	s.dispatcher.UpdateIntervals(cfg, global)
	// Re-size the main-loop ticker to match the new FastestInterval.
	// If nothing actually changed, UpdateInterval's select-default
	// short-circuits gracefully.
	s.UpdateInterval(s.dispatcher.FastestInterval())
	s.logger.Info("scan dispatcher intervals updated",
		"fastest_interval", s.dispatcher.FastestInterval(),
	)
}

// Dispatcher returns the scheduler's ScanDispatcher. Exposed so the
// API layer can surface per-subsystem lastRun timestamps on
// /api/v1/snapshot/latest without reaching into scheduler internals.
// Callers should treat the returned value as read-mostly; mutate only
// via SetDispatcherIntervals.
func (s *Scheduler) Dispatcher() *ScanDispatcher {
	return s.dispatcher
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

	// Phase 1: collect the 9 non-configurable subsystems (system,
	// disks, network, logs, parity, UPS, update check, tunnels,
	// backup). These always run every tick — they're cheap and have
	// no user-facing cadence knob.
	snap, err := s.collector.Collect()
	if err != nil {
		s.logger.Error("collection failed", "error", err)
		return
	}

	// Phase 2: dispatch per-subsystem collectors per their configured
	// cadence (issue #260 / PRD #239). The dispatcher's Tick decides
	// which of the 6 configurable subsystems (SMART, Docker, Proxmox,
	// Kubernetes, ZFS, GPU) are due on this tick. Previously-collected
	// values from prior ticks are carried forward on snap by merging
	// from s.latest below.
	now := snap.Timestamp
	due := s.dispatcher.Tick(now)
	skipped := Skipped(due)

	// Carry forward the most recent cached values for subsystems that
	// are NOT running this tick. A subsystem that skipped should keep
	// its previous snapshot value rather than showing empty data.
	s.carryForwardSubsystems(snap, skipped)

	// Emit the canonical per-tick INFO log summarising dispatcher
	// decisions (user story 15).
	s.logger.Info("scan tick",
		"due", due,
		"skipped", skipped,
		"tick_interval", s.dispatcher.FastestInterval(),
	)

	for _, subsystem := range due {
		s.runSubsystem(subsystem, snap)
		s.dispatcher.MarkRan(subsystem, now)
	}

	// Stamp per-subsystem last-run timestamps on the snapshot so API
	// consumers can surface "last scanned 4m ago" style staleness
	// indicators (issue #260 user story 17; dashboard UI deferred).
	lastRun := s.dispatcher.LastRunMap()
	if len(lastRun) > 0 {
		snap.SubsystemLastRan = make(map[string]string, len(lastRun))
		for name, ts := range lastRun {
			snap.SubsystemLastRan[name] = ts.Format(time.RFC3339)
		}
	}

	// Drive replacement detection (issue #130). Runs on every scan but
	// only actually does work on Unraid, where ArraySlot gives us a
	// stable per-bay identifier that outlives individual drives.
	if err := detectDriveReplacements(s.store, snap.System.Platform, snap.SMART, snap.Timestamp); err != nil {
		s.logger.Warn("drive replacement detection failed", "error", err)
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

// runSubsystem invokes the matching per-subsystem Collector method
// and merges its result into snap. The stale-SMART force-wake safety
// net fires immediately after CollectSMART (issue #260 — relocated
// from the post-Collect() call site that slice 1 used, so max-age is
// now scoped to SMART's cadence).
func (s *Scheduler) runSubsystem(name string, snap *internal.Snapshot) {
	switch name {
	case "smart":
		smart, standby, _ := s.collector.CollectSMART(snap.System.Platform)
		snap.SMART = smart
		snap.SMARTStandbyDevices = standby
		// Stale-SMART force-wake (issue #238 / slice 1). Previously
		// ran unconditionally after Collect(); now runs ONLY when
		// SMART actually ran on this tick, so max-age honours the
		// user's SMART cadence. If a user sets SMART to 30d and
		// max-age to 7d, max-age becomes the governing cadence
		// (force-wake fires more frequently than stated SMART
		// interval) — the UI confirm() dialog warned them about
		// this on save.
		s.mu.RLock()
		maxAgeDays := s.smartMaxAgeDays
		s.mu.RUnlock()
		if maxAgeDays > 0 {
			staleChecker := NewStaleSMARTChecker(s.store, maxAgeDays, s.logger)
			if stale := staleChecker.Check(snap); len(stale) > 0 {
				staleChecker.Apply(snap, stale, s.collector.CollectSMARTForced)
			}
		}
	case "docker":
		docker, _ := s.collector.CollectDocker()
		snap.Docker = docker
		// Enrich top processes with container attribution (previously
		// done inline in Collect(); preserved here so the full-scan
		// snapshot still has enrichment when both Docker + system ran
		// on the same tick).
		if docker.Available && len(docker.Containers) > 0 && len(snap.System.TopProcesses) > 0 {
			collector.EnrichProcessContainers(snap.System.TopProcesses, docker.Containers, "")
		}
	case "proxmox":
		pve, _ := s.collector.CollectProxmox()
		if pve != nil {
			snap.Proxmox = pve
		}
	case "kubernetes":
		kube, _ := s.collector.CollectKubernetes()
		if kube != nil {
			snap.Kubernetes = kube
		}
	case "zfs":
		zfs, _ := s.collector.CollectZFS()
		if zfs != nil && zfs.Available {
			snap.ZFS = zfs
		}
	case "gpu":
		gpu := s.collector.CollectGPU()
		if gpu != nil && gpu.Available {
			snap.GPU = gpu
		}
	}
}

// carryForwardSubsystems copies prior-tick values from s.latest into
// the current snapshot for each skipped subsystem. Without this, a
// subsystem that didn't run this tick would show empty data — which
// is technically honest but degrades the dashboard experience
// dramatically. Snapshot-level timestamps (surfaced via ScanLastRan)
// tell consumers how stale each subsystem's data actually is.
func (s *Scheduler) carryForwardSubsystems(snap *internal.Snapshot, skipped []string) {
	s.mu.RLock()
	prev := s.latest
	s.mu.RUnlock()
	if prev == nil {
		return
	}
	for _, name := range skipped {
		switch name {
		case "smart":
			snap.SMART = prev.SMART
			snap.SMARTStandbyDevices = prev.SMARTStandbyDevices
		case "docker":
			snap.Docker = prev.Docker
		case "proxmox":
			snap.Proxmox = prev.Proxmox
		case "kubernetes":
			snap.Kubernetes = prev.Kubernetes
		case "zfs":
			snap.ZFS = prev.ZFS
		case "gpu":
			snap.GPU = prev.GPU
		}
	}
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
	s.pruneOrphanServiceCheckHistory(normalized)

	s.logger.Info("service check config updated", "checks", len(normalized))
}

// PurgeOrphanServiceCheckHistory removes history rows for any check_key NOT
// present in the currently configured service checks. Safe to call at any
// time — idempotent, and a no-op when the store has no orphans.
//
// This is a defense-in-depth API: normally UpdateServiceChecks handles the
// purge automatically as part of config updates. Callers that cannot
// guarantee UpdateServiceChecks has run (e.g. startup paths where the
// persisted settings failed to load) can invoke this directly to collapse
// any drift between in-memory config and the history table. Issue #181.
//
// Returns the number of rows deleted.
func (s *Scheduler) PurgeOrphanServiceCheckHistory() (int, error) {
	s.mu.RLock()
	checks := make([]internal.ServiceCheckConfig, len(s.serviceChecks))
	copy(checks, s.serviceChecks)
	s.mu.RUnlock()
	return s.pruneOrphanServiceCheckHistory(checks), nil
}

// pruneOrphanServiceCheckHistory is the shared implementation for the orphan
// purge. It derives keep-keys from the supplied (already-normalized) checks
// and delegates to the store. Returns the count of rows pruned.
func (s *Scheduler) pruneOrphanServiceCheckHistory(checks []internal.ServiceCheckConfig) int {
	if s.store == nil {
		return 0
	}
	keepKeys := make([]string, 0, len(checks))
	for _, c := range checks {
		keepKeys = append(keepKeys, CheckKey(c))
	}
	pruned, err := s.store.DeleteServiceChecksNotIn(keepKeys)
	if err != nil {
		s.logger.Warn("prune orphaned service check history", "error", err)
		return 0
	}
	if pruned > 0 {
		s.logger.Info("pruned orphaned service check history", "rows", pruned)
	}
	return pruned
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
//
// Logging semantics (issue #226): every cycle emits exactly one log line so
// operators can tell from grep alone whether the loop is still firing and
// why it degraded when container_stats_history stops advancing.
//
//   - Collector error          -> WARN  "container stats: collector returned error"
//   - Docker unavailable / nil -> WARN  "container stats: docker unavailable"
//   - Save error               -> WARN  "container stats: failed to save"
//   - Success                  -> INFO  "container stats: saved" with containers=N
//
// Previously all three failure modes collapsed into a silent return which
// masked 36+ hours of real-world data loss on a UAT host.
func (s *Scheduler) collectContainerStats() {
	s.mu.RLock()
	fn := s.dockerStatsFn
	s.mu.RUnlock()
	if fn == nil {
		fn = s.collector.CollectDockerStats
	}
	docker, err := fn()
	if err != nil {
		s.logger.Warn("container stats: collector returned error", "error", err)
		return
	}
	if docker == nil {
		s.logger.Warn("container stats: collector returned nil docker info")
		return
	}
	if !docker.Available {
		s.logger.Warn("container stats: docker unavailable")
		return
	}
	if err := s.store.SaveContainerStats(docker); err != nil {
		s.logger.Warn("container stats: failed to save", "error", err)
		return
	}
	s.logger.Info("container stats: saved", "containers", len(docker.Containers))
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

// runSpeedTest executes a network speed test and records the attempt
// state in the store.
//
// Records one of four outcomes on every invocation, except the disabled
// branch which is idempotent (first call writes status=disabled, later
// calls no-op so the tick loop doesn't churn a row every minute):
//
//   - disabled → interval == SpeedTestIntervalDisabled (#180): skip the
//     run entirely; record status=disabled once.
//   - pending → written BEFORE invoking the runner so the dashboard
//     widget and scheduled type=speed check can render the in-progress
//     state during the ~30-60 second window Ookla takes to complete.
//   - success → runner returned a result; saved to speedtest_history
//     and the attempt state flips to status=success.
//   - failed → runner returned nil (Ookla missing, network error,
//     zero-throughput parse failure per #170); attempt flips to
//     status=failed with a descriptive error message.
//
// Each branch persists the attempt to the store AND mirrors it onto
// s.latest.SpeedTest.LastAttempt so the dashboard widget can pick it
// up from the next /api/v1/snapshot/latest call without a separate
// DB round-trip. The cached-snapshot mirror is best-effort: if
// s.latest is still nil (first scan hasn't completed), the store
// copy is canonical and the snapshot will pick it up on rebuild.
//
// The Test button (handleTestServiceCheck) is unaffected — it builds
// its own ServiceChecker with a speed runner and does not route
// through this loop.
func (s *Scheduler) runSpeedTest() {
	// Disabled branch: check current state first; only write on transition.
	s.mu.RLock()
	interval := s.speedTestInterval
	runner := s.speedTestRunFn
	registry := s.liveTestRegistry
	s.mu.RUnlock()

	now := time.Now().UTC()

	if interval == SpeedTestIntervalDisabled {
		// Idempotent: if the last stored status is already "disabled",
		// don't write again. This matters because the 1-minute tick
		// loop may call this path many times and we don't want to
		// churn a row per tick.
		if existing, err := s.store.GetLastSpeedTestAttempt(); err == nil && existing != nil && existing.Status == "disabled" {
			return
		}
		s.logger.Info("speed test: disabled in settings, recording state")
		s.recordSpeedTestAttempt(now, "disabled", "")
		return
	}

	// Write pending state first so the widget + scheduled check see
	// "in progress" for the duration of the Ookla invocation.
	s.recordSpeedTestAttempt(now, "pending", "")

	s.logger.Info("running speed test")

	// Registry path (PRD #283 slice 2 / issue #285): route the test
	// through LiveTestRegistry so SSE subscribers can attach. This is
	// the production path. The registry's StartTest is idempotent: a
	// concurrent manual /api/v1/speedtest/run during this cron tick
	// will return the in-flight handle, NOT start a parallel test.
	if registry != nil {
		s.runSpeedTestViaRegistry(registry)
		return
	}

	// Legacy path: tests that haven't wired a registry use the
	// result-only shim. Preserved for backwards-compat with #180 +
	// #210 test suites that don't need the registry.
	var result *internal.SpeedTestResult
	if runner != nil {
		result = runner()
	}
	s.handleSpeedTestResult(result)
}

// runSpeedTestViaRegistry drives a speed test through the LiveTest
// registry, blocking until the test completes, then handles the
// terminal result the same way the legacy path does (write history
// row + flip LastSpeedTestAttempt to success/failed).
//
// The scheduler's invocation is the broadcast SOURCE; SSE handlers
// in the API layer are subscribers. If a manual /run request races
// this cron tick, the registry's idempotency guarantees both paths
// converge on the same in-flight test (one runner invocation).
//
// Sets the nasdoctor_speedtest_in_progress Prometheus gauge to 1
// while the test runs (PRD #283). The set+unset is wrapped in a
// defer so a panic in the runner still flips the gauge back to 0
// — otherwise alerting on stuck tests would emit false positives.
func (s *Scheduler) runSpeedTestViaRegistry(registry livetest.Registry) {
	if s.metrics != nil {
		s.metrics.SetSpeedTestInProgress(true)
		defer s.metrics.SetSpeedTestInProgress(false)
	}
	ctx := context.Background()
	lt, err := registry.StartTest(ctx)
	if err != nil {
		s.logger.Warn("speed test: registry start failed", "error", err)
		s.recordSpeedTestAttempt(time.Now().UTC(), "failed",
			fmt.Sprintf("registry start failed: %v", err))
		return
	}
	// Wait for the test to fully complete (samples drained,
	// subscribers fanned out, registry slot cleared). Don't drain
	// samples here — that's the SSE handler's job; we only care
	// about the terminal result.
	<-lt.Done()
	if err := lt.Err(); err != nil {
		s.logger.Info("speed test failed via registry", "error", err)
		s.recordSpeedTestAttempt(time.Now().UTC(), "failed",
			fmt.Sprintf("speed test failed: %v", err))
		return
	}
	s.handleSpeedTestResult(lt.Result())
}

// handleSpeedTestResult applies the post-run side effects shared by
// both the registry path and the legacy result-only path: log,
// persist history row, mirror state onto s.latest. Extracted to keep
// the two paths in sync.
func (s *Scheduler) handleSpeedTestResult(result *internal.SpeedTestResult) {
	if result == nil {
		s.logger.Info("speed test failed: no speedtest tool available or zero-throughput result")
		s.recordSpeedTestAttempt(time.Now().UTC(), "failed",
			"no speedtest tool available (install speedtest or speedtest-cli) or test returned zero throughput")
		return
	}
	s.logger.Info("speed test complete",
		"download", fmt.Sprintf("%.1f Mbps", result.DownloadMbps),
		"upload", fmt.Sprintf("%.1f Mbps", result.UploadMbps),
		"latency", fmt.Sprintf("%.1f ms", result.LatencyMs),
	)
	if err := s.store.SaveSpeedTest("speedtest-"+time.Now().Format("20060102-150405"), result); err != nil {
		s.logger.Warn("failed to save speed test result", "error", err)
	}
	s.recordSpeedTestSuccess(time.Now().UTC(), result)
}

// recordSpeedTestAttempt persists the attempt state to the store AND
// mirrors it onto s.latest.SpeedTest.LastAttempt (if the cached
// snapshot exists) so the dashboard widget sees the current state on
// the next /api/v1/snapshot/latest call. Preserves any existing
// s.latest.SpeedTest.Latest value — only overwrites LastAttempt.
//
// Does NOT write speedtest_history: that's the caller's responsibility
// (only the success branch of runSpeedTest writes a history row).
func (s *Scheduler) recordSpeedTestAttempt(ts time.Time, status, errorMsg string) {
	if err := s.store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: ts,
		Status:    status,
		ErrorMsg:  errorMsg,
	}); err != nil {
		s.logger.Warn("failed to save attempt state", "status", status, "error", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest == nil {
		return
	}
	if s.latest.SpeedTest == nil {
		s.latest.SpeedTest = &internal.SpeedTestInfo{}
	}
	s.latest.SpeedTest.LastAttempt = &internal.SpeedTestAttempt{
		Timestamp: ts,
		Status:    status,
		ErrorMsg:  errorMsg,
	}
}

// recordSpeedTestSuccess is the success-branch counterpart: writes the
// attempt row AND updates s.latest.SpeedTest.{Latest,LastAttempt,
// Available} atomically so the widget sees Latest + LastAttempt
// transition together.
func (s *Scheduler) recordSpeedTestSuccess(ts time.Time, result *internal.SpeedTestResult) {
	if err := s.store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: ts,
		Status:    "success",
	}); err != nil {
		s.logger.Warn("failed to save success attempt state", "error", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest == nil {
		return
	}
	s.latest.SpeedTest = &internal.SpeedTestInfo{
		Available: true,
		Latest:    result,
		LastAttempt: &internal.SpeedTestAttempt{
			Timestamp: ts,
			Status:    "success",
		},
	}
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
