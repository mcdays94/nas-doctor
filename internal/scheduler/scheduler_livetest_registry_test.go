package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// fakeRegistry is the test seam for runSpeedTestViaRegistry. It
// records every StartTest call so tests can assert that the cron-
// driven path goes through the registry, and synthesises a
// pre-completed LiveTest so the scheduler doesn't block on Done().
//
// We can't actually construct a *livetest.LiveTest from outside the
// package (its fields are unexported). Instead, this fake builds a
// LiveTest by spinning up a real livetest.Manager with a deterministic
// runner and proxying its handle. Tests verify "registry was used"
// via the StartCalled counter — that's the contract this slice
// promises (cron tick → registry, not raw runner).
type fakeRegistry struct {
	startCalls int64
	mgr        *livetest.Manager
}

func (f *fakeRegistry) StartTest(ctx context.Context) (*livetest.LiveTest, error) {
	atomic.AddInt64(&f.startCalls, 1)
	return f.mgr.StartTest(ctx)
}

func (f *fakeRegistry) GetLive(id int64) (*livetest.LiveTest, bool) {
	return f.mgr.GetLive(id)
}

func (f *fakeRegistry) InProgress() bool {
	return f.mgr.InProgress()
}

// RegisterStateChangeObserver / RegisterCompletionHandler proxy to the
// inner livetest.Manager so the scheduler's observer wiring fires
// even when tests pass *fakeRegistry instead of the bare manager.
// Without these methods the type-assertion in
// Scheduler.SetLiveTestRegistry would fall through and persistence
// would silently skip on the cron path. Issue #294.
func (f *fakeRegistry) RegisterStateChangeObserver(fn func(running bool)) {
	f.mgr.RegisterStateChangeObserver(fn)
}

func (f *fakeRegistry) RegisterCompletionHandler(fn func(*livetest.LiveTest)) {
	f.mgr.RegisterCompletionHandler(fn)
}

func (f *fakeRegistry) Calls() int64 {
	return atomic.LoadInt64(&f.startCalls)
}

// instantRunner is a livetest.Runner that returns a successful
// result immediately with no samples. Mirrors the slice-1 ookla
// fallback shape (final-only result).
type instantRunner struct {
	result *internal.SpeedTestResult
}

func (r *instantRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	ch := make(chan collector.SpeedTestSample)
	close(ch)
	return r.result, ch, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRunSpeedTest_GoesViaRegistry asserts the scheduler's cron-driven
// runSpeedTest routes through the LiveTestRegistry when one is wired,
// rather than calling the legacy result-only shim.
func TestRunSpeedTest_GoesViaRegistry(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()

	s := New(nil, store, nil, nil, logger, time.Hour)
	// No legacy runner — if the registry path doesn't fire, we'd
	// expect a "no speedtest tool available" error. Existing test
	// would fail loudly (no-tool path writes status=failed).

	runner := &instantRunner{
		result: &internal.SpeedTestResult{
			DownloadMbps: 100,
			UploadMbps:   50,
			LatencyMs:    10,
			Engine:       internal.SpeedTestEngineSpeedTestGo,
		},
	}
	mgr := livetest.NewManager(runner, logger, nil)
	reg := &fakeRegistry{mgr: mgr}
	s.SetLiveTestRegistry(reg)

	s.runSpeedTest()

	if reg.Calls() != 1 {
		t.Errorf("registry StartTest calls = %d, want 1", reg.Calls())
	}

	// Verify the success path persisted history + flipped LastAttempt.
	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil || att.Status != "success" {
		t.Errorf("LastAttempt status = %v, want success", att)
	}
}

// TestRunSpeedTest_DisabledShortCircuits asserts the disabled branch
// short-circuits BEFORE touching the registry — the registry MUST
// NOT be called when the user has set scan to "Disabled". This
// preserves the v0.9.6 behaviour from #210 where the disabled branch
// is idempotent.
func TestRunSpeedTest_DisabledShortCircuits(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)
	s.SetSpeedTestInterval(SpeedTestIntervalDisabled)

	runner := &instantRunner{result: &internal.SpeedTestResult{Engine: "speedtest_go"}}
	mgr := livetest.NewManager(runner, logger, nil)
	reg := &fakeRegistry{mgr: mgr}
	s.SetLiveTestRegistry(reg)

	s.runSpeedTest()

	if reg.Calls() != 0 {
		t.Errorf("registry called %d times in disabled state, want 0", reg.Calls())
	}
	att, _ := store.GetLastSpeedTestAttempt()
	if att == nil || att.Status != "disabled" {
		t.Errorf("LastAttempt = %v, want disabled", att)
	}
}

// TestRunSpeedTest_LegacyPath_StillWorksWhenRegistryNil verifies the
// pre-#285 fallback: when no registry is wired, the scheduler still
// uses the result-only speedTestRunFn shim (so legacy tests don't
// need rewriting).
func TestRunSpeedTest_LegacyPath_StillWorksWhenRegistryNil(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)

	calls := int64(0)
	s.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		atomic.AddInt64(&calls, 1)
		return &internal.SpeedTestResult{
			DownloadMbps: 100, UploadMbps: 50, LatencyMs: 5,
			Engine: internal.SpeedTestEngineOoklaCLI,
		}
	})
	// Do NOT wire a registry.

	s.runSpeedTest()

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("legacy runner called %d times, want 1", got)
	}
	att, _ := store.GetLastSpeedTestAttempt()
	if att == nil || att.Status != "success" {
		t.Errorf("LastAttempt = %v, want success", att)
	}
}
