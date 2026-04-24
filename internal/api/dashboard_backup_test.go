package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_BackupSection_HasEmptyStateHint verifies the Backup dashboard
// section renders an explanatory hint when no backup providers are detected,
// instead of silently rendering an empty section (see issue #132).
//
// Rewritten in v0.9.10-rc2 (issue #279 Finding 3): the old copy recommended a
// "custom Dockerfile or sibling container" for Borg specifically, which is
// now wrong because Borg is bundled in the image and configurable via
// Settings → Backup Monitors → Borg.
func TestDashboardJS_BackupSection_HasEmptyStateHint(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"mentions Borg", "Borg"},
		{"mentions Restic", "Restic"},
		{"mentions Proxmox Backup Server", "Proxmox Backup Server"},
		{"mentions Duplicati", "Duplicati"},
		{"has Backup Monitoring title in empty state", "Backup Monitoring"},
		{"acknowledges Borg is bundled", "bundle"},
		{"points user at Backup Monitors settings", "/settings#backup-monitors"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestDashboardJS_BackupSection_EmptyStateCopy_RemovesStaleCustomDockerfile pins
// the rc2 rewrite — the old "custom Dockerfile or sibling container" phrasing
// must not reappear, because borg is now bundled and configurable. Restic/PBS/
// Duplicati still need a sidecar/Dockerfile so we keep the sidecar hint, but
// the old mass-attribution to all four providers is gone.
func TestDashboardJS_BackupSection_EmptyStateCopy_RemovesStaleCustomDockerfile(t *testing.T) {
	js := DashboardJS
	// The exact old substring that bundled all four providers into a
	// single recommendation to "install one of these (custom Dockerfile…)".
	// That guidance is now wrong for Borg and must be gone.
	if strings.Contains(js, "Install one of these (custom Dockerfile or sibling container sharing volumes/network) to enable monitoring.") {
		t.Error("DashboardJS still carries the pre-rc2 stale empty-state copy — the 'custom Dockerfile' recommendation applied to Borg too")
	}
}

// TestDashboardJS_BackupSection_PreservesExistingBehavior verifies the
// original "Backup Jobs (N)" rendering path is still present after the
// empty-state addition (regression guard).
func TestDashboardJS_BackupSection_PreservesExistingBehavior(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"sections.backup defined", "sections.backup = function(sn)"},
		{"data-section attribute", `data-section="backup"`},
		{"renders backup jobs count", "Backup Jobs ("},
		{"guards on backup.available and jobs", "backup.available && backup.jobs && backup.jobs.length > 0"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestDefaultSettings_BackupHiddenByDefault locks in the UX decision that
// Backup Monitoring is a niche, opt-in surface for new installs (see issue
// #132 discussion). Users with a backup provider installed in their container
// can toggle it on in Settings → Dashboard Sections.
func TestDefaultSettings_BackupHiddenByDefault(t *testing.T) {
	settings := defaultSettings()
	if settings.Sections.Backup {
		t.Error("defaultSettings().Sections.Backup = true; want false (niche feature, opt-in)")
	}
	// Regression guards — the sections that users actually want visible by
	// default must stay true. Catches a future refactor that accidentally
	// disables the commonly-used sections.
	for name, got := range map[string]bool{
		"Findings":  settings.Sections.Findings,
		"DiskSpace": settings.Sections.DiskSpace,
		"SMART":     settings.Sections.SMART,
		"Docker":    settings.Sections.Docker,
		"UPS":       settings.Sections.UPS,
		"Processes": settings.Sections.Processes,
	} {
		if !got {
			t.Errorf("defaultSettings().Sections.%s = false; want true", name)
		}
	}
}

// TestSettingsHTML_BackupToggleStartsOff verifies the Dashboard Sections
// toggle for Backup Monitoring renders in the OFF state at page load, so the
// "Backup: false" server-side default is visually consistent with the initial
// DOM and there's no flicker of the toggle flipping after settings load.
func TestSettingsHTML_BackupToggleStartsOff(t *testing.T) {
	html := SettingsPage
	onMarker := `class="toggle on" id="sec-backup"`
	offMarker := `class="toggle" id="sec-backup"`
	if strings.Contains(html, onMarker) {
		t.Errorf("settings.html has %q — the sec-backup toggle should NOT default to 'on' class", onMarker)
	}
	if !strings.Contains(html, offMarker) {
		t.Errorf("settings.html missing %q — sec-backup toggle should render off by default", offMarker)
	}
}
