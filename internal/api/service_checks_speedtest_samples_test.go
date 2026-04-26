package api

import (
	"os/exec"
	"strings"
	"testing"
)

// TestServiceChecksTemplate_JSBlocksParse asserts every <script>
// block in service_checks.html is valid JavaScript. Same shape as the
// settings_js_parses test (added in v0.9.9). Catches the */-in-comment
// hazard from rc1 of v0.9.9 + the backtick-in-comment hazard from
// the DashboardJS raw-string literal class. Service-checks scripts
// are dense enough that a comment-syntax slip would silently break
// the entire log-table render.
func TestServiceChecksTemplate_JSBlocksParse(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node binary not in PATH; skipping JS parse check (dev-time guard only)")
	}
	blocks := extractScriptBlocks(serviceChecksPageHTML)
	if len(blocks) == 0 {
		t.Fatal("no <script>...</script> blocks found in service_checks.html")
	}
	for i, js := range blocks {
		if strings.TrimSpace(js) == "" {
			continue
		}
		cmd := exec.Command("node", "--check", "-")
		cmd.Stdin = strings.NewReader(js)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("service_checks.html <script> block %d failed to parse as JS:\n%s", i, string(out))
		}
	}
}

// TestServiceChecksHTML_RendersSpeedTestSamplesContainer asserts the
// expanded-log row for type=speed entries includes the per-sample
// mini-chart container + a stable canvas id template that the
// renderer wires up. PRD #283 slice 3 / issue #286.
func TestServiceChecksHTML_RendersSpeedTestSamplesContainer(t *testing.T) {
	html := loadServiceChecksHTML(t)

	// The container must opt into the renderer via the
	// data-speedtest-history-id attribute. Without this attribute the
	// lazy-render hook can't find the panel and the chart never draws.
	if !strings.Contains(html, "data-speedtest-history-id=") {
		t.Error("service_checks.html missing data-speedtest-history-id attribute on the speed-row mini-chart container")
	}
	// The canvas element must be addressable via a stable per-row id
	// pattern so NasChart.line can target it.
	if !strings.Contains(html, "speedtest-samples-") {
		t.Error("service_checks.html missing speedtest-samples- canvas id prefix")
	}
	// The template must reference the new endpoint, not the historical
	// chart endpoint.
	if !strings.Contains(html, "/api/v1/speedtest/samples/") {
		t.Error("service_checks.html does not call the new /api/v1/speedtest/samples/{id} endpoint")
	}
	// The renderer must reuse NasChart.line (PRD constraint: no new
	// chart library code).
	if !strings.Contains(html, "NasChart.line") {
		t.Error("service_checks.html mini-chart renderer must call NasChart.line (PRD #283 reuse-only constraint)")
	}
}

// TestServiceChecksHTML_EmptyStateCopyForLegacyEntries asserts the
// fallback hint for tests that pre-date the per-sample feature
// (sthid==0 or no samples returned) is rendered with the canonical
// copy. The exact wording is part of the user-visible UX and is
// pinned so a future refactor can't silently change it. Issue #286
// acceptance criterion.
func TestServiceChecksHTML_EmptyStateCopyForLegacyEntries(t *testing.T) {
	html := loadServiceChecksHTML(t)
	if !strings.Contains(html, "No per-sample data available — run a new test to populate.") {
		t.Error("service_checks.html missing the canonical empty-state copy 'No per-sample data available — run a new test to populate.'")
	}
}

// TestServiceChecksHTML_LazyRenderHookFiresOnOpen asserts that the
// toggleLogDetail handler invokes the speedtest-samples renderer when
// the row opens, not on every render or on close. This is the
// performance contract: the renderer issues a fetch + draws a canvas,
// which we don't want to repeat unnecessarily.
func TestServiceChecksHTML_LazyRenderHookFiresOnOpen(t *testing.T) {
	html := loadServiceChecksHTML(t)
	if !strings.Contains(html, "lazyRenderSpeedtestSamples") {
		t.Error("service_checks.html missing lazyRenderSpeedtestSamples renderer hook")
	}
	// The renderer must be called from inside the toggleLogDetail
	// handler's "open" branch — the simplest pin is that the
	// function's name appears in the script alongside toggleLogDetail.
	if !strings.Contains(html, "window.toggleLogDetail") {
		t.Error("service_checks.html missing window.toggleLogDetail handler — the renderer's invocation point must exist")
	}
	// Idempotent rendering: the data-rendered attribute is the cheapest
	// pin to detect "already drew once".
	if !strings.Contains(html, "data-rendered") {
		t.Error("service_checks.html mini-chart renderer must guard against re-fetch via a data-rendered marker")
	}
}
