package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSettingsDefault_SMART_NestedDefaults guards the defaults for
// the new Settings.SMART sub-struct introduced by issue #237. Every
// fresh install (and every upgrader via the migration in getSettings)
// must land on these concrete defaults:
//   - WakeDrives = false (unchanged from v0.9.5 #198 standby-aware default)
//   - MaxAgeDays = 7     (new; safety-net threshold documented in PRD #236)
func TestSettingsDefault_SMART_NestedDefaults(t *testing.T) {
	d := defaultSettings()
	if d.SMART.WakeDrives {
		t.Errorf("defaultSettings().SMART.WakeDrives must be false (#198 standby-aware default)")
	}
	if d.SMART.MaxAgeDays != 7 {
		t.Errorf("defaultSettings().SMART.MaxAgeDays = %d, want 7 (PRD #236)", d.SMART.MaxAgeDays)
	}
}

// TestSettingsRoundTrip_SMART_Nested exercises PUT→GET for the new
// nested smart shape. The client sends `smart: {wake_drives: true,
// max_age_days: 14}` and the server must return the same values on
// the subsequent GET. No new scheduler behaviour fires in this slice
// (that's #238's scope) — this test only pins the schema contract.
func TestSettingsRoundTrip_SMART_Nested(t *testing.T) {
	srv := newSettingsTestServer()

	putBody := map[string]interface{}{
		"scan_interval": "30m",
		"theme":         "midnight",
		"smart": map[string]interface{}{
			"wake_drives":   true,
			"max_age_days":  14,
		},
	}
	buf, _ := json.Marshal(putBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleUpdateSettings(rec, req)
	if rec.Code != http.StatusOK {
		b, _ := io.ReadAll(rec.Body)
		t.Fatalf("PUT returned %d: %s", rec.Code, b)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec2 := httptest.NewRecorder()
	srv.handleGetSettings(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET returned %d", rec2.Code)
	}
	var got Settings
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse GET response: %v", err)
	}
	if !got.SMART.WakeDrives {
		t.Errorf("smart.wake_drives did not round-trip; got false, want true")
	}
	if got.SMART.MaxAgeDays != 14 {
		t.Errorf("smart.max_age_days did not round-trip; got %d, want 14", got.SMART.MaxAgeDays)
	}
}
