package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestSaveSnapshot_EmptyAndDuplicateFindingIDsDoNotRollBack is the regression
// guard for the silent history-freeze in #323/#325.
//
// The findings table has PRIMARY KEY (snapshot_id, id). Before the fix, two
// findings with the same id — most commonly two empty ("") ids from the SMART
// trend findings, which arrive un-numbered — violated that constraint, aborted
// the SaveSnapshot transaction, and rolled back the snapshot AND its SMART /
// system history. Because the trend is computed from that now-frozen history,
// the failure latched until the appdata was wiped.
//
// SaveSnapshot must now persist every scan even when findings arrive with
// empty or duplicate ids, and it must not lose the accompanying history.
func TestSaveSnapshot_EmptyAndDuplicateFindingIDsDoNotRollBack(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(filepath.Join(dir, "test.db"), logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	snap := &internal.Snapshot{
		ID:        "snap-1",
		Timestamp: time.Now().UTC(),
		Duration:  0.5,
		SMART: []internal.SMARTInfo{
			{Device: "/dev/sda", Serial: "SN-A", Model: "Disk A", Temperature: 45, HealthPassed: true},
			{Device: "/dev/sdb", Serial: "SN-B", Model: "Disk B", Temperature: 46, HealthPassed: true},
		},
		Findings: []internal.Finding{
			// Two SMART trend findings as the scheduler would append them
			// before the fix: no ID at all.
			{Severity: internal.SeverityWarning, Category: internal.CategorySMART, Title: "Trend: /dev/sda warming"},
			{Severity: internal.SeverityWarning, Category: internal.CategorySMART, Title: "Trend: /dev/sdb warming"},
			// And an explicit duplicate ID for good measure.
			{ID: "F001", Severity: internal.SeverityInfo, Category: internal.CategorySMART, Title: "dup one"},
			{ID: "F001", Severity: internal.SeverityInfo, Category: internal.CategorySMART, Title: "dup two"},
		},
	}

	if err := db.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot returned error (transaction rolled back — history would be lost): %v", err)
	}

	// The snapshot itself must be retrievable.
	got, err := db.GetSnapshot("snap-1")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got == nil {
		t.Fatal("snapshot was not persisted — SaveSnapshot rolled back")
	}

	// All four findings must have been written (unique-ified), not dropped.
	var findingRows int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM findings WHERE snapshot_id = ?", "snap-1").Scan(&findingRows); err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if findingRows != 4 {
		t.Errorf("findings rows = %d; want 4 (none may be dropped or collide)", findingRows)
	}

	// The whole point: the SMART history for this scan must survive. A rolled-
	// back transaction is exactly what froze the trend charts in #323.
	var smartRows int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM smart_history WHERE snapshot_id = ?", "snap-1").Scan(&smartRows); err != nil {
		t.Fatalf("count smart_history: %v", err)
	}
	if smartRows != 2 {
		t.Errorf("smart_history rows = %d; want 2 (history must be preserved alongside the snapshot)", smartRows)
	}
}
