package demo

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// simStep is the interval between simulated samples. Tests shorten it.
var simStep = 100 * time.Millisecond

// StreamingSpeedTest simulates the stream behind the speed check Test button
// (#346): a short latency phase, then download and upload ramping up like a
// TCP transfer with some noise, paced in real time. The result matches the
// demo snapshot's speed test, and the channels close the way
// collector.RunStreamingSpeedTest closes them, including on cancel.
func StreamingSpeedTest(ctx context.Context) (<-chan collector.SpeedTestSample, <-chan collector.StreamingSpeedFinal) {
	updates := make(chan collector.SpeedTestSample, 64)
	final := make(chan collector.StreamingSpeedFinal, 1)
	go func() {
		defer close(updates)
		defer close(final)
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		ticker := time.NewTicker(simStep)
		defer ticker.Stop()

		emit := func(s collector.SpeedTestSample) bool {
			if ctx.Err() != nil {
				return false
			}
			select {
			case <-ctx.Done():
				return false
			case <-ticker.C:
			}
			s.At = time.Now()
			select {
			case updates <- s:
				return true
			case <-ctx.Done():
				return false
			}
		}
		stopped := func() { final <- collector.StreamingSpeedFinal{RunErr: ctx.Err()} }

		const latencySamples = 10
		var latSum, latSqSum float64
		for i := 0; i < latencySamples; i++ {
			lat := math.Max(4, 8+rng.NormFloat64()*1.2)
			latSum += lat
			latSqSum += lat * lat
			if !emit(collector.SpeedTestSample{Phase: collector.SpeedTestPhaseLatency, LatencyMs: lat}) {
				stopped()
				return
			}
		}
		latency := latSum / latencySamples
		jitter := math.Sqrt(math.Max(latSqSum/latencySamples-latency*latency, 0))

		// transfer ramps toward target and returns the mean once the ramp
		// has settled, which is roughly how the engines report throughput.
		transfer := func(phase collector.SpeedTestPhase, target float64) (float64, bool) {
			const samples = 80
			var sum float64
			var counted int
			for i := 0; i < samples; i++ {
				elapsed := float64(i+1) / 10
				v := target * (1 - math.Exp(-elapsed/0.7)) * (1 + rng.NormFloat64()*0.035)
				if rng.Float64() < 0.04 {
					v *= 0.85
				}
				v = math.Max(0, v)
				if i >= samples/4 {
					sum += v
					counted++
				}
				if !emit(collector.SpeedTestSample{Phase: phase, Mbps: v}) {
					return 0, false
				}
			}
			return sum / float64(counted), true
		}
		down, ok := transfer(collector.SpeedTestPhaseDownload, 940)
		if !ok {
			stopped()
			return
		}
		up, ok := transfer(collector.SpeedTestPhaseUpload, 450)
		if !ok {
			stopped()
			return
		}

		final <- collector.StreamingSpeedFinal{Result: &internal.SpeedTestResult{
			Timestamp:    time.Now(),
			DownloadMbps: down,
			UploadMbps:   up,
			LatencyMs:    latency,
			JitterMs:     jitter,
			ServerName:   "Lisbon",
			ISP:          "MEO",
			Engine:       internal.SpeedTestEngineSpeedTestGo,
		}}
	}()
	return updates, final
}
