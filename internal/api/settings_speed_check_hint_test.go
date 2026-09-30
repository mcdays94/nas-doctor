package api

import (
	"strings"
	"testing"
)

// Issue #340 (follow-up to #333): a user disabled the "Internet Speed"
// service check expecting scheduled speed tests to stop. A type=speed check
// only reads the latest result; tests are controlled from the Speed Test
// card. The speed check editor has to say so, since that's where users look.
func TestSettingsPage_SpeedCheckEditorSaysItDoesNotRunTests(t *testing.T) {
	start := strings.Index(SettingsPage, `id="sc-speed-wrap"`)
	end := strings.Index(SettingsPage, `id="sc-traceroute-wrap"`)
	if start < 0 || end < start {
		t.Fatal("could not locate the speed check editor block (sc-speed-wrap to sc-traceroute-wrap)")
	}
	block := SettingsPage[start:end]
	for _, want := range []string{
		"doesn't run speed tests",
		"set Speed Test to Disabled",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("speed check editor is missing %q; users disable the check expecting tests to stop (#340)", want)
		}
	}
}
