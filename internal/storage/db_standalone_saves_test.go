package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// ── Standalone collection loop saves ──

// TestSaveContainerStats_NoMatchingSnapshot_Succeeds reproduces #155: the
// 5-minute container stats loop uses synthetic "cstats-<ms>" snapshot IDs
// that don't match any row in `snapshots`. Before the fix, the FK constraint
// rejected every INSERT with "FOREIGN KEY constraint failed (787)"; after,
// the save succeeds because the FK was dropped.
func TestSaveContainerStats_NoMatchingSnapshot_Succeeds(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	docker := &internal.DockerInfo{
		Available: true,
		Containers: []internal.ContainerInfo{
			{
				ID: "c1", Name: "web", Image: "nginx:latest", State: "running",
				CPU: 12.5, MemMB: 128, MemPct: 4.2,
				NetIn: 1024, NetOut: 2048,
			},
		},
	}

	// No snapshot rows exist — this is the exact hardware scenario.
	if err := db.SaveContainerStats(docker); err != nil {
		t.Fatalf("SaveContainerStats: %v (regression of #155 — FK must stay dropped)", err)
	}

	history, err := db.GetContainerHistory(1)
	if err != nil {
		t.Fatalf("GetContainerHistory: %v", err)
	}
	found := false
	for _, p := range history {
		if p.Name == "web" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("history missing container 'web'; got %d points total", len(history))
	}
}

// TestSaveSpeedTest_NoMatchingSnapshot_Succeeds is the speedtest twin of the
// above: the scheduler's independent speedtest loop uses "speedtest-<ts>"
// IDs that aren't in `snapshots`. FK removal lets the save succeed.
func TestSaveSpeedTest_NoMatchingSnapshot_Succeeds(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	result := &internal.SpeedTestResult{
		DownloadMbps: 94.0,
		UploadMbps:   93.0,
		LatencyMs:    3.5,
		ServerName:   "example-pop",
		ISP:          "Example ISP",
		Timestamp:    time.Now().UTC(),
	}

	if err := db.SaveSpeedTest("speedtest-20260420-180000", result); err != nil {
		t.Fatalf("SaveSpeedTest: %v (regression of #155)", err)
	}

	// Count directly rather than going through GetSpeedTestHistory, which
	// filters on a time window. The row existing at all is what proves the
	// FK removal works — window filtering is unrelated to #155.
	var n int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM speedtest_history WHERE snapshot_id = ?`, "speedtest-20260420-180000").Scan(&n); err != nil {
		t.Fatalf("count speedtest_history: %v", err)
	}
	if n != 1 {
		t.Fatalf("speedtest_history row count: got %d, want 1 (regression of #155)", n)
	}
}

// TestMigration_DropSnapshotFK_PreservesData proves the FK-dropping migration
// is safe for existing users: we simulate the pre-fix schema by rebuilding
// the two tables WITH the old FK, seeding a couple of rows against a valid
// parent snapshot, then running the migration helper and verifying every
// row survived.
func TestMigration_DropSnapshotFK_PreservesData(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Force the pre-fix schema shape by rebuilding both tables WITH the FK.
	for _, table := range []string{"container_stats_history", "speedtest_history"} {
		if _, err := db.db.Exec(fmt.Sprintf(`DROP TABLE %s`, table)); err != nil {
			t.Fatalf("drop %s: %v", table, err)
		}
	}
	if _, err := db.db.Exec(`CREATE TABLE container_stats_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
		container_id TEXT NOT NULL,
		name TEXT NOT NULL,
		image TEXT,
		cpu_pct REAL, mem_mb REAL, mem_pct REAL,
		net_in_bytes REAL, net_out_bytes REAL,
		block_read_bytes REAL, block_write_bytes REAL,
		timestamp DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("recreate container_stats_history with FK: %v", err)
	}
	if _, err := db.db.Exec(`CREATE TABLE speedtest_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
		download_mbps REAL, upload_mbps REAL,
		latency_ms REAL, jitter_ms REAL,
		server_name TEXT, isp TEXT,
		timestamp DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("recreate speedtest_history with FK: %v", err)
	}

	// Seed a valid parent snapshot + one row in each child table so the FK is happy.
	snapID := "scan-seed"
	ts := time.Now().UTC()
	if _, err := db.db.Exec(`INSERT INTO snapshots (id, timestamp, duration_seconds, data) VALUES (?, ?, 0, ?)`, snapID, ts, `{}`); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if _, err := db.db.Exec(`INSERT INTO container_stats_history (snapshot_id, container_id, name, image, cpu_pct, mem_mb, timestamp) VALUES (?, 'c1', 'web', 'nginx', 10.0, 100.0, ?)`, snapID, ts); err != nil {
		t.Fatalf("seed container_stats_history: %v", err)
	}
	if _, err := db.db.Exec(`INSERT INTO speedtest_history (snapshot_id, download_mbps, upload_mbps, timestamp) VALUES (?, 50.0, 25.0, ?)`, snapID, ts); err != nil {
		t.Fatalf("seed speedtest_history: %v", err)
	}

	// Run the migration helper for each table. Idempotent: second call must no-op.
	for i := 0; i < 2; i++ {
		if err := db.dropSnapshotFKIfPresent("container_stats_history"); err != nil {
			t.Fatalf("pass %d: drop FK container_stats_history: %v", i, err)
		}
		if err := db.dropSnapshotFKIfPresent("speedtest_history"); err != nil {
			t.Fatalf("pass %d: drop FK speedtest_history: %v", i, err)
		}
	}

	// Rows must still exist.
	var cnt int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM container_stats_history`).Scan(&cnt); err != nil || cnt != 1 {
		t.Errorf("container_stats_history rows after migration: got %d, want 1 (err=%v)", cnt, err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM speedtest_history`).Scan(&cnt); err != nil || cnt != 1 {
		t.Errorf("speedtest_history rows after migration: got %d, want 1 (err=%v)", cnt, err)
	}

	// FK must be gone.
	for _, table := range []string{"container_stats_history", "speedtest_history"} {
		rows, err := db.db.Query(fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, table))
		if err != nil {
			t.Fatalf("pragma foreign_key_list(%s): %v", table, err)
		}
		if rows.Next() {
			t.Errorf("%s still has a foreign_key after migration", table)
		}
		rows.Close()
	}

	// Indexes must still exist (migration recreates them).
	for _, idx := range []string{"idx_container_stats_ts", "idx_container_stats_name", "idx_speedtest_ts"} {
		var name string
		if err := db.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&name); err != nil {
			t.Errorf("missing index %s after migration: %v", idx, err)
		}
	}

	// After the FK is dropped, synthetic-ID saves must now work.
	if err := db.SaveContainerStats(&internal.DockerInfo{
		Available: true,
		Containers: []internal.ContainerInfo{
			{ID: "c2", Name: "web2", State: "running", CPU: 1, MemMB: 1},
		},
	}); err != nil {
		t.Errorf("SaveContainerStats after migration: %v", err)
	}
}

// openTestDB creates a fresh SQLite database file under t.TempDir() so each
// test has an isolated schema. The caller owns Close().
func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}
