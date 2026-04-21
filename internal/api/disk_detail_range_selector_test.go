package api

import (
	"strings"
	"testing"
)

// TestDiskDetailHTMLContainsRangeSelector asserts the /disk/<serial> page
// template wires up the 1D / 1W / 1M / 1Y time-window selector.
//
// Issue #166: prior to this change /disk/<serial> rendered all available
// SMART history at once (~500 rows ≈ 10 days at default scan cadence),
// which produced unreadable x-axis labels. A range selector, mirroring
// the /stats process-history pattern, gives users control and trims
// label density to what fits.
//
// This is a UI cross-reference test (see AGENTS.md §4b): if someone
// renames _loadDiskHistoryRange or drops one of the range buttons in a
// future refactor, the server-side tests are what catches it — there is
// no browser-side test harness to cover this.
func TestDiskDetailHTMLContainsRangeSelector(t *testing.T) {
	tmpl := DiskDetailPage
	if tmpl == "" {
		t.Fatal("DiskDetailPage embedded template is empty")
	}

	checks := []struct {
		name   string
		substr string
	}{
		// Range selector button labels live in the JS-side ranges
		// array as {h:N,l:"LABEL"} tuples; the page renders them via
		// string concat so these labels must appear verbatim.
		{"1D button label", `l:"1D"`},
		{"1W button label", `l:"1W"`},
		{"1M button label", `l:"1M"`},
		{"1Y button label", `l:"1Y"`},

		// The hours values passed through to the API. If any of these
		// drift, the selector will silently stop matching the active
		// window on first paint.
		{"24h mapping", "h:24"},
		{"168h mapping", "h:168"},
		{"720h mapping", "h:720"},
		{"8760h mapping", "h:8760"},

		// Handler name — locked in so a rename requires updating both
		// sides and gets caught here.
		{"range-load handler", "_loadDiskHistoryRange"},

		// API contract: fetch with ?hours= so we stay on the time-window
		// code path and not the legacy row-limited one.
		{"API fetch with hours", "/api/v1/disks/"},
		{"hours query param", "?hours=\" + hours"},

		// Default window is 1D on first load (issue #166 scope decision).
		// The variable initial value must be 24 so the page opens on 1D.
		{"default 1D window", "diskHistoryHours = 24"},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tmpl, tc.substr) {
				t.Errorf("disk_detail.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestDiskDetailHTMLRangeSelectorDefaultIs1D pins down the default-on-load
// behavior. The buttons MUST default to 1D so a freshly opened /disk/<serial>
// doesn't render the full history and reintroduce #166. See scope
// decisions in the issue: "Default: 1D".
func TestDiskDetailHTMLRangeSelectorDefaultIs1D(t *testing.T) {
	tmpl := DiskDetailPage

	// Sanity: the initial value of diskHistoryHours must be 24 (1D), not
	// 168 / 720 / 8760 — if someone swaps the default we want the test
	// to fail loud rather than ship a regression.
	if !strings.Contains(tmpl, "diskHistoryHours = 24") {
		t.Error("expected diskHistoryHours default = 24 (1D) in disk_detail.html")
	}
	for _, wrong := range []string{"diskHistoryHours = 168", "diskHistoryHours = 720", "diskHistoryHours = 8760"} {
		if strings.Contains(tmpl, wrong) {
			t.Errorf("disk_detail.html uses wrong default window (%q); issue #166 requires 1D default", wrong)
		}
	}
}
