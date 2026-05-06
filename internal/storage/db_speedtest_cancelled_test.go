package storage

// Issue #304 — pin the speedtest_history.status migration + the
// SaveSpeedTestCancelledReturningID code path.
//
// Two properties:
//
//  1. Schema migration is idempotent — calling it twice is a no-op.
//     The ensureColumn helper is supposed to handle this but the
//     test gives us a regression guard.
//
//  2. SaveSpeedTestCancelledReturningID inserts a row with
//     status='cancelled', and GetLatestSpeedTestResult skips it
//     (so a cancelled mid-test row never becomes the dashboard's
//     "Latest" widget content).

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

func TestSpeedTestHistory_StatusColumn_DefaultsToSuccess(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Insert a normal success row via the existing path. Pre-#304
	// callers are unaware of the status column; the schema's
	// DEFAULT 'success' must back-fill it.
	id, err := db.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: 100,
		UploadMbps:   50,
		LatencyMs:    8,
		Engine:       internal.SpeedTestEngineSpeedTestGo,
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}

	var status string
	if err := db.db.QueryRow(`SELECT status FROM speedtest_history WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want 'success'", status)
	}
}

func TestSaveSpeedTestCancelledReturningID_StampsCancelledStatus(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	id, err := db.SaveSpeedTestCancelledReturningID(
		"snap-cancel",
		time.Now().UTC(),
		internal.SpeedTestEngineSpeedTestGo,
	)
	if err != nil {
		t.Fatalf("SaveSpeedTestCancelledReturningID: %v", err)
	}
	if id == 0 {
		t.Fatal("returned id = 0, want non-zero")
	}

	var status string
	if err := db.db.QueryRow(`SELECT status FROM speedtest_history WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "cancelled" {
		t.Errorf("status = %q, want 'cancelled'", status)
	}
}

func TestGetLatestSpeedTestResult_SkipsCancelledRows(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Insert a successful row first.
	successTS := time.Now().Add(-2 * time.Minute)
	if _, err := db.SaveSpeedTestReturningID("snap-s", &internal.SpeedTestResult{
		Timestamp:    successTS,
		DownloadMbps: 100,
		UploadMbps:   50,
		LatencyMs:    8,
		Engine:       internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save success row: %v", err)
	}

	// Then a more-recent cancelled row. Without the status filter
	// in GetLatestSpeedTestResult, the cancelled row would surface
	// as the "Latest" widget content — wrong.
	cancelTS := time.Now()
	if _, err := db.SaveSpeedTestCancelledReturningID("snap-c", cancelTS, internal.SpeedTestEngineSpeedTestGo); err != nil {
		t.Fatalf("save cancelled row: %v", err)
	}

	res, ok, err := db.GetLatestSpeedTestResult()
	if err != nil {
		t.Fatalf("GetLatestSpeedTestResult: %v", err)
	}
	if !ok {
		t.Fatal("ok = false; expected the success row to surface")
	}
	// Compare timestamps via Unix seconds — DB roundtrip can lose
	// sub-second precision depending on the SQLite driver config.
	if res.Timestamp.Unix() != successTS.Unix() {
		t.Errorf("Latest result picked the wrong row: got ts=%v, want ts=%v (cancelled row should have been skipped)",
			res.Timestamp, successTS)
	}
	if res.DownloadMbps == 0 {
		t.Error("returned cancelled row's zero DownloadMbps; cancelled rows should be filtered out")
	}
}

func TestSpeedTestHistory_StatusMigrationIdempotent(t *testing.T) {
	t.Parallel()
	// Open the DB twice in succession. The second open re-runs every
	// migration step including ensureColumn for status; if any are
	// non-idempotent the second open errors out (or the schema gets
	// corrupted). Catches issue #304 regressions that might
	// accidentally introduce a non-idempotent ALTER TABLE alongside
	// the status column add.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db1, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	// Insert a row first to verify it survives the second migration.
	if _, err := db1.SaveSpeedTestReturningID("snap", &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: 100,
		Engine:       internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save row: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	db2, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("second open: %v (migrations not idempotent?)", err)
	}
	defer db2.Close()
	// Confirm the row survived + still has status='success'.
	var status string
	if err := db2.db.QueryRow(`SELECT status FROM speedtest_history ORDER BY id DESC LIMIT 1`).Scan(&status); err != nil {
		t.Fatalf("query post-second-open: %v", err)
	}
	if status != "success" {
		t.Errorf("status after second migration = %q, want 'success'", status)
	}
}
