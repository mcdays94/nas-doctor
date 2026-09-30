package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #339 (follow-up to #333): the speed-test loop ran a test 2 minutes
// after every start, whatever the cadence, and counted the interval from boot.
// On a weekly schedule that meant an extra full-rate test on every restart.
// speedTestStartupRun decides whether the startup run happens and which
// lastRun the loop resumes from.
func TestSpeedTestStartupRun(t *testing.T) {
	now := time.Date(2026, 9, 30, 13, 31, 0, 0, time.UTC)
	cases := []struct {
		name      string
		lastTest  time.Time
		interval  time.Duration
		scheduled bool
		wantRun   bool
		wantLast  time.Time
	}{
		{"disabled still records its state", now.Add(-time.Hour), SpeedTestIntervalDisabled, false, true, now},
		{"fresh install gets a first test", time.Time{}, 4 * time.Hour, false, true, now},
		{"fresh install with a schedule gets a first test", time.Time{}, 4 * time.Hour, true, true, now},
		{"schedule waits for its next time", now.Add(-72 * time.Hour), 4 * time.Hour, true, false, now.Add(-72 * time.Hour)},
		{"interval resumes from the last test", now.Add(-1 * time.Hour), 4 * time.Hour, false, false, now.Add(-1 * time.Hour)},
		{"interval runs when a test is overdue", now.Add(-5 * time.Hour), 4 * time.Hour, false, true, now},
		{"future timestamp is clamped to now", now.Add(time.Hour), 4 * time.Hour, false, false, now},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run, last := speedTestStartupRun(now, c.lastTest, c.interval, c.scheduled)
			if run != c.wantRun || !last.Equal(c.wantLast) {
				t.Errorf("speedTestStartupRun = (%v, %v); want (%v, %v)", run, last, c.wantRun, c.wantLast)
			}
		})
	}
}

// The startup plan must read the newest persisted result. This is the #333
// reporter's setup: a weekly schedule and a test already on record, so a
// restart must not run one.
func TestSpeedTestStartupPlan_WeeklyScheduleSkipsRestartRun(t *testing.T) {
	s := newSpeedTestSchedulerForTest(4*time.Hour, nil)
	s.SetSpeedTestSchedule([]string{"06:00"}, "sunday", "weekly")
	now := time.Now()
	lastTest := now.Add(-26 * time.Hour)
	if err := s.store.SaveSpeedTest("snap-1", &internal.SpeedTestResult{Timestamp: lastTest, DownloadMbps: 900}); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	run, last := s.speedTestStartupPlan(now)
	if run {
		t.Error("startup plan runs a test on a weekly schedule with a test on record; want it skipped")
	}
	if !last.Equal(lastTest) {
		t.Errorf("lastRun = %v; want the last persisted test %v", last, lastTest)
	}
}

// With nothing on record the loop still runs its first test, so a new
// install gets a reading without waiting for the cadence.
func TestSpeedTestStartupPlan_NoResultRunsFirstTest(t *testing.T) {
	s := newSpeedTestSchedulerForTest(4*time.Hour, nil)
	now := time.Now()

	run, last := s.speedTestStartupPlan(now)
	if !run || !last.Equal(now) {
		t.Errorf("startup plan with no result = (%v, %v); want (true, %v)", run, last, now)
	}
}
