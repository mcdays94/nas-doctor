package api

import (
	"strings"
	"testing"
)

// TestDashboardJS_SpeedtestLive_PanelRendered asserts the speed-test card
// emits the empty #speedtest-live element that speedtestLive fills with
// the shared NasSpeedLive panel, marks the content the panel replaces
// while it is open, and refills the panel after every render. It replaced
// the PRD #283 gauge-and-sparkline strip (issue #346 follow-up).
func TestDashboardJS_SpeedtestLive_PanelRendered(t *testing.T) {
	required := []string{
		`'<div id="speedtest-live" hidden></div>'`,
		`data-speedtest-summary`,
		`data-action="speedtest-run-now"`,
		`window.NasSpeedLive.create({`,
		`compact: true`,
		`speedtestLive.redraw()`,
	}
	for _, fragment := range required {
		if !strings.Contains(DashboardJS, fragment) {
			t.Errorf("DashboardJS missing live panel fragment: %q", fragment)
		}
	}
	for _, gone := range []string{"speedtest-live-strip", "speedtest-live-gauge", "speedtest-live-spark", "NasChart.gauge('speedtest-live"} {
		if strings.Contains(DashboardJS, gone) {
			t.Errorf("DashboardJS still has the old strip: %q", gone)
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

// TestChartJS_SpeedLivePanelStyles asserts the panel's stylesheet ships
// with the component. NasSpeedLive injects it because the dashboard
// themes don't link /css/shared.css (the v0.9.7 rc5/rc6 lesson), so one
// copy in charts.js styles the panel on the dashboard and in Settings.
func TestChartJS_SpeedLivePanelStyles(t *testing.T) {
	required := []string{
		"function injectSpeedLiveCSS",
		".speed-live{",
		".speed-live[data-tone=light]{",
		".speed-live[hidden],.speed-live [hidden]{display:none}",
		".speed-live-step[data-state=active][data-step=download]",
		".speed-live-readouts{",
		".speed-live-chart{",
		".speed-live-compact{",
		".speed-live-compact .speed-live-chart{height:120px}",
	}
	for _, rule := range required {
		if !strings.Contains(ChartJS, rule) {
			t.Errorf("ChartJS missing panel style: %q", rule)
		}
	}
}

// TestDashboardCSS_NoStaleStripRules makes sure the strip's CSS left with
// the strip, from shared.css and from both theme templates.
func TestDashboardCSS_NoStaleStripRules(t *testing.T) {
	for name, body := range map[string]string{
		"shared.css":    SharedCSS,
		"midnight.html": DashboardMidnight,
		"clean.html":    DashboardClean,
	} {
		if strings.Contains(body, ".speedtest-live-") {
			t.Errorf("%s still has .speedtest-live-* rules for the removed strip", name)
		}
	}
}

// TestDashboardJS_SpeedtestHistory_FallsBackToAWindowWithTests pins the
// history chart's range fallback. Speed tests run a few times a day, so
// the default 1H window is usually empty and the chart used to stay
// blank with no explanation.
func TestDashboardJS_SpeedtestHistory_FallsBackToAWindowWithTests(t *testing.T) {
	required := []string{
		`var SPEEDTEST_FALLBACK_HOURS = [24, 168, 720];`,
		`id="speedtest-chart-note"`,
		`"No speed tests in the last "`,
		`", showing the last "`,
		`p.download_mbps > 0 || p.upload_mbps > 0`,
	}
	for _, fragment := range required {
		if !strings.Contains(DashboardJS, fragment) {
			t.Errorf("DashboardJS missing history fallback fragment: %q", fragment)
		}
	}
}

// TestDashboardJS_SpeedtestLive_RefreshesOnlyItsCard guards against a
// full dashboard render when a test finishes. A full render fades every
// section in again and resets the scroll position, so the page jumped
// away from the card right as the result appeared.
func TestDashboardJS_SpeedtestLive_RefreshesOnlyItsCard(t *testing.T) {
	start := strings.Index(DashboardJS, "var speedtestLive = (function() {")
	end := strings.Index(DashboardJS, "window.speedtestLive = speedtestLive;")
	if start < 0 || end < start {
		t.Fatal("could not locate the speedtestLive module in DashboardJS")
	}
	module := DashboardJS[start:end]
	if !strings.Contains(module, "function refreshCard()") {
		t.Error("speedtestLive has no refreshCard; the card's figures won't update after a test")
	}
	for _, full := range []string{"polling.loadAll(", "_renderFn("} {
		if strings.Contains(module, full) {
			t.Errorf("speedtestLive calls %s; refresh only the speed-test card", full)
		}
	}
}
