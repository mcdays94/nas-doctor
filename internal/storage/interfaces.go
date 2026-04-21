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
}

// HistoryStore handles time-series history for disks, system, GPU, containers, and speed tests.
type HistoryStore interface {
	GetDiskHistory(serial string, limit int) ([]DiskHistoryPoint, error)
	GetDiskHistoryInRange(serial string, window time.Duration) ([]DiskHistoryPoint, error)
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
	GetSpeedTestHistory(hours int) ([]SpeedTestHistoryPoint, error)
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
}

// Compile-time checks: *DB must satisfy Store.
var _ Store = (*DB)(nil)
