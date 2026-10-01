package collector

import (
	"context"

	"github.com/mcdays94/nas-doctor/internal"
)

// SampleSink receives speed-test samples while the engine measures them
// (issue #348). SpeedTestRunner.Run only returns once the whole test has
// run, so a consumer that waits for the returned channel only ever sees a
// replay at the end. The engine calls the sink from its own goroutines, so
// it must be quick and must not block.
type SampleSink func(SpeedTestSample)

type sampleSinkKey struct{}

// withSampleSink asks a supporting engine to hand each sample to sink as it
// is measured instead of buffering it until Run returns.
func withSampleSink(ctx context.Context, sink SampleSink) context.Context {
	return context.WithValue(ctx, sampleSinkKey{}, sink)
}

// SampleSinkFrom returns the sink set on ctx, or nil. Engines call it at the
// start of Run.
func SampleSinkFrom(ctx context.Context) SampleSink {
	sink, _ := ctx.Value(sampleSinkKey{}).(SampleSink)
	return sink
}

// emitSpeedSample delivers one sample: straight to sink when there is one,
// otherwise into ch without blocking. It returns false when ch was full and
// the sample was dropped.
func emitSpeedSample(sink SampleSink, ch chan<- SpeedTestSample, s SpeedTestSample) bool {
	if sink != nil {
		sink(s)
		return true
	}
	select {
	case ch <- s:
		return true
	default:
		return false
	}
}

// liveSampleBuffer is how many samples RunWithLiveSamples holds between the
// engine's goroutines and the caller. Samples are dropped only if the caller
// falls this far behind.
const liveSampleBuffer = 256

// RunWithLiveSamples runs runner and calls onSample for each sample, in
// order, from the calling goroutine. Engines that honour the context sink
// deliver while the test runs; others deliver through the returned channel
// after Run, as before. It returns Run's result and error.
func RunWithLiveSamples(ctx context.Context, runner SpeedTestRunner, onSample func(SpeedTestSample)) (*internal.SpeedTestResult, error) {
	live := make(chan SpeedTestSample, liveSampleBuffer)
	ctx = withSampleSink(ctx, func(s SpeedTestSample) {
		select {
		case live <- s:
		default:
		}
	})

	type outcome struct {
		res      *internal.SpeedTestResult
		samples  <-chan SpeedTestSample
		err      error
		panicked bool
		panicVal any
	}
	done := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			if p := recover(); p != nil {
				o = outcome{panicked: true, panicVal: p}
			}
			done <- o
		}()
		o.res, o.samples, o.err = runner.Run(ctx)
	}()

	for {
		select {
		case s := <-live:
			onSample(s)
		case o := <-done:
			if o.panicked {
				// Re-raise on the caller's goroutine so its own
				// recover (the registry's drive loop) still sees an
				// engine panic, as when it called Run directly.
				panic(o.panicVal)
			}
			// Deliver what the sink queued before Run returned, then
			// anything the runner reported through its channel.
		drain:
			for {
				select {
				case s := <-live:
					onSample(s)
				default:
					break drain
				}
			}
			if o.err == nil && o.samples != nil {
				for s := range o.samples {
					onSample(s)
				}
			}
			return o.res, o.err
		}
	}
}
