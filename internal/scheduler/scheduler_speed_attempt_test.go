package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #210 — scheduler.runSpeedTest writes LastSpeedTestAttempt
// before and after invoking the speed-test runner, so downstream
// consumers (the scheduled type=speed service check, the dashboard
// widget) see the outcome even when the runner itself has no DB
// side effect.

// On a successful run the recorded state should be {status=success,
// error_msg=""}, and a speedtest_history row must also exist.
func TestScheduler_SpeedTest_RecordsSuccess(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{
			Timestamp:    time.Now().UTC(),
			DownloadMbps: 400,
			UploadMbps:   50,
			LatencyMs:    15,
		}
	})

	sched.runSpeedTest()

	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil {
		t.Fatal("expected attempt state to be recorded, got nil")
	}
	if att.Status != "success" {
		t.Errorf("Status = %q, want success", att.Status)
	}
	if att.ErrorMsg != "" {
		t.Errorf("ErrorMsg = %q, want empty", att.ErrorMsg)
	}
	points, err := store.GetSpeedTestHistory(1)
	if err != nil {
		t.Fatalf("GetSpeedTestHistory: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 speedtest_history row, got %d", len(points))
	}
}

// When the runner returns nil (Ookla missing / network down) the
// attempt state should flip to {status=failed, error_msg=<reason>}.
// No speedtest_history row is written.
func TestScheduler_SpeedTest_RecordsFailure(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return nil
	})

	sched.runSpeedTest()

	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil {
		t.Fatal("expected attempt state after failure, got nil")
	}
	if att.Status != "failed" {
		t.Errorf("Status = %q, want failed", att.Status)
	}
	if att.ErrorMsg == "" {
		t.Error("expected non-empty ErrorMsg on failure")
	}
	points, err := store.GetSpeedTestHistory(1)
	if err != nil {
		t.Fatalf("GetSpeedTestHistory: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("expected 0 history rows on failure, got %d", len(points))
	}
}

// Before invoking the runner, runSpeedTest writes a "pending" attempt
// state so the widget + scheduled check can render the in-progress
// condition correctly for the first-boot gap.
func TestScheduler_SpeedTest_WritesPendingBeforeRun(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)

	observed := make(chan string, 1)
	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		// When the runner runs, the pending state must already be visible.
		att, _ := store.GetLastSpeedTestAttempt()
		if att != nil {
			observed <- att.Status
		} else {
			observed <- "<nil>"
		}
		return &internal.SpeedTestResult{DownloadMbps: 100, UploadMbps: 10, LatencyMs: 5}
	})

	sched.runSpeedTest()

	select {
	case got := <-observed:
		if got != "pending" {
			t.Fatalf("mid-run attempt status = %q, want pending", got)
		}
	default:
		t.Fatal("runner did not observe attempt state")
	}

	// After the run completes the status should be success, not pending.
	att, _ := store.GetLastSpeedTestAttempt()
	if att == nil || att.Status != "success" {
		t.Fatalf("post-run status = %v, want success", att)
	}
}

// When the speed-test interval is set to the Disabled sentinel, the
// scheduler should record status=disabled once and skip invocation.
// Repeated calls must not churn writes (idempotent).
func TestScheduler_SpeedTest_DisabledWritesOnce(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.SetSpeedTestInterval(SpeedTestIntervalDisabled)

	invocations := 0
	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		invocations++
		return &internal.SpeedTestResult{DownloadMbps: 1}
	})

	sched.runSpeedTest()
	sched.runSpeedTest()
	sched.runSpeedTest()

	if invocations != 0 {
		t.Errorf("runner should not be invoked when disabled, got %d calls", invocations)
	}

	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil {
		t.Fatal("expected attempt state, got nil")
	}
	if att.Status != "disabled" {
		t.Errorf("Status = %q, want disabled", att.Status)
	}

	// Idempotency: the timestamp from the first disabled write should not
	// move on subsequent calls (no churn). We approximate this by saving
	// an earlier timestamp and confirming it survives a redundant call.
	baseline := att.Timestamp
	time.Sleep(5 * time.Millisecond)
	sched.runSpeedTest()
	att2, _ := store.GetLastSpeedTestAttempt()
	if !att2.Timestamp.Equal(baseline) {
		t.Errorf("disabled attempt timestamp changed on second call (not idempotent): %v → %v", baseline, att2.Timestamp)
	}
}
