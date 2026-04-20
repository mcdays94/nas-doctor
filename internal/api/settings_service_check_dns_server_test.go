package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestSettingsHTML_HasDNSServerInput verifies the settings template ships
// an `sc-dns-server` input inside an `sc-dns-wrap` container, analogous
// to the sc-speed-wrap pattern. Without the input, the feature is
// unreachable from the UI even though the backend supports it.
//
// This is the regression-guard established by #160: we recently shipped
// an interval select that auto-assigned a value with no matching option
// (v0.9.2-rc4→rc5). Cross-referencing the wrap + input + JS hooks here
// is the cheapest way to prevent a similar "backend wired, UI missing"
// ship-blocker.
func TestSettingsHTML_HasDNSServerInput(t *testing.T) {
	html := loadSettingsHTML(t)

	checks := []struct {
		name   string
		substr string
	}{
		{"wrap container", `id="sc-dns-wrap"`},
		{"input element", `id="sc-dns-server"`},
		{"label", `for="sc-dns-server"`},
	}
	for _, tc := range checks {
		if !strings.Contains(html, tc.substr) {
			t.Errorf("settings.html missing %q (expected substring %q)", tc.name, tc.substr)
		}
	}
}

// TestSettingsHTML_DNSServerWrap_ToggledByOnServiceTypeChange verifies
// that onServiceTypeChange shows the DNS wrap only when type === "dns",
// mirroring the sc-speed-wrap visibility logic. Without this wiring the
// input is invisible to users regardless of the selected type.
func TestSettingsHTML_DNSServerWrap_ToggledByOnServiceTypeChange(t *testing.T) {
	html := loadSettingsHTML(t)

	startRe := regexp.MustCompile(`function\s+onServiceTypeChange\s*\(\s*\)\s*\{`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("onServiceTypeChange() not found")
	}
	end := loc[1] + 2500
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]

	if !strings.Contains(body, `"sc-dns-wrap"`) {
		t.Errorf("onServiceTypeChange should reference sc-dns-wrap; body:\n%s", body)
	}
	// Must guard on type === "dns" somewhere in the function body.
	if !regexp.MustCompile(`type\s*===\s*["']dns["']`).MatchString(body) {
		t.Errorf("onServiceTypeChange should check for dns type; body:\n%s", body)
	}
}

// TestSettingsHTML_ReadServiceCheckForm_IncludesDNSServer verifies the
// save path reads sc-dns-server into the payload's dns_server field.
func TestSettingsHTML_ReadServiceCheckForm_IncludesDNSServer(t *testing.T) {
	html := loadSettingsHTML(t)

	startRe := regexp.MustCompile(`function\s+readServiceCheckForm\s*\(\s*\)\s*\{`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("readServiceCheckForm() not found")
	}
	end := loc[1] + 3000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]

	if !strings.Contains(body, `"sc-dns-server"`) {
		t.Errorf("readServiceCheckForm should read sc-dns-server; body:\n%s", body)
	}
	if !strings.Contains(body, "dns_server") {
		t.Errorf("readServiceCheckForm should emit dns_server in payload; body:\n%s", body)
	}
}

// TestSettingsHTML_EditServiceCheck_LoadsDNSServer verifies the edit
// path populates the input from sc.dns_server.
func TestSettingsHTML_EditServiceCheck_LoadsDNSServer(t *testing.T) {
	html := loadSettingsHTML(t)

	startRe := regexp.MustCompile(`function\s+editServiceCheck\s*\(`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("editServiceCheck() not found")
	}
	end := loc[1] + 2000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]

	if !strings.Contains(body, `"sc-dns-server"`) {
		t.Errorf("editServiceCheck should set sc-dns-server input; body:\n%s", body)
	}
	if !strings.Contains(body, "sc.dns_server") {
		t.Errorf("editServiceCheck should read sc.dns_server from loaded config; body:\n%s", body)
	}
}

// TestSettingsHTML_ToggleServiceCheckForm_ResetsDNSServer verifies the
// new-check path clears sc-dns-server so the form doesn't leak a
// previously-edited value.
func TestSettingsHTML_ToggleServiceCheckForm_ResetsDNSServer(t *testing.T) {
	html := loadSettingsHTML(t)

	startRe := regexp.MustCompile(`function\s+toggleServiceCheckForm\s*\(`)
	loc := startRe.FindStringIndex(html)
	if loc == nil {
		t.Fatal("toggleServiceCheckForm() not found")
	}
	end := loc[1] + 2000
	if end > len(html) {
		end = len(html)
	}
	body := html[loc[0]:end]

	if !strings.Contains(body, `"sc-dns-server"`) {
		t.Errorf("toggleServiceCheckForm should reset sc-dns-server; body:\n%s", body)
	}
}
