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
// Expected: v2-shape output with smart.wake_drives=true, seeded
// smart.max_age_days=7, and settings_version bumped to 2.
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

	if got.SettingsVersion != 2 {
		t.Errorf("settings_version: got %d, want 2", got.SettingsVersion)
	}
	if !got.SMART.WakeDrives {
		t.Errorf("smart.wake_drives: got false, want true (lifted from wake_drives_for_smart)")
	}
	if got.SMART.MaxAgeDays != 7 {
		t.Errorf("smart.max_age_days: got %d, want 7 (PRD #236 user story 2 default)", got.SMART.MaxAgeDays)
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

	if got.SMART.WakeDrives {
		t.Errorf("smart.wake_drives: got true, want false (no legacy opt-in present)")
	}
	if got.SMART.MaxAgeDays != 7 {
		t.Errorf("smart.max_age_days: got %d, want 7 (seeded for every upgrader)", got.SMART.MaxAgeDays)
	}
}

// TestMigrateSettings_V2_Idempotent confirms that a v2-shaped blob is
// passed through unchanged. Running the migration on an already-
// migrated blob is the code path hit on every subsequent read of the
// persisted config, so it must not mutate preserved values.
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

	if got.SettingsVersion != 2 {
		t.Errorf("settings_version: got %d, want 2 preserved", got.SettingsVersion)
	}
	if !got.SMART.WakeDrives {
		t.Errorf("smart.wake_drives: got false, want true (should not be clobbered)")
	}
	if got.SMART.MaxAgeDays != 14 {
		t.Errorf("smart.max_age_days: got %d, want 14 (should not be reseeded)", got.SMART.MaxAgeDays)
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

	if got.SMART.MaxAgeDays != 0 {
		t.Errorf("smart.max_age_days: got %d, want 0 (user explicitly disabled the safety net — must be preserved)", got.SMART.MaxAgeDays)
	}
}

// TestGetSettings_V1toV2_PersistsMigration closes the loop end-to-end
// through the HTTP-facing getSettings() function: a stored v1 blob is
// transparently migrated to v2 on first read, and the persisted
// representation on the store is rewritten so subsequent reads skip
// the migration path. This guards against a subtle regression where
// the migration only applies in-memory and the stored blob stays v1.
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
	if loaded.SettingsVersion != 2 {
		t.Errorf("loaded settings_version: got %d, want 2", loaded.SettingsVersion)
	}
	if !loaded.SMART.WakeDrives {
		t.Errorf("loaded smart.wake_drives: got false, want true")
	}
	if loaded.SMART.MaxAgeDays != 7 {
		t.Errorf("loaded smart.max_age_days: got %d, want 7", loaded.SMART.MaxAgeDays)
	}

	// Subsequent read: the stored blob must now be v2-shaped, so a
	// fresh migrateSettings invocation on it is an idempotent no-op.
	raw, err := srv.store.GetConfig(settingsConfigKey)
	if err != nil {
		t.Fatalf("re-read stored settings: %v", err)
	}
	var persisted map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		t.Fatalf("parse persisted settings: %v", err)
	}
	if v, _ := persisted["settings_version"].(float64); int(v) != 2 {
		t.Errorf("persisted settings_version: got %v, want 2 (migration must rewrite the store)", persisted["settings_version"])
	}
	smartMap, ok := persisted["smart"].(map[string]interface{})
	if !ok {
		t.Fatalf("persisted settings missing nested smart object: %v", persisted["smart"])
	}
	if wd, _ := smartMap["wake_drives"].(bool); !wd {
		t.Errorf("persisted smart.wake_drives: got %v, want true", smartMap["wake_drives"])
	}
	if mad, _ := smartMap["max_age_days"].(float64); int(mad) != 7 {
		t.Errorf("persisted smart.max_age_days: got %v, want 7", smartMap["max_age_days"])
	}
}
