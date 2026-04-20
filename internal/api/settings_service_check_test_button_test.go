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
