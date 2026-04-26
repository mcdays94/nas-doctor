package storage

import (
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// TestSpeedTestSamples_Schema asserts the migration creates the
// speedtest_samples table with the expected columns + the index. PRD
// #283 slice 3 / issue #286. Treats the SQL CREATE TABLE statement as
// the source-of-truth and pins it via PRAGMA table_info so a future
// refactor that drops a column or renames the index gets caught at
// test-time.
func TestSpeedTestSamples_Schema(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	rows, err := db.db.Query("PRAGMA table_info(speedtest_samples)")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	want := map[string]string{
		"test_id":      "INTEGER",
		"sample_index": "INTEGER",
		"phase":        "TEXT",
		"ts":           "TIMESTAMP",
		"mbps":         "REAL",
		"latency_ms":   "REAL",
	}
	got := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notnull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[name] = colType
	}
	for col, typ := range want {
		if got[col] != typ {
			t.Errorf("speedtest_samples.%s column type = %q, want %q", col, got[col], typ)
		}
	}

	// Index named per the migration.
	idxRows, err := db.db.Query("PRAGMA index_list(speedtest_samples)")
	if err != nil {
		t.Fatalf("index_list: %v", err)
	}
	defer idxRows.Close()
	foundIdx := false
	for idxRows.Next() {
		var seq int
		var name, origin string
		var unique, partial int
		if err := idxRows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan idx: %v", err)
		}
		if name == "speedtest_samples_test_id" {
			foundIdx = true
		}
	}
	if !foundIdx {
		t.Errorf("speedtest_samples_test_id index missing from speedtest_samples")
	}
}

// TestSpeedTestSamples_InsertAndRetrieve_PreservesOrder asserts
// InsertSpeedTestSamples + GetSpeedTestSamples round-trip and that the
// retrieval is ordered by sample_index ascending. The mini-chart on
// /service-checks renders left-to-right; an unordered retrieval would
// produce a chart with backwards-jumping samples that the chart
// library would render as crossed line segments.
func TestSpeedTestSamples_InsertAndRetrieve_PreservesOrder(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Parent history row (FK target).
	id, err := db.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 100, UploadMbps: 10, LatencyMs: 5,
		Timestamp: time.Now(), Engine: internal.SpeedTestEngineSpeedTestGo,
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero history ID")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	samples := []SpeedTestSample{
		{SampleIndex: 0, Phase: "latency", Timestamp: now.Add(0 * time.Second), LatencyMs: 8.2},
		{SampleIndex: 1, Phase: "latency", Timestamp: now.Add(1 * time.Second), LatencyMs: 9.1},
		{SampleIndex: 2, Phase: "download", Timestamp: now.Add(2 * time.Second), Mbps: 421},
		{SampleIndex: 3, Phase: "download", Timestamp: now.Add(3 * time.Second), Mbps: 723},
		{SampleIndex: 4, Phase: "upload", Timestamp: now.Add(4 * time.Second), Mbps: 88},
	}
	if err := db.InsertSpeedTestSamples(id, samples); err != nil {
		t.Fatalf("InsertSpeedTestSamples: %v", err)
	}

	got, err := db.GetSpeedTestSamples(id)
	if err != nil {
		t.Fatalf("GetSpeedTestSamples: %v", err)
	}
	if len(got) != len(samples) {
		t.Fatalf("len(samples) = %d, want %d", len(got), len(samples))
	}
	for i, s := range got {
		if s.SampleIndex != i {
			t.Errorf("samples[%d].SampleIndex = %d, want %d (order not preserved)", i, s.SampleIndex, i)
		}
	}
	if got[0].Phase != "latency" || got[2].Phase != "download" || got[4].Phase != "upload" {
		t.Errorf("phase ordering broken: %+v", got)
	}
	if got[3].Mbps != 723 {
		t.Errorf("Mbps round-trip failed: got %f, want 723", got[3].Mbps)
	}
}

// TestSpeedTestSamples_CascadeDeleteFromHistory asserts that deleting
// the parent speedtest_history row drops the linked samples via the
// FK ON DELETE CASCADE. Without this, the prune cycle would orphan
// rows in speedtest_samples forever. Issue #286 acceptance criterion.
func TestSpeedTestSamples_CascadeDeleteFromHistory(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	id, err := db.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 100, Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}
	if err := db.InsertSpeedTestSamples(id, []SpeedTestSample{
		{SampleIndex: 0, Phase: "download", Timestamp: time.Now(), Mbps: 100},
		{SampleIndex: 1, Phase: "download", Timestamp: time.Now(), Mbps: 200},
	}); err != nil {
		t.Fatalf("InsertSpeedTestSamples: %v", err)
	}
	// Sanity: pre-delete count.
	pre, _ := db.GetSpeedTestSamples(id)
	if len(pre) != 2 {
		t.Fatalf("expected 2 samples pre-delete, got %d", len(pre))
	}

	// Delete the parent row.
	if _, err := db.db.Exec("DELETE FROM speedtest_history WHERE id = ?", id); err != nil {
		t.Fatalf("delete history: %v", err)
	}

	// Samples must now be gone.
	post, err := db.GetSpeedTestSamples(id)
	if err != nil {
		t.Fatalf("GetSpeedTestSamples post-delete: %v", err)
	}
	if len(post) != 0 {
		t.Errorf("FK ON DELETE CASCADE did not fire: %d samples remain", len(post))
	}
}

// TestSpeedTestSamples_FKConstraint_RejectsMissingParent asserts that
// inserting samples for a test_id with no matching speedtest_history
// row fails. This guards against a runner-stage bug where samples
// would be flushed before the parent history row was written —
// without the FK, those samples would silently accumulate.
func TestSpeedTestSamples_FKConstraint_RejectsMissingParent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	err := db.InsertSpeedTestSamples(99999, []SpeedTestSample{
		{SampleIndex: 0, Phase: "download", Timestamp: time.Now(), Mbps: 100},
	})
	if err == nil {
		t.Fatal("expected FK error for missing parent test_id, got nil")
	}
}

// TestSpeedTestSamples_PrimaryKeyConstraint_RejectsDuplicateIndex
// asserts that re-inserting an already-stored (test_id, sample_index)
// pair fails. Catches double-insert bugs in the scheduler's
// completion handler.
func TestSpeedTestSamples_PrimaryKeyConstraint_RejectsDuplicateIndex(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	id, err := db.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 100, Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}
	if err := db.InsertSpeedTestSamples(id, []SpeedTestSample{
		{SampleIndex: 0, Phase: "download", Timestamp: time.Now(), Mbps: 100},
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err = db.InsertSpeedTestSamples(id, []SpeedTestSample{
		{SampleIndex: 0, Phase: "download", Timestamp: time.Now(), Mbps: 999},
	})
	if err == nil {
		t.Fatal("expected PK error on duplicate (test_id, sample_index), got nil")
	}
}

// TestSpeedTestSamples_GetUnknownTestID_ReturnsEmpty asserts that
// querying for samples on a test_id that doesn't exist returns an
// empty slice rather than an error. The /api/v1/speedtest/samples/{id}
// HTTP handler distinguishes "test exists but has no samples" (legacy
// row, return empty array + 200) from "test_id unknown" (404), and
// this method is the building block for both cases.
func TestSpeedTestSamples_GetUnknownTestID_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	got, err := db.GetSpeedTestSamples(7777777)
	if err != nil {
		t.Fatalf("GetSpeedTestSamples: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// TestSpeedTestSamples_MigrationIdempotent asserts that running the
// migration on an already-migrated DB is a no-op. Mirrors the v0.9.9
// V2a shape-sentinel pattern (idempotent migrations) — for SQL the
// equivalent is `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT
// EXISTS` which the table definition already uses; this test just
// pins the contract.
func TestSpeedTestSamples_MigrationIdempotent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	// migrate is called by Open; running it again should not error
	// (CREATE TABLE IF NOT EXISTS is idempotent).
	if err := db.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	// And insert/retrieve must still work afterwards.
	id, err := db.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 100, Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}
	if err := db.InsertSpeedTestSamples(id, []SpeedTestSample{
		{SampleIndex: 0, Phase: "download", Timestamp: time.Now(), Mbps: 100},
	}); err != nil {
		t.Fatalf("InsertSpeedTestSamples post-2nd-migration: %v", err)
	}
}

// TestServiceChecksHistory_SpeedTestHistoryIDColumn asserts the v0.9.11
// migration adds the speedtest_history_id column to
// service_checks_history and SaveServiceCheckResults persists it.
// This is the linkage the /service-checks expanded-log mini-chart
// uses to fetch /api/v1/speedtest/samples/{id}. Issue #286.
func TestServiceChecksHistory_SpeedTestHistoryIDColumn(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Column exists.
	rows, err := db.db.Query("PRAGMA table_info(service_checks_history)")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notnull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "speedtest_history_id" {
			found = true
			if colType != "INTEGER" {
				t.Errorf("speedtest_history_id type = %q, want INTEGER", colType)
			}
		}
	}
	if !found {
		t.Fatal("service_checks_history.speedtest_history_id column missing")
	}

	// SaveServiceCheckResults persists the value, ListLatestServiceChecks
	// returns it.
	checked := time.Now().UTC().Format(time.RFC3339)
	if err := db.SaveServiceCheckResults([]internal.ServiceCheckResult{
		{
			Key: "k1", Name: "Internet Speed", Type: "speed", Target: "speedtest",
			Status: "up", ResponseMS: 5,
			CheckedAt: checked, FailureThreshold: 1, FailureSeverity: "warning",
			SpeedTestHistoryID: 42,
		},
	}); err != nil {
		t.Fatalf("SaveServiceCheckResults: %v", err)
	}
	entries, err := db.ListLatestServiceChecks(10)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].SpeedTestHistoryID != 42 {
		t.Errorf("entries[0].SpeedTestHistoryID = %d, want 42", entries[0].SpeedTestHistoryID)
	}

	// And history endpoint returns it too.
	hist, err := db.GetServiceCheckHistory("k1", 10)
	if err != nil {
		t.Fatalf("GetServiceCheckHistory: %v", err)
	}
	if len(hist) != 1 || hist[0].SpeedTestHistoryID != 42 {
		t.Errorf("history SpeedTestHistoryID = %v, want 42", hist)
	}
}
