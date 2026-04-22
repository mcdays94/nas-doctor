package scheduler

import (
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #180 — "Disabled" option in the speed-test interval dropdown.
//
// Design:
//   - scheduler.SpeedTestIntervalDisabled is a sentinel Duration that
//     represents "do not run the standalone speed-test loop".
//   - When the scheduler's speedTestInterval is set to this sentinel,
//     runSpeedTest() is a no-op: no Ookla invocation, no DB writes.
//   - SetSpeedTestInterval must preserve the sentinel (it's NOT subject
//     to the 5-minute minimum clamp that applies to real intervals).
//   - The Test button (handleTestServiceCheck) is unaffected — it builds
//     its own ServiceChecker with SetSpeedTestRunner, so a disabled
//     standalone loop does not break on-demand runs.
//
// The Scheduler's runSpeedTest() previously called collector.RunSpeedTest()
// directly, which is impossible to intercept from a test. These tests
// drive the addition of a runner-injection hook (mirroring the pattern
// already used by ServiceChecker.SetSpeedTestRunner).

// newSpeedTestSchedulerForTest builds a minimal scheduler suitable for
// exercising runSpeedTest() in isolation. The returned scheduler has an
// injectable speed-test runner (set it with SetSpeedTestRunner) so tests
// can observe whether the runner was invoked or skipped.
func newSpeedTestSchedulerForTest(interval time.Duration, runner SpeedTestRunner) *Scheduler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := &Scheduler{
		store:             storage.NewFakeStore(),
		logger:            logger,
		interval:          time.Hour,
		speedTestInterval: interval,
		serviceChecks:     []internal.ServiceCheckConfig{},
		stop:              make(chan struct{}),
		restart:           make(chan time.Duration, 1),
	}
	s.SetSpeedTestRunner(runner)
	return s
}

// When the interval is set to the disabled sentinel, runSpeedTest must
// be a no-op and never invoke the runner.
func TestRunSpeedTest_SkippedWhenDisabled(t *testing.T) {
	var calls int32
	runner := func() *internal.SpeedTestResult {
		atomic.AddInt32(&calls, 1)
		return &internal.SpeedTestResult{DownloadMbps: 100}
	}

	s := newSpeedTestSchedulerForTest(SpeedTestIntervalDisabled, runner)
	s.runSpeedTest()

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("runSpeedTest invoked runner %d time(s) when disabled; want 0 (issue #180)", got)
	}
}

// When the interval is a normal positive duration, runSpeedTest must
// invoke the runner exactly once. This is the happy path — a control
// test that proves the skip guard in the disabled case is doing real
// work, not just broken plumbing.
func TestRunSpeedTest_RunsWhenEnabled(t *testing.T) {
	var calls int32
	runner := func() *internal.SpeedTestResult {
		atomic.AddInt32(&calls, 1)
		return &internal.SpeedTestResult{DownloadMbps: 100}
	}

	s := newSpeedTestSchedulerForTest(4*time.Hour, runner)
	s.runSpeedTest()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("runSpeedTest invoked runner %d time(s) when enabled; want 1", got)
	}
}

// SetSpeedTestInterval must preserve the disabled sentinel rather than
// clamping it up to the 5-minute minimum. Without this, calling
// SetSpeedTestInterval(SpeedTestIntervalDisabled) would silently re-enable
// the loop at 5-minute resolution — a data-exfiltration regression for
// metered-connection users.
func TestSetSpeedTestInterval_PreservesDisabledSentinel(t *testing.T) {
	s := newSpeedTestSchedulerForTest(4*time.Hour, func() *internal.SpeedTestResult { return nil })

	s.SetSpeedTestInterval(SpeedTestIntervalDisabled)

	s.mu.RLock()
	got := s.speedTestInterval
	s.mu.RUnlock()

	if got != SpeedTestIntervalDisabled {
		t.Errorf("SetSpeedTestInterval(disabled) stored %v; want %v (sentinel must survive the clamp)", got, SpeedTestIntervalDisabled)
	}
}

// The default scheduler constructed via New() must NOT start in the
// disabled state. Opt-in only — existing users keep their current
// 4-hour default on upgrade.
func TestNewScheduler_DefaultIntervalNotDisabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(nil, storage.NewFakeStore(), nil, nil, logger, time.Hour)

	s.mu.RLock()
	got := s.speedTestInterval
	s.mu.RUnlock()

	if got == SpeedTestIntervalDisabled {
		t.Errorf("newly constructed scheduler must not default to disabled (issue #180 is opt-in)")
	}
	if got <= 0 {
		t.Errorf("default speedTestInterval = %v; want a positive duration", got)
	}
}
