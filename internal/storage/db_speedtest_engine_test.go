package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// TestSpeedTestHistory_EngineColumn_AddedWithDefault asserts the
// schema migration adds an `engine` column to speedtest_history and
// new INSERTs default the value to "ookla_cli". Mirrors the v0.9.10
// borgbackup-bundling pattern: a purely additive column with a
// stable default that pre-switchover rows can adopt without data
// loss. PRD #283 / issue #284.
func TestSpeedTestHistory_EngineColumn_AddedWithDefault(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Confirm the column exists with the right default.
	rows, err := db.db.Query("PRAGMA table_info(speedtest_history)")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	var found bool
	var defaultValue string
	for rows.Next() {
		var cid int
		var name, colType string
		var notnull int
		var dflt *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "engine" {
			found = true
			if dflt != nil {
				defaultValue = *dflt
			}
		}
	}
	if !found {
		t.Fatal("speedtest_history.engine column missing — migration must add it")
	}
	// SQLite reports defaults verbatim. Default literal is 'ookla_cli'
	// (single-quoted).
	if defaultValue != "'ookla_cli'" {
		t.Errorf("engine default = %q, want %q", defaultValue, "'ookla_cli'")
	}
}

// TestSpeedTestHistory_BackfillsExistingRowsWithOoklaCLI asserts that
// rows inserted before the column existed get back-filled with
// "ookla_cli" — the engine in use prior to the v0.9.x switchover.
// PRD #283 user story 15 ("historical chart can mark the
// engine-switchover point").
func TestSpeedTestHistory_BackfillsExistingRowsWithOoklaCLI(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Save a row WITHOUT setting Engine — simulates a pre-#284 row
	// or a result coming back from a runner that didn't stamp the
	// field for some reason. The DB default fills in.
	if err := db.SaveSpeedTest("snap-1", &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: 100, UploadMbps: 10, LatencyMs: 5,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	row := db.db.QueryRow(`SELECT engine FROM speedtest_history WHERE snapshot_id = 'snap-1'`)
	var engine string
	if err := row.Scan(&engine); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if engine != internal.SpeedTestEngineOoklaCLI {
		t.Errorf("engine for unstamped row = %q, want %q (default)", engine, internal.SpeedTestEngineOoklaCLI)
	}
}

// TestSpeedTestHistory_PersistsExplicitEngine asserts a result
// stamped with Engine="speedtest_go" round-trips through the store.
// This is the path the new compositeRunner takes for primary-engine
// results.
func TestSpeedTestHistory_PersistsExplicitEngine(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := db.SaveSpeedTest("snap-2", &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: 500, UploadMbps: 50, LatencyMs: 4,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	row := db.db.QueryRow(`SELECT engine FROM speedtest_history WHERE snapshot_id = 'snap-2'`)
	var engine string
	if err := row.Scan(&engine); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("engine = %q, want %q", engine, internal.SpeedTestEngineSpeedTestGo)
	}
}

// TestSpeedTestHistory_MigrationIdempotent asserts that opening an
// already-migrated DB twice does NOT corrupt the engine column or
// double-add it. The v0.9.10 pattern: purely additive migrations
// have a `IF NOT EXISTS` guard at every layer.
func TestSpeedTestHistory_MigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db1, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := db1.SaveSpeedTest("snap-pre", &internal.SpeedTestResult{
		Timestamp: time.Now(), DownloadMbps: 1, UploadMbps: 1, LatencyMs: 1,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	db1.Close()

	// Re-open. The migration runs again; must be a no-op for the
	// engine column (no PRAGMA error, no data loss).
	db2, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()

	row := db2.db.QueryRow(`SELECT engine FROM speedtest_history WHERE snapshot_id = 'snap-pre'`)
	var engine string
	if err := row.Scan(&engine); err != nil {
		t.Fatalf("scan after re-migration: %v", err)
	}
	if engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("engine after re-migration = %q, want %q (data must not be lost)", engine, internal.SpeedTestEngineSpeedTestGo)
	}

	// And a fresh insert still defaults correctly post-re-migration.
	if err := db2.SaveSpeedTest("snap-post", &internal.SpeedTestResult{
		Timestamp: time.Now(), DownloadMbps: 1, UploadMbps: 1, LatencyMs: 1,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	row2 := db2.db.QueryRow(`SELECT engine FROM speedtest_history WHERE snapshot_id = 'snap-post'`)
	var engine2 string
	if err := row2.Scan(&engine2); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if engine2 != internal.SpeedTestEngineOoklaCLI {
		t.Errorf("engine of post-re-migration row = %q, want %q (default)", engine2, internal.SpeedTestEngineOoklaCLI)
	}
}

// TestGetSpeedTestHistory_IncludesEngine asserts the read API
// surfaces the engine column to the dashboard so the historical
// chart can annotate the switchover point (user story 15).
func TestGetSpeedTestHistory_IncludesEngine(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	now := time.Now()
	if err := db.SaveSpeedTest("snap-old", &internal.SpeedTestResult{
		Timestamp: now.Add(-10 * time.Minute), DownloadMbps: 80, UploadMbps: 8, LatencyMs: 25,
	}); err != nil {
		t.Fatalf("save old: %v", err)
	}
	if err := db.SaveSpeedTest("snap-new", &internal.SpeedTestResult{
		Timestamp: now, DownloadMbps: 500, UploadMbps: 50, LatencyMs: 4,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save new: %v", err)
	}
	pts, err := db.GetSpeedTestHistory(24)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("history len = %d, want 2", len(pts))
	}
	// In timestamp ASC order, the older row comes first.
	if pts[0].Engine != internal.SpeedTestEngineOoklaCLI {
		t.Errorf("pts[0].Engine = %q, want ookla_cli (default for unstamped row)", pts[0].Engine)
	}
	if pts[1].Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("pts[1].Engine = %q, want speedtest_go", pts[1].Engine)
	}
}
