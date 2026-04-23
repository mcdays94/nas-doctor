package scheduler

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// tracerouteRunner satisfies the injected seam shape used by
// runTraceCheck. Returning a non-nil MTRResult + nil error is the
// "mtr worked" path; returning nil + err is the "mtr failed / not
// installed" path.
type tracerouteFixture struct {
	result *collector.MTRResult
	err    error
}

func (f tracerouteFixture) Run(_ string, _ int) (*collector.MTRResult, error) {
	return f.result, f.err
}

// hopReached returns a hop shaped like a successful final hop.
func hopReached(host string, loss, avg float64) collector.MTRHop {
	return collector.MTRHop{Count: 1, Host: host, LossPct: loss, Sent: 5, AvgMs: avg}
}

// hopBlackHole returns a hop shaped like mtr's '???' sentinel.
func hopBlackHole() collector.MTRHop {
	return collector.MTRHop{Count: 2, Host: "???", LossPct: 100, Sent: 5}
}

func TestRunTraceCheck_Reached_ReportsUp(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target:          "8.8.8.8",
			Hops:            []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopReached("8.8.8.8", 0, 12.3)},
			EndToEndLossPct: 0,
			FinalRTTMs:      12.3,
		},
	}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-google",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "8.8.8.8",
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up, got %q (error=%q)", result.Status, result.Error)
	}
	// Details are nil when SetCollectDetails wasn't called — separate
	// coverage in TestRunTraceCheck_Reached_DetailsSummary.
}

func TestRunTraceCheck_Reached_DetailsSummary(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target:          "8.8.8.8",
			Hops:            []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopReached("8.8.8.8", 0, 12.3)},
			EndToEndLossPct: 0,
			FinalRTTMs:      12.3,
		},
	}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-google",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "8.8.8.8",
		Enabled: true,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Details == nil {
		t.Fatal("expected Details populated")
	}
	if v, ok := result.Details["hops_count"].(int); !ok || v != 2 {
		t.Errorf("expected hops_count=2, got %v (type=%T)", result.Details["hops_count"], result.Details["hops_count"])
	}
	if v, ok := result.Details["final_rtt_ms"].(float64); !ok || v != 12.3 {
		t.Errorf("expected final_rtt_ms=12.3, got %v", result.Details["final_rtt_ms"])
	}
	if v, ok := result.Details["end_to_end_loss_pct"].(float64); !ok || v != 0 {
		t.Errorf("expected end_to_end_loss_pct=0, got %v", result.Details["end_to_end_loss_pct"])
	}
	if v, ok := result.Details["target"].(string); !ok || v != "8.8.8.8" {
		t.Errorf("expected target=8.8.8.8, got %v", result.Details["target"])
	}
}

func TestRunTraceCheck_HopsExposedInDetails(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target:     "8.8.8.8",
			Hops:       []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopReached("8.8.8.8", 0, 12.3)},
			FinalRTTMs: 12.3,
		},
	}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-hops-detail",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "8.8.8.8",
		Enabled: true,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	raw, ok := result.Details["hops"]
	if !ok {
		t.Fatal("expected hops array exposed in Details (Test-button render needs it)")
	}
	hops, ok := raw.([]collector.MTRHop)
	if !ok {
		t.Fatalf("expected Details.hops to be []collector.MTRHop, got %T", raw)
	}
	if len(hops) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(hops))
	}
}

func TestRunTraceCheck_Unreachable_ReportsDown(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target: "192.0.2.99",
			Hops:   []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopBlackHole()},
		},
	}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-unreach",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "192.0.2.99",
		Enabled: true,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down when final hop is black hole, got %q (error=%q)", result.Status, result.Error)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error describing unreachability")
	}
}

func TestRunTraceCheck_LossOverThreshold_ReportsDegraded(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target:          "8.8.8.8",
			Hops:            []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopReached("8.8.8.8", 10, 15)},
			EndToEndLossPct: 10,
			FinalRTTMs:      15,
		},
	}.Run)

	threshold := 5.0
	check := internal.ServiceCheckConfig{
		Name:       "trace-degraded",
		Type:       internal.ServiceCheckTraceroute,
		Target:     "8.8.8.8",
		Enabled:    true,
		MaxLossPct: &threshold,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "degraded" {
		t.Fatalf("expected status degraded, got %q (error=%q)", result.Status, result.Error)
	}
	if !strings.Contains(strings.ToLower(result.Error), "loss") {
		t.Fatalf("expected error to mention loss, got %q", result.Error)
	}
}

func TestRunTraceCheck_LossAtThreshold_ReportsUp(t *testing.T) {
	// Threshold is "degraded when loss > threshold". Loss == threshold
	// is still up.
	sc, _ := newTestChecker()
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target:          "8.8.8.8",
			Hops:            []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopReached("8.8.8.8", 5, 15)},
			EndToEndLossPct: 5,
		},
	}.Run)

	threshold := 5.0
	check := internal.ServiceCheckConfig{
		Name:       "trace-at-threshold",
		Type:       internal.ServiceCheckTraceroute,
		Target:     "8.8.8.8",
		Enabled:    true,
		MaxLossPct: &threshold,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up when loss exactly equals threshold, got %q", result.Status)
	}
}

func TestRunTraceCheck_NoThreshold_IgnoresLoss(t *testing.T) {
	// MaxLossPct nil → reachability-only mode. 50% loss with final
	// hop responding should still report up.
	sc, _ := newTestChecker()
	sc.SetTraceRunner(tracerouteFixture{
		result: &collector.MTRResult{
			Target:          "8.8.8.8",
			Hops:            []collector.MTRHop{hopReached("10.0.0.1", 0, 0.5), hopReached("8.8.8.8", 50, 100)},
			EndToEndLossPct: 50,
		},
	}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-no-threshold",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "8.8.8.8",
		Enabled: true,
		// MaxLossPct: nil deliberately.
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up with nil threshold regardless of loss, got %q", result.Status)
	}
}

func TestRunTraceCheck_RunnerError_ReportsDown(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetTraceRunner(tracerouteFixture{
		err: errors.New("mtr: command not found"),
	}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-no-binary",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "8.8.8.8",
		Enabled: true,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down on runner error, got %q", result.Status)
	}
	if !strings.Contains(result.Error, "mtr") {
		t.Fatalf("expected error to mention mtr, got %q", result.Error)
	}
}

func TestRunTraceCheck_EmptyTarget_ReportsDown(t *testing.T) {
	sc, _ := newTestChecker()
	// No runner needed — validation should short-circuit. If the
	// runner is called anyway the test will fail because Run returns
	// nil,nil (invalid shape) below.
	sc.SetTraceRunner(tracerouteFixture{}.Run)

	check := internal.ServiceCheckConfig{
		Name:    "trace-empty",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "   ",
		Enabled: true,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down on empty target, got %q", result.Status)
	}
}

func TestRunTraceCheck_NoRunnerInjected_ReportsDown(t *testing.T) {
	// The scheduler's persistent ServiceChecker is constructed WITHOUT
	// a trace runner; only the Test endpoint and RunDueChecks dispatch
	// inject one. A scheduled traceroute check without a runner should
	// report down cleanly (no panic) with a descriptive error.
	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:    "trace-no-runner",
		Type:    internal.ServiceCheckTraceroute,
		Target:  "8.8.8.8",
		Enabled: true,
	}
	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down when no runner injected, got %q (error=%q)", result.Status, result.Error)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error when no runner injected")
	}
}
