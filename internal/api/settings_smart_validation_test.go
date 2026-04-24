package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleUpdateSettings_SMART_MaxAgeDays_RangeRejected pins the
// server-side validation for smart.max_age_days. The UI clamps
// client-side (min=0 max=30) but a caller using the HTTP API
// directly could still submit out-of-range values — the server must
// reject them with 400 rather than silently clamping.
//
// Valid range: 0-30 inclusive. 0 disables the safety net (per PRD
// user story 5); 31+ and negative values are invalid input.
func TestHandleUpdateSettings_SMART_MaxAgeDays_RangeRejected(t *testing.T) {
	cases := []struct {
		name       string
		maxAgeDays int
		wantStatus int
	}{
		{"zero allowed (disables safety net)", 0, http.StatusOK},
		{"one allowed (maximum caution)", 1, http.StatusOK},
		{"seven is the default", 7, http.StatusOK},
		{"thirty is the ceiling", 30, http.StatusOK},
		{"thirty-one rejected", 31, http.StatusBadRequest},
		{"one hundred rejected", 100, http.StatusBadRequest},
		{"negative rejected", -1, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSettingsTestServer()
			body, _ := json.Marshal(map[string]interface{}{
				"settings_version": 3,
				"scan_interval":    "30m",
				"theme":            "midnight",
				"advanced_scans": map[string]interface{}{
					"smart":      map[string]interface{}{"wake_drives": false, "max_age_days": tc.maxAgeDays, "interval_sec": 0},
					"docker":     map[string]interface{}{"interval_sec": 0},
					"proxmox":    map[string]interface{}{"interval_sec": 0},
					"kubernetes": map[string]interface{}{"interval_sec": 0},
					"zfs":        map[string]interface{}{"interval_sec": 0},
					"gpu":        map[string]interface{}{"interval_sec": 0},
				},
			})
			req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.handleUpdateSettings(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("max_age_days=%d returned %d, want %d; body=%s",
					tc.maxAgeDays, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest {
				lower := strings.ToLower(rec.Body.String())
				if !strings.Contains(lower, "max_age_days") {
					t.Errorf("400 error body should name the offending field; got: %s", rec.Body.String())
				}
			}
		})
	}
}

// TestHandleUpdateSettings_SMART_MaxAgeDays_InvalidDoesNotPersist
// ensures an out-of-range PUT does NOT partially persist anything:
// a subsequent GET must still see the pre-edit state. This is a
// standard "rejected input leaves the store untouched" guarantee.
func TestHandleUpdateSettings_SMART_MaxAgeDays_InvalidDoesNotPersist(t *testing.T) {
	srv := newSettingsTestServer()

	// First, persist a known-good state.
	good, _ := json.Marshal(map[string]interface{}{
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
	})
	rec := httptest.NewRecorder()
	srv.handleUpdateSettings(rec, httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(good)))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed PUT failed: %d %s", rec.Code, rec.Body.String())
	}

	// Submit invalid max_age_days=99.
	bad, _ := json.Marshal(map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": false, "max_age_days": 99, "interval_sec": 0},
			"docker":     map[string]interface{}{"interval_sec": 0},
			"proxmox":    map[string]interface{}{"interval_sec": 0},
			"kubernetes": map[string]interface{}{"interval_sec": 0},
			"zfs":        map[string]interface{}{"interval_sec": 0},
			"gpu":        map[string]interface{}{"interval_sec": 0},
		},
	})
	rec2 := httptest.NewRecorder()
	srv.handleUpdateSettings(rec2, httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(bad)))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT must return 400, got %d: %s", rec2.Code, rec2.Body.String())
	}

	// GET and confirm nothing leaked through.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec3 := httptest.NewRecorder()
	srv.handleGetSettings(rec3, req)
	if rec3.Code != http.StatusOK {
		t.Fatalf("GET failed: %d", rec3.Code)
	}
	var got Settings
	if err := json.Unmarshal(rec3.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse GET: %v", err)
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("max_age_days leaked from rejected PUT: got %d, want 14 (seeded value)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("wake_drives leaked from rejected PUT: got false, want true (seeded value)")
	}
}
