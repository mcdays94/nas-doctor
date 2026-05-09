// Streaming speed-test runner — issue #318.
//
// PR #317 introduced /api/v1/service-checks/test-stream as an SSE
// endpoint that streams per-cycle hop progress for type=traceroute
// Test invocations. Issue #318 extends the same endpoint to support
// type=speed, replacing the dashboard Test button's "dead spinner"
// during a 30-60s Ookla run with live phase + sample feedback.
//
// Why a NEW abstraction rather than reusing livetest.Registry:
//
//   - livetest.Registry is the dashboard speed-test card's singleton
//     state machine: one in-flight test at a time, multi-subscriber
//     fan-out for replay-on-attach across browser tabs, history-row
//     persistence on completion via registered completion handlers,
//     Prometheus in-progress gauge management. None of those are
//     desirable on the Test button path — Test invocations are
//     ephemeral, don't compete with the dashboard's own scheduled
//     run, and don't write history.
//
//   - Mixing Test-button traffic into the singleton would mean
//     clicking Test on the settings page collapses onto whatever
//     test is currently running on the dashboard (or the next
//     scheduled cron tick) instead of running its own throwaway
//     measurement. That breaks the existing UX expectation from
//     #170 (Test = run now, ad hoc).
//
// What we DO reuse: the underlying SpeedTestRunner engine (composite
// of speedtest-go primary + Ookla CLI fallback, defined in
// speedtest_runner.go). The streaming wrapper just calls Run(ctx),
// forwards samples to a channel, and emits a single terminal
// SpeedTestResult. Process-group SIGKILL on cancellation is handled
// inside the engine's Ookla path (v0.9.14 #304); cancelling the ctx
// passed in here propagates through to the subprocess.

package collector

import (
	"context"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// StreamingSpeedFinal is the terminal value emitted by a streaming
// speed-test run. Exactly one is sent on the final channel before
// it's closed.
type StreamingSpeedFinal struct {
	// Result is the canonical SpeedTestResult assembled by the
	// underlying engine. Nil when RunErr is non-nil.
	Result *internal.SpeedTestResult
	// RunErr is the underlying error from the engine, if any.
	// nil on the happy path. Cancellation surfaces here as
	// context.Canceled / context.DeadlineExceeded.
	RunErr error
}

// StreamingSpeedTestRunner is the function signature the streaming
// /test-stream endpoint depends on for type=speed checks.
// Implementations push samples onto the returned `updates` channel
// and a single terminal value onto the returned `final` channel
// before closing both.
//
// Contract — mirrors StreamingTracerouteRunner verbatim:
//   - Samples are emitted in phase order (latency → download →
//     upload). The Phase field on each sample drives SSE-side
//     phase_change derivation.
//   - When ctx is cancelled, both channels are closed promptly.
//     final may emit a value carrying ctx.Err() as RunErr or close
//     without emitting if cancellation races with completion.
//   - On engine error, final emits a StreamingSpeedFinal with
//     RunErr non-nil and Result nil; callers should treat any
//     non-nil RunErr as "down".
//
// The production implementation is RunStreamingSpeedTest. Tests
// inject a stub via Server.streamingSpeedTestRunner so they don't
// need an actual speedtest tool installed. Issue #318.
type StreamingSpeedTestRunner func(ctx context.Context) (updates <-chan SpeedTestSample, final <-chan StreamingSpeedFinal)

// RunStreamingSpeedTest is the production StreamingSpeedTestRunner.
// Adapts the existing composite SpeedTestRunner (speedtest-go primary
// + Ookla CLI fallback) to the streaming channel-pair shape used by
// the /test-stream SSE handler.
//
// Behaviour:
//   - Latches the engine result + samples channel from Run(ctx).
//   - Forwards each sample onto `updates` until the engine's
//     samples channel closes.
//   - Emits one StreamingSpeedFinal carrying the engine result OR
//     the engine error onto `final`, then closes both channels.
//
// Cancellation propagates through the ctx wired into engine.Run; the
// Ookla CLI path uses process-group SIGKILL (v0.9.14 #304) so a
// browser closing the EventSource promptly kills the subprocess
// instead of letting it run its full 30-60s window.
func RunStreamingSpeedTest(ctx context.Context) (<-chan SpeedTestSample, <-chan StreamingSpeedFinal) {
	return runStreamingSpeedTestWithRunner(ctx, defaultStreamingSpeedTestRunner())
}

// runStreamingSpeedTestWithRunner is the seam that lets tests inject
// a fake SpeedTestRunner without exposing the channel-pair shape.
// Production wiring goes through RunStreamingSpeedTest which builds
// the composite at first call.
func runStreamingSpeedTestWithRunner(ctx context.Context, runner SpeedTestRunner) (<-chan SpeedTestSample, <-chan StreamingSpeedFinal) {
	// Buffered enough so a fast-emitting engine doesn't block on
	// our forwarder; sized to match livetest.SubscribeBufferSize
	// for parity with the dashboard SSE consumer.
	updates := make(chan SpeedTestSample, 64)
	final := make(chan StreamingSpeedFinal, 1)

	go func() {
		defer close(updates)
		defer close(final)

		if runner == nil {
			final <- StreamingSpeedFinal{
				RunErr: errStreamingSpeedNoRunner,
			}
			return
		}

		res, samples, err := runner.Run(ctx)
		if err != nil {
			final <- StreamingSpeedFinal{RunErr: err}
			return
		}

		// Forward samples until the engine closes the channel.
		// Defensive against a runner that returns nil samples
		// despite the contract — treat as "no live samples,
		// straight to final" rather than blocking forever.
		if samples != nil {
			for {
				select {
				case s, ok := <-samples:
					if !ok {
						samples = nil
						break
					}
					select {
					case updates <- s:
					case <-ctx.Done():
						// Best-effort emit final with
						// ctx.Err() so SSE consumers see
						// a clean error event. We still
						// drain the engine's samples
						// channel implicitly when this
						// goroutine returns and the GC
						// reclaims it.
						final <- StreamingSpeedFinal{RunErr: ctx.Err()}
						return
					}
				case <-ctx.Done():
					final <- StreamingSpeedFinal{RunErr: ctx.Err()}
					return
				}
				if samples == nil {
					break
				}
			}
		}

		final <- StreamingSpeedFinal{Result: res}
	}()

	return updates, final
}

// errStreamingSpeedNoRunner is the sentinel emitted when no engine is
// configured — defensive guard so a misconfigured server returns a
// clean error event instead of hanging the EventSource indefinitely.
var errStreamingSpeedNoRunner = streamingSpeedErr("streaming speed runner: no engine configured")

type streamingSpeedErr string

func (e streamingSpeedErr) Error() string { return string(e) }

// defaultStreamingSpeedTestRunner constructs the production composite
// runner used by RunStreamingSpeedTest. Lazily built so tests that
// inject their own runner via runStreamingSpeedTestWithRunner do not
// pay the cost of probing for the upstream speedtest-go library.
func defaultStreamingSpeedTestRunner() SpeedTestRunner {
	primary := NewSpeedTestGoRunner()
	fallback := NewOoklaCLIRunner()
	return NewCompositeSpeedTestRunner(primary, fallback)
}
