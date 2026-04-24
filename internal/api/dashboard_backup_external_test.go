package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_BackupWidget_RendersErrorState pins that the
// Backup widget branches on bj.error to render an error card with
// the specific reason visible. PRD #278 user stories 12-14 require
// the dashboard to surface per-repo failure reasons.
func TestDashboardJS_BackupWidget_RendersErrorState(t *testing.T) {
	js := DashboardJS
	contracts := []struct {
		name   string
		substr string
	}{
		{"branches on .error", "bj.error"},
		{"exposes error_reason as status text", "bj.error_reason"},
		{"has error-card marker class", "backup-card-error"},
		{"error message rendered from bj.error", "bj.error || 'Probe failed'"},
	}
	for _, tc := range contracts {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q for error-state contract: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestDashboardJS_BackupWidget_HasConfiguredPill pins the
// "Configured" pill path — rendered only when bj.configured is
// truthy. Distinguishes auto-detect entries (no pill) from
// explicit-config entries (pill visible) per PRD #278.
func TestDashboardJS_BackupWidget_HasConfiguredPill(t *testing.T) {
	js := DashboardJS
	// The branch guard and the uppercase label must both exist —
	// either alone would be a bug (always-render pill, or render
	// nothing even when configured).
	if !strings.Contains(js, "bj.configured") {
		t.Error("DashboardJS missing bj.configured branch guard")
	}
	if !strings.Contains(js, "CONFIGURED") {
		t.Error("DashboardJS missing CONFIGURED pill label")
	}
}

// TestDashboardJS_BackupWidget_PrefersLabelOverName pins the
// fallback behaviour: bj.label (user-supplied display name) wins;
// bj.name (repo basename) is the fallback.
func TestDashboardJS_BackupWidget_PrefersLabelOverName(t *testing.T) {
	js := DashboardJS
	if !strings.Contains(js, "bj.label || bj.name") {
		t.Error("DashboardJS missing label||name fallback — user stories 8 (explicit label) + 16 (upgrader sees repo basename)")
	}
}

// TestDashboardJS_BackupWidget_PreservesExistingHealthyRender
// regression-guards the pre-#279 happy-path card so the error-state
// branch doesn't break the common case.
func TestDashboardJS_BackupWidget_PreservesExistingHealthyRender(t *testing.T) {
	js := DashboardJS
	for _, s := range []string{
		"Snapshots:",
		"Size:",
		"Last:",
		"Encrypted",
		"last_success",
	} {
		if !strings.Contains(js, s) {
			t.Errorf("DashboardJS dropped healthy-card element %q on refactor", s)
		}
	}
}
