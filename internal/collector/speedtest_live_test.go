package collector

import (
	"context"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #348: Run blocks for the whole test, so a consumer that only reads
// the samples channel after Run returns sees a replay at the end, never a
// live sample. Engines deliver samples through a sink on the context while
// they measure; these tests use a fake that does the same and stays running
// until it is released.

type sinkEngine struct {
	samples []SpeedTestSample
	release chan struct{}
}

func newSinkEngine(n int) *sinkEngine {
	e := &sinkEngine{release: make(chan struct{})}
	for i := 0; i < n; i++ {
		e.samples = append(e.samples, SpeedTestSample{Phase: SpeedTestPhaseDownload, At: time.Now(), Mbps: float64(100 * (i + 1))})
	}
	return e
}

func (e *sinkEngine) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	if sink := SampleSinkFrom(ctx); sink != nil {
		for _, s := range e.samples {
			sink(s)
		}
	}
	select {
	case <-e.release:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	ch := make(chan SpeedTestSample)
	close(ch)
	return &internal.SpeedTestResult{DownloadMbps: 900}, ch, nil
}

// channelOnlyEngine reports its samples through the returned channel after
// Run, like the Ookla engine and the older test fakes.
type channelOnlyEngine struct{ samples []SpeedTestSample }

func (e channelOnlyEngine) Run(context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	ch := make(chan SpeedTestSample, len(e.samples))
	for _, s := range e.samples {
		ch <- s
	}
	close(ch)
	return &internal.SpeedTestResult{DownloadMbps: 500}, ch, nil
}

func waitFor[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	var zero T
	return zero
}

func TestRunWithLiveSamples_DeliversWhileTheTestRuns(t *testing.T) {
	engine := newSinkEngine(3)
	got := make(chan SpeedTestSample, 3)
	type outcome struct {
		res *internal.SpeedTestResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := RunWithLiveSamples(context.Background(), engine, func(s SpeedTestSample) { got <- s })
		done <- outcome{res, err}
	}()

	for i := 0; i < 3; i++ {
		s := waitFor(t, got, "a live sample while Run is still running")
		if s.Mbps != float64(100*(i+1)) {
			t.Errorf("sample %d Mbps = %v; want %v (samples must stay in order)", i, s.Mbps, 100*(i+1))
		}
	}
	close(engine.release)
	o := waitFor(t, done, "RunWithLiveSamples to return")
	if o.err != nil || o.res == nil || o.res.DownloadMbps != 900 {
		t.Errorf("RunWithLiveSamples = (%+v, %v); want the engine's result", o.res, o.err)
	}
}

func TestRunWithLiveSamples_ChannelOnlyEngineStillDelivers(t *testing.T) {
	engine := channelOnlyEngine{samples: []SpeedTestSample{{Mbps: 1}, {Mbps: 2}}}
	var got []float64
	res, err := RunWithLiveSamples(context.Background(), engine, func(s SpeedTestSample) { got = append(got, s.Mbps) })
	if err != nil || res == nil || res.DownloadMbps != 500 {
		t.Fatalf("RunWithLiveSamples = (%+v, %v); want the engine's result", res, err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("delivered %v; want [1 2]", got)
	}
}

type panickingEngine struct{}

func (panickingEngine) Run(context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	panic("engine blew up")
}

// Run executes on a helper goroutine, so an engine panic has to come back to
// the caller's goroutine, where the registry's recover handles it. Left on
// the helper goroutine it would crash the process.
func TestRunWithLiveSamples_RepanicsOnTheCallersGoroutine(t *testing.T) {
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = RunWithLiveSamples(context.Background(), panickingEngine{}, func(SpeedTestSample) {})
	}()
	if recovered != "engine blew up" {
		t.Errorf("recovered %v; want the engine's panic on the caller's goroutine", recovered)
	}
}

// The Test-button stream (#346) must forward samples as they are measured,
// not after the engine returns.
func TestRunStreamingSpeedTest_ForwardsSamplesWhileRunning(t *testing.T) {
	engine := newSinkEngine(3)
	updates, final := runStreamingSpeedTestWithRunner(context.Background(), engine)
	for i := 0; i < 3; i++ {
		waitFor(t, updates, "a streamed sample while the engine is still running")
	}
	close(engine.release)
	fin := waitFor(t, final, "the final result")
	if fin.RunErr != nil || fin.Result == nil {
		t.Errorf("final = %+v; want the engine's result", fin)
	}
}

// emitSpeedSample is the engine's per-sample delivery: straight to the sink
// when one is set, otherwise into the buffered channel, dropping when full.
func TestEmitSpeedSample(t *testing.T) {
	var sunk []SpeedTestSample
	ch := make(chan SpeedTestSample, 1)
	if !emitSpeedSample(func(s SpeedTestSample) { sunk = append(sunk, s) }, ch, SpeedTestSample{Mbps: 1}) || len(sunk) != 1 || len(ch) != 0 {
		t.Errorf("with a sink: sunk=%d buffered=%d; want the sink to get it and nothing buffered", len(sunk), len(ch))
	}
	if !emitSpeedSample(nil, ch, SpeedTestSample{Mbps: 2}) || len(ch) != 1 {
		t.Errorf("without a sink: buffered=%d; want 1", len(ch))
	}
	if emitSpeedSample(nil, ch, SpeedTestSample{Mbps: 3}) {
		t.Error("full buffer without a sink: want the sample dropped (false)")
	}
}
