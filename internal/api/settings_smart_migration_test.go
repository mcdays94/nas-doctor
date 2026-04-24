package api

import (
	"encoding/json"
	"testing"
)

// TestMigrateSettings_V1toV2_LiftsWakeDrivesForSMART covers user story
// 11 of PRD #236: the user's existing wake_drives_for_smart preference
// is automatically preserved across the schema migration.
//
// Input: v1 blob with the legacy flat wake_drives_for_smart=true field.
// Expected: WakeDrives=true on the target SMART sub-struct (v2 put
// this under Settings.SMART; v3 moves it to Settings.AdvancedScans.SMART
// via the v2→v3 ladder — see #259). Asserts the final v3 shape here.
func TestMigrateSettings_V1toV2_LiftsWakeDrivesForSMART(t *testing.T) {
	raw := []byte(`{
		"settings_version": 1,
		"scan_interval": "30m",
		"theme": "midnight",
		"wake_drives_for_smart": true
	}`)

	got := migrateSettings(raw, defaultSettings())
	// Caller (getSettings) is responsible for stamping the new version
	// number; migrateSettings only populates the struct. Simulate that.
	if got.SettingsVersion < currentSettingsVersion {
		got.SettingsVersion = currentSettingsVersion
	}

	if got.SettingsVersion != currentSettingsVersion {
		t.Errorf("settings_version: got %d, want %d", got.SettingsVersion, currentSettingsVersion)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (lifted from wake_drives_for_smart)")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 7 (PRD #236 user story 2 default)", got.AdvancedScans.SMART.MaxAgeDays)
	}
}

// TestMigrateSettings_V1toV2_SeedsMaxAgeDaysForOptedOutUpgrader covers
// the case where the v1 user had wake_drives_for_smart=false (the
// default post-v0.9.5). The migration must still seed max_age_days=7
// because the safety net is a distinct concept from the opt-in
// wake-drives behaviour.
func TestMigrateSettings_V1toV2_SeedsMaxAgeDaysForOptedOutUpgrader(t *testing.T) {
	raw := []byte(`{
		"settings_version": 1,
		"scan_interval": "30m",
		"theme": "midnight"
	}`)

	got := migrateSettings(raw, defaultSettings())

	if got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got true, want false (no legacy opt-in present)")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 7 (seeded for every upgrader)", got.AdvancedScans.SMART.MaxAgeDays)
	}
}

// TestMigrateSettings_V2_Idempotent confirms that a v2-shaped blob is
// passed through and lifted onto the v3 shape: WakeDrives and
// MaxAgeDays values survive the v2→v3 relocation.
func TestMigrateSettings_V2_Idempotent(t *testing.T) {
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

	if got.SettingsVersion != currentSettingsVersion {
		t.Errorf("settings_version: got %d, want %d", got.SettingsVersion, currentSettingsVersion)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (should not be clobbered by v2→v3 relocation)")
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 14 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 14 (should not be reseeded)", got.AdvancedScans.SMART.MaxAgeDays)
	}
}

// TestMigrateSettings_V2UserTouchesSettings_DoesNotReMigrateZeroMaxAge
// pins the combined preservation contract after issue #268: a v2 blob
// with the full spectrum of edge-case user choices (explicit
// max_age_days=0 AND wake_drives=true) survives both the v1→v2 ladder
// and the v2→v3 relocation with values intact.
func TestMigrateSettings_V2UserTouchesSettings_DoesNotReMigrateZeroMaxAge(t *testing.T) {
	raw := []byte(`{
		"settings_version": 2,
		"scan_interval": "30m",
		"theme": "midnight",
		"smart": {
			"wake_drives": true,
			"max_age_days": 0
		}
	}`)

	got := migrateSettings(raw, defaultSettings())
	if got.SettingsVersion < currentSettingsVersion {
		got.SettingsVersion = currentSettingsVersion
	}

	if got.SettingsVersion != currentSettingsVersion {
		t.Errorf("settings_version: got %d, want %d", got.SettingsVersion, currentSettingsVersion)
	}
	if got.AdvancedScans.SMART.MaxAgeDays != 0 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 0 (explicit user zero must survive — was the visible half of #268)", got.AdvancedScans.SMART.MaxAgeDays)
	}
	if !got.AdvancedScans.SMART.WakeDrives {
		t.Errorf("advanced_scans.smart.wake_drives: got false, want true (explicit user true must survive — was the silent half of #268)")
	}
}

// TestMigrateSettings_V2_PreservesMaxAgeDaysZero ensures that a user
// who has deliberately disabled the safety net (set max_age_days=0)
// does not silently get it re-enabled by the migration. Idempotency
// must hold for explicit user-chosen values, including edge cases.
func TestMigrateSettings_V2_PreservesMaxAgeDaysZero(t *testing.T) {
	raw := []byte(`{
		"settings_version": 2,
		"scan_interval": "30m",
		"theme": "midnight",
		"smart": {
			"wake_drives": false,
			"max_age_days": 0
		}
	}`)

	got := migrateSettings(raw, defaultSettings())

	if got.AdvancedScans.SMART.MaxAgeDays != 0 {
		t.Errorf("advanced_scans.smart.max_age_days: got %d, want 0 (user explicitly disabled the safety net — must be preserved)", got.AdvancedScans.SMART.MaxAgeDays)
	}
}

// TestGetSettings_V1toV2_PersistsMigration closes the loop end-to-end
// through the HTTP-facing getSettings() function: a stored v1 blob is
// transparently migrated to the current version on first read, and the
// persisted representation is rewritten so subsequent reads skip the
// migration path. Since the current version is now v3 (#259), the end
// state assertions target the v3 shape; slice-1 intent (WakeDrives
// lift, MaxAgeDays seed) is preserved inside advanced_scans.smart.
func TestGetSettings_V1toV2_PersistsMigration(t *testing.T) {
	srv := newSettingsTestServer()
	if err := srv.store.SetConfig(settingsConfigKey, `{
		"settings_version": 1,
		"scan_interval": "30m",
		"theme": "midnight",
		"wake_drives_for_smart": true
	}`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	loaded := srv.getSettings()
	if loaded.SettingsVersion != currentSettingsVersion {
		t.Errorf("loaded settings_version: got %d, want %d", loaded.SettingsVersion, currentSettingsVersion)
	}
	if !loaded.AdvancedScans.SMART.WakeDrives {
		t.Errorf("loaded advanced_scans.smart.wake_drives: got false, want true")
	}
	if loaded.AdvancedScans.SMART.MaxAgeDays != 7 {
		t.Errorf("loaded advanced_scans.smart.max_age_days: got %d, want 7", loaded.AdvancedScans.SMART.MaxAgeDays)
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
	if v, _ := persisted["settings_version"].(float64); int(v) != currentSettingsVersion {
		t.Errorf("persisted settings_version: got %v, want %d (migration must rewrite the store)", persisted["settings_version"], currentSettingsVersion)
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
	if mad, _ := smartMap["max_age_days"].(float64); int(mad) != 7 {
		t.Errorf("persisted advanced_scans.smart.max_age_days: got %v, want 7", smartMap["max_age_days"])
	}
}
