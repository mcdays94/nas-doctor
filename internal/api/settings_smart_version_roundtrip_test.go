package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests in this file pin the end-to-end contract that fixed issue #268:
//
//   - The client MUST include settings_version in PUT payloads AND the
//     server MUST preserve a stored server-authoritative version across
//     any save, never letting a client omit or downgrade it.
//
// Without both halves, an older/misbehaving frontend can corrupt the
// stored settings_version to 0, which re-triggers the v1→v2 migration
// on the next internal getSettings() call. That migration:
//   - overwrites smart.wake_drives with the legacy flat
//     wake_drives_for_smart field (absent in v2 blobs → false), and
//   - flips smart.max_age_days=0 back to the 7-day seed.
//
// Both silent resets are user-visible data loss.

// seedV2Settings writes a v2-shaped settings blob into the store,
// simulating the persisted state of a user who has already upgraded
// past the v1→v2 migration and deliberately configured SMART to keep
// drives asleep (max_age_days=0, wake_drives=true — a real
// combination: "never wake to scan, but do wake when something else
// wakes them").
//
// Writes the authentic v2 wire shape (flat "smart": {...}) rather
// than a marshalled current-schema struct, so the v2→v3 migration
// path gets exercised end-to-end on the first getSettings() call.
func seedV2Settings(t *testing.T, srv *Server, maxAgeDays int, wakeDrives bool) {
	t.Helper()
	blob := map[string]interface{}{
		"settings_version": 2,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"smart": map[string]interface{}{
			"wake_drives":  wakeDrives,
			"max_age_days": maxAgeDays,
		},
	}
	data, err := json.Marshal(blob)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := srv.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

// seedV3Settings writes a v3-shaped settings blob (the current
// schema) into the store. Used by tests that want to exercise the
// post-migration steady state directly.
func seedV3Settings(t *testing.T, srv *Server, maxAgeDays int, wakeDrives bool) {
	t.Helper()
	s := defaultSettings()
	s.SettingsVersion = 3
	s.AdvancedScans.SMART.MaxAgeDays = maxAgeDays
	s.AdvancedScans.SMART.WakeDrives = wakeDrives
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := srv.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

// readStoredSettings returns the raw JSON blob persisted in the store,
// decoded through a map so we can assert on absence of version fields
// as well as presence.
func readStoredSettings(t *testing.T, srv *Server) map[string]interface{} {
	t.Helper()
	raw, err := srv.store.GetConfig(settingsConfigKey)
	if err != nil {
		t.Fatalf("read stored settings: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode stored settings: %v", err)
	}
	return out
}

// v3AdvancedScansPayload returns a fresh map that carries an
// advanced_scans tree with all six subsystems seeded at IntervalSec=0
// plus the given SMART knobs. Centralises the boilerplate so the #268
// regression tests stay focused on version-preservation behaviour.
func v3AdvancedScansPayload(wakeDrives bool, maxAgeDays int) map[string]any {
	return map[string]any{
		"smart":      map[string]any{"wake_drives": wakeDrives, "max_age_days": maxAgeDays, "interval_sec": 0},
		"docker":     map[string]any{"interval_sec": 0},
		"proxmox":    map[string]any{"interval_sec": 0},
		"kubernetes": map[string]any{"interval_sec": 0},
		"zfs":        map[string]any{"interval_sec": 0},
		"gpu":        map[string]any{"interval_sec": 0},
	}
}

// TestHandleUpdateSettings_PreservesStoredSettingsVersion — core
// backend regression for #268, re-pinned for the v3 schema. A v3-
// shaped blob is in storage. The client PUTs a payload that OMITS
// settings_version. The server must preserve the stored version
// rather than persisting the zero-value from the unmarshal.
func TestHandleUpdateSettings_PreservesStoredSettingsVersion(t *testing.T) {
	srv := newSettingsTestServer()
	seedV3Settings(t, srv, 0, true)

	// Client payload WITHOUT settings_version. Valid scan_interval +
	// theme so the handler accepts it.
	rec := putSettings(t, srv, map[string]any{
		"scan_interval":  "30m",
		"theme":          "midnight",
		"advanced_scans": v3AdvancedScansPayload(true, 0),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := readStoredSettings(t, srv)
	gotVersion, _ := stored["settings_version"].(float64)
	if int(gotVersion) != 3 {
		t.Errorf("stored settings_version after PUT: got %v, want 3 (server must preserve, not accept client omission as 0)", stored["settings_version"])
	}
}

// TestHandleUpdateSettings_ClientCannotDowngradeSettingsVersion —
// defence-in-depth. Even an explicit client-side settings_version=0
// in the PUT body must not overwrite the stored v3. settings_version
// is server-authoritative.
func TestHandleUpdateSettings_ClientCannotDowngradeSettingsVersion(t *testing.T) {
	srv := newSettingsTestServer()
	seedV3Settings(t, srv, 14, false)

	rec := putSettings(t, srv, map[string]any{
		"settings_version": 0, // malicious/buggy client tries to downgrade
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans":   v3AdvancedScansPayload(false, 14),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := readStoredSettings(t, srv)
	gotVersion, _ := stored["settings_version"].(float64)
	if int(gotVersion) != 3 {
		t.Errorf("client-sent settings_version=0 must be ignored; stored version = %v, want 3", stored["settings_version"])
	}
}

// TestHandleUpdateSettings_MaxAgeDaysZero_SurvivesInternalGetSettings
// — the full end-to-end scenario users hit: set max_age_days=0, save,
// something (dashboard load, backup API, anything) triggers
// getSettings() internally which runs migration + persist. The 0 must
// survive that round trip.
func TestHandleUpdateSettings_MaxAgeDaysZero_SurvivesInternalGetSettings(t *testing.T) {
	srv := newSettingsTestServer()
	seedV3Settings(t, srv, 7, false)

	// User edits: set max_age_days=0, payload includes
	// settings_version=3 (the state the fixed frontend produces).
	rec := putSettings(t, srv, map[string]any{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans":   v3AdvancedScansPayload(false, 0),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from PUT, got %d: %s", rec.Code, rec.Body.String())
	}

	// Force the getSettings() code path (migrate + persist back).
	loaded := srv.getSettings()
	if loaded.AdvancedScans.SMART.MaxAgeDays != 0 {
		t.Errorf("after internal getSettings(): advanced_scans.smart.max_age_days = %d, want 0 (user-chosen value must not be re-seeded by a stale migration)", loaded.AdvancedScans.SMART.MaxAgeDays)
	}

	// And the stored blob too, since getSettings() persists.
	stored := readStoredSettings(t, srv)
	advScans, ok := stored["advanced_scans"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored settings missing advanced_scans object: %v", stored["advanced_scans"])
	}
	smartMap, ok := advScans["smart"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored advanced_scans missing smart object: %v", advScans["smart"])
	}
	mad, _ := smartMap["max_age_days"].(float64)
	if int(mad) != 0 {
		t.Errorf("persisted advanced_scans.smart.max_age_days after getSettings(): got %v, want 0", smartMap["max_age_days"])
	}
}

// TestHandleUpdateSettings_WakeDrivesTrue_SurvivesInternalGetSettings
// — same scenario as above but for wake_drives=true. The re-migration
// bug clobbered this field by reading the missing legacy
// wake_drives_for_smart top-level key (absent in v2+ blobs → false).
func TestHandleUpdateSettings_WakeDrivesTrue_SurvivesInternalGetSettings(t *testing.T) {
	srv := newSettingsTestServer()
	seedV3Settings(t, srv, 7, false)

	rec := putSettings(t, srv, map[string]any{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans":   v3AdvancedScansPayload(true, 7),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from PUT, got %d: %s", rec.Code, rec.Body.String())
	}

	loaded := srv.getSettings()
	if !loaded.AdvancedScans.SMART.WakeDrives {
		t.Errorf("after internal getSettings(): advanced_scans.smart.wake_drives = false, want true (user-chosen value must not be clobbered by a stale migration)")
	}

	stored := readStoredSettings(t, srv)
	advScans, ok := stored["advanced_scans"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored settings missing advanced_scans object: %v", stored["advanced_scans"])
	}
	smartMap, ok := advScans["smart"].(map[string]interface{})
	if !ok {
		t.Fatalf("stored advanced_scans missing smart object: %v", advScans["smart"])
	}
	wd, _ := smartMap["wake_drives"].(bool)
	if !wd {
		t.Errorf("persisted advanced_scans.smart.wake_drives after getSettings(): got %v, want true", smartMap["wake_drives"])
	}
}

// TestSettingsHTMLPayloadIncludesSettingsVersion — frontend regression
// guard. buildSettingsPayload() must include settings_version in the
// returned payload object so the backend has an incoming value to
// preserve. This is the user-facing half of the fix; without it, the
// backend's max-of-stored-and-incoming still works (stored wins over
// missing), but future backend refactors that loosen the guard must
// still receive an explicit version from the UI.
func TestSettingsHTMLPayloadIncludesSettingsVersion(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	fnStart := strings.Index(content, "function buildSettingsPayload()")
	if fnStart == -1 {
		t.Fatalf("could not locate buildSettingsPayload() in settings.html")
	}
	// Locate the returned object literal. We scan from the function
	// start for "return {" and then find the matching close-brace at
	// brace-depth 0 within the block. Simple brace counter is
	// sufficient because the returned object has no embedded strings
	// containing unbalanced braces in this codebase.
	returnStart := strings.Index(content[fnStart:], "return {")
	if returnStart == -1 {
		t.Fatalf("could not locate `return {` inside buildSettingsPayload()")
	}
	returnStart += fnStart + len("return ")
	depth := 0
	returnEnd := -1
	for i := returnStart; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				returnEnd = i + 1
				break
			}
		}
		if returnEnd != -1 {
			break
		}
	}
	if returnEnd == -1 {
		t.Fatalf("could not find matching brace for buildSettingsPayload() return object")
	}
	payload := content[returnStart:returnEnd]
	if !strings.Contains(payload, "settings_version") {
		t.Errorf("buildSettingsPayload() return object must include a settings_version key (issue #268). Payload was:\n%s", payload)
	}
}
