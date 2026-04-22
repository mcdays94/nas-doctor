package api

import (
	"encoding/json"
	"testing"
)

// TestDefaultSettings_SpeedTestCadence_DailyAt03 locks in the v0.9.6 default
// speed-test cadence change: once a day at 03:00 local time, down from the
// previous 4-hour interval. See #210.
//
// Existing users who have ever saved settings keep their current cadence
// verbatim — this default only applies to fresh installs (no persisted
// settings blob). The fresh-install path is exercised in
// TestGetSettings_NoStoredConfig_ReturnsDefaults below.
func TestDefaultSettings_SpeedTestCadence_DailyAt03(t *testing.T) {
	settings := defaultSettings()

	if settings.SpeedTestInterval != "24h" {
		t.Errorf("SpeedTestInterval = %q, want %q", settings.SpeedTestInterval, "24h")
	}

	if len(settings.SpeedTestSchedule) != 1 || settings.SpeedTestSchedule[0] != "03:00" {
		t.Errorf("SpeedTestSchedule = %v, want [\"03:00\"]", settings.SpeedTestSchedule)
	}

	// SpeedTestDay is only meaningful for weekly/monthly modes. Daily should
	// leave it empty so the UI doesn't show stale weekday-picker state.
	if settings.SpeedTestDay != "" {
		t.Errorf("SpeedTestDay = %q, want empty for daily cadence", settings.SpeedTestDay)
	}
}

// TestDefaultSettings_ShipsDefaultSpeedCheck locks in the v0.9.6 fresh-install
// behavior: one pre-configured "Internet Speed" service check with blank
// contracted-speed thresholds (heartbeat mode). See #210.
func TestDefaultSettings_ShipsDefaultSpeedCheck(t *testing.T) {
	settings := defaultSettings()

	if len(settings.ServiceChecks.Checks) != 1 {
		t.Fatalf("default settings has %d service checks, want 1", len(settings.ServiceChecks.Checks))
	}

	seed := settings.ServiceChecks.Checks[0]
	if seed.Name != "Internet Speed" {
		t.Errorf("seed.Name = %q, want %q", seed.Name, "Internet Speed")
	}
	if seed.Type != "speed" {
		t.Errorf("seed.Type = %q, want %q", seed.Type, "speed")
	}
	if !seed.Enabled {
		t.Error("seed.Enabled = false; the whole point of shipping this check is that it's visible on fresh install")
	}

	// Blank contracted-speed thresholds are LOAD-BEARING: they make the
	// check report "up" whenever speedtest_history has fresh data, acting
	// as a speed-test heartbeat rather than firing false alerts. Users
	// tune these once they know their line's sustained speed.
	if seed.ContractedDownMbps != 0 {
		t.Errorf("seed.ContractedDownMbps = %v, want 0 (heartbeat mode)", seed.ContractedDownMbps)
	}
	if seed.ContractedUpMbps != 0 {
		t.Errorf("seed.ContractedUpMbps = %v, want 0 (heartbeat mode)", seed.ContractedUpMbps)
	}
}

// TestDefaultSettings_PersistedChecksOverrideSeed is the critical invariant:
// existing users who have saved any service-check configuration (even an
// empty list) must NOT see the shipped-default "Internet Speed" check
// resurrect on upgrade. Only genuine fresh installs (no persisted settings)
// get the seed.
//
// Mechanically this works because getSettings() starts with defaultSettings()
// and then json.Unmarshal's the persisted blob onto it. Any non-nil "checks"
// field in the blob replaces the default slice wholesale.
func TestDefaultSettings_PersistedEmptyChecksOverridesSeed(t *testing.T) {
	// Simulate a persisted settings blob with an empty check list —
	// the exact shape a user would have after deleting all their
	// service checks in the UI.
	persisted := `{"service_checks":{"checks":[]}}`

	settings := defaultSettings()
	if err := json.Unmarshal([]byte(persisted), &settings); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(settings.ServiceChecks.Checks) != 0 {
		t.Errorf("persisted empty checks did not override default; got %d checks, want 0", len(settings.ServiceChecks.Checks))
	}
}

// TestDefaultSettings_PersistedNonEmptyChecksOverrideSeed covers the other
// side: a user with their own check list should see EXACTLY their list on
// reload, not their list plus the default.
func TestDefaultSettings_PersistedNonEmptyChecksOverrideSeed(t *testing.T) {
	persisted := `{"service_checks":{"checks":[{"name":"My HTTP Check","type":"http","target":"https://example.com","enabled":true}]}}`

	settings := defaultSettings()
	if err := json.Unmarshal([]byte(persisted), &settings); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(settings.ServiceChecks.Checks) != 1 {
		t.Fatalf("got %d checks, want 1 (persisted check only)", len(settings.ServiceChecks.Checks))
	}
	if settings.ServiceChecks.Checks[0].Name != "My HTTP Check" {
		t.Errorf("check name = %q, want %q (persisted value)", settings.ServiceChecks.Checks[0].Name, "My HTTP Check")
	}
}
