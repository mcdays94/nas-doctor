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
	"log/slog"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

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
