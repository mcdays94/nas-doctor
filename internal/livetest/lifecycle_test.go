// Lifecycle observer tests for the LiveTestRegistry. Issue #294 R3.
//
// The registry exposes two callback hooks so production wiring can
// reflect test state without the registry caring about Prometheus or
// SQLite directly:
//
//   - StateChange(running bool) — fires synchronously at StartTest
//     (running=true) and at completion (running=false). Wired to the
//     nasdoctor_speedtest_in_progress Prometheus gauge so the gauge
//     flips correctly regardless of whether the test was started by
//     the cron loop OR a manual POST /api/v1/speedtest/run. Closes
//     #294 R3a.
//
//   - Completion(*LiveTest) — fires once after the runner returns and
//     before m.active is cleared. Wired to scheduler persistence so
//     EVERY test (cron OR API-triggered) writes its history row +
//     samples through the same code path. Closes #294 R3b.
package livetest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRegistry_StateChangeObserver_FiresOnStartAndEnd asserts the
// registered observer is called once with running=true at StartTest
// and once with running=false after the test completes. This is the
// gauge-flip contract for R3a.
func TestRegistry_StateChangeObserver_FiresOnStartAndEnd(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &Result{Engine: "speedtest_go"}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	var (
		mu     sync.Mutex
		states []bool
	)
	mgr.RegisterStateChangeObserver(func(running bool) {
		mu.Lock()
		defer mu.Unlock()
		states = append(states, running)
	})

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	// Synchronous after StartTest: running=true must already have fired.
	mu.Lock()
	gotInitial := append([]bool(nil), states...)
	mu.Unlock()
	if len(gotInitial) != 1 || gotInitial[0] != true {
		t.Fatalf("after StartTest: states = %v, want [true]", gotInitial)
	}

	close(runner.done)
	<-lt.Done()

	// Allow the observer goroutine to fire (defer ordering means
	// running=false is invoked just after Done closes).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(states)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	final := append([]bool(nil), states...)
	mu.Unlock()
	if len(final) != 2 {
		t.Fatalf("states = %v, want [true false]", final)
	}
	if final[0] != true || final[1] != false {
		t.Errorf("states = %v, want [true false]", final)
	}
}

// TestRegistry_StateChange_FiresFalseEvenOnRunnerError asserts that
// if Run returns an error immediately (no samples, no result), the
// observer still receives running=false so the in_progress gauge
// returns to 0. Without this, a fast-failing runner would leave the
// gauge stuck at 1 forever.
func TestRegistry_StateChange_FiresFalseEvenOnRunnerError(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.err = errors.New("FetchUserInfo: timeout")
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	var (
		mu     sync.Mutex
		states []bool
	)
	mgr.RegisterStateChangeObserver(func(running bool) {
		mu.Lock()
		states = append(states, running)
		mu.Unlock()
	})

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-lt.Done()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(states)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(states) != 2 || states[0] != true || states[1] != false {
		t.Errorf("states = %v, want [true false] even on runner error", states)
	}
}

// TestRegistry_CompletionHandler_FiresOnSuccess asserts the registered
// completion handler is called once with the completed *LiveTest after
// a successful run. The handler must observe both Result() and
// SnapshotSamples() so production wiring can persist history+samples.
// This is the persistence contract for R3b.
func TestRegistry_CompletionHandler_FiresOnSuccess(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &Result{
		DownloadMbps: 100, UploadMbps: 50, LatencyMs: 8,
		Engine: "speedtest_go",
	}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	completed := make(chan *LiveTest, 1)
	mgr.RegisterCompletionHandler(func(lt *LiveTest) {
		completed <- lt
	})

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	// Push two samples so the handler can verify SnapshotSamples()
	// is non-empty.
	runner.samples <- Sample{Phase: "download", Mbps: 100}
	runner.samples <- Sample{Phase: "upload", Mbps: 50}
	close(runner.done)
	<-lt.Done()

	select {
	case got := <-completed:
		if got != lt {
			t.Errorf("handler got different LiveTest pointer (got=%p, want=%p)", got, lt)
		}
		if got.Result() == nil {
			t.Error("handler invoked but Result() was nil — handler ran before result stamped")
		}
		samples := got.SnapshotSamples()
		if len(samples) != 2 {
			t.Errorf("SnapshotSamples() len = %d, want 2", len(samples))
		}
	case <-time.After(time.Second):
		t.Fatal("completion handler did not fire within 1s")
	}
}

// TestRegistry_CompletionHandler_FiresOnError asserts the handler also
// fires when the runner returns an error — production needs this so
// failed manual /run requests still record a "failed" attempt row,
// matching the cron-path behaviour. lt.Err() must be non-nil when the
// handler runs.
func TestRegistry_CompletionHandler_FiresOnError(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.err = errors.New("network unreachable")
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	completed := make(chan *LiveTest, 1)
	mgr.RegisterCompletionHandler(func(lt *LiveTest) {
		completed <- lt
	})

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-lt.Done()

	select {
	case got := <-completed:
		if got.Err() == nil {
			t.Error("handler invoked but Err() was nil — should be non-nil for error path")
		}
		if got.Result() != nil {
			t.Errorf("Result() = %+v, want nil on error path", got.Result())
		}
	case <-time.After(time.Second):
		t.Fatal("completion handler did not fire within 1s on error path")
	}
}

// TestRegistry_GetLive_GraceWindow asserts that GetLive(testID)
// continues to return the LiveTest for a brief window after completion
// so a late-arriving SSE subscriber can attach, receive the full
// replay, and see the terminal events. Without this grace window, a
// fast-failing runner (issue #294 R3 — showwin's FetchUserInfo timing
// out in <100ms on UAT) clears the registry slot before the browser
// can issue GET /stream/{id}, and the user sees a 404 instead of an
// error event in the stream.
func TestRegistry_GetLive_GraceWindow(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.err = errors.New("fast failure")
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-lt.Done() // runner has returned, slot has been cleared

	// Even after completion, GetLive must still resolve for a
	// brief grace window (PRD #283 alludes to this in the
	// "subscriber attaching after completion" comment in
	// livetest.go). Pin the contract: at least 100ms of grace.
	got, ok := mgr.GetLive(lt.ID())
	if !ok || got != lt {
		t.Fatalf("GetLive immediately after Done() — ok=%v, got=%v, want the just-completed test (grace window)", ok, got)
	}
}

// TestRegistry_StateChangeObserver_NoObserverDoesntPanic asserts the
// observer hook is optional — if no observer is registered, StartTest
// must not panic.
func TestRegistry_StateChangeObserver_NoObserverDoesntPanic(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &Result{Engine: "speedtest_go"}
	mgr := NewManager(runner, quietLogger(), counterIDGen())
	// No RegisterStateChangeObserver call.
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	close(runner.done)
	<-lt.Done()
}

// TestRegistry_CompletionHandler_NoHandlerDoesntPanic mirrors the above
// for the completion hook.
func TestRegistry_CompletionHandler_NoHandlerDoesntPanic(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &Result{Engine: "speedtest_go"}
	mgr := NewManager(runner, quietLogger(), counterIDGen())
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	close(runner.done)
	<-lt.Done()
}

// TestRegistry_StateChange_TrueIsSynchronousWithStart asserts that
// when StartTest returns, the running=true notification has already
// fired. Critical for the gauge: production wiring registers a
// callback that calls metrics.SetSpeedTestInProgress(true), and a
// metrics scrape immediately after POST /run must already see 1.
func TestRegistry_StateChange_TrueIsSynchronousWithStart(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &Result{Engine: "speedtest_go"}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	var trueFired int64
	mgr.RegisterStateChangeObserver(func(running bool) {
		if running {
			atomic.StoreInt64(&trueFired, 1)
		}
	})

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	if atomic.LoadInt64(&trueFired) == 0 {
		t.Fatal("running=true observer not invoked synchronously with StartTest — POST /run may return before in_progress gauge flips")
	}
	close(runner.done)
	<-lt.Done()
}
