package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestStyles_DuplicacyBackupRow_ThreeSourceParity locks the Duplicacy
// row CSS rules across the three CSS sources that must agree on every
// rule the dashboard JS references:
//
//   - SharedCSS (internal/api/styles.go) — served at /css/shared.css,
//     consumed by every page EXCEPT the dashboard themes.
//   - midnight.html — dashboard theme template, has its own inline
//     <style> block and does NOT link shared.css.
//   - clean.html — same story, second dashboard theme.
//
// AGENTS.md §"Theme template parity is critical": v0.9.7 rc4/5/6
// surfaced this exact class of bug — pill rules existed in shared.css
// but the dashboard themes didn't load shared.css, so .pill-trace
// rendered as plain text on the dashboard despite passing every
// "rule-exists-somewhere" test. The cure is the three-source pin: any
// new Duplicacy row class added to dashboard.go MUST exist with
// matching colour values in all three sources before the change is
// considered safe to ship.
//
// Issue #314.
func TestStyles_DuplicacyBackupRow_ThreeSourceParity(t *testing.T) {
	// Approved palette for Duplicacy row variants. Both classes use
	// Tailwind 400-weight shades to match the broader design-system
	// pattern (purple-400 for the kind tag, cyan-400 for the running
	// badge — perceptually distinct from every existing pill colour
	// pinned by TestStyles_ServiceCheckPillPalette).
	want := map[string]string{
		"duplicacy-kind-tag":      "#c084fc", // purple-400
		"duplicacy-running-badge": "#22d3ee", // cyan-400
	}

	sources := map[string]string{
		"SharedCSS":     SharedCSS,
		"midnight.html": DashboardMidnight,
		"clean.html":    DashboardClean,
	}

	for sourceName, css := range sources {
		for className, wantColor := range want {
			got := extractDuplicacyClassColor(t, css, sourceName, className)
			if got != wantColor {
				t.Errorf(".%s in %s: got %q, want %q — update this test AND all three CSS sources together. The dashboard themes do not link /css/shared.css so any Duplicacy row class must be inlined into BOTH theme templates.", className, sourceName, got, wantColor)
			}
		}
		// .backup-card-duplicacy must exist as a rule in each source
		// (even if its body is empty / inherits from .backup-card) so
		// the data-provider="duplicacy" + class wiring on the
		// dashboard rows is honoured by future theme overrides.
		if !strings.Contains(css, ".backup-card-duplicacy") {
			t.Errorf("%s missing .backup-card-duplicacy class — needed even with empty body so themes can override the duplicacy variant independently from the borg backup-card", sourceName)
		}
	}
}

// extractDuplicacyClassColor returns the #RRGGBB color value on the
// .className rule from css. Returns "" and fails the test if the
// rule is missing or the colour can't be parsed. Mirrors the shape
// of extractPillColor in styles_pill_palette_test.go.
func extractDuplicacyClassColor(t *testing.T, css, sourceLabel, className string) string {
	t.Helper()
	re := regexp.MustCompile(`\.` + regexp.QuoteMeta(className) + `\b[^{]*\{[^}]*color:\s*(#[0-9a-fA-F]{6})`)
	m := re.FindStringSubmatch(css)
	if len(m) < 2 {
		t.Errorf(".%s: rule or colour not found in %s", className, sourceLabel)
		return ""
	}
	return strings.ToLower(m[1])
}
