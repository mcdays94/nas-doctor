package api

import (
	"strings"
	"testing"
)

// TestSettingsHTML_SaveSettingsIsDebounced asserts that the
// settings page wraps auto-save flows in a debounce window so a
// rapid burst of change events (e.g. clicking through a per-
// subsystem interval picker dropdown) collapses into a single
// PUT /api/v1/settings call.
//
// Background: during v0.9.8-rc1 UAT the container log showed 7
// full settings-save cycles within ~1s while the user clicked
// through a small number of pickers. Each picker change event
// fires saveSettings(); without debouncing, a 4-step picker walk
// produces 4 PUTs back-to-back. v0.9.9-rc2 confirmed the cause
// during slice 2b UAT. Issue #258 tracks the fix.
//
// Contract this test pins:
//
//  1. saveSettings() must be a thin debounced wrapper that uses
//     setTimeout + clearTimeout to collapse rapid invocations.
//  2. The actual PUT-firing function must live under a separate
//     name (saveSettingsImmediate) so the explicit "Save All
//     Settings" button can bypass the debounce.
//  3. The "Save All Settings" sticky-bar button MUST call
//     saveSettingsImmediate() — explicit user clicks should not
//     be delayed.
//  4. The previously-existing async PUT body (fetch, .then,
//     showToast, markSaved, .catch error path) must move into
//     saveSettingsImmediate so error reporting still surfaces
//     promptly to the user.
func TestSettingsHTML_SaveSettingsIsDebounced(t *testing.T) {
	html := SettingsPage

	// (1) A debounce helper exists. We pin both the timer-handle
	// variable name and the setTimeout/clearTimeout pair, and we
	// assert the wrapper function is actually named saveSettings
	// so the 49 existing call sites continue to flow through it.
	mustContain(t, html, "var saveSettingsDebounceTimer = null;",
		"expected debounce timer handle declaration")
	mustContain(t, html, "function saveSettings() {",
		"expected saveSettings to remain the public entry point (debounced wrapper)")
	mustContain(t, html, "clearTimeout(saveSettingsDebounceTimer)",
		"debounce wrapper must clear any pending timer before scheduling a new one")
	mustContain(t, html, "saveSettingsDebounceTimer = setTimeout(",
		"debounce wrapper must schedule the deferred save via setTimeout")
	mustContain(t, html, "SAVE_SETTINGS_DEBOUNCE_MS",
		"debounce window must be a named constant (kept hardcoded; not user-configurable)")
	// 300ms is the agreed default — short enough to be invisible
	// to users, long enough to coalesce the 150-200ms cadence
	// observed in the rc1 log.
	mustContain(t, html, "var SAVE_SETTINGS_DEBOUNCE_MS = 300;",
		"debounce window must default to 300ms")

	// (2) The PUT-firing function lives under a separate name so
	// callers that need synchronous behaviour (the explicit Save
	// button) can target it directly.
	mustContain(t, html, "function saveSettingsImmediate()",
		"expected saveSettingsImmediate to host the actual PUT logic")

	// (3) The sticky save bar's explicit button must NOT be
	// debounced. Find the save-bar block and assert it calls the
	// immediate variant.
	saveBarIdx := strings.Index(html, `<div class="save-bar"`)
	if saveBarIdx < 0 {
		t.Fatal("could not locate save-bar block")
	}
	saveBarEnd := strings.Index(html[saveBarIdx:], "</div>")
	if saveBarEnd < 0 {
		t.Fatal("malformed save-bar block")
	}
	saveBarBlock := html[saveBarIdx : saveBarIdx+saveBarEnd+200]
	if !strings.Contains(saveBarBlock, `onclick="saveSettingsImmediate()"`) {
		t.Errorf("Save All Settings button must call saveSettingsImmediate() directly so explicit clicks bypass the debounce, got block:\n%s", saveBarBlock)
	}

	// (4) The async body (fetch + .then chain + .catch error
	// path) must live inside saveSettingsImmediate, not the
	// debounced wrapper. We anchor on the function header and
	// scan for the fetch + error toast pattern within it.
	immStart := strings.Index(html, "function saveSettingsImmediate()")
	if immStart < 0 {
		t.Fatal("saveSettingsImmediate not found")
	}
	// Scan the next ~3000 chars — enough to cover the body, well
	// short of the next top-level function.
	tail := immStart + 3000
	if tail > len(html) {
		tail = len(html)
	}
	body := html[immStart:tail]
	if !strings.Contains(body, `fetch("/api/v1/settings"`) {
		t.Error("saveSettingsImmediate must contain the PUT /api/v1/settings fetch call")
	}
	if !strings.Contains(body, `method: "PUT"`) {
		t.Error("saveSettingsImmediate must issue a PUT request")
	}
	if !strings.Contains(body, `showToast("Error: " + e.message, "error")`) {
		t.Error("saveSettingsImmediate must preserve the .catch error-toast path so debouncing does not swallow failures")
	}
	if !strings.Contains(body, `showToast("Settings saved", "success")`) {
		t.Error("saveSettingsImmediate must preserve the success toast")
	}
	if !strings.Contains(body, "markSaved()") {
		t.Error("saveSettingsImmediate must preserve the markSaved() call so the sticky bar dismisses after a save")
	}

	// The debounced wrapper's body should be small — roughly the
	// clearTimeout + setTimeout pair, no fetch. Pin that the
	// fetch lives ONLY in the immediate variant.
	wrapStart := strings.Index(html, "function saveSettings() {")
	if wrapStart < 0 {
		t.Fatal("saveSettings wrapper not found")
	}
	wrapEnd := strings.Index(html[wrapStart:], "\n}\n")
	if wrapEnd < 0 {
		t.Fatal("saveSettings wrapper has no closing brace")
	}
	wrapBody := html[wrapStart : wrapStart+wrapEnd]
	if strings.Contains(wrapBody, `fetch("/api/v1/settings"`) {
		t.Errorf("saveSettings (the debounced wrapper) must NOT issue the fetch directly — the PUT belongs in saveSettingsImmediate. Wrapper body:\n%s", wrapBody)
	}
}
