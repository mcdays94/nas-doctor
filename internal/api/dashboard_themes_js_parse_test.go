package api

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDashboardThemes_JSBlocksParse extends the v0.9.9 rc1
// regression guard (see settings_js_parses_test.go for the full
// backstory) to the two dashboard theme templates.
//
// midnight.html and clean.html each contain a sizeable inline
// <script> block that renders the entire dashboard. A `*/` inside
// a `/* */` block comment, or a stray backtick inside a JS comment,
// would silently abort parsing and leave the dashboard a blank
// page in browsers — exactly the v0.9.9-rc1 class of failure but
// affecting the dashboard rather than settings.
//
// Issue #269 introduces new inline JS in BOTH theme templates
// (the conditional cpu_temp_c / mobo_temp_c stat-item rendering),
// which is high-risk for this footgun. Hence this guard.
func TestDashboardThemes_JSBlocksParse(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node binary not in PATH; skipping JS parse check (dev-time guard only)")
	}

	cases := []struct {
		name string
		tmpl string
	}{
		{"midnight.html", DashboardMidnight},
		{"clean.html", DashboardClean},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := extractScriptBlocks(tc.tmpl)
			if len(blocks) == 0 {
				t.Fatalf("no <script>...</script> blocks found in %s", tc.name)
			}
			for i, js := range blocks {
				if strings.TrimSpace(js) == "" {
					continue
				}
				cmd := exec.Command("node", "--check", "-")
				cmd.Stdin = strings.NewReader(js)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Errorf("%s <script> block %d failed to parse as JS:\n%s", tc.name, i, string(out))
				}
			}
		})
	}
}
