package api

import (
	"strings"
	"testing"
)

// TestStyles_PillTrace_HasCSSRule pins the CSS rule for the
// traceroute service-check pill. #189 (v0.9.7) added the pill-trace
// class to both service_checks.html and settings.html via pillClass
// / serviceTypePillClass helpers, but the initial implementation
// forgot to add a matching .pill-trace rule to the shared styles.go.
// Result: the Traceroute label in the service-checks list rendered
// without the colored background that every other check type has
// (TCP green, HTTP blue, DNS pink, etc). Caught during rc2 UAT.
//
// This test guards against a regression where the CSS class gets
// renamed / removed on one side but not the other.
func TestStyles_PillTrace_HasCSSRule(t *testing.T) {
	if !strings.Contains(SharedCSS, ".pill-trace") {
		t.Error("styles.go: missing .pill-trace CSS rule — traceroute type pill in service-checks list will render without colored background (other .pill-* rules exist for http/tcp/dns/smb/nfs/ping/speed)")
	}
	// The rule must define both background and color so the pill
	// matches the visual pattern of the other check types.
	lines := strings.Split(SharedCSS, "\n")
	var rule string
	for _, line := range lines {
		if strings.Contains(line, ".pill-trace") {
			rule = line
			break
		}
	}
	if rule == "" {
		t.Fatal("unreachable: first assertion passed but line scan failed")
	}
	if !strings.Contains(rule, "background:") {
		t.Errorf(".pill-trace rule missing background property: %q", rule)
	}
	if !strings.Contains(rule, "color:") {
		t.Errorf(".pill-trace rule missing color property: %q", rule)
	}
}
