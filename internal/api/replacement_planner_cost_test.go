package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cost_per_tb is entered in the user's own currency (Settings says so),
// so the Replacement Planner must not print a "$" in front of the
// amounts it derives from it. This runs the planner's render script in
// node with a stub document and fetch, and checks the HTML it builds.
// Skips when node isn't installed.

// replacementPlannerCostHarnessJS stubs just enough DOM for render():
// getElementById("app") and createElement for esc(). fetch resolves to
// a plan with every cost figure set. No backticks: Go raw string.
const replacementPlannerCostHarnessJS = `
var app = { innerHTML: "" };
global.document = {
  title: "",
  getElementById: function(id) { return app; },
  createElement: function() {
    var t = "";
    return {
      set textContent(v) { t = String(v); },
      get innerHTML() { return t.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
    };
  }
};
var plan = {
  total_drives: 3, replace_now: 1, replace_soon: 1, monitor: 0, healthy: 1,
  cost_configured: true, total_cost: 540, total_cost_all: 810,
  data_version: "Q2 2026",
  drives: [
    { device: "sda", model: "M1", disk_type: "hdd", size_gb: 12000, urgency: "replace_now", urgency_label: "", health_score: 20, life_used_pct: 90, power_on_hours: 40000, remaining_years: 0.5, failure_mult: 3, risk_factors: [], cost_estimate: 270 },
    { device: "sdb", model: "M2", disk_type: "hdd", size_gb: 12000, urgency: "replace_soon", urgency_label: "", health_score: 50, life_used_pct: 70, power_on_hours: 30000, remaining_years: 1.5, failure_mult: 2, risk_factors: [], cost_estimate: 270 },
    { device: "sdc", model: "M3", disk_type: "hdd", size_gb: 12000, urgency: "healthy", urgency_label: "", health_score: 95, life_used_pct: 10, power_on_hours: 5000, remaining_years: 6, failure_mult: 1, risk_factors: [], cost_estimate: 270 }
  ]
};
global.fetch = function() { return Promise.resolve({ json: function() { return plan; } }); };
`

func TestReplacementPlanner_CostsHaveNoDollarSign(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node binary not in PATH; skipping planner render check")
	}

	var renderJS string
	for _, js := range extractScriptBlocks(replacementPlannerHTML) {
		if strings.Contains(js, "function render(plan)") {
			renderJS = js
		}
	}
	if renderJS == "" {
		t.Fatal("render(plan) script block not found in replacement_planner.html")
	}

	script := replacementPlannerCostHarnessJS + renderJS +
		"\nsetTimeout(function() { process.stdout.write(app.innerHTML); }, 0);\n"
	path := filepath.Join(t.TempDir(), "planner.js")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", path).CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	html := string(out)

	if !strings.Contains(html, "Estimated Replacement Costs") {
		t.Fatalf("cost box not rendered; got:\n%s", html)
	}
	if strings.Contains(html, "$") {
		t.Errorf("planner output contains \"$\" but cost_per_tb is in the user's currency:\n%s", html)
	}
	for _, want := range []string{">540<", ">810<", ">270<"} {
		if !strings.Contains(html, want) {
			t.Errorf("planner output missing amount %q", want)
		}
	}
}
