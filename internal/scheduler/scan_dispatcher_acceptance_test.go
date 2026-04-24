package scheduler

import (
	"testing"
	"time"
)

// TestScanDispatcher_Acceptance_Docker5minSMART1dayGlobal30m pins the
// acceptance criterion from issue #260:
//
//	"Setting Docker.IntervalSec = 300 and SMART.IntervalSec = 86400
//	 + global 30m: FastestInterval returns 300s; each tick dispatcher
//	 fires Docker and, once per day, SMART"
//
// This is the headline user-visible outcome of slice 2b. If this
// test breaks, a user who configured Docker-every-5m + SMART-every-day
// would see either the wrong cadence OR the dispatcher firing SMART
// every 5 minutes (waking drives unnecessarily).
func TestScanDispatcher_Acceptance_Docker5minSMART1dayGlobal30m(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	clock := start
	nowFn := func() time.Time { return clock }

	d := NewScanDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,   // 5 min
		SMARTSec:  86400, // 1 day
	}, 30*time.Minute, nowFn)

	// Precondition: FastestInterval = 5 min (Docker).
	if got, want := d.FastestInterval(), 5*time.Minute; got != want {
		t.Fatalf("FastestInterval = %v, want 5m", got)
	}

	// First tick at start: everything is due (never ran).
	firstDue := d.Tick(start)
	if len(firstDue) != len(configurableSubsystems) {
		t.Errorf("first tick should fire all subsystems; got %v", firstDue)
	}

	// Mark all as ran at start.
	for _, name := range firstDue {
		d.MarkRan(name, start)
	}

	// Advance 5 minutes — Docker should be due; SMART should NOT be.
	clock = start.Add(5 * time.Minute)
	due := d.Tick(clock)
	hasDocker, hasSmart := false, false
	for _, name := range due {
		if name == "docker" {
			hasDocker = true
		}
		if name == "smart" {
			hasSmart = true
		}
	}
	if !hasDocker {
		t.Errorf("Docker should be due at t+5m; got %v", due)
	}
	if hasSmart {
		t.Errorf("SMART should NOT be due at t+5m (interval is 1 day); got %v", due)
	}

	// Ran Docker. Advance another 5 min.
	d.MarkRan("docker", clock)
	clock = start.Add(10 * time.Minute)
	due = d.Tick(clock)
	hasSmart = false
	for _, name := range due {
		if name == "smart" {
			hasSmart = true
		}
	}
	if hasSmart {
		t.Errorf("SMART still not due at t+10m; got %v", due)
	}

	// Fast-forward to t+1day: SMART finally due.
	clock = start.Add(24 * time.Hour)
	due = d.Tick(clock)
	hasSmart = false
	for _, name := range due {
		if name == "smart" {
			hasSmart = true
		}
	}
	if !hasSmart {
		t.Errorf("SMART should be due at t+1d; got %v", due)
	}
}

// TestScanDispatcher_Acceptance_SMART5minGlobal30m pins the
// second acceptance bullet: "SMART.IntervalSec = 300 and observing
// the logs: SMART fires every 5 min, global subsystems every 30 min".
func TestScanDispatcher_Acceptance_SMART5minGlobal30m(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	clock := start
	nowFn := func() time.Time { return clock }
	d := NewScanDispatcher(DispatcherIntervalsConfig{
		SMARTSec: 300,
	}, 30*time.Minute, nowFn)

	// FastestInterval = 5m (SMART).
	if got := d.FastestInterval(); got != 5*time.Minute {
		t.Fatalf("FastestInterval = %v, want 5m", got)
	}

	// Mark all as ran at start.
	for _, name := range configurableSubsystems {
		d.MarkRan(name, start)
	}

	// t+5m: only SMART should be due (everything else is at 30m global).
	clock = start.Add(5 * time.Minute)
	due := d.Tick(clock)
	if len(due) != 1 || due[0] != "smart" {
		t.Errorf("t+5m: expected only smart due; got %v", due)
	}

	// Mark SMART ran; advance another 5m.
	d.MarkRan("smart", clock)
	clock = start.Add(10 * time.Minute)
	due = d.Tick(clock)
	if len(due) != 1 || due[0] != "smart" {
		t.Errorf("t+10m: expected only smart due (again); got %v", due)
	}

	// t+30m: SMART + everything else (all global subsystems hit their 30m).
	d.MarkRan("smart", clock)
	clock = start.Add(30 * time.Minute)
	due = d.Tick(clock)
	if len(due) < 5 {
		t.Errorf("t+30m: expected all global subsystems due; got %v", due)
	}
}
