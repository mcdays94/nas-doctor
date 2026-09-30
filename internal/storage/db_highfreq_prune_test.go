package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPruneHighFreqStats verifies the timestamp-based retention for the
// high-frequency history tables that carry synthetic snapshot_ids and are
// therefore never reached by snapshot pruning. Before this, they grew without
// bound (the DB-longevity failure that eventually forced the size cap to
// over-delete real snapshot history).
func TestPruneHighFreqStats(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(filepath.Join(dir, "test.db"), logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour)    // beyond the 30d horizon
	recent := now.Add(-2 * 24 * time.Hour)  // within it
	cutoff := now.Add(-30 * 24 * time.Hour) // the retention boundary

	// container_stats_history: one old row, one recent.
	for _, ts := range []time.Time{old, recent} {
		if _, err := db.db.Exec(
			`INSERT INTO container_stats_history (snapshot_id, container_id, name, timestamp) VALUES (?, ?, ?, ?)`,
			"cstats-x", "c1", "plex", ts,
		); err != nil {
			t.Fatalf("insert container row: %v", err)
		}
	}
	// process_history: one old row, one recent.
	for _, ts := range []time.Time{old, recent} {
		if _, err := db.db.Exec(
			`INSERT INTO process_history (snapshot_id, pid, name, timestamp) VALUES (?, ?, ?, ?)`,
			"pstats-x", 1234, "ffmpeg", ts,
		); err != nil {
			t.Fatalf("insert process row: %v", err)
		}
	}
	// speedtest_history: one old test (id 1) + one recent (id 2), each with a
	// sample row, so we can confirm the old test's samples are cleaned up and
	// the recent test's samples survive.
	for id, ts := range map[int64]time.Time{1: old, 2: recent} {
		if _, err := db.db.Exec(
			`INSERT INTO speedtest_history (id, snapshot_id, download_mbps, timestamp) VALUES (?, ?, ?, ?)`,
			id, "speedtest-x", 500.0, ts,
		); err != nil {
			t.Fatalf("insert speedtest row: %v", err)
		}
		if _, err := db.db.Exec(
			`INSERT INTO speedtest_samples (test_id, sample_index, phase, ts, mbps) VALUES (?, ?, ?, ?, ?)`,
			id, 0, "download", ts, 480.0,
		); err != nil {
			t.Fatalf("insert speedtest sample: %v", err)
		}
	}

	// Prune everything older than 30 days.
	if n, err := db.PruneContainerStats(cutoff); err != nil || n != 1 {
		t.Fatalf("PruneContainerStats = (%d, %v); want (1, nil)", n, err)
	}
	if n, err := db.PruneProcessHistory(cutoff); err != nil || n != 1 {
		t.Fatalf("PruneProcessHistory = (%d, %v); want (1, nil)", n, err)
	}
	if n, err := db.PruneSpeedTestHistory(cutoff); err != nil || n != 1 {
		t.Fatalf("PruneSpeedTestHistory = (%d, %v); want (1, nil)", n, err)
	}

	// Exactly the recent rows survive in each table.
	assertCount(t, db, "container_stats_history", 1)
	assertCount(t, db, "process_history", 1)
	assertCount(t, db, "speedtest_history", 1)
	// The old test's sample must be gone (orphan cleanup, not relying on the
	// FK cascade); the recent test's sample must remain.
	assertCount(t, db, "speedtest_samples", 1)
	var remainingTestID int64
	if err := db.db.QueryRow(`SELECT test_id FROM speedtest_samples`).Scan(&remainingTestID); err != nil {
		t.Fatalf("scan remaining sample: %v", err)
	}
	if remainingTestID != 2 {
		t.Errorf("surviving sample belongs to test_id %d; want 2 (recent)", remainingTestID)
	}

	// Idempotent: a second prune removes nothing.
	if n, _ := db.PruneContainerStats(cutoff); n != 0 {
		t.Errorf("second PruneContainerStats removed %d; want 0", n)
	}
}

func assertCount(t *testing.T, db *DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Errorf("%s row count = %d; want %d", table, got, want)
	}
}
