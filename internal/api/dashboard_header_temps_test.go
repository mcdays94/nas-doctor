package api

import (
	"strings"
	"testing"
)

// Issue #269 — CPU + mainboard temperature gauges in the dashboard
// header stats row. The render lives INSIDE the theme templates'
// inline <script> blocks (NOT in DashboardJS), because the header
// is theme-specific markup. midnight.html uses .stat-item-label /
// .stat-item-value; clean.html uses .stat-label / .stat-val.
//
// These tests assert that:
//   1. classForTemp helper exists in DashboardJS and applies the
//      <60 / 60-75 / >75 thresholds documented in the issue.
//   2. Both theme templates render conditional cpu_temp_c and
//      mobo_temp_c stat-item divs gated on a truthy value (so
//      sensors-missing → no gauge, NOT empty placeholder).
//   3. Both theme templates inline the conditional, with no new
//      CSS class names (theme-template parity rule).

// TestDashboardJS_ClassForTemp_Thresholds pins the colour-mapping
// boundaries against the issue acceptance criteria item 2:
// "Values colour-coded by threshold (green < 60°C, yellow 60-75°C,
// red > 75°C)".
func TestDashboardJS_ClassForTemp_HelperPresent(t *testing.T) {
	js := DashboardJS
	if !strings.Contains(js, "util.classForTemp") {
		t.Fatal("DashboardJS: util.classForTemp helper is missing — issue #269 needs a temperature → colour mapping helper")
	}
	// Pin the thresholds. We don't pin exact comparison operators
	// because the helper might use `>=` vs `>`, but the numeric
	// values 60 and 75 MUST appear inside the helper body so a
	// future refactor that moves the boundaries (and breaks the
	// documented green/amber/red thresholds) is caught.
	start := strings.Index(js, "util.classForTemp")
	if start < 0 {
		t.Fatal("classForTemp not found")
	}
	body := js[start:]
	end := strings.Index(body, "};")
	if end < 0 {
		t.Fatal("classForTemp body terminator not found")
	}
	helper := body[:end]
	if !strings.Contains(helper, "75") {
		t.Error("classForTemp does not reference 75 (the >75°C red threshold per issue #269)")
	}
	if !strings.Contains(helper, "60") {
		t.Error("classForTemp does not reference 60 (the 60-75°C amber threshold per issue #269)")
	}
}

// TestDashboardJS_ClassForTemp_Values exercises the helper through
// the JS source (we can't actually evaluate JS in a Go test, so we
// pin the textual mapping instead). Each colour MUST be referenced.
func TestDashboardJS_ClassForTemp_Values(t *testing.T) {
	js := DashboardJS
	start := strings.Index(js, "util.classForTemp")
	if start < 0 {
		t.Fatal("classForTemp not found")
	}
	body := js[start:]
	end := strings.Index(body, "};")
	if end < 0 {
		t.Fatal("classForTemp body terminator not found")
	}
	helper := body[:end]

	for _, want := range []string{"text-red", "text-amber", "text-green"} {
		if !strings.Contains(helper, want) {
			t.Errorf("classForTemp does not return %q — palette must use the existing inlined classes (theme-template parity)", want)
		}
	}
}

// TestThemeTemplates_RenderTempStatItems asserts that BOTH theme
// templates emit stat-item markup for cpu_temp_c and mobo_temp_c,
// gated on a truthy value. This is the core acceptance criteria
// item 1 (header stats row adds CPU + Mobo temp).
func TestThemeTemplates_RenderTempStatItems(t *testing.T) {
	cases := []struct {
		name    string
		tmpl    string
		needles []string
	}{
		{
			name: "midnight",
			tmpl: DashboardMidnight,
			needles: []string{
				"sys.cpu_temp_c",
				"sys.mobo_temp_c",
				"classForTemp",
				// Conditional render — the gauge should ONLY appear
				// when value is non-zero. Look for a guard pattern.
				"cpuTemp > 0",
				"moboTemp > 0",
			},
		},
		{
			name: "clean",
			tmpl: DashboardClean,
			needles: []string{
				"sys.cpu_temp_c",
				"sys.mobo_temp_c",
				"classForTemp",
				"cpuTemp > 0",
				"moboTemp > 0",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range tc.needles {
				if !strings.Contains(tc.tmpl, n) {
					t.Errorf("%s.html: missing %q — issue #269 expects cpu_temp_c and mobo_temp_c rendered with a conditional guard", tc.name, n)
				}
			}
		})
	}
}

// TestThemeTemplates_GracefulFallback regression-guards the most
// important UX bit: when the sensors are missing (sys.cpu_temp_c
// is zero/undefined), the stat-item must NOT render at all.
//
// We can't execute the JS, but we CAN check that the temperature
// markup is INSIDE the `if (cpuTemp > 0)` / `if (moboTemp > 0)`
// guards rather than unconditionally appended. The simplest
// invariant: for each template, the literal string
//
//   if (cpuTemp > 0)
//
// must appear BEFORE the literal `CPU \u00b0C` / `cpu_temp_c`
// rendering line. Same for mobo.
func TestThemeTemplates_GracefulFallback(t *testing.T) {
	cases := []struct {
		name string
		tmpl string
	}{
		{name: "midnight", tmpl: DashboardMidnight},
		{name: "clean", tmpl: DashboardClean},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpuGuard := strings.Index(tc.tmpl, "cpuTemp > 0")
			cpuRender := strings.Index(tc.tmpl, "CPU \\u00b0C")
			if cpuGuard < 0 || cpuRender < 0 {
				t.Fatalf("%s.html: missing CPU temp guard or render — guard=%d render=%d", tc.name, cpuGuard, cpuRender)
			}
			if cpuGuard >= cpuRender {
				t.Errorf("%s.html: 'cpuTemp > 0' guard at offset %d must precede the CPU temp render at offset %d", tc.name, cpuGuard, cpuRender)
			}
			moboGuard := strings.Index(tc.tmpl, "moboTemp > 0")
			moboRender := strings.Index(tc.tmpl, "Mobo \\u00b0C")
			if moboGuard < 0 || moboRender < 0 {
				t.Fatalf("%s.html: missing Mobo temp guard or render — guard=%d render=%d", tc.name, moboGuard, moboRender)
			}
			if moboGuard >= moboRender {
				t.Errorf("%s.html: 'moboTemp > 0' guard at offset %d must precede the Mobo temp render at offset %d", tc.name, moboGuard, moboRender)
			}
		})
	}
}

// TestThemeTemplates_NoNewCSSClasses — AGENTS.md theme-parity rule:
// the theme templates do NOT link /css/shared.css, so any new
// CSS class would silently not apply. This feature uses the
// existing text-red/text-amber/text-green classes (already inlined
// in both templates). Pin the absence of the obvious naive class
// names so a future patch can't accidentally regress.
func TestThemeTemplates_NoNewTempClasses(t *testing.T) {
	disallowed := []string{
		`class="temp-hot"`,
		`class="temp-warm"`,
		`class="temp-cool"`,
		`class="cpu-temp"`,
		`class="mobo-temp"`,
	}
	for _, tmpl := range []struct {
		name string
		src  string
	}{
		{"midnight", DashboardMidnight},
		{"clean", DashboardClean},
		{"DashboardJS", DashboardJS},
	} {
		for _, tok := range disallowed {
			if strings.Contains(tmpl.src, tok) {
				t.Errorf("%s introduces CSS class token %q — theme templates do NOT link shared.css. Reuse text-red/text-amber/text-green via classForTemp instead", tmpl.name, tok)
			}
		}
	}
}
