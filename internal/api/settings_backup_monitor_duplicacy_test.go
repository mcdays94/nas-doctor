package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSettings_BackupMonitorDuplicacy_AdditiveDefault pins that a
// fresh Settings struct carries a nil Duplicacy slice — additive
// only, no settings_version bump (PRD #310 §3 / issue #311).
func TestSettings_BackupMonitorDuplicacy_AdditiveDefault(t *testing.T) {
	s := defaultSettings()
	if len(s.BackupMonitor.Duplicacy) != 0 {
		t.Errorf("Duplicacy default len = %d; want 0", len(s.BackupMonitor.Duplicacy))
	}
	// settings_version must not bump when Duplicacy is added.
	// Mirrors the assertion in the Borg additive default test.
	if s.SettingsVersion != currentSettingsVersion {
		t.Errorf("SettingsVersion = %d; want %d (no bump)", s.SettingsVersion, currentSettingsVersion)
	}
	if currentSettingsVersion != 3 {
		t.Errorf("currentSettingsVersion = %d; #311 must NOT bump past 3", currentSettingsVersion)
	}
}

// TestSettings_OldV3Blob_DecodesWithEmptyDuplicacy simulates a
// v0.9.x → v0.10.0 upgrader: a v3-shaped settings blob that
// predates issue #311 (no `duplicacy` key under backup_monitor)
// must decode cleanly with a nil/empty Duplicacy slice.
func TestSettings_OldV3Blob_DecodesWithEmptyDuplicacy(t *testing.T) {
	oldV3 := []byte(`{
		"settings_version": 3,
		"scan_interval": "30m",
		"backup_monitor": {"borg": []},
		"advanced_scans": {"smart": {"wake_drives": false, "max_age_days": 7}}
	}`)
	var s Settings
	if err := json.Unmarshal(oldV3, &s); err != nil {
		t.Fatalf("unmarshal old v3: %v", err)
	}
	if len(s.BackupMonitor.Duplicacy) != 0 {
		t.Errorf("Duplicacy = %+v; want nil/empty for an upgrader", s.BackupMonitor.Duplicacy)
	}
	// Existing Borg field must still decode untouched.
	if s.BackupMonitor.Borg == nil {
		t.Error("Borg = nil; want []")
	}
}

// TestPUTSettings_RoundTripsBackupMonitorDuplicacy exercises the
// full PUT → GET cycle: a Duplicacy array with one cli-repo + one
// web-cache entry survives persistence with field-level fidelity.
// Mirrors TestPUTSettings_RoundTripsBackupMonitorBorg.
func TestPUTSettings_RoundTripsBackupMonitorDuplicacy(t *testing.T) {
	srv := newTestServer(t)

	payload := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		BackupMonitor: BackupMonitorSettings{
			Duplicacy: []DuplicacyEntry{
				{
					Enabled:    true,
					Label:      "Documents",
					Kind:       "cli-repo",
					Path:       "/mnt/duplicacy/documents",
					StaleAfter: 0, // default-30 — leaving zero must round-trip as zero
				},
				{
					Enabled:    true,
					Label:      "Media via duplicacy-web",
					Kind:       "web-cache",
					Path:       "/cache/localhost/0",
					StorageID:  "media-storage",
					StaleAfter: 14,
				},
				{
					Enabled: false,
					Label:   "Disabled",
					Kind:    "cli-repo",
					Path:    "/mnt/duplicacy/old",
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
		rb, _ := io.ReadAll(w.Body)
		t.Fatalf("PUT status = %d; body=%s", w.Code, rb)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getW := httptest.NewRecorder()
	srv.handleGetSettings(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getW.Code)
	}
	var back Settings
	if err := json.Unmarshal(getW.Body.Bytes(), &back); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(back.BackupMonitor.Duplicacy) != 3 {
		t.Fatalf("Duplicacy len = %d; want 3", len(back.BackupMonitor.Duplicacy))
	}
	d0 := back.BackupMonitor.Duplicacy[0]
	if d0.Label != "Documents" || d0.Kind != "cli-repo" || d0.Path != "/mnt/duplicacy/documents" {
		t.Errorf("entry 0 = %+v; field mapping incomplete", d0)
	}
	if d0.StaleAfter != 0 {
		t.Errorf("entry 0 StaleAfter = %d; want 0 (zero-value must round-trip)", d0.StaleAfter)
	}
	d1 := back.BackupMonitor.Duplicacy[1]
	if d1.Kind != "web-cache" || d1.StorageID != "media-storage" || d1.StaleAfter != 14 {
		t.Errorf("entry 1 = %+v; web-cache fields not preserved", d1)
	}
	d2 := back.BackupMonitor.Duplicacy[2]
	if d2.Enabled {
		t.Errorf("entry 2 Enabled = true; want false")
	}
	if back.SettingsVersion != currentSettingsVersion {
		t.Errorf("SettingsVersion = %d; want %d", back.SettingsVersion, currentSettingsVersion)
	}
}

// TestPUTSettings_DuplicacyAndBorgCoexist confirms the namespacing
// invariant: configuring Duplicacy entries does NOT clobber any
// existing Borg config in the same blob.
func TestPUTSettings_DuplicacyAndBorgCoexist(t *testing.T) {
	srv := newTestServer(t)

	payload := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{
				{Enabled: true, Label: "Offsite Borg", RepoPath: "/mnt/borg/offsite"},
			},
			Duplicacy: []DuplicacyEntry{
				{Enabled: true, Label: "Documents", Kind: "cli-repo", Path: "/mnt/dup/documents"},
			},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleUpdateSettings(w, req)
	if w.Code != http.StatusOK {
		rb, _ := io.ReadAll(w.Body)
		t.Fatalf("PUT status = %d; body=%s", w.Code, rb)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	getW := httptest.NewRecorder()
	srv.handleGetSettings(getW, getReq)
	var back Settings
	if err := json.Unmarshal(getW.Body.Bytes(), &back); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(back.BackupMonitor.Borg) != 1 || back.BackupMonitor.Borg[0].RepoPath != "/mnt/borg/offsite" {
		t.Errorf("Borg side dropped/clobbered; got %+v", back.BackupMonitor.Borg)
	}
	if len(back.BackupMonitor.Duplicacy) != 1 || back.BackupMonitor.Duplicacy[0].Path != "/mnt/dup/documents" {
		t.Errorf("Duplicacy side dropped/clobbered; got %+v", back.BackupMonitor.Duplicacy)
	}
}

// TestSettings_DuplicacyEmptyVsNil pins the JSON-shape contract:
// an explicitly-empty array round-trips as omitted (omitempty),
// and a nil slice marshals identically. V1b form-handling and
// future migration code can rely on this.
func TestSettings_DuplicacyEmptyVsNil(t *testing.T) {
	t.Run("nil slice — omitempty drops the key", func(t *testing.T) {
		s := BackupMonitorSettings{Borg: []BorgExternalRepo{}, Duplicacy: nil}
		raw, _ := json.Marshal(s)
		// omitempty drops nil slices; the duplicacy key should not
		// appear at all.
		if strings.Contains(string(raw), `"duplicacy"`) {
			t.Errorf("nil slice marshalled with key present: %s", raw)
		}
	})
	t.Run("explicit empty slice — omitempty also drops", func(t *testing.T) {
		s := BackupMonitorSettings{Duplicacy: []DuplicacyEntry{}}
		raw, _ := json.Marshal(s)
		// Go's encoding/json omitempty for a length-0 slice drops
		// the field. Both nil and []T{} map to "no entries" on
		// the wire. Pin this so V1b form-handling doesn't have
		// to disambiguate.
		if strings.Contains(string(raw), `"duplicacy"`) {
			t.Errorf("empty slice marshalled with key present: %s", raw)
		}
	})
	t.Run("populated — key appears with array", func(t *testing.T) {
		s := BackupMonitorSettings{Duplicacy: []DuplicacyEntry{{Enabled: true, Kind: "cli-repo", Path: "/x"}}}
		raw, _ := json.Marshal(s)
		if !strings.Contains(string(raw), `"duplicacy"`) {
			t.Errorf("populated slice missing key: %s", raw)
		}
	})
}
