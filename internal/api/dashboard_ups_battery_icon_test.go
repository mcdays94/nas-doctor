package api

import (
	"strings"
	"testing"
)

// Issue #235: the UPS Power card currently renders battery level as
// plain text ("Battery: 87%"). Users can't tell at a glance whether
// the battery is full, low, or somewhere in between without reading
// the number. Add a smartphone-style SVG battery glyph next to the
// percentage:
//
//   1. Outline rectangle with a small terminal nub on the right
//   2. Inner fill proportional to battery_percent
//   3. Colour-code by level:
//        > 50% -> var(--green)
//        20-50% -> var(--amber)
//        < 20% -> var(--red)
//   4. Optional charging bolt overlay when ups.on_battery === false
//
// The icon lives in sections.ups inside DashboardJS. Because
// midnight.html and clean.html do NOT link /css/shared.css
// (documented in AGENTS.md), every style must be inline — no new
// CSS class names. These tests pin the render branches and the
// regression-critical invariant that the existing battery text is
// preserved, not replaced.

// upsSection returns the body of sections.ups from DashboardJS, so
// the assertions below can't be accidentally satisfied by markup
// elsewhere on the dashboard (e.g. some other section that happens
// to ship an SVG).
func upsSection(t *testing.T) string {
	t.Helper()
	js := DashboardJS
	start := strings.Index(js, "sections.ups = function")
	if start < 0 {
		t.Fatal("DashboardJS: sections.ups function not found")
	}
	rest := js[start:]
	end := strings.Index(rest[1:], "sections.")
	if end < 0 {
		end = len(rest)
	} else {
		end++
	}
	return rest[:end]
}

// The battery icon must render as an inline SVG — class-free,
// because theme templates don't pick up shared.css rules.
func TestDashboardJS_UPSBatteryIcon_RendersSVG(t *testing.T) {
	body := upsSection(t)

	if !strings.Contains(body, "<svg") {
		t.Error("DashboardJS: sections.ups does not emit an <svg element; battery icon is missing")
	}
	// The battery shape needs two rectangles — the outline and the
	// terminal nub on the right. We don't pin exact coords because
	// those are a free design choice, just that there's more than
	// one <rect so the glyph has structure.
	rectCount := strings.Count(body, "<rect")
	if rectCount < 2 {
		t.Errorf("DashboardJS: sections.ups renders only %d <rect element(s); a smartphone-style battery needs at least an outline rect + a terminal nub rect + a fill rect", rectCount)
	}
}

// Regression guard — the textual "Battery: NN%" must still render.
// The icon is an ADDITION, not a replacement. Screen readers and
// users scanning the raw number rely on the text.
func TestDashboardJS_UPSBatteryIcon_BatteryTextStillPresent(t *testing.T) {
	body := upsSection(t)

	if !strings.Contains(body, "Battery:") {
		t.Error("DashboardJS: sections.ups no longer renders the 'Battery:' text label; battery text must remain alongside the icon, not be replaced")
	}
	if !strings.Contains(body, "battery_percent") {
		t.Error("DashboardJS: sections.ups no longer references ups.battery_percent; the battery value is missing")
	}
	// The existing toFixed(0) formatting for the percentage must survive.
	if !strings.Contains(body, "battery_percent || 0).toFixed(0)") {
		t.Error("DashboardJS: the 'battery_percent || 0).toFixed(0)' formatting is gone; existing render was replaced rather than augmented")
	}
}

// The icon's fill-width must be derived from ups.battery_percent
// (scaled to whatever viewport width the author picks). We can't
// pin exact numbers because the SVG dimensions are a design choice,
// but the fill rect's width MUST reference battery_percent — if it
// doesn't, the icon is static and unrelated to the actual state.
func TestDashboardJS_UPSBatteryIcon_FillScalesWithBatteryPercent(t *testing.T) {
	body := upsSection(t)

	// Find the battery-icon region. We look for the word 'battery'
	// with a case-insensitive match in comments/vars near the icon,
	// or more defensively, just assert the body references
	// battery_percent in close proximity to <rect. Crude check:
	// the fill rect width must be a JS expression involving
	// battery_percent (string-concatenated, e.g.
	// ' + battery_percent + '). If the SVG is rendered by a
	// helper function, the helper body is inside DashboardJS too
	// and will still match.
	if !strings.Contains(body, "battery_percent") {
		t.Fatal("pre-condition: battery_percent already checked by another test")
	}
	// Count battery_percent references in the ups section. We expect
	// at least two — one for the text and one for the icon fill. If
	// only one, the icon is hardcoded.
	refs := strings.Count(body, "battery_percent")
	if refs < 2 {
		t.Errorf("DashboardJS: sections.ups references ups.battery_percent only %d time(s); expected >= 2 (once for text, once for icon fill)", refs)
	}
}

// Colour thresholds must match the issue: > 50% green, 20-50% amber,
// < 20% red. We assert all three CSS variables are referenced inside
// the ups section. The icon helper lives in DashboardJS alongside
// sections.ups (or directly inside it) — in either case the colour
// references show up when sections.ups is invoked, so checking the
// full DashboardJS string would be too permissive. We scope to the
// ups section.
func TestDashboardJS_UPSBatteryIcon_UsesGreenAmberRedPalette(t *testing.T) {
	body := upsSection(t)

	// If the icon helper lives outside sections.ups, we still want
	// the colour references reachable from this section's code path.
	// Expand the search to the whole DashboardJS for the palette —
	// but first check that a helper call OR inline branching is
	// present in the ups section.
	if !strings.Contains(body, "var(--green)") {
		t.Error("DashboardJS: sections.ups (or its battery-icon helper) does not reference var(--green); the > 50% threshold is not colour-coded")
	}
	if !strings.Contains(body, "var(--amber)") {
		t.Error("DashboardJS: sections.ups does not reference var(--amber); the 20-50% threshold is not colour-coded")
	}
	if !strings.Contains(body, "var(--red)") {
		t.Error("DashboardJS: sections.ups does not reference var(--red); the < 20% threshold is not colour-coded")
	}
}

// The threshold logic must name the break-points from the issue: 50
// and 20. Pinning these prevents a silent refactor that moves the
// breakpoints to e.g. 30/60 without updating the documented design.
func TestDashboardJS_UPSBatteryIcon_ThresholdsAt50And20(t *testing.T) {
	body := upsSection(t)

	// Either 'pct > 50' / 'pct >= 50' / '> 50' etc — we don't want
	// to pin exact operator choice. Accept any of the most natural
	// comparisons. Keep this flexible but meaningful.
	has50 := strings.Contains(body, "> 50") || strings.Contains(body, ">= 50") || strings.Contains(body, ">=50") || strings.Contains(body, ">50")
	has20 := strings.Contains(body, "< 20") || strings.Contains(body, "<= 20") || strings.Contains(body, "<=20") || strings.Contains(body, "<20") || strings.Contains(body, ">= 20") || strings.Contains(body, ">=20") || strings.Contains(body, "> 20") || strings.Contains(body, ">20")

	if !has50 {
		t.Error("DashboardJS: battery-icon colour logic does not contain a '> 50' (or >=) comparison; green threshold from issue #235 is not implemented")
	}
	if !has20 {
		t.Error("DashboardJS: battery-icon colour logic does not contain a '< 20' (or > 20) comparison; red threshold from issue #235 is not implemented")
	}
}

// Charging-bolt overlay: when ups.on_battery === false (on AC),
// the icon should get an optional lightning-bolt hint. The hint
// distinguishes 'fully topped off on AC' from 'discharging at 87%'.
// We don't pin the exact SVG path — just that the ups section
// branches on on_battery and emits a bolt-shaped extra element
// (we look for the <path glyph, which is the natural shape for a
// bolt, and the on_battery conditional gate).
func TestDashboardJS_UPSBatteryIcon_ChargingBoltGatedOnAC(t *testing.T) {
	body := upsSection(t)

	if !strings.Contains(body, "on_battery") {
		t.Error("DashboardJS: sections.ups does not reference ups.on_battery in its battery-icon rendering; charging state is not reflected visually")
	}
	// The bolt is rendered as an SVG <path element (polygons / lines
	// are pedagogically possible but the issue design sketch uses a
	// path, and it's the idiomatic choice). If a future author
	// prefers a polygon, update this test — but at least the bolt
	// must come from a shape element distinct from the <rect outline
	// and fill.
	if !strings.Contains(body, "<path") {
		t.Error("DashboardJS: sections.ups does not emit an SVG <path element; charging-bolt overlay is missing")
	}
}

// No new CSS class names in DashboardJS for this battery-icon
// feature — the theme templates don't link shared.css, so any new
// class-based styling would silently not apply on the dashboard.
// We can't enumerate all class tokens (too many pre-existing ones),
// but we can pin the absence of the specific tokens a naive
// implementation would introduce: "battery-icon", "battery-fill",
// "battery-outline". If a future author chooses to add classes,
// they MUST inline the rules into midnight.html and clean.html and
// remove this test.
func TestDashboardJS_UPSBatteryIcon_NoNewCSSClassesInlined(t *testing.T) {
	body := upsSection(t)

	disallowed := []string{
		`class="battery-icon"`,
		`class="battery-fill"`,
		`class="battery-outline"`,
		`class="battery-nub"`,
		`class="charging-bolt"`,
	}
	for _, tok := range disallowed {
		if strings.Contains(body, tok) {
			t.Errorf("DashboardJS: sections.ups introduces CSS class token %q — theme templates midnight.html/clean.html do NOT link shared.css, so the rule will not apply on the dashboard. Use inline style=%q instead.", tok, "...")
		}
	}
}
