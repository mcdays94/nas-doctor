package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// resolveSlotKey returns the stable identifier for a drive. On Unraid the
// ArraySlot is preferred because it persists across drive replacements
// within the same bay (e.g., "disk1" maps to whatever physical drive the
// user has pushed into that slot). On other platforms the serial number
// is used; this is less stable (it changes every time the drive does)
// but it's the best identifier available.
//
// Returns the empty string when neither is populated — callers must skip
// events for empty slot keys.
func resolveSlotKey(d internal.SMARTInfo) string {
	if d.ArraySlot != "" {
		return d.ArraySlot
	}
	return d.Serial
}

// detectDriveReplacements compares the current SMART scan against the
// last-observed per-slot state and emits a "replacement" drive_event when
// a slot's serial number has changed. Also UPSERTs the per-slot state so
// the next scan has fresh baseline data.
//
// Behaviour contract (issue #130):
//   - Only runs on Unraid. Other platforms lack stable slot identity
//     (slot_key == serial), so detection would be meaningless.
//   - First-ever observation of a slot (no prior state row) establishes
//     baseline and emits no event. This avoids a flood of spurious
//     "replacement" events on fresh install.
//   - If a slot was populated in the prior scan but is absent from the
//     current scan (drive physically pulled), no event is emitted — we
//     wait for the replacement to arrive. State is left unchanged.
//   - If a slot's serial differs from the stored value, one event is
//     emitted and the state row is updated.
//
// `platform` is stamped onto new drive_events rows for filtering in
// cross-platform deployments.
func detectDriveReplacements(store storage.DriveEventStore, platform string, scan []internal.SMARTInfo, at time.Time) error {
	// Phase 2 only supports Unraid — see doc comment above.
	if platform != "unraid" {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	for _, d := range scan {
		slotKey := resolveSlotKey(d)
		if slotKey == "" || d.Serial == "" {
			// Can't track a slot we can't identify. Skip.
			continue
		}
		// Only track ArraySlot-based keys for detection. If the drive
		// has no array slot (e.g., a standalone drive on Unraid that
		// isn't array-assigned) we can't meaningfully compare against
		// "this slot's previous occupant".
		if d.ArraySlot == "" {
			continue
		}

		prev, err := store.GetDriveSlotState(slotKey)
		if err != nil {
			return fmt.Errorf("load slot state for %s: %w", slotKey, err)
		}

		if prev == nil {
			// First observation — establish baseline, no event.
			if err := store.SaveDriveSlotState(storage.DriveSlotState{
				SlotKey:    slotKey,
				Serial:     d.Serial,
				Model:      d.Model,
				Platform:   platform,
				ObservedAt: at,
			}); err != nil {
				return fmt.Errorf("save slot state for %s: %w", slotKey, err)
			}
			continue
		}

		if prev.Serial == d.Serial {
			// Same drive — just freshen observed_at.
			if err := store.SaveDriveSlotState(storage.DriveSlotState{
				SlotKey:    slotKey,
				Serial:     d.Serial,
				Model:      d.Model,
				Platform:   platform,
				ObservedAt: at,
			}); err != nil {
				return fmt.Errorf("refresh slot state for %s: %w", slotKey, err)
			}
			continue
		}

		// Serial changed → emit replacement event.
		payload := map[string]string{
			"old_serial": prev.Serial,
			"old_model":  prev.Model,
			"new_serial": d.Serial,
			"new_model":  d.Model,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal replacement payload: %w", err)
		}
		if _, err := store.SaveDriveEvent(storage.DriveEvent{
			SlotKey:   slotKey,
			Platform:  platform,
			EventType: "replacement",
			EventTime: at,
			Content:   string(raw),
			IsAuto:    true,
		}); err != nil {
			return fmt.Errorf("save replacement event for %s: %w", slotKey, err)
		}
		// Update baseline to the new serial so the next scan sees a
		// steady state rather than repeatedly emitting the same event.
		if err := store.SaveDriveSlotState(storage.DriveSlotState{
			SlotKey:    slotKey,
			Serial:     d.Serial,
			Model:      d.Model,
			Platform:   platform,
			ObservedAt: at,
		}); err != nil {
			return fmt.Errorf("update slot state after replacement for %s: %w", slotKey, err)
		}
	}

	// Note: if a slot was present previously and is now absent from the
	// scan, we intentionally do NOT touch its state row. The user pulled
	// the drive; when a replacement arrives, the next scan will see a
	// serial mismatch and emit the event at the correct moment.
	return nil
}
