package scheduler

import (
	"sort"
	"testing"
	"time"
)

// helper: build a fixed-clock dispatcher.
func newFixedClockDispatcher(cfg DispatcherIntervalsConfig, global time.Duration, start time.Time) (*ScanDispatcher, *time.Time) {
	clock := start
	nowFn := func() time.Time { return clock }
	d := NewScanDispatcher(cfg, global, nowFn)
	return d, &clock
}

func assertSameSet(t *testing.T, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if len(g) != len(w) {
		t.Errorf("got %v, want %v", got, want)
		return
	}
	for i := range g {
		if g[i] != w[i] {
			t.Errorf("got %v, want %v", got, want)
			return
		}
	}
}

// TestScanDispatcher_Tick_AllUseGlobal_FirstTickFiresAll: with every
// subsystem at IntervalSec=0 (use global), the very first Tick after
// construction must return every configurable subsystem — matching
// the pre-slice-2 "run everything once on startup" behaviour.
func TestScanDispatcher_Tick_AllUseGlobal_FirstTickFiresAll(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{}, 30*time.Minute, start)

	due := d.Tick(start)
	assertSameSet(t, due, ConfigurableSubsystems())
}

// TestScanDispatcher_Tick_AfterMarkRanWithinInterval_NotDue: once a
// subsystem has MarkRan called, subsequent Ticks within the interval
// must not return it.
func TestScanDispatcher_Tick_AfterMarkRanWithinInterval_NotDue(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300, // 5 min
	}, 30*time.Minute, start)

	d.MarkRan("docker", start)

	// 2 min later: not due.
	due := d.Tick(start.Add(2 * time.Minute))
	for _, name := range due {
		if name == "docker" {
			t.Errorf("docker returned as due 2m after MarkRan with 5m interval; due=%v", due)
		}
	}
}

// TestScanDispatcher_Tick_AfterIntervalElapsed_Due: at exactly the
// interval boundary a subsystem is due again.
func TestScanDispatcher_Tick_AfterIntervalElapsed_Due(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,
	}, 30*time.Minute, start)

	d.MarkRan("docker", start)

	due := d.Tick(start.Add(5 * time.Minute))
	foundDocker := false
	for _, name := range due {
		if name == "docker" {
			foundDocker = true
		}
	}
	if !foundDocker {
		t.Errorf("expected docker due exactly 5m after MarkRan; got %v", due)
	}
}

// TestScanDispatcher_Tick_MixedIntervals_OnlyDueFire: Docker every 5
// min, SMART every 1 hour, others use 30m global — at t=5m, only
// Docker should be due (others were just run at t=0).
func TestScanDispatcher_Tick_MixedIntervals_OnlyDueFire(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,  // 5 min
		SMARTSec:  3600, // 1 h
	}, 30*time.Minute, start)

	// Simulate first tick at start — mark everything ran.
	for _, name := range ConfigurableSubsystems() {
		d.MarkRan(name, start)
	}

	// 5 minutes later: only docker should be due.
	due := d.Tick(start.Add(5 * time.Minute))
	assertSameSet(t, due, []string{"docker"})
}

// TestScanDispatcher_Tick_UseGlobalFallback_MatchesGlobal: a subsystem
// at IntervalSec=0 fires on the global cadence. With global=30m, SMART
// at IntervalSec=0, marking SMART ran at start, Tick at 30m must
// include smart.
func TestScanDispatcher_Tick_UseGlobalFallback_MatchesGlobal(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		// SMARTSec=0 means use global
	}, 30*time.Minute, start)

	d.MarkRan("smart", start)

	// 29 min in — not yet due.
	due := d.Tick(start.Add(29 * time.Minute))
	for _, name := range due {
		if name == "smart" {
			t.Errorf("smart due at 29m with global=30m; got %v", due)
		}
	}

	// 30 min in — due.
	due = d.Tick(start.Add(30 * time.Minute))
	found := false
	for _, name := range due {
		if name == "smart" {
			found = true
		}
	}
	if !found {
		t.Errorf("smart not due at 30m with global=30m; got %v", due)
	}
}

// TestScanDispatcher_MarkRan_IgnoresUnknown: passing a name outside
// the configurable list must be a no-op, not a panic.
func TestScanDispatcher_MarkRan_IgnoresUnknown(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{}, 30*time.Minute, start)

	// Must not panic. Must not affect internal state.
	d.MarkRan("system", start)   // not configurable
	d.MarkRan("unknown", start)  // not configurable
	d.MarkRan("", start)         // empty

	// All configurable subsystems should still be due (no MarkRan was
	// recorded for any of them).
	due := d.Tick(start.Add(time.Second))
	assertSameSet(t, due, ConfigurableSubsystems())
}

// TestScanDispatcher_FastestInterval_AllGlobal_ReturnsGlobal: when no
// subsystem has a per-subsystem override, FastestInterval is global.
func TestScanDispatcher_FastestInterval_AllGlobal_ReturnsGlobal(t *testing.T) {
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{}, 30*time.Minute, time.Now())
	if got, want := d.FastestInterval(), 30*time.Minute; got != want {
		t.Errorf("FastestInterval = %v, want %v", got, want)
	}
}

// TestScanDispatcher_FastestInterval_PicksMinimum: with Docker at 5m
// and global at 30m, FastestInterval is 5m.
func TestScanDispatcher_FastestInterval_PicksMinimum(t *testing.T) {
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,
	}, 30*time.Minute, time.Now())
	if got, want := d.FastestInterval(), 5*time.Minute; got != want {
		t.Errorf("FastestInterval = %v, want %v", got, want)
	}
}

// TestScanDispatcher_FastestInterval_LongerSubsystemDoesNotWin: with
// SMART at 1d but global at 30m, FastestInterval is 30m (global is
// still faster than SMART's override).
func TestScanDispatcher_FastestInterval_LongerSubsystemDoesNotWin(t *testing.T) {
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		SMARTSec: 86400, // 1 day
	}, 30*time.Minute, time.Now())
	if got, want := d.FastestInterval(), 30*time.Minute; got != want {
		t.Errorf("FastestInterval = %v, want %v (SMART=1d must not beat global=30m)", got, want)
	}
}

// TestScanDispatcher_FastestInterval_PicksFromMixed: Docker at 5m and
// SMART at 1h, global 30m — fastest is Docker's 5m.
func TestScanDispatcher_FastestInterval_PicksFromMixed(t *testing.T) {
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,
		SMARTSec:  3600,
	}, 30*time.Minute, time.Now())
	if got, want := d.FastestInterval(), 5*time.Minute; got != want {
		t.Errorf("FastestInterval = %v, want %v", got, want)
	}
}

// TestScanDispatcher_UpdateIntervals_ResetsLastRunForChangedOnly: when
// Docker's interval changes from 5m to 10m, docker's lastRun is
// cleared (so the new cadence kicks in immediately) but SMART's
// lastRun is preserved (interval unchanged).
func TestScanDispatcher_UpdateIntervals_ResetsLastRunForChangedOnly(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,
		SMARTSec:  3600,
	}, 30*time.Minute, start)

	// Simulate first tick at start.
	for _, name := range ConfigurableSubsystems() {
		d.MarkRan(name, start)
	}

	// Apply new config: Docker changed to 600s, SMART unchanged.
	d.UpdateIntervals(DispatcherIntervalsConfig{
		DockerSec: 600,
		SMARTSec:  3600,
	}, 30*time.Minute)

	// At t=1s: docker must be due (lastRun cleared), SMART must NOT be
	// due (lastRun preserved, 1s < 1h).
	due := d.Tick(start.Add(time.Second))
	foundDocker, foundSmart := false, false
	for _, name := range due {
		if name == "docker" {
			foundDocker = true
		}
		if name == "smart" {
			foundSmart = true
		}
	}
	if !foundDocker {
		t.Errorf("docker should be due after UpdateIntervals changed its interval; got %v", due)
	}
	if foundSmart {
		t.Errorf("smart should NOT be due; its interval was unchanged; got %v", due)
	}
}

// TestScanDispatcher_UpdateIntervals_GlobalChangeAffectsUseGlobalSubsystems:
// when a subsystem is on "use global" and global changes, that
// subsystem's effective interval changes and its lastRun resets.
func TestScanDispatcher_UpdateIntervals_GlobalChangeAffectsUseGlobalSubsystems(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		// all zero = use global
	}, 30*time.Minute, start)

	// Run everything at start.
	for _, name := range ConfigurableSubsystems() {
		d.MarkRan(name, start)
	}

	// Verify nothing is due at t+5s (still on 30m cadence).
	due := d.Tick(start.Add(5 * time.Second))
	if len(due) != 0 {
		t.Errorf("no subsystem should be due 5s after run, global=30m; got %v", due)
	}

	// Reduce global to 10s — every subsystem's effective interval
	// changes, so all lastRun values reset.
	d.UpdateIntervals(DispatcherIntervalsConfig{}, 10*time.Second)

	// Immediately all subsystems should be due.
	due = d.Tick(start.Add(6 * time.Second))
	assertSameSet(t, due, ConfigurableSubsystems())
}

// TestScanDispatcher_LastRunMap_ExcludesNeverRan: subsystems that
// have not yet MarkRan'd are omitted from LastRunMap.
func TestScanDispatcher_LastRunMap_ExcludesNeverRan(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{}, 30*time.Minute, start)

	d.MarkRan("docker", start)
	d.MarkRan("smart", start.Add(5*time.Minute))

	m := d.LastRunMap()
	if len(m) != 2 {
		t.Errorf("LastRunMap size = %d, want 2 (only docker + smart recorded); got %v", len(m), m)
	}
	if _, ok := m["docker"]; !ok {
		t.Errorf("docker missing from LastRunMap; got %v", m)
	}
	if _, ok := m["smart"]; !ok {
		t.Errorf("smart missing from LastRunMap; got %v", m)
	}
	if _, ok := m["proxmox"]; ok {
		t.Errorf("proxmox should be absent from LastRunMap; got %v", m)
	}
}

// TestScanDispatcher_Skipped_ReturnsComplement: the Skipped helper
// returns the configurable subsystems NOT in `due`, in canonical
// order.
func TestScanDispatcher_Skipped_ReturnsComplement(t *testing.T) {
	got := Skipped([]string{"docker", "smart"})
	want := []string{"proxmox", "kubernetes", "zfs", "gpu"}
	if len(got) != len(want) {
		t.Errorf("Skipped len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Skipped[%d] = %s, want %s (canonical order matters); got %v", i, got[i], want[i], got)
		}
	}
}

// TestScanDispatcher_Skipped_EmptyDue_ReturnsAll: when no subsystems
// are due, Skipped is the full list.
func TestScanDispatcher_Skipped_EmptyDue_ReturnsAll(t *testing.T) {
	got := Skipped(nil)
	want := ConfigurableSubsystems()
	if len(got) != len(want) {
		t.Errorf("Skipped len = %d, want %d", len(got), len(want))
	}
}

// TestScanDispatcher_Skipped_AllDue_ReturnsEmpty: when every
// subsystem is due, Skipped is empty.
func TestScanDispatcher_Skipped_AllDue_ReturnsEmpty(t *testing.T) {
	got := Skipped(ConfigurableSubsystems())
	if len(got) != 0 {
		t.Errorf("Skipped should be empty when all are due; got %v", got)
	}
}

// TestScanDispatcher_TickOrdering_Canonical: Tick returns subsystems
// in configurableSubsystems order so log output is deterministic.
func TestScanDispatcher_TickOrdering_Canonical(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{}, 30*time.Minute, start)
	due := d.Tick(start)
	want := ConfigurableSubsystems()
	if len(due) != len(want) {
		t.Fatalf("due len = %d, want %d", len(due), len(want))
	}
	for i := range want {
		if due[i] != want[i] {
			t.Errorf("Tick order not canonical: due[%d]=%s, want %s; got %v, want %v", i, due[i], want[i], due, want)
		}
	}
}

// TestScanDispatcher_FastestInterval_MinimumClampGuard: if somehow
// the dispatcher ended up with zero-or-negative intervals (e.g.
// defensive global clamp in applyIntervalsLocked), FastestInterval
// still returns a positive duration.
func TestScanDispatcher_FastestInterval_NonPositiveGlobalClampsToDefault(t *testing.T) {
	// Pass global=0 which triggers the defensive clamp to 30m.
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{}, 0, time.Now())
	got := d.FastestInterval()
	if got <= 0 {
		t.Errorf("FastestInterval must be positive; got %v", got)
	}
}
