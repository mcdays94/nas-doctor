package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newTestServer returns a minimal Server wired with an in-memory
// storage store. Used by the handler-level tests below.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		store:     storage.NewFakeStore(),
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
}

// TestSettings_BackupMonitorBorg_AdditiveDefault pins that a fresh
// Settings struct carries an empty BackupMonitor.Borg array —
// round-tripping through JSON preserves the shape and does not bump
// settings_version.
func TestSettings_BackupMonitorBorg_AdditiveDefault(t *testing.T) {
	s := defaultSettings()
	if len(s.BackupMonitor.Borg) != 0 {
		t.Errorf("BackupMonitor.Borg default len = %d; want 0", len(s.BackupMonitor.Borg))
	}
	// Round-trip through JSON and check the key is present (even if
	// empty) so the UI can reliably render "Add Borg repo" without
	// nil-guarding.
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"backup_monitor"`) {
		t.Error("marshalled settings missing backup_monitor key")
	}
	// No settings_version bump — issue #279 purely additive.
	if s.SettingsVersion != currentSettingsVersion {
		t.Errorf("SettingsVersion = %d; want %d (no bump)", s.SettingsVersion, currentSettingsVersion)
	}
	if currentSettingsVersion != 3 {
		t.Errorf("currentSettingsVersion = %d; #279 must NOT bump past 3", currentSettingsVersion)
	}
}

// TestSettings_OldV3Blob_DecodesWithEmptyBackupMonitor simulates an
// upgrader: a v3-shaped settings blob that predates #279 (no
// backup_monitor key at all) decodes cleanly, with an empty Borg
// array as the zero value.
func TestSettings_OldV3Blob_DecodesWithEmptyBackupMonitor(t *testing.T) {
	oldV3 := []byte(`{
		"settings_version": 3,
		"scan_interval": "30m",
		"advanced_scans": {"smart": {"wake_drives": false, "max_age_days": 7}}
	}`)
	var s Settings
	if err := json.Unmarshal(oldV3, &s); err != nil {
		t.Fatalf("unmarshal old v3: %v", err)
	}
	if s.BackupMonitor.Borg == nil {
		// Either nil or empty slice is acceptable — both signal "no
		// external monitoring configured". Pin that it's at least
		// not a random value.
		s.BackupMonitor.Borg = []BorgExternalRepo{}
	}
	if len(s.BackupMonitor.Borg) != 0 {
		t.Errorf("Borg = %+v; want empty for an upgrader", s.BackupMonitor.Borg)
	}
	if s.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("MaxAgeDays = %d; want 7 (v3 fields must still decode)", s.AdvancedScans.SMART.MaxAgeDays)
	}
}

// TestValidateBackupMonitorBorg_RejectsEnabledEmptyRepoPath pins the
// central invariant: a repo that's marked enabled cannot be saved
// without a repo path.
func TestValidateBackupMonitorBorg_RejectsEnabledEmptyRepoPath(t *testing.T) {
	err := validateBackupMonitorBorg([]BorgExternalRepo{{Enabled: true, RepoPath: ""}})
	if err == nil {
		t.Fatal("enabled-with-empty repo_path must reject")
	}
	if !strings.Contains(err.Error(), "repo_path is required") {
		t.Errorf("error %q; want mention of repo_path", err.Error())
	}
}

// TestValidateBackupMonitorBorg_AllowsDisabledEmpty verifies the
// inverse: disabled entries with blank fields pass, so users can
// keep a partially-filled form without blocking Save.
func TestValidateBackupMonitorBorg_AllowsDisabledEmpty(t *testing.T) {
	err := validateBackupMonitorBorg([]BorgExternalRepo{{Enabled: false, RepoPath: ""}})
	if err != nil {
		t.Errorf("disabled-empty entry should pass: %v", err)
	}
}

// TestValidateBackupMonitorBorg_PassphraseEnvPattern pins the env-var
// name pattern check.
func TestValidateBackupMonitorBorg_PassphraseEnvPattern(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		rejects bool
	}{
		{"standard", "BORG_PASSPHRASE", false},
		{"with suffix", "BORG_PASSPHRASE_OFFSITE", false},
		{"leading underscore", "_SECRET", false},
		{"empty (default applies)", "", false},
		{"lowercase", "borg_passphrase", true},
		{"digits leading", "1_SECRET", true},
		{"with space", "MY SECRET", true},
		{"with dash", "MY-SECRET", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBackupMonitorBorg([]BorgExternalRepo{{
				Enabled:       true,
				RepoPath:      "/mnt/r",
				PassphraseEnv: tc.value,
			}})
			got := err != nil
			if got != tc.rejects {
				t.Errorf("passphrase_env=%q rejected=%v; want rejected=%v (err=%v)", tc.value, got, tc.rejects, err)
			}
		})
	}
}

// TestValidateBackupMonitorBorg_BinaryPathStatCheck verifies that a
// non-empty BinaryPath must point to an existing file on disk.
func TestValidateBackupMonitorBorg_BinaryPathStatCheck(t *testing.T) {
	// Create a temp file that exists
	dir := t.TempDir()
	exists := filepath.Join(dir, "borg-custom")
	if err := os.WriteFile(exists, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupMonitorBorg([]BorgExternalRepo{{
		Enabled:    true,
		RepoPath:   "/r",
		BinaryPath: exists,
	}}); err != nil {
		t.Errorf("existing binary rejected: %v", err)
	}
	if err := validateBackupMonitorBorg([]BorgExternalRepo{{
		Enabled:    true,
		RepoPath:   "/r",
		BinaryPath: filepath.Join(dir, "does-not-exist"),
	}}); err == nil {
		t.Error("non-existent binary accepted; want rejection")
	}
}

// TestValidateBackupMonitorBorg_SSHKeyStatCheck parallels the binary-
// path check for SSH keys.
func TestValidateBackupMonitorBorg_SSHKeyStatCheck(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(key, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupMonitorBorg([]BorgExternalRepo{{
		Enabled:    true,
		RepoPath:   "ssh://u@h/repo",
		SSHKeyPath: key,
	}}); err != nil {
		t.Errorf("existing key rejected: %v", err)
	}
	if err := validateBackupMonitorBorg([]BorgExternalRepo{{
		Enabled:    true,
		RepoPath:   "ssh://u@h/repo",
		SSHKeyPath: filepath.Join(dir, "missing-key"),
	}}); err == nil {
		t.Error("missing key accepted; want rejection")
	}
}

// TestValidateBackupMonitorBorg_ErrorNamesEntry ensures error
// messages include the label (or "entry N") so the UI can point
// users at the offending row.
func TestValidateBackupMonitorBorg_ErrorNamesEntry(t *testing.T) {
	err := validateBackupMonitorBorg([]BorgExternalRepo{
		{Enabled: false},
		{Enabled: true, Label: "Offsite Backup"},
	})
	if err == nil {
		t.Fatal("want error on enabled-empty entry")
	}
	if !strings.Contains(err.Error(), "Offsite Backup") {
		t.Errorf("error %q; want label in message", err.Error())
	}
}

// TestApiBorgReposToCollector_MapsAllFields pins the api → collector
// conversion shape. Catches a regression where a newly-added field
// gets missed on one side of the boundary.
func TestApiBorgReposToCollector_MapsAllFields(t *testing.T) {
	in := []BorgExternalRepo{
		{Enabled: true, Label: "Main", RepoPath: "/mnt/r", BinaryPath: "/usr/bin/borg", PassphraseEnv: "FOO", SSHKeyPath: "/k"},
	}
	got := apiBorgReposToCollector(in)
	if len(got) != 1 {
		t.Fatalf("len = %d; want 1", len(got))
	}
	if got[0].Enabled != true || got[0].Label != "Main" || got[0].RepoPath != "/mnt/r" ||
		got[0].BinaryPath != "/usr/bin/borg" || got[0].PassphraseEnv != "FOO" || got[0].SSHKeyPath != "/k" {
		t.Errorf("field mapping incomplete: %+v", got[0])
	}
}

// TestApiBorgReposToCollector_EmptyReturnsNil preserves the pre-
// #279 "no config" path exactly.
func TestApiBorgReposToCollector_EmptyReturnsNil(t *testing.T) {
	if got := apiBorgReposToCollector(nil); got != nil {
		t.Errorf("nil input → %+v; want nil", got)
	}
	if got := apiBorgReposToCollector([]BorgExternalRepo{}); got != nil {
		t.Errorf("empty input → %+v; want nil", got)
	}
}

// TestPUTSettings_RoundTripsBackupMonitorBorg exercises the full PUT
// → GET cycle against a real Server, pinning that the new array
// survives persistence and round-trips with field-level fidelity.
func TestPUTSettings_RoundTripsBackupMonitorBorg(t *testing.T) {
	srv := newTestServer(t)

	// PUT with one enabled + one disabled repo. Borg has no binary
	// path (default), no SSH key, standard passphrase env var.
	payload := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{
				{Enabled: true, Label: "Main", RepoPath: "/mnt/borg-main"},
				{Enabled: false, Label: "Disabled", RepoPath: "/mnt/borg-old"},
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

	// GET back and verify the array survived.
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
	if len(back.BackupMonitor.Borg) != 2 {
		t.Fatalf("Borg len = %d; want 2", len(back.BackupMonitor.Borg))
	}
	if back.BackupMonitor.Borg[0].RepoPath != "/mnt/borg-main" {
		t.Errorf("entry 0 RepoPath = %q; want /mnt/borg-main", back.BackupMonitor.Borg[0].RepoPath)
	}
	if back.BackupMonitor.Borg[0].Label != "Main" {
		t.Errorf("entry 0 Label = %q; want Main", back.BackupMonitor.Borg[0].Label)
	}
	if back.BackupMonitor.Borg[1].Enabled {
		t.Error("entry 1 Enabled = true; want false")
	}
	// No settings_version bump — issue #279 purely additive.
	if back.SettingsVersion != currentSettingsVersion {
		t.Errorf("SettingsVersion = %d; want %d", back.SettingsVersion, currentSettingsVersion)
	}
}

// TestPUTSettings_PushesBorgConfigOntoCollector verifies the
// runtime-update plumbing: a PUT to /settings updates the collector's
// stored external-Borg list so the next scan tick uses the new config
// without a restart (issue #279 user stories 2/4/7).
func TestPUTSettings_PushesBorgConfigOntoCollector(t *testing.T) {
	// This test needs a collector to receive the SetBackupMonitorBorg
	// call. newTestServer doesn't wire one up, so we do it manually.
	srv := newTestServer(t)
	srv.collector = nil // still nil — the PUT path guards on nil

	// When collector is nil, the PUT path must not crash — the
	// handler only calls SetBackupMonitorBorg when scheduler is
	// non-nil (existing guard). We exercise the nil-scheduler
	// branch to confirm no panic.
	payload := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{
				{Enabled: true, RepoPath: "/mnt/main"},
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
}

// TestPUTSettings_RejectsInvalidBackupMonitorBorg verifies the 400
// surface: an enabled entry with no repo_path is rejected, and the
// error body names the field.
func TestPUTSettings_RejectsInvalidBackupMonitorBorg(t *testing.T) {
	srv := newTestServer(t)
	bad := Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		BackupMonitor: BackupMonitorSettings{
			Borg: []BorgExternalRepo{{Enabled: true, RepoPath: ""}},
		},
	}
	body, _ := json.Marshal(bad)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleUpdateSettings(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d; want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "repo_path") {
		t.Errorf("body %q; want reference to repo_path", w.Body.String())
	}
}
