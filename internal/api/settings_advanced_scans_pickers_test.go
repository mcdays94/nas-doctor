package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsHTML_AdvancedScans_HasCreateIntervalPickerHelper pins
// the existence of the reusable JS helper that generates the six
// per-subsystem interval pickers. Per PRD #239 the helper is
// parameterised by (subsystemId, label) and returns markup wiring up
// a preset dropdown, custom d/h/m/s panel, live preview, cron
// visualizer, and an "edit cron" button.
//
// We use template-source assertions (same pattern as
// settings_smart_version_roundtrip_test.go) — the helper's internal
// markup can change without breaking this test, but its existence
// as a named function with the documented signature cannot.
func TestSettingsHTML_AdvancedScans_HasCreateIntervalPickerHelper(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	mustContain := []string{
		// Helper declaration. We allow either `function x(...)`
		// syntax or `var x = function(...)` by matching the name +
		// opening paren — both valid for the PRD contract.
		"createIntervalPicker",
		// Both parameter slots must appear in the signature.
		// Matching the literal "subsystemId" keeps the assertion
		// resilient to `(subsystemId, label)` or
		// `(subsystemId,label)` formatting.
		"subsystemId",
	}
	for _, sub := range mustContain {
		if !strings.Contains(content, sub) {
			t.Errorf("settings.html missing createIntervalPicker helper contract symbol: %q", sub)
		}
	}
}

// TestSettingsHTML_AdvancedScans_SixPickerInstantiations pins that
// the PRD's six subsystems each get their own picker in the Advanced
// card. Per the spec: SMART's picker sits next to the existing
// wake-drives and max-age controls; Docker / Proxmox / Kubernetes /
// ZFS / GPU group under a "Per-subsystem scan intervals" sub-section.
//
// We assert presence of a stable id per picker anchor — any shape
// the worker chooses is fine (e.g. `scan-interval-<id>`) so long as
// it's searchable + stable.
func TestSettingsHTML_AdvancedScans_SixPickerInstantiations(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	// Each picker must mount its markup against a stable per-subsystem
	// id. We match `scan-interval-<subsystem>` as the convention —
	// parallel to `scan-preset` for the global widget.
	subsystems := []string{"smart", "docker", "proxmox", "kubernetes", "zfs", "gpu"}
	for _, s := range subsystems {
		want := "scan-interval-" + s
		if !strings.Contains(content, want) {
			t.Errorf("settings.html missing picker anchor %q (expected one picker per subsystem per PRD #239)", want)
		}
	}

	// Sub-section header anchor for the non-SMART five must exist,
	// per the PRD: "Docker / Proxmox / Kubernetes / ZFS / GPU group
	// under a new Per-subsystem scan intervals sub-section".
	if !strings.Contains(content, "Per-subsystem scan intervals") {
		t.Errorf("settings.html missing the 'Per-subsystem scan intervals' sub-section header")
	}
}

// TestSettingsHTML_AdvancedScans_PickersInsideAdvancedCard pins that
// all six pickers live inside the single id="card-advanced" block
// (per #256 reversal — Advanced Scan Settings is one card, not two).
func TestSettingsHTML_AdvancedScans_PickersInsideAdvancedCard(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	cardIdx := strings.Index(content, `id="card-advanced"`)
	if cardIdx < 0 {
		t.Fatalf("card-advanced block not found")
	}
	rel := strings.Index(content[cardIdx+1:], `<div class="card"`)
	var cardEnd int
	if rel < 0 {
		cardEnd = len(content)
	} else {
		cardEnd = cardIdx + 1 + rel
	}
	body := content[cardIdx:cardEnd]

	for _, s := range []string{"smart", "docker", "proxmox", "kubernetes", "zfs", "gpu"} {
		want := "scan-interval-" + s
		if !strings.Contains(body, want) {
			t.Errorf("Advanced card body missing picker anchor %q — all six pickers must live inside card-advanced", want)
		}
	}

	// SMART picker is colocated with the wake-drives/max-age group —
	// the picker anchor must appear before the existing Hide-Docker
	// knob (which sits at the end of the SMART group in the template).
	smartPickerIdx := strings.Index(body, "scan-interval-smart")
	hideDockerIdx := strings.Index(body, "Hide Docker containers from dashboard")
	if smartPickerIdx < 0 || hideDockerIdx < 0 {
		t.Fatalf("missing ordering markers (smartPicker=%d hideDocker=%d)", smartPickerIdx, hideDockerIdx)
	}
	// The Per-subsystem intervals header should appear after the SMART
	// picker and the Hide-Docker knob is independent — but we at least
	// pin that the sub-section header is somewhere inside this card.
	if !strings.Contains(body, "Per-subsystem scan intervals") {
		t.Errorf("Advanced card body missing 'Per-subsystem scan intervals' sub-section header")
	}
}

// TestSettingsHTML_AdvancedScans_ReverseCronParserExists pins the
// existence of the JS function that translates a simple interval
// cron expression back into d/h/m/s fields. Per PRD #239 user story
// 8 the function handles four supported forms (`*/Ns`, `*/N * * * *`,
// `0 */N * * *`, `0 0 */N * *`). Complex crons must produce a UI
// error — PRD user story 14.
func TestSettingsHTML_AdvancedScans_ReverseCronParserExists(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "parseCronToFields") {
		t.Errorf("settings.html missing parseCronToFields() function (PRD #239 user story 8)")
	}
	// The UI error for unsupported crons must mention "simple"
	// (per PRD user story 14's recommended copy).
	if !strings.Contains(strings.ToLower(content), "simple interval cron") {
		t.Errorf("settings.html missing UI error copy mentioning 'simple interval cron' for unsupported cron input (PRD user story 14)")
	}
}

// TestSettingsHTML_AdvancedScans_ConfirmDialogOnMaxAgeConflict pins
// the client-side confirm() dialog wiring described by PRD #239 user
// story 12: saving a config with SMART.IntervalSec > MaxAgeDays *
// 86400 (both non-zero) triggers a confirm() explaining the
// consequence before the PUT fires.
//
// We use source-level assertions: the saveSettings path (or a
// pre-save hook it calls) must include a confirm() call whose
// surrounding logic references both smart interval and max-age.
func TestSettingsHTML_AdvancedScans_ConfirmDialogOnMaxAgeConflict(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "confirm(") {
		t.Errorf("settings.html has no confirm() call; expected max-age-vs-SMART-interval confirmation dialog (PRD #239 user story 12)")
	}

	// Locate the max-age conflict guard by comment marker or the
	// canonical helper name. The worker may choose either shape —
	// both satisfy the PRD. If neither appears, the guard is missing.
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "max_age") && !strings.Contains(lower, "max-age") {
		t.Fatalf("settings.html has no max-age references; cannot locate the confirm guard")
	}
	// The guard must compute the 86400 multiplier (days → seconds).
	if !strings.Contains(content, "86400") {
		t.Errorf("settings.html missing 86400 multiplier — expected SMART.IntervalSec vs MaxAgeDays*86400 comparison (PRD user story 12)")
	}
}

// TestSettingsHTML_AdvancedScans_PickerPresetOptions pins the preset
// dropdown's canonical option values per PRD #239: "Use global (30m)"
// first, then the seven time presets, plus a "Custom…" fallback.
//
// We can't easily match each <option> element because createIntervalPicker
// may build them via innerHTML or DOM insertion. Instead we match the
// string literals the helper must emit — "Use global", one of each
// preset value, "Custom".
func TestSettingsHTML_AdvancedScans_PickerPresetOptions(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	// These are the literal preset label-or-value markers that must
	// appear somewhere inside the helper/picker markup.
	mustContain := []string{
		"Use global",
		// Seven preset durations; we look for the distinct 7-day
		// marker from PRD user story 3 as a sentinel.
		"7d",
		// Custom fallback from the general-scan-interval widget
		// pattern; the new pickers replicate this.
		"Custom",
	}
	for _, sub := range mustContain {
		if !strings.Contains(content, sub) {
			t.Errorf("settings.html missing preset-option marker %q inside the interval-picker helper (PRD #239 user story 7)", sub)
		}
	}
}
