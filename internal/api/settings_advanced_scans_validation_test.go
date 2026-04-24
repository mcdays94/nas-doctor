package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleUpdateSettings_AdvancedScans_IntervalSec_RangeRejected
// pins server-side validation for advanced_scans.*.interval_sec.
// Valid set: {0} ∪ [30, 2678400] (0 = "use global", 30s = minimum
// positive cadence to keep the scheduler tick sensible, 2678400 = 31
// days = effectively-disabled ceiling). Per PRD #239 user story 13.
//
// The UI clamps client-side but a direct API caller could still
// submit out-of-range values; the server must reject with 400 and a
// clear message that names the offending field + the valid range.
func TestHandleUpdateSettings_AdvancedScans_IntervalSec_RangeRejected(t *testing.T) {
	cases := []struct {
		name        string
		subsystem   string
		intervalSec int
		wantStatus  int
	}{
		{"zero is valid (use global)", "docker", 0, http.StatusOK},
		{"thirty is the minimum positive", "docker", 30, http.StatusOK},
		{"five minutes", "docker", 300, http.StatusOK},
		{"one day", "docker", 86400, http.StatusOK},
		{"31 days (maximum)", "docker", 2678400, http.StatusOK},
		{"one second rejected (below 30)", "docker", 1, http.StatusBadRequest},
		{"twenty-nine seconds rejected (below 30)", "docker", 29, http.StatusBadRequest},
		{"31 days plus one rejected", "docker", 2678401, http.StatusBadRequest},
		{"one year rejected", "docker", 31536000, http.StatusBadRequest},
		{"negative rejected", "docker", -1, http.StatusBadRequest},
		// Exercise SMART (has extra fields besides IntervalSec but
		// the IntervalSec bound check applies identically).
		{"smart: out of range rejected", "smart", 29, http.StatusBadRequest},
		{"smart: valid 7-day interval", "smart", 604800, http.StatusOK},
		// Exercise all 6 subsystems at a valid value to confirm
		// every field gets validated.
		{"proxmox: valid 1-hour interval", "proxmox", 3600, http.StatusOK},
		{"kubernetes: out-of-range rejected", "kubernetes", 10, http.StatusBadRequest},
		{"zfs: valid 2-hour interval", "zfs", 7200, http.StatusOK},
		{"gpu: out-of-range rejected", "gpu", 2678401, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSettingsTestServer()
			scans := map[string]interface{}{
				"smart":      map[string]interface{}{"wake_drives": false, "max_age_days": 7, "interval_sec": 0},
				"docker":     map[string]interface{}{"interval_sec": 0},
				"proxmox":    map[string]interface{}{"interval_sec": 0},
				"kubernetes": map[string]interface{}{"interval_sec": 0},
				"zfs":        map[string]interface{}{"interval_sec": 0},
				"gpu":        map[string]interface{}{"interval_sec": 0},
			}
			// Overwrite the relevant subsystem with the test value.
			sub := scans[tc.subsystem].(map[string]interface{})
			sub["interval_sec"] = tc.intervalSec
			scans[tc.subsystem] = sub

			body, _ := json.Marshal(map[string]interface{}{
				"settings_version": 3,
				"scan_interval":    "30m",
				"theme":            "midnight",
				"advanced_scans":   scans,
			})
			req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.handleUpdateSettings(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("%s=%d returned %d, want %d; body=%s",
					tc.subsystem, tc.intervalSec, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest {
				lower := strings.ToLower(rec.Body.String())
				// Error message must name the offending subsystem and
				// the field so users know exactly which input to fix.
				if !strings.Contains(lower, "interval_sec") {
					t.Errorf("400 body should name interval_sec; got: %s", rec.Body.String())
				}
				if !strings.Contains(lower, tc.subsystem) {
					t.Errorf("400 body should name subsystem %q; got: %s", tc.subsystem, rec.Body.String())
				}
			}
		})
	}
}

// TestHandleUpdateSettings_AdvancedScans_RoundTrip exercises PUT→GET
// for the full AdvancedScans shape. Non-default values for every
// subsystem must survive the round trip intact.
func TestHandleUpdateSettings_AdvancedScans_RoundTrip(t *testing.T) {
	srv := newSettingsTestServer()

	put := map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": true, "max_age_days": 14, "interval_sec": 604800},
			"docker":     map[string]interface{}{"interval_sec": 300},
			"proxmox":    map[string]interface{}{"interval_sec": 3600},
			"kubernetes": map[string]interface{}{"interval_sec": 7200},
			"zfs":        map[string]interface{}{"interval_sec": 7200},
			"gpu":        map[string]interface{}{"interval_sec": 0},
		},
	}
	body, _ := json.Marshal(put)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleUpdateSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT returned %d: %s", rr.Code, rr.Body.String())
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
		t.Errorf("advanced_scans.smart.wake_drives did not round-trip")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("advanced_scans.smart.max_age_days did not round-trip; got %d", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if got.AdvancedScans.SMART.IntervalSec != 604800 {
		t.Errorf("advanced_scans.smart.interval_sec did not round-trip; got %d", got.AdvancedScans.SMART.IntervalSec)
	}
	if got.AdvancedScans.Docker.IntervalSec != 300 {
		t.Errorf("advanced_scans.docker.interval_sec did not round-trip; got %d", got.AdvancedScans.Docker.IntervalSec)
	}
	if got.AdvancedScans.Proxmox.IntervalSec != 3600 {
		t.Errorf("advanced_scans.proxmox.interval_sec did not round-trip; got %d", got.AdvancedScans.Proxmox.IntervalSec)
	}
	if got.AdvancedScans.Kubernetes.IntervalSec != 7200 {
		t.Errorf("advanced_scans.kubernetes.interval_sec did not round-trip; got %d", got.AdvancedScans.Kubernetes.IntervalSec)
	}
	if got.AdvancedScans.ZFS.IntervalSec != 7200 {
		t.Errorf("advanced_scans.zfs.interval_sec did not round-trip; got %d", got.AdvancedScans.ZFS.IntervalSec)
	}
	if got.AdvancedScans.GPU.IntervalSec != 0 {
		t.Errorf("advanced_scans.gpu.interval_sec did not round-trip; got %d", got.AdvancedScans.GPU.IntervalSec)
	}
}

// TestHandleUpdateSettings_AdvancedScans_InvalidDoesNotPersist ensures
// an out-of-range PUT does NOT partially persist. Subsequent GET must
// see the pre-edit state (mirrors the SMART.MaxAgeDays equivalent).
func TestHandleUpdateSettings_AdvancedScans_InvalidDoesNotPersist(t *testing.T) {
	srv := newSettingsTestServer()

	good, _ := json.Marshal(map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": false, "max_age_days": 7, "interval_sec": 0},
			"docker":     map[string]interface{}{"interval_sec": 300},
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

	bad, _ := json.Marshal(map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": false, "max_age_days": 7, "interval_sec": 0},
			"docker":     map[string]interface{}{"interval_sec": 5},
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
	if got.AdvancedScans.Docker.IntervalSec != 300 {
		t.Errorf("docker.interval_sec leaked from rejected PUT: got %d, want 300 (seeded value)", got.AdvancedScans.Docker.IntervalSec)
	}
}
