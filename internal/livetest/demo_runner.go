// demo_runner.go — synthetic Runner used by demo mode to simulate
// a live speed test without touching the network. Emits a
// deterministic-but-jittery sequence of latency / download / upload
// samples over ~12 seconds so the dashboard's live-progress strip
// has something to render in screencasts and UAT runs against
// `nas-doctor -demo`. PRD #283 / issue #285.
package livetest

import (
	"context"
	"math/rand"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// NewDemoSpeedTestRunner returns a Runner that simulates a 12-second
// speed test with realistic phase ordering and sample counts. Useful
// only for demo mode; production wiring uses
// collector.DefaultSpeedTestRunner.
func NewDemoSpeedTestRunner() Runner {
	return &demoRunner{}
}

type demoRunner struct{}

func (demoRunner) Run(ctx context.Context) (*Result, <-chan Sample, error) {
	out := make(chan Sample, 64)
	go func() {
		defer close(out)
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		// Latency phase — 4 samples over ~1.2s.
		for i := 0; i < 4; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(300 * time.Millisecond):
			}
			out <- Sample{
				Phase:     collector.SpeedTestPhaseLatency,
				At:        time.Now(),
				LatencyMs: 5 + rng.Float64()*4,
			}
		}
		// Download phase — 8 samples over ~6s; Mbps climbs then
		// stabilises around 920.
		for i := 0; i < 8; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(750 * time.Millisecond):
			}
			t := float64(i) / 8
			mbps := 200 + t*700 + rng.Float64()*60
			out <- Sample{
				Phase: collector.SpeedTestPhaseDownload,
				At:    time.Now(),
				Mbps:  mbps,
			}
		}
		// Upload phase — 6 samples over ~4.5s; Mbps climbs to ~88.
		for i := 0; i < 6; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(750 * time.Millisecond):
			}
			t := float64(i) / 6
			mbps := 30 + t*60 + rng.Float64()*8
			out <- Sample{
				Phase: collector.SpeedTestPhaseUpload,
				At:    time.Now(),
				Mbps:  mbps,
			}
		}
	}()
	return &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: 920.5,
		UploadMbps:   88.3,
		LatencyMs:    7.8,
		JitterMs:     1.6,
		ServerName:   "Demo Server",
		ISP:          "Demo ISP",
		Engine:       internal.SpeedTestEngineSpeedTestGo,
	}, out, nil
}
