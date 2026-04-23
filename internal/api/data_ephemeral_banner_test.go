package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #227 — "/data is not a persistent bind-mount" detection and UI banner.
//
// These tests lock in three cross-layer invariants so a future refactor
// can't silently break the user-visible banner (the whole point of the
// feature — catch the silent-data-loss footgun loudly, as documented in
// the AGENTS.md two-axis test pattern).
//
//  1. Server has a SetDataPersistent setter consumed by main.go.
//  2. /api/v1/status exposes data_ephemeral so the browser JS can react.
//  3. DashboardJS renders the banner when data_ephemeral is true, in
//     BOTH theme templates (midnight + clean), because the dashboard
//     theme templates don't link shared.css (see AGENTS.md). Any class
//     the JS references must have its CSS rule inlined in each theme.

// newEphemeralBannerTestServer builds a minimal Server for exercising
// /api/v1/status against an in-memory FakeStore.
func newEphemeralBannerTestServer() *Server {
	return &Server{
		store:     storage.NewFakeStore(),
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
}

// TestSetDataPersistent_DefaultsToPersistent verifies the zero-value of
// Server.dataPersistent is true. Most deployments are correctly
// bind-mounted, and the field is set during main.go startup; if the
// startup check doesn't run (demo mode, test harnesses, third-party
// embedders) we must not show a false-positive banner.
func TestSetDataPersistent_DefaultsToPersistent(t *testing.T) {
	srv := newEphemeralBannerTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/status returned %d: %s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// A zero-value Server (no SetDataPersistent called) must not claim
	// /data is ephemeral — this protects unrelated tests that spin up
	// bare Servers from suddenly rendering a scary banner in fixtures.
	if eph, ok := parsed["data_ephemeral"].(bool); ok && eph {
		t.Errorf("zero-value Server should not report data_ephemeral=true; got %v", parsed["data_ephemeral"])
	}
}

// TestSetDataPersistent_FalseExposedAsEphemeralTrue verifies that when
// main.go determines /data is on the overlay fs and calls
// SetDataPersistent(false), the /api/v1/status response propagates
// data_ephemeral=true to the browser.
func TestSetDataPersistent_FalseExposedAsEphemeralTrue(t *testing.T) {
	srv := newEphemeralBannerTestServer()
	srv.SetDataPersistent(false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	eph, ok := parsed["data_ephemeral"].(bool)
	if !ok {
		t.Fatalf("data_ephemeral missing from /api/v1/status response: %v", parsed)
	}
	if !eph {
		t.Errorf("expected data_ephemeral=true after SetDataPersistent(false), got %v", eph)
	}
}

// TestSetDataPersistent_TrueExposedAsEphemeralFalse verifies the happy
// path — when /data IS a real bind-mount, data_ephemeral is false (or
// absent via omitempty), so no banner appears.
func TestSetDataPersistent_TrueExposedAsEphemeralFalse(t *testing.T) {
	srv := newEphemeralBannerTestServer()
	srv.SetDataPersistent(true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, req)
	body, _ := io.ReadAll(rec.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Either absent (omitempty) or explicitly false — both are fine.
	if eph, present := parsed["data_ephemeral"].(bool); present && eph {
		t.Errorf("expected data_ephemeral=false (or absent) after SetDataPersistent(true), got %v", eph)
	}
}

// TestDashboardJS_RendersEphemeralBannerWhenFlagSet verifies the browser
// JS reads data_ephemeral from the status payload and renders a warning
// banner. We assert three substrings:
//
//  1. The JS reads `data_ephemeral` from statusData (otherwise the
//     banner would never appear regardless of server state).
//  2. The banner contains the word "persistent" or "bind-mount" so the
//     user understands what to fix.
//  3. The banner uses a class/id we can cross-reference with theme CSS.
func TestDashboardJS_RendersEphemeralBannerWhenFlagSet(t *testing.T) {
	js := DashboardJS

	if !strings.Contains(js, "data_ephemeral") {
		t.Errorf("DashboardJS must read data_ephemeral from statusData to render the ephemeral /data banner (issue #227)")
	}

	// The banner markup must include an identifiable CSS hook so the
	// theme templates can style it. "data-ephemeral-banner" is the
	// canonical attribute — if a refactor renames it, update BOTH the
	// JS and both theme templates. See theme-parity test below.
	hookRE := regexp.MustCompile(`data-ephemeral-banner|id=["']ephemeral-data-banner["']|class=["'][^"']*ephemeral-data-banner`)
	if !hookRE.MatchString(js) {
		t.Errorf("DashboardJS ephemeral /data banner must carry a stable CSS hook (data-ephemeral-banner attr or id=ephemeral-data-banner class). Found none — theme-parity test can't lock the styling in.")
	}

	// The banner must name the problem so a user can act on it. We
	// accept either phrase but require at least one — grep-based.
	lowerJS := strings.ToLower(js)
	if !strings.Contains(lowerJS, "persistent") && !strings.Contains(lowerJS, "bind-mount") {
		t.Errorf("DashboardJS ephemeral /data banner must include guidance (\"persistent\" / \"bind-mount\") so users know what to fix")
	}
}

// TestThemeTemplates_StyleEphemeralBanner verifies both dashboard theme
// templates (midnight + clean) carry a CSS rule for the ephemeral-data
// banner hook emitted by DashboardJS. This is the second axis of the
// two-axis test pattern called out in AGENTS.md: asserting "JS emits
// class X" (above) is not the same as asserting "class X is styled on
// every page where it appears". Dashboard theme templates intentionally
// do not link shared.css, so rules must be inlined in each.
func TestThemeTemplates_StyleEphemeralBanner(t *testing.T) {
	files := []string{
		filepath.Join("templates", "midnight.html"),
		filepath.Join("templates", "clean.html"),
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			content := string(data)
			// Must style the selector emitted by DashboardJS.
			if !strings.Contains(content, "ephemeral-data-banner") {
				t.Errorf("%s must style the ephemeral-data-banner selector; DashboardJS emits the class/attr but this theme inlines no matching CSS rule, so the banner would render as unstyled plain text on the dashboard (see AGENTS.md on theme templates not linking shared.css)", f)
			}
		})
	}
}
