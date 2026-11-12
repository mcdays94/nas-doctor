package api

import (
	"regexp"
	"strings"
	"testing"
)

// settingsFunc returns the source of a top-level function in settings.html.
func settingsFunc(t *testing.T, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)\nfunction\s+` + regexp.QuoteMeta(name) + `\s*\([^)]*\)\s*\{.*?\n\}`)
	m := re.FindString(SettingsPage)
	if m == "" {
		t.Fatalf("function %s not found in settings.html", name)
	}
	return m
}

// TestSettingsPage_SendsTheServersFieldNames pins the three name
// mismatches that made quiet hours and maintenance windows impossible to
// save from the page.
func TestSettingsPage_SendsTheServersFieldNames(t *testing.T) {
	payload := settingsFunc(t, "buildSettingsPayload")
	for _, want := range []string{"start_hhmm:", "end_hhmm:"} {
		if !strings.Contains(payload, want) {
			t.Errorf("buildSettingsPayload does not send %s", want)
		}
	}
	if regexp.MustCompile(`\bstart:\s*document`).MatchString(payload) {
		t.Error("buildSettingsPayload still sends quiet_hours.start (the server reads start_hhmm)")
	}
	mw := settingsFunc(t, "saveMaintenanceWindow")
	for _, want := range []string{"start_iso:", "end_iso:"} {
		if !strings.Contains(mw, want) {
			t.Errorf("saveMaintenanceWindow does not build %s", want)
		}
	}
}

// TestSettingsPage_DoesNotEchoServerOwnedFields: the fleet list and the
// dismissed findings have their own endpoints. Sending this page's copy
// back with every save undid changes made elsewhere.
func TestSettingsPage_DoesNotEchoServerOwnedFields(t *testing.T) {
	payload := settingsFunc(t, "buildSettingsPayload")
	for _, bad := range []string{"\n    fleet:", "\n    dismissed_findings:"} {
		if strings.Contains(payload, bad) {
			t.Errorf("buildSettingsPayload sends %q", strings.TrimSpace(bad))
		}
	}
}

// TestSettingsPage_LoadsEveryFieldItSaves: each form control the payload
// reads must be populated by loadSettings, or a save writes the control's
// default over the stored value.
func TestSettingsPage_LoadsEveryFieldItSaves(t *testing.T) {
	load := settingsFunc(t, "loadSettings")
	for _, id := range []string{
		"notif-cooldown", "qh-timezone", "qh-start", "qh-end",
		"ret-snapshot-days", "ret-max-db", "ret-notify-days",
		"backup-toggle", "backup-path", "backup-keep",
	} {
		if !strings.Contains(load, `"`+id+`"`) {
			t.Errorf("loadSettings never sets #%s, but buildSettingsPayload reads it", id)
		}
	}
	for _, call := range []string{"setQuietHoursEnabledUI(", "setBackupIntervalUI(", "renderMaintenanceWindows()"} {
		if !strings.Contains(load, call) {
			t.Errorf("loadSettings does not call %s", call)
		}
	}
}

// TestSettingsPage_RefusesToSaveBeforeLoad: the form starts with defaults
// and empty lists; a save before the stored settings arrive wiped
// webhooks, service checks and rules.
func TestSettingsPage_RefusesToSaveBeforeLoad(t *testing.T) {
	for _, fn := range []string{"saveSettingsImmediate", "saveSettings"} {
		src := settingsFunc(t, fn)
		if !strings.Contains(src, "if (!settingsLoaded)") {
			t.Errorf("%s does not check settingsLoaded before saving", fn)
		}
	}
	if !strings.Contains(SettingsPage, `<body class="settings-loading">`) {
		t.Error("settings page should start inert until settings load")
	}
}

// TestSettingsPage_EditsKeepUnknownFields: editing or toggling a service
// check, or editing a webhook, must not drop fields the form does not show.
func TestSettingsPage_EditsKeepUnknownFields(t *testing.T) {
	norm := settingsFunc(t, "normalizeServiceCheck")
	if !strings.Contains(norm, "for (var key in sc)") {
		t.Error("normalizeServiceCheck must start from a copy of the stored check")
	}
	if !strings.Contains(settingsFunc(t, "saveWebhook"), "cleanWebhookHeaders()") {
		t.Error("saveWebhook does not store the custom headers")
	}
	if !strings.Contains(settingsFunc(t, "editWebhook"), "wh.headers") {
		t.Error("editWebhook does not load the custom headers")
	}
}
