package api

import (
	"regexp"
	"strings"
	"testing"
)

// canonicalPillMapping is the single source of truth for how every
// service-check type maps to its CSS pill class. Every place in the
// codebase that renders a type-pill (settings.html, service_checks.html,
// dashboard.go's DashboardJS) must agree on this mapping.
//
// Adding a new check type? Add it here and all three helpers become
// testable against it in one go.
type canonicalPillMapping struct {
	typeName  string
	pillClass string
}

var canonicalPillMap = []canonicalPillMapping{
	{"http", "pill-http"},
	{"tcp", "pill-tcp"},
	{"dns", "pill-dns"},
	{"ping", "pill-ping"},
	{"smb", "pill-smb"},
	{"nfs", "pill-nfs"},
	{"speed", "pill-speed"},
	{"traceroute", "pill-trace"},
}

// TestServiceCheckPillHelpers_AgreeAcrossTemplates catches the
// "N independent copies of the same mapping, one of them drifts" bug.
//
// Background: before v0.9.7 rc4, three separate pill-class helpers
// lived in the codebase:
//
//	1. settings.html serviceTypePillClass(t)        — correct
//	2. service_checks.html pillClass(t)              — wrong: nfs→pill-smb
//	3. (none for dashboard widget — rendered plain text)
//
// This test asserts that every known type maps to its approved class
// in every file that contains a helper. If a new type is added to
// canonicalPillMap above, every helper must be updated or this test
// fails. If a future refactor consolidates the three helpers into
// one, this test still passes (it just becomes tautological).
//
// #189 rc4 / v0.9.7.
func TestServiceCheckPillHelpers_AgreeAcrossTemplates(t *testing.T) {
	sources := map[string]string{
		"settings.html (serviceTypePillClass)": loadSettingsHTML(t),
		"service_checks.html (pillClass)":      loadServiceChecksHTML(t),
		"dashboard.go (util.pillClass)":        DashboardJS,
	}

	for source, content := range sources {
		for _, m := range canonicalPillMap {
			// A helper mapping type -> class will have both literals
			// close together (e.g. `if(t==="nfs")return"pill-nfs"`).
			// Anchor on the type string, allow up to 80 chars of
			// intervening syntax, then assert the class literal.
			pattern := `"` + m.typeName + `"[^}]{0,80}"` + m.pillClass + `"`
			matched, err := regexp.MatchString(pattern, content)
			if err != nil {
				t.Fatalf("regex error for %s[%s]: %v", source, m.typeName, err)
			}
			if !matched {
				t.Errorf("%s: missing or wrong mapping for type %q — expected class %q", source, m.typeName, m.pillClass)
			}
		}
	}
}

// TestDashboardJS_ServiceCheckTypePill asserts the Services section of
// the dashboard widget renders the TYPE column as a coloured pill span
// (matching /service-checks and /settings) rather than plain uppercase
// text. Caught during v0.9.7 rc4 UAT: the dashboard Service Checks
// widget rendered PING / HTTP / DNS / TCP / TRACEROUTE as grey plain
// text in the TYPE column while the same data on /service-checks
// rendered coloured pills.
//
// #189 rc4 / v0.9.7.
func TestDashboardJS_ServiceCheckTypePill(t *testing.T) {
	t.Run("pill_class_helper_is_defined", func(t *testing.T) {
		// The helper must exist as util.pillClass so downstream
		// code can call it without re-implementing the mapping.
		if !strings.Contains(DashboardJS, "util.pillClass = function") {
			t.Fatal("util.pillClass helper missing from DashboardJS — every other type-pill renderer has an equivalent helper")
		}
	})

	t.Run("old_plain_text_rendering_removed", func(t *testing.T) {
		// The rc3 rendering was:
		//   h += '<td ...>' + esc((sc.type || '').toUpperCase()) + '</td>';
		// — grey plain text. The fix replaces it with a pill span.
		if strings.Contains(DashboardJS, `esc((sc.type || '').toUpperCase()) + '</td>'`) {
			t.Error("dashboard services section still renders type as plain toUpperCase() text; expected coloured pill span via util.pillClass")
		}
	})

	t.Run("services_section_uses_pill_helper", func(t *testing.T) {
		// Anchor on the section header so we're certain we're
		// asserting against the right block (DashboardJS has many
		// section renderers).
		idx := strings.Index(DashboardJS, "Service Checks (")
		if idx < 0 {
			t.Fatal("'Service Checks (' header anchor not found in DashboardJS — test can't locate the right section")
		}
		// Scan a generous window after the anchor; the current
		// services renderer is under 2kB.
		end := idx + 3000
		if end > len(DashboardJS) {
			end = len(DashboardJS)
		}
		section := DashboardJS[idx:end]

		if !strings.Contains(section, "util.pillClass") {
			t.Error("services section doesn't reference util.pillClass — the TYPE column probably still renders as plain text")
		}
		if !strings.Contains(section, `class="pill `) {
			t.Error("services section missing 'class=\"pill ...\"' span wrapper around the type")
		}
	})
}
