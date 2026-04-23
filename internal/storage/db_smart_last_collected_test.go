package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDB_GetLastSMARTCollectedAt_ReturnsLatestTimestamp verifies the
// scheduler-facing "when was this device last read" query. Used by the
// StaleSMARTChecker (issue #238) to decide whether to force-wake a drive
// that has been in standby longer than Settings.SMART.MaxAgeDays.
//
// Expected query: SELECT MAX(timestamp) FROM smart_history WHERE device = ?
// backed by idx_smart_history_device.
func TestDB_GetLastSMARTCollectedAt_ReturnsLatestTimestamp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Seed a snapshot the smart_history rows can reference.
	snapID := "snap-1"
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.db.Exec(
		`INSERT INTO snapshots (id, timestamp, duration_seconds, data) VALUES (?, ?, ?, ?)`,
		snapID, now, 0.1, "{}",
	); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	// Two rows for /dev/sda (8h and 2h old), one row for /dev/sdb (1h old).
	rows := []struct {
		device string
		age    time.Duration
	}{
		{"/dev/sda", 8 * time.Hour},
		{"/dev/sda", 2 * time.Hour},
		{"/dev/sdb", 1 * time.Hour},
	}
	for _, r := range rows {
		ts := now.Add(-r.age)
		if _, err := db.db.Exec(
			`INSERT INTO smart_history (snapshot_id, device, serial, model, temperature, reallocated, pending, udma_crc, command_timeout, power_on_hours, health_passed, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapID, r.device, "SER-"+r.device, "MODEL", 30, int64(0), int64(0), int64(0), int64(0), int64(1000), true, ts,
		); err != nil {
			t.Fatalf("insert smart_history: %v", err)
		}
	}

	// /dev/sda: latest is 2h old.
	got, found, err := db.GetLastSMARTCollectedAt("/dev/sda")
	if err != nil {
		t.Fatalf("GetLastSMARTCollectedAt(/dev/sda): %v", err)
	}
	if !found {
		t.Fatalf("expected found=true for /dev/sda")
	}
	wantSDA := now.Add(-2 * time.Hour)
	delta := got.Sub(wantSDA)
	if delta < -2*time.Second || delta > 2*time.Second {
		t.Errorf("/dev/sda: got %v, want ≈ %v (delta %v)", got, wantSDA, delta)
	}
}

// TestDB_GetLastSMARTCollectedAt_MissingDeviceReturnsNotFound verifies the
// "new drive" path: a device that has no smart_history rows yet must return
// found=false without an error, so the StaleSMARTChecker can skip it (new
// drives are not force-woken per PRD #236 user story 7).
func TestDB_GetLastSMARTCollectedAt_MissingDeviceReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	got, found, err := db.GetLastSMARTCollectedAt("/dev/never-seen")
	if err != nil {
		t.Fatalf("unexpected error for unknown device: %v", err)
	}
	if found {
		t.Errorf("expected found=false for unknown device, got found=true with ts=%v", got)
	}
	if !got.IsZero() {
		t.Errorf("expected zero timestamp for not-found, got %v", got)
	}
}
