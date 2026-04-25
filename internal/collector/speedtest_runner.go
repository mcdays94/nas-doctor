// Package collector — speedtest_runner.go introduces the SpeedTestRunner
// deep-module abstraction that PRD #283 (issue #284) calls for: a single
// interface with two production implementations (showwin/speedtest-go +
// Ookla CLI fallback) wrapped in a composite that prefers the primary
// and falls back on error.
//
// Design mirrors internal/collector/borg_runner.go (v0.9.10 / issue
// #279) verbatim — interface in this file, production impls behind a
// constructor, fakeable engine for tests, contract tests pinning the
// (result, samples, err) tuple invariant.
//
// Slice 1 of the PRD (this issue) returns a sample channel that is
// drained-and-discarded by RunSpeedTest. Slice 2 (#285) wires it to a
// LiveTestRegistry for SSE fan-out. Defining the channel return now
// keeps the slices independently shippable without re-shaping the
// interface later.
package collector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// SpeedTestPhase enumerates the three test phases. Stable string values
// — they are emitted on the SSE wire in slice 2 and persisted into
// speedtest_samples.phase in slice 3.
type SpeedTestPhase string

const (
	SpeedTestPhaseLatency  SpeedTestPhase = "latency"
	SpeedTestPhaseDownload SpeedTestPhase = "download"
	SpeedTestPhaseUpload   SpeedTestPhase = "upload"
)

// SpeedTestSample is a single in-flight sample emitted during a test.
// The sample-channel return type is wired through every runner impl in
// slice 1 but is drained-and-discarded by the scheduler for now;
// slice 2 (#285) feeds these to LiveTestRegistry's broadcast fan-out.
//
// Fields are intentionally a superset of what any one phase needs —
// LatencyMs is meaningful for latency-phase samples, Mbps for
// download/upload-phase samples. Zero values are valid and unambiguous.
type SpeedTestSample struct {
	Phase     SpeedTestPhase
	At        time.Time
	Mbps      float64
	LatencyMs float64
}

// SpeedTestRunner is the engine-agnostic interface that
// internal/collector.RunSpeedTest delegates to. Three implementations:
//   - speedtestGoRunner: showwin/speedtest-go primary path. Stamps
//     Engine="speedtest_go" on the result.
//   - ooklaCLIRunner: bundled Ookla CLI fallback path. Stamps
//     Engine="ookla_cli". Sample channel always closes empty in
//     slice 1 — the CLI emits final-only JSON and there's no useful
//     per-sample telemetry to surface.
//   - compositeRunner: tries primary, falls back on error.
//
// Contract:
//   - On success: result is non-nil, samples is a non-nil channel that
//     EVENTUALLY closes (caller MUST drain it or leak a goroutine), err
//     is nil.
//   - On failure: result is nil, samples is nil (no leak risk), err is
//     non-nil.
//
// See speedtest_runner_test.go for the contract tests pinning the
// invariant against every impl.
type SpeedTestRunner interface {
	Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error)
}

// speedTestEngine is the swappable inner-loop driver that the
// production speedtest-go and Ookla-CLI runners delegate to. Tests
// inject a fakeSpeedTestEngine to exercise every result/error branch
// without going to the network. Production wiring (in this file)
// constructs the real engine at startup and never swaps it.
type speedTestEngine interface {
	Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error)
}

// ---------- speedtestGoRunner ----------

// speedtestGoRunner is the primary engine — showwin/speedtest-go. It
// supports per-sample callbacks during each phase, which is exactly
// what slice 2's LiveTestRegistry needs. Slice 1 ignores the samples
// (drains the channel and discards) but the runner still emits them
// so slice 2 doesn't reshape the interface.
type speedtestGoRunner struct {
	engine speedTestEngine
}

// newSpeedTestGoRunnerWithEngine constructs a speedtestGoRunner with
// an explicit engine (for tests). Production callers use
// NewSpeedTestGoRunner which wires the real upstream library.
func newSpeedTestGoRunnerWithEngine(engine speedTestEngine) SpeedTestRunner {
	return &speedtestGoRunner{engine: engine}
}

// NewSpeedTestGoRunner returns the production speedtestGoRunner backed
// by the showwin/speedtest-go library. Returns nil if the library is
// not available at runtime (e.g. older binaries without the dep) — the
// composite gracefully degrades to fallback-only when its primary is
// nil.
func NewSpeedTestGoRunner() SpeedTestRunner {
	return newSpeedTestGoRunnerWithEngine(newRealSpeedTestGoEngine())
}

func (r *speedtestGoRunner) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	if r.engine == nil {
		return nil, nil, errors.New("speedtestGoRunner: engine is nil")
	}
	res, samples, err := r.engine.Run(ctx)
	if err != nil {
		return nil, nil, err
	}
	if res == nil {
		return nil, nil, errors.New("speedtestGoRunner: engine returned nil result without error")
	}
	res.Engine = internal.SpeedTestEngineSpeedTestGo
	return res, samples, nil
}

// ---------- ooklaCLIRunner ----------

// ooklaCLIRunner is the fallback engine — the bundled Ookla CLI binary
// at /usr/local/bin/speedtest. The CLI emits final-only JSON; the
// sample channel always closes empty for this engine in slice 1.
type ooklaCLIRunner struct {
	engine speedTestEngine
}

func newOoklaCLIRunnerWithEngine(engine speedTestEngine) SpeedTestRunner {
	return &ooklaCLIRunner{engine: engine}
}

// NewOoklaCLIRunner returns the production ooklaCLIRunner backed by
// the existing internal.runOoklaSpeedtest path. Slice 1 wraps the
// legacy text-mode invocation as an engine adapter so the runner
// surface is uniform.
func NewOoklaCLIRunner() SpeedTestRunner {
	return newOoklaCLIRunnerWithEngine(newRealOoklaCLIEngine())
}

func (r *ooklaCLIRunner) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	if r.engine == nil {
		return nil, nil, errors.New("ooklaCLIRunner: engine is nil")
	}
	res, samples, err := r.engine.Run(ctx)
	if err != nil {
		return nil, nil, err
	}
	if res == nil {
		return nil, nil, errors.New("ooklaCLIRunner: engine returned nil result without error")
	}
	res.Engine = internal.SpeedTestEngineOoklaCLI
	return res, samples, nil
}

// ---------- compositeRunner ----------

// compositeRunner tries primary first and falls back to secondary on
// error. Either may be nil — a nil primary degrades to fallback-only.
// If both are nil, every Run() call returns an error (no engine
// available); if both fail at runtime, the most recent (fallback)
// error is wrapped with the primary error in its message so the
// scheduler log can show both.
type compositeRunner struct {
	primary  SpeedTestRunner
	fallback SpeedTestRunner
}

// NewCompositeSpeedTestRunner constructs the composite. Production
// wiring (cmd/nas-doctor/main.go) passes NewSpeedTestGoRunner() and
// NewOoklaCLIRunner(); tests pass deterministic fakes.
func NewCompositeSpeedTestRunner(primary, fallback SpeedTestRunner) SpeedTestRunner {
	return &compositeRunner{primary: primary, fallback: fallback}
}

func (r *compositeRunner) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	if r.primary == nil && r.fallback == nil {
		return nil, nil, errors.New("compositeRunner: no engines configured")
	}
	var primaryErr error
	if r.primary != nil {
		res, samples, err := r.primary.Run(ctx)
		if err == nil {
			return res, samples, nil
		}
		primaryErr = err
	}
	if r.fallback == nil {
		return nil, nil, fmt.Errorf("primary failed and no fallback configured: %w", primaryErr)
	}
	res, samples, err := r.fallback.Run(ctx)
	if err == nil {
		return res, samples, nil
	}
	if primaryErr != nil {
		return nil, nil, fmt.Errorf("both engines failed: primary=%v, fallback=%w", primaryErr, err)
	}
	return nil, nil, err
}

// ---------- Engine adapters (production) ----------

// realSpeedTestGoEngine wraps showwin/speedtest-go and adapts its API
// to the speedTestEngine interface. Per-server iteration is
// flattened: we pick the first server returned by FindServer (the
// closest match), run latency/download/upload, and return the
// composed SpeedTestResult.
//
// Slice 1: samples channel is closed before return — slice 2 will
// stream per-phase samples via the runner's existing showwin
// callbacks.
type realSpeedTestGoEngine struct {
	// guard: zero-value usable, but this lock is mostly here to make
	// the engine fail-fast obvious if a future caller accidentally
	// shares the engine across goroutines.
	mu sync.Mutex
}

func newRealSpeedTestGoEngine() speedTestEngine {
	return &realSpeedTestGoEngine{}
}

func (e *realSpeedTestGoEngine) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return runSpeedtestGoLibrary(ctx)
}

// realOoklaCLIEngine adapts the legacy runOoklaSpeedtest path (the
// text-mode JSON parse) to the speedTestEngine interface. The sample
// channel always closes empty for this engine in slice 1.
type realOoklaCLIEngine struct{}

func newRealOoklaCLIEngine() speedTestEngine {
	return &realOoklaCLIEngine{}
}

func (e *realOoklaCLIEngine) Run(_ context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	res := runOoklaSpeedtest()
	if res == nil {
		return nil, nil, errors.New("ooklaCLIRunner: speedtest CLI unavailable or returned zero throughput")
	}
	ch := make(chan SpeedTestSample)
	close(ch)
	return res, ch, nil
}
