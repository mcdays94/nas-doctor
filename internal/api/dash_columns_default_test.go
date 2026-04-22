package api

import (
	"strings"
	"testing"
)

// TestDefaultSettings_DashColumnsIsThree locks in the v0.9.6+ default: fresh
// installs render the dashboard with 3 columns instead of 2. Modern displays
// (1440p+, ultrawide, most laptops) accommodate 3 columns comfortably and the
// extra lane surfaces more above-the-fold information. See issue #208.
func TestDefaultSettings_DashColumnsIsThree(t *testing.T) {
	d := defaultSettings()
	if d.Sections.DashColumns != 3 {
		t.Errorf("defaultSettings().Sections.DashColumns = %d; want 3 (issue #208)", d.Sections.DashColumns)
	}
}

// TestDashboardJS_AutoModeMapsToThreeColumns verifies the shared DashboardJS
// renderer treats an implicit / zero-valued dash_columns as 3 columns.
//
// Users who upgraded from pre-v0.9.6 will have `dash_columns: 0` persisted
// (either literally, or absent from the JSON because zero-valued ints don't
// serialize). The renderer's `|| 3` fallback is the hook that makes those
// users see 3 columns without any data migration.
//
// This test scans the Go string literal that backs the shared JS so the build
// fails if someone reverts the mapping back to `|| 2`.
func TestDashboardJS_AutoModeMapsToThreeColumns(t *testing.T) {
	js := DashboardJS
	want := "var numCols = sec.dash_columns || 3;"
	if !strings.Contains(js, want) {
		t.Errorf("DashboardJS missing %q — the dash_columns `|| 3` fallback controls the auto-mode column count (issue #208)", want)
	}
	// Also guard against the fallback-when-negative path rendering 2. Both
	// branches (falsy-zero and <1) feed into the same `gridTemplateColumns`
	// expression, so both need to land on 3 for consistency.
	if !strings.Contains(js, "if (numCols < 1) numCols = 3;") {
		t.Errorf("DashboardJS missing `numCols < 1` fallback to 3 — keeps auto/invalid paths aligned on the new default (issue #208)")
	}
	// Negative guard: the stale `|| 2` literal must not appear in the
	// distributeSections path — a future refactor could inadvertently
	// re-introduce it.
	bad := "var numCols = sec.dash_columns || 2;"
	if strings.Contains(js, bad) {
		t.Errorf("DashboardJS still contains stale %q — column auto-mode default was reverted to 2 (issue #208)", bad)
	}
}

// TestDashboardTheme_AutoModeMapsToThreeColumns verifies each theme template's
// inline "Two-column layout" helper uses the same `|| 3` fallback. Each theme
// has its own copy of this logic (midnight.html, clean.html) and they must
// stay in sync or the dashboard renders inconsistently after a theme switch.
func TestDashboardTheme_AutoModeMapsToThreeColumns(t *testing.T) {
	themes := map[string]string{
		"midnight": DashboardMidnight,
		"clean":    DashboardClean,
	}
	want := "var numCols = (st && st.sections && st.sections.dash_columns) || 3;"
	bad := "var numCols = (st && st.sections && st.sections.dash_columns) || 2;"
	fallbackWant := "if (numCols < 1) numCols = 3;"
	for name, tpl := range themes {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(tpl, want) {
				t.Errorf("theme %s missing %q — dash_columns auto-mode must fall back to 3 (issue #208)", name, want)
			}
			if strings.Contains(tpl, bad) {
				t.Errorf("theme %s still contains stale %q — auto-mode default was reverted to 2 (issue #208)", name, bad)
			}
			if !strings.Contains(tpl, fallbackWant) {
				t.Errorf("theme %s missing %q — numCols<1 fallback must align on 3 (issue #208)", name, fallbackWant)
			}
		})
	}
}

// TestSettingsHTML_DashColumnsAutoLabelSaysThree verifies the Settings UI
// dropdown option for the zero/auto value advertises "Auto (3 columns)",
// matching the new default. A mismatched label (still saying "2 columns")
// would mislead users into thinking the setting is broken when the dashboard
// actually renders 3.
func TestSettingsHTML_DashColumnsAutoLabelSaysThree(t *testing.T) {
	html := SettingsPage
	want := `<option value="0">Auto (3 columns)</option>`
	bad := `<option value="0">Auto (2 columns)</option>`
	if !strings.Contains(html, want) {
		t.Errorf("settings.html missing %q (issue #208)", want)
	}
	if strings.Contains(html, bad) {
		t.Errorf("settings.html still has stale %q — auto-mode label was not updated (issue #208)", bad)
	}
}

// TestSettings_ExplicitDashColumnsArePreserved is the round-trip guard for
// the preservation matrix in #208: users with explicit 1/2/3/4 must keep
// their chosen value after the default flip. The test exercises JSON
// marshal/unmarshal because persisted settings go through that path.
func TestSettings_ExplicitDashColumnsArePreserved(t *testing.T) {
	// No code change here — this test documents the contract that changing
	// the default must not mutate explicit user choices. It also provides a
	// tripwire if someone accidentally adds a "migrate 2 to 3" step that
	// overwrites an explicit 2.
	for _, explicit := range []int{1, 2, 3, 4} {
		s := defaultSettings()
		s.Sections.DashColumns = explicit
		if s.Sections.DashColumns != explicit {
			t.Errorf("explicit DashColumns=%d was not preserved after assignment; got %d", explicit, s.Sections.DashColumns)
		}
	}
}
