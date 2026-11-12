package api

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// allPageTemplates lists every embedded page, including the sign-in page.
func allPageTemplates() map[string]string {
	return map[string]string{
		"midnight.html":            DashboardMidnight,
		"clean.html":               DashboardClean,
		"stats.html":               statsPageHTML,
		"fleet.html":               fleetPageHTML,
		"alerts.html":              alertsPageHTML,
		"service_checks.html":      serviceChecksPageHTML,
		"parity.html":              parityPageHTML,
		"replacement_planner.html": replacementPlannerHTML,
		"settings.html":            SettingsPage,
		"disk_detail.html":         DiskDetailPage,
		"login.html":               loginPageHTML,
	}
}

// TestAllTemplates_JSBlocksParse runs `node --check` over every inline
// script of every page. One unparseable block silently disables a page.
func TestAllTemplates_JSBlocksParse(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node binary not in PATH; skipping JS parse check")
	}
	for name, tpl := range allPageTemplates() {
		for i, js := range extractScriptBlocks(tpl) {
			if strings.TrimSpace(js) == "" {
				continue
			}
			cmd := exec.Command("node", "--check", "-")
			cmd.Stdin = strings.NewReader(js)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s <script> block %d does not parse:\n%s", name, i, out)
			}
		}
	}
	for name, js := range map[string]string{"charts.js": ChartJS, "dashboard.js": DashboardJS} {
		cmd := exec.Command("node", "--check", "-")
		cmd.Stdin = strings.NewReader(js)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s does not parse:\n%s", name, out)
		}
	}
}

// TestTemplates_NoThirdPartyLookups: the service-checks page used to ask
// Google's favicon service for an icon per monitored host, which sent
// internal hostnames and addresses off the network on every page load.
// Fonts are the one external resource left, and the CSP allows only them.
func TestTemplates_NoThirdPartyLookups(t *testing.T) {
	urlRE := regexp.MustCompile(`(?:src|href)=\\?["']?(https?://[^"'\s>\\]+)`)
	for name, tpl := range allPageTemplates() {
		if strings.Contains(tpl, "google.com/s2/favicons") {
			t.Errorf("%s still fetches favicons from a third party", name)
		}
		for _, m := range urlRE.FindAllStringSubmatch(tpl, -1) {
			u := m[1]
			switch {
			case strings.HasPrefix(u, "https://fonts.googleapis.com"), strings.HasPrefix(u, "https://fonts.gstatic.com"):
			case strings.HasPrefix(u, "https://github.com/"), strings.HasPrefix(u, "https://buymeacoffee.com/"):
				// plain links the user clicks, not resources the page loads
			default:
				t.Errorf("%s references external URL %q; the Content-Security-Policy does not allow loading it", name, u)
			}
		}
	}
}

// TestPages_EscapeUntrustedText pins the injection fixes.
func TestPages_EscapeUntrustedText(t *testing.T) {
	preview := settingsFunc(t, "updateRulePreview")
	if !strings.Contains(preview, "escapeHtml(targetLabel)") {
		t.Error("rule preview inserts the target label without escaping")
	}
	if strings.Contains(SettingsPage, `'<option value="' + d.node_names[i]`) || strings.Contains(SettingsPage, `'<option value="' + pve.node_name`) {
		t.Error("Proxmox node names are concatenated into option markup")
	}
	if strings.Contains(alertsPageHTML, `loadAlertEvents(' + a.id + ', \''`) {
		t.Error("alert title is quoted into an inline handler's JavaScript")
	}
	if !strings.Contains(alertsPageHTML, "data-alert-title=") {
		t.Error("alert Timeline button should carry the title in a data attribute")
	}
	if strings.Contains(fleetPageHTML, `(n.overall_health||"?")`) {
		t.Error("fleet page writes a peer's health string into the page unescaped")
	}
	if !strings.Contains(ChartJS, "tipEsc(d.label") {
		t.Error("chart tooltips insert series labels without escaping")
	}
	if strings.Contains(DashboardJS, `var storageLabel = dk.label || dk.mount_point;`) {
		t.Error("dashboard storage label is not escaped")
	}
	if strings.Contains(DashboardJS, `onclick="window.location=\'/disk/'`) {
		t.Error("drive row builds its link inside an inline handler string")
	}
}
