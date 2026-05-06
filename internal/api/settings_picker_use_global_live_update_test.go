package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsHTML_PickerUseGlobalLabels_LiveUpdate is the regression
// guard for issue #306. The per-subsystem scan-interval pickers
// introduced in v0.9.9 (PRD #239 slice 2) computed their
// "Use global (X)" label once at picker mount time, baking the
// humanised duration into the option's textContent. If the user
// changed the global scan_interval via the Diagnostic Scan Interval
// widget AFTER the pickers were rendered, every picker continued to
// advertise the OLD value until a full page reload. The setting
// itself was correct (the pickers all DID use the new global) — only
// the label was stale, which was misleading.
//
// The fix uses option (2) from the issue body: a surgical DOM-mutate
// approach. A new updateAllPickerUseGlobalLabels() helper walks the
// six picker selects and rewrites the textContent of each value="0"
// option in place. The helper is wired as a change/input listener on
// the global #scan-preset select AND the four custom d/h/m/s inputs,
// so the label tracks the live form state — not just the post-save
// snapshot in `settings.scan_interval`.
//
// Why DOM-mutate, not full re-render: the issue's option (1) (call
// renderAdvancedScanPickers again) would have to handle preserving
// each picker's currently-selected non-default value (e.g. SMART set
// to 1h), the open/closed state of its custom panel, the cron-edit
// expander, and any in-flight d/h/m/s edits. createIntervalPicker is
// idempotent via its data-inited="1" guard but setPickerFromIntervalSec
// would fight live user edits. The DOM-mutate approach only touches
// the one bit that's actually stale (the option text) and leaves
// every other piece of picker state untouched.
func TestSettingsHTML_PickerUseGlobalLabels_LiveUpdate(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	content := string(data)

	// (1) Helper must exist with the documented name. Anchors the
	// fix to a discoverable function rather than an anonymous
	// closure that's hard to grep for in future audits.
	if !strings.Contains(content, "function updateAllPickerUseGlobalLabels") {
		t.Errorf("settings.html missing updateAllPickerUseGlobalLabels() helper — issue #306 fix should declare a named function so future maintainers can find the live-update path")
	}

	// (2) The helper must walk every picker's value="0" option and
	// rewrite its textContent. We pin both the lookup pattern (the
	// option-value === "0" comparison that identifies the
	// "Use global" entry uniquely) and the textContent assignment
	// with the humanised label embedded in parentheses.
	mustContain := []string{
		// Iterates the same six subsystems renderAdvancedScanPickers
		// instantiates. Ordering not asserted — set membership only.
		`"smart"`,
		`"docker"`,
		`"proxmox"`,
		`"kubernetes"`,
		`"zfs"`,
		`"gpu"`,
		// Picker-select id pattern matches createIntervalPicker's
		// mount convention.
		`"scan-interval-" + ids[i] + "-preset"`,
		// The "Use global" option is identified by value === "0"
		// (the use-global sentinel from advScanPresets).
		`sel.options[j].value === "0"`,
		// Mutation target is textContent — preserves the option
		// element identity (no innerHTML rebuild) so the select's
		// .value isn't reset.
		"sel.options[j].textContent",
		// Label format must keep the "Use global (X)" shape that
		// matches createIntervalPicker's initial render. Mismatch
		// would make the live-update visually distinct from the
		// fresh-render label.
		`"Use global (" + humanized + ")"`,
	}
	for _, sub := range mustContain {
		if !strings.Contains(content, sub) {
			t.Errorf("settings.html missing live-update DOM mutation marker %q — issue #306 fix must walk picker selects and rewrite the value=\"0\" option's textContent", sub)
		}
	}

	// (3) The change listener must be wired on the global
	// scan-preset select so a preset switch (30m → 7d) propagates
	// to the pickers without a save round-trip. Plus the four
	// custom-panel inputs, so live d/h/m/s edits in the global
	// custom panel also propagate.
	listenerWiring := []string{
		`document.getElementById("scan-preset").addEventListener("change", updateAllPickerUseGlobalLabels)`,
		`document.getElementById("scan-days").addEventListener("input", updateAllPickerUseGlobalLabels)`,
		`document.getElementById("scan-hours").addEventListener("input", updateAllPickerUseGlobalLabels)`,
		`document.getElementById("scan-minutes").addEventListener("input", updateAllPickerUseGlobalLabels)`,
		`document.getElementById("scan-seconds").addEventListener("input", updateAllPickerUseGlobalLabels)`,
	}
	for _, sub := range listenerWiring {
		if !strings.Contains(content, sub) {
			t.Errorf("settings.html missing live-update listener wiring %q — issue #306 fix must attach the helper to BOTH the global #scan-preset change event AND the custom d/h/m/s input events so the label tracks live form state, not just post-save snapshots", sub)
		}
	}

	// (4) The helper must use the SAME humanisation pipeline as
	// createIntervalPicker / globalIntervalDisplay
	// (parseIntervalDurationToFields → fieldsToIntervalDuration).
	// If a future refactor swaps the two paths, the picker labels
	// would drift between fresh-render and live-update — a subtle
	// regression that's hard to spot. Pinning both helpers in this
	// scope ensures the live-update goes through the same humaniser
	// that turns Go's "168h0m0s" into "7d" (v0.9.9-rc3 UAT lesson).
	humaniser := []string{
		"parseIntervalDurationToFields(dur)",
		"fieldsToIntervalDuration(fields)",
	}
	for _, sub := range humaniser {
		if !strings.Contains(content, sub) {
			t.Errorf("settings.html missing humaniser pipeline marker %q in updateAllPickerUseGlobalLabels — live-update label must match createIntervalPicker's fresh-render label exactly (v0.9.9-rc3 UAT)", sub)
		}
	}

	// (5) The helper must read from getScanInterval() (the live
	// form value), not just settings.scan_interval (the post-save
	// snapshot). Reading the snapshot would mean the label only
	// updates after a save round-trip, which defeats the "live"
	// part of the fix.
	if !strings.Contains(content, "getScanInterval()") {
		t.Errorf("settings.html updateAllPickerUseGlobalLabels must call getScanInterval() to read the live form value (not just settings.scan_interval) so the label tracks the user's typing in real-time")
	}
}
