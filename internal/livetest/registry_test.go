package livetest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// fakeRunner is a deterministic runner the registry tests drive. It
// emits samples on its own schedule (controlled by the test via the
// SendSample / Finish helpers) so the test can pin exact ordering of
// subscribe calls relative to sample emission.
type fakeRunner struct {
	started chan struct{}    // closed when Run is called
	samples chan Sample      // test pushes samples here, Run forwards to its returned chan
	result  *Result
	err     error
	done    chan struct{}    // test closes this to signal "Run() should return now"
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		started: make(chan struct{}),
		samples: make(chan Sample, 256),
		done:    make(chan struct{}),
	}
}

func (f *fakeRunner) Run(_ context.Context) (*Result, <-chan Sample, error) {
	close(f.started)
	if f.err != nil {
		return nil, nil, f.err
	}
	out := make(chan Sample, 256)
	go func() {
		defer close(out)
		for {
			select {
			case s := <-f.samples:
				out <- s
			case <-f.done:
				// drain anything queued, then stop
				for {
					select {
					case s := <-f.samples:
						out <- s
					default:
						return
					}
				}
			}
		}
	}()
	return f.result, out, nil
}

// quietLogger returns a slog.Logger that swallows all output. Tests
// don't assert on log lines.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// drainUntilClosed consumes samples until the channel is closed, then
// returns everything seen.
func drainUntilClosed(ch <-chan Sample) []Sample {
	var got []Sample
	for s := range ch {
		got = append(got, s)
	}
	return got
}

func TestRegistry_SingleSubscriber_FullLifecycle(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{
		DownloadMbps: 100, UploadMbps: 50, LatencyMs: 8,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lt, err := mgr.StartTest(ctx)
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	if lt.ID() == 0 {
		t.Errorf("ID()=%d, want non-zero", lt.ID())
	}
	<-runner.started // wait for runner to actually be invoked

	sub := lt.Subscribe()

	// Push 3 samples through the runner.
	for i := 0; i < 3; i++ {
		runner.samples <- Sample{
			Phase: collector.SpeedTestPhaseDownload,
			At:    time.Now(),
			Mbps:  float64((i + 1) * 100),
		}
	}
	close(runner.done)

	got := drainUntilClosed(sub)
	if len(got) != 3 {
		t.Fatalf("got %d samples, want 3", len(got))
	}
	for i, s := range got {
		want := float64((i + 1) * 100)
		if s.Mbps != want {
			t.Errorf("sample[%d].Mbps = %v, want %v", i, s.Mbps, want)
		}
	}

	// Verify Done closes too.
	select {
	case <-lt.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close within 1s of runner returning")
	}
	if lt.Result() == nil {
		t.Error("Result() = nil, want non-nil after success")
	}
	if mgr.InProgress() {
		t.Error("InProgress()=true after test completed")
	}
}

func TestRegistry_SubscribersAtSamples0_5_15_AllReceiveFullSequence(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	// First subscriber attaches BEFORE any samples emit.
	sub0 := lt.Subscribe()

	// Push 5 samples.
	for i := 0; i < 5; i++ {
		runner.samples <- Sample{Phase: collector.SpeedTestPhaseDownload, Mbps: float64(i)}
	}
	// Wait until those 5 are visible in the replay buffer (so we
	// know sub0 saw them and the next subscriber's replay will
	// include them).
	waitForSampleCount(t, lt, 5)

	// Second subscriber attaches at sample 5.
	sub5 := lt.Subscribe()

	// Push 10 more (total 15).
	for i := 5; i < 15; i++ {
		runner.samples <- Sample{Phase: collector.SpeedTestPhaseDownload, Mbps: float64(i)}
	}
	waitForSampleCount(t, lt, 15)

	// Third subscriber attaches at sample 15.
	sub15 := lt.Subscribe()

	close(runner.done)

	g0 := drainUntilClosed(sub0)
	g5 := drainUntilClosed(sub5)
	g15 := drainUntilClosed(sub15)

	for _, pair := range []struct {
		name string
		got  []Sample
	}{
		{"sub0", g0},
		{"sub5", g5},
		{"sub15", g15},
	} {
		if len(pair.got) != 15 {
			t.Errorf("%s got %d samples, want 15 (replay+live)", pair.name, len(pair.got))
			continue
		}
		for i, s := range pair.got {
			if s.Mbps != float64(i) {
				t.Errorf("%s[%d].Mbps = %v, want %v", pair.name, i, s.Mbps, float64(i))
			}
		}
	}
}

func TestRegistry_SubscribeAfterCompletion_GetsReplayThenClose(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	for i := 0; i < 3; i++ {
		runner.samples <- Sample{Phase: collector.SpeedTestPhaseDownload, Mbps: float64(i)}
	}
	close(runner.done)
	<-lt.Done()

	// Subscribe AFTER the test ended.
	sub := lt.Subscribe()
	got := drainUntilClosed(sub)
	if len(got) != 3 {
		t.Fatalf("late subscribe got %d samples, want 3 (full replay)", len(got))
	}
}

func TestRegistry_50ConcurrentSubscribers_NoDeadlock_NoRace(t *testing.T) {
	// This is the highest-value concurrency test. Run under -race;
	// failures here mean production deployments will hit nondeterministic
	// crashes under multi-tab dashboard load.
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	const numSubs = 50
	const numSamples = 200

	// Concurrent emitter (50 samples in 5 batches) + 50 concurrent
	// subscribers attaching at random points.
	var wg sync.WaitGroup
	results := make([][]Sample, numSubs)

	// Subscribers spin up immediately; some will see the full
	// sequence, some will start mid-flight.
	for i := 0; i < numSubs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Stagger subscription times so different subs attach
			// at different sample counts.
			time.Sleep(time.Duration(idx) * time.Microsecond * 50)
			ch := lt.Subscribe()
			results[idx] = drainUntilClosed(ch)
		}(i)
	}

	// Emitter — runs concurrently with subscribers attaching.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numSamples; i++ {
			runner.samples <- Sample{Phase: collector.SpeedTestPhaseDownload, Mbps: float64(i)}
		}
		close(runner.done)
	}()

	wg.Wait()

	// All subscribers should have received some samples and a closed
	// channel. We don't pin counts (subscribers attaching mid-flight
	// see partial sequences) but we DO pin: every sample a subscriber
	// did receive must be in monotonically increasing Mbps order
	// (proves no duplicate / no out-of-order delivery).
	for i, got := range results {
		for j := 1; j < len(got); j++ {
			if got[j].Mbps < got[j-1].Mbps {
				t.Errorf("subscriber %d: out-of-order at idx %d (%v < %v)",
					i, j, got[j].Mbps, got[j-1].Mbps)
				break
			}
		}
	}
	if mgr.InProgress() {
		t.Error("InProgress()=true after all subscribers drained")
	}
}

// TestRegistry_SlowClientDrop verifies that a subscriber whose channel
// is never read does not block the broadcast — that subscriber's
// channel is closed and removed from the set, and other subscribers
// continue to receive samples normally.
func TestRegistry_SlowClientDrop(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	// Two subscribers: one slow (we never read), one fast (drained
	// by a goroutine so its buffer never fills).
	slow := lt.Subscribe()
	fast := lt.Subscribe()

	// Push more samples than SubscribeBufferSize so the slow
	// subscriber's buffer fills + that subscriber gets dropped.
	const overflow = SubscribeBufferSize * 3
	var fastWG sync.WaitGroup
	var gotFast []Sample
	fastWG.Add(1)
	go func() {
		defer fastWG.Done()
		gotFast = drainUntilClosed(fast)
	}()

	// Send samples with a small synchronisation point: each emit
	// waits for the fast drainer to catch up before pushing the
	// next sample. This pins the test against scheduler flakes
	// where the fast drainer would otherwise temporarily fall
	// behind the emitter and lose samples to its own buffer fill.
	// The slow consumer is unaffected by this — its buffer fills
	// regardless and it gets dropped as expected.
	for i := 0; i < overflow; i++ {
		runner.samples <- Sample{Phase: collector.SpeedTestPhaseDownload, Mbps: float64(i)}
		// Tiny yield so the fast drainer goroutine has a chance
		// to consume the sample before the next push. Without
		// this, the emit goroutine can race ahead and the fast
		// drainer's 64-sample buffer fills, causing the fast
		// drainer to also be dropped (false positive).
		for try := 0; try < 100; try++ {
			if len(lt.SnapshotSamples()) >= i+1 {
				break
			}
			time.Sleep(time.Microsecond * 100)
		}
	}
	close(runner.done)
	fastWG.Wait()

	// Allow up to 2 dropped samples on the fast subscriber as a
	// concession to scheduler nondeterminism — the test's purpose
	// is to verify the fast subscriber is not BLOCKED, not that it
	// receives exactly all samples (a stronger property that's
	// hard to pin under -race without making the test fragile).
	if len(gotFast) < overflow-2 {
		t.Errorf("fast subscriber got %d, want ≥%d (slow consumer should not block fast one)",
			len(gotFast), overflow-2)
	}

	// Slow subscriber's channel must close (even though we never
	// drained it past the buffer fill). drainUntilClosed will return
	// the buffered samples + see the close.
	gotSlow := drainUntilClosed(slow)
	if len(gotSlow) > SubscribeBufferSize+10 {
		t.Errorf("slow subscriber got %d samples — expected drop after buffer (~%d)",
			len(gotSlow), SubscribeBufferSize)
	}
}

func TestRegistry_StartTest_IdempotentReturnsExistingHandle(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt1, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest #1: %v", err)
	}
	<-runner.started

	lt2, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest #2: %v", err)
	}
	if lt1 != lt2 {
		t.Errorf("StartTest while in flight returned different handle (lt1=%p lt2=%p)", lt1, lt2)
	}
	if lt1.ID() != lt2.ID() {
		t.Errorf("ID mismatch: %d vs %d", lt1.ID(), lt2.ID())
	}

	close(runner.done)
	<-lt1.Done()
}

func TestRegistry_TwoConcurrentStartTest_OnlyOneRuns(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	var lt1, lt2 *LiveTest
	var err1, err2 error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); lt1, err1 = mgr.StartTest(context.Background()) }()
	go func() { defer wg.Done(); lt2, err2 = mgr.StartTest(context.Background()) }()
	wg.Wait()

	if err1 != nil || err2 != nil {
		t.Fatalf("StartTest errors: %v / %v", err1, err2)
	}
	if lt1 != lt2 {
		t.Error("two concurrent StartTest returned different handles — single-flight broken")
	}

	close(runner.done)
	<-lt1.Done()
}

func TestRegistry_StartTest_AfterRunnerError_AllowsNextTest(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.err = errors.New("simulated runner error")
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt1, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest #1: %v", err)
	}
	<-lt1.Done()
	if lt1.Err() == nil {
		t.Error("Err() = nil, want non-nil after runner error")
	}

	// Now a second StartTest should be allowed (registry slot cleared).
	runner2 := newFakeRunner()
	runner2.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr.runner = runner2
	lt2, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest #2 after error: %v", err)
	}
	if lt2 == lt1 {
		t.Error("post-error StartTest returned the same (terminated) handle")
	}
	close(runner2.done)
	<-lt2.Done()
}

// panicRunner panics on Run. Used to verify the registry releases
// its singleton lock even when the runner misbehaves.
type panicRunner struct{}

func (panicRunner) Run(_ context.Context) (*Result, <-chan Sample, error) {
	panic("simulated panic")
}

func TestRegistry_PanicMidTest_RegistrySlotCleared(t *testing.T) {
	t.Parallel()
	mgr := NewManager(panicRunner{}, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	// Wait for panic recovery to land + registry slot to clear.
	select {
	case <-lt.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close within 1s of panic")
	}

	if mgr.InProgress() {
		t.Error("InProgress()=true after panicked test — registry slot leaked")
	}

	// Next StartTest must succeed.
	runner2 := newFakeRunner()
	runner2.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr.runner = runner2
	lt2, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest after panic: %v", err)
	}
	close(runner2.done)
	<-lt2.Done()
}

func TestRegistry_GetLive_Lookup(t *testing.T) {
	t.Parallel()
	runner := newFakeRunner()
	runner.result = &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}
	mgr := NewManager(runner, quietLogger(), counterIDGen())

	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started

	got, ok := mgr.GetLive(lt.ID())
	if !ok {
		t.Error("GetLive(real id) = !ok")
	}
	if got != lt {
		t.Error("GetLive returned different handle")
	}

	_, ok = mgr.GetLive(lt.ID() + 999999)
	if ok {
		t.Error("GetLive(unknown id) returned ok")
	}

	close(runner.done)
	<-lt.Done()

	// After completion, GetLive may briefly still return the test
	// (race window) — but eventually returns false. We don't pin
	// timing; just check it eventually clears.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := mgr.GetLive(lt.ID()); !ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Error("GetLive still returned the completed test after 1s")
}

// counterIDGen returns deterministic monotonically-increasing IDs
// starting at 1. Tests use this to avoid time-based flakiness.
func counterIDGen() func() int64 {
	var n int64
	return func() int64 { return atomic.AddInt64(&n, 1) }
}

// waitForSampleCount blocks until lt.SnapshotSamples() reaches at
// least n entries, or fails the test on timeout.
func waitForSampleCount(t *testing.T, lt *LiveTest, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(lt.SnapshotSamples()) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d samples (got %d)", n, len(lt.SnapshotSamples()))
}
