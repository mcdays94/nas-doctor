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
// template ships the Advanced section with the wake-drives toggle +
// disclaimer required by issue #198.
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
		// Advanced card anchor so the sticky section nav can link to it.
		{"advanced card anchor", `id="card-advanced"`},
		// Section nav link to the advanced card.
		{"advanced nav link", `href="#card-advanced"`},
		// Disclosure element — using <details>/<summary> per the issue's
		// guidance (no extra JS needed).
		{"details element", `<details`},
		{"summary element", `<summary`},
		// The toggle control + its stable id for load/save wiring.
		{"wake-drives toggle id", `id="wake-drives-for-smart"`},
		// Load path reads the JSON field name.
		{"load binds field", `data.wake_drives_for_smart`},
		// Save payload writes the JSON field name.
		{"save sends field", `wake_drives_for_smart:`},
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
// the new setting to make sure it persists and is returned by handleGetSettings.
func TestSettingsRoundTrip_WakeDrivesForSMART(t *testing.T) {
	s := newSettingsTestServer()

	// PUT enabling the flag.
	put := Settings{
		ScanInterval:       "30m",
		Theme:              ThemeMidnight,
		WakeDrivesForSMART: true,
	}
	body, _ := json.Marshal(put)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleUpdateSettings(rr, req)
	if rr.Code != http.StatusOK {
		b, _ := io.ReadAll(rr.Body)
		t.Fatalf("PUT returned %d: %s", rr.Code, b)
	}

	// GET and verify the flag survived the round-trip.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr2 := httptest.NewRecorder()
	s.handleGetSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET returned %d", rr2.Code)
	}
	var got Settings
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse GET response: %v", err)
	}
	if !got.WakeDrivesForSMART {
		t.Errorf("WakeDrivesForSMART did not round-trip; got false, wanted true")
	}
}

// TestSettingsDefault_WakeDrivesForSMARTIsFalse guards against a regression
// where the default flips; the whole point of #198 is that default=false
// means drives in standby are NOT woken by SMART scans.
func TestSettingsDefault_WakeDrivesForSMARTIsFalse(t *testing.T) {
	d := defaultSettings()
	if d.WakeDrivesForSMART {
		t.Errorf("defaultSettings().WakeDrivesForSMART must be false (standby-aware by default, issue #198)")
	}
}
