package api

// Issue #304 — pin the dashboard's Cancel button wiring + theme
// CSS parity.
//
// Three properties matter:
//
//  1. The button is rendered with data-action="speedtest-cancel" so
//     the body-level click delegation routes to the cancel handler
//     (mirrors the speedtest-run-now pattern).
//  2. DashboardJS attaches a listener on the `cancelled` SSE event so
//     the strip's terminal state is finalised cleanly when the
//     server confirms the abort. Without this, a Cancel that races
//     against the runner's natural completion would leave the strip
//     in an indeterminate state.
//  3. The CSS rule that flips the button from cursor:not-allowed to
//     cursor:pointer must exist verbatim in BOTH midnight.html AND
//     clean.html (theme-template parity per v0.9.7 lesson — themes
//     don't link shared.css). Defense-in-depth against a future
//     refactor that drops one theme's CSS.

import (
	"strings"
	"testing"
)

// TestDashboardJS_SpeedtestCancelButton_HasDataAction pins the
// data-action attribute so the body-level click listener picks up
// the click. Without it, the button is inert (gone back to the
// pre-#304 behaviour).
func TestDashboardJS_SpeedtestCancelButton_HasDataAction(t *testing.T) {
	if !strings.Contains(DashboardJS, `data-action="speedtest-cancel"`) {
		t.Error("DashboardJS missing data-action='speedtest-cancel' on the cancel button")
	}
}

// TestDashboardJS_SpeedtestCancel_ListenerRegistered ensures the JS
// module attaches a `cancelled` event listener on the EventSource so
// the strip transitions to idle on a server-confirmed abort.
func TestDashboardJS_SpeedtestCancel_ListenerRegistered(t *testing.T) {
	if !strings.Contains(DashboardJS, `addEventListener('cancelled'`) {
		t.Error("DashboardJS missing addEventListener('cancelled', ...) on the EventSource")
	}
	if !strings.Contains(DashboardJS, `function onCancelled`) {
		t.Error("DashboardJS missing onCancelled handler — strip won't finalise on cancel")
	}
}

// TestDashboardJS_SpeedtestCancel_PostsToCancelEndpoint pins the URL
// the click handler hits, so a refactor that splits the URL across
// helpers can't silently target the wrong path.
func TestDashboardJS_SpeedtestCancel_PostsToCancelEndpoint(t *testing.T) {
	if !strings.Contains(DashboardJS, `'/api/v1/speedtest/cancel/'`) {
		t.Error("DashboardJS does not POST to /api/v1/speedtest/cancel/<id>")
	}
}

// TestDashboardJS_SpeedtestCancel_EnableStateOnStart pins the
// transition from disabled to enabled when a test starts streaming.
// The button's initial disabled state is preserved (idle), enabled
// inside onStart, then re-disabled inside onEnd / onError /
// onCancelled.
func TestDashboardJS_SpeedtestCancel_EnableStateOnStart(t *testing.T) {
	// The setCancelEnabled helper must exist + be called from
	// onStart (enable) AND onEnd / onError / onCancelled (disable).
	for _, fragment := range []string{
		`function setCancelEnabled`,
		`setCancelEnabled(true, 'Cancel')`,
		`setCancelEnabled(false, 'Cancel')`,
	} {
		if !strings.Contains(DashboardJS, fragment) {
			t.Errorf("DashboardJS missing fragment: %q", fragment)
		}
	}
}

// TestThemes_SpeedtestCancelButton_PointerCursorParity asserts the
// `cursor:pointer` rule (which makes the button click-able from a
// UX standpoint) is present in BOTH theme templates with consistent
// values. Without this, a refactor that drops one theme's update
// reintroduces the v0.9.11 deferred-stub bug on that theme.
func TestThemes_SpeedtestCancelButton_PointerCursorParity(t *testing.T) {
	// Both themes should have:
	//   1. A .speedtest-live-cancel rule with cursor:pointer (the
	//      enabled-default state).
	//   2. A :hover:not(:disabled) rule (so the user gets feedback
	//      that hovering is meaningful).
	//   3. A :disabled rule that sets cursor:not-allowed (so the
	//      button still has the pre-test "you can't click me yet"
	//      feedback when phase=idle).
	for themeName, themeBody := range map[string]string{
		"midnight.html": DashboardMidnight,
		"clean.html":    DashboardClean,
	} {
		if !strings.Contains(themeBody, ".speedtest-live-cancel") {
			t.Errorf("%s: missing .speedtest-live-cancel rule", themeName)
		}
		if !strings.Contains(themeBody, "cursor:pointer") && !strings.Contains(themeBody, "cursor: pointer") {
			t.Errorf("%s: cancel button missing cursor:pointer (still cursor:not-allowed?)", themeName)
		}
		if !strings.Contains(themeBody, ".speedtest-live-cancel:hover:not(:disabled)") {
			t.Errorf("%s: cancel button missing :hover:not(:disabled) rule (no hover feedback)", themeName)
		}
		if !strings.Contains(themeBody, ".speedtest-live-cancel:disabled") {
			t.Errorf("%s: cancel button missing :disabled rule (idle state still needs not-allowed cursor)", themeName)
		}
	}
}

// TestThemes_SpeedtestCancelButton_NoLingeringNotAllowedOnDefault is
// the defensive check that no theme template carries the literal
// `.speedtest-live-cancel{...cursor:not-allowed}` (or its spaced
// variant) in the BASE rule. The :disabled state is allowed to keep
// not-allowed; the base rule must not.
func TestThemes_SpeedtestCancelButton_NoLingeringNotAllowedOnDefault(t *testing.T) {
	for themeName, themeBody := range map[string]string{
		"midnight.html": DashboardMidnight,
		"clean.html":    DashboardClean,
	} {
		// Find the base rule's text and verify cursor:not-allowed
		// is NOT in it. We anchor on `.speedtest-live-cancel{` /
		// `.speedtest-live-cancel ` (the base rule's selector
		// without a pseudo-class) followed by the rule body up to
		// the next `}`.
		body := themeBody
		idx := strings.Index(body, ".speedtest-live-cancel{")
		if idx == -1 {
			idx = strings.Index(body, ".speedtest-live-cancel ")
		}
		if idx == -1 {
			t.Errorf("%s: could not locate base .speedtest-live-cancel rule", themeName)
			continue
		}
		// Slice up to the next `}` to scope the assertion to the base rule.
		tail := body[idx:]
		end := strings.Index(tail, "}")
		if end == -1 {
			continue
		}
		baseRule := tail[:end]
		// Base rule must not carry not-allowed (the :disabled rule
		// later in the file may, and that's correct).
		if strings.Contains(baseRule, "not-allowed") {
			t.Errorf("%s: base .speedtest-live-cancel rule still has cursor:not-allowed (issue #304 regression). Rule: %q", themeName, baseRule)
		}
	}
}
