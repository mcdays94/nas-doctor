package notifier

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// TestPrometheus_SpeedTestEngineGauge_StampsActiveEngine asserts that
// after Update() with a snapshot whose latest result was produced by
// speedtest-go, /metrics exports
// nasdoctor_speedtest_engine{engine="speedtest_go"} 1 and
// nasdoctor_speedtest_engine{engine="ookla_cli"} 0. Mirrors the
// engine-swap visibility requirement from PRD #283 / issue #284 user
// story 17.
func TestPrometheus_SpeedTestEngineGauge_StampsActiveEngine(t *testing.T) {
	m := NewMetrics()
	m.Update(&internal.Snapshot{
		SpeedTest: &internal.SpeedTestInfo{
			Available: true,
			Latest: &internal.SpeedTestResult{
				Timestamp:    time.Now(),
				DownloadMbps: 500, UploadMbps: 50, LatencyMs: 4,
				Engine: internal.SpeedTestEngineSpeedTestGo,
			},
		},
	})

	body := scrapeMetrics(t, m)

	// Both labels must be present (we always stamp them); the
	// active engine reads 1, the other reads 0.
	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="speedtest_go"} 1`) {
		t.Errorf("expected nasdoctor_speedtest_engine{engine=\"speedtest_go\"} 1 in /metrics; body:\n%s", body)
	}
	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="ookla_cli"} 0`) {
		t.Errorf("expected nasdoctor_speedtest_engine{engine=\"ookla_cli\"} 0 in /metrics; body:\n%s", body)
	}
}

// TestPrometheus_SpeedTestEngineGauge_FallsBackToOoklaCLI asserts that
// pre-#284 historical rows (which carry no Engine field) cause the
// gauge to fall back to the ookla_cli label. This preserves
// continuity for installs upgrading from v0.9.x → post-#284 where
// the dashboard widget's "latest" is still the pre-switchover row.
func TestPrometheus_SpeedTestEngineGauge_FallsBackToOoklaCLI(t *testing.T) {
	m := NewMetrics()
	m.Update(&internal.Snapshot{
		SpeedTest: &internal.SpeedTestInfo{
			Available: true,
			Latest: &internal.SpeedTestResult{
				Timestamp:    time.Now(),
				DownloadMbps: 80, UploadMbps: 8, LatencyMs: 25,
				// Engine deliberately empty — pre-#284 row.
			},
		},
	})

	body := scrapeMetrics(t, m)
	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="ookla_cli"} 1`) {
		t.Errorf("expected nasdoctor_speedtest_engine{engine=\"ookla_cli\"} 1 (fallback for pre-#284 unstamped row); body:\n%s", body)
	}
	if !strings.Contains(body, `nasdoctor_speedtest_engine{engine="speedtest_go"} 0`) {
		t.Errorf("expected nasdoctor_speedtest_engine{engine=\"speedtest_go\"} 0; body:\n%s", body)
	}
}

// scrapeMetrics serves a single /metrics request via httptest and
// returns the response body.
func scrapeMetrics(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}).ServeHTTP(rec, req)
	return rec.Body.String()
}
