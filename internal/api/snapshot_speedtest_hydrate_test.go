package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #290 (Slice A of #261): /api/v1/snapshot/latest must surface
// the most-recent speedtest_history row inside snap.SpeedTest.Latest
// even when the in-memory snapshot lacks it. The persisted Snapshot
// envelope does not carry SpeedTest forward across restarts (Collect()
// does not populate it; only the speed-test loop does, mutating
// s.latest in place). So after a container restart, a user with
// historical data would see a blank speed-test card on the dashboard
// until the next per-tick speed-test loop fired — which on the
// default ~24h interval is exactly the cold-start UX gap this slice
// closes.

// TestHandleLatestSnapshot_HydratesSpeedTestFromHistory asserts that a
// snapshot persisted WITHOUT a SpeedTest field gets enriched from the
// speedtest_history table on read. The dashboard widget's happy path
// (spd.available && spd.latest) must light up.
func TestHandleLatestSnapshot_HydratesSpeedTestFromHistory(t *testing.T) {
	srv := newSettingsTestServer()

	// Persist a snapshot with no SpeedTest data — simulates the
	// state right after a container restart, before runSpeedTest
	// fires for the first time.
	snap := &internal.Snapshot{
		ID:        "post-restart-snap",
		Timestamp: time.Now().UTC(),
		// SpeedTest deliberately nil
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Seed a historical row that pre-dated the restart.
	histTs := time.Now().Add(-3 * time.Hour).UTC()
	if err := srv.store.SaveSpeedTest("snap-old", &internal.SpeedTestResult{
		Timestamp:    histTs,
		DownloadMbps: 250, UploadMbps: 25, LatencyMs: 12, JitterMs: 2,
		ServerName: "Hetzner",
		ISP:        "Vodafone",
		Engine:     internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/latest returned %d: %s", rec.Code, rec.Body.String())
	}

	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if got.SpeedTest == nil {
		t.Fatal("snap.SpeedTest is nil — handler did not hydrate from speedtest_history")
	}
	if !got.SpeedTest.Available {
		t.Error("snap.SpeedTest.Available = false; widget happy-path gate will skip")
	}
	if got.SpeedTest.Latest == nil {
		t.Fatal("snap.SpeedTest.Latest is nil after hydration")
	}
	if got.SpeedTest.Latest.DownloadMbps != 250 {
		t.Errorf("Latest.DownloadMbps = %v, want 250", got.SpeedTest.Latest.DownloadMbps)
	}
	if got.SpeedTest.Latest.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("Latest.Engine = %q, want %q", got.SpeedTest.Latest.Engine, internal.SpeedTestEngineSpeedTestGo)
	}
	if got.SpeedTest.Latest.Timestamp.Unix() != histTs.Unix() {
		t.Errorf("Latest.Timestamp = %v, want %v", got.SpeedTest.Latest.Timestamp, histTs)
	}
}

// TestHandleLatestSnapshot_PreservesExistingSpeedTest asserts that when
// the in-memory snapshot already carries SpeedTest.Latest (the steady-
// state where the scheduler's loop has fired at least once), the
// hydration path does NOT clobber it. Hydration is a fallback, not an
// override.
func TestHandleLatestSnapshot_PreservesExistingSpeedTest(t *testing.T) {
	srv := newSettingsTestServer()

	freshTs := time.Now().Add(-5 * time.Minute).UTC()
	snap := &internal.Snapshot{
		ID:        "live-snap",
		Timestamp: time.Now().UTC(),
		SpeedTest: &internal.SpeedTestInfo{
			Available: true,
			Latest: &internal.SpeedTestResult{
				Timestamp:    freshTs,
				DownloadMbps: 999, // sentinel — must survive
				UploadMbps:   99,
				LatencyMs:    1,
				Engine:       internal.SpeedTestEngineSpeedTestGo,
			},
		},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Stale history row that must NOT win against the live snapshot's
	// Latest field. This simulates the steady state.
	if err := srv.store.SaveSpeedTest("stale", &internal.SpeedTestResult{
		Timestamp:    time.Now().Add(-10 * time.Hour).UTC(),
		DownloadMbps: 1, // very different from the sentinel
	}); err != nil {
		t.Fatalf("SaveSpeedTest stale: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SpeedTest == nil || got.SpeedTest.Latest == nil {
		t.Fatal("SpeedTest.Latest dropped during enrichment")
	}
	if got.SpeedTest.Latest.DownloadMbps != 999 {
		t.Errorf("DownloadMbps = %v, want 999 (live snapshot value should win over stale history row)", got.SpeedTest.Latest.DownloadMbps)
	}
}

// TestHandleLatestSnapshot_NoHistoryNoEnrichment asserts the genuine
// empty-state path: no in-memory SpeedTest AND no history rows means
// the response carries no speed_test field, preserving the v0.9.6
// #210 first-boot empty-state copy on the dashboard widget.
func TestHandleLatestSnapshot_NoHistoryNoEnrichment(t *testing.T) {
	srv := newSettingsTestServer()

	snap := &internal.Snapshot{
		ID:        "fresh-install",
		Timestamp: time.Now().UTC(),
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SpeedTest != nil {
		t.Errorf("SpeedTest = %+v on fresh install, want nil — empty-state copy on the dashboard would be skipped", got.SpeedTest)
	}
}

// TestHandleLatestSnapshot_HydrationPreservesLastAttempt asserts that
// when the in-memory snapshot has SpeedTest.LastAttempt set (e.g.
// status=failed) but no Latest, hydration adds Latest from history
// without dropping the LastAttempt signal. Both fields ride together.
func TestHandleLatestSnapshot_HydrationPreservesLastAttempt(t *testing.T) {
	srv := newSettingsTestServer()

	attemptTs := time.Now().Add(-1 * time.Minute).UTC()
	snap := &internal.Snapshot{
		ID:        "post-restart-with-attempt",
		Timestamp: time.Now().UTC(),
		SpeedTest: &internal.SpeedTestInfo{
			LastAttempt: &internal.SpeedTestAttempt{
				Timestamp: attemptTs,
				Status:    "failed",
				ErrorMsg:  "timeout",
			},
		},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := srv.store.SaveSpeedTest("ok", &internal.SpeedTestResult{
		Timestamp:    time.Now().Add(-2 * time.Hour).UTC(),
		DownloadMbps: 500, UploadMbps: 50, LatencyMs: 5,
		Engine: internal.SpeedTestEngineOoklaCLI,
	}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got internal.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SpeedTest == nil || got.SpeedTest.Latest == nil {
		t.Fatal("SpeedTest.Latest not hydrated")
	}
	if got.SpeedTest.LastAttempt == nil {
		t.Fatal("SpeedTest.LastAttempt dropped during hydration")
	}
	if got.SpeedTest.LastAttempt.Status != "failed" {
		t.Errorf("LastAttempt.Status = %q, want failed", got.SpeedTest.LastAttempt.Status)
	}
}
