package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_BackupSection_HasEmptyStateHint verifies the Backup dashboard
// section renders an explanatory hint when no backup providers are detected,
// instead of silently rendering an empty section (see issue #132).
func TestDashboardJS_BackupSection_HasEmptyStateHint(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"empty state message", "No backup provider detected"},
		{"mentions Borg", "Borg"},
		{"mentions Restic", "Restic"},
		{"mentions Proxmox Backup Server", "Proxmox Backup Server"},
		{"mentions Duplicati", "Duplicati"},
		{"links to dashboard sections settings", "/settings#card-sections"},
		{"has Backup Monitoring title in empty state", "Backup Monitoring"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
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
