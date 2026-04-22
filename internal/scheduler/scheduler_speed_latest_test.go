package scheduler

import (
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #210 (item 6): the scheduler must mirror LastSpeedTestAttempt
// onto s.latest.SpeedTest.LastAttempt in EVERY branch (pending, failed,
// disabled, success), not just success, so the dashboard widget picks
// up the state via /api/v1/snapshot/latest without a separate DB
// round-trip.

// Before the runner invokes, a pending attempt must be visible on
// s.latest.SpeedTest.LastAttempt. This is the "Running initial speed
// test..." render condition. At this point Latest is nil (no Ookla
// result yet), so the widget distinguishes pending-first-boot from
// stable-running by the presence of Latest.
func TestScheduler_SpeedTest_MirrorsPendingOnLatest(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.latest = &internal.Snapshot{} // simulate first scan already ran

	observed := make(chan *internal.SpeedTestAttempt, 1)
	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		// When the runner fires, s.latest.SpeedTest.LastAttempt must
		// already be populated as pending — the scheduler writes
		// pending BEFORE invoking the runner.
		sched.mu.RLock()
		defer sched.mu.RUnlock()
		if sched.latest != nil && sched.latest.SpeedTest != nil {
			observed <- sched.latest.SpeedTest.LastAttempt
		} else {
			observed <- nil
		}
		return &internal.SpeedTestResult{DownloadMbps: 100, UploadMbps: 10, LatencyMs: 5}
	})

	sched.runSpeedTest()

	select {
	case got := <-observed:
		if got == nil {
			t.Fatal("pending attempt not mirrored onto s.latest.SpeedTest.LastAttempt before runner invocation")
		}
		if got.Status != "pending" {
			t.Errorf("mid-run LastAttempt.Status = %q, want pending", got.Status)
		}
	default:
		t.Fatal("runner did not observe cached-snapshot state")
	}
}

// After a successful run, s.latest.SpeedTest.{Latest,LastAttempt,
// Available} must all be populated and coherent. The widget's happy
// path reads Latest; LastAttempt.Status=success is a redundant signal
// that the check/widget trust.
func TestScheduler_SpeedTest_MirrorsSuccessOnLatest(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.latest = &internal.Snapshot{}

	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{DownloadMbps: 200, UploadMbps: 20, LatencyMs: 10}
	})

	sched.runSpeedTest()

	sched.mu.RLock()
	defer sched.mu.RUnlock()
	if sched.latest == nil || sched.latest.SpeedTest == nil {
		t.Fatal("expected s.latest.SpeedTest to be non-nil after success")
	}
	spd := sched.latest.SpeedTest
	if !spd.Available {
		t.Error("SpeedTest.Available = false after success; widget's happy path gate will skip")
	}
	if spd.Latest == nil {
		t.Fatal("SpeedTest.Latest is nil after success; expected the runner result")
	}
	if spd.Latest.DownloadMbps != 200 {
		t.Errorf("SpeedTest.Latest.DownloadMbps = %v, want 200", spd.Latest.DownloadMbps)
	}
	if spd.LastAttempt == nil {
		t.Fatal("SpeedTest.LastAttempt is nil after success; expected success marker")
	}
	if spd.LastAttempt.Status != "success" {
		t.Errorf("LastAttempt.Status = %q, want success", spd.LastAttempt.Status)
	}
}

// After a failed run, s.latest.SpeedTest.LastAttempt must surface the
// failure so the widget can render a descriptive state (not just
// silently fall back to an empty tile). Latest stays nil.
func TestScheduler_SpeedTest_MirrorsFailureOnLatest(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.latest = &internal.Snapshot{}

	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return nil // simulate Ookla missing or zero-throughput guard hit
	})

	sched.runSpeedTest()

	sched.mu.RLock()
	defer sched.mu.RUnlock()
	if sched.latest.SpeedTest == nil {
		t.Fatal("expected s.latest.SpeedTest to be non-nil after failure (LastAttempt should still ride there)")
	}
	spd := sched.latest.SpeedTest
	if spd.Latest != nil {
		t.Error("SpeedTest.Latest should be nil after failure")
	}
	if spd.LastAttempt == nil {
		t.Fatal("SpeedTest.LastAttempt is nil after failure")
	}
	if spd.LastAttempt.Status != "failed" {
		t.Errorf("LastAttempt.Status = %q, want failed", spd.LastAttempt.Status)
	}
	if spd.LastAttempt.ErrorMsg == "" {
		t.Error("LastAttempt.ErrorMsg is empty on failure; widget + service check depend on a descriptive message")
	}
}

// Disabled branch: s.latest.SpeedTest.LastAttempt shows status=disabled
// so the widget can optionally render a muted state (future UI work;
// current widget does not render anything for disabled, which is fine).
func TestScheduler_SpeedTest_MirrorsDisabledOnLatest(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	sched.latest = &internal.Snapshot{}
	sched.SetSpeedTestInterval(SpeedTestIntervalDisabled)

	sched.runSpeedTest()

	sched.mu.RLock()
	defer sched.mu.RUnlock()
	if sched.latest.SpeedTest == nil {
		t.Fatal("expected s.latest.SpeedTest to be non-nil after disabled transition")
	}
	if sched.latest.SpeedTest.LastAttempt == nil {
		t.Fatal("LastAttempt is nil after disabled transition")
	}
	if sched.latest.SpeedTest.LastAttempt.Status != "disabled" {
		t.Errorf("LastAttempt.Status = %q, want disabled", sched.latest.SpeedTest.LastAttempt.Status)
	}
}

// Edge case: if s.latest is nil (first scan hasn't completed yet —
// scheduler hasn't populated the cached snapshot), the attempt must
// still be written to the store so the service check can read it.
// No panic on the nil dereference.
func TestScheduler_SpeedTest_NilLatest_StoreStillUpdated(t *testing.T) {
	store := storage.NewFakeStore()
	sched := newSchedulerForTest(store)
	// Do NOT set sched.latest — leave it nil.

	sched.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{DownloadMbps: 50}
	})

	// Should not panic.
	sched.runSpeedTest()

	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil {
		t.Fatal("attempt not written to store when s.latest is nil")
	}
	if att.Status != "success" {
		t.Errorf("Status = %q, want success", att.Status)
	}
}
