package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDB_GetDiskHistoryInRange verifies the time-windowed variant of
// GetDiskHistory only returns rows whose timestamp falls within the
// requested look-back window.
//
// Issue #166: /disk/<serial> charts need a 1D / 1W / 1M / 1Y selector.
// Filtering by row count is not equivalent to filtering by time window,
// so we need a separate method.
func TestDB_GetDiskHistoryInRange(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Seed a snapshot and several smart_history rows at known ages.
	snapID := "snap-1"
	now := time.Now().UTC()
	if _, err := db.db.Exec(
		`INSERT INTO snapshots (id, timestamp, duration_seconds, data) VALUES (?, ?, ?, ?)`,
		snapID, now, 0.1, "{}",
	); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	// Ages in hours: 1h (in 1D), 23h (in 1D), 25h (out of 1D, in 1W),
	// 48h (out of 1D, in 1W), 168h (right at 1W boundary — excluded by strict >).
	ages := []time.Duration{
		1 * time.Hour,
		23 * time.Hour,
		25 * time.Hour,
		48 * time.Hour,
		169 * time.Hour, // >1W ago
	}
	for i, age := range ages {
		ts := now.Add(-age)
		if _, err := db.db.Exec(
			`INSERT INTO smart_history (snapshot_id, device, serial, model, temperature, reallocated, pending, udma_crc, command_timeout, power_on_hours, health_passed, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapID, "/dev/sda", "SERIAL1", "MODEL", 30+i, int64(i), int64(0), int64(0), int64(0), int64(1000+i), true, ts,
		); err != nil {
			t.Fatalf("insert smart_history row %d: %v", i, err)
		}
	}

	// 1D window (24h): expect the 1h and 23h rows only.
	points, err := db.GetDiskHistoryInRange("SERIAL1", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetDiskHistoryInRange(24h): %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 rows within 24h, got %d", len(points))
	}
	// Ordered ASC by timestamp — oldest first.
	if !points[0].Timestamp.Before(points[1].Timestamp) {
		t.Errorf("expected ASC order, got %v then %v", points[0].Timestamp, points[1].Timestamp)
	}

	// 1W window (168h): expect 1h, 23h, 25h, 48h (4 rows; 169h is outside).
	pointsWeek, err := db.GetDiskHistoryInRange("SERIAL1", 168*time.Hour)
	if err != nil {
		t.Fatalf("GetDiskHistoryInRange(168h): %v", err)
	}
	if len(pointsWeek) != 4 {
		t.Fatalf("expected 4 rows within 168h, got %d", len(pointsWeek))
	}

	// 1Y window should include all 5.
	pointsYear, err := db.GetDiskHistoryInRange("SERIAL1", 8760*time.Hour)
	if err != nil {
		t.Fatalf("GetDiskHistoryInRange(1y): %v", err)
	}
	if len(pointsYear) != 5 {
		t.Fatalf("expected 5 rows within 1y, got %d", len(pointsYear))
	}

	// Different serial returns nothing even within the window.
	pointsOther, err := db.GetDiskHistoryInRange("NONEXISTENT", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetDiskHistoryInRange(NONEXISTENT): %v", err)
	}
	if len(pointsOther) != 0 {
		t.Errorf("expected 0 rows for unknown serial, got %d", len(pointsOther))
	}
}

// TestDB_GetDiskHistoryInRange_PreservesLegacyGetDiskHistory ensures the
// original row-limited GetDiskHistory still works unchanged alongside the
// new time-windowed variant. Scheduler + other callers still rely on it.
func TestDB_GetDiskHistoryInRange_PreservesLegacyGetDiskHistory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	snapID := "snap-legacy"
	now := time.Now().UTC()
	if _, err := db.db.Exec(
		`INSERT INTO snapshots (id, timestamp, duration_seconds, data) VALUES (?, ?, ?, ?)`,
		snapID, now, 0.1, "{}",
	); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	for i := 0; i < 3; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		if _, err := db.db.Exec(
			`INSERT INTO smart_history (snapshot_id, device, serial, model, temperature, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			snapID, "/dev/sda", "LEGACY", "MODEL", 35, ts,
		); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	points, err := db.GetDiskHistory("LEGACY", 500)
	if err != nil {
		t.Fatalf("GetDiskHistory: %v", err)
	}
	if len(points) != 3 {
		t.Errorf("legacy GetDiskHistory: expected 3 rows, got %d", len(points))
	}
}
