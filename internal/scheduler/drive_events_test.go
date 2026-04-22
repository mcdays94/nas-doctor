package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestDetectDriveReplacements_DetectsReplacement_OnSerialChange simulates
// two consecutive SMART scans where slot "disk1" changes serial number
// between them. The second scan should produce exactly one "replacement"
// drive_event row with the old and new serial/model in its content JSON.
//
// Core detection contract for issue #130.
func TestDetectDriveReplacements_DetectsReplacement_OnSerialChange(t *testing.T) {
	store := storage.NewFakeStore()

	// First scan — fresh install, no prior state exists.
	scan1 := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "OLDSERIAL", Model: "OLDMODEL", ArraySlot: "disk1"},
	}
	if err := detectDriveReplacements(store, "unraid", scan1, time.Now().UTC()); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	// No events should have been emitted on fresh install.
	events, err := store.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after fresh-install scan, got %d", len(events))
	}

	// Second scan — same slot, DIFFERENT serial → replacement.
	replacedAt := time.Now().UTC().Add(time.Hour)
	scan2 := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "NEWSERIAL", Model: "NEWMODEL", ArraySlot: "disk1"},
	}
	if err := detectDriveReplacements(store, "unraid", scan2, replacedAt); err != nil {
		t.Fatalf("second scan: %v", err)
	}

	events, err = store.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 replacement event, got %d: %+v", len(events), events)
	}
	ev := events[0]
	if ev.EventType != "replacement" {
		t.Errorf("event_type = %q, want replacement", ev.EventType)
	}
	if !ev.IsAuto {
		t.Errorf("replacement event is_auto = false, want true")
	}
	if ev.SlotKey != "disk1" {
		t.Errorf("slot_key = %q, want disk1", ev.SlotKey)
	}
	if !ev.EventTime.Equal(replacedAt) {
		t.Errorf("event_time = %v, want %v", ev.EventTime, replacedAt)
	}

	var content struct {
		OldSerial string `json:"old_serial"`
		NewSerial string `json:"new_serial"`
		OldModel  string `json:"old_model"`
		NewModel  string `json:"new_model"`
	}
	if err := json.Unmarshal([]byte(ev.Content), &content); err != nil {
		t.Fatalf("parse content JSON %q: %v", ev.Content, err)
	}
	if content.OldSerial != "OLDSERIAL" {
		t.Errorf("old_serial = %q, want OLDSERIAL", content.OldSerial)
	}
	if content.NewSerial != "NEWSERIAL" {
		t.Errorf("new_serial = %q, want NEWSERIAL", content.NewSerial)
	}
	if content.OldModel != "OLDMODEL" {
		t.Errorf("old_model = %q, want OLDMODEL", content.OldModel)
	}
	if content.NewModel != "NEWMODEL" {
		t.Errorf("new_model = %q, want NEWMODEL", content.NewModel)
	}
}

// TestDetectDriveReplacements_NoReplacementEventOnFirstScan — the very
// first scan establishes baseline state but must not emit any events.
func TestDetectDriveReplacements_NoReplacementEventOnFirstScan(t *testing.T) {
	store := storage.NewFakeStore()
	scan := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "S1", Model: "M1", ArraySlot: "disk1"},
		{Device: "/dev/sdc", Serial: "S2", Model: "M2", ArraySlot: "parity"},
	}
	if err := detectDriveReplacements(store, "unraid", scan, time.Now().UTC()); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	for _, slot := range []string{"disk1", "parity"} {
		events, err := store.ListDriveEvents(slot)
		if err != nil {
			t.Fatalf("ListDriveEvents %s: %v", slot, err)
		}
		if len(events) != 0 {
			t.Errorf("slot %s: expected 0 events, got %d", slot, len(events))
		}
	}
}

// TestDetectDriveReplacements_NoReplacementEventWhenSlotGoesEmpty — if a
// slot had a drive in the previous scan and is now missing, do NOT emit
// an event; wait for the replacement drive to show up.
func TestDetectDriveReplacements_NoReplacementEventWhenSlotGoesEmpty(t *testing.T) {
	store := storage.NewFakeStore()
	scan1 := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "S1", Model: "M1", ArraySlot: "disk1"},
	}
	if err := detectDriveReplacements(store, "unraid", scan1, time.Now().UTC()); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	// Second scan — slot is missing (drive pulled out, not yet replaced).
	scan2 := []internal.SMARTInfo{}
	if err := detectDriveReplacements(store, "unraid", scan2, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	events, err := store.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events when slot goes empty, got %d", len(events))
	}

	// Third scan — same slot, same serial as before. This should NOT
	// trigger an event either (the stored state still matches).
	scan3 := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "S1", Model: "M1", ArraySlot: "disk1"},
	}
	if err := detectDriveReplacements(store, "unraid", scan3, time.Now().UTC().Add(2*time.Hour)); err != nil {
		t.Fatalf("third scan: %v", err)
	}
	events, err = store.ListDriveEvents("disk1")
	if err != nil {
		t.Fatalf("ListDriveEvents after reinsert: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events when drive returns with same serial, got %d", len(events))
	}
}

// TestDetectDriveReplacements_SkipsNonUnraid — on platforms without
// stable slot identity, skip detection entirely to avoid false positives.
// (Serial-as-slot-key can't meaningfully detect "drive replaced in slot"
// because the slot_key IS the serial.)
func TestDetectDriveReplacements_SkipsNonUnraid(t *testing.T) {
	store := storage.NewFakeStore()
	scan := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "S1", Model: "M1", ArraySlot: ""},
	}
	// First scan on a non-Unraid platform — no state should be written.
	if err := detectDriveReplacements(store, "synology", scan, time.Now().UTC()); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	// Serial change wouldn't make sense here either; verify no state row.
	if state, _ := store.GetDriveSlotState("S1"); state != nil {
		t.Errorf("non-Unraid platform unexpectedly wrote drive_slot_state for serial %q", state.SlotKey)
	}
	// No events regardless.
	if evs, _ := store.ListDriveEvents("S1"); len(evs) != 0 {
		t.Errorf("expected 0 events on non-Unraid platform, got %d", len(evs))
	}
}

// TestDetectDriveReplacements_MultipleSlotsIndependent ensures events
// for one slot don't leak into another, and that one replacement doesn't
// block another slot's detection in the same scan.
func TestDetectDriveReplacements_MultipleSlotsIndependent(t *testing.T) {
	store := storage.NewFakeStore()
	// Seed state for two slots.
	scan1 := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "S1", Model: "M1", ArraySlot: "disk1"},
		{Device: "/dev/sdc", Serial: "S2", Model: "M2", ArraySlot: "disk2"},
	}
	if err := detectDriveReplacements(store, "unraid", scan1, time.Now().UTC()); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	// Replace both.
	scan2 := []internal.SMARTInfo{
		{Device: "/dev/sdb", Serial: "S1-NEW", Model: "M1", ArraySlot: "disk1"},
		{Device: "/dev/sdc", Serial: "S2-NEW", Model: "M2", ArraySlot: "disk2"},
	}
	if err := detectDriveReplacements(store, "unraid", scan2, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	for _, slot := range []string{"disk1", "disk2"} {
		evs, err := store.ListDriveEvents(slot)
		if err != nil {
			t.Fatalf("ListDriveEvents %s: %v", slot, err)
		}
		if len(evs) != 1 {
			t.Errorf("slot %s: expected 1 replacement event, got %d", slot, len(evs))
		}
	}
}
