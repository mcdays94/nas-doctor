package scheduler

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #190: an orphan service_checks_history row reappeared after the
// purge even though the scheduler's in-memory check list was correct. Both
// scheduled paths copy the check list, run the checks (seconds, or minutes
// for speed tests), and only then save. A settings save that removes a
// check and prunes its history while a run is in flight used to let the
// run write the removed check's result back afterwards.

// blockingSpeedScheduler returns a scheduler whose single configured check
// is a speed check that blocks inside the runner until release is closed.
// started is closed once the runner has been entered, i.e. once the run has
// already copied the check list.
func blockingSpeedScheduler(t *testing.T, store storage.Store) (sched *Scheduler, check internal.ServiceCheckConfig, started, release chan struct{}) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched = newSchedulerForTest(store)
	sched.checker = NewServiceChecker(store, logger)

	started = make(chan struct{})
	release = make(chan struct{})
	var once sync.Once
	sched.checker.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		once.Do(func() { close(started) })
		<-release
		return &internal.SpeedTestResult{DownloadMbps: 500, UploadMbps: 50, LatencyMs: 10}
	})

	check = internal.ServiceCheckConfig{Name: "Speed", Type: "speed", Target: "ookla", Enabled: true}
	sched.UpdateServiceChecks([]internal.ServiceCheckConfig{check})
	return sched, check, started, release
}

func waitStarted(t *testing.T, started chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("check runner never started")
	}
}

func waitDone(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("service check run never finished")
	}
}

func TestRunDueServiceChecks_CheckRemovedMidRun_LeavesNoOrphan(t *testing.T) {
	store := storage.NewFakeStore()
	sched, check, started, release := blockingSpeedScheduler(t, store)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sched.runDueServiceChecks()
	}()
	waitStarted(t, started)

	// The user deletes the check while it is running.
	sched.UpdateServiceChecks(nil)
	close(release)
	waitDone(t, done)

	if keys := historyKeys(t, store); keys[CheckKey(check)] {
		t.Fatalf("removed check %q was written back to history after the purge: %v", check.Name, keys)
	}
}

func TestRunServiceChecksNow_CheckRemovedMidRun_LeavesNoOrphan(t *testing.T) {
	store := storage.NewFakeStore()
	sched, check, started, release := blockingSpeedScheduler(t, store)

	var results []internal.ServiceCheckResult
	done := make(chan struct{})
	go func() {
		defer close(done)
		results, _ = sched.RunServiceChecksNow()
	}()
	waitStarted(t, started)

	sched.UpdateServiceChecks(nil)
	close(release)
	waitDone(t, done)

	if keys := historyKeys(t, store); keys[CheckKey(check)] {
		t.Fatalf("removed check %q was written back to history after the purge: %v", check.Name, keys)
	}
	// The full-scan path puts these results on the snapshot, so a removed
	// check must not show up there either.
	if len(results) != 0 {
		t.Fatalf("expected no results for a check removed mid-run, got %d", len(results))
	}
}

// Editing a check changes its key (name|type|target|port), so the old key
// is an orphan as soon as the edit is saved.
func TestRunDueServiceChecks_CheckEditedMidRun_KeepsOnlyNewKey(t *testing.T) {
	store := storage.NewFakeStore()
	sched, check, started, release := blockingSpeedScheduler(t, store)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sched.runDueServiceChecks()
	}()
	waitStarted(t, started)

	renamed := check
	renamed.Name = "Speed (renamed)"
	sched.UpdateServiceChecks([]internal.ServiceCheckConfig{renamed})
	close(release)
	waitDone(t, done)

	if keys := historyKeys(t, store); keys[CheckKey(check)] {
		t.Fatalf("old key of an edited check was written back to history: %v", keys)
	}
}

// Control: a check that stays configured is still saved.
func TestRunDueServiceChecks_CheckStillConfigured_IsSaved(t *testing.T) {
	store := storage.NewFakeStore()
	sched, check, _, release := blockingSpeedScheduler(t, store)
	close(release)

	sched.runDueServiceChecks()

	if keys := historyKeys(t, store); !keys[CheckKey(check)] {
		t.Fatalf("configured check %q was not saved: %v", check.Name, keys)
	}
}
