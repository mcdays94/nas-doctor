package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// The degraded message has to quote the threshold of the direction that
// failed. It used to quote the download threshold even when only upload was
// below contract ("upload below contracted speed ... threshold 1000").
func TestRunCheck_Speed_UploadBelowQuotesUploadThreshold(t *testing.T) {
	check := internal.ServiceCheckConfig{
		Name:               "speed-ul-low",
		Type:               internal.ServiceCheckSpeed,
		Enabled:            true,
		ContractedDownMbps: 1000,
		ContractedUpMbps:   500,
		MarginPct:          10,
	}
	measured := internal.SpeedTestResult{DownloadMbps: 932, UploadMbps: 445, LatencyMs: 8}

	t.Run("scheduled check reading history", func(t *testing.T) {
		sc, store := newTestChecker()
		now := time.Now().UTC()
		_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{Timestamp: now.Add(-time.Minute), Status: "success"})
		r := measured
		r.Timestamp = now.Add(-time.Minute)
		_ = store.SaveSpeedTest("test-1", &r)
		assertUploadThreshold(t, sc.RunCheck(check, now))
	})

	t.Run("Test button with a runner", func(t *testing.T) {
		sc, _ := newTestChecker()
		sc.SetSpeedTestRunner(func() *internal.SpeedTestResult { r := measured; return &r })
		assertUploadThreshold(t, sc.RunCheck(check, time.Now().UTC()))
	})
}

func assertUploadThreshold(t *testing.T, result internal.ServiceCheckResult) {
	t.Helper()
	if result.Status != "degraded" {
		t.Fatalf("expected degraded, got %q (error=%q)", result.Status, result.Error)
	}
	if !strings.Contains(result.Error, "upload below contracted speed") || !strings.Contains(result.Error, "threshold 500 ") {
		t.Errorf("Error = %q; want the upload threshold, 500", result.Error)
	}
}
