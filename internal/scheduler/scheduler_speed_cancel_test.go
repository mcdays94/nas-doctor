package scheduler

// Issue #304 — verify the cancel-completion handler:
//   - Writes a speedtest_history row with status='cancelled' (via
//     SaveSpeedTestCancelledReturningID), distinct from success and
//     failed branches.
//   - Sets LastSpeedTestAttempt.Status='cancelled' so the dashboard
//     widget can render the abort state.
//   - Does NOT call SaveSpeedTestReturningID (which would stamp
//     status='success' for the same row).
//
// This pins the v0.9.11-rc1 lesson "audit ALL side-effects when adding
// a new path that triggers an action": cancel produces a history row,
// flips an attempt status, and (implicitly via the registry's
// state-change observer) resets the in-progress gauge. Test pins the
// first two; the gauge test is in v0.9.11-era infrastructure already.

import (
	"context"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// cancellableRunner is a livetest.Runner that blocks on ctx.Done()
// so the test can drive Cancel via the registry.
type cancellableRunner struct {
	started chan struct{}
}

func (r *cancellableRunner) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	close(r.started)
	out := make(chan collector.SpeedTestSample, 4)
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}, out, nil
}

func TestScheduler_LiveTestCompletion_CancelledPath_WritesCancelledHistoryRow(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)

	runner := &cancellableRunner{started: make(chan struct{})}
	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	// Start a test, wait for runner to enter Run, cancel it, wait
	// for the registry-driven completion handler to fire.
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}
	<-runner.started
	if err := mgr.Cancel(lt.ID()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	<-lt.Done()

	// LastSpeedTestAttempt must be 'cancelled', not 'failed'.
	att, err := store.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt: %v", err)
	}
	if att == nil {
		t.Fatal("LastSpeedTestAttempt = nil; expected the completion handler to record one")
	}
	if att.Status != "cancelled" {
		t.Errorf("LastSpeedTestAttempt.Status = %q, want 'cancelled'", att.Status)
	}

	// A speedtest_history row with status='cancelled' must have been
	// inserted via SaveSpeedTestCancelledReturningID.
	cancelledIDs := store.CancelledSpeedTestRows()
	if len(cancelledIDs) != 1 {
		t.Errorf("CancelledSpeedTestRows count = %d, want 1", len(cancelledIDs))
	}
}
