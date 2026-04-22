package api

import (
	"strings"
	"testing"
)

// Issue #210 (item 6): the speed-test widget must render a "Running
// initial speed test..." state when the scheduler has marked an attempt
// as pending but no history row has landed yet. This is the fresh-
// install first-boot gap — without this branch, the widget silently
// rendered an empty tile and users had no signal that the feature
// was active.
//
// This is a cross-reference test: we grep the embedded DashboardJS
// string constant for the pending-branch invariants. Lets future
// refactors catch if the pending render path disappears or regresses
// in form.

func TestDashboardJS_SpeedTestWidget_HasPendingRenderBranch(t *testing.T) {
	js := DashboardJS

	// The pending branch must inspect spd.last_attempt.status.
	if !strings.Contains(js, "spd.last_attempt") {
		t.Error("DashboardJS: speedtest widget does not reference spd.last_attempt; pending-state render path is missing")
	}
	if !strings.Contains(js, "'pending'") {
		t.Error("DashboardJS: no string literal 'pending' found; pending-state discriminator is missing")
	}

	// The user-visible copy. If this string changes (e.g. translations),
	// update the test — but at least the refactor has to touch the test,
	// not silently disappear.
	if !strings.Contains(js, "Running initial speed test") {
		t.Error("DashboardJS: speedtest widget does not render 'Running initial speed test' copy for pending state")
	}
}

// Regression guard: the existing happy-path render (spd.available &&
// spd.latest) must survive the pending-branch addition. If the widget
// ever renders ONLY the pending state it would be a regression — users
// with real data need to keep seeing their charts.
func TestDashboardJS_SpeedTestWidget_HappyPathIntact(t *testing.T) {
	js := DashboardJS

	if !strings.Contains(js, "spd.available && spd.latest") {
		t.Error("DashboardJS: speedtest widget happy-path gate 'spd.available && spd.latest' missing; users with real speed data would see pending render instead of their chart")
	}
	// The download_mbps render is a load-bearing piece of the happy path.
	if !strings.Contains(js, "download_mbps.toFixed") {
		t.Error("DashboardJS: speedtest widget no longer renders download_mbps; happy path is broken")
	}
}
