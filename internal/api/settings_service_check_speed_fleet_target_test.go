package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestSettingsHTML_OnServiceTypeChange_HidesInstanceForSpeedType pins
// issue #215's UI fix: when the user picks type=speed in the
// service-check editor, onServiceTypeChange must hide the
// sc-instance-wrap (the fleet-target picker) AND coerce the
// sc-instance value back to "" so the saved payload doesn't carry a
// stale fleet ID.
//
// Background: every service-check type ignores ServiceCheckConfig.
// Instance — the scheduler does not dispatch to peers, it always runs
// the check against local state (resolvers, sockets, history rows).
// For type=speed this is especially confusing because the dashboard
// reads from the local speedtest_history table; selecting a peer in
// the dropdown would silently measure the local instance's WAN. The
// safest UX is to remove the option for the type that makes the
// confusion most acute.
//
// If a future PR wires up real fleet dispatch (likely alongside #205
// — Uptime Kuma federation — which needs the same primitives), this
// test will fail and the diff will surface the deliberate behaviour
// change.
func TestSettingsHTML_OnServiceTypeChange_HidesInstanceForSpeedType(t *testing.T) {
	html := loadSettingsHTML(t)

	startRe := regexp.MustCompile(`function\s+onServiceTypeChange\s*\(\s*\)\s*\{`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("onServiceTypeChange() not found in settings.html")
	}
	// Function body window — generous because this function has grown
	// over time as new check types were added; 4 KB easily covers it.
	end := loc[1] + 4000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]

	if !strings.Contains(body, `"sc-instance-wrap"`) {
		t.Errorf("onServiceTypeChange should reference sc-instance-wrap; body:\n%s", body)
	}
	if !strings.Contains(body, `"sc-instance"`) {
		t.Errorf("onServiceTypeChange should reference sc-instance (to clear its value); body:\n%s", body)
	}
	// The function body must contain a guard that, when type === "speed",
	// hides the wrap. Two-axis check: (1) the type comparison appears,
	// (2) the wrap's display is set to "none" within speed-type scope.
	speedGuard := regexp.MustCompile(`type\s*===\s*["']speed["']`)
	if !speedGuard.MatchString(body) {
		t.Errorf("onServiceTypeChange should branch on type=='speed'; body:\n%s", body)
	}
	// String-match on the precise pair we wrote to make a "fixed but
	// later silently regressed" outcome impossible: hiding the wrap
	// AND clearing the value have to both be present. If a future
	// refactor changes the local variable name, this test will fail
	// and force the author to update the regression guard
	// deliberately.
	if !regexp.MustCompile(`instanceWrap\.style\.display\s*=\s*"none"`).MatchString(body) {
		t.Errorf("onServiceTypeChange should set instanceWrap.style.display=\"none\" for type=speed; body:\n%s", body)
	}
	if !regexp.MustCompile(`instanceSelect\.value\s*=\s*""`).MatchString(body) {
		t.Errorf("onServiceTypeChange should clear instanceSelect.value for type=speed; body:\n%s", body)
	}
}

// TestSettingsHTML_InstanceFieldStillExistsForOtherTypes is a defensive
// guard: the fix for #215 is type-targeted; it must NOT remove the
// sc-instance-wrap markup from the template (existing fleet-targeted
// HTTP/TCP/etc. checks still need to render their picker on edit). We
// pin both the wrapper element and the select so a future cleanup PR
// that decides to remove the picker globally is forced to update this
// test deliberately.
func TestSettingsHTML_InstanceFieldStillExistsForOtherTypes(t *testing.T) {
	html := loadSettingsHTML(t)

	if !strings.Contains(html, `id="sc-instance-wrap"`) {
		t.Error("settings.html missing sc-instance-wrap container — fleet picker removed entirely?")
	}
	if !strings.Contains(html, `id="sc-instance"`) {
		t.Error("settings.html missing sc-instance select — fleet picker removed entirely?")
	}
}
