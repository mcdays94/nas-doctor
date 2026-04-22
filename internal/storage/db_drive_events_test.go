package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStore_DriveEvents_SaveAndList verifies that a manual note can be
// saved and read back by slot_key, ordered newest-first. This is the
// minimum contract for issue #130 Phase 1.
func TestStore_DriveEvents_SaveAndList(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	older := now.Add(-2 * time.Hour)

	id1, err := db.SaveDriveEvent(DriveEvent{
		SlotKey:   "disk1",
		Platform:  "unraid",
		EventType: "note",
		EventTime: older,
		Content:   "SATA cable replaced",
		IsAuto:    false,
	})
	if err != nil {
		t.Fatalf("SaveDriveEvent older: %v", err)
	}
	if id1 <= 0 {
		t.Errorf("expected positive id, got %d", id1)
	}

	id2, err := db.SaveDriveEvent(DriveEvent{
		SlotKey:   "disk1",
		Platform:  "unraid",
		EventType: "note",
		EventTime: now,
		Content:   "Second note",
		IsAuto:    false,
	})
	if err != nil {
		t.Fatalf("SaveDriveEvent newer: %v", err)
	}
	if id2 == id1 || id2 <= 0 {
		t.Errorf("expected distinct positive id, got %d vs %d", id2, id1)
	}

	// An event on a different slot must not come back.
	if _, err := db.SaveDriveEvent(DriveEvent{
		SlotKey:   "disk2",
		Platform:  "unraid",
		EventType: "note",
		EventTime: now,
		Content:   "Other slot",
		IsAuto:    false,
	}); err != nil {
		t.Fatalf("SaveDriveEvent other slot: %v", err)
	}

	events, err := db.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events for disk1, got %d", len(events))
	}
	// newest first
	if !events[0].EventTime.After(events[1].EventTime) {
		t.Errorf("events not ordered newest-first: %v vs %v", events[0].EventTime, events[1].EventTime)
	}
	if events[0].Content != "Second note" {
		t.Errorf("first event content = %q, want Second note", events[0].Content)
	}
	if events[1].Content != "SATA cable replaced" {
		t.Errorf("second event content = %q, want SATA cable replaced", events[1].Content)
	}
	for _, ev := range events {
		if ev.SlotKey != "disk1" {
			t.Errorf("event leaked from other slot: slot_key=%q", ev.SlotKey)
		}
		if ev.IsAuto {
			t.Errorf("note event unexpectedly is_auto=true")
		}
		if ev.CreatedAt.IsZero() {
			t.Errorf("created_at not populated")
		}
	}
}

// TestStore_DriveEvents_UpdateAndDelete_ManualOnly verifies that
// is_auto=1 events are immutable: both UpdateDriveEvent and
// DeleteDriveEvent return a "forbidden" error for auto events.
func TestStore_DriveEvents_UpdateAndDelete_ManualOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()

	manualID, err := db.SaveDriveEvent(DriveEvent{
		SlotKey:   "disk1",
		Platform:  "unraid",
		EventType: "note",
		EventTime: now,
		Content:   "initial",
		IsAuto:    false,
	})
	if err != nil {
		t.Fatalf("save manual: %v", err)
	}
	autoID, err := db.SaveDriveEvent(DriveEvent{
		SlotKey:   "disk1",
		Platform:  "unraid",
		EventType: "replacement",
		EventTime: now,
		Content:   `{"old_serial":"OLD","new_serial":"NEW"}`,
		IsAuto:    true,
	})
	if err != nil {
		t.Fatalf("save auto: %v", err)
	}

	// Manual event: Update allowed.
	newTime := now.Add(-1 * time.Hour)
	newContent := "updated"
	if err := db.UpdateDriveEvent("disk1", manualID, &newTime, &newContent); err != nil {
		t.Fatalf("UpdateDriveEvent manual: %v", err)
	}

	events, err := db.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	var found *DriveEvent
	for i := range events {
		if events[i].ID == manualID {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("manual event %d not found after update", manualID)
	}
	if found.Content != "updated" {
		t.Errorf("update did not persist content: got %q", found.Content)
	}
	if found.UpdatedAt == nil {
		t.Errorf("updated_at not populated after manual update")
	}

	// Auto event: Update rejected.
	contentChange := "should fail"
	err = db.UpdateDriveEvent("disk1", autoID, nil, &contentChange)
	if err == nil {
		t.Errorf("expected error when updating auto event, got nil")
	} else if !IsDriveEventImmutableErr(err) {
		t.Errorf("expected immutable-error sentinel, got %v", err)
	}

	// Auto event: Delete rejected.
	err = db.DeleteDriveEvent("disk1", autoID)
	if err == nil {
		t.Errorf("expected error when deleting auto event, got nil")
	} else if !IsDriveEventImmutableErr(err) {
		t.Errorf("expected immutable-error sentinel, got %v", err)
	}

	// Manual event: Delete allowed.
	if err := db.DeleteDriveEvent("disk1", manualID); err != nil {
		t.Fatalf("DeleteDriveEvent manual: %v", err)
	}
	events, err = db.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents after delete: %v", err)
	}
	for _, ev := range events {
		if ev.ID == manualID {
			t.Errorf("manual event not deleted")
		}
	}
	// Auto event still there.
	found = nil
	for i := range events {
		if events[i].ID == autoID {
			found = &events[i]
		}
	}
	if found == nil {
		t.Errorf("auto event gone after failed delete attempt — should still exist")
	}
}

// TestStore_DriveEvents_MissingSlotOrID returns zero events / clear errors
// for bogus lookups.
func TestStore_DriveEvents_MissingSlotOrID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	events, err := db.ListDriveEvents("nonexistent")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty, got %d events", len(events))
	}

	// Update/Delete on missing ID surfaces a not-found error.
	content := "x"
	err = db.UpdateDriveEvent("nope", 9999, nil, &content)
	if err == nil {
		t.Errorf("expected error updating nonexistent event")
	}
	err = db.DeleteDriveEvent("nope", 9999)
	if err == nil {
		t.Errorf("expected error deleting nonexistent event")
	}
}
