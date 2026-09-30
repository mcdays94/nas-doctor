package scheduler

import (
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestEnsureUniqueFindingIDs verifies that appended, un-numbered findings (the
// SMART trend findings) receive unique IDs that do not collide with the
// analyzer's existing F%03d numbering — the collision that used to roll back
// SaveSnapshot and freeze history (#323/#325).
func TestEnsureUniqueFindingIDs(t *testing.T) {
	t.Run("empty IDs are filled without colliding with existing ones", func(t *testing.T) {
		findings := []internal.Finding{
			{ID: "F001", Title: "analyzer a"},
			{ID: "F002", Title: "analyzer b"},
			{ID: "", Title: "trend a"}, // appended trend finding, no ID
			{ID: "", Title: "trend b"}, // appended trend finding, no ID
		}
		ensureUniqueFindingIDs(findings)

		seen := map[string]bool{}
		for i, f := range findings {
			if f.ID == "" {
				t.Errorf("finding %d (%q) still has an empty ID", i, f.Title)
			}
			if seen[f.ID] {
				t.Errorf("duplicate ID %q assigned", f.ID)
			}
			seen[f.ID] = true
		}
		// Pre-existing IDs must be preserved.
		if findings[0].ID != "F001" || findings[1].ID != "F002" {
			t.Errorf("existing IDs were mutated: %q, %q", findings[0].ID, findings[1].ID)
		}
	})

	t.Run("all empty IDs get distinct values", func(t *testing.T) {
		findings := []internal.Finding{{}, {}, {}}
		ensureUniqueFindingIDs(findings)
		seen := map[string]bool{}
		for i, f := range findings {
			if f.ID == "" {
				t.Fatalf("finding %d still empty", i)
			}
			if seen[f.ID] {
				t.Fatalf("duplicate ID %q", f.ID)
			}
			seen[f.ID] = true
		}
	})

	t.Run("nil and empty slices are safe", func(t *testing.T) {
		ensureUniqueFindingIDs(nil)
		ensureUniqueFindingIDs([]internal.Finding{})
	})
}
