package demo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// Demo mode simulates the speed check Test so the button shows live progress
// without measuring the demo host's own connection (#346).

func setSimStep(d time.Duration) func() {
	old := simStep
	simStep = d
	return func() { simStep = old }
}

func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("simulated speed test did not finish within %s", d)
	}
}

func TestStreamingSpeedTest_RunsEveryPhaseThenResult(t *testing.T) {
	defer setSimStep(time.Millisecond)()
	within(t, 5*time.Second, func() {
		updates, final := StreamingSpeedTest(context.Background())
		var order []collector.SpeedTestPhase
		counts := map[collector.SpeedTestPhase]int{}
		peak := map[collector.SpeedTestPhase]float64{}
		for s := range updates {
			if len(order) == 0 || order[len(order)-1] != s.Phase {
				order = append(order, s.Phase)
			}
			counts[s.Phase]++
			if s.Mbps > peak[s.Phase] {
				peak[s.Phase] = s.Mbps
			}
		}
		want := []collector.SpeedTestPhase{collector.SpeedTestPhaseLatency, collector.SpeedTestPhaseDownload, collector.SpeedTestPhaseUpload}
		if len(order) != len(want) {
			t.Fatalf("phase order = %v; want %v", order, want)
		}
		for i := range want {
			if order[i] != want[i] {
				t.Fatalf("phase order = %v; want %v", order, want)
			}
		}
		if counts[collector.SpeedTestPhaseDownload] < 20 || counts[collector.SpeedTestPhaseUpload] < 20 {
			t.Errorf("too few samples to draw a chart: %v", counts)
		}
		if peak[collector.SpeedTestPhaseDownload] <= peak[collector.SpeedTestPhaseUpload] {
			t.Errorf("download peak %.0f should exceed upload peak %.0f", peak[collector.SpeedTestPhaseDownload], peak[collector.SpeedTestPhaseUpload])
		}

		fin := <-final
		if fin.RunErr != nil || fin.Result == nil {
			t.Fatalf("final = %+v; want a result and no error", fin)
		}
		r := fin.Result
		if r.DownloadMbps <= 0 || r.UploadMbps <= 0 || r.LatencyMs <= 0 || r.ServerName == "" || r.ISP == "" || r.Engine == "" {
			t.Errorf("result is missing fields the card shows: %+v", r)
		}
	})
}

func TestStreamingSpeedTest_StopsWhenCancelled(t *testing.T) {
	defer setSimStep(5 * time.Millisecond)()
	within(t, 5*time.Second, func() {
		ctx, cancel := context.WithCancel(context.Background())
		updates, final := StreamingSpeedTest(ctx)
		<-updates
		cancel()
		for range updates {
		}
		fin := <-final
		if !errors.Is(fin.RunErr, context.Canceled) || fin.Result != nil {
			t.Errorf("final after cancel = %+v; want RunErr=context.Canceled and no result", fin)
		}
	})
}
