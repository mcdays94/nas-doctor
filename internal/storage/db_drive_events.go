package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DriveEvent is a single entry in the per-slot maintenance log for a drive.
//
// Two event types are supported:
//
//   - "note"        — user-entered freeform text. Content is plain string.
//     IsAuto=false, mutable via Update/Delete.
//   - "replacement" — system-detected when a slot's serial number changes
//     between SMART scans. Content is a JSON blob containing
//     the old/new serial and model. IsAuto=true, immutable.
//
// SlotKey semantics (issue #130):
//   - On Unraid: ArraySlot (e.g., "disk1", "parity", "cache"), which is stable
//     across drive replacements.
//   - On non-Unraid platforms: the drive serial number. Less stable but the
//     best identifier available where the OS doesn't map physical bays.
type DriveEvent struct {
	ID        int64      `json:"id"`
	SlotKey   string     `json:"slot_key"`
	Platform  string     `json:"platform"`
	EventType string     `json:"event_type"`
	EventTime time.Time  `json:"event_time"`
	Content   string     `json:"content"`
	IsAuto    bool       `json:"is_auto"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// DriveEventImmutableError is returned by UpdateDriveEvent / DeleteDriveEvent
// when the target row has is_auto=1. API callers translate this to HTTP 403.
type DriveEventImmutableError struct {
	ID int64
}

func (e *DriveEventImmutableError) Error() string {
	return fmt.Sprintf("drive event %d is system-generated and cannot be modified", e.ID)
}

// IsDriveEventImmutableErr reports whether err is a DriveEventImmutableError.
func IsDriveEventImmutableErr(err error) bool {
	var e *DriveEventImmutableError
	return errors.As(err, &e)
}

// DriveEventNotFoundError is returned when an Update/Delete targets an ID
// that does not exist (or belongs to a different slot_key).
type DriveEventNotFoundError struct {
	SlotKey string
	ID      int64
}

func (e *DriveEventNotFoundError) Error() string {
	return fmt.Sprintf("drive event %d not found for slot %q", e.ID, e.SlotKey)
}

// IsDriveEventNotFoundErr reports whether err is a DriveEventNotFoundError.
func IsDriveEventNotFoundErr(err error) bool {
	var e *DriveEventNotFoundError
	return errors.As(err, &e)
}

// SaveDriveEvent inserts a new row into drive_events and returns the new id.
// created_at is set to the current UTC time on insert.
func (d *DB) SaveDriveEvent(ev DriveEvent) (int64, error) {
	if ev.SlotKey == "" {
		return 0, fmt.Errorf("slot_key is required")
	}
	if ev.EventType == "" {
		return 0, fmt.Errorf("event_type is required")
	}
	if ev.EventTime.IsZero() {
		ev.EventTime = time.Now().UTC()
	}
	res, err := d.db.Exec(
		`INSERT INTO drive_events (slot_key, platform, event_type, event_time, content, is_auto, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.SlotKey, ev.Platform, ev.EventType, ev.EventTime, ev.Content, boolToInt(ev.IsAuto), time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert drive_event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("drive_event last insert id: %w", err)
	}
	return id, nil
}

// ListDriveEvents returns every event for slotKey, newest first.
func (d *DB) ListDriveEvents(slotKey string) ([]DriveEvent, error) {
	rows, err := d.db.Query(
		`SELECT id, slot_key, platform, event_type, event_time, content, is_auto, created_at, updated_at
		 FROM drive_events
		 WHERE slot_key = ?
		 ORDER BY event_time DESC, id DESC`,
		slotKey,
	)
	if err != nil {
		return nil, fmt.Errorf("query drive_events: %w", err)
	}
	defer rows.Close()

	var out []DriveEvent
	for rows.Next() {
		var ev DriveEvent
		var isAuto int
		var updatedAt sql.NullTime
		if err := rows.Scan(&ev.ID, &ev.SlotKey, &ev.Platform, &ev.EventType, &ev.EventTime, &ev.Content, &isAuto, &ev.CreatedAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan drive_event: %w", err)
		}
		ev.IsAuto = isAuto != 0
		if updatedAt.Valid {
			t := updatedAt.Time
			ev.UpdatedAt = &t
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return out, nil
}

// UpdateDriveEvent modifies a manual (is_auto=0) drive event.
// If eventTime or content is nil the corresponding field is left unchanged.
// Auto events (is_auto=1) are immutable and cause DriveEventImmutableError.
// Missing IDs or mismatched slot_key cause DriveEventNotFoundError.
func (d *DB) UpdateDriveEvent(slotKey string, id int64, eventTime *time.Time, content *string) error {
	var (
		isAuto int
		stored string
	)
	err := d.db.QueryRow(
		`SELECT is_auto, slot_key FROM drive_events WHERE id = ?`,
		id,
	).Scan(&isAuto, &stored)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && stored != slotKey) {
		return &DriveEventNotFoundError{SlotKey: slotKey, ID: id}
	}
	if err != nil {
		return fmt.Errorf("lookup drive_event: %w", err)
	}
	if isAuto != 0 {
		return &DriveEventImmutableError{ID: id}
	}

	// Build an UPDATE that only changes the fields the caller provided.
	// Always bump updated_at when any change is applied.
	now := time.Now().UTC()
	switch {
	case eventTime != nil && content != nil:
		_, err = d.db.Exec(
			`UPDATE drive_events SET event_time = ?, content = ?, updated_at = ? WHERE id = ?`,
			*eventTime, *content, now, id,
		)
	case eventTime != nil:
		_, err = d.db.Exec(
			`UPDATE drive_events SET event_time = ?, updated_at = ? WHERE id = ?`,
			*eventTime, now, id,
		)
	case content != nil:
		_, err = d.db.Exec(
			`UPDATE drive_events SET content = ?, updated_at = ? WHERE id = ?`,
			*content, now, id,
		)
	default:
		// No-op — nothing to update.
		return nil
	}
	if err != nil {
		return fmt.Errorf("update drive_event: %w", err)
	}
	return nil
}

// DeleteDriveEvent removes a manual (is_auto=0) drive event.
// Auto events are immutable. Missing rows return DriveEventNotFoundError.
func (d *DB) DeleteDriveEvent(slotKey string, id int64) error {
	var (
		isAuto int
		stored string
	)
	err := d.db.QueryRow(
		`SELECT is_auto, slot_key FROM drive_events WHERE id = ?`,
		id,
	).Scan(&isAuto, &stored)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && stored != slotKey) {
		return &DriveEventNotFoundError{SlotKey: slotKey, ID: id}
	}
	if err != nil {
		return fmt.Errorf("lookup drive_event: %w", err)
	}
	if isAuto != 0 {
		return &DriveEventImmutableError{ID: id}
	}

	if _, err := d.db.Exec(`DELETE FROM drive_events WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete drive_event: %w", err)
	}
	return nil
}

// GetDriveEvent fetches a single event by id, or nil if not found.
func (d *DB) GetDriveEvent(id int64) (*DriveEvent, error) {
	var ev DriveEvent
	var isAuto int
	var updatedAt sql.NullTime
	err := d.db.QueryRow(
		`SELECT id, slot_key, platform, event_type, event_time, content, is_auto, created_at, updated_at
		 FROM drive_events WHERE id = ?`,
		id,
	).Scan(&ev.ID, &ev.SlotKey, &ev.Platform, &ev.EventType, &ev.EventTime, &ev.Content, &isAuto, &ev.CreatedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query drive_event: %w", err)
	}
	ev.IsAuto = isAuto != 0
	if updatedAt.Valid {
		t := updatedAt.Time
		ev.UpdatedAt = &t
	}
	return &ev, nil
}

// DriveSlotState records the last-observed serial+model for a given
// slot_key, used by the SMART collector to detect drive replacements
// across scans (issue #130).
type DriveSlotState struct {
	SlotKey    string
	Serial     string
	Model      string
	Platform   string
	ObservedAt time.Time
}

// GetDriveSlotState returns the last-known state for slotKey, or (nil, nil)
// if no state has been recorded yet (fresh install, or brand-new slot).
func (d *DB) GetDriveSlotState(slotKey string) (*DriveSlotState, error) {
	var state DriveSlotState
	err := d.db.QueryRow(
		`SELECT slot_key, serial, model, platform, observed_at
		 FROM drive_slot_state WHERE slot_key = ?`,
		slotKey,
	).Scan(&state.SlotKey, &state.Serial, &state.Model, &state.Platform, &state.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query drive_slot_state: %w", err)
	}
	return &state, nil
}

// SaveDriveSlotState UPSERTs the last-observed state for a slot.
func (d *DB) SaveDriveSlotState(state DriveSlotState) error {
	if state.SlotKey == "" {
		return fmt.Errorf("slot_key is required")
	}
	if state.ObservedAt.IsZero() {
		state.ObservedAt = time.Now().UTC()
	}
	_, err := d.db.Exec(
		`INSERT INTO drive_slot_state (slot_key, serial, model, platform, observed_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(slot_key) DO UPDATE SET
		   serial = excluded.serial,
		   model = excluded.model,
		   platform = excluded.platform,
		   observed_at = excluded.observed_at`,
		state.SlotKey, state.Serial, state.Model, state.Platform, state.ObservedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert drive_slot_state: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
