package scheduler

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// helper: create a logger that discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// helper: create a snapshot at a given time offset from now.
func makeSnapshot(id string, age time.Duration) *internal.Snapshot {
	return &internal.Snapshot{
		ID:        id,
		Timestamp: time.Now().Add(-age),
	}
}

// helper: default config with generous retention (nothing pruned).
func defaultCfg() RetentionManagerConfig {
	return RetentionManagerConfig{
		SnapshotMaxAge:     90 * 24 * time.Hour,
		SnapshotKeepMin:    10,
		ServiceCheckMaxAge: 30 * 24 * time.Hour,
		NotificationMaxAge: 30 * 24 * time.Hour,
		AlertMaxAge:        30 * 24 * time.Hour,
		MaxDBSizeMB:        500,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 1: Snapshot pruning — old snapshots removed, keep-minimum respected
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_SnapshotPruning_KeepMinRespected(t *testing.T) {
	store := storage.NewFakeStore()
	for i := 0; i < 15; i++ {
		if err := store.SaveSnapshot(makeSnapshot(
			fmt.Sprintf("snap-%02d", i),
			100*24*time.Hour+time.Duration(i)*time.Minute,
		)); err != nil {
			t.Fatal(err)
		}
	}

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.SnapshotKeepMin = 10

	result := rm.RunRetention(cfg)

	if result.SnapshotsPruned != 5 {
		t.Fatalf("expected 5 snapshots pruned, got %d", result.SnapshotsPruned)
	}
	if store.SnapshotCount() != 10 {
		t.Fatalf("expected 10 remaining snapshots, got %d", store.SnapshotCount())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 2: Snapshot pruning — all within age → nothing pruned
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_SnapshotPruning_AllRecent(t *testing.T) {
	store := storage.NewFakeStore()
	for i := 0; i < 5; i++ {
		if err := store.SaveSnapshot(makeSnapshot(
			fmt.Sprintf("snap-%02d", i),
			time.Duration(i)*time.Hour,
		)); err != nil {
			t.Fatal(err)
		}
	}

	rm := NewRetentionManager(store, store, discardLogger())
	result := rm.RunRetention(defaultCfg())

	if result.SnapshotsPruned != 0 {
		t.Fatalf("expected 0 snapshots pruned, got %d", result.SnapshotsPruned)
	}
	if store.SnapshotCount() != 5 {
		t.Fatalf("expected 5 remaining, got %d", store.SnapshotCount())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 3: Service check pruning — old entries removed
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_ServiceCheckPruning(t *testing.T) {
	store := storage.NewFakeStore()

	oldTime := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	recentTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)

	_ = store.SaveServiceCheckResults([]internal.ServiceCheckResult{
		{Key: "check-1", Name: "Old check", CheckedAt: oldTime, Status: "up"},
		{Key: "check-2", Name: "Recent check", CheckedAt: recentTime, Status: "up"},
	})

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.ServiceCheckMaxAge = 30 * 24 * time.Hour

	result := rm.RunRetention(cfg)

	if result.ServiceChecksPruned != 1 {
		t.Fatalf("expected 1 service check pruned, got %d", result.ServiceChecksPruned)
	}
	if store.ServiceCheckCount() != 1 {
		t.Fatalf("expected 1 remaining, got %d", store.ServiceCheckCount())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 4: Notification log pruning — old entries removed
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_NotificationLogPruning(t *testing.T) {
	store := storage.NewFakeStore()

	store.AddNotificationLogEntry(storage.NotificationLogEntry{
		ID: 1, CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
	})
	store.AddNotificationLogEntry(storage.NotificationLogEntry{
		ID: 2, CreatedAt: time.Now().Add(-1 * time.Hour),
	})

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.NotificationMaxAge = 30 * 24 * time.Hour

	result := rm.RunRetention(cfg)

	if result.NotificationsPruned != 1 {
		t.Fatalf("expected 1 notification pruned, got %d", result.NotificationsPruned)
	}
	if store.NotificationLogCount() != 1 {
		t.Fatalf("expected 1 remaining, got %d", store.NotificationLogCount())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 5: Alert pruning — resolved alerts older than max age removed
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_AlertPruning(t *testing.T) {
	store := storage.NewFakeStore()

	store.AddAlert(storage.AlertRecord{
		ID: 1, Status: storage.AlertStatusResolved,
		ResolvedAt: time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
	})
	store.AddAlert(storage.AlertRecord{
		ID: 2, Status: storage.AlertStatusResolved,
		ResolvedAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	})
	store.AddAlert(storage.AlertRecord{
		ID: 3, Status: storage.AlertStatusOpen,
	})

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.AlertMaxAge = 30 * 24 * time.Hour

	result := rm.RunRetention(cfg)

	if result.AlertsPruned != 1 {
		t.Fatalf("expected 1 alert pruned, got %d", result.AlertsPruned)
	}
	if store.AlertCount() != 2 {
		t.Fatalf("expected 2 remaining, got %d", store.AlertCount())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 6: Orphan cleanup — orphaned findings removed
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_OrphanCleanup(t *testing.T) {
	store := storage.NewFakeStore()
	store.AddOrphanedFindings(7)

	rm := NewRetentionManager(store, store, discardLogger())
	result := rm.RunRetention(defaultCfg())

	if result.OrphansPruned != 7 {
		t.Fatalf("expected 7 orphans pruned, got %d", result.OrphansPruned)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 7: Size-based pruning — triggers when DB exceeds target size
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_SizeBasedPruning(t *testing.T) {
	store := storage.NewFakeStore()
	store.DBSizeMB = 600

	for i := 0; i < 20; i++ {
		_ = store.SaveSnapshot(makeSnapshot(fmt.Sprintf("snap-%02d", i), time.Duration(i)*time.Hour))
	}

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.MaxDBSizeMB = 500

	result := rm.RunRetention(cfg)

	if result.SizePruned == 0 {
		t.Fatal("expected size-based pruning, got 0")
	}
	if store.SnapshotCount() >= 20 {
		t.Fatalf("expected fewer than 20 snapshots, got %d", store.SnapshotCount())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 8: Vacuum runs after age-based pruning
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_VacuumAfterPruning(t *testing.T) {
	store := storage.NewFakeStore()
	store.AddNotificationLogEntry(storage.NotificationLogEntry{
		ID: 1, CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
	})

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.NotificationMaxAge = 30 * 24 * time.Hour

	result := rm.RunRetention(cfg)

	if !result.Vacuumed {
		t.Fatal("expected vacuum to run after pruning")
	}
	if !store.VacuumCalled {
		t.Fatal("expected FakeStore.Vacuum() to be called")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 9: Vacuum skipped when size-based pruning occurred
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_VacuumSkippedAfterSizePruning(t *testing.T) {
	store := storage.NewFakeStore()
	store.DBSizeMB = 600

	for i := 0; i < 20; i++ {
		_ = store.SaveSnapshot(makeSnapshot(fmt.Sprintf("snap-%02d", i), time.Duration(i)*time.Minute))
	}

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := defaultCfg()
	cfg.MaxDBSizeMB = 500

	result := rm.RunRetention(cfg)

	if result.Vacuumed {
		t.Fatal("expected vacuum NOT to run after size-based pruning")
	}
	if result.SizePruned == 0 {
		t.Fatal("expected size-based pruning to occur")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 10: Combined retention — all operations run, results correct
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_CombinedAllOperations(t *testing.T) {
	store := storage.NewFakeStore()

	for i := 0; i < 5; i++ {
		_ = store.SaveSnapshot(makeSnapshot(
			fmt.Sprintf("snap-%02d", i),
			100*24*time.Hour+time.Duration(i)*time.Minute,
		))
	}

	oldTime := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	_ = store.SaveServiceCheckResults([]internal.ServiceCheckResult{
		{Key: "svc-old", Name: "Old", CheckedAt: oldTime, Status: "up"},
	})

	store.AddNotificationLogEntry(storage.NotificationLogEntry{
		ID: 1, CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
	})

	store.AddAlert(storage.AlertRecord{
		ID: 1, Status: storage.AlertStatusResolved,
		ResolvedAt: time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
	})

	store.AddOrphanedFindings(3)

	rm := NewRetentionManager(store, store, discardLogger())
	cfg := RetentionManagerConfig{
		SnapshotMaxAge:     90 * 24 * time.Hour,
		SnapshotKeepMin:    2,
		ServiceCheckMaxAge: 30 * 24 * time.Hour,
		NotificationMaxAge: 30 * 24 * time.Hour,
		AlertMaxAge:        30 * 24 * time.Hour,
		MaxDBSizeMB:        0,
	}

	result := rm.RunRetention(cfg)

	if result.SnapshotsPruned != 3 {
		t.Errorf("snapshots: expected 3, got %d", result.SnapshotsPruned)
	}
	if result.ServiceChecksPruned != 1 {
		t.Errorf("service checks: expected 1, got %d", result.ServiceChecksPruned)
	}
	if result.NotificationsPruned != 1 {
		t.Errorf("notifications: expected 1, got %d", result.NotificationsPruned)
	}
	if result.AlertsPruned != 1 {
		t.Errorf("alerts: expected 1, got %d", result.AlertsPruned)
	}
	if result.OrphansPruned != 3 {
		t.Errorf("orphans: expected 3, got %d", result.OrphansPruned)
	}
	if !result.Vacuumed {
		t.Error("expected vacuum to run")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 11: No-op retention — everything within bounds → zero counts
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_NoOp(t *testing.T) {
	store := storage.NewFakeStore()

	for i := 0; i < 3; i++ {
		_ = store.SaveSnapshot(makeSnapshot(fmt.Sprintf("snap-%02d", i), time.Duration(i)*time.Hour))
	}
	recentTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	_ = store.SaveServiceCheckResults([]internal.ServiceCheckResult{
		{Key: "svc-1", Name: "Recent", CheckedAt: recentTime, Status: "up"},
	})
	store.AddNotificationLogEntry(storage.NotificationLogEntry{
		ID: 1, CreatedAt: time.Now().Add(-1 * time.Hour),
	})

	rm := NewRetentionManager(store, store, discardLogger())
	result := rm.RunRetention(defaultCfg())

	if result.SnapshotsPruned != 0 || result.ServiceChecksPruned != 0 ||
		result.NotificationsPruned != 0 || result.AlertsPruned != 0 ||
		result.OrphansPruned != 0 || result.SizePruned != 0 {
		t.Errorf("expected all zeros, got %+v", result)
	}
	if result.Vacuumed {
		t.Error("expected vacuum NOT to run")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 12: RunBackup — disabled returns nil
// ─────────────────────────────────────────────────────────────────────────────

func TestRunBackup_Disabled(t *testing.T) {
	store := storage.NewFakeStore()
	rm := NewRetentionManager(store, store, discardLogger())

	result, err := rm.RunBackup(BackupManagerConfig{Enabled: false}, time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %+v", result)
	}
	if store.BackupCalls != 0 {
		t.Fatalf("expected 0 backup calls, got %d", store.BackupCalls)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 13: RunBackup — not due yet
// ─────────────────────────────────────────────────────────────────────────────

func TestRunBackup_NotDueYet(t *testing.T) {
	store := storage.NewFakeStore()
	rm := NewRetentionManager(store, store, discardLogger())

	lastBackup := time.Now().Add(-1 * time.Hour)
	cfg := BackupManagerConfig{Enabled: true, IntervalH: 24}

	result, err := rm.RunBackup(cfg, lastBackup, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil when not due")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 14: RunBackup — due, creates backup
// ─────────────────────────────────────────────────────────────────────────────

func TestRunBackup_Due(t *testing.T) {
	store := storage.NewFakeStore()
	rm := NewRetentionManager(store, store, discardLogger())

	lastBackup := time.Now().Add(-48 * time.Hour)
	cfg := BackupManagerConfig{Enabled: true, IntervalH: 24, Path: t.TempDir()}

	result, err := rm.RunBackup(cfg, lastBackup, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected backup result")
	}
	if store.BackupCalls != 1 {
		t.Fatalf("expected 1 backup call, got %d", store.BackupCalls)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 15: RunBackup — first ever backup (zero lastBackup)
// ─────────────────────────────────────────────────────────────────────────────

func TestRunBackup_FirstEver(t *testing.T) {
	store := storage.NewFakeStore()
	rm := NewRetentionManager(store, store, discardLogger())

	cfg := BackupManagerConfig{Enabled: true, IntervalH: 168, Path: t.TempDir()}

	result, err := rm.RunBackup(cfg, time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected backup on first run")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 16: NewRetentionManager with nil logger
// ─────────────────────────────────────────────────────────────────────────────

func TestNewRetentionManager_NilLogger(t *testing.T) {
	store := storage.NewFakeStore()
	rm := NewRetentionManager(store, store, nil)
	if rm.logger == nil {
		t.Fatal("expected non-nil logger")
	}
	_ = rm.RunRetention(defaultCfg()) // smoke test: should not panic
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 17: RunRetention with nil service check store
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRetention_NilServiceCheckStore(t *testing.T) {
	store := storage.NewFakeStore()
	rm := NewRetentionManager(store, nil, discardLogger())

	result := rm.RunRetention(defaultCfg())

	if result.ServiceChecksPruned != 0 {
		t.Fatalf("expected 0 with nil svc store, got %d", result.ServiceChecksPruned)
	}
}
