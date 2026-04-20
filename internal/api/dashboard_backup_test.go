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
