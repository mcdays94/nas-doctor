package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// loadServiceChecksHTML returns the service_checks.html template content
// for assertions. Keeps the test hermetic (does not rely on the embedded
// FS being initialized; reads the source file directly).
func loadServiceChecksHTML(t *testing.T) string {
	t.Helper()
	path := filepath.Join("templates", "service_checks.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read service_checks.html: %v", err)
	}
	return string(data)
}

// TestServiceChecksHTML_LogDetailWrapsLongValues guards the CSS fix for
// issue #176. When a log entry is expanded on the Service Checks page,
// long field values (URLs, error messages, check keys) must wrap within
// their grid item; otherwise the unbroken string pushes the grid track
// wider than its parent card and the card visibly overflows on desktop
// and mobile alike.
//
// We assert three structural invariants in the CSS:
//  1. .log-detail-item has min-width:0 — without this, the CSS grid
//     track (grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)))
//     refuses to shrink below the intrinsic width of its unbreakable
//     content, so the grid expands horizontally beyond the card.
//  2. .log-detail-item .ld-val has overflow-wrap:anywhere and
//     word-break:break-word — together these force browsers to break
//     at any character boundary for tokens without whitespace (URLs,
//     base64 strings, etc.). Both are set because older engines honor
//     word-break while modern engines prefer overflow-wrap; shipping
//     both is the broadest-compat pattern.
//  3. .log-table td must also wrap. The un-expanded log table lives
//     inside the same .log-panel card; a long unbreakable value in a
//     row cell (e.g. the Check Name column) stretches the <table>
//     past the panel's width, which transitively pushes the
//     colspan=6 expanded detail row past the card edge — even when
//     the detail grid itself would have fit. We unset the inherited
//     white-space:nowrap from the shared stylesheet (styles.go)
//     and set word-break/overflow-wrap so no single cell can grow
//     the table beyond 100% of the panel's inner width.
//
// This is a grep-based CSS test: it does not exercise a running page
// (that's covered by §4c Playwright validation on the worker side),
// but it does prevent accidental regression if someone later refactors
// the log-detail styles and drops the wrap rules.
func TestServiceChecksHTML_LogDetailWrapsLongValues(t *testing.T) {
	html := loadServiceChecksHTML(t)

	// Invariant 1: .log-detail-item must set min-width:0 so CSS grid
	// tracks can shrink below the intrinsic width of long tokens.
	itemRule := extractCSSRule(t, html, ".log-detail-item{")
	if !strings.Contains(itemRule, "min-width:0") {
		t.Errorf(".log-detail-item CSS must include min-width:0 so grid tracks shrink below intrinsic width of long unbreakable tokens; rule was:\n  %s", itemRule)
	}

	// Invariant 2: .log-detail-item .ld-val must wrap.
	valRule := extractCSSRule(t, html, ".log-detail-item .ld-val{")
	if !regexp.MustCompile(`overflow-wrap\s*:\s*anywhere`).MatchString(valRule) {
		t.Errorf(".log-detail-item .ld-val CSS must include overflow-wrap:anywhere; rule was:\n  %s", valRule)
	}
	if !regexp.MustCompile(`word-break\s*:\s*break-word`).MatchString(valRule) {
		t.Errorf(".log-detail-item .ld-val CSS must include word-break:break-word; rule was:\n  %s", valRule)
	}

	// Invariant 3: .log-table td must override the shared stylesheet's
	// white-space:nowrap and permit character-boundary wrapping.
	tdRule := extractCSSRule(t, html, ".log-table td{")
	if !regexp.MustCompile(`white-space\s*:\s*normal`).MatchString(tdRule) {
		t.Errorf(".log-table td CSS must include white-space:normal to unset the shared stylesheet's nowrap (otherwise long cell values stretch the table past the panel card); rule was:\n  %s", tdRule)
	}
	if !regexp.MustCompile(`overflow-wrap\s*:\s*anywhere`).MatchString(tdRule) {
		t.Errorf(".log-table td CSS must include overflow-wrap:anywhere; rule was:\n  %s", tdRule)
	}
	if !regexp.MustCompile(`word-break\s*:\s*break-word`).MatchString(tdRule) {
		t.Errorf(".log-table td CSS must include word-break:break-word; rule was:\n  %s", tdRule)
	}
}

// TestServiceChecksHTML_LogDetailClassesCrossReference is the §4b
// cross-reference guard: the CSS class names styled in the <style>
// block must actually be emitted by the JS that renders expanded log
// rows. If someone renames one side without the other, logic tests
// and the CSS rule test both stay green while the fix silently stops
// applying — classic drift trap.
func TestServiceChecksHTML_LogDetailClassesCrossReference(t *testing.T) {
	html := loadServiceChecksHTML(t)

	// The JS in renderLogTable() emits both of these classes when it
	// builds the expanded detail row. If either disappears from the
	// template, the CSS wrap fix becomes a no-op.
	for _, cls := range []string{"log-detail-item", "ld-val"} {
		cssNeedle := "." + cls
		if !strings.Contains(html, cssNeedle) {
			t.Errorf("service_checks.html CSS no longer targets %q — wrap rule would no-op", cssNeedle)
		}
		jsNeedle := `class="` + cls
		// The JS builds classes via string concatenation; the literal
		// "class=\"<name>" fragment appears in the template source.
		if !strings.Contains(html, jsNeedle) && !strings.Contains(html, `"`+cls+`"`) {
			t.Errorf("service_checks.html JS no longer emits %q in rendered HTML — CSS rule would style nothing", cls)
		}
	}
}

// extractCSSRule returns the body (between { and }) of the first CSS
// rule in html whose opening matches prefix (e.g. ".log-detail-item{").
// Fails the test if the rule isn't present; returning a guaranteed
// non-empty string lets the caller assert on contents without
// dereferencing nil.
func extractCSSRule(t *testing.T, html, prefix string) string {
	t.Helper()
	start := strings.Index(html, prefix)
	if start < 0 {
		t.Fatalf("CSS rule %q not found in service_checks.html — template was restructured or rule was removed", prefix)
	}
	bodyStart := start + len(prefix)
	end := strings.Index(html[bodyStart:], "}")
	if end < 0 {
		t.Fatalf("CSS rule %q found but never closed with '}' — malformed CSS?", prefix)
	}
	return html[bodyStart : bodyStart+end]
}
