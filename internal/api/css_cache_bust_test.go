package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Issue #229 — cache-bust /css/shared.css via a ?v=<version> query string
// so every release (and every rc/nightly rev) forces CF edge + browser
// caches to re-fetch. Surfaced during v0.9.7-rc3 UAT when CF served a
// 12+h stale copy of shared.css and hid the new .pill-trace styling.
//
// Two axes of coverage:
//
//  1. Template source: every template file under internal/api/templates/
//     that contains a `/css/shared.css` link MUST have `?v=__VERSION__`
//     appended. The `__VERSION__` token is the project-wide placeholder
//     (same one already used for /js/dashboard.js?v=__VERSION__).
//
//  2. Serve-handler: every page handler that writes one of those
//     templates MUST substitute `__VERSION__` with s.version before
//     writing the body. Otherwise the raw placeholder ships to the
//     browser and the query-string is meaningless.
//
// If a future PR adds a new page template that links shared.css, the
// template-source test below will flag it — but the author will also
// need to wire the substitution into that template's handler (the
// per-handler integration tests are the template for that).

// stylesheetLinkRE matches: <link rel="stylesheet" href="/css/shared.css...">
// where ... is anything up to the closing quote.
var stylesheetLinkRE = regexp.MustCompile(`<link[^>]+href="(/css/shared\.css[^"]*)"`)

// TestTemplates_SharedCSSLink_UsesVersionCacheBust scans every HTML
// template in internal/api/templates/ and, for each template that
// links /css/shared.css, asserts the href ends with `?v=__VERSION__`.
// The __VERSION__ placeholder is substituted at request time by the
// corresponding page handler.
func TestTemplates_SharedCSSLink_UsesVersionCacheBust(t *testing.T) {
	templates := map[string]string{
		"alerts.html":              alertsPageHTML,
		"fleet.html":               fleetPageHTML,
		"parity.html":              parityPageHTML,
		"replacement_planner.html": replacementPlannerHTML,
		"service_checks.html":      serviceChecksPageHTML,
		"settings.html":            SettingsPage,
		"stats.html":               statsPageHTML,
	}

	const wantSuffix = "?v=__VERSION__"

	for name, body := range templates {
		matches := stylesheetLinkRE.FindAllStringSubmatch(body, -1)
		if len(matches) == 0 {
			// Template doesn't link shared.css — nothing to assert.
			// (midnight.html and clean.html fall into this bucket
			// because the dashboard themes are self-contained.)
			continue
		}
		for _, m := range matches {
			href := m[1]
			if !strings.HasSuffix(href, wantSuffix) {
				t.Errorf("%s: /css/shared.css link missing cache-bust suffix %q; got href=%q (issue #229)",
					name, wantSuffix, href)
			}
		}
	}
}

// newTestServerForPageHandlers builds a minimal Server with a known
// version string so we can assert __VERSION__ substitution against a
// concrete value rather than the placeholder. The routes mirror the
// real registration in api_extended.go.
func newTestServerForPageHandlers(t *testing.T, version string) http.Handler {
	t.Helper()
	srv := &Server{
		store:     storage.NewFakeStore(),
		logger:    slog.Default(),
		version:   version,
		startTime: time.Now(),
	}
	r := chi.NewRouter()
	r.Get("/alerts", srv.handleAlertsPage)
	r.Get("/fleet", srv.handleFleetPage)
	r.Get("/parity", srv.handleParityPage)
	r.Get("/replacement-planner", srv.handleReplacementPlannerPage)
	r.Get("/service-checks", srv.handleServiceChecksPage)
	r.Get("/settings", srv.handleSettingsPage)
	r.Get("/stats", srv.handleStatsPage)
	return r
}

// TestPageHandlers_SubstituteVersionInSharedCSSLink is the integration
// half of the fix: ensures that the rendered HTML response, as a
// browser would see it, contains the concrete version in the
// shared.css href — NOT the raw __VERSION__ placeholder.
func TestPageHandlers_SubstituteVersionInSharedCSSLink(t *testing.T) {
	const testVersion = "9.9.9-test"

	pages := []struct {
		name string
		path string
	}{
		{"alerts", "/alerts"},
		{"fleet", "/fleet"},
		{"parity", "/parity"},
		{"replacement_planner", "/replacement-planner"},
		{"service_checks", "/service-checks"},
		{"settings", "/settings"},
		{"stats", "/stats"},
	}

	handler := newTestServerForPageHandlers(t, testVersion)

	wantHref := "/css/shared.css?v=" + testVersion
	badHref := "/css/shared.css?v=__VERSION__"

	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", p.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: status=%d, want 200", p.path, w.Code)
			}
			body := w.Body.String()

			// Must contain the substituted href.
			if !strings.Contains(body, wantHref) {
				t.Errorf("GET %s: response missing substituted CSS link %q (issue #229)", p.path, wantHref)
			}
			// Must NOT contain the raw placeholder — that would mean
			// the handler is writing the template bytes without running
			// the substitution.
			if strings.Contains(body, badHref) {
				t.Errorf("GET %s: response still contains raw placeholder %q — handler is not substituting __VERSION__ (issue #229)", p.path, badHref)
			}
		})
	}
}
