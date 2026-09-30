package api

import (
	"time"

	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/logfwd"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
)

// ApplyRuntimeSettings pushes a resolved Settings blob into the running
// scheduler and collector. It is the single translation point from a
// persisted/edited Settings object to live runtime state, shared by the
// settings-save handler (handleUpdateSettings) and process startup
// (cmd/nas-doctor). Before this existed the two paths were maintained
// separately and drifted: speed-test cadence, notification routing rules,
// and log forwarding were applied on save but NOT re-applied at boot, so
// every restart silently reverted them until the user saved again
// (issue #333, plus the rules-dropped and dead-log-forwarding regressions
// that had no tracking issue).
//
// It deliberately does NOT own three things, because each caller constructs
// them differently and must keep that ownership:
//   - the global scan interval — scheduler.New at boot vs UpdateInterval on save
//   - the notifier — built into scheduler.New at boot vs UpdateNotifier on save
//   - the service-check list — boot sequences it with the orphan-history purge
//
// globalScanInterval is the already-resolved global cadence, passed in
// rather than re-parsed from settings.ScanInterval so the ScanDispatcher's
// "use global" subsystems see the value the caller actually intends (at boot
// settings.ScanInterval may be blank while the effective interval comes from
// the flag/env).
//
// coll may be nil (a scheduler-only caller); the collector integration
// configs are then skipped.
func ApplyRuntimeSettings(sched *scheduler.Scheduler, coll *collector.Collector, settings Settings, globalScanInterval time.Duration) {
	if sched == nil {
		return
	}

	// ── Speed-test cadence (issue #333) ──
	// parseSpeedTestInterval handles real durations ("4h", "30m") and the
	// "disabled" sentinel; keyword cadences ("weekly"/"monthly") are rejected
	// there and flow through SetSpeedTestSchedule instead. The schedule call
	// runs unconditionally so an empty schedule (interval mode) is applied as
	// such rather than leaving a stale scheduled list in place.
	if d, ok := parseSpeedTestInterval(settings.SpeedTestInterval); ok {
		sched.SetSpeedTestInterval(d)
	}
	sched.SetSpeedTestSchedule(settings.SpeedTestSchedule, settings.SpeedTestDay, settings.SpeedTestInterval)

	// ── Retention (clamped to the same floors the save handler applies
	// before persisting, so a raw blob at boot yields identical config) ──
	retention := scheduler.RetentionConfig{
		SnapshotDays:  settings.Retention.SnapshotDays,
		MaxDBSizeMB:   settings.Retention.MaxDBSizeMB,
		NotifyLogDays: settings.Retention.NotifyLogDays,
	}
	if retention.SnapshotDays < 7 {
		retention.SnapshotDays = 90
	}
	if retention.MaxDBSizeMB < 50 {
		retention.MaxDBSizeMB = 500
	}
	if retention.NotifyLogDays < 1 {
		retention.NotifyLogDays = 30
	}
	sched.UpdateRetention(retention)

	// ── Backup ──
	keepCount := settings.Backup.KeepCount
	if keepCount <= 0 {
		keepCount = 4
	}
	intervalH := settings.Backup.IntervalH
	if intervalH <= 0 {
		intervalH = 168
	}
	sched.UpdateBackup(scheduler.BackupConfig{
		Enabled:   settings.Backup.Enabled,
		Path:      settings.Backup.Path,
		KeepCount: keepCount,
		IntervalH: intervalH,
	})

	// ── Alerting (routing rules INCLUDED) ──
	// UpdateAlerting clamps DefaultCooldownSec and normalises nil slices, so
	// the raw values are passed straight through as the save handler does.
	sched.UpdateAlerting(scheduler.AlertingConfig{
		Rules:              settings.Notifications.Rules,
		Policies:           settings.Notifications.Policies, // legacy compat
		QuietHours:         settings.Notifications.QuietHours,
		MaintenanceWindows: settings.Notifications.MaintenanceWindows,
		DefaultCooldownSec: settings.Notifications.DefaultCooldownSec,
	})

	// ── SMART max-age safety net + per-subsystem scan cadences ──
	sched.SetSMARTMaxAgeDays(settings.AdvancedScans.SMART.MaxAgeDays)
	sched.SetDispatcherIntervals(scheduler.DispatcherIntervalsConfig{
		SMARTSec:      settings.AdvancedScans.SMART.IntervalSec,
		DockerSec:     settings.AdvancedScans.Docker.IntervalSec,
		ProxmoxSec:    settings.AdvancedScans.Proxmox.IntervalSec,
		KubernetesSec: settings.AdvancedScans.Kubernetes.IntervalSec,
		ZFSSec:        settings.AdvancedScans.ZFS.IntervalSec,
		GPUSec:        settings.AdvancedScans.GPU.IntervalSec,
	}, globalScanInterval)

	// ── Log forwarding ──
	sched.UpdateLogForwarder(logForwardDestinations(settings.LogPush))

	// ── Collector integration configs ──
	if coll == nil {
		return
	}
	coll.SetProxmoxConfig(collector.ProxmoxConfig{
		Enabled:  settings.Proxmox.Enabled,
		URL:      settings.Proxmox.URL,
		TokenID:  settings.Proxmox.TokenID,
		Secret:   settings.Proxmox.Secret,
		NodeName: settings.Proxmox.NodeName,
		Alias:    settings.Proxmox.Alias,
	})
	coll.SetKubeConfig(collector.KubeConfig{
		Enabled:   settings.Kubernetes.Enabled,
		URL:       settings.Kubernetes.URL,
		Token:     settings.Kubernetes.Token,
		Alias:     settings.Kubernetes.Alias,
		InCluster: settings.Kubernetes.InCluster,
	})
	coll.SetSMARTConfig(collector.SMARTConfig{
		WakeDrives: settings.AdvancedScans.SMART.WakeDrives,
	})
	coll.SetBackupMonitorBorg(apiBorgReposToCollector(settings.BackupMonitor.Borg))
	coll.SetBackupMonitorDuplicacy(apiDuplicacyEntriesToCollector(settings.BackupMonitor.Duplicacy))
}

// logForwardDestinations converts the API-layer log-forward settings into the
// logfwd package's destination slice, returning nil when forwarding is
// disabled or no destinations are configured (nil clears the forwarder).
// Shared by ApplyRuntimeSettings and the save handler so the two produce
// identical wiring.
func logForwardDestinations(cfg SettingsLogForward) []logfwd.Destination {
	if !cfg.Enabled || len(cfg.Destinations) == 0 {
		return nil
	}
	dests := make([]logfwd.Destination, 0, len(cfg.Destinations))
	for _, d := range cfg.Destinations {
		dests = append(dests, logfwd.Destination{
			Name:    d.Name,
			Type:    d.Type,
			URL:     d.URL,
			Enabled: d.Enabled,
			Headers: d.Headers,
			Labels:  d.Labels,
			Format:  d.Format,
		})
	}
	return dests
}
