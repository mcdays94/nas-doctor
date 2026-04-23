package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeDashboard_SetsNoCacheHeader locks in the fix for issue #234.
//
// Before the fix, serveDashboard wrote the HTML with zero cache headers.
// Browsers then applied heuristic freshness and could serve stale HTML.
// Because the HTML embeds `<script src="/js/dashboard.js?v=<VERSION>">`
// with VERSION baked at render time, stale HTML means the browser keeps
// requesting the previous version's JS URL and never revalidates against
// the newly-upgraded binary. The `?v=VERSION` cache-bust only works when
// the HTML itself is fresh.
//
// The fix mirrors the pattern already used for /js/dashboard.js and
// /js/charts.js (see api.go:125 and :132): explicitly set
// "Cache-Control: no-cache" so the browser revalidates the dashboard
// HTML on every load.
func TestServeDashboard_SetsNoCacheHeader(t *testing.T) {
	srv := &Server{version: "test"}
	w := httptest.NewRecorder()

	srv.serveDashboard(w, ThemeMidnight)

	got := w.Header().Get("Cache-Control")
	if got != "no-cache" {
		t.Errorf("serveDashboard must set Cache-Control: no-cache so the dashboard HTML revalidates on every load (otherwise the `?v=VERSION` cache-bust on /js/dashboard.js points at a stale URL after an upgrade); got Cache-Control=%q", got)
	}
}

// TestServeDashboard_SetsNoCacheHeader_CleanTheme is the parallel
// assertion for the clean theme. Both theme branches must set the
// header — the fix must not regress one while covering the other.
func TestServeDashboard_SetsNoCacheHeader_CleanTheme(t *testing.T) {
	srv := &Server{version: "test"}
	w := httptest.NewRecorder()

	srv.serveDashboard(w, ThemeClean)

	got := w.Header().Get("Cache-Control")
	if got != "no-cache" {
		t.Errorf("serveDashboard (clean theme) must also set Cache-Control: no-cache; got %q", got)
	}
}

// TestServeDashboard_PreservesExistingBehavior is a regression guard.
// Adding the new header must not displace the pre-existing
// Content-Type header or corrupt the HTML body. If a future refactor
// reorders header writes after a w.Write (which would silently drop
// them), this test catches it.
func TestServeDashboard_PreservesExistingBehavior(t *testing.T) {
	srv := &Server{version: "test-version-xyz"}
	w := httptest.NewRecorder()

	srv.serveDashboard(w, ThemeMidnight)

	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type header must remain text/html; charset=utf-8; got %q", ct)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Fatalf("serveDashboard wrote an empty body")
	}
	// The body must have had the __VERSION__ placeholder substituted.
	if strings.Contains(body, "__VERSION__") {
		t.Errorf("serveDashboard did not substitute __VERSION__ placeholder in the HTML body")
	}
	if !strings.Contains(body, "test-version-xyz") {
		t.Errorf("serveDashboard body must contain the substituted version string %q", "test-version-xyz")
	}
}
