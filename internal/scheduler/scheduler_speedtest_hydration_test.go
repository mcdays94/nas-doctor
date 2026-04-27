package scheduler

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #294 R1+R2: Prometheus must see a hydrated snapshot even when
// the scheduler's in-memory s.latest.SpeedTest is nil but a historical
// row exists in speedtest_history. Prior to this fix the API handler
// was the ONLY place that hydrated, leaving Prometheus stuck reading
// 0 / missing labels after a fresh container start.
//
// This test pins the contract: Scheduler.Latest() returns a snapshot
// whose SpeedTest.Latest is hydrated from history when needed, AND
// Prometheus's Update() is fed the hydrated snapshot during RunOnce.

// TestScheduler_Latest_HydratesSpeedTestFromHistory asserts that after
// a fresh container start (s.latest populated via SetLatest with a
// snapshot that has no SpeedTest), Latest() returns a snapshot where
// SpeedTest.Latest has been hydrated from speedtest_history.
func TestScheduler_Latest_HydratesSpeedTestFromHistory(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()

	// Seed a historical row.
	histTs := time.Now().Add(-2 * time.Hour).UTC()
	if err := store.SaveSpeedTest("snap-pre-restart", &internal.SpeedTestResult{
		Timestamp:    histTs,
		DownloadMbps: 250, UploadMbps: 25, LatencyMs: 12,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)

	// Simulate post-restart: cache holds a snapshot that lacks
	// SpeedTest (the persisted Snapshot envelope doesn't carry
	// it forward — only the speed-test loop populates it).
	s.SetLatest(&internal.Snapshot{
		ID:        "post-restart",
		Timestamp: time.Now().UTC(),
	})

	got := s.Latest()
	if got == nil {
		t.Fatal("Latest() returned nil")
	}
	if got.SpeedTest == nil || got.SpeedTest.Latest == nil {
		t.Fatalf("Latest().SpeedTest.Latest = nil; want hydrated from history. got=%+v", got.SpeedTest)
	}
	if got.SpeedTest.Latest.DownloadMbps != 250 {
		t.Errorf("DownloadMbps = %v, want 250", got.SpeedTest.Latest.DownloadMbps)
	}
	if got.SpeedTest.Latest.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("Engine = %q, want speedtest_go", got.SpeedTest.Latest.Engine)
	}
	if !got.SpeedTest.Available {
		t.Error("Available = false; widget happy-path gate skips")
	}
}

// TestScheduler_Latest_PreservesExistingSpeedTest asserts hydration is
// a fallback — when the cached snapshot already carries Latest, the
// stored history row must not clobber it.
func TestScheduler_Latest_PreservesExistingSpeedTest(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	if err := store.SaveSpeedTest("stale", &internal.SpeedTestResult{
		Timestamp:    time.Now().Add(-10 * time.Hour).UTC(),
		DownloadMbps: 1, // sentinel — must NOT win
	}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	s := New(nil, store, nil, nil, quietLogger(), time.Hour)
	s.SetLatest(&internal.Snapshot{
		Timestamp: time.Now().UTC(),
		SpeedTest: &internal.SpeedTestInfo{
			Available: true,
			Latest: &internal.SpeedTestResult{
				Timestamp:    time.Now().Add(-1 * time.Minute).UTC(),
				DownloadMbps: 999, // sentinel — must survive
				Engine:       internal.SpeedTestEngineSpeedTestGo,
			},
		},
	})

	got := s.Latest()
	if got.SpeedTest == nil || got.SpeedTest.Latest == nil {
		t.Fatal("Latest dropped during hydration")
	}
	if got.SpeedTest.Latest.DownloadMbps != 999 {
		t.Errorf("DownloadMbps = %v, want 999 (live data must beat stale history)", got.SpeedTest.Latest.DownloadMbps)
	}
}

// TestScheduler_Latest_NilSnapshotReturnsNil asserts the no-cache path
// is unaffected by the new hydration logic.
func TestScheduler_Latest_NilSnapshotReturnsNil(t *testing.T) {
	t.Parallel()
	s := New(nil, storage.NewFakeStore(), nil, nil, quietLogger(), time.Hour)
	if got := s.Latest(); got != nil {
		t.Errorf("Latest() = %+v on empty cache, want nil", got)
	}
}

// TestPrometheus_SeesHydratedSpeedTest_AfterRestart wires a real
// notifier.Metrics into the Scheduler and asserts that scraping
// /metrics after a "post-restart" SetLatest exposes the speedtest
// gauges with values from the historical row. This is the explicit
// regression guard for #294 R1+R2: Prometheus reads the hydrated
// snapshot through the same code path the API handler uses.
func TestPrometheus_SeesHydratedSpeedTest_AfterRestart(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()

	// Seed history so hydration has something to find.
	if err := store.SaveSpeedTest("pre-restart-row", &internal.SpeedTestResult{
		Timestamp:    time.Now().Add(-3 * time.Hour).UTC(),
		DownloadMbps: 92.10, UploadMbps: 8.50, LatencyMs: 14.2,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	m := notifier.NewMetrics()
	s := New(nil, store, nil, m, quietLogger(), time.Hour)

	// Simulate post-restart: cache snapshot lacks SpeedTest.
	postRestart := &internal.Snapshot{
		ID:        "post-restart",
		Timestamp: time.Now().UTC(),
	}
	s.SetLatest(postRestart)

	// Push the hydrated snapshot through the Prometheus exporter
	// the way RunOnce does. After this fix, Latest() returns the
	// hydrated snapshot, so feeding that into Update() must surface
	// the speedtest gauges.
	m.Update(s.Latest())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}).ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="speedtest_go"} 1`) {
		t.Errorf("expected nasdoctor_speedtest_engine{engine=\"speedtest_go\"} 1 in /metrics; body excerpt:\n%s",
			grepLines(body, "speedtest"))
	}
	if !strings.Contains(body, "nasdoctor_speedtest_download_mbps 92.1") {
		t.Errorf("expected nasdoctor_speedtest_download_mbps 92.1 in /metrics; body excerpt:\n%s",
			grepLines(body, "speedtest_download"))
	}
}

// grepLines returns a string containing only the lines of body that
// contain substr. Used to keep failure messages readable.
func grepLines(body, substr string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, substr) {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}
