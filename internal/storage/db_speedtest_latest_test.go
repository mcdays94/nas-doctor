package storage

import (
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// Issue #290 (Slice A of #261): the dashboard speed-test card needs
// to render the most-recent successful test even when the in-memory
// scheduler cache (s.latest.SpeedTest) is empty — typically right
// after a container restart, before the per-tick speedtest loop has
// fired again. The Snapshot envelope persisted via SaveSnapshot does
// NOT carry SpeedTest forward (Collect() does not populate it; only
// the speed-test loop does, mutating s.latest in place). So the
// truth source is the speedtest_history table itself.
//
// GetLatestSpeedTestResult is the cheap "give me the most-recent row
// shaped as an internal.SpeedTestResult" hop the API layer needs to
// hydrate snap.SpeedTest.Latest on /api/v1/snapshot/latest when the
// in-memory snapshot lacks it.

// TestGetLatestSpeedTestResult_EmptyStoreReturnsNoRow asserts an empty
// speedtest_history is reported via (nil, false, nil) so callers can
// distinguish "no row" from "error" without an err string match.
func TestGetLatestSpeedTestResult_EmptyStoreReturnsNoRow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	got, ok, err := db.GetLatestSpeedTestResult()
	if err != nil {
		t.Fatalf("GetLatestSpeedTestResult on empty DB: %v", err)
	}
	if ok {
		t.Errorf("ok = true on empty DB, want false")
	}
	if got != nil {
		t.Errorf("result = %+v on empty DB, want nil", got)
	}
}

// TestGetLatestSpeedTestResult_ReturnsMostRecentRow asserts that with
// multiple history rows the lookup returns the one with the highest
// timestamp shaped as an internal.SpeedTestResult, including the
// engine column (carried from #284 / Slice 1).
func TestGetLatestSpeedTestResult_ReturnsMostRecentRow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	older := time.Now().Add(-2 * time.Hour).UTC()
	newer := time.Now().Add(-1 * time.Hour).UTC()

	if err := db.SaveSpeedTest("snap-old", &internal.SpeedTestResult{
		Timestamp:    older,
		DownloadMbps: 80, UploadMbps: 8, LatencyMs: 30, JitterMs: 4,
		ServerName: "Old Server", ISP: "OldISP",
		Engine: internal.SpeedTestEngineOoklaCLI,
	}); err != nil {
		t.Fatalf("save older: %v", err)
	}
	if err := db.SaveSpeedTest("snap-new", &internal.SpeedTestResult{
		Timestamp:    newer,
		DownloadMbps: 200, UploadMbps: 20, LatencyMs: 10, JitterMs: 1,
		ServerName: "New Server", ISP: "NewISP",
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save newer: %v", err)
	}

	got, ok, err := db.GetLatestSpeedTestResult()
	if err != nil {
		t.Fatalf("GetLatestSpeedTestResult: %v", err)
	}
	if !ok {
		t.Fatal("ok = false with rows present")
	}
	if got == nil {
		t.Fatal("result = nil with rows present")
	}
	if got.DownloadMbps != 200 {
		t.Errorf("DownloadMbps = %v, want 200 (newer row)", got.DownloadMbps)
	}
	if got.UploadMbps != 20 {
		t.Errorf("UploadMbps = %v, want 20", got.UploadMbps)
	}
	if got.LatencyMs != 10 {
		t.Errorf("LatencyMs = %v, want 10", got.LatencyMs)
	}
	if got.JitterMs != 1 {
		t.Errorf("JitterMs = %v, want 1", got.JitterMs)
	}
	if got.ServerName != "New Server" {
		t.Errorf("ServerName = %q, want %q", got.ServerName, "New Server")
	}
	if got.ISP != "NewISP" {
		t.Errorf("ISP = %q, want %q", got.ISP, "NewISP")
	}
	if got.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("Engine = %q, want %q", got.Engine, internal.SpeedTestEngineSpeedTestGo)
	}
	// Timestamp round-trips at second resolution (SQLite stores via
	// time.Time -> RFC3339); compare via Unix to avoid timezone fuzz.
	if got.Timestamp.Unix() != newer.Unix() {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, newer)
	}
}
