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
	"time"

	"github.com/mcdays94/nas-doctor/internal/scheduler"
)

// Issue #180 — "Disabled" option in the speed-test interval dropdown.
//
// These tests assert the UI + wire-level + handler contracts that
// complement the scheduler-side sentinel:
//
//   - settings.html renders a <option value="disabled">Disabled</option>
//     inside the speedtest-interval dropdown.
//   - parseSpeedTestInterval("disabled") returns the scheduler sentinel.
//   - The Settings PUT/GET round-trip persists speedtest_interval="disabled".
//   - The default Settings value is NOT "disabled" on a fresh install
//     (strictly opt-in; existing users keep the 4-hour default on upgrade).

// TestSettingsHTMLHasDisabledOption verifies the speed-test interval
// dropdown includes a Disabled option wired with the JSON string
// sentinel "disabled". This is a template cross-reference test: without
// it, the wire format and the dropdown markup could drift apart silently.
func TestSettingsHTMLHasDisabledOption(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	// The dropdown block should contain <option value="disabled">Disabled</option>.
	// Use case-exact matching so a future typo (e.g. "Disable", "off")
	// is caught immediately.
	if !strings.Contains(content, `value="disabled"`) {
		t.Error(`settings.html missing option value="disabled" on the speed-test interval dropdown (issue #180)`)
	}
	if !strings.Contains(content, `>Disabled<`) {
		t.Error(`settings.html missing visible label ">Disabled<" on the speed-test interval dropdown (issue #180)`)
	}

	// The existing options must all still be present — no regression in
	// the dropdown's existing contract.
	for _, val := range []string{`value="30m"`, `value="1h"`, `value="4h"`, `value="24h"`, `value="weekly"`, `value="monthly"`} {
		if !strings.Contains(content, val) {
			t.Errorf("settings.html no longer contains existing dropdown option %q", val)
		}
	}
}

// TestParseSpeedTestInterval_Disabled asserts the helper that converts
// the wire-level string to a scheduler Duration returns the sentinel
// for the "disabled" string, and a normal parsed duration for valid
// time strings.
func TestParseSpeedTestInterval_Disabled(t *testing.T) {
	d, ok := parseSpeedTestInterval("disabled")
	if !ok {
		t.Fatal(`parseSpeedTestInterval("disabled") returned ok=false; want true`)
	}
	if d != scheduler.SpeedTestIntervalDisabled {
		t.Errorf(`parseSpeedTestInterval("disabled") = %v; want SpeedTestIntervalDisabled (%v)`, d, scheduler.SpeedTestIntervalDisabled)
	}
}

func TestParseSpeedTestInterval_Duration(t *testing.T) {
	d, ok := parseSpeedTestInterval("4h")
	if !ok {
		t.Fatal(`parseSpeedTestInterval("4h") returned ok=false; want true`)
	}
	if d != 4*time.Hour {
		t.Errorf(`parseSpeedTestInterval("4h") = %v; want 4h`, d)
	}
}

func TestParseSpeedTestInterval_Invalid(t *testing.T) {
	// Keyword values like "weekly"/"monthly" are interpreted by
	// SetSpeedTestSchedule (not SetSpeedTestInterval), so the parser
	// reports them as not-a-duration rather than falsely accepting them.
	if _, ok := parseSpeedTestInterval("weekly"); ok {
		t.Error(`parseSpeedTestInterval("weekly") returned ok=true; want false (keywords are handled by the schedule path)`)
	}
	if _, ok := parseSpeedTestInterval(""); ok {
		t.Error(`parseSpeedTestInterval("") returned ok=true; want false`)
	}
	if _, ok := parseSpeedTestInterval("garbage"); ok {
		t.Error(`parseSpeedTestInterval("garbage") returned ok=true; want false`)
	}
}

// TestSettingsRoundTrip_SpeedTestDisabled exercises the GET/PUT cycle
// to make sure the "disabled" wire value persists and is returned by
// handleGetSettings. Without this, a user picking Disabled would save
// the value, see it reload, but the scheduler would never learn.
func TestSettingsRoundTrip_SpeedTestDisabled(t *testing.T) {
	s := newSettingsTestServer()

	put := Settings{
		ScanInterval:      "30m",
		Theme:             ThemeMidnight,
		SpeedTestInterval: "disabled",
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
	if got.SpeedTestInterval != "disabled" {
		t.Errorf(`SpeedTestInterval did not round-trip; got %q, want "disabled"`, got.SpeedTestInterval)
	}
}

// TestDefaultSettings_SpeedTestNotDisabled guards against a regression
// where the default flips to Disabled. Fresh installs and upgrades must
// keep whatever the current standalone-loop default is (nil/empty at
// the Settings layer; the scheduler supplies its own 4h default).
func TestDefaultSettings_SpeedTestNotDisabled(t *testing.T) {
	d := defaultSettings()
	if d.SpeedTestInterval == "disabled" {
		t.Error(`defaultSettings().SpeedTestInterval = "disabled"; want empty or a positive duration (issue #180 is opt-in)`)
	}
}
