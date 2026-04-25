package collector

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// fakeSpeedTestEngine is the test double used to drive the runner
// implementations through their entire surface — happy path, error
// path, and the deterministic-sample emission used by slice 2's
// LiveTestRegistry. The result/samples/err triplet returned by Run()
// mirrors the SpeedTestRunner interface contract verbatim: either a
// non-nil result + a sample channel that eventually closes, or a nil
// result + nil channel + non-nil error.
type fakeSpeedTestEngine struct {
	result  *internal.SpeedTestResult
	samples []SpeedTestSample
	err     error
	calls   int
	mu      sync.Mutex
}

func (f *fakeSpeedTestEngine) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, nil, f.err
	}
	ch := make(chan SpeedTestSample, len(f.samples))
	for _, s := range f.samples {
		ch <- s
	}
	close(ch)
	return f.result, ch, nil
}

func (f *fakeSpeedTestEngine) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// drainSamples reads all SpeedTestSample values from a channel until
// it closes or the timeout elapses. Used by the contract tests to
// confirm the channel terminates cleanly without leaks.
func drainSamples(t *testing.T, ch <-chan SpeedTestSample, timeout time.Duration) []SpeedTestSample {
	t.Helper()
	var out []SpeedTestSample
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case s, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, s)
		case <-deadline.C:
			t.Fatalf("samples channel did not close within %s", timeout)
			return out
		}
	}
}

// ---------- Contract tests ----------

// TestSpeedTestRunner_Contract_SuccessPath_SampleChannelEventuallyCloses
// asserts the contract's success branch: every implementation must
// return (non-nil result, sample channel that eventually closes, nil
// error). Run against every concrete impl + the composite to guard
// the interface invariant.
func TestSpeedTestRunner_Contract_SuccessPath_SampleChannelEventuallyCloses(t *testing.T) {
	successResult := &internal.SpeedTestResult{
		DownloadMbps: 100, UploadMbps: 10, LatencyMs: 5, ServerName: "Test",
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}
	successSamples := []SpeedTestSample{
		{Phase: SpeedTestPhaseDownload, Mbps: 90, At: time.Now()},
		{Phase: SpeedTestPhaseDownload, Mbps: 99, At: time.Now()},
	}
	cases := []struct {
		name   string
		runner SpeedTestRunner
	}{
		{
			name: "speedtestGoRunner_with_fakeEngine",
			runner: newSpeedTestGoRunnerWithEngine(&fakeSpeedTestEngine{
				result: successResult, samples: successSamples,
			}),
		},
		{
			name: "ooklaCLIRunner_with_fakeEngine",
			runner: newOoklaCLIRunnerWithEngine(&fakeSpeedTestEngine{
				result: successResult, samples: nil,
			}),
		},
		{
			name: "compositeRunner_primarySucceeds",
			runner: NewCompositeSpeedTestRunner(
				newSpeedTestGoRunnerWithEngine(&fakeSpeedTestEngine{result: successResult, samples: successSamples}),
				newOoklaCLIRunnerWithEngine(&fakeSpeedTestEngine{result: nil, err: errors.New("should not be called")}),
			),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			res, samples, err := tc.runner.Run(ctx)
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if res == nil {
				t.Fatal("Run() result is nil on success branch")
			}
			if samples == nil {
				t.Fatal("Run() samples channel is nil on success branch — slice 2 will require this")
			}
			drainSamples(t, samples, 2*time.Second)
		})
	}
}

// TestSpeedTestRunner_Contract_ErrorPath_NoChannelLeak asserts the
// contract's error branch: every impl must return (nil, nil, err)
// when the underlying engine errors. A returned-but-unclosed channel
// would leak a goroutine in production (slice 2's LiveTestRegistry
// fan-out goroutine would block forever). Returning nil avoids that
// class of bug entirely.
func TestSpeedTestRunner_Contract_ErrorPath_NoChannelLeak(t *testing.T) {
	failingEngine := func() *fakeSpeedTestEngine {
		return &fakeSpeedTestEngine{err: errors.New("engine boom")}
	}
	cases := []struct {
		name   string
		runner SpeedTestRunner
	}{
		{"speedtestGoRunner", newSpeedTestGoRunnerWithEngine(failingEngine())},
		{"ooklaCLIRunner", newOoklaCLIRunnerWithEngine(failingEngine())},
		{"compositeRunner_bothFail", NewCompositeSpeedTestRunner(
			newSpeedTestGoRunnerWithEngine(failingEngine()),
			newOoklaCLIRunnerWithEngine(failingEngine()),
		)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			res, samples, err := tc.runner.Run(ctx)
			if err == nil {
				t.Fatal("Run() expected non-nil error on engine failure")
			}
			if res != nil {
				t.Errorf("Run() result must be nil on error; got %+v", res)
			}
			if samples != nil {
				t.Errorf("Run() samples channel must be nil on error; got non-nil — would leak in slice 2")
			}
		})
	}
}

// ---------- speedtestGoRunner ----------

// TestSpeedTestGoRunner_HappyPath_StampsSpeedTestGoEngine asserts that
// a successful run via the speedtest-go engine returns a result whose
// Engine field is "speedtest_go" (constant SpeedTestEngineSpeedTestGo).
// This is the column value persisted to speedtest_history.engine.
func TestSpeedTestGoRunner_HappyPath_StampsSpeedTestGoEngine(t *testing.T) {
	engine := &fakeSpeedTestEngine{
		result: &internal.SpeedTestResult{DownloadMbps: 500, UploadMbps: 50, LatencyMs: 8},
	}
	runner := newSpeedTestGoRunnerWithEngine(engine)
	res, samples, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("Engine = %q, want %q", res.Engine, internal.SpeedTestEngineSpeedTestGo)
	}
	drainSamples(t, samples, time.Second)
	if engine.callCount() != 1 {
		t.Errorf("engine called %d times, want 1", engine.callCount())
	}
}

// TestSpeedTestGoRunner_DeterministicSamplesPreservedInOrder asserts
// that samples emitted by the underlying engine pass through the
// runner in order. Slice 2's LiveTestRegistry depends on this for
// per-sample replay correctness.
func TestSpeedTestGoRunner_DeterministicSamplesPreservedInOrder(t *testing.T) {
	now := time.Now()
	want := []SpeedTestSample{
		{Phase: SpeedTestPhaseLatency, At: now, LatencyMs: 7.0},
		{Phase: SpeedTestPhaseDownload, At: now.Add(time.Second), Mbps: 100},
		{Phase: SpeedTestPhaseDownload, At: now.Add(2 * time.Second), Mbps: 200},
		{Phase: SpeedTestPhaseUpload, At: now.Add(3 * time.Second), Mbps: 50},
	}
	engine := &fakeSpeedTestEngine{
		result:  &internal.SpeedTestResult{DownloadMbps: 200, UploadMbps: 50, LatencyMs: 7},
		samples: want,
	}
	runner := newSpeedTestGoRunnerWithEngine(engine)
	_, samplesCh, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got := drainSamples(t, samplesCh, time.Second)
	if len(got) != len(want) {
		t.Fatalf("sample count: got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Phase != w.Phase || got[i].Mbps != w.Mbps {
			t.Errorf("sample[%d]: got %+v, want %+v", i, got[i], w)
		}
	}
}

// ---------- ooklaCLIRunner ----------

// TestOoklaCLIRunner_HappyPath_StampsOoklaCLIEngine asserts that the
// CLI fallback runner stamps Engine="ookla_cli" on the result, and
// returns a sample channel that closes (slice 1: with no samples;
// slice 2 may add server-pick / phase-transition synthetic samples).
func TestOoklaCLIRunner_HappyPath_StampsOoklaCLIEngine(t *testing.T) {
	engine := &fakeSpeedTestEngine{
		result: &internal.SpeedTestResult{DownloadMbps: 80, UploadMbps: 8, LatencyMs: 25},
	}
	runner := newOoklaCLIRunnerWithEngine(engine)
	res, samples, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.Engine != internal.SpeedTestEngineOoklaCLI {
		t.Errorf("Engine = %q, want %q", res.Engine, internal.SpeedTestEngineOoklaCLI)
	}
	drainSamples(t, samples, time.Second)
}

// ---------- compositeRunner ----------

// TestCompositeRunner_PrimarySucceeds_FallbackNotInvoked asserts the
// composite's normal path: when the primary (speedtest-go) succeeds,
// the fallback (Ookla CLI) is never called and the result stamps
// speedtest_go.
func TestCompositeRunner_PrimarySucceeds_FallbackNotInvoked(t *testing.T) {
	primaryEng := &fakeSpeedTestEngine{result: &internal.SpeedTestResult{DownloadMbps: 100}}
	fallbackEng := &fakeSpeedTestEngine{err: errors.New("should not be called")}
	composite := NewCompositeSpeedTestRunner(
		newSpeedTestGoRunnerWithEngine(primaryEng),
		newOoklaCLIRunnerWithEngine(fallbackEng),
	)
	res, samples, err := composite.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("Engine = %q, want speedtest_go (primary)", res.Engine)
	}
	drainSamples(t, samples, time.Second)
	if fallbackEng.callCount() != 0 {
		t.Errorf("fallback called %d times, want 0 — fallback must NOT run when primary succeeds", fallbackEng.callCount())
	}
}

// TestCompositeRunner_PrimaryFails_FallbackTakesOver asserts the
// resilience case: primary errors → fallback runs → result stamps
// ookla_cli. This is the scenario where the bundled Alpine
// speedtest-go has trouble (network policy, broken upstream, etc.)
// and we still want a usable result on the dashboard.
func TestCompositeRunner_PrimaryFails_FallbackTakesOver(t *testing.T) {
	primaryEng := &fakeSpeedTestEngine{err: errors.New("primary boom")}
	fallbackResult := &internal.SpeedTestResult{DownloadMbps: 80, UploadMbps: 8, LatencyMs: 25}
	fallbackEng := &fakeSpeedTestEngine{result: fallbackResult}
	composite := NewCompositeSpeedTestRunner(
		newSpeedTestGoRunnerWithEngine(primaryEng),
		newOoklaCLIRunnerWithEngine(fallbackEng),
	)
	res, samples, err := composite.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() unexpected error after fallback success: %v", err)
	}
	if res == nil {
		t.Fatal("Run() result is nil but fallback produced a result")
	}
	if res.Engine != internal.SpeedTestEngineOoklaCLI {
		t.Errorf("Engine = %q, want ookla_cli (fallback path)", res.Engine)
	}
	drainSamples(t, samples, time.Second)
	if primaryEng.callCount() != 1 {
		t.Errorf("primary called %d times, want 1", primaryEng.callCount())
	}
	if fallbackEng.callCount() != 1 {
		t.Errorf("fallback called %d times, want 1", fallbackEng.callCount())
	}
}

// TestCompositeRunner_BothEnginesFail_PropagatesError asserts that
// when both engines error, the composite returns a non-nil error
// (the fallback's, since it ran most recently) and a nil result. The
// scheduler currently treats a nil result as "no speedtest tool
// available" — a non-nil error here gives slice 2 a richer signal
// without changing slice 1 semantics.
func TestCompositeRunner_BothEnginesFail_PropagatesError(t *testing.T) {
	primaryEng := &fakeSpeedTestEngine{err: errors.New("primary failed")}
	fallbackEng := &fakeSpeedTestEngine{err: errors.New("fallback failed too")}
	composite := NewCompositeSpeedTestRunner(
		newSpeedTestGoRunnerWithEngine(primaryEng),
		newOoklaCLIRunnerWithEngine(fallbackEng),
	)
	res, samples, err := composite.Run(context.Background())
	if err == nil {
		t.Fatal("Run() expected non-nil error when both engines fail")
	}
	if res != nil {
		t.Errorf("Run() result must be nil when both engines fail; got %+v", res)
	}
	if samples != nil {
		t.Error("Run() samples channel must be nil when both engines fail")
	}
	if !strings.Contains(err.Error(), "fallback failed too") && !strings.Contains(err.Error(), "primary failed") {
		t.Errorf("error must mention an underlying engine failure; got %v", err)
	}
}

// TestCompositeRunner_NilPrimary_DegradesToFallbackOnly guards a
// configuration edge case: if a deployment opts out of speedtest-go
// (e.g. disabled at compile time, or not registered at startup), the
// composite must still work with only a fallback. Same shape as the
// "primary errored" case from a behavioural standpoint.
func TestCompositeRunner_NilPrimary_DegradesToFallbackOnly(t *testing.T) {
	fallbackEng := &fakeSpeedTestEngine{result: &internal.SpeedTestResult{DownloadMbps: 80}}
	composite := NewCompositeSpeedTestRunner(
		nil,
		newOoklaCLIRunnerWithEngine(fallbackEng),
	)
	res, samples, err := composite.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.Engine != internal.SpeedTestEngineOoklaCLI {
		t.Errorf("Engine = %q, want ookla_cli", res.Engine)
	}
	drainSamples(t, samples, time.Second)
}
