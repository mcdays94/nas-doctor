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
	"fmt"
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
//
// State-change + completion observers are registered ONCE during
// production wiring (cmd/nas-doctor/main.go) and are read on every
// test lifecycle event without acquiring the registry mutex; the
// observer slice is treated as effectively-immutable after registration
// to avoid lock-ordering hazards (driveTest defer chain holds m.mu
// briefly to clear the slot, then calls observers without it).
type Manager struct {
	runner Runner
	logger *slog.Logger
	idGen  func() int64

	mu       sync.Mutex
	active   *LiveTest // nil when no test in flight
	graceLT  *LiveTest // last completed test, available via GetLive within graceWindow
	graceAt  time.Time // when graceLT completed; cleared after graceWindow elapses

	// Observers registered before any tests run. Production wires
	// these in main.go; tests wire their own. No mutex protection
	// because the registration calls happen before any StartTest
	// in production AND before parallel goroutines exist in tests.
	stateChangeObservers []func(running bool)
	completionHandlers   []func(*LiveTest)
}

// graceWindow is how long a just-completed test remains discoverable
// via GetLive. Sized so a fast-failing runner (e.g. showwin's
// FetchUserInfo erroring in <100ms — issue #294 R3 root cause on UAT)
// can still hand a LiveTest off to a late-arriving SSE client. The
// client gets the full replay (including the error event) and a
// closed channel, which is the desired UX (error visible in stream)
// rather than a 404 (confusing "test never existed" surface).
const graceWindow = 5 * time.Second

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

// RegisterStateChangeObserver registers a callback invoked at every
// test-lifecycle transition: running=true synchronously inside
// StartTest (BEFORE the runner goroutine launches and BEFORE
// StartTest returns), running=false on the runner goroutine after
// the registry slot is cleared. Issue #294 R3a — production wires
// this to notifier.Metrics.SetSpeedTestInProgress so the gauge flips
// regardless of whether the test was triggered by cron or by a
// manual POST /api/v1/speedtest/run.
//
// Observers are called in registration order. Panic-safety: if an
// observer panics, the panic is logged and remaining observers still
// fire (defensive against careless production wiring).
func (m *Manager) RegisterStateChangeObserver(fn func(running bool)) {
	if fn == nil {
		return
	}
	m.stateChangeObservers = append(m.stateChangeObservers, fn)
}

// RegisterCompletionHandler registers a callback invoked exactly once
// after the runner returns and BEFORE the registry slot is cleared.
// The LiveTest's Result()/Err()/SnapshotSamples() are all readable
// at handler invocation time. Issue #294 R3b — production wires this
// to scheduler.handleSpeedTestResultWithSamples so EVERY completed
// test (cron OR API-triggered) writes its history row + per-sample
// telemetry through the same code path.
//
// Multiple handlers are called in registration order. Panic-safety
// matches state-change observers.
func (m *Manager) RegisterCompletionHandler(fn func(*LiveTest)) {
	if fn == nil {
		return
	}
	m.completionHandlers = append(m.completionHandlers, fn)
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
	// Clear any prior grace test now that a fresh one is active —
	// the grace window only applies to the most-recently-completed
	// test, not a stale handle.
	m.graceLT = nil
	m.graceAt = time.Time{}
	m.mu.Unlock()

	m.logger.Info("livetest: start",
		"test_id", t.id,
		"caller", callerFromContext(ctx),
	)

	// Fire running=true BEFORE the runner goroutine launches so the
	// in_progress gauge is set to 1 synchronously with StartTest's
	// return. A metrics scrape immediately after POST /run will
	// observe in_progress=1 even if the runner hasn't actually
	// started yet — the registry IS in progress from the caller's
	// perspective.
	m.notifyStateChange(true)

	// Drive the runner asynchronously. The goroutine owns the
	// transition to terminal state (result/error -> done channel
	// closed -> registry's active slot cleared). Nothing else
	// touches m.active.
	go m.driveTest(ctx, t)
	return t, nil
}

// notifyStateChange invokes every registered state-change observer
// with the given running flag. Panic in any observer is logged and
// recovered; remaining observers still fire.
func (m *Manager) notifyStateChange(running bool) {
	for _, fn := range m.stateChangeObservers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Warn("livetest: state-change observer panicked",
						"running", running, "panic", r)
				}
			}()
			fn(running)
		}()
	}
}

// notifyCompletion invokes every registered completion handler with
// the just-completed LiveTest. Panic in any handler is logged and
// recovered.
func (m *Manager) notifyCompletion(t *LiveTest) {
	for _, fn := range m.completionHandlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Warn("livetest: completion handler panicked",
						"test_id", t.id, "panic", r)
				}
			}()
			fn(t)
		}()
	}
}

// callerFromContext extracts a free-form "caller" tag from the
// context (set via WithCaller). Used purely for structured logs so
// future UAT can tell at a glance whether a test came from cron, the
// API, or a test fixture. Returns "" if nothing was attached.
func callerFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(callerCtxKey{}).(string); ok {
		return v
	}
	return ""
}

type callerCtxKey struct{}

// WithCaller annotates the context with a caller tag. Production
// wiring sets this to "cron" / "api" so /var/log entries from
// livetest are filterable. Tests can leave it unset.
func WithCaller(ctx context.Context, caller string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callerCtxKey{}, caller)
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
		res        *Result
		err        error
		samplesSeen int
	)

	defer func() {
		if r := recover(); r != nil {
			err = errFromPanic(r)
			m.logger.Warn("livetest: runner panicked", "panic", r, "test_id", t.id)
		}
		// Stamp terminal state on the LiveTest. After this returns,
		// Result()/Err()/SnapshotSamples() are all readable but the
		// Done channel is NOT yet closed. This split lets the
		// completion handler run BEFORE any goroutine waiting on
		// <-lt.Done() proceeds — critical so the cron path observes
		// "Done unblocked → persistence already happened" instead of
		// racing against the registry's persistence callback.
		// Issue #294 R3b.
		t.stampTerminal(res, err)
		// Fire completion handlers synchronously. Production wires
		// this to scheduler persistence (history row + samples).
		// Both cron- and API-triggered tests therefore produce
		// identical persistence side effects via the SAME callback.
		m.notifyCompletion(t)
		// Now broadcast subscriber-channel close + Done close. SSE
		// clients waiting on the channel get the buffered events +
		// terminal close; cron callers waiting on Done() unblock
		// AFTER the persistence handler has returned.
		t.closeSubscribersAndDone()
		// Clear the active slot + stash for the grace window so
		// late SSE clients can still attach. GetLive checks both
		// active and graceLT.
		m.mu.Lock()
		m.active = nil
		m.graceLT = t
		m.graceAt = time.Now()
		m.mu.Unlock()
		// Broadcast running=false. By firing AFTER the slot is
		// cleared, callers asserting "gauge==0 implies not in
		// progress" stay correct (Prometheus scrapes during the
		// race window will see in_progress=1 AND active==nil but
		// the gauge clamps quickly to 0 once notify fires).
		m.notifyStateChange(false)

		var resultSummary string
		if res != nil {
			resultSummary = "success"
		} else if err != nil {
			resultSummary = "error"
		} else {
			resultSummary = "unknown"
		}
		m.logger.Info("livetest: end",
			"test_id", t.id,
			"samples_seen", samplesSeen,
			"result", resultSummary,
			"err", err,
		)
	}()

	m.logger.Info("livetest: drive",
		"test_id", t.id,
		"runner_type", fmt.Sprintf("%T", m.runner),
	)

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
		samplesSeen++
	}
}

// GetLive returns the current in-flight LiveTest if its ID matches.
// Returns (nil, false) if no test is in flight, the ID doesn't match,
// AND no recently-completed test is within the grace window.
//
// The grace window (graceWindow, currently 5s) keeps a just-completed
// test discoverable so a late-arriving SSE subscriber can still
// attach, receive the full replay (including the terminal event),
// and exit cleanly. Without this, a fast-failing runner — e.g.
// showwin's FetchUserInfo erroring in <100ms on UAT (issue #294 R3
// root cause) — clears the registry slot before the browser can
// issue GET /stream/{id}, and the user sees a 404 instead of the
// error event in the stream.
func (m *Manager) GetLive(testID int64) (*LiveTest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.id == testID {
		return m.active, true
	}
	if m.graceLT != nil && m.graceLT.id == testID {
		if time.Since(m.graceAt) <= graceWindow {
			return m.graceLT, true
		}
		// Grace expired — clear so subsequent calls are O(1).
		m.graceLT = nil
		m.graceAt = time.Time{}
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
