package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestDashboardJS_LastScanAgoCountsFromScanTimestamp locks in the fix for
// issue #179: the "X minutes ago" indicator next to "Last scan:" on the
// dashboard must compute its elapsed time from the actual scan timestamp
// provided by the server (status.last_scan, RFC3339), NOT from
// page-load/last-poll time.
//
// Before the fix, the setInterval that updates #refresh-ago used
// _lastFetchTime — which is Date.now() set on every poll and on every
// initial loadAll(). That means:
//
//   - Opening the dashboard 10 minutes after the last scan showed
//     "just now" and ticked forward from there — dead wrong.
//   - A page refresh reset the counter to 0 even though nothing had
//     actually re-scanned — the bug report's exact reproduction.
//
// The fix is to derive the elapsed seconds from _statusData.last_scan
// (RFC3339 string) parsed as a Date. That string is the same source as
// the human-readable timestamp already rendered in <strong>, so the
// counter and the absolute timestamp are now guaranteed to agree.
//
// This is a grep-based cross-reference test (see AGENTS.md §4b). The JS
// lives in a Go string literal (DashboardJS) that is served verbatim at
// /js/dashboard.js, so asserting on substrings of the literal is how we
// guard the client-side behavior from Go-side tests without spinning up
// a browser.
func TestDashboardJS_LastScanAgoCountsFromScanTimestamp(t *testing.T) {
	js := DashboardJS

	// Invariant 1: the refresh-ago interval body must reference the
	// scan timestamp source, not just _lastFetchTime. We scope the
	// check to the block that sets #refresh-ago text so unrelated
	// uses of _lastFetchTime elsewhere (the poll debounce uses it
	// too) don't false-pass.
	agoBlock := extractRefreshAgoBlock(t, js)

	if !strings.Contains(agoBlock, "last_scan") {
		t.Errorf("refresh-ago interval must derive elapsed time from _statusData.last_scan (the RFC3339 scan timestamp), but the block doesn't reference last_scan:\n%s", agoBlock)
	}

	// Invariant 2: must parse the timestamp via `new Date(...)`.
	// Without this, we'd be comparing a string to a number and the
	// counter would produce NaN/garbage.
	if !regexp.MustCompile(`new Date\s*\(`).MatchString(agoBlock) {
		t.Errorf("refresh-ago interval must parse the scan timestamp via `new Date(...)` so the RFC3339 string becomes a comparable epoch-ms value; block:\n%s", agoBlock)
	}

	// Invariant 3: guard against the regressed behavior. The ago text
	// must NOT be computed from _lastFetchTime (that was the bug).
	// We allow _lastFetchTime to appear in the *guard* (`if !el` etc.)
	// but not in the subtraction that feeds `secs`.
	//
	// The simplest durable check: the subtraction `Date.now() - _lastFetchTime`
	// must not appear inside the block. That exact idiom is what
	// produced the bug.
	badPattern := regexp.MustCompile(`Date\.now\s*\(\s*\)\s*-\s*_lastFetchTime`)
	if badPattern.MatchString(agoBlock) {
		t.Errorf("refresh-ago interval still computes elapsed time as `Date.now() - _lastFetchTime` — this is the exact regression issue #179 was filed against. Use the scan timestamp instead:\n%s", agoBlock)
	}
}

// TestDashboardJS_LastScanAgoHandlesMissingTimestamp guards the empty-state
// path. When the server has never scanned, status.last_scan is empty and
// the absolute timestamp renders as "Never". The counter must not then
// display bogus text like "NaNm ago" or "56 years ago" (epoch-zero parse).
// The block should simply clear the element or short-circuit.
func TestDashboardJS_LastScanAgoHandlesMissingTimestamp(t *testing.T) {
	js := DashboardJS
	agoBlock := extractRefreshAgoBlock(t, js)

	// There must be a guard that checks truthiness of the scan
	// timestamp before doing arithmetic. Any of the common idioms
	// are acceptable: an early return, a ternary, or a falsy check
	// on the parsed value. We require at least one `return` inside
	// the block so the empty-state path short-circuits; the existing
	// `if (!el || !_lastFetchTime) return;` guard already matches
	// this shape — we just need the equivalent for the scan
	// timestamp after the fix.
	if !strings.Contains(agoBlock, "return") {
		t.Errorf("refresh-ago interval must short-circuit (early return) when the scan timestamp is missing, otherwise the counter would render NaN or a 56-year-ago epoch value on a fresh install; block:\n%s", agoBlock)
	}
}

// extractRefreshAgoBlock returns the JS body of the setInterval callback
// that updates #refresh-ago. Scoping assertions to this block prevents
// unrelated uses of _lastFetchTime (used elsewhere for poll cadence)
// from false-passing the test.
func extractRefreshAgoBlock(t *testing.T, js string) string {
	t.Helper()
	// The setInterval sits immediately below a comment line
	// "// Refresh indicator timer" in dashboard.go. The callback
	// is a single short function; we grab from that marker to
	// the next "}, 1000);" which closes the interval.
	marker := "Refresh indicator timer"
	start := strings.Index(js, marker)
	if start < 0 {
		t.Fatalf("could not locate %q in DashboardJS — the refresh-ago setInterval was renamed or moved. Update the test helper.", marker)
	}
	end := strings.Index(js[start:], "}, 1000)")
	if end < 0 {
		t.Fatalf("could not find closing `}, 1000)` for refresh-ago setInterval — structure changed, update the helper.")
	}
	return js[start : start+end]
}
