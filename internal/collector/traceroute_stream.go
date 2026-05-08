// Streaming traceroute runner — issue #224.
//
// The non-streaming RunMTR (traceroute.go) shells out to `mtr --report
// --report-cycles=N --json` and parses the resulting JSON document
// after the subprocess completes. That works for scheduled checks and
// the synchronous Test button, but produces a 5-15s "dead spinner"
// UX on the settings page Test button — exactly the regression
// surfaced in v0.9.7 UAT of #189 that issue #224 tracks.
//
// RunStreamingMTR provides the live-progress abstraction the new
// /api/v1/service-checks/test-stream endpoint consumes. It runs the
// mtr binary one-cycle-at-a-time and emits cumulative hop tables
// after each cycle so the dashboard's notification panel can render
// rows as they're discovered. The final cycle's hop table is also
// returned as the canonical MTRResult so the SSE handler can emit a
// single terminal `result` event whose shape is byte-identical to
// the synchronous /test endpoint's response — frontend renderers
// can therefore reuse the existing renderServiceCheckDetails helper.
//
// Cancellation: the subprocess is started in its own process group
// via setProcessGroup; on ctx cancellation we send SIGKILL to the
// entire group (negative pid). This is the v0.9.14 #304 pattern —
// avoids the orphaned-grandchild bug where exec.CommandContext kills
// only the direct shell and leaves mtr inheriting the stdout pipe,
// blocking cmd.Wait() on EOF for the natural duration of the
// subprocess. mtr is a pure binary not wrapped by /bin/sh in our
// invocation so the bug is less likely to surface, but the same
// process-group plumbing is the right defensive default and keeps
// the fan-out of subprocess kill semantics consistent with the
// speedtest path.

package collector

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// StreamingMTRUpdate is a single update emitted by the streaming
// runner. Cycle is the 1-indexed cycle number the update came from
// (1..total). Hops is the cumulative hop table after that cycle.
type StreamingMTRUpdate struct {
	Cycle      int      `json:"cycle"`
	TotalCycle int      `json:"total_cycles"`
	Hops       []MTRHop `json:"hops"`
}

// StreamingTracerouteRunner is the function signature the streaming
// /test-stream endpoint depends on. Implementations push per-cycle
// updates onto the returned `updates` channel and a single terminal
// MTRResult onto the returned `final` channel before closing both.
//
// The production implementation is RunStreamingMTR. Tests inject a
// stub via Server.streamingTracerouteRunner so they don't need
// an actual mtr binary.
//
// Contract:
//   - Updates are emitted in cycle order.
//   - When ctx is cancelled, both channels are closed promptly
//     (within ~250ms on the production path, immediately on test
//     stubs). final may close without emitting if cancellation
//     races with completion.
//   - On underlying mtr error, an empty MTRResult is sent on final
//     and `runErr` carries the error; callers should treat any
//     non-nil runErr as "down".
type StreamingTracerouteRunner func(ctx context.Context, target string, cycles int) (updates <-chan StreamingMTRUpdate, final <-chan StreamingTraceFinal)

// StreamingTraceFinal is the terminal value emitted by a streaming
// traceroute run. Exactly one is sent on the final channel before
// it's closed.
type StreamingTraceFinal struct {
	// Result is the canonical MTRResult assembled from the last
	// cycle. Nil when RunErr is non-nil OR when the run was
	// cancelled before any hops were collected.
	Result *MTRResult
	// RunErr is the underlying error from mtr execution, if any.
	// nil on the happy path. Cancellation surfaces here as
	// context.Canceled / context.DeadlineExceeded.
	RunErr error
}

// streamingMTRExecFn is the seam tests use to inject a fake
// per-cycle mtr executor. Default implementation invokes mtr with
// --report-cycles=1 once per cycle so we get fresh stdout per
// invocation and can emit cumulative hops between invocations.
//
// Why one-cycle-per-invocation: real mtr --report-cycles=N flushes
// JSON only when the report is complete (it does not emit per-cycle
// progress). To get genuinely live progress without depending on
// mtr's debug `--raw` mode (which is not stable across versions and
// has been historically prone to format drift), we run the binary N
// times and assemble a cumulative hop table from the most-recent
// run's output. Each run is fast (~1s on healthy networks) so the
// user sees progress every second, which matches the live-progress
// expectation from issue #224.
var streamingMTRExecFn = func(ctx context.Context, target string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "mtr",
		"--report",
		"--report-cycles=1",
		"--json",
		"--no-dns",
		target,
	)
	setProcessGroup(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Watch ctx; on cancellation, kill the whole process group.
	// See v0.9.14 #304 fix-up — exec.CommandContext alone leaves
	// orphan grandchildren when the subprocess re-execs (mtr does
	// not, but the defensive plumbing is cheap and keeps subprocess
	// kill semantics consistent across the codebase).
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			killProcessGroup(cmd)
		case <-done:
		}
	}()
	if err := cmd.Wait(); err != nil {
		// mtr returns non-zero when the network is unreachable AND
		// when stderr carries diagnostic noise; surface stderr in
		// the error so callers (and tests) can render it. Keep the
		// underlying ExitError wrapped so errors.Is(err, ctx.Err())
		// continues to work after ctx cancellation.
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.Bytes(), fmt.Errorf("mtr: %s: %w", msg, err)
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

// RunStreamingMTR runs mtr against target in cycles iterations,
// emitting cumulative hop tables after each cycle. The returned
// channels are closed when the run completes or ctx is cancelled.
//
// cycles == 0 is treated as 5 (matches RunMTR's default).
//
// Process-group SIGKILL on ctx cancellation: see v0.9.14 #304 fix-up.
func RunStreamingMTR(ctx context.Context, target string, cycles int) (<-chan StreamingMTRUpdate, <-chan StreamingTraceFinal) {
	target = strings.TrimSpace(target)
	updates := make(chan StreamingMTRUpdate, 32)
	final := make(chan StreamingTraceFinal, 1)

	if cycles <= 0 {
		cycles = 5
	}

	go func() {
		defer close(updates)
		defer close(final)

		if target == "" {
			final <- StreamingTraceFinal{RunErr: fmt.Errorf("traceroute target is required")}
			return
		}

		var lastResult *MTRResult
		var lastErr error
		for i := 1; i <= cycles; i++ {
			select {
			case <-ctx.Done():
				final <- StreamingTraceFinal{Result: lastResult, RunErr: ctx.Err()}
				return
			default:
			}
			out, err := streamingMTRExecFn(ctx, target)
			if err != nil {
				lastErr = err
				// On error mid-run we still want to surface what
				// we have so far rather than blank-failing. Break
				// out of the loop and let the terminal block emit.
				break
			}
			if len(out) == 0 {
				continue
			}
			res, parseErr := ParseMTRReport(out)
			if parseErr != nil {
				// Parser noise on a partial run is recoverable —
				// the final cycle may produce valid JSON. Don't
				// abort the run; just don't emit this cycle.
				lastErr = parseErr
				continue
			}
			lastResult = res
			lastErr = nil

			// Send the cumulative update. Drop on cancellation
			// so a stuck SSE writer can't pin the goroutine.
			update := StreamingMTRUpdate{
				Cycle:      i,
				TotalCycle: cycles,
				Hops:       res.Hops,
			}
			select {
			case updates <- update:
			case <-ctx.Done():
				final <- StreamingTraceFinal{Result: lastResult, RunErr: ctx.Err()}
				return
			}
		}

		final <- StreamingTraceFinal{Result: lastResult, RunErr: lastErr}
	}()

	return updates, final
}
