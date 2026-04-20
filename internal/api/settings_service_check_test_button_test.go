package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// loadSettingsHTML returns the settings.html template content for assertions.
func loadSettingsHTML(t *testing.T) string {
	t.Helper()
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	return string(data)
}

// TestSettingsHTML_HasServiceCheckTestButton verifies the Test button is
// rendered inside the service-check form, alongside Save and Cancel.
func TestSettingsHTML_HasServiceCheckTestButton(t *testing.T) {
	html := loadSettingsHTML(t)
	if !strings.Contains(html, `onclick="testServiceCheck()"`) {
		t.Fatal("settings.html missing Test button (expected onclick=\"testServiceCheck()\")")
	}
	// Button must live in the same action row as Save/Cancel.
	idx := strings.Index(html, `onclick="testServiceCheck()"`)
	windowStart := idx - 400
	if windowStart < 0 {
		windowStart = 0
	}
	window := html[windowStart : idx+400]
	if !strings.Contains(window, `saveServiceCheck()`) || !strings.Contains(window, `cancelServiceCheckForm()`) {
		t.Fatalf("Test button should sit alongside Save/Cancel in the same action row; local window:\n%s", window)
	}
}

// TestSettingsHTML_TestServiceCheckFunctionExists verifies a testServiceCheck
// JS function is defined in the page.
func TestSettingsHTML_TestServiceCheckFunctionExists(t *testing.T) {
	html := loadSettingsHTML(t)
	re := regexp.MustCompile(`function\s+testServiceCheck\s*\(`)
	if !re.MatchString(html) {
		t.Fatal("settings.html missing `function testServiceCheck(` definition")
	}
}

// TestSettingsHTML_ReadServiceCheckFormExtracted verifies the DOM-reading
// logic has been factored out into a readServiceCheckForm() helper.
func TestSettingsHTML_ReadServiceCheckFormExtracted(t *testing.T) {
	html := loadSettingsHTML(t)
	re := regexp.MustCompile(`function\s+readServiceCheckForm\s*\(`)
	if !re.MatchString(html) {
		t.Fatal("settings.html missing `function readServiceCheckForm(` helper — extract DOM reading so saveServiceCheck and testServiceCheck share it")
	}
}

// TestSettingsHTML_SaveServiceCheckUsesReader verifies saveServiceCheck
// invokes readServiceCheckForm instead of duplicating the DOM reads.
func TestSettingsHTML_SaveServiceCheckUsesReader(t *testing.T) {
	html := loadSettingsHTML(t)
	// locate the saveServiceCheck body
	startRe := regexp.MustCompile(`function\s+saveServiceCheck\s*\(\s*\)\s*\{`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("saveServiceCheck() function not found")
	}
	// Take a reasonable window of the body.
	end := loc[1] + 2000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]
	if !strings.Contains(body, "readServiceCheckForm(") {
		t.Fatalf("saveServiceCheck should call readServiceCheckForm(); body window:\n%s", body)
	}
}

// TestSettingsHTML_TestServiceCheckPostsToCorrectEndpoint verifies the JS
// targets the new /api/v1/service-checks/test endpoint.
func TestSettingsHTML_TestServiceCheckPostsToCorrectEndpoint(t *testing.T) {
	html := loadSettingsHTML(t)
	if !strings.Contains(html, "/api/v1/service-checks/test") {
		t.Fatal("settings.html missing fetch to /api/v1/service-checks/test")
	}
}

// TestSettingsHTML_TestServiceCheckShowsSpeedWarning verifies that when the
// check type is speed the user is warned about the long runtime.
func TestSettingsHTML_TestServiceCheckShowsSpeedWarning(t *testing.T) {
	html := loadSettingsHTML(t)
	// Locate testServiceCheck body.
	startRe := regexp.MustCompile(`function\s+testServiceCheck\s*\(\s*\)\s*\{`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("testServiceCheck() function not found")
	}
	end := loc[1] + 3000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]
	if !strings.Contains(strings.ToLower(body), "60s") && !strings.Contains(strings.ToLower(body), "60 s") {
		t.Fatalf("testServiceCheck should warn users that speed tests can take up to 60s; body window:\n%s", body)
	}
}

// TestSettingsHTML_SpeedCheckInterval_DefaultsToDaily verifies that when the
// user selects "speed" in the service-check type dropdown, onServiceTypeChange
// bumps the interval from the new-check default of 300s (5min) to 86400s
// (daily). Running Ookla speedtest every 5 minutes would waste bandwidth and
// likely trigger ISP throttling.
func TestSettingsHTML_SpeedCheckInterval_DefaultsToDaily(t *testing.T) {
	html := loadSettingsHTML(t)
	startRe := regexp.MustCompile(`function\s+onServiceTypeChange\s*\(\s*\)\s*\{`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("onServiceTypeChange() function not found")
	}
	end := loc[1] + 2000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]
	// Must guard on type === "speed" to avoid overwriting the interval for
	// other types.
	if !regexp.MustCompile(`type\s*===\s*["']speed["']`).MatchString(body) {
		t.Error("onServiceTypeChange should check for speed type before changing interval")
	}
	// Must set interval to 86400 (seconds in a day).
	if !strings.Contains(body, "86400") {
		t.Errorf("onServiceTypeChange should default speed-type interval to 86400s (daily); got body:\n%s", body)
	}
	// Must guard on still-at-default-300 so user-customised values aren't overwritten.
	if !regexp.MustCompile(`===?\s*300`).MatchString(body) {
		t.Error("onServiceTypeChange should only change interval if user hasn't customised from the 300s default")
	}
}

// TestSettingsHTML_IntervalSelect_OptionsMatchAnyAutoDefault guards against the
// regression where onServiceTypeChange sets the interval to a value not in the
// <select id="sc-interval"> options list. Observed in rc4: the JS set value
// to 86400 but the select had no matching <option>, so the dropdown rendered
// blank. Every value the JS can auto-assign MUST have a corresponding option.
func TestSettingsHTML_IntervalSelect_OptionsMatchAnyAutoDefault(t *testing.T) {
	html := loadSettingsHTML(t)

	// Extract the <select id="sc-interval">...</select> block.
	selectRe := regexp.MustCompile(`(?s)<select id="sc-interval">(.*?)</select>`)
	m := selectRe.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("<select id=\"sc-interval\"> not found")
	}
	optsBlock := m[1]

	// Collect every value="…" occurrence.
	optRe := regexp.MustCompile(`value="(\d+)"`)
	present := map[string]bool{}
	for _, match := range optRe.FindAllStringSubmatch(optsBlock, -1) {
		present[match[1]] = true
	}

	// Every value that onServiceTypeChange or similar code can set as a
	// default MUST be in the options.
	required := []struct{ value, label string }{
		{"300", "5 minutes (new-check baseline)"},
		{"86400", "daily (auto-assigned for speed checks)"},
	}
	for _, r := range required {
		if !present[r.value] {
			t.Errorf("<select id=\"sc-interval\"> missing option value=%q (%s) — onServiceTypeChange sets this value and it must match an existing <option>", r.value, r.label)
		}
	}

	// Nice-to-have options (don't fail loudly, but flag as missing for
	// future richness — users who opt into daily speed tests probably want
	// weekly as an alternative too).
	niceTo := []string{"604800"}
	for _, v := range niceTo {
		if !present[v] {
			t.Logf("NOTE: <select id=\"sc-interval\"> missing option value=%q (consider adding)", v)
		}
	}
}

// TestHandleTestServiceCheck_DisablesWriteDeadline verifies that the handler
// code invokes http.NewResponseController(w).SetWriteDeadline(time.Time{}) so
// that speed tests can run longer than the 30s baseline http.Server.WriteTimeout
// set in cmd/nas-doctor/main.go. Without this, long Ookla runs trigger a
// transport-layer 502 Bad Gateway upstream (observed during v0.9.2-rc3 UAT).
//
// This is a source-level assertion rather than a behavioural test — exercising
// the real 30s timeout would make the suite slow and flaky. The fix itself is
// one line of well-known Go 1.20+ API, so a grep-style guard is proportionate.
func TestHandleTestServiceCheck_DisablesWriteDeadline(t *testing.T) {
	path := filepath.Join(".", "api_extended.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read api_extended.go: %v", err)
	}
	src := string(data)
	handlerRe := regexp.MustCompile(`func\s+\(s\s+\*Server\)\s+handleTestServiceCheck\s*\(`)
	loc := handlerRe.FindStringIndex(src)
	if loc == nil {
		t.Fatal("handleTestServiceCheck function not found")
	}
	// Look within the first ~1500 bytes of the handler body.
	end := loc[0] + 1500
	if end > len(src) {
		end = len(src)
	}
	body := src[loc[0]:end]
	if !strings.Contains(body, "NewResponseController") {
		t.Error("handleTestServiceCheck should use http.NewResponseController to override WriteTimeout for long-running speed tests")
	}
	if !strings.Contains(body, "SetWriteDeadline(time.Time{})") {
		t.Error("handleTestServiceCheck should call SetWriteDeadline(time.Time{}) to disable the per-connection write deadline")
	}
}
