package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsHTMLIncludesWakeDrivesForSMARTToggle verifies the settings
// template ships the wake-drives toggle + disclaimer required by issue #198.
// Originally (pre-#237) the toggle lived directly inside the generic
// Advanced card. #237 moved it out to a dedicated "Advanced Scan Settings"
// card; #256 merged it back into the generic Advanced card (id="card-advanced")
// after UAT flagged the two-card split as clutter. #259 relocated the
// backing field from data.smart to data.advanced_scans.smart.
//
// This is a cross-reference test: it confirms the HTML mentions every
// symbol the JS load/save wiring expects, so a future refactor that
// renames one side can't silently break the round-trip.
func TestSettingsHTMLIncludesWakeDrivesForSMARTToggle(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	checks := []struct {
		name   string
		substr string
	}{
		// Generic Advanced card anchor (post-#256 home).
		{"advanced card anchor", `id="card-advanced"`},
		// Section nav link to the Advanced card.
		{"advanced nav link", `href="#card-advanced"`},
		// Disclosure element — using <details>/<summary> per the issue's
		// guidance (no extra JS needed).
		{"details element", `<details`},
		{"summary element", `<summary`},
		// The toggle control + its stable id for load/save wiring.
		{"wake-drives toggle id", `id="wake-drives-for-smart"`},
		// Load path reads the v3 nested JSON field.
		{"load binds nested field", `advanced_scans`},
		// Save payload writes the v3 nested JSON field.
		{"save sends nested field", `advanced_scans:`},
		// Disclaimer text must communicate the wear trade-off. We keep
		// the assertion loose so copy can be edited, but pin the key
		// concepts: spin-ups and opt-in intent.
		{"disclaimer mentions spin-ups", `spin-up`},
		{"disclaimer mentions scan interval", `30-min`},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.substr) {
				t.Errorf("settings.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestSettingsRoundTrip_WakeDrivesForSMART exercises the GET/PUT cycle for
// the wake-drives flag using the v3 nested schema
// (Settings.AdvancedScans.SMART.WakeDrives, advanced_scans.smart.wake_drives
// on the wire).
func TestSettingsRoundTrip_WakeDrivesForSMART(t *testing.T) {
	srv := newSettingsTestServer()

	put := map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": true, "max_age_days": 7, "interval_sec": 0},
			"docker":     map[string]interface{}{"interval_sec": 0},
			"proxmox":    map[string]interface{}{"interval_sec": 0},
			"kubernetes": map[string]interface{}{"interval_sec": 0},
			"zfs":        map[string]interface{}{"interval_sec": 0},
			"gpu":        map[string]interface{}{"interval_sec": 0},
		},
	}
	body, _ := json.Marshal(put)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleUpdateSettings(rr, req)
	if rr.Code != http.StatusOK {
		b, _ := io.ReadAll(rr.Body)
		t.Fatalf("PUT returned %d: %s", rr.Code, b)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr2 := httptest.NewRecorder()
	srv.handleGetSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET returned %d", rr2.Code)
	}
	var got Settings
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse GET response: %v", err)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives did not round-trip; got false, wanted true")
	}
}

// TestSettingsDefault_WakeDrivesForSMARTIsFalse guards against a regression
// where the default flips; the whole point of #198 is that default=false
// means drives in standby are NOT woken by SMART scans.
func TestSettingsDefault_WakeDrivesForSMARTIsFalse(t *testing.T) {
	d := defaultSettings()
	if d.AdvancedScans.SMART.WakeDrives {
		t.Errorf("defaultSettings().AdvancedScans.SMART.WakeDrives must be false (standby-aware by default, issue #198)")
	}
}
