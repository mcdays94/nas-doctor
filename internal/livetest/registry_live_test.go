package livetest

import (
	"context"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// Issue #348: the dashboard's live strip only received samples after the
// engine returned, because driveTest read the samples channel after Run.
// liveRunner behaves like the real engine: it reports samples through the
// context sink while measuring and returns only when released.
type liveRunner struct {
	release chan struct{}
}

func (r *liveRunner) Run(ctx context.Context) (*Result, <-chan Sample, error) {
	if sink := collector.SampleSinkFrom(ctx); sink != nil {
		for i := 1; i <= 3; i++ {
			sink(Sample{Phase: collector.SpeedTestPhaseDownload, At: time.Now(), Mbps: float64(100 * i)})
		}
	}
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	ch := make(chan Sample)
	close(ch)
	return &internal.SpeedTestResult{DownloadMbps: 300, Engine: internal.SpeedTestEngineSpeedTestGo}, ch, nil
}

func TestRegistry_SubscribersGetSamplesWhileTheTestRuns(t *testing.T) {
	runner := &liveRunner{release: make(chan struct{})}
	mgr := NewManager(runner, quietLogger(), counterIDGen())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lt, err := mgr.StartTest(ctx)
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	sub := lt.Subscribe()
	for i := 1; i <= 3; i++ {
		select {
		case s, ok := <-sub:
			if !ok {
				t.Fatalf("subscription closed after %d samples", i-1)
			}
			if s.Mbps != float64(100*i) {
				t.Errorf("sample %d Mbps = %v; want %v", i, s.Mbps, 100*i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("sample %d did not reach the subscriber while the engine was still running", i)
		}
	}

	close(runner.release)
	select {
	case <-lt.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("test did not finish after the engine returned")
	}
	if lt.Result() == nil {
		t.Error("Result() = nil; want the engine's result")
	}
}
