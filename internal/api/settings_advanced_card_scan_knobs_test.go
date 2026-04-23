package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSettingsHTML_NoOrphanAdvancedScansCardReference is a regression guard
// introduced by issue #256. The V1a design from #237 created a dedicated
// "Advanced Scan Settings" card (id="card-advanced-scans") and moved the
// wake-drives toggle into it. UAT on v0.9.8-rc1 showed that two
// "Advanced"-prefixed cards on the same page added clutter without clarity,
// so #256 reverses that split: every scan-policy knob moves BACK into the
// single existing generic Advanced card (id="card-advanced").
//
// This test grep-checks the whole repo for any lingering reference to the
// defunct "card-advanced-scans" anchor — template, nav, tests, comments.
// A non-zero count means something was missed during the reversal.
func TestSettingsHTML_NoOrphanAdvancedScansCardReference(t *testing.T) {
	// Start at the repo root (two levels up from internal/api).
	root := filepath.Join("..", "..")

	var offenders []string

	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable paths, don't fail the walk
		}
		if info.IsDir() {
			// Skip VCS + build artefact dirs.
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only scan source-ish files.
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go", ".html", ".js", ".css", ".md", ".yaml", ".yml", ".json":
			// ok
		default:
			return nil
		}
		// Skip this test file itself — it legitimately mentions the string.
		if strings.HasSuffix(path, "settings_advanced_card_scan_knobs_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), "card-advanced-scans") {
			offenders = append(offenders, path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk failed: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Errorf("orphan references to the retired 'card-advanced-scans' anchor found in %d file(s) — issue #256 merged this card back into card-advanced, every mention should be gone:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestSettingsHTML_AdvancedCardDescriptorMentionsScanKeywords pins the
// descriptor-cue enhancement from #256. The Advanced card's descriptor
// (the <div class="card-desc"> line right below the card title) must
// surface the SMART scan + max-age keywords so users can gauge what's
// inside without expanding the <details> block.
//
// Exact phrasing is the worker's choice; this test only asserts the
// keywords are present (case-insensitive).
func TestSettingsHTML_AdvancedCardDescriptorMentionsScanKeywords(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	cardIdx := strings.Index(content, `id="card-advanced"`)
	if cardIdx < 0 {
		t.Fatalf("card-advanced block not found in settings.html")
	}
	// The descriptor is the first card-desc after the card anchor, but
	// before the next card opens. Slice the card body and regex-match
	// within it.
	nextCardIdx := strings.Index(content[cardIdx+1:], `<div class="card"`)
	var cardBody string
	if nextCardIdx < 0 {
		cardBody = content[cardIdx:]
	} else {
		cardBody = content[cardIdx : cardIdx+1+nextCardIdx]
	}

	descRE := regexp.MustCompile(`(?s)<div class="card-desc">(.*?)</div>`)
	m := descRE.FindStringSubmatch(cardBody)
	if m == nil {
		t.Fatalf("no <div class=\"card-desc\"> found inside card-advanced block")
	}
	desc := strings.ToLower(m[1])

	for _, kw := range []string{"smart scan controls", "max-age"} {
		if !strings.Contains(desc, kw) {
			t.Errorf("Advanced card descriptor missing keyword %q (case-insensitive); got: %q", kw, strings.TrimSpace(m[1]))
		}
	}
}

// TestSettingsHTML_ScanKnobsLiveInsideAdvancedCard pins the structural
// reshape done by #256. The wake-drives toggle and the max-age input
// (both originally introduced in #237 inside a dedicated card) must now
// live INSIDE the generic Advanced card (id="card-advanced"), specifically
// inside its <details> disclosure block.
//
// We assert position via byte-offset ordering: the card anchor must
// appear before both elements, and the next card (if any) must appear
// after them. The existing Hide-Docker-Containers knob must also remain
// inside the same card, so we pin all three siblings at once.
func TestSettingsHTML_ScanKnobsLiveInsideAdvancedCard(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	cardIdx := strings.Index(content, `id="card-advanced"`)
	if cardIdx < 0 {
		t.Fatalf("card-advanced block not found in settings.html")
	}

	// Determine the card's byte range: from its anchor to the start of
	// the next <div class="card"> (or end of file).
	rel := strings.Index(content[cardIdx+1:], `<div class="card"`)
	var cardEnd int
	if rel < 0 {
		cardEnd = len(content)
	} else {
		cardEnd = cardIdx + 1 + rel
	}
	body := content[cardIdx:cardEnd]

	mustContainInside := []struct {
		name   string
		substr string
	}{
		{"wake-drives toggle id", `id="wake-drives-for-smart"`},
		{"max-age input id", `id="smart-max-age-days"`},
		{"max-age min attribute", `min="0"`},
		{"max-age max attribute", `max="30"`},
		{"hide-docker knob label", `Hide Docker containers from dashboard`},
		// Disclosure element must still wrap the expert-only content.
		{"details element", `<details`},
		{"summary element", `<summary`},
	}
	for _, tc := range mustContainInside {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(body, tc.substr) {
				t.Errorf("card-advanced missing %q — expected substring inside the card body: %q", tc.name, tc.substr)
			}
		})
	}

	// Intra-card ordering: wake-drives toggle must appear before the
	// max-age input (design decision in #256: SMART scan knobs grouped
	// together, wake-drives first), and both must appear before the
	// Hide-Docker-Containers knob (its own group).
	toggleIdx := strings.Index(body, `id="wake-drives-for-smart"`)
	maxAgeIdx := strings.Index(body, `id="smart-max-age-days"`)
	hideDockerIdx := strings.Index(body, `Hide Docker containers from dashboard`)
	if toggleIdx < 0 || maxAgeIdx < 0 || hideDockerIdx < 0 {
		t.Fatalf("ordering markers missing (toggle=%d maxAge=%d hideDocker=%d)", toggleIdx, maxAgeIdx, hideDockerIdx)
	}
	if !(toggleIdx < maxAgeIdx) {
		t.Errorf("wake-drives toggle should appear before the max-age input (wake=%d maxAge=%d)", toggleIdx, maxAgeIdx)
	}
	if !(maxAgeIdx < hideDockerIdx) {
		t.Errorf("max-age input should appear before the Hide-Docker-Containers knob (maxAge=%d hideDocker=%d)", maxAgeIdx, hideDockerIdx)
	}
}
