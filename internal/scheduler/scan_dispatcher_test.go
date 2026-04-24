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

// TestScanDispatcher_UpdateIntervals_HonorsElapsedAgainstNewInterval:
// UpdateIntervals preserves lastRun. The natural elapsed-vs-interval
// check in Tick decides whether a subsystem is due against the new
// interval. Exercises both directions:
//
//   - Docker 5m → 10m: elapsed 1s < 10m → NOT due.
//   - SMART 1h unchanged: elapsed 1s < 1h → NOT due.
//
// Advancing past the new intervals fires them naturally without
// needing a lastRun reset.
func TestScanDispatcher_UpdateIntervals_HonorsElapsedAgainstNewInterval(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		DockerSec: 300,
		SMARTSec:  3600,
	}, 30*time.Minute, start)

	// Simulate first tick at start.
	for _, name := range ConfigurableSubsystems() {
		d.MarkRan(name, start)
	}

	// Apply new config: Docker changed to 600s (10m), SMART unchanged.
	d.UpdateIntervals(DispatcherIntervalsConfig{
		DockerSec: 600,
		SMARTSec:  3600,
	}, 30*time.Minute)

	// At t=1s: nothing is due — elapsed 1s is well under every
	// effective interval (10m Docker, 1h SMART, 30m everything else).
	due := d.Tick(start.Add(time.Second))
	for _, name := range due {
		if name == "docker" || name == "smart" {
			t.Errorf("%s should NOT be due 1s after UpdateIntervals; got %v", name, due)
		}
	}

	// Advance to t=10m: Docker is due (10m ≥ 10m), SMART is not
	// (10m < 1h).
	due = d.Tick(start.Add(10 * time.Minute))
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
		t.Errorf("docker should be due at t=10m with new 10m interval; got %v", due)
	}
	if foundSmart {
		t.Errorf("smart should NOT be due at t=10m (1h interval, 10m elapsed); got %v", due)
	}
}

// TestScanDispatcher_UpdateIntervals_GlobalChangePreservesLastRun:
// when a subsystem is on "use global" and global changes, the
// subsystem's lastRun is preserved. The new global interval is
// checked against the original last-run timestamp — a subsystem
// that ran X ago only fires if X ≥ new global.
func TestScanDispatcher_UpdateIntervals_GlobalChangePreservesLastRun(t *testing.T) {
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

	// Reduce global to 10s. With lastRun preserved, the natural
	// elapsed-vs-interval check decides.
	d.UpdateIntervals(DispatcherIntervalsConfig{}, 10*time.Second)

	// At t+6s: 6s < 10s → nothing due yet. The old code incorrectly
	// fired everything here because it reset lastRun on interval
	// change.
	due = d.Tick(start.Add(6 * time.Second))
	if len(due) != 0 {
		t.Errorf("nothing should be due 6s after run with 10s global; got %v", due)
	}

	// At t+11s: 11s ≥ 10s → everything naturally due.
	due = d.Tick(start.Add(11 * time.Second))
	assertSameSet(t, due, ConfigurableSubsystems())
}

// TestScanDispatcher_UpdateIntervals_PickerWalkBackToUseGlobal:
// regression guard for the v0.9.9-rc2 UAT finding. User clicked
// through a picker's preset dropdown (SMART: 1h → 2h → … → 7d →
// back to "Use global") while global was 7 days. Previously, every
// intermediate UpdateIntervals call deleted SMART's lastRun, so the
// FINAL state had lastRun=zero and SMART fired on the next tick
// despite the user being back on "Use global=7d". The fix: preserve
// lastRun across UpdateIntervals. This test walks the same path and
// asserts SMART is NOT due when the user settles back on "Use
// global" 20 minutes after the original SMART run.
func TestScanDispatcher_UpdateIntervals_PickerWalkBackToUseGlobal(t *testing.T) {
	start := time.Date(2026, 4, 24, 10, 45, 46, 0, time.UTC)
	global := 7 * 24 * time.Hour // 7 days — user's UAT global
	d, _ := newFixedClockDispatcher(DispatcherIntervalsConfig{
		// all zero = use global = 7d
	}, global, start)

	// All subsystems run at startup (the dispatcher's
	// lr.IsZero() → due path fires them, scheduler calls MarkRan).
	for _, name := range ConfigurableSubsystems() {
		d.MarkRan(name, start)
	}

	// User opens Settings, starts clicking through the SMART picker
	// dropdown. Each click fires saveSettings → UpdateIntervals.
	walk := []int{3600, 7200, 21600, 43200, 86400, 604800, 0}
	// 1h → 2h → 6h → 12h → 24h → 7d → back to "Use global" (0)
	for _, sec := range walk {
		d.UpdateIntervals(DispatcherIntervalsConfig{SMARTSec: sec}, global)
	}

	// 20 minutes after the original SMART run, user settles. Global
	// is 7d, SMART is back on use-global. Elapsed 20m << 7d → SMART
	// must NOT be due.
	due := d.Tick(start.Add(20 * time.Minute))
	for _, name := range due {
		if name == "smart" {
			t.Errorf("smart must NOT be due 20m after last run with 7d global; got %v", due)
		}
	}

	// At the same tick, no other subsystem should be due either —
	// they all use global=7d.
	if len(due) != 0 {
		t.Errorf("no subsystem should be due 20m after last run with 7d global; got %v", due)
	}
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
