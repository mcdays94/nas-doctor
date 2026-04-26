// Package livetest implements the LiveTestRegistry deep module from
// PRD #283 / issue #285 (slice 2 of the speed-test live-progress
// streaming PRD).
//
// The registry is the broadcast state machine that decouples the
// speed-test runner (collector layer) from SSE subscribers (HTTP
// layer). A single test runs at a time; multiple HTTP clients
// (multiple browser tabs, reconnecting EventSource clients) can
// Subscribe to the same in-flight test and each gets the FULL
// sample sequence (replay-on-subscribe + live fan-out).
//
// Design constraints:
//
//   - StartTest is idempotent: a second concurrent call while a test
//     is in flight returns the existing handle. This is what makes
//     "click Run twice" / "open dashboard in two tabs" / "cron tick
//     fires while user clicks Run" all collapse onto one test.
//
//   - Subscribers attaching mid-test must see every sample emitted
//     so far before live samples start. This is what makes browser
//     reconnects-mid-test render a smooth chart from sample 0.
//
//   - Slow subscribers must not block the emitter. A synthetic slow
//     consumer (channel never read) is dropped from the broadcast
//     set — its channel is closed and the warning logged. Other
//     subscribers continue receiving normally.
//
//   - The registry is tested in isolation via a fake runner (see
//     registry_test.go). Production wiring lives in the scheduler.
package livetest

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// Sample is the per-tick datum emitted by the runner during a test.
// Aliases collector.SpeedTestSample so HTTP/SSE callers can stream
// samples without depending on the collector package directly.
type Sample = collector.SpeedTestSample

// Result is the final result of a completed test. Mirror of
// internal.SpeedTestResult so HTTP/SSE callers can decode without
// pulling in the broader internal model.
type Result = internal.SpeedTestResult

// Phase aliases collector.SpeedTestPhase. The registry doesn't itself
// emit phase-change events to subscribers (those are derived from
// each sample's Phase field by the SSE handler) but the alias is
// re-exported here so SSE handler code can switch on phase values
// without importing collector.
type Phase = collector.SpeedTestPhase

// Runner is the dependency the registry uses to drive a single test.
// It mirrors collector.SpeedTestRunner exactly — the registry doesn't
// know or care which engine is in play, just that Run returns a
// (result, samples, err) tuple where samples eventually closes.
type Runner interface {
	Run(ctx context.Context) (*Result, <-chan Sample, error)
}

// SubscribeBufferSize is the per-subscriber channel buffer.
// Subscribers slower than this are dropped (see liveTest.emit). Sized
// for ~30 seconds of samples at 1Hz emit rate plus headroom; in
// practice samples emit at sub-second rates from showwin's callbacks
// during the throughput phases, so 64 gives the SSE flusher generous
// budget before backpressure kicks in.
const SubscribeBufferSize = 64

// Registry is the contract: scheduler + HTTP handlers depend on this
// interface, not the concrete Manager, so tests can inject a fake.
//
// StartTest acquires the singleton mutex. If a test is already in
// flight, the existing handle is returned (callers cannot tell
// whether their call started the test or attached to an existing
// one — that's the idempotency guarantee).
//
// GetLive looks up an in-flight test by ID. Returns (nil, false) if
// no test is in flight or the ID doesn't match the current test.
// The registry only tracks ONE test at a time — completed test IDs
// are forgotten. Subscribers wanting samples for a completed test
// should rely on the result/end events delivered on their existing
// subscription before completion, not look up by ID afterwards.
type Registry interface {
	StartTest(ctx context.Context) (*LiveTest, error)
	GetLive(testID int64) (*LiveTest, bool)
	InProgress() bool
}

// Manager is the production Registry. Holds a single optional in-flight
// LiveTest under a mutex. After completion the LiveTest is retained
// for a short grace window so a late-arriving SSE subscriber can see
// the final result + end events before the test is fully forgotten.
type Manager struct {
	runner Runner
	logger *slog.Logger
	idGen  func() int64

	mu     sync.Mutex
	active *LiveTest // nil when no test in flight
}

// NewManager constructs a Registry-implementing Manager. Pass a
// production runner (e.g. the collector's compositeRunner) for live
// use; pass a fake runner in tests.
//
// idGen is the test-ID source. Production wiring uses
// time.Now().UnixNano(); tests pass a deterministic counter.
func NewManager(runner Runner, logger *slog.Logger, idGen func() int64) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	if idGen == nil {
		idGen = func() int64 { return time.Now().UnixNano() }
	}
	return &Manager{runner: runner, logger: logger, idGen: idGen}
}

// StartTest acquires the singleton lock and starts a new test if none
// is in flight. If a test IS in flight, the existing handle is
// returned without starting a new run (the idempotency guarantee).
//
// The returned *LiveTest's Done channel closes when the test
// completes (success or error) AND all final fan-out events have
// been delivered to existing subscribers. A subscriber that subscribes
// AFTER Done is closed will still receive the full replay (see
// LiveTest.Subscribe).
func (m *Manager) StartTest(ctx context.Context) (*LiveTest, error) {
	m.mu.Lock()
	if m.active != nil {
		// Idempotent: existing test in flight, return the handle.
		// Caller cannot tell whether they started it or attached.
		// This is what makes Run-now multi-tab transparent.
		existing := m.active
		m.mu.Unlock()
		return existing, nil
	}
	if m.runner == nil {
		m.mu.Unlock()
		return nil, errors.New("livetest: no runner configured")
	}

	id := m.idGen()
	t := &LiveTest{
		id:          id,
		startedAt:   time.Now(),
		samples:     make([]Sample, 0, 64),
		subscribers: make(map[chan Sample]struct{}),
		done:        make(chan struct{}),
		complete:    make(chan struct{}),
	}
	m.active = t
	m.mu.Unlock()

	// Drive the runner asynchronously. The goroutine owns the
	// transition to terminal state (result/error -> done channel
	// closed -> registry's active slot cleared). Nothing else
	// touches m.active.
	go m.driveTest(ctx, t)
	return t, nil
}

// driveTest invokes the runner and broadcasts samples to subscribers.
// On completion (success or error) it stamps the result, closes the
// done channel, and clears m.active so the next StartTest can begin
// a fresh test.
//
// Cleanup order is critical: m.active must be cleared BEFORE
// LiveTest.Done is closed. Tests (and production observers) wait on
// Done then check InProgress; if Done closes first there's a race
// window where InProgress=true even though the test is over. The
// finish call (which closes Done) is therefore deferred until AFTER
// m.active is cleared.
//
// Panic recovery: if the runner panics mid-test, the registry must
// release the singleton lock so subsequent StartTest calls aren't
// permanently blocked. The recovered panic is logged + stamped as
// an error result so subscribers see a clean error event.
func (m *Manager) driveTest(ctx context.Context, t *LiveTest) {
	var (
		res *Result
		err error
	)

	defer func() {
		if r := recover(); r != nil {
			err = errFromPanic(r)
			m.logger.Warn("livetest: runner panicked", "panic", r, "test_id", t.id)
		}
		// Clear the registry slot FIRST so observers waiting on
		// Done() see InProgress()=false the moment Done closes.
		m.mu.Lock()
		m.active = nil
		m.mu.Unlock()
		// Then transition the test to its terminal state, which
		// closes every subscriber channel + Done.
		if err != nil {
			t.finishWithError(err)
		} else {
			t.finishWithResult(res)
		}
	}()

	var samples <-chan Sample
	res, samples, err = m.runner.Run(ctx)
	if err != nil {
		return
	}
	if samples == nil {
		// Runner contract violation — but defend gracefully so the
		// registry doesn't block forever waiting for a nil channel.
		return
	}
	// Drain samples + broadcast each to subscribers. The runner is
	// responsible for closing the channel; this loop returns when
	// it does.
	for s := range samples {
		t.emit(s)
	}
}

// GetLive returns the current in-flight LiveTest if its ID matches.
// Returns (nil, false) if no test is in flight or the ID is for a
// completed test (the registry forgets completed IDs once active
// is cleared).
//
// Note: there's an unavoidable race window between a test completing
// and m.active being cleared. In that window, GetLive may still
// return the just-completed test. Callers that subscribe in that
// window see a fully-populated replay + immediate end event, which
// is the desired behaviour.
func (m *Manager) GetLive(testID int64) (*LiveTest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.id == testID {
		return m.active, true
	}
	return nil, false
}

// InProgress reports whether a test is currently in flight. Used by
// the Prometheus exporter for the nasdoctor_speedtest_in_progress
// gauge.
func (m *Manager) InProgress() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active != nil
}

func errFromPanic(r any) error {
	switch v := r.(type) {
	case error:
		return v
	case string:
		return errors.New(v)
	default:
		return errors.New("livetest: runner panicked")
	}
}
