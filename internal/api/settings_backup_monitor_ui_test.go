package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsHTML_BorgMonitorSection_HasUIAnchors pins the markup
// anchors for the external Borg monitor sub-section in the Advanced
// card. The UI must expose: a container list, an Add button, and
// the headline. These are the DOM targets renderBorgMonitorList
// writes into; the assertions catch accidental renames.
func TestSettingsHTML_BorgMonitorSection_HasUIAnchors(t *testing.T) {
	html := mustReadSettingsHTML(t)
	contracts := []struct {
		name   string
		substr string
	}{
		{"section container", `id="borg-monitor-section"`},
		{"list container", `id="borg-monitor-list"`},
		{"add button calls addBorgMonitorRepo", "addBorgMonitorRepo()"},
		{"sub-section headline", "Backup Monitors &rarr; Borg"},
		{"explains bundled binary", "bundled in the image"},
		{"mentions BORG_PASSPHRASE env var", "BORG_PASSPHRASE"},
	}
	for _, tc := range contracts {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(html, tc.substr) {
				t.Errorf("settings.html missing %q (%s)", tc.substr, tc.name)
			}
		})
	}
}

// TestSettingsHTML_BorgMonitor_HelperFunctions pins the existence of
// the six JS helpers that drive add / remove / render / collect /
// test / load flows. Template-source assertion; internal markup can
// change without breaking this test, but the helper contract cannot.
func TestSettingsHTML_BorgMonitor_HelperFunctions(t *testing.T) {
	html := mustReadSettingsHTML(t)
	helpers := []string{
		"function addBorgMonitorRepo",
		"function removeBorgMonitorRepo",
		"function renderBorgMonitorList",
		"function collectBorgMonitorRepos",
		"function testBorgMonitorRepo",
		"function loadBorgMonitorFromSettings",
	}
	for _, h := range helpers {
		if !strings.Contains(html, h) {
			t.Errorf("settings.html missing helper %q", h)
		}
	}
}

// TestSettingsHTML_BorgMonitor_PayloadIncludesBackupMonitor pins the
// buildSettingsPayload extension: the saved Settings PUT body must
// carry the new backup_monitor.borg array so server-side persistence
// actually happens.
func TestSettingsHTML_BorgMonitor_PayloadIncludesBackupMonitor(t *testing.T) {
	html := mustReadSettingsHTML(t)
	// Both the top-level key and the nested borg array must appear
	// — either alone would be a bug.
	if !strings.Contains(html, "backup_monitor:") {
		t.Error("settings.html missing backup_monitor: key in buildSettingsPayload")
	}
	if !strings.Contains(html, "borg: collectBorgMonitorRepos()") {
		t.Error("settings.html missing borg: collectBorgMonitorRepos() in payload")
	}
}

// TestSettingsHTML_BorgMonitor_TestButtonCallsEndpoint pins the
// Test-button JS fetch → POST /api/v1/backup-monitor/borg/test shape.
// The endpoint name is part of the feature's public contract; tests
// catch an accidental path rename.
func TestSettingsHTML_BorgMonitor_TestButtonCallsEndpoint(t *testing.T) {
	html := mustReadSettingsHTML(t)
	if !strings.Contains(html, "/api/v1/backup-monitor/borg/test") {
		t.Error("settings.html missing POST URL for Borg monitor Test button")
	}
}

// TestSettingsHTML_BorgMonitor_LoadHookFires pins that loadSettings
// actually calls loadBorgMonitorFromSettings — otherwise the list
// renders empty on every page load regardless of stored config.
func TestSettingsHTML_BorgMonitor_LoadHookFires(t *testing.T) {
	html := mustReadSettingsHTML(t)
	if !strings.Contains(html, "loadBorgMonitorFromSettings(data.backup_monitor)") {
		t.Error("settings.html missing load hook: loadBorgMonitorFromSettings must be called from loadSettings with data.backup_monitor")
	}
}

// mustReadSettingsHTML reads the template from disk for source-level
// assertions. Used across the backup-monitor UI tests.
func mustReadSettingsHTML(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("templates", "settings.html"))
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	return string(data)
}
