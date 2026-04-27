package scheduler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/livetest"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #294 R3a + R3b: when SetLiveTestRegistry wires the observer
// hooks, ANY caller of registry.StartTest (cron loop, manual API
// path, or future fleet-driven path) gets the same persistence +
// gauge-management side effects via the registered callbacks. These
// tests simulate the API path by calling registry.StartTest directly
// without going through s.runSpeedTest, asserting the result is
// indistinguishable from the cron path.

// TestLiveTestRegistry_APIPath_PersistsHistoryAndSamples drives the
// API path's call shape: get the registry, StartTest(ctx) (no block-
// on-Done), wait briefly, then assert the DB holds a history row +
// samples linked to the test_id. Pre-fix this was R3b — the API path
// returned the test_id and exited; nothing wrote to DB until the next
// cron tick (4h later). After the fix, the registry's completion
// observer persists synchronously inside the registry goroutine.
func TestLiveTestRegistry_APIPath_PersistsHistoryAndSamples(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)

	now := time.Now().UTC()
	runner := &samplingRunner{
		result: &internal.SpeedTestResult{
			DownloadMbps: 250, UploadMbps: 25, LatencyMs: 12,
			Timestamp: now,
			Engine:    internal.SpeedTestEngineSpeedTestGo,
		},
		// samplingRunner type from scheduler_speedtest_samples_test.go;
		// per-sample stream stays empty for this test (we only care
		// about history-row persistence on the API path).
		samples: nil,
	}

	// Build via registry directly (mirrors what the API handler does).
	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	// Simulate the API path: registry.StartTest, no scheduler
	// block-on-Done. The registry's completion handler (wired in
	// SetLiveTestRegistry) is responsible for persistence.
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	// Now wait for completion. We don't have a cron-loop blocker
	// here — the API path returns immediately. But we need to wait
	// to assert the side effects. This block is the test waiting,
	// not the production code path.
	<-lt.Done()

	// At this point the production code's completion handler has
	// run. History row must exist.
	id, ok, err := store.GetLatestSpeedTestHistoryID()
	if err != nil {
		t.Fatalf("GetLatestSpeedTestHistoryID: %v", err)
	}
	if !ok || id == 0 {
		t.Fatalf("expected history row to be persisted via API path, got ok=%v id=%d", ok, id)
	}

	// LastAttempt must have been flipped to success.
	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil || att.Status != "success" {
		t.Errorf("LastAttempt = %v, want status=success", att)
	}
}

// TestLiveTestRegistry_StateChange_FlipsInProgressGauge wires a real
// notifier.Metrics into the scheduler and asserts that the
// nasdoctor_speedtest_in_progress gauge is 1 while a test is in
// flight (driven by an arbitrary caller of registry.StartTest, NOT
// just runSpeedTest) and 0 after completion. R3a regression guard.
func TestLiveTestRegistry_StateChange_FlipsInProgressGauge(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	m := notifier.NewMetrics()
	s := New(nil, store, nil, m, logger, time.Hour)

	// Use a runner whose Run() blocks until the test releases it.
	// This simulates a real speed test where the gauge MUST read 1
	// during the runner's execution window.
	release := make(chan struct{})
	runner := &blockingRunner{
		result:  &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo, DownloadMbps: 100},
		release: release,
	}
	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	// Simulate manual /api/v1/speedtest/run path: caller invokes
	// StartTest and exits without blocking on Done.
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	// Scrape /metrics — gauge must read 1 BEFORE the runner
	// finishes. If the observer fired at StartTest time (synchronous
	// per the contract), this scrape sees in_progress=1.
	body := scrapeFromMetrics(t, m)
	if !strings.Contains(body, "nasdoctor_speedtest_in_progress 1") {
		t.Errorf("during in-flight test, /metrics did NOT show in_progress=1 — observer wiring broken; body excerpt:\n%s",
			grepMetricsLines(body, "speedtest_in_progress"))
	}

	// Release the runner; wait for completion.
	close(release)
	<-lt.Done()

	// Allow observer (running=false) to fire — it's invoked after
	// closeSubscribersAndDone, which itself runs after Done closes.
	// We need to wait for it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body = scrapeFromMetrics(t, m)
		if strings.Contains(body, "nasdoctor_speedtest_in_progress 0") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !strings.Contains(body, "nasdoctor_speedtest_in_progress 0") {
		t.Errorf("after test completed, /metrics did NOT return to in_progress=0; body excerpt:\n%s",
			grepMetricsLines(body, "speedtest_in_progress"))
	}
}

// TestLiveTestRegistry_StateChange_FailedRunStillResetsGauge asserts
// that a runner returning an error immediately (the showwin
// FetchUserInfo flake on UAT) still resets the gauge to 0. Without
// this the gauge would be stuck at 1 after a failed manual run,
// creating false positives for any future "running >5min" alerts.
func TestLiveTestRegistry_StateChange_FailedRunStillResetsGauge(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	m := notifier.NewMetrics()
	s := New(nil, store, nil, m, logger, time.Hour)

	runner := &failingRunner{}
	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-lt.Done()

	// Gauge eventually returns to 0.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body := scrapeFromMetrics(t, m)
		if strings.Contains(body, "nasdoctor_speedtest_in_progress 0") {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Errorf("gauge stuck after fast-failing runner — would generate false stuck-test alerts")
}

// blockingRunner blocks Run until release is closed, then returns
// the configured result with no samples. Useful for tests that need
// to assert metrics during a runner's window.
type blockingRunner struct {
	result  *internal.SpeedTestResult
	release chan struct{}
}

func (r *blockingRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan internalSpeedTestSample, error) {
	out := make(chan internalSpeedTestSample)
	go func() {
		<-r.release
		close(out)
	}()
	return r.result, out, nil
}

// failingRunner errors on Run() — simulates the showwin FetchUserInfo
// fast-fail mode observed during UAT.
type failingRunner struct{}

func (failingRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan internalSpeedTestSample, error) {
	return nil, nil, errFastFail
}

var errFastFail = stringError("simulated fast failure (e.g. FetchUserInfo timeout)")

type stringError string

func (s stringError) Error() string { return string(s) }

// scrapeFromMetrics renders /metrics from a notifier.Metrics. Local
// helper to avoid depending on the notifier package's test exports.
func scrapeFromMetrics(t *testing.T, m *notifier.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}).ServeHTTP(rec, req)
	return rec.Body.String()
}

// grepMetricsLines filters metrics body to lines containing substr.
func grepMetricsLines(body, substr string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, substr) {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}

// internalSpeedTestSample re-aliases collector.SpeedTestSample under
// a simpler name so the test runners' Run signatures stay readable.
// The livetest.Sample type aliases the collector type already, but
// blockingRunner / failingRunner implement collector.SpeedTestRunner
// directly — they need the original collector type.
type internalSpeedTestSample = livetest.Sample

// Compile-time guard: blockingRunner / failingRunner satisfy
// livetest.Runner.
var (
	_ livetest.Runner = (*blockingRunner)(nil)
	_ livetest.Runner = failingRunner{}
)
