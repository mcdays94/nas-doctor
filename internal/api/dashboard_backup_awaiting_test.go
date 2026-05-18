package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #328 — Dashboard Backup widget renders "Initial scan pending" placeholder
// instead of "no backup provider detected" copy when the user has configured
// external Borg/Duplicacy repos via Settings but no scan has completed yet.
//
// Three cross-layer invariants are pinned here so a future refactor cannot
// silently regress the user-visible polish:
//
//  1. /api/v1/status exposes backup_monitor.{borg_count,duplicacy_count} so
//     the dashboard JS knows how many external repos are configured WITHOUT
//     leaking paths or credentials.
//  2. Only ENABLED entries are counted — disabled repos must not contribute
//     to the placeholder render or the user sees "Initial scan pending"
//     forever when they have all repos disabled.
//  3. DashboardJS renders the "Initial scan pending" copy when the counts
//     are non-zero AND snap.backup has no rows; the existing data-path and
//     "no backup provider detected" empty state are preserved unchanged.

// TestDashboardJS_BackupSection_RendersAwaitingFirstScan verifies the Backup
// dashboard section renders a distinct "Initial scan pending" placeholder
// when the user has configured external Borg/Duplicacy repos but no snapshot
// data has landed yet (see issue #328).
//
// This is the post-fresh-deploy window @adriendecombe observed in the #323
// debug thread — before #328, the dashboard rendered configured-but-no-data
// repos with the same "No backup provider detected or configured" copy used
// for actually-empty installs, which read as a failure. The fix introduces
// a third rendering branch keyed on _statusData.backup_monitor counts.
func TestDashboardJS_BackupSection_RendersAwaitingFirstScan(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"headline copy", "Initial scan pending"},
		{"reads borg count from statusData", "backup_monitor.borg_count"},
		{"reads duplicacy count from statusData", "backup_monitor.duplicacy_count"},
		{"explains first-scan-in-progress state", "First scan in progress"},
		{"links user to Backup Monitors settings", "/settings#backup-monitors"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q (expected substring: %q) — required for issue #328 awaiting-first-scan placeholder", tc.name, tc.substr)
			}
		})
	}
}

// TestDashboardJS_BackupSection_AwaitingPlaceholder_DistinctFromFailure pins
// the requirement that the "Initial scan pending" state is VISUALLY distinct
// from a real failure render. Specifically: a SYNCING pill must be present
// (the explicit non-error cue), and the awaiting copy must not be styled
// with the same red colour used for failed Borg/Duplicacy probes.
//
// This is the polish guard for #328: the bug was that configured-but-no-data
// repos rendered as "failing" — a future refactor that conflates the
// awaiting branch with the failure branch (or strips the SYNCING pill)
// would regress the fix. The check covers BOTH the explicit positive cue
// (SYNCING pill string + cyan colour) AND the absence of the red colour
// near the "Initial scan pending" headline.
func TestDashboardJS_BackupSection_AwaitingPlaceholder_DistinctFromFailure(t *testing.T) {
	js := DashboardJS

	// Positive cues.
	if !strings.Contains(js, "SYNCING") {
		t.Error(`DashboardJS missing "SYNCING" pill — the explicit "this is not a failure" visual cue must be present in the awaiting-first-scan placeholder`)
	}
	// Cyan colour token used by the SYNCING badge. Mirrors the
	// "currently_running" badge from the Duplicacy renderer for visual
	// consistency. If a future refactor changes the colour family
	// (e.g. to amber or to red), this test catches it.
	if !strings.Contains(js, "rgba(34,211,238,0.15)") {
		t.Error(`DashboardJS missing cyan colour token rgba(34,211,238,0.15) used by the SYNCING badge background — the awaiting placeholder must remain visually distinct from a failure state`)
	}

	// Negative cue: no red colouring in the immediate vicinity of the
	// "Initial scan pending" headline. The window is generous (1000
	// chars in each direction) to catch a future refactor that re-uses
	// the existing error-card colour palette inside the awaiting branch.
	idx := strings.Index(js, "Initial scan pending")
	if idx < 0 {
		t.Fatal(`"Initial scan pending" not present at all — the headline copy regression already caught upstream; this test is a follow-on guard`)
	}
	start := idx - 1000
	if start < 0 {
		start = 0
	}
	end := idx + 1000
	if end > len(js) {
		end = len(js)
	}
	window := js[start:end]
	for _, redToken := range []string{"var(--red", "#dc2626"} {
		if strings.Contains(window, redToken) {
			t.Errorf(`red colour token %q appears within 1000 chars of "Initial scan pending" — the awaiting placeholder must NOT look like a failure (issue #328)`, redToken)
		}
	}
}

// newBackupAwaitingTestServer builds a minimal Server for exercising
// /api/v1/status with the backup-monitor settings exposure path.
func newBackupAwaitingTestServer() *Server {
	return &Server{
		store:     storage.NewFakeStore(),
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
}

// seedSettings writes a Settings blob to the FakeStore so getSettings()
// returns the requested shape. Mirrors the production flow that loads
// settings from store.GetConfig(settingsConfigKey).
func seedSettings(t *testing.T, srv *Server, s Settings) {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := srv.store.SetConfig(settingsConfigKey, string(raw)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

// TestStatusEndpoint_BackupMonitorCounts_OnlyEnabled verifies that disabled
// external Borg / Duplicacy entries do NOT contribute to the counts surfaced
// in /api/v1/status. Without this guard, a user who toggled a repo off in
// Settings would still see "Initial scan pending" forever — the placeholder
// only makes sense when there's a configured-AND-enabled repo that the
// scheduler will actually probe.
func TestStatusEndpoint_BackupMonitorCounts_OnlyEnabled(t *testing.T) {
	srv := newBackupAwaitingTestServer()
	seedSettings(t, srv, Settings{
		SettingsVersion: currentSettingsVersion,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{
				{Enabled: true, RepoPath: "/mnt/borg/main"},
				{Enabled: false, RepoPath: "/mnt/borg/disabled-a"},
				{Enabled: false, RepoPath: "/mnt/borg/disabled-b"},
			},
			Duplicacy: []DuplicacyEntry{
				{Enabled: false, Path: "/mnt/dup/disabled", Kind: "cli-repo"},
				{Enabled: true, Path: "/mnt/dup/photos", Kind: "cli-repo"},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	bm, ok := parsed["backup_monitor"].(map[string]interface{})
	if !ok {
		t.Fatalf("backup_monitor missing (would have made the awaiting placeholder unreachable): %v", parsed)
	}
	if got, _ := bm["borg_count"].(float64); int(got) != 1 {
		t.Errorf("borg_count = %v; want 1 (only one Enabled=true Borg entry); disabled entries must not be counted", bm["borg_count"])
	}
	if got, _ := bm["duplicacy_count"].(float64); int(got) != 1 {
		t.Errorf("duplicacy_count = %v; want 1 (only one Enabled=true Duplicacy entry); disabled entries must not be counted", bm["duplicacy_count"])
	}
}

// TestStatusEndpoint_BackupMonitorAbsent_WhenNothingConfigured locks in
// omitempty behavior: when neither Borg nor Duplicacy entries exist (or
// all are disabled), the backup_monitor key is absent from the response
// entirely. This is what allows the dashboard JS to fall through to the
// "No backup provider detected" empty-state branch unchanged. Without
// this, the JS would have to also count the values, duplicating the
// truth check in two places.
func TestStatusEndpoint_BackupMonitorAbsent_WhenNothingConfigured(t *testing.T) {
	srv := newBackupAwaitingTestServer()
	seedSettings(t, srv, Settings{
		SettingsVersion: currentSettingsVersion,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{
				{Enabled: false, RepoPath: "/mnt/borg/disabled"},
			},
			Duplicacy: []DuplicacyEntry{},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, present := parsed["backup_monitor"]; present {
		t.Errorf("backup_monitor must be absent (omitempty) when nothing is enabled; got %v", parsed["backup_monitor"])
	}
}

// TestStatusEndpoint_ExposesBackupMonitorCounts verifies /api/v1/status surfaces
// the count of enabled Borg and Duplicacy external repos from
// settings.BackupMonitor.{Borg,Duplicacy}. The dashboard JS reads these to
// detect the "configured-but-not-yet-scanned" state and render the
// "Initial scan pending" placeholder (see issue #328 + the matching
// DashboardJS test above).
//
// Surface is intentionally a flat pair of integer counts. No paths, no
// credentials — just enough signal for the JS to pick the right render
// branch. Mirrors the data_ephemeral pattern: the server already knows
// from settings, surface a derived flag/count for the browser to react.
func TestStatusEndpoint_ExposesBackupMonitorCounts(t *testing.T) {
	srv := newBackupAwaitingTestServer()
	seedSettings(t, srv, Settings{
		SettingsVersion: currentSettingsVersion,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{
				{Enabled: true, RepoPath: "/mnt/borg/main"},
				{Enabled: true, RepoPath: "/mnt/borg/offsite"},
			},
			Duplicacy: []DuplicacyEntry{
				{Enabled: true, Path: "/mnt/dup/photos", Kind: "cli-repo"},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/status returned %d: %s", rec.Code, rec.Body.String())
	}

	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	bm, ok := parsed["backup_monitor"].(map[string]interface{})
	if !ok {
		t.Fatalf("backup_monitor missing from /api/v1/status response: keys=%v", keysOf(parsed))
	}
	if got, _ := bm["borg_count"].(float64); int(got) != 2 {
		t.Errorf("backup_monitor.borg_count = %v; want 2", bm["borg_count"])
	}
	if got, _ := bm["duplicacy_count"].(float64); int(got) != 1 {
		t.Errorf("backup_monitor.duplicacy_count = %v; want 1", bm["duplicacy_count"])
	}
}


