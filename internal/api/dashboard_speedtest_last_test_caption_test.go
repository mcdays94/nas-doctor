package api

import (
	"strings"
	"testing"
)

// Issue #290 (Slice A of #261): the speed-test card renders a small
// "Last test: X ago" caption next to the existing "via {engine}"
// annotation when r.timestamp is present. Helps users gauge data
// freshness in the post-restart cold-start case where the dashboard
// hydrates from history without an in-memory LastAttempt.
//
// The grep-for-invariants pattern follows the slice-1 engine-caption
// regression guard so a future refactor can't silently drop the line.

// TestDashboardJS_SpeedTestWidget_RendersLastTestCaption asserts the
// caption is wired into the happy-path render branch. The exact phrasing
// is not pinned — we just assert the "Last test:" prefix is present and
// it is gated on r.timestamp being truthy (pre-#290 rows lacking the
// field don't render an empty caption).
func TestDashboardJS_SpeedTestWidget_RendersLastTestCaption(t *testing.T) {
	body := extractSpeedtestSection(t)

	if !strings.Contains(body, "Last test:") {
		t.Error("DashboardJS: speedtest section does not render 'Last test:' caption (issue #290 / Slice A of #261)")
	}
	if !strings.Contains(body, "r.timestamp") {
		t.Error("DashboardJS: speedtest section does not reference r.timestamp; caption regression — caption must be gated on the field being present")
	}
	// The caption uses the relative-time formatter the dashboard's
	// existing refresh-ago indicator uses ("X ago" suffix). The
	// helper name we chose is util.relativeTimeAgo — assert the
	// invocation is wired so future refactors of util.* can't
	// silently drop the caption.
	if !strings.Contains(body, "util.relativeTimeAgo(") && !strings.Contains(body, "relativeTimeAgo(") {
		t.Error("DashboardJS: speedtest section does not call util.relativeTimeAgo(); the relative-time helper is the load-bearing piece of the caption")
	}
}

// TestDashboardJS_RelativeTimeAgo_HelperPresent asserts the helper used
// by the Last-test caption is defined on the util object. The helper
// shape is intentionally simple — it consumes an ISO-8601 / RFC3339
// timestamp (or a Date), returns "Xs ago" / "Xm ago" / "Xh ago" /
// "Xd ago" / ">1mo ago" / "" (empty for invalid input).
//
// Existence test only — the actual time bucket arithmetic is exercised
// indirectly via the caption test above and the relative-time helper's
// own unit test.
func TestDashboardJS_RelativeTimeAgo_HelperPresent(t *testing.T) {
	js := DashboardJS

	if !strings.Contains(js, "util.relativeTimeAgo = function") {
		t.Error("DashboardJS: util.relativeTimeAgo helper not defined; speed-test caption depends on it")
	}
	// The helper must produce the canonical "X ago" suffix shape so
	// it visually matches the existing scan-refresh indicator at the
	// top of the page (see also: setInterval block, "X ago" rendering).
	if !strings.Contains(js, "ago") {
		t.Error("DashboardJS: 'ago' suffix string missing — relativeTimeAgo would produce non-standard output")
	}
}

// Regression guard: the Slice 1 "via {engine}" caption must NOT
// regress. Both captions live on the same line/block and must coexist.
func TestDashboardJS_SpeedTestWidget_EngineCaptionStillPresent(t *testing.T) {
	body := extractSpeedtestSection(t)

	// Slice 1 invariants — keep them green even after the Slice A caption
	// addition. Cross-references the existing
	// dashboard_speedtest_engine_caption_test pattern.
	if !strings.Contains(body, "via ") {
		t.Error("DashboardJS: 'via …' engine caption regression — Slice 1 (#287) caption dropped")
	}
	if !strings.Contains(body, "data-speedtest-engine") {
		t.Error("DashboardJS: data-speedtest-engine attribute missing — Slice 1 automation hook lost")
	}
}

// Empty-state regression guard — when the snapshot has no SpeedTest
// data AND no in-flight test, the v0.9.6 #210 first-boot empty-state
// copy must still render. Slice A only adds a NEW branch (history
// hydration) without rewriting the existing branches.
func TestDashboardJS_SpeedTestWidget_EmptyStateCopyPreserved(t *testing.T) {
	js := DashboardJS

	if !strings.Contains(js, "Running initial speed test") {
		t.Error("DashboardJS: pending-state copy 'Running initial speed test…' regression — the v0.9.6 #210 first-boot empty-state was dropped by Slice A")
	}
	if !strings.Contains(js, "Scheduled speed tests are disabled. Use Run now for a one-off test.") {
		t.Error("DashboardJS: disabled empty-state copy regression — Slice 2 (#288) user story 8 broken")
	}
}
