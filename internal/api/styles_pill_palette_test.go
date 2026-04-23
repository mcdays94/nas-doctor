package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestStyles_ServiceCheckPillPalette pins the pill colour palette for
// all service-check types and asserts no two types share a colour.
//
// Why this test exists: v0.9.7 rc4 UAT caught that .pill-http
// (#60a5fa), .pill-speed (#38bdf8) and .pill-trace (#818cf8) were
// all technically-distinct hex values but perceptually all in the
// "blue" family at 14% background opacity — users couldn't tell them
// apart in the service-checks list. Likewise .pill-smb and .pill-nfs
// explicitly shared amber via a multi-selector rule.
//
// This test locks the approved palette per type so:
//
//	(a) Accidental regressions (copy-paste duplicates, or re-adding a
//	    multi-selector that groups two types under one colour) fail
//	    loudly.
//	(b) Intentional palette changes require updating this test,
//	    forcing the conversation about whether the new colour is
//	    perceptually distinct from its neighbours.
//
// #189 rc4.
func TestStyles_ServiceCheckPillPalette(t *testing.T) {
	// Approved palette — each type has a perceptually-distinct hue.
	// Red is reserved for .pill-critical, so not used here.
	// Tailwind 400-weight shades for consistency with the rest of the
	// design system.
	want := map[string]string{
		"http":  "#60a5fa", // blue-400
		"tcp":   "#34d399", // emerald-400
		"dns":   "#f472b6", // pink-400
		"smb":   "#fbbf24", // amber-400
		"nfs":   "#fb923c", // orange-400
		"ping":  "#a78bfa", // violet-400
		"speed": "#22d3ee", // cyan-400
		"trace": "#2dd4bf", // teal-400
	}

	t.Run("shared_css_has_pinned_colors", func(t *testing.T) {
		for pillType, wantColor := range want {
			got := extractPillColor(t, SharedCSS, "SharedCSS", pillType)
			if got != wantColor {
				t.Errorf(".pill-%s in SharedCSS: got %q, want %q — update this test AND styles.go together, and verify the new colour is perceptually distinct from neighbours at 14%% background opacity", pillType, got, wantColor)
			}
		}
	})

	t.Run("all_pills_have_distinct_colors", func(t *testing.T) {
		seen := map[string]string{} // color -> pill type that claimed it first
		for pillType := range want {
			got := extractPillColor(t, SharedCSS, "SharedCSS", pillType)
			if got == "" {
				continue // pinned-color subtest will have reported this
			}
			if prev, dup := seen[got]; dup {
				t.Errorf(".pill-%s and .pill-%s both resolve to %s — every service-check type must have a unique pill colour", pillType, prev, got)
			}
			seen[got] = pillType
		}
	})

	// Dashboard theme templates (midnight.html, clean.html) have their
	// own inline <style> blocks and do NOT link /css/shared.css. So the
	// .pill-<type> rules must exist inline in each theme, with the SAME
	// approved colours as SharedCSS.
	//
	// Caught during v0.9.7 rc5 UAT: midnight theme rendered every
	// service-check type as plain text in the Service Checks widget.
	// Root cause: midnight.html had no .pill or .pill-<type> rules at
	// all. The JS was rendering <span class="pill pill-http">PING</span>
	// correctly, but with no CSS rules matching, the browser rendered
	// it as plain inline text. Clean theme had .pill base + 5 unrelated
	// .pill-healthy/warning/critical/info/neutral rules but none for
	// the 8 service-check types.
	//
	// This subtest pins every theme template to the canonical palette.
	themeSources := map[string]string{
		"midnight.html": DashboardMidnight,
		"clean.html":    DashboardClean,
	}
	t.Run("theme_templates_have_same_palette", func(t *testing.T) {
		for themeName, css := range themeSources {
			for pillType, wantColor := range want {
				got := extractPillColor(t, css, themeName, pillType)
				if got != wantColor {
					t.Errorf("%s .pill-%s: got %q, want %q — dashboard theme templates must include every service-check pill rule inline (they don't link shared.css)", themeName, pillType, got, wantColor)
				}
			}
		}
	})
}

// extractPillColor returns the #RRGGBB value on the .pill-<type> rule
// from css. Returns "" and fails the test if the rule is missing or
// the colour can't be parsed. sourceLabel is used in error messages
// to identify which CSS source failed (e.g. "SharedCSS",
// "midnight.html"). Multi-selector rules like ".pill-x,.pill-y {...}"
// match for either x or y and return the same colour for both —
// which the distinctness subtest then catches.
func extractPillColor(t *testing.T, css, sourceLabel, pillType string) string {
	t.Helper()
	re := regexp.MustCompile(`\.pill-` + regexp.QuoteMeta(pillType) + `\b[^{]*\{[^}]*color:\s*(#[0-9a-fA-F]{6})`)
	m := re.FindStringSubmatch(css)
	if len(m) < 2 {
		t.Errorf(".pill-%s: rule or colour not found in %s", pillType, sourceLabel)
		return ""
	}
	return strings.ToLower(m[1])
}
