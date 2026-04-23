package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsHTML_AdvancedScansCard pins the structural contract of
// the new "Advanced Scan Settings" card introduced by issue #237.
//
// The card lives alongside the existing generic Advanced card; it
// holds the scan-policy knobs (wake-drives toggle + new max-age
// input). The existing Advanced card keeps the Hide Docker
// Containers knob. Separating the two concerns is user story 22 in
// PRD #236.
func TestSettingsHTML_AdvancedScansCard(t *testing.T) {
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
		// New card anchor + title + sticky-nav entry.
		{"card anchor", `id="card-advanced-scans"`},
		{"card title", `Advanced Scan Settings`},
		{"nav link to new card", `href="#card-advanced-scans"`},

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

		// Wake-drives toggle lives HERE now, not in the legacy card.
		{"wake-drives toggle still present", `id="wake-drives-for-smart"`},
	}
	for _, tc := range mustContain {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.substr) {
				t.Errorf("settings.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}

	// The wake-drives toggle must appear AFTER the new card opens
	// and BEFORE the legacy Advanced card opens. This enforces that
	// the toggle moved — it's no longer inside id="card-advanced".
	newCardIdx := strings.Index(content, `id="card-advanced-scans"`)
	legacyCardIdx := strings.Index(content, `id="card-advanced"`)
	toggleIdx := strings.Index(content, `id="wake-drives-for-smart"`)
	if newCardIdx < 0 || legacyCardIdx < 0 || toggleIdx < 0 {
		t.Fatalf("missing required markers (new=%d legacy=%d toggle=%d)", newCardIdx, legacyCardIdx, toggleIdx)
	}
	if !(newCardIdx < toggleIdx && toggleIdx < legacyCardIdx) {
		t.Errorf("wake-drives toggle must sit between the new card (at %d) and the legacy card (at %d); got toggle at %d — did it get left behind in the legacy Advanced card?", newCardIdx, legacyCardIdx, toggleIdx)
	}

	// The Hide-Docker-Containers knob must remain inside the legacy
	// Advanced card. Its marker phrase is unique enough to serve as
	// an anchor. If this assertion fails someone moved the wrong
	// thing.
	hideDockerIdx := strings.Index(content, `Hide Docker containers from dashboard`)
	if hideDockerIdx < 0 {
		t.Fatalf("Hide Docker containers label vanished from settings.html")
	}
	if hideDockerIdx < legacyCardIdx {
		t.Errorf("Hide Docker containers knob must stay inside the legacy Advanced card (at %d); got marker at %d — did this move by mistake?", legacyCardIdx, hideDockerIdx)
	}
}
