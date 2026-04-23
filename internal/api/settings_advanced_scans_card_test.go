package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsHTML_AdvancedScanKnobs pins the structural contract of
// the SMART scan-policy knobs (wake-drives toggle + max-age input).
//
// History:
//   - #237 (V1a of PRD #236) introduced these knobs inside a new dedicated
//     Advanced Scan Settings card.
//   - #256 reversed that split after UAT feedback (two "Advanced"-prefixed
//     cards on one page was clutter, not clarity). The knobs now live
//     inside the single generic Advanced card id="card-advanced".
//
// This test keeps the original V1a intent — every pre-click validation
// attribute and JSON-binding symbol must still be present — but pinned
// against the new home (card-advanced). The companion test
// TestSettingsHTML_ScanKnobsLiveInsideAdvancedCard verifies they live
// inside that specific card via byte-offset ordering.
func TestSettingsHTML_AdvancedScanKnobs(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	mustContain := []struct {
		name   string
		substr string
	}{
		// Generic Advanced card anchor + sticky-nav entry — this is the
		// new home for the scan knobs since #256.
		{"advanced card anchor", `id="card-advanced"`},
		{"nav link to advanced card", `href="#card-advanced"`},

		// Max-age input: id + nested JSON key on the load and save
		// paths. Range validation is a separate assertion below.
		{"max-age input id", `id="smart-max-age-days"`},
		{"load binds nested max_age_days", `smart.max_age_days`},

		// The input must enforce 0-30 client-side so the browser's
		// native validity UI catches out-of-range values before the
		// server round-trip rejects them.
		{"max-age min attribute", `min="0"`},
		{"max-age max attribute", `max="30"`},

		// Inline explanation must surface the disable-by-zero
		// convention (PRD user stories 5 + 23).
		{"max-age explanation mentions disabled", `disabled`},

		// Wake-drives toggle still present under its stable id.
		{"wake-drives toggle still present", `id="wake-drives-for-smart"`},
	}
	for _, tc := range mustContain {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.substr) {
				t.Errorf("settings.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}

	// After #256 both the wake-drives toggle and the Hide-Docker-Containers
	// knob must live inside the same card-advanced block; the toggle appears
	// first (SMART group), the Docker knob second (Docker group). If this
	// assertion fails the merge regressed.
	advancedIdx := strings.Index(content, `id="card-advanced"`)
	toggleIdx := strings.Index(content, `id="wake-drives-for-smart"`)
	hideDockerIdx := strings.Index(content, `Hide Docker containers from dashboard`)
	if advancedIdx < 0 || toggleIdx < 0 || hideDockerIdx < 0 {
		t.Fatalf("missing required markers (advanced=%d toggle=%d hideDocker=%d)", advancedIdx, toggleIdx, hideDockerIdx)
	}
	if !(advancedIdx < toggleIdx && toggleIdx < hideDockerIdx) {
		t.Errorf("wake-drives toggle must sit inside the Advanced card (at %d) AND before the Hide-Docker-Containers knob (at %d); got toggle at %d — did #256's merge regress?", advancedIdx, hideDockerIdx, toggleIdx)
	}
}
