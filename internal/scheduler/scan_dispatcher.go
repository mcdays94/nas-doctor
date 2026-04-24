// Package scheduler — scan_dispatcher.go implements the per-subsystem
// scan dispatcher introduced in issue #260 (PRD #239 slice 2b).
//
// Design (agreed in the Apr 2026 grilling session recorded on #239):
//
//   - The dispatcher is a deep module that owns "what runs when"
//     decisions for the six configurable subsystems: SMART, Docker,
//     Proxmox, Kubernetes, ZFS, GPU.
//   - Each subsystem has an IntervalSec value from
//     Settings.AdvancedScans. 0 means "use global" — the effective
//     interval falls back to the scheduler's global scan_interval.
//   - The scheduler sizes its main ticker at FastestInterval(), which
//     is min(global, all non-zero per-subsystem intervals). Each tick
//     the scheduler calls Tick(now) to get the list of due subsystems
//     and invokes their matching Collect* methods on the Collector.
//   - The nine non-configurable subsystems (system, disks, network,
//     logs, parity, UPS, update check, tunnels, backup) always run
//     every tick via Collector.Collect() — they are not routed
//     through the dispatcher at all.
//   - `now` is injected via a func seam so unit tests can pin time.
//
// Field meaning:
//
//   - intervals[subsystem] is the EFFECTIVE interval (already
//     resolved: 0 from settings collapses to global at UpdateIntervals
//     time, so this map never contains zero).
//   - lastRun[subsystem] records the time MarkRan was called (zero
//     value means "never ran" — treated as always due on Tick).
package scheduler

import (
	"sort"
	"sync"
	"time"
)

// configurableSubsystems is the canonical list of subsystems the
// dispatcher manages. Enumerated in one place so dispatcher, scheduler
// integration code, and tests agree on ordering + membership.
//
// The nine non-configurable subsystems (system, disks, network, logs,
// parity, UPS, update check, tunnels, backup) are intentionally
// absent — they are hard-wired to the global scan interval via
// Collector.Collect() and are out of scope for the dispatcher.
var configurableSubsystems = []string{
	"smart",
	"docker",
	"proxmox",
	"kubernetes",
	"zfs",
	"gpu",
}

// DispatcherIntervalsConfig is the minimal shape ScanDispatcher needs
// to ingest a settings update. The api package's
// AdvancedScansSettings is converted to this shape before UpdateIntervals
// is called so the scheduler package doesn't have to import internal/api.
type DispatcherIntervalsConfig struct {
	// Per-subsystem interval overrides, in seconds. 0 means "use
	// global" — the dispatcher substitutes the global scan interval
	// when resolving the effective cadence.
	SMARTSec      int
	DockerSec     int
	ProxmoxSec    int
	KubernetesSec int
	ZFSSec        int
	GPUSec        int
}

// ScanDispatcher owns per-subsystem scheduling decisions. It is safe
// for concurrent use — all state is guarded by a single mutex, and
// methods are short enough that contention is not a concern.
type ScanDispatcher struct {
	mu sync.Mutex
	// now is the time seam. Tests pin it to a fake clock.
	now func() time.Time
	// intervals are the EFFECTIVE per-subsystem intervals, already
	// resolved against global (so "0 = use global" has been replaced
	// with the global interval at config-update time). Missing key
	// means "never configured" and is treated as "use global" on the
	// fly — but in practice the map is always fully populated after
	// NewScanDispatcher or UpdateIntervals.
	intervals map[string]time.Duration
	// lastRun records the timestamp of the most recent MarkRan for
	// each subsystem. Zero value means "never ran" and is treated as
	// always due on the next Tick.
	lastRun map[string]time.Time
	// global is the scan_interval fallback for subsystems whose
	// IntervalSec is 0.
	global time.Duration
}

// NewScanDispatcher builds a dispatcher with the given per-subsystem
// config and global fallback. nowFn may be nil in which case
// time.Now is used. Passing a non-nil nowFn is the seam tests use to
// simulate the passage of time.
func NewScanDispatcher(cfg DispatcherIntervalsConfig, global time.Duration, nowFn func() time.Time) *ScanDispatcher {
	if nowFn == nil {
		nowFn = time.Now
	}
	d := &ScanDispatcher{
		now:       nowFn,
		intervals: make(map[string]time.Duration, len(configurableSubsystems)),
		lastRun:   make(map[string]time.Time, len(configurableSubsystems)),
		global:    global,
	}
	d.applyIntervalsLocked(cfg, global)
	return d
}

// applyIntervalsLocked resolves per-subsystem intervals against the
// global fallback and writes them into d.intervals. Caller must hold
// d.mu (or must not yet have published d to other goroutines).
func (d *ScanDispatcher) applyIntervalsLocked(cfg DispatcherIntervalsConfig, global time.Duration) {
	if global <= 0 {
		global = 30 * time.Minute // defensive — matches default scan interval
	}
	byName := map[string]int{
		"smart":      cfg.SMARTSec,
		"docker":     cfg.DockerSec,
		"proxmox":    cfg.ProxmoxSec,
		"kubernetes": cfg.KubernetesSec,
		"zfs":        cfg.ZFSSec,
		"gpu":        cfg.GPUSec,
	}
	for _, name := range configurableSubsystems {
		sec := byName[name]
		if sec <= 0 {
			d.intervals[name] = global
			continue
		}
		d.intervals[name] = time.Duration(sec) * time.Second
	}
	d.global = global
}

// Tick returns the names of subsystems whose effective interval has
// elapsed since their last MarkRan. Returned in canonical
// configurableSubsystems order for stable log output.
//
// A subsystem with zero-value lastRun (never ran) is always returned.
// This is intentional: on first tick after startup every subsystem
// fires, which matches the pre-slice-2 "run everything on first tick"
// behaviour.
func (d *ScanDispatcher) Tick(now time.Time) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	due := make([]string, 0, len(configurableSubsystems))
	for _, name := range configurableSubsystems {
		interval := d.intervals[name]
		if interval <= 0 {
			interval = d.global
		}
		lr := d.lastRun[name]
		if lr.IsZero() || now.Sub(lr) >= interval {
			due = append(due, name)
		}
	}
	return due
}

// MarkRan records the given time as the latest run for the named
// subsystem. Names outside configurableSubsystems are silently
// ignored so callers don't need to pre-validate — passing "system"
// or "tunnels" here is a no-op rather than a panic.
func (d *ScanDispatcher) MarkRan(subsystem string, when time.Time) {
	if !isConfigurable(subsystem) {
		return
	}
	d.mu.Lock()
	d.lastRun[subsystem] = when
	d.mu.Unlock()
}

// LastRunMap returns a copy of the per-subsystem lastRun timestamps.
// Safe to hand to the API layer for JSON serialization. Zero-value
// timestamps are omitted — they represent "never ran" and serialize
// better as a missing key than as "0001-01-01T00:00:00Z". Returned
// in configurableSubsystems order as a map[string]time.Time; callers
// that need deterministic JSON ordering should iterate the
// configurableSubsystems slice themselves.
func (d *ScanDispatcher) LastRunMap() map[string]time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]time.Time, len(d.lastRun))
	for name, ts := range d.lastRun {
		if ts.IsZero() {
			continue
		}
		out[name] = ts
	}
	return out
}

// FastestInterval returns min(global, all effective per-subsystem
// intervals). Used by the scheduler at startup to size its ticker.
// Guaranteed to be positive and at least 1 second (defensive clamp).
func (d *ScanDispatcher) FastestInterval() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	fastest := d.global
	for _, name := range configurableSubsystems {
		interval := d.intervals[name]
		if interval <= 0 {
			continue
		}
		if interval < fastest {
			fastest = interval
		}
	}
	if fastest <= 0 {
		fastest = 30 * time.Minute
	}
	if fastest < time.Second {
		fastest = time.Second
	}
	return fastest
}

// UpdateIntervals applies a new settings configuration. Subsystems
// whose EFFECTIVE interval changed are treated as "never ran" so the
// new cadence takes effect on the next tick (user-lowers-interval
// story). Subsystems whose interval is unchanged keep their lastRun
// state so we don't spuriously re-run them.
//
// Called from the settings-save path in the API handler. Safe to call
// concurrently with Tick / MarkRan.
func (d *ScanDispatcher) UpdateIntervals(cfg DispatcherIntervalsConfig, global time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	prev := make(map[string]time.Duration, len(d.intervals))
	for k, v := range d.intervals {
		prev[k] = v
	}
	d.applyIntervalsLocked(cfg, global)
	for name, newInterval := range d.intervals {
		if prev[name] != newInterval {
			delete(d.lastRun, name)
		}
	}
}

// Skipped returns the subsystems in configurableSubsystems that are
// NOT in the `due` list. Used for the per-tick INFO log summary. The
// input `due` slice does not need to be sorted; the returned slice is
// in canonical configurableSubsystems order.
func Skipped(due []string) []string {
	inDue := make(map[string]struct{}, len(due))
	for _, name := range due {
		inDue[name] = struct{}{}
	}
	out := make([]string, 0, len(configurableSubsystems)-len(due))
	for _, name := range configurableSubsystems {
		if _, ok := inDue[name]; !ok {
			out = append(out, name)
		}
	}
	return out
}

// ConfigurableSubsystems returns a defensive copy of the canonical
// subsystem list. Exported for tests and the scheduler integration
// layer. Do not mutate the returned slice in shared state.
func ConfigurableSubsystems() []string {
	out := make([]string, len(configurableSubsystems))
	copy(out, configurableSubsystems)
	return out
}

func isConfigurable(name string) bool {
	for _, s := range configurableSubsystems {
		if s == name {
			return true
		}
	}
	return false
}

// sortedDue is a helper to produce deterministic ordering when the
// caller already has a slice but doesn't care about
// configurableSubsystems order. Not currently used inline — the
// dispatcher returns results in canonical order — but kept for future
// callers (and imported by tests asserting log format).
//
//nolint:unused // reserved for future use
func sortedDue(due []string) []string {
	out := append([]string(nil), due...)
	sort.Strings(out)
	return out
}
