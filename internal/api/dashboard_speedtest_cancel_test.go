package api

// Issue #304 — pin the dashboard's Cancel button wiring.
//
// Three properties matter:
//
//  1. The live panel's Cancel button routes to the cancel handler.
//     The button belongs to the shared NasSpeedLive panel, which calls
//     onStop while a test runs.
//  2. DashboardJS attaches a listener on the `cancelled` SSE event so
//     the panel's terminal state is finalised cleanly when the
//     server confirms the abort. Without this, a Cancel that races
//     against the runner's natural completion would leave the panel
//     in an indeterminate state.
//  3. The button's base rule uses cursor:pointer, and only its
//     :disabled rule uses cursor:not-allowed. The rules ship in
//     ChartJS with the panel, which injects them on every page, so
//     the theme-parity problem (themes don't link shared.css) is gone.

import (
	"strings"
	"testing"
)

// TestDashboardJS_SpeedtestCancel_WiredToPanelStop pins the route from
// the live panel's Cancel button to the cancel handler. The panel calls
// onStop while a test runs; without this wiring the button is inert
// (the pre-#304 behaviour).
func TestDashboardJS_SpeedtestCancel_WiredToPanelStop(t *testing.T) {
	for _, fragment := range []string{`stopLabel: 'Cancel'`, `onStop: function() { cancel(); }`} {
		if !strings.Contains(DashboardJS, fragment) {
			t.Errorf("DashboardJS missing Cancel wiring: %q", fragment)
		}
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

// TestChartJS_SpeedLiveStopButton_Cursor asserts the panel's Cancel/Stop
// button reads as clickable while a test runs and as unavailable while
// it is disabled (between a click and the server's cancelled event).
// The button's styles ship with NasSpeedLive, so they hold on both
// dashboard themes and in Settings.
func TestChartJS_SpeedLiveStopButton_Cursor(t *testing.T) {
	idx := strings.Index(ChartJS, ".speed-live-stop{")
	if idx == -1 {
		t.Fatal("ChartJS: could not locate the base .speed-live-stop rule")
	}
	base := ChartJS[idx:]
	base = base[:strings.Index(base, "}")]
	if !strings.Contains(base, "cursor:pointer") {
		t.Errorf("base .speed-live-stop rule lacks cursor:pointer: %q", base)
	}
	// Issue #304 regression: not-allowed belongs on :disabled only.
	if strings.Contains(base, "not-allowed") {
		t.Errorf("base .speed-live-stop rule has cursor:not-allowed: %q", base)
	}
	if !strings.Contains(ChartJS, ".speed-live-stop:hover:not(:disabled){") {
		t.Error("ChartJS missing .speed-live-stop:hover:not(:disabled) rule (no hover feedback)")
	}
	if !strings.Contains(ChartJS, ".speed-live-stop:disabled{cursor:not-allowed") {
		t.Error("ChartJS missing .speed-live-stop:disabled rule with cursor:not-allowed")
	}
}
