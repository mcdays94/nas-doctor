package notifier

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// TestPrometheus_SpeedTestEngine_EndToEndFromRunner asserts the full
// pipeline: a SpeedTestRunner produces a result with Engine stamped
// → that result lands in a Snapshot.SpeedTest.Latest → Update()
// flips the gauge → /metrics scrape contains the right label = 1.
// Closes the issue #284 acceptance criterion: "the gauge for that
// engine reads 1 in the /metrics exporter output".
func TestPrometheus_SpeedTestEngine_EndToEndFromRunner(t *testing.T) {
	// Compose primary-success scenario via the production composite
	// shape but with fake engines so we don't go to the network.
	fakePrimaryEngine := &mockEngine{
		result: &internal.SpeedTestResult{
			Timestamp:    time.Now(),
			DownloadMbps: 500, UploadMbps: 50, LatencyMs: 4,
		},
	}
	primary := newPrimarySpeedtestGoRunner(fakePrimaryEngine)
	composite := collector.NewCompositeSpeedTestRunner(primary, nil)

	res, samples, err := composite.Run(t.Context())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Drain the channel as production does.
	for range samples {
	}
	if res.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Fatalf("Engine = %q, want speedtest_go", res.Engine)
	}

	// Now feed the result into the Prometheus exporter and scrape.
	m := NewMetrics()
	m.Update(&internal.Snapshot{
		SpeedTest: &internal.SpeedTestInfo{
			Available: true,
			Latest:    res,
		},
	})
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="speedtest_go"} 1`) {
		t.Errorf("expected speedtest_go = 1 in /metrics; body:\n%s", body)
	}
	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="ookla_cli"} 0`) {
		t.Errorf("expected ookla_cli = 0 (other-engine label) in /metrics; body:\n%s", body)
	}
}

// mockEngine is the in-package equivalent of the collector test's
// fakeSpeedTestEngine. We can't reach into the collector package's
// unexported types from here, so the runner-with-engine layer is
// driven through the exported NewCompositeSpeedTestRunner +
// newPrimarySpeedtestGoRunner test helper.
type mockEngine struct {
	result *internal.SpeedTestResult
}

// newPrimarySpeedtestGoRunner: a thin in-test runner that emulates
// the speedtestGoRunner contract — stamps Engine="speedtest_go" on
// the result, returns a closed sample channel. Used only for this
// e2e test where we need a runner whose result.Engine is
// "speedtest_go" without invoking the real showwin library.
func newPrimarySpeedtestGoRunner(eng *mockEngine) collector.SpeedTestRunner {
	return &mockRunner{eng: eng, engine: internal.SpeedTestEngineSpeedTestGo}
}

type mockRunner struct {
	eng    *mockEngine
	engine string
}

func (r *mockRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	if r.eng == nil || r.eng.result == nil {
		return nil, nil, errMockRunnerNoResult
	}
	res := *r.eng.result // copy so we don't mutate the input
	res.Engine = r.engine
	ch := make(chan collector.SpeedTestSample)
	close(ch)
	return &res, ch, nil
}

var errMockRunnerNoResult = stringErr("mockRunner: no result configured")

type stringErr string

func (s stringErr) Error() string { return string(s) }
