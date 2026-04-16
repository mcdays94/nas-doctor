package storage

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// FakeStore is an in-memory Store implementation for testing.
// It provides correct behavior for the most commonly tested methods;
// less-used methods return zero values and are marked with TODO comments.
type FakeStore struct {
	mu sync.RWMutex

	// Snapshots keyed by ID, ordered slice for listing.
	snapshots []*internal.Snapshot

	// Service check history, newest first per key.
	serviceChecks []internal.ServiceCheckResult

	// Config key/value pairs.
	config map[string]string

	// Notification log entries.
	notificationLog []NotificationLogEntry

	// Notification dedup state: "fingerprint|routeKey" → sentAt.
	notificationState map[string]time.Time

	// Alert state.
	alerts      []AlertRecord
	alertSeq    int64
	alertEvents []AlertEvent

	// Alert suppression: fingerprint → reason (e.g., "acknowledged", "snoozed").
	suppressedAlerts map[string]string

	// Process history.
	processHistory []ProcessHistoryPoint

	// Finding history keyed by category.
	findingsByCategory map[string][]internal.Finding

	// LifecycleStore test hooks.
	VacuumCalled bool    // observable by tests
	DBSizeMB     float64 // simulated DB file size for GetDBStats/PruneToSizeMB
	BackupCalls  int     // number of CreateBackup invocations

	// Orphaned findings: findings whose snapshot ID doesn't match any snapshot.
	// Seeded by tests via AddOrphanedFindings().
	orphanedFindingCount int
}

// NewFakeStore creates a ready-to-use in-memory store.
func NewFakeStore() *FakeStore {
	return &FakeStore{
		config:             make(map[string]string),
		notificationState:  make(map[string]time.Time),
		findingsByCategory: make(map[string][]internal.Finding),
	}
}

// Compile-time check: FakeStore must satisfy Store.
var _ Store = (*FakeStore)(nil)

// ── SnapshotStore ──

func (f *FakeStore) SaveSnapshot(snap *internal.Snapshot) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Store a shallow copy to avoid mutation.
	cp := *snap
	f.snapshots = append(f.snapshots, &cp)
	return nil
}

func (f *FakeStore) GetLatestSnapshot() (*internal.Snapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.snapshots) == 0 {
		return nil, nil
	}
	// Return the most recent by timestamp.
	latest := f.snapshots[0]
	for _, s := range f.snapshots[1:] {
		if s.Timestamp.After(latest.Timestamp) {
			latest = s
		}
	}
	return latest, nil
}

func (f *FakeStore) GetSnapshot(id string) (*internal.Snapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, s := range f.snapshots {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (f *FakeStore) ListSnapshots(limit int) ([]SnapshotSummary, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// Sort by timestamp descending.
	sorted := make([]*internal.Snapshot, len(f.snapshots))
	copy(sorted, f.snapshots)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})
	if limit > 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}
	var out []SnapshotSummary
	for _, s := range sorted {
		summary := SnapshotSummary{
			ID:        s.ID,
			Timestamp: s.Timestamp,
			Duration:  s.Duration,
		}
		for _, f := range s.Findings {
			switch f.Severity {
			case "critical":
				summary.CriticalCount++
			case "warning":
				summary.WarningCount++
			case "info":
				summary.InfoCount++
			}
		}
		out = append(out, summary)
	}
	return out, nil
}

// ── AlertStore ──

func (f *FakeStore) SyncAlertStates(_ string, _ []AlertStateFinding, _ time.Time) error {
	// TODO: implement alert sync for testing
	return nil
}

func (f *FakeStore) ListAlerts(_ string, _ int, _ time.Time) ([]AlertRecord, error) {
	// TODO: implement alert listing for testing
	return nil, nil
}

func (f *FakeStore) GetAlert(_ int64, _ time.Time) (*AlertRecord, error) {
	// TODO: implement alert retrieval for testing
	return nil, nil
}

func (f *FakeStore) GetAlertEvents(_ int64, _ int) ([]AlertEvent, error) {
	// TODO: implement alert events for testing
	return nil, nil
}

func (f *FakeStore) AcknowledgeAlert(_ int64, _ string, _ time.Time) (bool, error) {
	// TODO: implement for testing
	return false, nil
}

func (f *FakeStore) UnacknowledgeAlert(_ int64, _ string, _ time.Time) (bool, error) {
	// TODO: implement for testing
	return false, nil
}

func (f *FakeStore) SnoozeAlert(_ int64, _ time.Time, _ string, _ time.Time) (bool, error) {
	// TODO: implement for testing
	return false, nil
}

func (f *FakeStore) UnsnoozeAlert(_ int64, _ string, _ time.Time) (bool, error) {
	// TODO: implement for testing
	return false, nil
}

func (f *FakeStore) IsAlertSuppressed(fingerprint string, _ time.Time) (bool, string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.suppressedAlerts != nil {
		if reason, ok := f.suppressedAlerts[fingerprint]; ok {
			return true, reason, nil
		}
	}
	return false, "", nil
}

// SuppressAlert marks a fingerprint as suppressed (acknowledged/snoozed) for testing.
func (f *FakeStore) SuppressAlert(fingerprint, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.suppressedAlerts == nil {
		f.suppressedAlerts = make(map[string]string)
	}
	f.suppressedAlerts[fingerprint] = reason
}

func (f *FakeStore) MarkAlertsNotifiedByFingerprint(_ []string, _ time.Time) error {
	return nil
}

// ── ServiceCheckStore ──

func (f *FakeStore) SaveServiceCheckResults(results []internal.ServiceCheckResult) error {
	if len(results) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.serviceChecks = append(f.serviceChecks, results...)
	return nil
}

func (f *FakeStore) GetLatestServiceCheckState(checkKey string) (ServiceCheckState, bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// Walk backwards to find the most recent entry for this key.
	for i := len(f.serviceChecks) - 1; i >= 0; i-- {
		r := f.serviceChecks[i]
		if r.Key == checkKey {
			checkedAt, _ := time.Parse(time.RFC3339, r.CheckedAt)
			return ServiceCheckState{
				Status:              r.Status,
				ConsecutiveFailures: r.ConsecutiveFailures,
				CheckedAt:           checkedAt,
			}, true, nil
		}
	}
	return ServiceCheckState{}, false, nil
}

func (f *FakeStore) ListLatestServiceChecks(limit int) ([]ServiceCheckEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// Collect the latest result per key.
	latest := make(map[string]internal.ServiceCheckResult)
	for _, r := range f.serviceChecks {
		if _, exists := latest[r.Key]; !exists {
			latest[r.Key] = r
		} else {
			// Keep the one with the later CheckedAt.
			existing := latest[r.Key]
			if r.CheckedAt > existing.CheckedAt {
				latest[r.Key] = r
			}
		}
	}
	var entries []ServiceCheckEntry
	for _, r := range latest {
		entries = append(entries, ServiceCheckEntry{
			Key:                 r.Key,
			Name:                r.Name,
			Type:                r.Type,
			Target:              r.Target,
			Status:              r.Status,
			ResponseMS:          r.ResponseMS,
			Error:               r.Error,
			ConsecutiveFailures: r.ConsecutiveFailures,
			FailureThreshold:    r.FailureThreshold,
			FailureSeverity:     string(r.FailureSeverity),
			CheckedAt:           r.CheckedAt,
		})
	}
	// Sort by CheckedAt descending.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CheckedAt > entries[j].CheckedAt
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (f *FakeStore) GetServiceCheckHistory(checkKey string, limit int) ([]ServiceCheckEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var entries []ServiceCheckEntry
	for i := len(f.serviceChecks) - 1; i >= 0; i-- {
		r := f.serviceChecks[i]
		if r.Key != checkKey {
			continue
		}
		entries = append(entries, ServiceCheckEntry{
			Key:                 r.Key,
			Name:                r.Name,
			Type:                r.Type,
			Target:              r.Target,
			Status:              r.Status,
			ResponseMS:          r.ResponseMS,
			Error:               r.Error,
			ConsecutiveFailures: r.ConsecutiveFailures,
			FailureThreshold:    r.FailureThreshold,
			FailureSeverity:     string(r.FailureSeverity),
			CheckedAt:           r.CheckedAt,
		})
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

// PruneServiceCheckHistory removes service check entries older than olderThan.
func (f *FakeStore) PruneServiceCheckHistory(olderThan time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var kept []internal.ServiceCheckResult
	pruned := 0
	for _, r := range f.serviceChecks {
		checkedAt, err := time.Parse(time.RFC3339, r.CheckedAt)
		if err != nil {
			kept = append(kept, r)
			continue
		}
		if checkedAt.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, r)
		}
	}
	f.serviceChecks = kept
	return pruned, nil
}

func (f *FakeStore) DeleteServiceCheckByKey(key string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var kept []internal.ServiceCheckResult
	deleted := 0
	for _, r := range f.serviceChecks {
		if r.Key == key {
			deleted++
		} else {
			kept = append(kept, r)
		}
	}
	f.serviceChecks = kept
	return deleted, nil
}

// ── HistoryStore ──

func (f *FakeStore) GetDiskHistory(_ string, _ int) ([]DiskHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetAvgTempDuringRange(_, _ time.Time) (float64, float64, error) {
	// TODO: implement for testing
	return 0, 0, nil
}

func (f *FakeStore) ListDisks() ([]DiskSummary, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetAllDiskSparklines(_ int) ([]DiskSparklines, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetAverageDiskTemperatureRange(_, _ time.Time, _ int) ([]NumericPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetDiskUsageHistory(_ int) ([]DiskUsageSeries, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetSystemHistory(_ int) ([]SystemHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetSystemHistoryRange(_, _ time.Time, _ int) ([]SystemHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetSystemSparkline(_ int) ([]SystemHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) GetGPUHistory(_ int) ([]GPUHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) SaveContainerStats(_ *internal.DockerInfo) error {
	// TODO: implement for testing
	return nil
}

func (f *FakeStore) GetContainerHistory(_ int) ([]ContainerHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

func (f *FakeStore) SaveProcessStats(procs []internal.ProcessInfo) error {
	if len(procs) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, p := range procs {
		name := extractProcessName(p.Command)
		if name == "" {
			continue
		}
		f.processHistory = append(f.processHistory, ProcessHistoryPoint{
			Timestamp: now,
			PID:       p.PID,
			User:      p.User,
			Name:      name,
			Command:   p.Command,
			CPUPct:    p.CPU,
			MemPct:    p.Mem,
		})
	}
	return nil
}

func (f *FakeStore) GetProcessHistory(_ int) ([]ProcessHistoryPoint, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// Return a copy sorted by name ASC, container_name ASC, timestamp ASC.
	out := make([]ProcessHistoryPoint, len(f.processHistory))
	copy(out, f.processHistory)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].ContainerName != out[j].ContainerName {
			return out[i].ContainerName < out[j].ContainerName
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out, nil
}

// extractProcessName extracts a short name from a command string (same logic as processName in db.go).
func extractProcessName(command string) string {
	if command == "" {
		return ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	exe := fields[0]
	if idx := strings.LastIndex(exe, "/"); idx >= 0 && idx < len(exe)-1 {
		exe = exe[idx+1:]
	}
	return exe
}

func (f *FakeStore) SaveSpeedTest(_ string, _ *internal.SpeedTestResult) error {
	// TODO: implement for testing
	return nil
}

func (f *FakeStore) GetSpeedTestHistory(_ int) ([]SpeedTestHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

// ── ConfigStore ──

func (f *FakeStore) GetConfig(key string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.config[key]
	if !ok {
		return "", fmt.Errorf("config key %q not found", key)
	}
	return v, nil
}

func (f *FakeStore) SetConfig(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config[key] = value
	return nil
}

// ── NotificationStore ──

func (f *FakeStore) CanSendNotification(fingerprint, routeKey string, cooldown time.Duration, now time.Time) (bool, error) {
	if fingerprint == "" || routeKey == "" || cooldown <= 0 {
		return true, nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	key := fingerprint + "|" + routeKey
	lastSent, ok := f.notificationState[key]
	if !ok {
		return true, nil
	}
	return now.Sub(lastSent) >= cooldown, nil
}

func (f *FakeStore) SaveNotificationState(fingerprint, routeKey, _ string, sentAt time.Time) error {
	if fingerprint == "" || routeKey == "" {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fingerprint + "|" + routeKey
	f.notificationState[key] = sentAt
	return nil
}

func (f *FakeStore) SaveNotificationLog(name, webhookType, status string, findingsCount int, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notificationLog = append(f.notificationLog, NotificationLogEntry{
		ID:            len(f.notificationLog) + 1,
		WebhookName:   name,
		WebhookType:   webhookType,
		Status:        status,
		FindingsCount: findingsCount,
		ErrorMessage:  errMsg,
		CreatedAt:     time.Now(),
	})
	return nil
}

func (f *FakeStore) GetNotificationLog(limit int) ([]NotificationLogEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.notificationLog) == 0 {
		return nil, nil
	}
	// Return newest first.
	out := make([]NotificationLogEntry, len(f.notificationLog))
	copy(out, f.notificationLog)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *FakeStore) GetNotificationLogRange(_, _ time.Time, _ int) ([]NotificationLogEntry, error) {
	// TODO: implement for testing
	return nil, nil
}

// ── FindingStore ──

func (f *FakeStore) GetFindingHistory(category string, limit int) ([]internal.Finding, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	findings := f.findingsByCategory[category]
	if limit > 0 && len(findings) > limit {
		findings = findings[:limit]
	}
	return findings, nil
}

// ── LifecycleStore ──

// PruneSnapshots removes snapshots older than olderThan, but always keeps at
// least keepMin snapshots (the most recent ones). Returns the count removed.
func (f *FakeStore) PruneSnapshots(olderThan time.Duration, keepMin int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapshots) == 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-olderThan)
	sorted := make([]*internal.Snapshot, len(f.snapshots))
	copy(sorted, f.snapshots)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})
	keep := make(map[string]bool)
	for i := 0; i < keepMin && i < len(sorted); i++ {
		keep[sorted[i].ID] = true
	}
	var kept []*internal.Snapshot
	pruned := 0
	for _, s := range f.snapshots {
		if !keep[s.ID] && s.Timestamp.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, s)
		}
	}
	f.snapshots = kept
	return pruned, nil
}

// PruneNotificationLog removes notification log entries older than olderThan.
func (f *FakeStore) PruneNotificationLog(olderThan time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	var kept []NotificationLogEntry
	pruned := 0
	for _, entry := range f.notificationLog {
		if entry.CreatedAt.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, entry)
		}
	}
	f.notificationLog = kept
	return pruned, nil
}

// PruneAlerts removes resolved alerts older than olderThan.
func (f *FakeStore) PruneAlerts(olderThan time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	var kept []AlertRecord
	pruned := 0
	for _, a := range f.alerts {
		if a.Status == "resolved" && a.ResolvedAt != "" {
			resolvedAt, err := time.Parse(time.RFC3339, a.ResolvedAt)
			if err == nil && resolvedAt.Before(cutoff) {
				pruned++
				continue
			}
		}
		kept = append(kept, a)
	}
	f.alerts = kept
	return pruned, nil
}

// PruneOrphanedFindings removes orphaned findings and returns the count.
func (f *FakeStore) PruneOrphanedFindings() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.orphanedFindingCount
	f.orphanedFindingCount = 0
	return n, nil
}

// PruneToSizeMB removes the oldest snapshots until the simulated DB size is
// at or below targetMB.
func (f *FakeStore) PruneToSizeMB(targetMB float64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DBSizeMB <= targetMB || len(f.snapshots) == 0 {
		return 0, nil
	}
	sort.Slice(f.snapshots, func(i, j int) bool {
		return f.snapshots[i].Timestamp.Before(f.snapshots[j].Timestamp)
	})
	perSnapshot := f.DBSizeMB / float64(len(f.snapshots))
	pruned := 0
	for f.DBSizeMB > targetMB && len(f.snapshots) > 0 {
		f.snapshots = f.snapshots[1:]
		f.DBSizeMB -= perSnapshot
		pruned++
	}
	return pruned, nil
}

// Vacuum records that vacuum was called.
func (f *FakeStore) Vacuum() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.VacuumCalled = true
	return nil
}

// GetDBStats returns statistics about the in-memory store contents.
func (f *FakeStore) GetDBStats() (*DBStats, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return &DBStats{
		FileSizeMB:       f.DBSizeMB,
		SnapshotCount:    len(f.snapshots),
		ServiceCheckRows: len(f.serviceChecks),
		NotifyLogRows:    len(f.notificationLog),
		AlertRows:        len(f.alerts),
	}, nil
}

// CreateBackup records the call and returns a fake result.
func (f *FakeStore) CreateBackup(_ string, _ *slog.Logger) (*BackupResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BackupCalls++
	return &BackupResult{
		Path:      "/tmp/fake-backup.db.gz",
		SizeMB:    0.1,
		Timestamp: time.Now(),
	}, nil
}

func (f *FakeStore) Close() error {
	return nil
}

func (f *FakeStore) DataDir() string {
	return "/tmp/fake-store"
}

// ── Test helpers (not part of Store interface) ──

// AddOrphanedFindings seeds orphaned finding count for PruneOrphanedFindings.
func (f *FakeStore) AddOrphanedFindings(count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orphanedFindingCount += count
}

// AddAlert adds an alert record for testing.
func (f *FakeStore) AddAlert(a AlertRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alerts = append(f.alerts, a)
}

// AddNotificationLogEntry adds a notification log entry with a specific timestamp.
func (f *FakeStore) AddNotificationLogEntry(entry NotificationLogEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notificationLog = append(f.notificationLog, entry)
}

// SnapshotCount returns the current number of snapshots.
func (f *FakeStore) SnapshotCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.snapshots)
}

// AlertCount returns the current number of alerts.
func (f *FakeStore) AlertCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.alerts)
}

// NotificationLogCount returns the current number of notification log entries.
func (f *FakeStore) NotificationLogCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.notificationLog)
}

// ServiceCheckCount returns the current number of service check entries.
func (f *FakeStore) ServiceCheckCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.serviceChecks)
}

// ResetVacuum resets the VacuumCalled flag.
func (f *FakeStore) ResetVacuum() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.VacuumCalled = false
}
