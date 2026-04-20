package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(dbPath, logger)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedServiceChecks(t *testing.T, db *DB, keys ...string) {
	t.Helper()
	now := time.Now().UTC()
	results := make([]internal.ServiceCheckResult, 0, len(keys))
	for i, k := range keys {
		results = append(results, internal.ServiceCheckResult{
			Key:              k,
			Name:             k + "-name",
			Type:             "http",
			Target:           "http://example.com/" + k,
			Status:           "up",
			ResponseMS:       10,
			FailureThreshold: 1,
			FailureSeverity:  internal.SeverityWarning,
			CheckedAt:        now.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		})
	}
	if err := db.SaveServiceCheckResults(results); err != nil {
		t.Fatalf("SaveServiceCheckResults: %v", err)
	}
}

func TestDB_DeleteServiceChecksNotIn_KeepsSpecifiedKeys(t *testing.T) {
	db := newTestDB(t)

	seedServiceChecks(t, db, "a", "b", "b", "c", "c", "c")

	deleted, err := db.DeleteServiceChecksNotIn([]string{"a", "b"})
	if err != nil {
		t.Fatalf("DeleteServiceChecksNotIn: %v", err)
	}
	// 3 rows for "c" should be removed.
	if deleted != 3 {
		t.Errorf("expected 3 rows deleted (for key c), got %d", deleted)
	}

	entries, err := db.ListLatestServiceChecks(100)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	gotKeys := make(map[string]bool)
	for _, e := range entries {
		gotKeys[e.Key] = true
	}
	if !gotKeys["a"] || !gotKeys["b"] {
		t.Errorf("expected keys a & b retained, got %v", gotKeys)
	}
	if gotKeys["c"] {
		t.Error("key c should have been purged")
	}
}

func TestDB_DeleteServiceChecksNotIn_EmptyKeepList_DeletesAll(t *testing.T) {
	db := newTestDB(t)

	seedServiceChecks(t, db, "a", "b", "c")

	deleted, err := db.DeleteServiceChecksNotIn(nil)
	if err != nil {
		t.Fatalf("DeleteServiceChecksNotIn(nil): %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 rows deleted with nil keep list, got %d", deleted)
	}

	entries, _ := db.ListLatestServiceChecks(100)
	if len(entries) != 0 {
		t.Errorf("expected 0 remaining entries, got %d", len(entries))
	}

	// Same behaviour for non-nil empty slice.
	seedServiceChecks(t, db, "x")
	deleted, err = db.DeleteServiceChecksNotIn([]string{})
	if err != nil {
		t.Fatalf("DeleteServiceChecksNotIn([]): %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 row deleted with empty slice, got %d", deleted)
	}
}

func TestDB_DeleteServiceChecksNotIn_NoOpWhenAllRetained(t *testing.T) {
	db := newTestDB(t)

	seedServiceChecks(t, db, "a", "b")

	deleted, err := db.DeleteServiceChecksNotIn([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("DeleteServiceChecksNotIn: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 rows deleted, got %d", deleted)
	}

	entries, _ := db.ListLatestServiceChecks(100)
	if len(entries) != 2 {
		t.Errorf("expected 2 remaining entries, got %d", len(entries))
	}
}
