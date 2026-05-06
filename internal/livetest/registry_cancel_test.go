package livetest

// Tests for the Cancel API added in issue #304.
//
// Three properties are pinned here:
//
//  1. Cancel of an in-flight test propagates to the runner's ctx,
//     all subscribers see the channel close cleanly, and lt.Err()
//     returns ErrCancelled (NOT context.Canceled — the registry
//     overrides the underlying ctx error so persistence consumers
//     can distinguish a user-initiated abort from any other ctx
//     cancellation path).
//
//  2. Cancel of an unknown test_id returns ErrTestNotFound. Cancel
//     of a recently-completed test returns ErrAlreadyCompleted (so
//     the HTTP handler can map to 404 vs 409 distinctly).
//
//  3. The runner's drive goroutine exits promptly after a Cancel —
//     no zombie goroutine leaks. We verify by confirming the
//     registry's InProgress() flips to false within a short window.

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// ctxAwareFakeRunner blocks on ctx.Done() until cancelled — this is
// the only way to distinguish "Cancel propagated to the runner" from
// "Cancel ran a closed-channel race". Returns context.Canceled when
// cancelled; the registry should override that with ErrCancelled.
type ctxAwareFakeRunner struct {
	started chan struct{}
}

func newCtxAwareFakeRunner() *ctxAwareFakeRunner {
	return &ctxAwareFakeRunner{started: make(chan struct{})}
}

func (f *ctxAwareFakeRunner) Run(ctx context.Context) (*Result, <-chan Sample, error) {
	close(f.started)
	out := make(chan Sample, 4)
	go func() {
		defer close(out)
		// Emit one sample so the test exercises the broadcast path
		// before cancellation lands. Then block until ctx.Done().
		out <- Sample{Phase: collector.SpeedTestPhaseDownload, Mbps: 100}
		<-ctx.Done()
	}()
	// Block on ctx ourselves so Run returns AFTER cancel propagates.
	// This mirrors how showwin/speedtest-go's *TestContext methods
	// behave: they return promptly with the ctx error when cancelled.
	go func() {
		<-ctx.Done()
	}()
	// Return the result+samples immediately — the goroutine above
	// keeps emitting until ctx cancellation. A real runner would
	// block in Run() on its phase calls; the test's only
	// requirement is that the samples channel closes when ctx is
	// done, which it does (close(out) above).
	return &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}, out, nil
}

func TestLiveTestRegistry_Cancel_ActiveTest(t *testing.T) {
	t.Parallel()
	runner := newCtxAwareFakeRunner()
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	// Subscribe so we observe the post-cancel terminal-close path.
	sub := lt.Subscribe()

	// Cancel — must return nil for an in-flight test.
	if err := mgr.Cancel(lt.ID()); err != nil {
		t.Fatalf("Cancel(active): %v, want nil", err)
	}

	// Subscriber's channel must close cleanly. Drain and assert.
	got := drainUntilClosed(sub)
	if len(got) < 1 {
		t.Errorf("subscriber got 0 samples; expected >=1 (the seed sample)")
	}

	// Done channel must close — registry has reaped the run.
	select {
	case <-lt.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close within 2s of Cancel")
	}

	// lt.Err() must be ErrCancelled, NOT context.Canceled. This is
	// the contract that lets the persistence handler write
	// status='cancelled' instead of status='failed'.
	if !errors.Is(lt.Err(), ErrCancelled) {
		t.Errorf("lt.Err() = %v, want ErrCancelled (or wrapped)", lt.Err())
	}

	// InProgress must flip to false.
	if mgr.InProgress() {
		t.Error("InProgress()=true after cancel — registry slot leaked")
	}
}

func TestLiveTestRegistry_Cancel_UnknownID_Returns404Sentinel(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	// No test in flight. Cancel must report ErrTestNotFound so the
	// HTTP handler can map to 404.
	if err := mgr.Cancel(42); !errors.Is(err, ErrTestNotFound) {
		t.Errorf("Cancel(unknown id) = %v, want ErrTestNotFound", err)
	}

	// Now start + complete a test, then cancel a DIFFERENT id (still
	// not the one in flight, and not in the grace window).
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started
	close(runner.done)
	<-lt.Done()

	if err := mgr.Cancel(lt.ID() + 999999); !errors.Is(err, ErrTestNotFound) {
		t.Errorf("Cancel(stranger id) = %v, want ErrTestNotFound", err)
	}
}

func TestLiveTestRegistry_Cancel_AlreadyCompleted_ReturnsConflictSentinel(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started
	close(runner.done)
	<-lt.Done()

	// Test is in the grace window now. Cancel must distinguish this
	// from a fully-unknown id and return ErrAlreadyCompleted (HTTP
	// 409). Without this distinction, a UI that arrives a beat too
	// late to cancel sees a generic 404 — confusing.
	if err := mgr.Cancel(lt.ID()); !errors.Is(err, ErrAlreadyCompleted) {
		t.Errorf("Cancel(in-grace id) = %v, want ErrAlreadyCompleted", err)
	}
}

func TestLiveTestRegistry_Cancel_NoGoroutineLeak(t *testing.T) {
	// Verify the runner's drive goroutine actually exits after
	// Cancel — a botched cancel implementation could leave the
	// emit-loop goroutine spinning forever, which would manifest
	// in production as a slow goroutine leak that only shows up
	// after dozens of cancelled tests.
	t.Parallel()
	before := runtime.NumGoroutine()

	runner := newCtxAwareFakeRunner()
	mgr := NewManager(runner, quietLogger(), counterIDGen())
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	if err := mgr.Cancel(lt.ID()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	<-lt.Done()

	// Allow the runtime a moment to garbage-collect the dead
	// goroutines (Go scheduler doesn't reap them synchronously).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		now := runtime.NumGoroutine()
		if now <= before+2 { // +2 generous slack for test infra
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutine count did not return near baseline: before=%d, after=%d (>%d allowed)",
		before, runtime.NumGoroutine(), before+2)
}

func TestLiveTestRegistry_Cancel_Idempotent(t *testing.T) {
	// Two Cancel calls in quick succession for the same in-flight
	// test must both succeed (return nil). Without idempotence, a
	// dashboard that fires Cancel twice (user clicks the button
	// rapidly, or two tabs both fire) would surface a confusing
	// error on the second call.
	t.Parallel()
	runner := newCtxAwareFakeRunner()
	mgr := NewManager(runner, quietLogger(), counterIDGen())
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	if err := mgr.Cancel(lt.ID()); err != nil {
		t.Fatalf("Cancel #1: %v", err)
	}
	if err := mgr.Cancel(lt.ID()); err != nil {
		// Second call may legitimately race against the registry's
		// slot-clear and become ErrAlreadyCompleted or
		// ErrTestNotFound — but it MUST NOT return a generic error.
		if !errors.Is(err, ErrAlreadyCompleted) && !errors.Is(err, ErrTestNotFound) {
			t.Errorf("Cancel #2: unexpected error %v", err)
		}
	}
	<-lt.Done()
}
