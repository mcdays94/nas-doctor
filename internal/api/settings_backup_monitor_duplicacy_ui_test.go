package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSettingsHTML_DuplicacyMonitorSubsection asserts the new
// "Duplicacy" subsection renders under the existing "Backup Monitors"
// group in settings.html. Pinned via DOM-id substrings (V1c dashboard
// + V1b form-handlers both query by id, so renames must surface as
// failing assertions).
func TestSettingsHTML_DuplicacyMonitorSubsection(t *testing.T) {
	tpl := SettingsPage

	checks := []struct {
		name   string
		substr string
	}{
		{"section container", `id="duplicacy-monitor-section"`},
		{"section heading text", "Backup Monitors &rarr; Duplicacy"},
		{"entry list container", `id="duplicacy-monitor-list"`},
		{"add-entry button handler", "addDuplicacyMonitorEntry()"},
		{"per-entry render fn", "renderDuplicacyEntryForm("},
		{"per-entry collect fn", "collectDuplicacyMonitorEntries("},
		{"per-entry test fn", "testDuplicacyMonitorEntry("},
		{"per-entry remove fn", "removeDuplicacyMonitorEntry("},
		{"settings-load wire-up", "loadDuplicacyMonitorFromSettings"},
		{"save-payload key", "duplicacy: collectDuplicacyMonitorEntries()"},
		{"test endpoint URL", "/api/v1/backup-monitor/duplicacy/test"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tpl, tc.substr) {
				t.Errorf("settings.html missing %s; expected substring %q", tc.name, tc.substr)
			}
		})
	}

	// Existing Borg subsection MUST be unmodified — V1b is purely
	// additive. The Borg id, label, and Add button are pinned.
	borgInvariants := []string{
		`id="borg-monitor-section"`,
		"Backup Monitors &rarr; Borg",
		`id="borg-monitor-list"`,
		"addBorgMonitorRepo()",
	}
	for _, s := range borgInvariants {
		if !strings.Contains(tpl, s) {
			t.Errorf("Borg subsection invariant missing: %q", s)
		}
	}
}

// TestSettingsHTML_DuplicacyKindDropdownToggle pins the JS that
// shows/hides the StorageID input based on the Kind dropdown's
// value — acceptance criterion 3. Mirrors how other conditional
// fields toggle (fleet-instance picker on type=speed, etc.).
func TestSettingsHTML_DuplicacyKindDropdownToggle(t *testing.T) {
	tpl := SettingsPage
	checks := []string{
		// The toggle handler is referenced by the Kind <select>.
		"onDuplicacyKindChange(",
		// Per-entry kind <select> id pattern.
		"duplicacy-kind-",
		// Per-entry storage_id wrapper id pattern (the element that
		// gets shown/hidden).
		"duplicacy-storage-id-wrap-",
		// The two valid options render.
		`value="cli-repo"`,
		`value="web-cache"`,
	}
	for _, s := range checks {
		if !strings.Contains(tpl, s) {
			t.Errorf("Kind toggle wiring missing: %q", s)
		}
	}
}

// TestSettingsHTML_DuplicacyStaleAfterPlaceholder pins that the
// StaleAfter input renders with a 30-day placeholder (acceptance
// criterion 4: empty/zero is rendered as default-30 placeholder).
// The placeholder is what cues the user that leaving the field
// blank is the right way to opt into the default.
func TestSettingsHTML_DuplicacyStaleAfterPlaceholder(t *testing.T) {
	tpl := SettingsPage
	if !strings.Contains(tpl, `placeholder="30"`) {
		t.Errorf("stale_after input missing 30-day placeholder; users won't see default cue")
	}
	if !strings.Contains(tpl, "duplicacy-stale-after-") {
		t.Errorf("stale_after input id pattern missing")
	}
}

// TestPUTSettings_DuplicacyEntry_FullRoundTrip — end-to-end:
// PUT one Duplicacy entry covering both Kinds, GET back, assert the
// shape is preserved (acceptance criterion 2: round-trip is
// idempotent). Distinct from the slimmer round-trip in
// settings_backup_monitor_duplicacy_test.go (V1a) — that test pins
// the schema; this one pins the Settings-UI workflow contract.
func TestPUTSettings_DuplicacyEntry_FullRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	payload := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		BackupMonitor: BackupMonitorSettings{
			Duplicacy: []DuplicacyEntry{
				{
					Enabled:    true,
					Label:      "Photos",
					Kind:       "web-cache",
					Path:       "/cache/localhost/0",
					StorageID:  "photos",
					StaleAfter: 7,
				},
			},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleUpdateSettings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d; body=%s", w.Code, w.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getW := httptest.NewRecorder()
	srv.handleGetSettings(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getW.Code)
	}
	var back Settings
	if err := json.Unmarshal(getW.Body.Bytes(), &back); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(back.BackupMonitor.Duplicacy) != 1 {
		t.Fatalf("Duplicacy len = %d; want 1", len(back.BackupMonitor.Duplicacy))
	}
	got := back.BackupMonitor.Duplicacy[0]
	if got.Label != "Photos" || got.Kind != "web-cache" || got.Path != "/cache/localhost/0" || got.StorageID != "photos" {
		t.Errorf("round-trip lost fields: %+v", got)
	}
	if got.StaleAfter != 7 {
		t.Errorf("StaleAfter = %d; want 7", got.StaleAfter)
	}
}

// TestRegisterExtendedRoutes_ExposesDuplicacyTest pins the new
// route is registered through the router (404/405 means the
// registration was skipped or shadowed).
func TestRegisterExtendedRoutes_ExposesDuplicacyTest(t *testing.T) {
	srv := newSettingsTestServer()
	handler := srv.Router()

	body, _ := json.Marshal(map[string]any{
		"kind": "cli-repo",
		"path": "/nonexistent-test-path-aaaa",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("expected route to be registered, got %d: %s", rec.Code, rec.Body.String())
	}
	// We expect 200 (handler is total — every reason is a 200) with
	// reason=path_not_found because the path doesn't exist on this
	// test host. That's the production runner's contract.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from router (handler is total), got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["reason"] != "path_not_found" {
		t.Errorf("expected reason=path_not_found for nonexistent path; got %v", got["reason"])
	}
}

// TestRegisterExtendedRoutes_DuplicacyTest_RequiresAPIKey pins the
// 401 contract: when an API key is configured AND the request comes
// from outside the same origin (no Referer), the test endpoint
// returns 401. PRD #310 §5 / acceptance criterion 5.
func TestRegisterExtendedRoutes_DuplicacyTest_RequiresAPIKey(t *testing.T) {
	srv := newSettingsTestServer()

	// Persist a settings blob carrying an API key so the middleware
	// activates.
	settingsWithKey := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		APIKey:          "test-api-key-313",
	}
	raw, _ := json.Marshal(settingsWithKey)
	if err := srv.store.SetConfig(settingsConfigKey, string(raw)); err != nil {
		t.Fatalf("set config: %v", err)
	}

	handler := srv.Router()

	body, _ := json.Marshal(map[string]any{"kind": "cli-repo", "path": "/x"})

	t.Run("no auth header → 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rec.Code)
		}
	})

	t.Run("wrong key → 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer wrong-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rec.Code)
		}
	})

	t.Run("correct key → not 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-api-key-313")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("status = 401 with correct key; want pass-through")
		}
	})
}
