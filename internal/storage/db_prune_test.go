package storage

import (
	"testing"
	"time"
)

// seedSnapshotWithHistory inserts one snapshot plus one row into every
// snapshot-bound history table (smart_history, system_history, gpu_history,
// container_stats_history, speedtest_history, process_history) AND one row
// into disk_usage_history (which is snapshot-independent).
//
// Inserts are done with raw SQL (bypassing SaveSnapshot) so each history
// table ends up with exactly one row per snapshot seeded — SaveSnapshot
// would add an extra system_history row automatically.
func seedSnapshotWithHistory(t *testing.T, db *DB, snapID string, ts time.Time) {
	t.Helper()

	// 1. snapshot (raw insert — SaveSnapshot would auto-insert system_history)
	if _, err := db.db.Exec(
		`INSERT INTO snapshots (id, timestamp, duration_seconds, data) VALUES (?, ?, ?, ?)`,
		snapID, ts, 0.1, "{}",
	); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	// 2. snapshot-bound history rows
	inserts := []struct {
		name string
		sql  string
		args []any
	}{
		{
			"smart_history",
			`INSERT INTO smart_history (snapshot_id, device, serial, model, temperature, timestamp) VALUES (?,?,?,?,?,?)`,
			[]any{snapID, "/dev/sda", "SN1", "MODEL", 35, ts},
		},
		{
			"system_history",
			`INSERT INTO system_history (snapshot_id, cpu_usage, mem_percent, timestamp) VALUES (?,?,?,?)`,
			[]any{snapID, 10.0, 50.0, ts},
		},
		{
			"gpu_history",
			`INSERT INTO gpu_history (snapshot_id, gpu_index, name, timestamp) VALUES (?,?,?,?)`,
			[]any{snapID, 0, "gpu0", ts},
		},
		{
			"container_stats_history",
			`INSERT INTO container_stats_history (snapshot_id, container_id, name, timestamp) VALUES (?,?,?,?)`,
			[]any{snapID, "cid1", "container1", ts},
		},
		{
			"speedtest_history",
			`INSERT INTO speedtest_history (snapshot_id, download_mbps, timestamp) VALUES (?,?,?)`,
			[]any{snapID, 100.0, ts},
		},
		{
			"process_history",
			`INSERT INTO process_history (snapshot_id, pid, name, timestamp) VALUES (?,?,?,?)`,
			[]any{snapID, 1, "init", ts},
		},
	}
	for _, ins := range inserts {
		if _, err := db.db.Exec(ins.sql, ins.args...); err != nil {
			t.Fatalf("insert %s: %v", ins.name, err)
		}
	}

	// 3. snapshot-independent disk_usage_history row (no snapshot_id column)
	if _, err := db.db.Exec(
		`INSERT INTO disk_usage_history (mount_point, label, device, total_gb, used_gb, free_gb, used_pct, timestamp) VALUES (?,?,?,?,?,?,?,?)`,
		"/mnt/disk1", "disk1", "/dev/sda1", 1000.0, 500.0, 500.0, 50.0, ts,
	); err != nil {
		t.Fatalf("insert disk_usage_history: %v", err)
	}
}

func countRows(t *testing.T, db *DB, table string) int {
	t.Helper()
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestPruneSnapshots_WithDiskUsageHistory demonstrates the bug:
// PruneSnapshots tries to `DELETE FROM disk_usage_history WHERE snapshot_id IN (...)`
// but that column doesn't exist, so the whole transaction rolls back and NO
// snapshot/history pruning happens.
func TestPruneSnapshots_WithDiskUsageHistory(t *testing.T) {
	db := newTestDB(t)

	// Seed one old snapshot + rows in every history table.
	seedSnapshotWithHistory(t, db, "snap-old", time.Now().Add(-48*time.Hour))

	// Prune everything older than 1h, keep 0 minimum.
	pruned, err := db.PruneSnapshots(1*time.Hour, 0)
	if err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 snapshot pruned, got %d", pruned)
	}

	// snapshot gone
	if n := countRows(t, db, "snapshots"); n != 0 {
		t.Errorf("expected 0 snapshots after prune, got %d", n)
	}
	// All 6 snapshot-bound history tables empty
	for _, table := range []string{
		"smart_history", "system_history", "gpu_history",
		"container_stats_history", "speedtest_history", "process_history",
	} {
		if n := countRows(t, db, table); n != 0 {
			t.Errorf("expected 0 rows in %s, got %d", table, n)
		}
	}

	// disk_usage_history untouched by PruneSnapshots (managed independently)
	if n := countRows(t, db, "disk_usage_history"); n != 1 {
		t.Errorf("expected disk_usage_history untouched (1 row), got %d", n)
	}
}

// TestPruneSnapshots_RollbackIsolation proves that when disk_usage_history is
// wrongly included in the prune loop, the transaction rollback cascades and
// deletes from smart_history/system_history are reverted too. After the fix,
// those deletes actually commit.
func TestPruneSnapshots_RollbackIsolation(t *testing.T) {
	db := newTestDB(t)

	// Two snapshots: one old (should prune), one recent (should stay).
	seedSnapshotWithHistory(t, db, "snap-old", time.Now().Add(-48*time.Hour))
	seedSnapshotWithHistory(t, db, "snap-new", time.Now().Add(-5*time.Minute))

	// 2 rows in each history table now
	if n := countRows(t, db, "smart_history"); n != 2 {
		t.Fatalf("seeding: expected 2 smart_history rows, got %d", n)
	}
	if n := countRows(t, db, "system_history"); n != 2 {
		t.Fatalf("seeding: expected 2 system_history rows, got %d", n)
	}

	// Prune snapshots older than 1h, keep minimum 0.
	pruned, err := db.PruneSnapshots(1*time.Hour, 0)
	if err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 snapshot pruned, got %d", pruned)
	}

	// Only the snap-old row should have been removed from each history table.
	// Pre-fix: transaction rolled back → still 2 rows each.
	// Post-fix: 1 row each remains.
	for _, table := range []string{"smart_history", "system_history"} {
		if n := countRows(t, db, table); n != 1 {
			t.Errorf("%s: expected 1 row to remain (rollback bug), got %d", table, n)
		}
	}
	// Snapshots table
	if n := countRows(t, db, "snapshots"); n != 1 {
		t.Errorf("expected 1 remaining snapshot, got %d", n)
	}
}

// TestPruneDiskUsageHistory_DeletesOldRowsOnly — the new dedicated prune path.
func TestPruneDiskUsageHistory_DeletesOldRowsOnly(t *testing.T) {
	db := newTestDB(t)

	now := time.Now()
	// 3 rows: 2 old, 1 recent
	rows := []struct {
		mount string
		ts    time.Time
	}{
		{"/mnt/disk1", now.Add(-400 * 24 * time.Hour)}, // very old
		{"/mnt/disk1", now.Add(-366 * 24 * time.Hour)}, // old (just past 365d)
		{"/mnt/disk1", now.Add(-30 * 24 * time.Hour)},  // recent
	}
	for _, r := range rows {
		if _, err := db.db.Exec(
			`INSERT INTO disk_usage_history (mount_point, label, device, total_gb, used_gb, free_gb, used_pct, timestamp) VALUES (?,?,?,?,?,?,?,?)`,
			r.mount, "label", "dev", 100.0, 50.0, 50.0, 50.0, r.ts,
		); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	cutoff := now.Add(-365 * 24 * time.Hour)
	deleted, err := db.PruneDiskUsageHistory(cutoff)
	if err != nil {
		t.Fatalf("PruneDiskUsageHistory: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}
	if n := countRows(t, db, "disk_usage_history"); n != 1 {
		t.Errorf("expected 1 row remaining, got %d", n)
	}
}

// TestPruneDiskUsageHistory_EmptyTable_NoError — defensive: should work on empty table.
func TestPruneDiskUsageHistory_EmptyTable_NoError(t *testing.T) {
	db := newTestDB(t)

	deleted, err := db.PruneDiskUsageHistory(time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}
