package notifier

import (
	"strings"
	"testing"
)

// TestPrometheus_SpeedTestInProgress_DefaultsZero asserts that on a
// fresh Metrics with no test in flight the gauge is 0. PRD #283 /
// issue #285 user story 17.
func TestPrometheus_SpeedTestInProgress_DefaultsZero(t *testing.T) {
	m := NewMetrics()
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, "nasdoctor_speedtest_in_progress 0") {
		t.Errorf("expected nasdoctor_speedtest_in_progress 0 in /metrics; body:\n%s", body)
	}
}

// TestPrometheus_SpeedTestInProgress_FlipsTo1 asserts that calling
// SetSpeedTestInProgress(true) flips the gauge to 1 and (false)
// flips it back to 0.
func TestPrometheus_SpeedTestInProgress_FlipsTo1(t *testing.T) {
	m := NewMetrics()
	m.SetSpeedTestInProgress(true)
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, "nasdoctor_speedtest_in_progress 1") {
		t.Errorf("expected gauge=1 after start; body:\n%s", body)
	}
	m.SetSpeedTestInProgress(false)
	body = scrapeMetrics(t, m)
	if !strings.Contains(body, "nasdoctor_speedtest_in_progress 0") {
		t.Errorf("expected gauge=0 after end; body:\n%s", body)
	}
}
