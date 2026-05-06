package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestRunSpeedCheck_FleetTargetReadsLocalHistory pins issue #215's
// resolution: a legacy saved speed check carrying a non-empty
// Instance (= a fleet server ID) is evaluated against the LOCAL
// speedtest_history table, exactly the same as a check with
// Instance="". This is the same behaviour as every other service
// check type — the Instance field is decorative metadata that the
// scheduler does not dispatch on (see runSpeedCheck docstring +
// internal/models.go ServiceCheckConfig.Instance docs).
//
// The actual user-visible fix is in the Settings UI
// (onServiceTypeChange in settings.html hides the Instance picker
// when type=speed and coerces the value to ""). This test exists
// because legacy saved configs may already have a non-empty
// Instance and we want to document + guard the runtime behaviour:
// the scheduler reads local data and never silently routes
// elsewhere. If a future PR wires up real fleet dispatch (alongside
// #205), this test will fail and the diff will surface the
// behaviour change for explicit review.
func TestRunSpeedCheck_FleetTargetReadsLocalHistory(t *testing.T) {
	tests := []struct {
		name     string
		instance string
	}{
		{"local target (Instance empty)", ""},
		{"legacy fleet target (Instance set)", "fleet-server-id-abc123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc, store := newTestChecker()

			now := time.Now().UTC()
			if err := store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
				Timestamp: now.Add(-1 * time.Minute),
				Status:    "success",
			}); err != nil {
				t.Fatalf("SaveSpeedTestAttempt: %v", err)
			}
			if err := store.SaveSpeedTest("local-row-1", &internal.SpeedTestResult{
				Timestamp:    now.Add(-1 * time.Minute),
				DownloadMbps: 750,
				UploadMbps:   200,
				LatencyMs:    8,
			}); err != nil {
				t.Fatalf("SaveSpeedTest: %v", err)
			}

			check := internal.ServiceCheckConfig{
				Name:     "speed-fleet-or-local",
				Type:     internal.ServiceCheckSpeed,
				Enabled:  true,
				Instance: tc.instance,
				// Blank thresholds → success path = up regardless
				// of measured throughput. Lets us prove the path
				// reached the local history row.
			}

			result := sc.RunCheck(check, now)
			if result.Status != "up" {
				t.Fatalf("expected up (local history is %.0f/%.0f Mbps), got %q (error=%q)",
					750.0, 200.0, result.Status, result.Error)
			}
			if result.DownloadMbps != 750 {
				t.Errorf("DownloadMbps = %.0f, want 750 (proves local history was read regardless of Instance=%q)",
					result.DownloadMbps, tc.instance)
			}
			if result.UploadMbps != 200 {
				t.Errorf("UploadMbps = %.0f, want 200 (proves local history was read regardless of Instance=%q)",
					result.UploadMbps, tc.instance)
			}
		})
	}
}
