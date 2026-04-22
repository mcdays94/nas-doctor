package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #210 — rewrite runSpeedCheck to read from LastSpeedTestAttempt
// + latest speedtest_history row instead of invoking Ookla directly.
// The scheduled ServiceChecker.SetSpeedTestRunner wiring is left in
// place for the ad-hoc Test-button path but is no longer consulted by
// the scheduled path.

// TestRunCheck_Speed_UsesLastAttempt_SuccessWithHistory_AppliesThresholds
// validates that when the stored attempt is "success" and a recent
// history row exists, the check applies contracted thresholds + margin
// to derive the up/degraded/down status.
func TestRunCheck_Speed_UsesLastAttempt_SuccessWithHistory_AppliesThresholds(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-5 * time.Minute),
		Status:    "success",
	})
	_ = store.SaveSpeedTest("test-1", &internal.SpeedTestResult{
		Timestamp:    now.Add(-5 * time.Minute),
		DownloadMbps: 500,
		UploadMbps:   100,
		LatencyMs:    5,
	})

	check := internal.ServiceCheckConfig{
		Name:               "speed-ok",
		Type:               internal.ServiceCheckSpeed,
		Enabled:            true,
		ContractedDownMbps: 400,
		ContractedUpMbps:   80,
		MarginPct:          10,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "up" {
		t.Fatalf("expected status=up, got %q (error=%q)", result.Status, result.Error)
	}
	if result.DownloadMbps != 500 {
		t.Errorf("DownloadMbps = %.0f, want 500", result.DownloadMbps)
	}
	if result.UploadMbps != 100 {
		t.Errorf("UploadMbps = %.0f, want 100", result.UploadMbps)
	}
	if result.DownloadOK == nil || !*result.DownloadOK {
		t.Error("DownloadOK should be true")
	}
	if result.UploadOK == nil || !*result.UploadOK {
		t.Error("UploadOK should be true")
	}
}

// Download below contracted threshold with upload passing → degraded.
func TestRunCheck_Speed_UsesLastAttempt_SuccessWithHistory_DownloadBelow(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-1 * time.Minute),
		Status:    "success",
	})
	_ = store.SaveSpeedTest("test-1", &internal.SpeedTestResult{
		Timestamp:    now.Add(-1 * time.Minute),
		DownloadMbps: 100, // below 400 * 0.9 = 360
		UploadMbps:   100,
		LatencyMs:    5,
	})

	check := internal.ServiceCheckConfig{
		Name:               "speed-dl-low",
		Type:               internal.ServiceCheckSpeed,
		Enabled:            true,
		ContractedDownMbps: 400,
		ContractedUpMbps:   80,
		MarginPct:          10,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "degraded" {
		t.Fatalf("expected degraded, got %q (error=%q)", result.Status, result.Error)
	}
	if !strings.Contains(result.Error, "download below contracted speed") {
		t.Errorf("expected download-related error, got %q", result.Error)
	}
}

// Blank thresholds (both zero) → report up unconditionally on success.
// This is the default shape of the shipped-by-default "Internet Speed"
// check — a heartbeat rather than a threshold alert.
func TestRunCheck_Speed_UsesLastAttempt_SuccessBlankThresholds_Up(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-1 * time.Minute),
		Status:    "success",
	})
	_ = store.SaveSpeedTest("test-1", &internal.SpeedTestResult{
		Timestamp:    now.Add(-1 * time.Minute),
		DownloadMbps: 50, // would fail any non-zero threshold
		UploadMbps:   10,
		LatencyMs:    20,
	})

	check := internal.ServiceCheckConfig{
		Name:    "speed-heartbeat",
		Type:    internal.ServiceCheckSpeed,
		Enabled: true,
		// Thresholds blank — zero values.
	}

	result := sc.RunCheck(check, now)
	if result.Status != "up" {
		t.Fatalf("expected up with blank thresholds, got %q (error=%q)", result.Status, result.Error)
	}
}

// Status "failed" → check reports down with the stored error message.
func TestRunCheck_Speed_UsesLastAttempt_Failed_ReportsStoredError(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-2 * time.Minute),
		Status:    "failed",
		ErrorMsg:  "ookla binary not found",
	})

	check := internal.ServiceCheckConfig{
		Name:    "speed-fail",
		Type:    internal.ServiceCheckSpeed,
		Enabled: true,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "down" {
		t.Fatalf("expected down, got %q", result.Status)
	}
	if !strings.Contains(result.Error, "ookla binary not found") {
		t.Errorf("expected stored error to surface, got %q", result.Error)
	}
}

// Status "pending" → check reports up with an "in progress" message.
func TestRunCheck_Speed_UsesLastAttempt_Pending_ReportsInProgress(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-30 * time.Second),
		Status:    "pending",
	})

	check := internal.ServiceCheckConfig{
		Name:    "speed-pending",
		Type:    internal.ServiceCheckSpeed,
		Enabled: true,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "up" {
		t.Fatalf("expected up while pending, got %q (error=%q)", result.Status, result.Error)
	}
}

// Status "disabled" → check reports down with "disabled in settings".
func TestRunCheck_Speed_UsesLastAttempt_Disabled_ReportsDown(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-1 * time.Hour),
		Status:    "disabled",
	})

	check := internal.ServiceCheckConfig{
		Name:    "speed-disabled",
		Type:    internal.ServiceCheckSpeed,
		Enabled: true,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "down" {
		t.Fatalf("expected down, got %q", result.Status)
	}
	if !strings.Contains(result.Error, "disabled") {
		t.Errorf("expected 'disabled' in error, got %q", result.Error)
	}
}

// Attempt state older than 30 days → stale, reports down.
func TestRunCheck_Speed_UsesLastAttempt_Stale_ReportsDown(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-40 * 24 * time.Hour),
		Status:    "success",
	})

	check := internal.ServiceCheckConfig{
		Name:               "speed-stale",
		Type:               internal.ServiceCheckSpeed,
		Enabled:            true,
		ContractedDownMbps: 100,
		ContractedUpMbps:   10,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "down" {
		t.Fatalf("expected down on stale state, got %q", result.Status)
	}
	if !strings.Contains(result.Error, "stale") {
		t.Errorf("expected 'stale' in error, got %q", result.Error)
	}
}

// No attempt state stored yet (fresh install pre-first tick) → reports
// down with a "no speed test run yet" message.
func TestRunCheck_Speed_UsesLastAttempt_NoStateYet_ReportsDown(t *testing.T) {
	sc, _ := newTestChecker()

	check := internal.ServiceCheckConfig{
		Name:    "speed-fresh",
		Type:    internal.ServiceCheckSpeed,
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected down on missing state, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error on missing state")
	}
}

// Corrupt/old history row where download+upload are both zero on a
// success-tagged attempt should be treated as failed, not up.
func TestRunCheck_Speed_UsesLastAttempt_SuccessButZeroHistory_ReportsDown(t *testing.T) {
	sc, store := newTestChecker()

	now := time.Now().UTC()
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: now.Add(-1 * time.Minute),
		Status:    "success",
	})
	_ = store.SaveSpeedTest("test-1", &internal.SpeedTestResult{
		Timestamp:    now.Add(-1 * time.Minute),
		DownloadMbps: 0,
		UploadMbps:   0,
		LatencyMs:    0,
	})

	check := internal.ServiceCheckConfig{
		Name:    "speed-zero",
		Type:    internal.ServiceCheckSpeed,
		Enabled: true,
	}

	result := sc.RunCheck(check, now)
	if result.Status != "down" {
		t.Fatalf("expected down on zero-throughput history, got %q (error=%q)", result.Status, result.Error)
	}
}
