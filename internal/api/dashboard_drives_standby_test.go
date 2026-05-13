package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_DrivesSection_RendersStandbyPlaceholders verifies that drives
// reported in snap.SMARTStandbyDevices are surfaced as placeholder rows in the
// DRIVES section, instead of being silently omitted.
//
// Surfaced by issue #323 / fix tracked in #324: after an appdata wipe, spun-down
// drives appear "missing" from the dashboard because the SMART snapshot only
// contains drives actively read this cycle. The StaleSMARTChecker won't
// force-wake them (PRD #236 user story 7 carve-out at stale_smart.go:108-111)
// until smart_history has rows for them, so the visibility gap can persist
// indefinitely. Pure UX fix here: honors smart.wake_drives=false as the user
// has it set, just makes the gap visible with an explanatory message.
func TestDashboardJS_DrivesSection_RendersStandbyPlaceholders(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"reads smart_standby_devices from snapshot", "smart_standby_devices"},
		{"explains the in-standby state", "in standby"},
		{"explains the wake_drives setting", "Wake drives for SMART check"},
		{"links to advanced scan settings", "/settings#card-advanced"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q (expected substring: %q)", tc.name, tc.substr)
			}
		})
	}
}

// TestDashboardJS_DrivesSection_StandbyCountedInHeader pins the requirement
// that the "Drives (N)" header in the DRIVES section counts standby drives
// alongside actively-read ones. Without this, a user with 4 awake + 4
// standby drives sees "Drives (4)" and reasonably concludes 4 are missing.
//
// The check is a substring on the count expression rather than the exact
// formatting, so a refactor that preserves the behavior (e.g. extracting
// the count to a helper) doesn't false-positive.
func TestDashboardJS_DrivesSection_StandbyCountedInHeader(t *testing.T) {
	js := DashboardJS

	if !strings.Contains(js, "standbyDevices.length") {
		t.Error("DRIVES (N) header count expression must reference standbyDevices.length so standby drives contribute to the visible count")
	}
}

// TestDashboardJS_DrivesSection_PreservesExistingBehavior is a regression
// guard for the active-drive render path. The standby additions must not
// break the existing handling of awake drives with full SMART data, slot
// labels, merged-view storage rows, or the section's outer scaffolding.
func TestDashboardJS_DrivesSection_PreservesExistingBehavior(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"sections.drives defined", "sections.drives = function(sn, st)"},
		{"data-section attribute", `data-section="drives"`},
		{"reads sn.smart", "sn.smart || []"},
		{"reads sn.disks", "sn.disks || []"},
		{"iterates active smart array", "for (var si = 0; si < smart.length; si++)"},
		{"renders slot label from array_slot", "s.array_slot"},
		{"renders NO DATA badge for data_available=false drives", ">NO DATA<"},
		{"renders merged-view storage rows for unmatched disks", "unmatchedDisks"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS regression: existing behavior %q lost (expected substring: %q)", tc.name, tc.substr)
			}
		})
	}
}
