package scheduler

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestScheduler_SetSMARTMaxAgeDays_RoundTrips confirms the setter
// introduced for issue #238 actually mutates the field the
// StaleSMARTChecker consumes. This is the seam the API settings
// handler uses to push user changes through to the scheduler.
func TestScheduler_SetSMARTMaxAgeDays_RoundTrips(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	col := collector.New(internal.HostPaths{}, logger)
	fake := storage.NewFakeStore()
	s := New(col, fake, &notifier.Notifier{}, nil, logger, 30*time.Minute)

	// Default from constructor matches defaultSettings().SMART.MaxAgeDays.
	if got := s.smartMaxAgeDays; got != 7 {
		t.Errorf("default smartMaxAgeDays = %d, want 7 (matches defaultSettings)", got)
	}

	s.SetSMARTMaxAgeDays(14)
	if got := s.smartMaxAgeDays; got != 14 {
		t.Errorf("after SetSMARTMaxAgeDays(14), got %d", got)
	}

	// 0 is the disabled sentinel — RunOnce must skip the Check+Apply
	// pass entirely when this is set (user story 5). We verify the
	// field update here; end-to-end disabled behaviour is exercised
	// by TestStaleSMARTChecker_Integration_MaxAgeZeroNoForceWake.
	s.SetSMARTMaxAgeDays(0)
	if got := s.smartMaxAgeDays; got != 0 {
		t.Errorf("after SetSMARTMaxAgeDays(0), got %d", got)
	}
}
