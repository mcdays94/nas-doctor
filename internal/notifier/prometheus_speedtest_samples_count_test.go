package notifier

import (
	"strings"
	"testing"
)

// TestPrometheus_SpeedTestSamplesCount_DefaultZero asserts that on a
// fresh Metrics with no test ever completed there is no sample-count
// series exported. The gauge vec is empty until the first call to
// SetSpeedTestSamplesCount, which is the desired behaviour: an
// uninitialised series would mislead "samples_count > 0" alerts.
// PRD #283 slice 3 / issue #286 user story 17.
func TestPrometheus_SpeedTestSamplesCount_DefaultZero(t *testing.T) {
	m := NewMetrics()
	body := scrapeMetrics(t, m)
	if strings.Contains(body, "nasdoctor_speedtest_samples_count{") {
		t.Errorf("expected NO nasdoctor_speedtest_samples_count series before any test completes; body:\n%s", body)
	}
}

// TestPrometheus_SpeedTestSamplesCount_StampsLatest asserts that
// SetSpeedTestSamplesCount populates a single label-value pair for
// the most recent test_id, and that subsequent calls REPLACE the
// label rather than accumulating new series. This is the
// cardinality-protection contract: if a future bug regressed and
// stopped resetting the GaugeVec, prometheus would accumulate one
// series per historical test_id forever.
func TestPrometheus_SpeedTestSamplesCount_StampsLatest(t *testing.T) {
	m := NewMetrics()
	m.SetSpeedTestSamplesCount(42, 28)
	body := scrapeMetrics(t, m)
	if !strings.Contains(body, `nasdoctor_speedtest_samples_count{test_id="42"} 28`) {
		t.Errorf("expected samples_count{test_id=42}=28; body:\n%s", body)
	}

	// Second test, different ID — the old series MUST be evicted.
	m.SetSpeedTestSamplesCount(43, 31)
	body = scrapeMetrics(t, m)
	if !strings.Contains(body, `nasdoctor_speedtest_samples_count{test_id="43"} 31`) {
		t.Errorf("expected samples_count{test_id=43}=31 after second call; body:\n%s", body)
	}
	if strings.Contains(body, `test_id="42"`) {
		t.Errorf("samples_count series for test_id=42 was not evicted — cardinality landmine; body:\n%s", body)
	}
}
