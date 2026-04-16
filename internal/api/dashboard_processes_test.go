package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDashboardJSContainsProcessesSection verifies that the shared dashboard JS
// module includes a sections.processes renderer function.
func TestDashboardJSContainsProcessesSection(t *testing.T) {
	js := DashboardJS

	checks := []struct {
		name   string
		substr string
	}{
		{"renderer function defined", "sections.processes = function(sn)"},
		{"data-section attribute", `data-section="processes"`},
		{"reads top_processes", "sn.system.top_processes"},
		{"section title", "Top Processes ("},
		{"CPU column header", "CPU%"},
		{"Mem column header", "Mem%"},
		{"container column", "Container"},
		{"user column", "User"},
		{"host badge for non-container procs", ">host</span>"},
		{"container name badge styling", "container_name"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(js, tc.substr) {
				t.Errorf("DashboardJS missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestDashboardJSSectionMapIncludesProcesses verifies the sectionMap in
// distributeSections() includes the processes entry.
func TestDashboardJSSectionMapIncludesProcesses(t *testing.T) {
	js := DashboardJS
	if !strings.Contains(js, `"processes": sec.processes !== false`) {
		t.Error("sectionMap in DashboardJS missing processes entry")
	}
}

// TestDashboardSectionsProcessesField verifies the DashboardSections struct
// serializes the processes field correctly.
func TestDashboardSectionsProcessesField(t *testing.T) {
	// Default: processes is false (Go zero value)
	sec := DashboardSections{}
	b, err := json.Marshal(sec)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := m["processes"]; !ok {
		t.Error("DashboardSections JSON missing 'processes' field")
	}

	// When set to true
	sec.Processes = true
	b, _ = json.Marshal(sec)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if v, ok := m["processes"]; !ok || v != true {
		t.Errorf("expected processes=true, got %v", v)
	}
}

// TestThemeTemplatesIncludeProcessesSection verifies both midnight.html and
// clean.html call sec.processes(sn) in their staging area.
func TestThemeTemplatesIncludeProcessesSection(t *testing.T) {
	templates := []string{"midnight.html", "clean.html"}
	for _, tmpl := range templates {
		t.Run(tmpl, func(t *testing.T) {
			path := filepath.Join("templates", tmpl)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read %s: %v", path, err)
			}
			content := string(data)
			if !strings.Contains(content, "sec.processes(sn)") {
				t.Errorf("%s missing sec.processes(sn) call in staging area", tmpl)
			}
		})
	}
}

// TestSettingsHTMLIncludesProcessesToggle verifies settings.html has the
// processes section toggle and save/load wiring.
func TestSettingsHTMLIncludesProcessesToggle(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	checks := []struct {
		name   string
		substr string
	}{
		{"toggle element", `id="sec-processes"`},
		{"secIds mapping", `"processes":"sec-processes"`},
		{"save payload", `processes: document.getElementById("sec-processes").classList.contains("on")`},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.substr) {
				t.Errorf("settings.html missing %q — expected: %q", tc.name, tc.substr)
			}
		})
	}
}
