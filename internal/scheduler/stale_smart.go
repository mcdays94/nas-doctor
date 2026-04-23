// Package scheduler — stale_smart.go implements the max-age SMART
// force-wake checker introduced in issue #238 (PRD #236 slice 1b).
//
// Design (agreed in the Apr 2026 grilling session):
//   - The collector is DB-unaware. It simply reports which drives were
//     in standby this cycle (snap.SMARTStandbyDevices) and otherwise
//     behaves identically to the v0.9.5 `-n standby` flow.
//   - The scheduler owns the max-age policy. After a normal scan,
//     StaleSMARTChecker.Check queries smart_history for each standby
//     device and returns the list whose last read has exceeded
//     Settings.SMART.MaxAgeDays. StaleSMARTChecker.Apply then invokes
//     the forced collector for that list and merges the fresh SMART
//     rows into the snapshot.
//   - MaxAgeDays==0 disables the feature entirely — no store queries,
//     no force-wake invocations. Preserves exact v0.9.5 behaviour.
package scheduler

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// ForcedSMARTCollectorFn is the seam StaleSMARTChecker.Apply invokes
// to force-read SMART for a list of devices. In production the
// scheduler wires this to collector.Collector.CollectSMARTForced; in
// tests it's a stub that returns the desired shape.
type ForcedSMARTCollectorFn func(devices []string) ([]internal.SMARTInfo, error)

// staleSMARTStore is the narrow dependency StaleSMARTChecker needs
// from the storage layer. Declared as its own interface here (rather
// than reusing storage.HistoryStore) so tests can inject a tiny mock
// without implementing the 20+ methods of the broader interface.
type staleSMARTStore interface {
	GetLastSMARTCollectedAt(device string) (time.Time, bool, error)
}

// StaleSMARTChecker is the deep module that implements the max-age
// force-wake safety net (issue #238 / PRD #236).
//
// Check() is a pure policy decision — which of the standby drives in
// the current snapshot have been unread for longer than MaxAgeDays?
//
// Apply() performs the side effect — invokes the forced collector for
// the returned device list and merges the fresh results into the
// snapshot. These are intentionally separate so the scheduler can
// short-circuit the callback wiring when Check returns empty.
type StaleSMARTChecker struct {
	store      staleSMARTStore
	maxAgeDays int
	logger     *slog.Logger
}

// NewStaleSMARTChecker constructs the checker. maxAgeDays is the
// number of days a drive may remain unread before force-wake; 0
// disables the feature entirely.
func NewStaleSMARTChecker(store staleSMARTStore, maxAgeDays int, logger *slog.Logger) *StaleSMARTChecker {
	return &StaleSMARTChecker{
		store:      store,
		maxAgeDays: maxAgeDays,
		logger:     logger,
	}
}

// Check returns the subset of snapshot.SMARTStandbyDevices whose last
// successful SMART read is older than MaxAgeDays.
//
// Returns empty (no store queries) when:
//   - snapshot is nil
//   - MaxAgeDays == 0 (feature disabled)
//   - snapshot.SMARTStandbyDevices is empty
//
// Drives with no smart_history row are SKIPPED (PRD #236 user story 7:
// new drives must not be force-woken). Drives whose store lookup
// errors are SKIPPED and logged at WARN so a DB blip doesn't cause a
// spurious force-wake.
func (c *StaleSMARTChecker) Check(snap *internal.Snapshot) []string {
	if snap == nil {
		return nil
	}
	if c.maxAgeDays <= 0 {
		return nil
	}
	if len(snap.SMARTStandbyDevices) == 0 {
		return nil
	}

	now := snap.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}
	threshold := time.Duration(c.maxAgeDays) * 24 * time.Hour

	var stale []string
	for _, dev := range snap.SMARTStandbyDevices {
		lastAt, found, err := c.store.GetLastSMARTCollectedAt(dev)
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("stale-SMART lookup failed; skipping device",
					"device", dev,
					"error", err,
				)
			}
			continue
		}
		if !found {
			// New drive — no history yet. Do not force-wake.
			continue
		}
		age := now.Sub(lastAt)
		if age > threshold {
			stale = append(stale, dev)
		}
	}
	return stale
}

// Apply invokes the forced-collector callback for the given devices
// and merges the fresh SMART results into the snapshot. For each
// device the caller asked to wake, the canonical INFO log line is
// emitted BEFORE the callback runs:
//
//	forcing SMART wake on <device>: last read <duration> ago exceeds max_age_days=<N>
//
// Merge semantics:
//   - Any SMARTInfo the callback returns is appended to snap.SMART
//     and the corresponding device is removed from
//     snap.SMARTStandbyDevices (the drive is no longer asleep).
//   - A callback error is logged at ERROR but does NOT abort. Devices
//     whose force-read failed remain in snap.SMARTStandbyDevices
//     (they're still considered asleep; we just don't have fresh data).
//   - A nil callback is a no-op (defensive guard — the scheduler
//     always wires a real callback in production, but a mis-wire
//     shouldn't panic).
func (c *StaleSMARTChecker) Apply(snap *internal.Snapshot, devices []string, fn ForcedSMARTCollectorFn) {
	if snap == nil || len(devices) == 0 || fn == nil {
		return
	}

	now := snap.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Emit the canonical per-device INFO log before the callback so
	// operators can see intent even if the force-read itself fails.
	for _, dev := range devices {
		lastAt, found, err := c.store.GetLastSMARTCollectedAt(dev)
		if err != nil || !found {
			// Shouldn't happen (Check already filtered these out) but
			// be defensive.
			if c.logger != nil {
				c.logger.Info(fmt.Sprintf("forcing SMART wake on %s: last read unknown ago exceeds max_age_days=%d", dev, c.maxAgeDays))
			}
			continue
		}
		age := now.Sub(lastAt)
		if c.logger != nil {
			c.logger.Info(fmt.Sprintf(
				"forcing SMART wake on %s: last read %s ago exceeds max_age_days=%d",
				dev,
				age.Round(time.Minute).String(),
				c.maxAgeDays,
			))
		}
	}

	results, err := fn(devices)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("force-wake SMART collector returned error (partial results may still be merged)",
				"error", err,
				"devices", devices,
			)
		}
	}

	if len(results) == 0 {
		return
	}

	// Merge fresh entries into snap.SMART. Replace existing entries
	// for the same device (defensive — snap.SMART shouldn't contain
	// standby devices, but if a future change adds a "standby marker"
	// entry we still want to overwrite cleanly).
	freshByDevice := make(map[string]internal.SMARTInfo, len(results))
	for _, r := range results {
		freshByDevice[r.Device] = r
	}
	for i, existing := range snap.SMART {
		if fresh, ok := freshByDevice[existing.Device]; ok {
			snap.SMART[i] = fresh
			delete(freshByDevice, existing.Device)
		}
	}
	for _, fresh := range freshByDevice {
		snap.SMART = append(snap.SMART, fresh)
	}

	// Remove force-woken devices from the standby list.
	wokenSet := make(map[string]struct{}, len(results))
	for _, r := range results {
		wokenSet[r.Device] = struct{}{}
	}
	filtered := snap.SMARTStandbyDevices[:0]
	for _, dev := range snap.SMARTStandbyDevices {
		if _, woken := wokenSet[dev]; !woken {
			filtered = append(filtered, dev)
		}
	}
	snap.SMARTStandbyDevices = filtered
}
