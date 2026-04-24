package api

import (
	"encoding/json"
	"testing"
)

// TestMigrateSettings_V2toV3_LiftsSMARTIntoAdvancedScans covers user
// story 11 of PRD #239: a user with slice-1 (#237) config gets
// WakeDrives and MaxAgeDays preserved verbatim across the v2→v3
// reshape that relocates them inside the new AdvancedScans umbrella.
//
// Input: v2 blob with the flat "smart": {...} object.
// Expected: v3-shape output with advanced_scans.smart.{wake_drives,
// max_age_days, interval_sec}. WakeDrives + MaxAgeDays preserve the
// input values; IntervalSec defaults to 0 ("use global"), since it
// did not exist in v2.
func TestMigrateSettings_V2toV3_LiftsSMARTIntoAdvancedScans(t *testing.T) {
	raw := []byte(`{
		"settings_version": 2,
		"scan_interval": "30m",
		"theme": "midnight",
		"smart": {
			"wake_drives": true,
			"max_age_days": 14
		}
	}`)

	got := migrateSettings(raw, defaultSettings())
	if got.SettingsVersion < currentSettingsVersion {
		got.SettingsVersion = currentSettingsVersion
	}

	if got.SettingsVersion != 3 {
		t.Errorf("settings_version: got %d, want 3", got.SettingsVersion)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (should be preserved from v2 smart block)")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 14 (should be preserved from v2 smart block)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if got.AdvancedScans.SMART.IntervalSec != 0 {
		t.Errorf("advanced_scans.smart.interval_sec: got %d, want 0 (new field seeded at 'use global')", got.AdvancedScans.SMART.IntervalSec)
	}
}

// TestMigrateSettings_V2toV3_SeedsAllSubsystemIntervalsToZero pins
// PRD #239 user story 10: every fresh slice-2 install and every
// upgrader lands on "use global" for all six configurable subsystems.
func TestMigrateSettings_V2toV3_SeedsAllSubsystemIntervalsToZero(t *testing.T) {
	raw := []byte(`{
		"settings_version": 2,
		"scan_interval": "30m",
		"theme": "midnight",
		"smart": {"wake_drives": false, "max_age_days": 7}
	}`)

	got := migrateSettings(raw, defaultSettings())

	if got.AdvancedScans.SMART.IntervalSec != 0 {
		t.Errorf("advanced_scans.smart.interval_sec: got %d, want 0", got.AdvancedScans.SMART.IntervalSec)
	}
	if got.AdvancedScans.Docker.IntervalSec != 0 {
		t.Errorf("advanced_scans.docker.interval_sec: got %d, want 0", got.AdvancedScans.Docker.IntervalSec)
	}
	if got.AdvancedScans.Proxmox.IntervalSec != 0 {
		t.Errorf("advanced_scans.proxmox.interval_sec: got %d, want 0", got.AdvancedScans.Proxmox.IntervalSec)
	}
	if got.AdvancedScans.Kubernetes.IntervalSec != 0 {
		t.Errorf("advanced_scans.kubernetes.interval_sec: got %d, want 0", got.AdvancedScans.Kubernetes.IntervalSec)
	}
	if got.AdvancedScans.ZFS.IntervalSec != 0 {
		t.Errorf("advanced_scans.zfs.interval_sec: got %d, want 0", got.AdvancedScans.ZFS.IntervalSec)
	}
	if got.AdvancedScans.GPU.IntervalSec != 0 {
		t.Errorf("advanced_scans.gpu.interval_sec: got %d, want 0", got.AdvancedScans.GPU.IntervalSec)
	}
}

// TestMigrateSettings_V1toV2toV3_ChainedIntegrity pins the
// complete upgrade path: a v1 blob climbs through both migration rungs
// and lands in a v3 shape with all slice-1 intent preserved.
// Covers the user stated in PRD #239 user story 11 layered on top of
// PRD #236 user story from slice 1.
func TestMigrateSettings_V1toV2toV3_ChainedIntegrity(t *testing.T) {
	raw := []byte(`{
		"settings_version": 1,
		"scan_interval": "30m",
		"theme": "midnight",
		"wake_drives_for_smart": true
	}`)

	got := migrateSettings(raw, defaultSettings())
	if got.SettingsVersion < currentSettingsVersion {
		got.SettingsVersion = currentSettingsVersion
	}

	if got.SettingsVersion != 3 {
		t.Errorf("settings_version: got %d, want 3 (full v1→v2→v3 climb)", got.SettingsVersion)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (lifted from legacy wake_drives_for_smart via v1→v2→v3 chain)")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 7 (seeded by v1→v2 then preserved through v2→v3)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if got.AdvancedScans.SMART.IntervalSec != 0 {
		t.Errorf("advanced_scans.smart.interval_sec: got %d, want 0", got.AdvancedScans.SMART.IntervalSec)
	}
}

// TestMigrateSettings_V3_Idempotent confirms a v3-shaped blob is
// passed through unchanged. Every subsequent read of the persisted
// config hits this path, so it must not mutate preserved values —
// including edge cases like explicit user zeros.
func TestMigrateSettings_V3_Idempotent(t *testing.T) {
	raw := []byte(`{
		"settings_version": 3,
		"scan_interval": "30m",
		"theme": "midnight",
		"advanced_scans": {
			"smart":      {"wake_drives": true, "max_age_days": 14, "interval_sec": 604800},
			"docker":     {"interval_sec": 300},
			"proxmox":    {"interval_sec": 7200},
			"kubernetes": {"interval_sec": 0},
			"zfs":        {"interval_sec": 7200},
			"gpu":        {"interval_sec": 0}
		}
	}`)

	got := migrateSettings(raw, defaultSettings())

	if got.SettingsVersion != 3 {
		t.Errorf("settings_version: got %d, want 3 preserved", got.SettingsVersion)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (should not be clobbered)")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 14 (should not be reseeded)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if got.AdvancedScans.SMART.IntervalSec != 604800 {
		t.Errorf("advanced_scans.smart.interval_sec: got %d, want 604800 (should not be reset)", got.AdvancedScans.SMART.IntervalSec)
	}
	if got.AdvancedScans.Docker.IntervalSec != 300 {
		t.Errorf("advanced_scans.docker.interval_sec: got %d, want 300", got.AdvancedScans.Docker.IntervalSec)
	}
	if got.AdvancedScans.Proxmox.IntervalSec != 7200 {
		t.Errorf("advanced_scans.proxmox.interval_sec: got %d, want 7200", got.AdvancedScans.Proxmox.IntervalSec)
	}
	if got.AdvancedScans.ZFS.IntervalSec != 7200 {
		t.Errorf("advanced_scans.zfs.interval_sec: got %d, want 7200", got.AdvancedScans.ZFS.IntervalSec)
	}
}

// TestMigrateSettings_V3_PreservesExplicitZeros is the direct
// descendant of the #268 regression guard for slice-2. A v3 blob
// whose user deliberately chose zero on a field (max_age_days=0 as
// "no safety net"; interval_sec=0 everywhere as "use global") must
// survive migrateSettings verbatim — no silent re-seeding on re-read.
//
// Companion to TestMigrateSettings_V2_PreservesMaxAgeDaysZero in
// settings_smart_migration_test.go. If this test fails we're
// re-introducing the class of data-loss bug that #268 fixed.
func TestMigrateSettings_V3_PreservesExplicitZeros(t *testing.T) {
	raw := []byte(`{
		"settings_version": 3,
		"scan_interval": "30m",
		"theme": "midnight",
		"advanced_scans": {
			"smart":      {"wake_drives": false, "max_age_days": 0, "interval_sec": 0},
			"docker":     {"interval_sec": 0},
			"proxmox":    {"interval_sec": 0},
			"kubernetes": {"interval_sec": 0},
			"zfs":        {"interval_sec": 0},
			"gpu":        {"interval_sec": 0}
		}
	}`)

	got := migrateSettings(raw, defaultSettings())

	if got.AdvancedScans.SMART.MaxAgeDays != 0 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 0 (user explicitly disabled safety net)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if got.AdvancedScans.SMART.IntervalSec != 0 {
		t.Errorf("advanced_scans.smart.interval_sec: got %d, want 0 preserved", got.AdvancedScans.SMART.IntervalSec)
	}
	if got.SettingsVersion != 3 {
		t.Errorf("settings_version: got %d, want 3 preserved (must not slip back to 0 and retrigger migrations — #268 class of bug)", got.SettingsVersion)
	}
}

// TestMigrateSettings_V3_ExplicitZeroVersionNotReClobbered pins the
// interaction between the new v3 shape and the #273 settings_version
// clamp. A blob whose on-disk settings_version is 0 (corrupted by a
// pre-#273 client) but that has otherwise-explicit v3 AdvancedScans
// fields set to zero values must NOT be silently repopulated by a
// re-migration. The caller (getSettings) stamps version back up to
// currentSettingsVersion after migrateSettings returns; migrateSettings
// itself should not re-seed user-chosen zeros even when the stored
// version looks old, provided the v3 shape is recognizable.
func TestMigrateSettings_V3_ExplicitZeroVersionNotReClobbered(t *testing.T) {
	// This is the post-#273 world: the client must send
	// settings_version; the server preserves it. But defence-in-depth
	// against future regressions — if a v3-shaped blob ever appears
	// with settings_version=0, we should treat it as v3 (the presence
	// of advanced_scans is the authoritative shape marker) and not
	// silently clobber the zero-values.
	raw := []byte(`{
		"settings_version": 0,
		"scan_interval": "30m",
		"theme": "midnight",
		"advanced_scans": {
			"smart":      {"wake_drives": true, "max_age_days": 0, "interval_sec": 0},
			"docker":     {"interval_sec": 0},
			"proxmox":    {"interval_sec": 0},
			"kubernetes": {"interval_sec": 0},
			"zfs":        {"interval_sec": 0},
			"gpu":        {"interval_sec": 0}
		}
	}`)

	got := migrateSettings(raw, defaultSettings())

	// max_age_days=0 must survive even though the version looks old;
	// the v2→v3 migration path should NOT fire because the
	// advanced_scans object already exists in the input.
	if got.AdvancedScans.SMART.MaxAgeDays != 0 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 0 (user explicitly disabled; must not be re-seeded)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (explicit user value must survive re-migration)")
	}
}

// TestGetSettings_V2toV3_PersistsMigration closes the loop end-to-end
// through the HTTP-facing getSettings() function: a stored v2 blob is
// transparently migrated to v3 on first read, and the persisted
// representation on the store is rewritten so subsequent reads skip
// the migration. Mirrors the slice-1 test
// TestGetSettings_V1toV2_PersistsMigration.
func TestGetSettings_V2toV3_PersistsMigration(t *testing.T) {
	srv := newSettingsTestServer()
	if err := srv.store.SetConfig(settingsConfigKey, `{
		"settings_version": 2,
		"scan_interval": "30m",
		"theme": "midnight",
		"smart": {
			"wake_drives": true,
			"max_age_days": 14
		}
	}`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	loaded := srv.getSettings()
	if loaded.SettingsVersion != 3 {
		t.Errorf("loaded settings_version: got %d, want 3", loaded.SettingsVersion)
	}
	if !loaded.AdvancedScans.SMART.WakeDrives {
		t.Errorf("loaded advanced_scans.smart.wake_drives: got false, want true")
	}
	if loaded.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("loaded advanced_scans.smart.max_age_days: got %d, want 14", loaded.AdvancedScans.SMART.MaxAgeDays)
	}

	// Subsequent read: the stored blob must now be v3-shaped.
	raw, err := srv.store.GetConfig(settingsConfigKey)
	if err != nil {
		t.Fatalf("re-read stored settings: %v", err)
	}
	var persisted map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		t.Fatalf("parse persisted settings: %v", err)
	}
	if v, _ := persisted["settings_version"].(float64); int(v) != 3 {
		t.Errorf("persisted settings_version: got %v, want 3 (migration must rewrite the store)", persisted["settings_version"])
	}
	advScans, ok := persisted["advanced_scans"].(map[string]interface{})
	if !ok {
		t.Fatalf("persisted settings missing advanced_scans object: %v", persisted["advanced_scans"])
	}
	smartMap, ok := advScans["smart"].(map[string]interface{})
	if !ok {
		t.Fatalf("persisted advanced_scans missing smart object: %v", advScans["smart"])
	}
	if wd, _ := smartMap["wake_drives"].(bool); !wd {
		t.Errorf("persisted advanced_scans.smart.wake_drives: got %v, want true", smartMap["wake_drives"])
	}
	if mad, _ := smartMap["max_age_days"].(float64); int(mad) != 14 {
		t.Errorf("persisted advanced_scans.smart.max_age_days: got %v, want 14", smartMap["max_age_days"])
	}
}

// TestSettingsDefault_AdvancedScans_NestedDefaults pins the defaults
// for the new Settings.AdvancedScans sub-struct (PRD #239). Fresh
// installs land on IntervalSec=0 for every configurable subsystem
// (PRD user story 10) and keep slice-1's WakeDrives=false,
// MaxAgeDays=7 SMART defaults.
func TestSettingsDefault_AdvancedScans_NestedDefaults(t *testing.T) {
	d := defaultSettings()
	if d.SettingsVersion != 3 {
		t.Errorf("defaultSettings().SettingsVersion = %d, want 3 (bumped by slice 2 PRD #239)", d.SettingsVersion)
	}
	if d.AdvancedScans.SMART.WakeDrives {
		t.Errorf("defaultSettings().AdvancedScans.SMART.WakeDrives must be false (#198 standby-aware default)")
	}
	if d.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("defaultSettings().AdvancedScans.SMART.MaxAgeDays = %d, want 7 (slice 1 default, preserved across v2→v3)", d.AdvancedScans.SMART.MaxAgeDays)
	}
	if d.AdvancedScans.SMART.IntervalSec != 0 {
		t.Errorf("defaultSettings().AdvancedScans.SMART.IntervalSec = %d, want 0 (use global)", d.AdvancedScans.SMART.IntervalSec)
	}
	subsystems := map[string]int{
		"docker":     d.AdvancedScans.Docker.IntervalSec,
		"proxmox":    d.AdvancedScans.Proxmox.IntervalSec,
		"kubernetes": d.AdvancedScans.Kubernetes.IntervalSec,
		"zfs":        d.AdvancedScans.ZFS.IntervalSec,
		"gpu":        d.AdvancedScans.GPU.IntervalSec,
	}
	for name, v := range subsystems {
		if v != 0 {
			t.Errorf("defaultSettings().AdvancedScans.%s.IntervalSec = %d, want 0", name, v)
		}
	}
}
