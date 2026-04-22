package api

import (
	"strings"
	"testing"
)

// TestSettingsHTML_TracerouteOptionInDropdown — the <select id="sc-type">
// must include a traceroute option alongside HTTP/TCP/DNS/ping/etc so
// users can create the new check type from the settings UI. Issue #189.
func TestSettingsHTML_TracerouteOptionInDropdown(t *testing.T) {
	html := loadSettingsHTML(t)
	if !strings.Contains(html, `<option value="traceroute"`) {
		t.Fatal(`settings.html missing <option value="traceroute"> in sc-type dropdown (#189)`)
	}
}

// TestSettingsHTML_TracerouteMaxLossField — a max_loss_pct input is
// rendered inside a conditional wrapper that becomes visible when the
// user selects traceroute. Pointer-typed on the Go side so blank = nil.
func TestSettingsHTML_TracerouteMaxLossField(t *testing.T) {
	html := loadSettingsHTML(t)
	if !strings.Contains(html, `id="sc-traceroute-wrap"`) {
		t.Fatal(`settings.html missing #sc-traceroute-wrap container for traceroute-specific fields (#189)`)
	}
	if !strings.Contains(html, `id="sc-max-loss-pct"`) {
		t.Fatal(`settings.html missing #sc-max-loss-pct input for traceroute MaxLossPct field (#189)`)
	}
}

// TestSettingsHTML_TracerouteDetailsRenderer — renderServiceCheckDetails
// handles type=traceroute and produces at least the hops summary
// (hops_count, final_rtt_ms, end_to_end_loss_pct) in the toast.
func TestSettingsHTML_TracerouteDetailsRenderer(t *testing.T) {
	html := loadSettingsHTML(t)
	// Look for the traceroute branch in the JS renderer.
	if !strings.Contains(html, `type === "traceroute"`) {
		t.Fatal(`settings.html renderServiceCheckDetails missing 'type === "traceroute"' branch (#189)`)
	}
	// The summary must surface the key metrics by name so users can see
	// why a trace is up/degraded/down.
	if !strings.Contains(html, "hops_count") {
		t.Error(`traceroute renderer should include hops_count in the toast summary`)
	}
	if !strings.Contains(html, "end_to_end_loss_pct") {
		t.Error(`traceroute renderer should include end_to_end_loss_pct in the toast summary`)
	}
	if !strings.Contains(html, "final_rtt_ms") {
		t.Error(`traceroute renderer should include final_rtt_ms in the toast summary`)
	}
}

// TestSettingsHTML_TracerouteHopsInTestToast — the toast renderer walks
// the details.hops array to render per-hop {host, avg, loss} so the
// Test button produces a diagnostic hop list (not just a summary).
func TestSettingsHTML_TracerouteHopsInTestToast(t *testing.T) {
	html := loadSettingsHTML(t)
	// Anchor on the renderer function so we're definitely inside
	// renderServiceCheckDetails (other helpers mention "traceroute"
	// too — pillClass, onServiceTypeChange, readServiceCheckForm).
	rendererStart := strings.Index(html, "function renderServiceCheckDetails")
	if rendererStart < 0 {
		t.Fatal("renderServiceCheckDetails function not found")
	}
	// Bound the function body — the next function declaration comes
	// well before 4000 chars.
	rendererEnd := rendererStart + 4000
	if rendererEnd > len(html) {
		rendererEnd = len(html)
	}
	renderer := html[rendererStart:rendererEnd]
	if !strings.Contains(renderer, `type === "traceroute"`) {
		t.Fatal("renderer missing 'type === \"traceroute\"' branch")
	}
	if !strings.Contains(renderer, "details.hops") {
		t.Fatalf("traceroute renderer must reference details.hops to render per-hop rows; renderer body:\n%s", renderer)
	}
}

// TestServiceChecksHTML_TracerouteRenderBranch — the log-page expanded
// detail row must render a traceroute-specific block so users browsing
// scheduled-run history see the same hop summary. Issue #189.
func TestServiceChecksHTML_TracerouteRenderBranch(t *testing.T) {
	html := loadServiceChecksHTML(t)
	// The renderServiceCheckDetails function is reused (injected from
	// settings.html via the shared JS bundle), or a parallel one lives
	// in service_checks.html. Either way, the template must include
	// the traceroute pill class so the log row type chip is coloured
	// correctly.
	if !strings.Contains(html, `pill-trace`) {
		t.Fatal(`service_checks.html missing pill-trace class for traceroute log rows (#189)`)
	}
	// And the pillClass helper must map "traceroute" to that class.
	if !strings.Contains(html, `"traceroute"`) {
		t.Fatal(`service_checks.html pillClass helper must map "traceroute" type (#189)`)
	}
}

// TestSettingsHTML_TracerouteTypePillClass — the serviceTypePillClass
// helper used in the service-check list must return pill-trace (or
// similar) for traceroute so the list rows are coloured.
func TestSettingsHTML_TracerouteTypePillClass(t *testing.T) {
	html := loadSettingsHTML(t)
	// serviceTypePillClass helper covers the "type === 'traceroute'"
	// case somewhere in the JS bundle.
	if !strings.Contains(html, `pill-trace`) {
		t.Fatal(`settings.html missing pill-trace class mapping for traceroute type (#189)`)
	}
}
