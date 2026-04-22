package scheduler

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newSchedulerForTest builds a minimal scheduler suitable for exercising
// config-management methods like UpdateServiceChecks. Collector/notifier are
// nil — the methods under test must not touch them.
func newSchedulerForTest(store storage.Store) *Scheduler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &Scheduler{
		store:             store,
		logger:            logger,
		interval:          time.Hour,
		speedTestInterval: 4 * time.Hour,
		serviceChecks:     []internal.ServiceCheckConfig{},
		stop:              make(chan struct{}),
		restart:           make(chan time.Duration, 1),
	}
}

func seedHistory(t *testing.T, store storage.Store, keys ...string) {
	t.Helper()
	now := time.Now().UTC()
	results := make([]internal.ServiceCheckResult, 0, len(keys))
	for i, k := range keys {
		results = append(results, internal.ServiceCheckResult{
			Key:              k,
			Name:             k,
			Type:             "http",
			Target:           "http://" + k,
			Status:           "up",
			FailureThreshold: 1,
			FailureSeverity:  internal.SeverityWarning,
			CheckedAt:        now.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		})
	}
	if err := store.SaveServiceCheckResults(results); err != nil {
		t.Fatalf("SaveServiceCheckResults: %v", err)
	}
}

func historyKeys(t *testing.T, store storage.Store) map[string]bool {
	t.Helper()
	entries, err := store.ListLatestServiceChecks(100)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	keys := make(map[string]bool)
	for _, e := range entries {
		keys[e.Key] = true
	}
	return keys
}

// UpdateServiceChecks must purge history rows whose keys are no longer
// present in the incoming config. Issue #133.
func TestUpdateServiceChecks_PurgesOrphanedHistory(t *testing.T) {
	store := storage.NewFakeStore()

	// User ran checks for A, B, C in the past.
	checkA := internal.ServiceCheckConfig{Name: "A", Type: "http", Target: "http://a.example"}
	checkB := internal.ServiceCheckConfig{Name: "B", Type: "http", Target: "http://b.example"}
	checkC := internal.ServiceCheckConfig{Name: "C", Type: "http", Target: "http://c.example"}
	seedHistory(t, store, CheckKey(checkA), CheckKey(checkB), CheckKey(checkC))

	before := historyKeys(t, store)
	if len(before) != 3 {
		t.Fatalf("precondition: expected 3 keys seeded, got %d (%v)", len(before), before)
	}

	// User deleted B and C in settings; only A remains.
	sched := newSchedulerForTest(store)
	sched.UpdateServiceChecks([]internal.ServiceCheckConfig{checkA})

	after := historyKeys(t, store)
	if !after[CheckKey(checkA)] {
		t.Error("expected history for A to be retained")
	}
	if after[CheckKey(checkB)] {
		t.Error("expected history for B to be purged (orphan)")
	}
	if after[CheckKey(checkC)] {
		t.Error("expected history for C to be purged (orphan)")
	}
}

// When the user removes every service check, all history must be purged too.
func TestUpdateServiceChecks_PurgesAllWhenConfigEmpty(t *testing.T) {
	store := storage.NewFakeStore()

	checkA := internal.ServiceCheckConfig{Name: "A", Type: "http", Target: "http://a.example"}
	checkB := internal.ServiceCheckConfig{Name: "B", Type: "http", Target: "http://b.example"}
	seedHistory(t, store, CheckKey(checkA), CheckKey(checkB))

	sched := newSchedulerForTest(store)
	sched.UpdateServiceChecks(nil)

	after := historyKeys(t, store)
	if len(after) != 0 {
		t.Errorf("expected all history purged, got %d keys: %v", len(after), after)
	}
}

// Invalid or blank checks in the config must not leave their (non-existent)
// keys as a signal to keep orphans around — but valid ones must keep theirs.
func TestUpdateServiceChecks_KeysDerivedFromNormalizedConfig(t *testing.T) {
	store := storage.NewFakeStore()

	valid := internal.ServiceCheckConfig{Name: "Valid", Type: "http", Target: "http://v.example"}
	seedHistory(t, store, CheckKey(valid), "stale-orphan-key")

	sched := newSchedulerForTest(store)
	sched.UpdateServiceChecks([]internal.ServiceCheckConfig{
		valid,
		{Name: "", Type: "http", Target: ""}, // dropped by normalization
		{Name: "Bad", Type: "bogus", Target: "x"},
	})

	after := historyKeys(t, store)
	if !after[CheckKey(valid)] {
		t.Error("valid check history must be retained")
	}
	if after["stale-orphan-key"] {
		t.Error("orphan history must be purged")
	}
}

// Issue #181 — reproduce the UAT scenario: settings.json on disk has only
// {A, B} but service_checks_history still contains rows for {A, B, C}
// from a prior-boot drift. Simulate startup (call UpdateServiceChecks with
// the loaded config) and assert C is purged — proving the existing
// UpdateServiceChecks purge IS a correct startup defense when it runs.
func TestScheduler_Startup_PurgesOrphanHistory(t *testing.T) {
	store := storage.NewFakeStore()

	checkA := internal.ServiceCheckConfig{Name: "A", Type: "http", Target: "http://a.example"}
	checkB := internal.ServiceCheckConfig{Name: "B", Type: "http", Target: "http://b.example"}
	checkC := internal.ServiceCheckConfig{Name: "Cloudflare 1.1.1.1", Type: "dns", Target: "1.1.1.1"}
	// Seed history as if C was legitimately running in a previous
	// container boot — the point of the bug is that its rows persist
	// into the next boot where C no longer exists in settings.json.
	seedHistory(t, store, CheckKey(checkA), CheckKey(checkB), CheckKey(checkC))

	// Simulate main.go startup: construct scheduler, apply only the
	// checks that remain on disk. UpdateServiceChecks MUST purge C.
	sched := newSchedulerForTest(store)
	sched.UpdateServiceChecks([]internal.ServiceCheckConfig{checkA, checkB})

	after := historyKeys(t, store)
	if len(after) != 2 {
		t.Errorf("expected 2 keys in history after startup purge, got %d: %v", len(after), after)
	}
	if after[CheckKey(checkC)] {
		t.Error("orphan Cloudflare 1.1.1.1 history must be purged at startup")
	}
	if !after[CheckKey(checkA)] || !after[CheckKey(checkB)] {
		t.Error("configured checks A and B must be retained")
	}
}

// Issue #181 defense-in-depth — PurgeOrphanServiceCheckHistory MUST be
// callable directly without first invoking UpdateServiceChecks, for the
// startup path where persisted settings failed to load (nil settings).
// When scheduler.serviceChecks is empty, the entire history table is
// drained: a legitimate outcome because the scheduler has no config to
// run checks against, so any existing history is necessarily stale.
func TestPurgeOrphanServiceCheckHistory_EmptyConfigDrainsHistory(t *testing.T) {
	store := storage.NewFakeStore()
	seedHistory(t, store, "orphan-1", "orphan-2", "orphan-3")

	sched := newSchedulerForTest(store)
	// No UpdateServiceChecks call — serviceChecks is the empty slice
	// assigned in newSchedulerForTest, matching main.go's behavior when
	// persistedSettings is nil and applySchedulerSettingsFromStore
	// returns early.
	pruned, err := sched.PurgeOrphanServiceCheckHistory()
	if err != nil {
		t.Fatalf("PurgeOrphanServiceCheckHistory: %v", err)
	}
	if pruned != 3 {
		t.Errorf("expected 3 rows pruned, got %d", pruned)
	}
	after := historyKeys(t, store)
	if len(after) != 0 {
		t.Errorf("expected empty history after purge with no config, got %d keys: %v", len(after), after)
	}
}

// Issue #181 — when the scheduler's in-memory config matches the history
// table, PurgeOrphanServiceCheckHistory is a safe no-op. This is the
// expected path for 99% of boots; it must not churn the DB.
func TestPurgeOrphanServiceCheckHistory_NoopWhenConfigMatches(t *testing.T) {
	store := storage.NewFakeStore()
	checkA := internal.ServiceCheckConfig{Name: "A", Type: "http", Target: "http://a.example"}
	seedHistory(t, store, CheckKey(checkA))

	sched := newSchedulerForTest(store)
	sched.UpdateServiceChecks([]internal.ServiceCheckConfig{checkA})

	// Second purge invocation (simulating the startup defense-in-depth
	// call in main.go immediately after applySchedulerSettingsFromStore)
	// MUST be a no-op: the config is already in sync.
	pruned, err := sched.PurgeOrphanServiceCheckHistory()
	if err != nil {
		t.Fatalf("PurgeOrphanServiceCheckHistory: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 rows pruned on second call, got %d", pruned)
	}
	after := historyKeys(t, store)
	if !after[CheckKey(checkA)] {
		t.Error("configured check A must still be in history")
	}
}

// Issue #181 — PurgeOrphanServiceCheckHistory must tolerate a nil store
// (defensive: a misbuilt scheduler must never panic on shutdown/startup).
func TestPurgeOrphanServiceCheckHistory_NilStoreSafe(t *testing.T) {
	sched := newSchedulerForTest(nil)
	pruned, err := sched.PurgeOrphanServiceCheckHistory()
	if err != nil {
		t.Fatalf("PurgeOrphanServiceCheckHistory with nil store: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned with nil store, got %d", pruned)
	}
}
