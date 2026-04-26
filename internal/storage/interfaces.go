// Package storage — interfaces.go defines the Store interface hierarchy.
//
// These interfaces are extracted from the 52 exported methods on *DB.
// Every consumer (api.Server, scheduler.Scheduler, etc.) should depend
// on the narrowest interface it needs, or on the composed Store when
// it needs everything.
package storage

import (
	"log/slog"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// SnapshotStore handles snapshot persistence and retrieval.
type SnapshotStore interface {
	SaveSnapshot(snap *internal.Snapshot) error
	GetLatestSnapshot() (*internal.Snapshot, error)
	GetSnapshot(id string) (*internal.Snapshot, error)
	ListSnapshots(limit int) ([]SnapshotSummary, error)
}

// AlertStore handles alert lifecycle (open, ack, snooze, resolve).
type AlertStore interface {
	SyncAlertStates(snapshotID string, findings []AlertStateFinding, now time.Time) error
	ListAlerts(status string, limit int, now time.Time) ([]AlertRecord, error)
	GetAlert(id int64, now time.Time) (*AlertRecord, error)
	GetAlertEvents(alertID int64, limit int) ([]AlertEvent, error)
	AcknowledgeAlert(id int64, actor string, now time.Time) (bool, error)
	UnacknowledgeAlert(id int64, actor string, now time.Time) (bool, error)
	SnoozeAlert(id int64, until time.Time, actor string, now time.Time) (bool, error)
	UnsnoozeAlert(id int64, actor string, now time.Time) (bool, error)
	IsAlertSuppressed(fingerprint string, now time.Time) (bool, string, error)
	MarkAlertsNotifiedByFingerprint(fingerprints []string, notifiedAt time.Time) error
}

// ServiceCheckStore handles service check results and history.
//
// As of issue #210 the scheduled type=speed service check dispatch reads
// the shared LastSpeedTestAttempt + latest speedtest_history row rather
// than running Ookla per-check, so the ServiceChecker needs access to
// those two methods as well. They live on the broader HistoryStore but
// are surfaced here so the narrow dependency remains a single interface.
type ServiceCheckStore interface {
	SaveServiceCheckResults(results []internal.ServiceCheckResult) error
	GetLatestServiceCheckState(checkKey string) (ServiceCheckState, bool, error)
	ListLatestServiceChecks(limit int) ([]ServiceCheckEntry, error)
	GetServiceCheckHistory(checkKey string, limit int) ([]ServiceCheckEntry, error)
	PruneServiceCheckHistory(olderThan time.Duration) (int, error)
	DeleteServiceCheckByKey(key string) (int, error)
	// DeleteServiceChecksNotIn removes every row from service_checks_history
	// whose check_key is NOT in keepKeys. Passing nil or an empty slice
	// deletes all rows. Returns the number of rows deleted.
	DeleteServiceChecksNotIn(keepKeys []string) (int, error)

	// Speed-check dispatch (issue #210) reads attempt state + history.
	GetLastSpeedTestAttempt() (*LastSpeedTestAttempt, error)
	GetSpeedTestHistory(hours int) ([]SpeedTestHistoryPoint, error)
}

// HistoryStore handles time-series history for disks, system, GPU, containers, and speed tests.
type HistoryStore interface {
	GetDiskHistory(serial string, limit int) ([]DiskHistoryPoint, error)
	GetDiskHistoryInRange(serial string, window time.Duration) ([]DiskHistoryPoint, error)
	// GetLastSMARTCollectedAt returns the latest smart_history.timestamp
	// for the given device and a found flag. Used by the scheduler's
	// StaleSMARTChecker (issue #238) to decide whether to force-wake a
	// drive that has been in standby longer than Settings.SMART.MaxAgeDays.
	GetLastSMARTCollectedAt(device string) (time.Time, bool, error)
	GetAvgTempDuringRange(start, end time.Time) (float64, float64, error)
	ListDisks() ([]DiskSummary, error)
	GetAllDiskSparklines(pointsPerDisk int) ([]DiskSparklines, error)
	GetAverageDiskTemperatureRange(start, end time.Time, maxPoints int) ([]NumericPoint, error)
	GetDiskUsageHistory(limit int) ([]DiskUsageSeries, error)
	GetSystemHistory(limit int) ([]SystemHistoryPoint, error)
	GetSystemHistoryRange(start, end time.Time, maxPoints int) ([]SystemHistoryPoint, error)
	GetSystemSparkline(limit int) ([]SystemHistoryPoint, error)
	GetGPUHistory(hours int) ([]GPUHistoryPoint, error)
	SaveContainerStats(docker *internal.DockerInfo) error
	GetContainerHistory(hours int) ([]ContainerHistoryPoint, error)
	SaveProcessStats(procs []internal.ProcessInfo) error
	SaveProcessStatsAt(procs []internal.ProcessInfo, ts time.Time) error
	GetProcessHistory(hours int) ([]ProcessHistoryPoint, error)
	SaveSpeedTest(snapshotID string, result *internal.SpeedTestResult) error
	// SaveSpeedTestReturningID is the ID-returning variant added in
	// PRD #283 slice 3 / issue #286 so the scheduler can wire the new
	// history row to per-sample bulk-insert.
	SaveSpeedTestReturningID(snapshotID string, result *internal.SpeedTestResult) (int64, error)
	GetSpeedTestHistory(hours int) ([]SpeedTestHistoryPoint, error)
	GetLatestSpeedTestHistoryID() (int64, bool, error)
	// Issue #210 — last attempt state (single-row table).
	SaveSpeedTestAttempt(att LastSpeedTestAttempt) error
	GetLastSpeedTestAttempt() (*LastSpeedTestAttempt, error)
	// PRD #283 slice 3 / issue #286 — per-sample telemetry.
	InsertSpeedTestSamples(testID int64, samples []SpeedTestSample) error
	GetSpeedTestSamples(testID int64) ([]SpeedTestSample, error)
}

// ConfigStore handles key/value configuration persistence.
type ConfigStore interface {
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
}

// NotificationStore handles notification delivery tracking and deduplication.
type NotificationStore interface {
	CanSendNotification(fingerprint, routeKey string, cooldown time.Duration, now time.Time) (bool, error)
	SaveNotificationState(fingerprint, routeKey, status string, sentAt time.Time) error
	SaveNotificationLog(name, webhookType, status string, findingsCount int, errMsg string) error
	GetNotificationLog(limit int) ([]NotificationLogEntry, error)
	GetNotificationLogRange(start, end time.Time, limit int) ([]NotificationLogEntry, error)
}

// FindingStore handles finding history queries.
type FindingStore interface {
	GetFindingHistory(category string, limit int) ([]internal.Finding, error)
}

// LifecycleStore handles data lifecycle operations (pruning, backup, stats).
type LifecycleStore interface {
	PruneSnapshots(olderThan time.Duration, keepMin int) (int, error)
	PruneNotificationLog(olderThan time.Duration) (int, error)
	PruneAlerts(olderThan time.Duration) (int, error)
	PruneOrphanedFindings() (int, error)
	PruneDiskUsageHistory(cutoff time.Time) (int64, error)
	PruneToSizeMB(targetMB float64) (int, error)
	Vacuum() error
	GetDBStats() (*DBStats, error)
	CreateBackup(backupDir string, logger *slog.Logger) (*BackupResult, error)
	Close() error
	DataDir() string
}

// DriveEventStore handles per-drive maintenance log events (issue #130).
// Events come in two flavours:
//   - manual "note" (user-entered; mutable)
//   - auto   "replacement" (system-detected on serial change; immutable)
//
// The SlotKey is the Unraid ArraySlot when available, else the drive serial.
type DriveEventStore interface {
	SaveDriveEvent(ev DriveEvent) (int64, error)
	ListDriveEvents(slotKey string) ([]DriveEvent, error)
	UpdateDriveEvent(slotKey string, id int64, eventTime *time.Time, content *string) error
	DeleteDriveEvent(slotKey string, id int64) error
	GetDriveEvent(id int64) (*DriveEvent, error)

	// Slot state tracking for replacement detection (issue #130).
	GetDriveSlotState(slotKey string) (*DriveSlotState, error)
	SaveDriveSlotState(state DriveSlotState) error
}

// Store composes all domain-specific interfaces into a single aggregate.
// Use the narrower interfaces when possible; use Store when a consumer
// genuinely needs access to multiple domains.
type Store interface {
	SnapshotStore
	AlertStore
	ServiceCheckStore
	HistoryStore
	ConfigStore
	NotificationStore
	FindingStore
	LifecycleStore
	DriveEventStore
}

// Compile-time checks: *DB must satisfy Store.
var _ Store = (*DB)(nil)
