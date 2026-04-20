package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSettingsHTML_GenerateAPIKey_HasSecureFallback verifies that
// generateAPIKey() in settings.html does not unconditionally call
// crypto.randomUUID() — which throws on non-secure contexts (plain HTTP
// on LAN) — and instead falls back to crypto.getRandomValues(). See #126.
func TestSettingsHTML_GenerateAPIKey_HasSecureFallback(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	// Must reference getRandomValues (the fallback for non-secure contexts)
	if !strings.Contains(content, "crypto.getRandomValues") {
		t.Error("generateAPIKey() must include a crypto.getRandomValues() fallback for non-secure contexts")
	}

	// Locate generateAPIKey() body and assert the guard is present
	genRE := regexp.MustCompile(`(?s)function\s+generateAPIKey\s*\(\s*\)\s*\{(.*?)\n\}`)
	m := genRE.FindStringSubmatch(content)
	if len(m) < 2 {
		t.Fatal("could not locate generateAPIKey() function body in settings.html")
	}
	body := m[1]

	// randomUUID must not be called unconditionally — it must be guarded by a
	// typeof check or a feature-detection conditional.
	guardRE := regexp.MustCompile(`typeof\s+crypto\.randomUUID`)
	if !guardRE.MatchString(body) {
		t.Error("generateAPIKey() must guard crypto.randomUUID with a typeof check so it does not throw in non-secure contexts")
	}

	// The getRandomValues fallback must live inside the function body.
	if !strings.Contains(body, "getRandomValues") {
		t.Error("generateAPIKey() must contain the getRandomValues() fallback inside its body")
	}
}

// TestSettingsHTML_CopyAPIKey_HasClipboardFallback verifies that
// copyAPIKey() in settings.html has a fallback for browsers where
// navigator.clipboard is unavailable (non-secure context). See #126.
func TestSettingsHTML_CopyAPIKey_HasClipboardFallback(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	copyRE := regexp.MustCompile(`(?s)function\s+copyAPIKey\s*\(\s*\)\s*\{(.*?)\n\}`)
	m := copyRE.FindStringSubmatch(content)
	if len(m) < 2 {
		t.Fatal("could not locate copyAPIKey() function body in settings.html")
	}
	body := m[1]

	// Must guard navigator.clipboard access rather than calling it directly.
	guardRE := regexp.MustCompile(`navigator\.clipboard\s*&&|typeof\s+navigator\.clipboard|typeof\s+navigator`)
	if !guardRE.MatchString(body) {
		t.Error("copyAPIKey() must guard navigator.clipboard with a conditional to handle non-secure contexts")
	}

	// The execCommand fallback can live in a helper — assert it exists
	// somewhere in the template so the copy button still works over HTTP.
	if !strings.Contains(content, "execCommand") {
		t.Error("settings.html must contain a document.execCommand('copy') fallback for non-secure contexts")
	}
}
