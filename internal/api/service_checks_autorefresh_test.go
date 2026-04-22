package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestServiceChecksHTML_AutoRefreshPausesWhenRowExpanded guards the fix
// for issue #187. The Service Checks page used to call
// `setInterval(loadAll, 60000)` unconditionally, which rebuilt the log
// table DOM every minute — collapsing any log row the user had expanded
// to inspect diagnostic details (status codes, DNS records, etc. added
// in #182/#183).
//
// The fix pauses the auto-refresh tick while any log row is expanded,
// and resumes once all rows are collapsed. This is a grep-based cross-
// reference test: it can't simulate a live tick, but it asserts the
// structural invariants that MUST be present for the fix to work. If a
// future refactor removes any of these, the guard silently no-ops and
// the bug regresses without any other test noticing.
//
// Invariants:
//  1. `setInterval` does NOT pass `loadAll` directly as its callback.
//     There must be an anonymous function or wrapper in between that
//     can early-return when rows are expanded. Direct
//     `setInterval(loadAll, N)` is the exact pre-fix code path.
//  2. The template declares an expanded-row state (`expandedRows`).
//     Without a shared state, the refresh guard has nothing to consult.
//  3. `toggleLogDetail` — the click handler on each log row — mutates
//     that state. If the handler ignores the state, it never changes
//     and the guard triggers either always or never.
//  4. A visual affordance (`refresh-paused`) exists somewhere in the
//     template so the user can tell WHY data isn't ticking forward.
//     Acceptance criterion from the issue body.
func TestServiceChecksHTML_AutoRefreshPausesWhenRowExpanded(t *testing.T) {
	html := loadServiceChecksHTML(t)

	// Invariant 1: the refresh tick is wrapped, not a direct callback.
	// `setInterval(loadAll,` (with optional whitespace) is the exact
	// pattern that caused the bug; we reject it outright.
	directCall := regexp.MustCompile(`setInterval\s*\(\s*loadAll\s*,`)
	if directCall.MatchString(html) {
		t.Error("setInterval(loadAll, ...) passes loadAll directly as the tick callback — " +
			"there's no way for the tick to early-return when a log row is expanded (#187)")
	}

	// A setInterval call must still exist — we didn't want to delete
	// auto-refresh entirely, just guard it.
	if !strings.Contains(html, "setInterval(") {
		t.Error("service_checks.html has no setInterval call — auto-refresh was removed entirely, which is not the intended fix")
	}

	// Invariant 2: expanded-row state variable exists.
	if !strings.Contains(html, "expandedRows") {
		t.Error("service_checks.html does not declare `expandedRows` state — the refresh guard has nothing to consult (#187)")
	}

	// Invariant 3: toggleLogDetail updates expandedRows.
	toggleBody := extractFuncBody(t, html, "window.toggleLogDetail=function(")
	if !strings.Contains(toggleBody, "expandedRows") {
		t.Errorf("window.toggleLogDetail does not reference `expandedRows` — the click handler "+
			"never updates expanded state, so the refresh guard can't know when rows are open. Body was:\n  %s", toggleBody)
	}

	// Invariant 4: a visible "paused" affordance exists. The user
	// needs to know why the page isn't auto-refreshing.
	if !strings.Contains(html, "refresh-paused") {
		t.Error("service_checks.html has no `refresh-paused` indicator element or class — " +
			"users will see stale data with no explanation (#187 acceptance criterion)")
	}
}

// extractFuncBody returns the body (between { and the matching }) of
// the first JS function whose declaration starts with `prefix`. Balances
// braces so nested objects/functions inside the body don't confuse the
// extraction. Fails the test if the prefix is not present.
func extractFuncBody(t *testing.T, html, prefix string) string {
	t.Helper()
	start := strings.Index(html, prefix)
	if start < 0 {
		t.Fatalf("function prefix %q not found in service_checks.html — template was restructured?", prefix)
	}
	// Find the opening `{` that closes the function signature.
	openBrace := strings.Index(html[start:], "){")
	if openBrace < 0 {
		t.Fatalf("function %q has no opening brace — malformed JS?", prefix)
	}
	cursor := start + openBrace + 2 // position just after `){`
	depth := 1
	for cursor < len(html) && depth > 0 {
		switch html[cursor] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return html[start+openBrace+2 : cursor]
			}
		}
		cursor++
	}
	t.Fatalf("function %q never closed — brace mismatch in service_checks.html", prefix)
	return ""
}
