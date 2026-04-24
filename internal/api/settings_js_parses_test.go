package api

import (
	"os/exec"
	"strings"
	"testing"
)

// TestSettingsTemplate_JSBlocksParse asserts every <script> block in
// settings.html is valid JavaScript. Shells out to `node --check` if
// available; otherwise skips (so CI without node is not blocked).
//
// Why this test exists: v0.9.9-rc1 shipped with a /* */ block comment
// containing the sequence `*/N s` (documenting cron examples), which
// closes the JS block comment prematurely. The entire <script> block
// failed to parse, silently breaking loadSettings(),
// renderAdvancedScanPickers(), loadDockerHiddenContainersCheckboxes(),
// and every onchange-driven save handler. Symptom on hardware UAT:
// Max-age input stuck on the static HTML default (value="7"), pickers
// not rendered, Hide-Docker list stuck on "Loading containers…", no
// Settings-Saved toast, changes reverted on refresh.
//
// No existing test would have caught this because template-source
// greps only verify that specific strings appear in the rendered
// HTML — they do not exercise JS parseability.
func TestSettingsTemplate_JSBlocksParse(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node binary not in PATH; skipping JS parse check (dev-time guard only)")
	}

	blocks := extractScriptBlocks(SettingsPage)
	if len(blocks) == 0 {
		t.Fatal("no <script>...</script> blocks found in settings.html")
	}

	for i, js := range blocks {
		if strings.TrimSpace(js) == "" {
			continue // external <script src="..."> tags produce empty bodies
		}
		cmd := exec.Command("node", "--check", "-")
		cmd.Stdin = strings.NewReader(js)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("settings.html <script> block %d failed to parse as JS:\n%s", i, string(out))
		}
	}
}

// extractScriptBlocks returns the bodies of every <script>...</script>
// pair in tpl, in document order. External <script src="..."> tags
// have empty bodies and are returned as empty strings (the caller
// filters those out — they're served as separate static assets and
// not inline JS under our control).
//
// Intentionally naive: no attribute parsing, no nested-script
// handling (HTML disallows nesting). Adequate for our template shape.
func extractScriptBlocks(tpl string) []string {
	var out []string
	cursor := 0
	for {
		openStart := strings.Index(tpl[cursor:], "<script")
		if openStart < 0 {
			break
		}
		openStart += cursor
		openEnd := strings.Index(tpl[openStart:], ">")
		if openEnd < 0 {
			break
		}
		openEnd += openStart + 1 // position after the '>'
		closeIdx := strings.Index(tpl[openEnd:], "</script>")
		if closeIdx < 0 {
			break
		}
		closeIdx += openEnd
		out = append(out, tpl[openEnd:closeIdx])
		cursor = closeIdx + len("</script>")
	}
	return out
}
