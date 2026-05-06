package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #305: prior to this fix, /metrics read s.latest directly with
// no fallback, so any gauge derived from s.latest reported zero/missing
// values between container restart and the first /api/v1/snapshot/latest
// hit (which lazily hydrated). The fix extracts a shared
// ensureLatestSnapshot helper that's called by BOTH the API path AND the
// /metrics handler, closing the post-restart silent window.
//
// These tests pin the helper's contract:
//   1. Hydration from disk when scheduler cache is empty.
//   2. Concurrent-safety under -race when N goroutines call simultaneously.
//   3. /metrics post-restart exposes gauges that derive from s.latest
//      WITHOUT any prior API hit.
//   4. /metrics post-restart exposes the speedtest engine gauge family
//      (the specific subset that originally surfaced this class of bug
//      in v0.9.11-rc1/rc2 UAT).

// quietLoggerForEnsureTest produces a logger that swallows debug noise
// during the test runs so failures aren't buried in scheduler chatter.
func quietLoggerForEnsureTest() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newServerWithSchedulerForEnsureTest builds a Server wired through a
// real Scheduler + FakeStore + (optionally) Metrics, mirroring the
// production composition in cmd/nas-doctor/main.go. Returns the
// Server, store, scheduler and metrics so individual tests can seed
// pre-restart state via the store before exercising ensureLatestSnapshot.
func newServerWithSchedulerForEnsureTest(t *testing.T, withMetrics bool) (*Server, *storage.FakeStore, *scheduler.Scheduler, *notifier.Metrics) {
	t.Helper()
	store := storage.NewFakeStore()
	logger := quietLoggerForEnsureTest()
	col := collector.New(internal.HostPaths{}, logger)
	var m *notifier.Metrics
	if withMetrics {
		m = notifier.NewMetrics()
	}
	sched := scheduler.New(col, store, &notifier.Notifier{}, m, logger, time.Hour)
	srv := &Server{
		store:     store,
		scheduler: sched,
		collector: col,
		metrics:   m,
		logger:    logger,
		version:   "test",
		startTime: time.Now(),
	}
	return srv, store, sched, m
}

// TestEnsureLatestSnapshot_HydratesFromDisk: when the scheduler cache
// is empty (post-restart) and a snapshot exists on disk, calling
// ensureLatestSnapshot must populate s.latest from disk and return it.
// Subsequent calls must hit the cache (don't re-read disk on every
// scrape). #305 root cause regression guard.
func TestEnsureLatestSnapshot_HydratesFromDisk(t *testing.T) {
	t.Parallel()
	srv, store, sched, _ := newServerWithSchedulerForEnsureTest(t, false)

	// Persist a snapshot pre-restart.
	preRestart := &internal.Snapshot{
		ID:        "pre-restart",
		Timestamp: time.Now().Add(-30 * time.Minute).UTC(),
		System: internal.SystemInfo{
			Hostname: "tower",
			Platform: "unraid",
		},
	}
	if err := store.SaveSnapshot(preRestart); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Simulate post-restart: scheduler cache is empty.
	if got := sched.Latest(); got != nil {
		t.Fatalf("precondition: scheduler.Latest() = %+v on fresh start, want nil", got)
	}

	got := srv.ensureLatestSnapshot()
	if got == nil {
		t.Fatal("ensureLatestSnapshot() returned nil after hydrating from disk")
	}
	if got.ID != "pre-restart" {
		t.Errorf("ensureLatestSnapshot().ID = %q, want %q", got.ID, "pre-restart")
	}
	if got.System.Hostname != "tower" {
		t.Errorf("ensureLatestSnapshot().System.Hostname = %q, want %q", got.System.Hostname, "tower")
	}

	// Cache must now be seeded — second call should hit the fast path.
	if cached := sched.Latest(); cached == nil || cached.ID != "pre-restart" {
		t.Errorf("scheduler cache not seeded after hydration; sched.Latest() = %+v", cached)
	}
}

// TestEnsureLatestSnapshot_NilWhenNoSnapshot: fresh install with no
// disk snapshot must return nil rather than a zero-value snapshot.
// Caller-side empty-state branches depend on this.
func TestEnsureLatestSnapshot_NilWhenNoSnapshot(t *testing.T) {
	t.Parallel()
	srv, _, _, _ := newServerWithSchedulerForEnsureTest(t, false)
	if got := srv.ensureLatestSnapshot(); got != nil {
		t.Errorf("ensureLatestSnapshot() = %+v on empty install, want nil", got)
	}
}

// TestEnsureLatestSnapshot_PrefersSchedulerCache: when the scheduler
// already has a snapshot (steady state — Collect tick has fired), the
// helper must return the cached value, not re-read disk.
func TestEnsureLatestSnapshot_PrefersSchedulerCache(t *testing.T) {
	t.Parallel()
	srv, store, sched, _ := newServerWithSchedulerForEnsureTest(t, false)

	// Disk has an OLD snapshot.
	if err := store.SaveSnapshot(&internal.Snapshot{ID: "stale-disk", Timestamp: time.Now().Add(-time.Hour).UTC()}); err != nil {
		t.Fatalf("SaveSnapshot stale: %v", err)
	}
	// Cache has a FRESH snapshot.
	fresh := &internal.Snapshot{ID: "fresh-cache", Timestamp: time.Now().UTC()}
	sched.SetLatest(fresh)

	got := srv.ensureLatestSnapshot()
	if got == nil || got.ID != "fresh-cache" {
		t.Errorf("ensureLatestSnapshot() = %+v, want fresh-cache (cache must beat disk)", got)
	}
}

// TestEnsureLatestSnapshot_ConcurrentSafe: N goroutines calling the
// helper simultaneously must all see the hydrated state with no data
// race. Run under `go test -race`.
func TestEnsureLatestSnapshot_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	srv, store, _, _ := newServerWithSchedulerForEnsureTest(t, false)

	if err := store.SaveSnapshot(&internal.Snapshot{
		ID:        "concurrent-target",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	const N = 64
	var wg sync.WaitGroup
	results := make([]*internal.Snapshot, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = srv.ensureLatestSnapshot()
		}(i)
	}
	wg.Wait()

	for i, snap := range results {
		if snap == nil {
			t.Errorf("goroutine %d saw nil snapshot — hydration race lost the result", i)
			continue
		}
		if snap.ID != "concurrent-target" {
			t.Errorf("goroutine %d saw ID=%q, want concurrent-target", i, snap.ID)
		}
	}
}

// TestMetricsEndpoint_PostRestart_HasSpeedtestGauges is the explicit
// regression guard for the v0.9.11-rc1/rc2 finding that originally
// motivated #305: nasdoctor_speedtest_engine was missing from /metrics
// after a fresh container start until /api/v1/snapshot/latest was
// curled. After the fix, scraping /metrics WITHOUT any prior API hit
// must surface the gauge family seeded from the persisted snapshot
// + speedtest_history hydration.
func TestMetricsEndpoint_PostRestart_HasSpeedtestGauges(t *testing.T) {
	t.Parallel()
	srv, store, _, m := newServerWithSchedulerForEnsureTest(t, true)

	// Pre-restart state: a snapshot envelope on disk + a historical
	// speedtest_history row. The Snapshot deliberately does NOT carry
	// SpeedTest (only the speed-test loop populates it in memory; the
	// persisted envelope drops it).
	if err := store.SaveSnapshot(&internal.Snapshot{
		ID:        "post-restart",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := store.SaveSpeedTest("hist", &internal.SpeedTestResult{
		Timestamp:    time.Now().Add(-2 * time.Hour).UTC(),
		DownloadMbps: 142.5, UploadMbps: 12.3, LatencyMs: 18.7,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	// Exercise the wrapped /metrics handler the same way the chi
	// router serves it: ensureLatestSnapshot first, then promhttp.
	srv.ensureLatestSnapshot()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}).ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="speedtest_go"} 1`) {
		t.Errorf("expected speedtest_go=1 gauge in /metrics post-restart with no prior API hit; speedtest lines:\n%s",
			grepBodyLines(body, "speedtest"))
	}
	if !strings.Contains(body, "nasdoctor_speedtest_download_mbps 142.5") {
		t.Errorf("expected download_mbps=142.5 in /metrics post-restart; speedtest lines:\n%s",
			grepBodyLines(body, "speedtest_download"))
	}
}

// TestMetricsEndpoint_PostRestart_AllSLatestGaugesPresent is the
// generic regression guard against this class of bug. It enumerates
// the subset of gauges that derive from s.latest and checks that
// every one of them appears in /metrics output post-restart with NO
// prior API hit. New gauges added to notifier.Metrics in future
// releases that read snap fields must work transparently — if a
// future change breaks the hydration plumbing, this test should fail
// across multiple gauge families simultaneously, making the regression
// loud rather than silent.
func TestMetricsEndpoint_PostRestart_AllSLatestGaugesPresent(t *testing.T) {
	t.Parallel()
	srv, store, _, m := newServerWithSchedulerForEnsureTest(t, true)

	// Build a richly-populated snapshot covering the major gauge
	// families that read from s.latest: System (CPU/mem/load/temps),
	// SMART, UPS, network, ZFS, and the Update gauge. Per-subsystem
	// values are sentinel-distinct so a typo or wiring break shows up
	// as a missing line rather than an unrelated coincidental match.
	snap := &internal.Snapshot{
		ID:        "post-restart-all-gauges",
		Timestamp: time.Now().UTC(),
		System: internal.SystemInfo{
			Hostname:   "tower",
			Platform:   "unraid",
			CPUUsage:   17.5,
			MemUsedMB:  8192,
			MemTotalMB: 32768,
			LoadAvg1:   0.55,
			LoadAvg5:   0.42,
			LoadAvg15:  0.31,
			UptimeSecs: 123456,
			CPUCores:   16,
			CPUTempC:   49,
			MoboTempC:  38,
		},
	}
	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Post-restart: scheduler cache is empty. The /metrics handler's
	// ensureLatestSnapshot wrapper must hydrate from disk + prime the
	// gauge family BEFORE promhttp serializes the registry.
	srv.ensureLatestSnapshot()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}).ServeHTTP(rec, req)
	body := rec.Body.String()

	// Subset of gauges derived from s.latest. If any are missing, the
	// hydration plumbing is broken — surface every miss in one failure
	// so the maintainer sees the pattern.
	wantSubstrings := []string{
		"nasdoctor_system_cpu_usage_percent 17.5",
		"nasdoctor_system_load_avg_1 0.55",
		"nasdoctor_system_load_avg_5 0.42",
		"nasdoctor_system_load_avg_15 0.31",
		"nasdoctor_system_uptime_seconds 123456",
		"nasdoctor_system_cpu_cores 16",
		"nasdoctor_system_cpu_temp_celsius 49",
		"nasdoctor_system_mobo_temp_celsius 38",
	}
	var missing []string
	for _, want := range wantSubstrings {
		if !strings.Contains(body, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		t.Errorf("post-restart /metrics missing %d gauge(s) derived from s.latest:\n  %s\n\nbody system lines:\n%s",
			len(missing), strings.Join(missing, "\n  "), grepBodyLines(body, "nasdoctor_system_"))
	}
}

// TestMetricsHandler_CallsEnsureLatestSnapshot wires the actual chi
// router and exercises the full /metrics request path end-to-end —
// guards against a refactor that adds a new /metrics route without
// the hydration wrapper.
func TestMetricsHandler_CallsEnsureLatestSnapshot(t *testing.T) {
	t.Parallel()
	srv, store, _, _ := newServerWithSchedulerForEnsureTest(t, true)

	if err := store.SaveSnapshot(&internal.Snapshot{
		ID:        "router-restart",
		Timestamp: time.Now().UTC(),
		System: internal.SystemInfo{
			Hostname: "router-tower",
			CPUUsage: 73.5,
		},
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Use the real router so the /metrics route's middleware chain +
	// hydration wrapper are exercised together.
	router := srv.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics returned %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "nasdoctor_system_cpu_usage_percent 73.5") {
		t.Errorf("GET /metrics did NOT surface s.latest-derived gauge; cpu lines:\n%s",
			grepBodyLines(body, "nasdoctor_system_cpu"))
	}
}

// grepBodyLines is a tiny helper for readable test failure output.
func grepBodyLines(body, substr string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, substr) {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}
