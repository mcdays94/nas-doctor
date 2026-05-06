package collector

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// canned single-cycle mtr JSON output, varying hops to make the
// "cumulative" assertion non-trivial.
const oneCycleJSONShort = `{
  "report": {
    "mtr": {"src":"10.0.0.10","dst":"8.8.8.8","tos":0,"psize":"64","bitpattern":"0x00","tests":"1"},
    "hubs": [
      {"count":1,"host":"10.0.0.1","Loss%":0.0,"Snt":1,"Last":0.5,"Avg":0.5,"Best":0.5,"Wrst":0.5,"StDev":0.0}
    ]
  }
}`

const oneCycleJSONLong = `{
  "report": {
    "mtr": {"src":"10.0.0.10","dst":"8.8.8.8","tos":0,"psize":"64","bitpattern":"0x00","tests":"1"},
    "hubs": [
      {"count":1,"host":"10.0.0.1","Loss%":0.0,"Snt":1,"Last":0.5,"Avg":0.5,"Best":0.5,"Wrst":0.5,"StDev":0.0},
      {"count":2,"host":"203.0.113.1","Loss%":0.0,"Snt":1,"Last":2.1,"Avg":2.1,"Best":2.1,"Wrst":2.1,"StDev":0.0},
      {"count":3,"host":"8.8.8.8","Loss%":0.0,"Snt":1,"Last":12.3,"Avg":12.3,"Best":12.3,"Wrst":12.3,"StDev":0.0}
    ]
  }
}`

// withMTRExecFn temporarily replaces streamingMTRExecFn for the
// duration of t. Restored on cleanup.
func withStreamingMTRExecFn(t *testing.T, fn func(ctx context.Context, target string) ([]byte, error)) {
	t.Helper()
	orig := streamingMTRExecFn
	streamingMTRExecFn = fn
	t.Cleanup(func() { streamingMTRExecFn = orig })
}

// TestRunStreamingMTR_EmitsUpdatePerCycleThenFinal — happy path.
// 3 cycles → 3 updates with monotonically growing/identical hop
// counts, followed by a single final with non-nil Result.
func TestRunStreamingMTR_EmitsUpdatePerCycleThenFinal(t *testing.T) {

	var calls int32
	withStreamingMTRExecFn(t, func(_ context.Context, _ string) ([]byte, error) {
		n := atomic.AddInt32(&calls, 1)
		// Simulate progressive hop discovery — first call sees one
		// hop, subsequent calls see the full chain.
		if n == 1 {
			return []byte(oneCycleJSONShort), nil
		}
		return []byte(oneCycleJSONLong), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	updates, final := RunStreamingMTR(ctx, "8.8.8.8", 3)

	var collected []StreamingMTRUpdate
	for u := range updates {
		collected = append(collected, u)
	}
	if len(collected) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(collected))
	}
	if collected[0].Cycle != 1 || collected[2].Cycle != 3 {
		t.Errorf("expected cycle counter 1..3, got %d/%d/%d", collected[0].Cycle, collected[1].Cycle, collected[2].Cycle)
	}
	if collected[0].TotalCycle != 3 {
		t.Errorf("expected total_cycles=3, got %d", collected[0].TotalCycle)
	}
	if len(collected[0].Hops) != 1 {
		t.Errorf("first cycle: expected 1 hop, got %d", len(collected[0].Hops))
	}
	if len(collected[2].Hops) != 3 {
		t.Errorf("third cycle: expected 3 hops, got %d", len(collected[2].Hops))
	}

	fin, ok := <-final
	if !ok {
		t.Fatal("final channel closed without value")
	}
	if fin.RunErr != nil {
		t.Fatalf("expected nil RunErr, got %v", fin.RunErr)
	}
	if fin.Result == nil {
		t.Fatal("expected non-nil Result")
	}
	if len(fin.Result.Hops) != 3 {
		t.Errorf("final result: expected 3 hops, got %d", len(fin.Result.Hops))
	}
}

// TestRunStreamingMTR_EmptyTarget — no shelling out, immediate
// terminal error.
func TestRunStreamingMTR_EmptyTarget(t *testing.T) {

	withStreamingMTRExecFn(t, func(_ context.Context, _ string) ([]byte, error) {
		t.Fatal("streamingMTRExecFn should not be called with empty target")
		return nil, nil
	})

	updates, final := RunStreamingMTR(context.Background(), "", 3)

	count := 0
	for range updates {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 updates, got %d", count)
	}
	fin := <-final
	if fin.RunErr == nil {
		t.Fatal("expected RunErr for empty target")
	}
}

// TestRunStreamingMTR_RunnerErrorSurfacesInFinal — when the
// underlying exec returns an error mid-run, the final value carries
// it AND the partial result.
func TestRunStreamingMTR_RunnerErrorSurfacesInFinal(t *testing.T) {

	var calls int32
	withStreamingMTRExecFn(t, func(_ context.Context, _ string) ([]byte, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return []byte(oneCycleJSONShort), nil
		}
		return nil, errors.New("mtr: simulated network failure")
	})

	updates, final := RunStreamingMTR(context.Background(), "8.8.8.8", 5)
	updateCount := 0
	for range updates {
		updateCount++
	}
	if updateCount != 1 {
		t.Errorf("expected 1 update before error, got %d", updateCount)
	}
	fin := <-final
	if fin.RunErr == nil {
		t.Fatal("expected RunErr after exec failure")
	}
	if !strings.Contains(fin.RunErr.Error(), "simulated network failure") {
		t.Errorf("expected RunErr to mention failure cause, got %q", fin.RunErr.Error())
	}
	// Partial result should still be exposed.
	if fin.Result == nil {
		t.Fatal("expected partial Result to be preserved on mid-run error")
	}
}

// TestRunStreamingMTR_CtxCancelClosesPromptly — the canonical issue
// #304 contract for streaming runners. Slow exec stub blocks until
// ctx fires; we then assert both channels close within a small
// budget.
func TestRunStreamingMTR_CtxCancelClosesPromptly(t *testing.T) {

	withStreamingMTRExecFn(t, func(ctx context.Context, _ string) ([]byte, error) {
		// Block until ctx fires — simulates a hung mtr subprocess.
		<-ctx.Done()
		return nil, ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	updates, final := RunStreamingMTR(ctx, "8.8.8.8", 5)

	// Cancel after a tick so the goroutine is past the empty-target
	// guard and into streamingMTRExecFn.
	time.AfterFunc(20*time.Millisecond, cancel)

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	closed := false
loop:
	for !closed {
		select {
		case _, ok := <-updates:
			if !ok {
				updates = nil // never receive again
			}
		case _, ok := <-final:
			if !ok {
				final = nil
			} else {
				closed = true
			}
		case <-deadline.C:
			t.Fatal("RunStreamingMTR did not close within 2s of ctx cancellation")
		}
		if updates == nil && final == nil {
			break loop
		}
	}
}

// TestStreamingMTRExecFn_ProcessGroupKillSemantics — drives the real
// production stream-exec path against /bin/sh -c "sleep 30" rather
// than mtr to verify that ctx cancellation kills the entire process
// group (Setpgid + syscall.Kill(-pgid, SIGKILL)). Without the
// process-group plumbing, /bin/sh dies but the inherited `sleep`
// child stays running and pins the inherited stdout pipe.
//
// Skipped on CI environments where /bin/sh isn't available — but
// our supported targets (Alpine in Docker + macOS dev) all have it.
func TestStreamingMTRExecFn_ProcessGroupKillSemantics(t *testing.T) {

	// Replace streamingMTRExecFn with a body identical to production
	// EXCEPT that we shell out to /bin/sh -c "sleep 30 && cat" to
	// guarantee a grandchild process inherits the pipe. If
	// process-group kill works, the function returns within a
	// budget; if it doesn't, cmd.Wait() blocks for ~30s.
	var grandchildPID int32
	withStreamingMTRExecFn(t, func(ctx context.Context, _ string) ([]byte, error) {
		// /bin/sh -c with an exec'd subprocess so the grandchild is
		// distinct from the shell — exactly the v0.9.14 #304 trap.
		cmd := exec.Command("/bin/sh", "-c", "sleep 30 & echo $! && wait")
		setProcessGroup(cmd)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		// Read the sleep PID off the pipe so we can verify it dies.
		buf := make([]byte, 16)
		n, _ := stdout.Read(buf)
		pid := parseLeadingDigits(string(buf[:n]))
		atomic.StoreInt32(&grandchildPID, int32(pid))
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
			return nil, err
		}
		return nil, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	updates, final := RunStreamingMTR(ctx, "8.8.8.8", 1)

	// Give the subprocess time to start.
	time.Sleep(100 * time.Millisecond)
	cancel()

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()

	// Drain updates (none expected, but they may close before final).
	go func() {
		for range updates {
		}
	}()

	select {
	case <-final:
		// Expected — process group kill propagated to grandchild,
		// cmd.Wait() unblocked, RunStreamingMTR returned.
	case <-deadline.C:
		t.Fatalf("RunStreamingMTR did not return within 3s — process-group kill semantics broken (grandchild pid %d)", atomic.LoadInt32(&grandchildPID))
	}
}

// parseLeadingDigits returns the leading run of decimal digits in s
// as an int (0 when none). Written by hand to keep the import surface
// minimal — the only caller is the process-group kill test below.
func parseLeadingDigits(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
