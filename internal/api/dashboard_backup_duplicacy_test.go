package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_BackupSection_DuplicacySupport pins the V1c
// dashboard rendering surface (PRD #310 / issue #314). The Backup
// section must:
//
//  1. Iterate snapshot.backup.duplicacy[] in ADDITION to backup.jobs[].
//  2. Render a per-entry kind tag ("DUPLICACY:CLI-REPO" /
//     "DUPLICACY:WEB-CACHE") so users can tell Borg rows from
//     Duplicacy rows at a glance.
//  3. Render a CONFIGURED pill on every Duplicacy row (entries are
//     always user-configured — there's no auto-detect for Duplicacy).
//  4. Render a status pill keyed off reason_code with the per-PRD
//     severity mapping (ok=success, no_snapshots_yet=info,
//     stale=warning, anything else=error).
//  5. Render the orthogonal RUNNING badge when currently_running is
//     true.
//  6. Render an error-card body when the reason maps to error
//     severity (path_not_found, path_unreadable, not_a_duplicacy_repo,
//     storage_id_not_found, corrupt_snapshot).
//  7. Combine Duplicacy + Borg counts in the section title so the
//     "Backup Jobs (N)" caption is correct in mixed-provider layouts.
//
// The test is JS-substring-driven (not a runtime exec) — same shape
// as the existing TestDashboardJS_BackupSection_* family. Asserts on
// the SOURCE of DashboardJS so any future refactor that splits
// rendering into a helper is caught immediately when the helper is
// renamed/relocated without preserving the dashboard contract.
func TestDashboardJS_BackupSection_DuplicacySupport(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		// (1) iterate backup.duplicacy
		{"reads backup.duplicacy", "backup.duplicacy"},
		{"loops over duplicacy entries", "for (var di = 0; di < dupList.length; di++)"},
		// (2) Kind tag — "DUPLICACY:" prefix + uppercase kind value
		{"emits DUPLICACY: kind prefix", "'DUPLICACY' + (de.kind ? ':' + de.kind.toUpperCase() : '')"},
		{"applies duplicacy-kind-tag class", "duplicacy-kind-tag"},
		// (3) CONFIGURED pill always rendered for duplicacy rows
		{"unconditional CONFIGURED pill on duplicacy rows", ">CONFIGURED<"},
		// (4) reason-code → severity mapping (closed set)
		{"defines dupSeverity helper", "var dupSeverity = function(reason)"},
		{"maps ok to success severity", "if (reason === 'ok') return 'ok'"},
		{"maps no_snapshots_yet to info severity", "if (reason === 'no_snapshots_yet') return 'info'"},
		{"maps stale to warning severity", "if (reason === 'stale') return 'warning'"},
		// (5) RUNNING badge — orthogonal aux flag
		{"renders RUNNING badge for currently_running entries", "duplicacy-running-badge"},
		{"badge text is RUNNING", ">RUNNING<"},
		{"guards on currently_running flag", "if (de.currently_running)"},
		// (6) error-card path
		{"branches on dupIsError for error-card body", "if (dupIsError)"},
		// (7) combined section title count
		{"counts both jobs and duplicacy in title", "totalRows = jobsList.length + dupList.length"},
		// per-row data fields the widget consumes (regression guards)
		{"reads de.snapshot_count", "de.snapshot_count"},
		{"reads de.latest_backup_size_bytes", "de.latest_backup_size_bytes"},
		{"reads de.latest_backup_at", "de.latest_backup_at"},
		{"reads de.latest_snapshot_id", "de.latest_snapshot_id"},
		{"reads de.latest_snapshot_revision", "de.latest_snapshot_revision"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestDashboardJS_BackupSection_EmptyState_AfterDuplicacy asserts the
// "no backup provider detected" empty-state copy is updated to mention
// Duplicacy as a configurable option. PRD #310 V1c README scour
// alignment.
func TestDashboardJS_BackupSection_DuplicacyEmptyStateMention(t *testing.T) {
	js := DashboardJS
	// Empty-state hint should mention Duplicacy as a configurable
	// provider so users know it's an option without having to dig
	// through Settings.
	if !strings.Contains(js, "Duplicacy") {
		t.Error("DashboardJS empty-state copy missing 'Duplicacy' — visitors with no backup configured won't know it's an option")
	}
	if !strings.Contains(js, "disk-read") {
		t.Error("DashboardJS empty-state copy missing 'disk-read' — should communicate that Duplicacy needs no extra binary install")
	}
}

// TestDashboardJS_BackupSection_NoBackticks asserts the rendered JS
// contains no backtick characters, which would terminate the Go
// raw-string DashboardJS literal early (AGENTS.md "backticks inside
// JS comments" gotcha).
func TestDashboardJS_BackupSection_NoBackticks(t *testing.T) {
	if strings.Contains(DashboardJS, "`") {
		t.Error("DashboardJS contains a backtick — would prematurely terminate the Go raw-string literal in dashboard.go")
	}
}
