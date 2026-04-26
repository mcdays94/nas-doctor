package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_SpeedtestLive_StripPlaceholderRendered asserts the
// DashboardJS speedtest section emits the live-progress strip
// placeholder div with the expected child elements (phase pill, gauge
// canvas, sparkline canvas, Cancel button). Renders the strip in
// data-state="idle" by default — the speedtestLive module flips it
// to "running" on Run-now click. PRD #283 / issue #285.
func TestDashboardJS_SpeedtestLive_StripPlaceholderRendered(t *testing.T) {
	required := []string{
		`id="speedtest-live-strip"`,
		`class="speedtest-live-strip"`,
		`data-state="idle"`,
		`speedtest-live-phase-pill`,
		`id="speedtest-live-gauge"`,
		`id="speedtest-live-spark"`,
		`id="speedtest-live-cancel"`,
		`data-action="speedtest-run-now"`,
	}
	for _, fragment := range required {
		if !strings.Contains(DashboardJS, fragment) {
			t.Errorf("DashboardJS missing required strip fragment: %q", fragment)
		}
	}
}

// TestDashboardJS_SpeedtestLive_DisabledEmptyStateCopy asserts the
// disabled-cron empty-state copy is present verbatim. PRD #283 user
// story 8: when the cron is "Disabled" the user must be told the
// Run-now button still works for one-off tests.
func TestDashboardJS_SpeedtestLive_DisabledEmptyStateCopy(t *testing.T) {
	want := "Scheduled speed tests are disabled. Use Run now for a one-off test."
	if !strings.Contains(DashboardJS, want) {
		t.Errorf("DashboardJS missing disabled empty-state copy: %q", want)
	}
}

// TestDashboardJS_SpeedtestLive_EventSourceWired asserts the
// EventSource lifecycle hooks are registered for the documented
// event names. We don't run the JS — we just look for the addEventListener
// calls verbatim. Defends against accidental rename of the SSE wire
// event names by either the Go handler or the dashboard JS.
func TestDashboardJS_SpeedtestLive_EventSourceWired(t *testing.T) {
	required := []string{
		`new EventSource('/api/v1/speedtest/stream/'`,
		`addEventListener('start'`,
		`addEventListener('phase_change'`,
		`addEventListener('sample'`,
		`addEventListener('result'`,
		`addEventListener('end'`,
	}
	for _, fragment := range required {
		if !strings.Contains(DashboardJS, fragment) {
			t.Errorf("DashboardJS missing EventSource hookup: %q", fragment)
		}
	}
}

// TestSharedCSS_SpeedtestLiveStripStyles asserts the strip's CSS
// rules are present in SharedCSS. The same rules MUST also be
// inlined into both theme templates (midnight + clean) because
// dashboard themes don't link /css/shared.css — see AGENTS
// architectural note.
func TestSharedCSS_SpeedtestLiveStripStyles(t *testing.T) {
	requiredRules := []string{
		".speedtest-live-strip",
		".speedtest-live-strip[data-state=\"running\"]",
		".speedtest-live-phase-pill",
		".speedtest-live-mbps",
		".speedtest-live-cancel",
	}
	for _, rule := range requiredRules {
		if !strings.Contains(SharedCSS, rule) {
			t.Errorf("SharedCSS missing rule: %q", rule)
		}
	}
}

// TestThemes_SpeedtestLiveStripStyles asserts the strip's CSS rules
// are duplicated into BOTH dashboard theme templates. Pinning prevents
// the v0.9.7 rc5/rc6 class of bug where new dashboard CSS exists in
// shared.css but renders as plain text on the dashboard because the
// themes don't link shared.css.
func TestThemes_SpeedtestLiveStripStyles(t *testing.T) {
	requiredRules := []string{
		".speedtest-live-strip",
		".speedtest-live-strip[data-state=\"running\"]",
		".speedtest-live-phase-pill",
		".speedtest-live-mbps",
	}
	for _, rule := range requiredRules {
		if !strings.Contains(DashboardMidnight, rule) {
			t.Errorf("midnight.html missing rule: %q", rule)
		}
		if !strings.Contains(DashboardClean, rule) {
			t.Errorf("clean.html missing rule: %q", rule)
		}
	}
}
