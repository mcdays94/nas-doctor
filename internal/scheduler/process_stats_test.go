package scheduler

import (
	"log/slog"
	"os"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newTestScheduler creates a minimal Scheduler suitable for unit tests.
// It uses a real Collector (which calls ps/proc) and a FakeStore.
func newTestScheduler() (*Scheduler, *storage.FakeStore) {
	store := storage.NewFakeStore()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	col := collector.New(internal.HostPaths{}, logger)

	s := New(col, store, nil, nil, logger, 0)
	return s, store
}

func TestCollectProcessStats_SavesProcessesToStore(t *testing.T) {
	s, store := newTestScheduler()

	// Seed a cached snapshot so collectProcessStats has somewhere to update
	s.latest = &internal.Snapshot{
		System: internal.SystemInfo{},
		Docker: internal.DockerInfo{Available: false},
	}

	s.collectProcessStats()

	// Verify process history was saved to the store
	history, err := store.GetProcessHistory(1)
	if err != nil {
		t.Fatalf("GetProcessHistory failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected process history entries after collectProcessStats, got 0")
	}
	// At least one process should have a non-empty name
	foundNamed := false
	for _, p := range history {
		if p.Name != "" {
			foundNamed = true
			break
		}
	}
	if !foundNamed {
		t.Error("expected at least one process with a non-empty name")
	}
}

func TestCollectProcessStats_UpdatesCachedSnapshot(t *testing.T) {
	s, _ := newTestScheduler()

	// Seed a cached snapshot with empty TopProcesses
	s.latest = &internal.Snapshot{
		System: internal.SystemInfo{TopProcesses: nil},
		Docker: internal.DockerInfo{Available: false},
	}

	s.collectProcessStats()

	s.mu.RLock()
	procs := s.latest.System.TopProcesses
	s.mu.RUnlock()

	if len(procs) == 0 {
		t.Fatal("expected cached TopProcesses to be updated, got empty")
	}
}

func TestCollectProcessStats_NilLatest_NoPanic(t *testing.T) {
	s, _ := newTestScheduler()

	// latest is nil — collectProcessStats should handle gracefully
	s.latest = nil

	// Should not panic
	s.collectProcessStats()
}

func TestCollectProcessStats_RespectsLimit(t *testing.T) {
	s, _ := newTestScheduler()

	s.latest = &internal.Snapshot{
		System: internal.SystemInfo{},
		Docker: internal.DockerInfo{Available: false},
	}

	s.collectProcessStats()

	s.mu.RLock()
	procs := s.latest.System.TopProcesses
	s.mu.RUnlock()

	// The implementation should collect at most 15 processes
	if len(procs) > 15 {
		t.Errorf("expected at most 15 processes, got %d", len(procs))
	}
}

func TestCollectProcessStats_UsesContainerData(t *testing.T) {
	s, _ := newTestScheduler()

	// Seed a cached snapshot with Docker containers available
	// (container attribution won't actually match any real process on the
	// test host, but we verify the code path doesn't panic)
	s.latest = &internal.Snapshot{
		System: internal.SystemInfo{},
		Docker: internal.DockerInfo{
			Available: true,
			Containers: []internal.ContainerInfo{
				{ID: "abc123def456", Name: "test-container"},
			},
		},
	}

	// Should not panic even though no processes will match
	s.collectProcessStats()

	s.mu.RLock()
	procs := s.latest.System.TopProcesses
	s.mu.RUnlock()

	if len(procs) == 0 {
		t.Fatal("expected some processes even when no container matches")
	}
}
