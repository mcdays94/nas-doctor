package api

import (
	"strings"
	"testing"
)

// extractSpeedtestSection returns the body of the
// `sections.speedtest = function ...` declaration in DashboardJS.
// Crude but sufficient: scan from the function declaration to the
// next *top-level* sections.<name> = function declaration. The
// previous heuristic (find next 'sections.') tripped on internal
// helper references like `sections._rangeButtons(...)` and ended
// the body too early.
func extractSpeedtestSection(t *testing.T) string {
	t.Helper()
	js := DashboardJS
	start := strings.Index(js, "sections.speedtest = function")
	if start < 0 {
		t.Fatal("DashboardJS: sections.speedtest function not found")
	}
	rest := js[start:]
	// Use the next top-level "\nsections.<name> = function" declaration
	// as the end bound — function-declaration lines always start at
	// column 0 in DashboardJS.
	end := strings.Index(rest[1:], "\nsections.")
	if end < 0 {
		end = len(rest)
	} else {
		end++
	}
	return rest[:end]
}

// PRD #283 / issue #284: the speed-test widget renders a small
// "via {engine}" caption beside the existing latest-line, surfacing
// which engine produced the most recent sample. Caption is informational
// (not promotional) — small font, default text colour. The engine
// label resolution: "speedtest_go" → "speedtest-go" (display form);
// any other engine string → "Ookla CLI" (catches "ookla_cli" and
// any future legacy values).
//
// This is a cross-reference test: it greps DashboardJS for the
// caption invariants so a future refactor can't accidentally drop
// the line without flagging.
func TestDashboardJS_SpeedTestWidget_RendersEngineCaption(t *testing.T) {
	body := extractSpeedtestSection(t)

	// 1) Section must read r.engine from the latest result.
	if !strings.Contains(body, "r.engine") {
		t.Error("DashboardJS: speedtest section does not reference r.engine — caption regression")
	}
	// 2) The two human-readable engine labels must be present.
	if !strings.Contains(body, "speedtest-go") {
		t.Error("DashboardJS: speedtest section does not contain the 'speedtest-go' display label")
	}
	if !strings.Contains(body, "Ookla CLI") {
		t.Error("DashboardJS: speedtest section does not contain the 'Ookla CLI' display label")
	}
	// 3) The caption must use the "via X" preposition so users see it
	//    as informational rather than as a settings affordance. Match
	//    the trailing-space literal "via " — emitted as part of the
	//    span content (e.g. `>via ` + engineLabel).
	if !strings.Contains(body, "via ") {
		t.Error("DashboardJS: speedtest section does not render 'via …' caption text")
	}
	// 4) data-speedtest-engine attribute carries the raw engine value
	//    (not the display label) so future Playwright UAT can pin
	//    the actual engine without parsing display text.
	if !strings.Contains(body, "data-speedtest-engine") {
		t.Error("DashboardJS: speedtest section does not emit data-speedtest-engine attribute — automation hook missing")
	}
}

// TestDashboardJS_SpeedTestWidget_EngineCaptionGracefulFallback
// asserts the caption is gated on r.engine being truthy. Pre-#284
// rows (no engine field) should not render an empty "via " caption —
// the closure prefix-checks r.engine before emitting anything.
func TestDashboardJS_SpeedTestWidget_EngineCaptionGracefulFallback(t *testing.T) {
	body := extractSpeedtestSection(t)

	// The if-gate on r.engine must come BEFORE the rendered "via "
	// literal so pre-#284 rows (no engine field) skip the caption
	// entirely. Find the FIRST string-emitting "via " — i.e. one
	// adjacent to a quote character or angle bracket — to skip past
	// the doc-comment's "via {engine}" text reference.
	guardIdx := strings.Index(body, "if (r.engine)")
	if guardIdx < 0 {
		t.Fatal("DashboardJS: 'if (r.engine)' guard missing — empty Engine field would render bare 'via undefined'")
	}
	// Search for a "via " that follows the guard.
	postGuard := body[guardIdx:]
	viaIdx := strings.Index(postGuard, "via ")
	if viaIdx < 0 {
		t.Error("DashboardJS: no 'via ' literal emitted within the r.engine-guarded block")
	}
}
