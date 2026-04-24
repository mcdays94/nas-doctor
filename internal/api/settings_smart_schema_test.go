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
// the SMART sub-struct. Originally introduced by issue #237 under
// Settings.SMART; relocated to Settings.AdvancedScans.SMART by #259.
// Every fresh install (and every upgrader via the migration in
// getSettings) must land on these concrete defaults:
//   - WakeDrives = false (unchanged from v0.9.5 #198 standby-aware default)
//   - MaxAgeDays = 7     (safety-net threshold documented in PRD #236)
func TestSettingsDefault_SMART_NestedDefaults(t *testing.T) {
	d := defaultSettings()
	if d.AdvancedScans.SMART.WakeDrives {
		t.Errorf("defaultSettings().AdvancedScans.SMART.WakeDrives must be false (#198 standby-aware default)")
	}
	if d.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("defaultSettings().AdvancedScans.SMART.MaxAgeDays = %d, want 7 (PRD #236)", d.AdvancedScans.SMART.MaxAgeDays)
	}
}

// TestSettingsRoundTrip_SMART_Nested exercises PUT→GET for the
// advanced_scans.smart wire shape (v3 schema, #259). The client
// sends the nested shape; the server must return matching values
// on the subsequent GET. No new scheduler behaviour fires in this
// slice — this test only pins the schema contract.
func TestSettingsRoundTrip_SMART_Nested(t *testing.T) {
	srv := newSettingsTestServer()

	putBody := map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": true, "max_age_days": 14, "interval_sec": 0},
			"docker":     map[string]interface{}{"interval_sec": 0},
			"proxmox":    map[string]interface{}{"interval_sec": 0},
			"kubernetes": map[string]interface{}{"interval_sec": 0},
			"zfs":        map[string]interface{}{"interval_sec": 0},
			"gpu":        map[string]interface{}{"interval_sec": 0},
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
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives did not round-trip; got false, want true")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("advanced_scans.smart.max_age_days did not round-trip; got %d, want 14", got.AdvancedScans.SMART.MaxAgeDays)
	}
}
