package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestRunRetention_PrunesHighFreqProcessHistory confirms RunRetention wires the
// new high-frequency retention: process_history rows older than the 30-day
// horizon are pruned, recent ones are kept. These rows carry synthetic
// snapshot_ids, so before this they were never pruned and grew without bound.
func TestRunRetention_PrunesHighFreqProcessHistory(t *testing.T) {
	store := storage.NewFakeStore()

	proc := []internal.ProcessInfo{{PID: 1, Command: "ffmpeg", CPU: 10}}
	// One row beyond the 30d horizon, one within it.
	if err := store.SaveProcessStatsAt(proc, time.Now().Add(-40*24*time.Hour)); err != nil {
		t.Fatalf("seed old process row: %v", err)
	}
	if err := store.SaveProcessStatsAt(proc, time.Now().Add(-2*24*time.Hour)); err != nil {
		t.Fatalf("seed recent process row: %v", err)
	}

	rm := NewRetentionManager(store, store, discardLogger())
	result := rm.RunRetention(defaultCfg())

	if result.ProcessHistoryPruned != 1 {
		t.Fatalf("ProcessHistoryPruned = %d; want 1 (the >30d row)", result.ProcessHistoryPruned)
	}
	remaining, err := store.GetProcessHistory(0)
	if err != nil {
		t.Fatalf("GetProcessHistory: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("remaining process_history rows = %d; want 1 (the recent row)", len(remaining))
	}
}
