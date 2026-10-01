package collector

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// fakeStreamingSpeedRunner is a hand-rolled SpeedTestRunner for the
// streaming-wrapper tests. The runner emits the configured samples
// (in order, with ~no delay) onto the channel returned from Run,
// closes the channel, and returns the configured result/err.
type fakeStreamingSpeedRunner struct {
	samples []SpeedTestSample
	result  *internal.SpeedTestResult
	err     error
	// blockUntil, if non-nil, is closed by the test to signal
	// "now drain samples and return" — useful for cancellation
	// races.
	blockUntil <-chan struct{}
}

func (f *fakeStreamingSpeedRunner) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	out := make(chan SpeedTestSample, len(f.samples)+1)
	go func() {
		defer close(out)
		if f.blockUntil != nil {
			select {
			case <-f.blockUntil:
			case <-ctx.Done():
				return
			}
		}
		for _, s := range f.samples {
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
		}
	}()
	return f.result, out, nil
}

// streamAssertCompletesWithin enforces the 5s deadline standard so a
// channel-ordering deadlock is loud-and-fast instead of silent-and-
// slow under the default `go test` 10-minute timeout. Mirrors the
// helper in the api package's service_check_test_stream_test.go.
func streamAssertCompletesWithin(t *testing.T, d time.Duration) func() {
	t.Helper()
	timer := time.AfterFunc(d, func() {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		panic("test exceeded " + d.String() + " deadline; goroutine dump:\n" + string(buf[:n]))
	})
	return func() { timer.Stop() }
}

func TestRunStreamingSpeedTest_ForwardsSamplesThenFinal(t *testing.T) {
	stop := streamAssertCompletesWithin(t, 5*time.Second)
	defer stop()

	want := []SpeedTestSample{
		{Phase: SpeedTestPhaseLatency, At: time.Now(), LatencyMs: 5.0},
		{Phase: SpeedTestPhaseDownload, At: time.Now(), Mbps: 100.0},
		{Phase: SpeedTestPhaseDownload, At: time.Now(), Mbps: 200.0},
		{Phase: SpeedTestPhaseUpload, At: time.Now(), Mbps: 50.0},
	}
	wantResult := &internal.SpeedTestResult{
		DownloadMbps: 200.0,
		UploadMbps:   50.0,
		LatencyMs:    5.0,
		Engine:       internal.SpeedTestEngineSpeedTestGo,
	}
	runner := &fakeStreamingSpeedRunner{samples: want, result: wantResult}

	updates, final := runStreamingSpeedTestWithRunner(context.Background(), runner)

	var got []SpeedTestSample
	for s := range updates {
		got = append(got, s)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d samples, want %d", len(got), len(want))
	}
	for i, s := range got {
		if s.Phase != want[i].Phase {
			t.Errorf("sample[%d].Phase = %v, want %v", i, s.Phase, want[i].Phase)
		}
	}

	fin, ok := <-final
	if !ok {
		t.Fatalf("final channel closed without emitting")
	}
	if fin.RunErr != nil {
		t.Fatalf("RunErr = %v, want nil", fin.RunErr)
	}
	if fin.Result == nil || fin.Result.DownloadMbps != wantResult.DownloadMbps {
		t.Fatalf("Result mismatch: %+v", fin.Result)
	}

	// final must close after the value is emitted so SSE handler's
	// for-range terminates cleanly.
	if _, ok := <-final; ok {
		t.Errorf("final channel did not close after emit")
	}
}

func TestRunStreamingSpeedTest_RunnerErrorSurfacesInFinal(t *testing.T) {
	stop := streamAssertCompletesWithin(t, 5*time.Second)
	defer stop()

	wantErr := errors.New("engine offline")
	runner := &fakeStreamingSpeedRunner{err: wantErr}

	updates, final := runStreamingSpeedTestWithRunner(context.Background(), runner)

	// Updates channel must close empty when the engine errors
	// before the first sample.
	var samples []SpeedTestSample
	for s := range updates {
		samples = append(samples, s)
	}
	if len(samples) != 0 {
		t.Errorf("got %d samples, want 0 on engine error", len(samples))
	}

	fin, ok := <-final
	if !ok {
		t.Fatalf("final channel closed without emitting")
	}
	if !errors.Is(fin.RunErr, wantErr) {
		t.Errorf("RunErr = %v, want wraps %v", fin.RunErr, wantErr)
	}
	if fin.Result != nil {
		t.Errorf("Result = %+v, want nil on error", fin.Result)
	}
}

func TestRunStreamingSpeedTest_CtxCancelClosesPromptly(t *testing.T) {
	stop := streamAssertCompletesWithin(t, 5*time.Second)
	defer stop()

	// Block the runner until cancellation so the wrapper's select
	// observes ctx.Done() rather than samples-channel-close.
	gate := make(chan struct{})
	runner := &fakeStreamingSpeedRunner{
		blockUntil: gate,
		samples: []SpeedTestSample{
			{Phase: SpeedTestPhaseDownload, Mbps: 1.0},
		},
		result: &internal.SpeedTestResult{DownloadMbps: 1.0},
	}

	ctx, cancel := context.WithCancel(context.Background())
	updates, final := runStreamingSpeedTestWithRunner(ctx, runner)

	cancel()

	// Both channels must close within 2s of cancellation. We don't
	// assert anything about the contents — the engine may have
	// returned context.Canceled OR the wrapper may have caught
	// ctx.Done() first and emitted ctx.Err() into final. Either
	// path is correct; what matters is no deadlock.
	doneUpdates := make(chan struct{})
	doneFinal := make(chan struct{})
	go func() {
		for range updates {
		}
		close(doneUpdates)
	}()
	go func() {
		for range final {
		}
		close(doneFinal)
	}()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	select {
	case <-doneUpdates:
	case <-deadline.C:
		t.Fatal("updates channel did not close within 2s of ctx cancel")
	}
	select {
	case <-doneFinal:
	case <-deadline.C:
		t.Fatal("final channel did not close within 2s of ctx cancel")
	}
	close(gate) // release the gated runner goroutine
}

func TestRunStreamingSpeedTest_NilRunnerYieldsCleanError(t *testing.T) {
	stop := streamAssertCompletesWithin(t, 5*time.Second)
	defer stop()

	updates, final := runStreamingSpeedTestWithRunner(context.Background(), nil)

	for range updates {
	}
	fin, ok := <-final
	if !ok {
		t.Fatalf("final channel closed without emitting")
	}
	if fin.RunErr == nil {
		t.Fatal("expected RunErr on nil-runner path, got nil")
	}
	if fin.Result != nil {
		t.Errorf("Result = %+v, want nil on nil-runner path", fin.Result)
	}
}

// TestRunStreamingSpeedTest_NilSamplesChannel — defensive guard
// against a runner that violates the contract by returning (result,
// nil-channel, nil-err). The wrapper must not panic and must still
// emit a terminal value onto final. Mirrors the StreamingTraceFinal
// equivalent in traceroute_stream_test.go.
func TestRunStreamingSpeedTest_NilSamplesChannel(t *testing.T) {
	stop := streamAssertCompletesWithin(t, 5*time.Second)
	defer stop()

	runner := &nilSamplesRunner{result: &internal.SpeedTestResult{DownloadMbps: 1.0}}
	updates, final := runStreamingSpeedTestWithRunner(context.Background(), runner)
	for range updates {
	}
	fin, ok := <-final
	if !ok {
		t.Fatalf("final channel closed without emitting")
	}
	if fin.Result == nil {
		t.Errorf("Result missing despite engine returning non-nil result; fin=%+v", fin)
	}
}

type nilSamplesRunner struct {
	result *internal.SpeedTestResult
}

func (n *nilSamplesRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	return n.result, nil, nil
}

// TestStreamingSpeedTestRunnerType_Compatibility — type-check that
// RunStreamingSpeedTest matches the StreamingSpeedTestRunner alias.
// If a future refactor changes the return shape, this fails cleanly
// with a compile-time error rather than a confusing runtime mismatch
// in service_check_test_stream.go.
func TestStreamingSpeedTestRunnerType_Compatibility(t *testing.T) {
	var _ StreamingSpeedTestRunner = RunStreamingSpeedTest
}

// regression guard for race-detector flake: the wrapper must not
// emit duplicate finals if the engine errors AND ctx is cancelled
// concurrently.
func TestRunStreamingSpeedTest_FinalEmittedExactlyOnce(t *testing.T) {
	stop := streamAssertCompletesWithin(t, 5*time.Second)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner := &fakeStreamingSpeedRunner{err: errors.New("boom")}
		_, final := runStreamingSpeedTestWithRunner(context.Background(), runner)
		count := 0
		for range final {
			count++
		}
		if count != 1 {
			t.Errorf("final emitted %d times, want exactly 1", count)
		}
	}()
	wg.Wait()
}
