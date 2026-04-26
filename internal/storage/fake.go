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

	// Disk usage history rows (snapshot-independent; keyed by timestamp).
	// Seeded via AddDiskUsageHistoryEntry() and pruned by PruneDiskUsageHistory().
	diskUsageHistory []diskUsageRow

	// Speed-test history (issue #210 — consumed by type=speed service
	// check via option B read-from-history dispatch).
	speedTestHistory []SpeedTestHistoryPoint

	// LastSpeedTestAttempt state (single-row, issue #210). nil until
	// the scheduler writes the first attempt outcome.
	speedTestAttempt *LastSpeedTestAttempt

	// Per-test sample buffer keyed by test_id (PRD #283 slice 3 /
	// issue #286). Mirrors the FK + cascade-delete shape of the
	// real DB so deleting a history row drops samples too.
	speedTestSamples map[int64][]SpeedTestSample
	// speedTestNextID is the synthetic auto-increment counter for
	// SaveSpeedTestReturningID. Starts at 1 so a zero ID is always
	// "no row".
	speedTestNextID int64

	// Drive maintenance events (issue #130).
	driveEvents     []DriveEvent
	driveEventSeq   int64
	driveSlotStates map[string]DriveSlotState
}

// diskUsageRow is the minimal fake representation of a disk_usage_history row.
type diskUsageRow struct {
	MountPoint string
	Timestamp  time.Time
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
			Details:             cloneDetails(r.Details),
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
			Details:             cloneDetails(r.Details),
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

// DeleteServiceChecksNotIn removes every history row whose key is NOT in
// keepKeys. A nil/empty keepKeys deletes all rows.
func (f *FakeStore) DeleteServiceChecksNotIn(keepKeys []string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(keepKeys) == 0 {
		deleted := len(f.serviceChecks)
		f.serviceChecks = nil
		return deleted, nil
	}

	keep := make(map[string]struct{}, len(keepKeys))
	for _, k := range keepKeys {
		keep[k] = struct{}{}
	}

	var kept []internal.ServiceCheckResult
	deleted := 0
	for _, r := range f.serviceChecks {
		if _, ok := keep[r.Key]; ok {
			kept = append(kept, r)
		} else {
			deleted++
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

func (f *FakeStore) GetDiskHistoryInRange(_ string, _ time.Duration) ([]DiskHistoryPoint, error) {
	// TODO: implement for testing
	return nil, nil
}

// GetLastSMARTCollectedAt satisfies HistoryStore so *FakeStore keeps
// satisfying the Store composite. The real scheduler tests construct
// dedicated mocks rather than poking at this FakeStore (see
// internal/scheduler/stale_smart_test.go).
func (f *FakeStore) GetLastSMARTCollectedAt(_ string) (time.Time, bool, error) {
	return time.Time{}, false, nil
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
	return f.SaveProcessStatsAt(procs, time.Now())
}

func (f *FakeStore) SaveProcessStatsAt(procs []internal.ProcessInfo, ts time.Time) error {
	if len(procs) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range procs {
		name := extractProcessName(p.Command)
		if name == "" {
			continue
		}
		f.processHistory = append(f.processHistory, ProcessHistoryPoint{
			Timestamp:     ts,
			PID:           p.PID,
			User:          p.User,
			Name:          name,
			Command:       p.Command,
			ContainerName: p.ContainerName,
			ContainerID:   p.ContainerID,
			CPUPct:        p.CPU,
			MemPct:        p.Mem,
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

func (f *FakeStore) SaveSpeedTest(_ string, result *internal.SpeedTestResult) error {
	_, err := f.SaveSpeedTestReturningID("", result)
	return err
}

// SaveSpeedTestReturningID mirrors *DB's same-named method: appends a
// history row + returns its synthetic ID. The fake assigns IDs from a
// monotonically-increasing counter so test code can correlate samples
// against a known parent row. PRD #283 / issue #286.
func (f *FakeStore) SaveSpeedTestReturningID(_ string, result *internal.SpeedTestResult) (int64, error) {
	if result == nil {
		return 0, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ts := result.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	f.speedTestNextID++
	id := f.speedTestNextID
	engine := result.Engine
	if engine == "" {
		engine = internal.SpeedTestEngineOoklaCLI
	}
	f.speedTestHistory = append(f.speedTestHistory, SpeedTestHistoryPoint{
		ID:           id,
		Timestamp:    ts,
		DownloadMbps: result.DownloadMbps,
		UploadMbps:   result.UploadMbps,
		LatencyMs:    result.LatencyMs,
		JitterMs:     result.JitterMs,
		ServerName:   result.ServerName,
		ISP:          result.ISP,
		Engine:       engine,
	})
	return id, nil
}

func (f *FakeStore) GetSpeedTestHistory(hours int) ([]SpeedTestHistoryPoint, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.speedTestHistory) == 0 {
		return nil, nil
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	out := make([]SpeedTestHistoryPoint, 0, len(f.speedTestHistory))
	for _, p := range f.speedTestHistory {
		if p.Timestamp.Before(cutoff) {
			continue
		}
		out = append(out, p)
	}
	// Ascending order matches the DB query.
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out, nil
}

// SaveSpeedTestAttempt records the current speed-test attempt state.
// Single-row semantics — the existing attempt is replaced.
func (f *FakeStore) SaveSpeedTestAttempt(att LastSpeedTestAttempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := att
	f.speedTestAttempt = &cp
	return nil
}

// GetLastSpeedTestAttempt returns the current attempt state or (nil, nil)
// if none has been recorded (fresh install pre-first scheduler tick).
func (f *FakeStore) GetLastSpeedTestAttempt() (*LastSpeedTestAttempt, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.speedTestAttempt == nil {
		return nil, nil
	}
	cp := *f.speedTestAttempt
	return &cp, nil
}

// GetLatestSpeedTestHistoryID returns the most-recently-saved history
// row's synthetic ID. Returns (0, false, nil) on an empty store.
// PRD #283 slice 3 / issue #286.
func (f *FakeStore) GetLatestSpeedTestHistoryID() (int64, bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.speedTestHistory) == 0 {
		return 0, false, nil
	}
	// Last appended row wins (ascending append order).
	return f.speedTestHistory[len(f.speedTestHistory)-1].ID, true, nil
}

// InsertSpeedTestSamples bulk-stores samples for an existing history
// row. Insert into a non-existent test_id returns an error to mirror
// the FK-constraint behaviour of the real DB. Re-inserting the same
// (test_id, sample_index) is rejected (mirrors the PK constraint).
// PRD #283 slice 3 / issue #286.
func (f *FakeStore) InsertSpeedTestSamples(testID int64, samples []SpeedTestSample) error {
	if len(samples) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Verify the parent row exists (FK constraint emulation).
	parentExists := false
	for _, p := range f.speedTestHistory {
		if p.ID == testID {
			parentExists = true
			break
		}
	}
	if !parentExists {
		return fmt.Errorf("FOREIGN KEY constraint failed: speedtest_history row %d not found", testID)
	}
	if f.speedTestSamples == nil {
		f.speedTestSamples = make(map[int64][]SpeedTestSample)
	}
	existing := f.speedTestSamples[testID]
	// Build an index set of already-stored sample_index values to
	// reject duplicates (PK constraint emulation).
	taken := make(map[int]struct{}, len(existing))
	for _, e := range existing {
		taken[e.SampleIndex] = struct{}{}
	}
	for _, s := range samples {
		if _, dup := taken[s.SampleIndex]; dup {
			return fmt.Errorf("UNIQUE constraint failed: speedtest_samples (test_id=%d, sample_index=%d)", testID, s.SampleIndex)
		}
		taken[s.SampleIndex] = struct{}{}
	}
	f.speedTestSamples[testID] = append(existing, samples...)
	return nil
}

// GetSpeedTestSamples returns samples for the given test_id ordered by
// sample_index ascending. An unknown test_id returns an empty slice
// (NOT an error) to match the *DB behaviour. PRD #283 slice 3 /
// issue #286.
func (f *FakeStore) GetSpeedTestSamples(testID int64) ([]SpeedTestSample, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	src := f.speedTestSamples[testID]
	out := make([]SpeedTestSample, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool { return out[i].SampleIndex < out[j].SampleIndex })
	return out, nil
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

// PruneDiskUsageHistory removes disk_usage_history rows with timestamp < cutoff.
func (f *FakeStore) PruneDiskUsageHistory(cutoff time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var kept []diskUsageRow
	var pruned int64
	for _, r := range f.diskUsageHistory {
		if r.Timestamp.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, r)
		}
	}
	f.diskUsageHistory = kept
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

// ── DriveEventStore ──

// SaveDriveEvent stores a new drive event and returns its assigned id.
func (f *FakeStore) SaveDriveEvent(ev DriveEvent) (int64, error) {
	if ev.SlotKey == "" {
		return 0, fmt.Errorf("slot_key is required")
	}
	if ev.EventType == "" {
		return 0, fmt.Errorf("event_type is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.driveEventSeq++
	ev.ID = f.driveEventSeq
	if ev.EventTime.IsZero() {
		ev.EventTime = time.Now().UTC()
	}
	ev.CreatedAt = time.Now().UTC()
	ev.UpdatedAt = nil
	f.driveEvents = append(f.driveEvents, ev)
	return ev.ID, nil
}

// ListDriveEvents returns events for slotKey, newest first.
func (f *FakeStore) ListDriveEvents(slotKey string) ([]DriveEvent, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var out []DriveEvent
	for _, ev := range f.driveEvents {
		if ev.SlotKey == slotKey {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EventTime.Equal(out[j].EventTime) {
			return out[i].ID > out[j].ID
		}
		return out[i].EventTime.After(out[j].EventTime)
	})
	return out, nil
}

// UpdateDriveEvent mutates a manual event's time and/or content.
func (f *FakeStore) UpdateDriveEvent(slotKey string, id int64, eventTime *time.Time, content *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.driveEvents {
		if f.driveEvents[i].ID != id {
			continue
		}
		if f.driveEvents[i].SlotKey != slotKey {
			return &DriveEventNotFoundError{SlotKey: slotKey, ID: id}
		}
		if f.driveEvents[i].IsAuto {
			return &DriveEventImmutableError{ID: id}
		}
		if eventTime != nil {
			f.driveEvents[i].EventTime = *eventTime
		}
		if content != nil {
			f.driveEvents[i].Content = *content
		}
		if eventTime != nil || content != nil {
			now := time.Now().UTC()
			f.driveEvents[i].UpdatedAt = &now
		}
		return nil
	}
	return &DriveEventNotFoundError{SlotKey: slotKey, ID: id}
}

// DeleteDriveEvent removes a manual event.
func (f *FakeStore) DeleteDriveEvent(slotKey string, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.driveEvents {
		if f.driveEvents[i].ID != id {
			continue
		}
		if f.driveEvents[i].SlotKey != slotKey {
			return &DriveEventNotFoundError{SlotKey: slotKey, ID: id}
		}
		if f.driveEvents[i].IsAuto {
			return &DriveEventImmutableError{ID: id}
		}
		f.driveEvents = append(f.driveEvents[:i], f.driveEvents[i+1:]...)
		return nil
	}
	return &DriveEventNotFoundError{SlotKey: slotKey, ID: id}
}

// GetDriveEvent returns a single event or nil if not found.
func (f *FakeStore) GetDriveEvent(id int64) (*DriveEvent, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for i := range f.driveEvents {
		if f.driveEvents[i].ID == id {
			cp := f.driveEvents[i]
			return &cp, nil
		}
	}
	return nil, nil
}

// GetDriveSlotState returns the last-observed state for slotKey, or
// (nil, nil) if no state has been recorded.
func (f *FakeStore) GetDriveSlotState(slotKey string) (*DriveSlotState, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if state, ok := f.driveSlotStates[slotKey]; ok {
		cp := state
		return &cp, nil
	}
	return nil, nil
}

// SaveDriveSlotState UPSERTs the last-observed state for a slot.
func (f *FakeStore) SaveDriveSlotState(state DriveSlotState) error {
	if state.SlotKey == "" {
		return fmt.Errorf("slot_key is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.driveSlotStates == nil {
		f.driveSlotStates = make(map[string]DriveSlotState)
	}
	if state.ObservedAt.IsZero() {
		state.ObservedAt = time.Now().UTC()
	}
	f.driveSlotStates[state.SlotKey] = state
	return nil
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

// AddDiskUsageHistoryEntry seeds a disk_usage_history row for testing.
func (f *FakeStore) AddDiskUsageHistoryEntry(mountPoint string, ts time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.diskUsageHistory = append(f.diskUsageHistory, diskUsageRow{
		MountPoint: mountPoint,
		Timestamp:  ts,
	})
}

// DiskUsageHistoryCount returns the number of disk_usage_history rows in the fake store.
func (f *FakeStore) DiskUsageHistoryCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.diskUsageHistory)
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

// cloneDetails returns a shallow copy of the per-type Details map so
// successive Reads don't hand out the same underlying map that a later
// Save mutates (issue #182: persisted log rows round-trip Details).
// Returns nil for nil/empty input so reads match the DB path where NULL
// JSON is surfaced as nil.
func cloneDetails(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
