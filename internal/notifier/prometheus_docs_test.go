package notifier

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// repoRoot returns the absolute path to the repository root.
// Tests in this file live at <root>/internal/notifier, so we walk up two levels.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// TestReadme_PrometheusCountConsistent guards against the v0.9.7-era drift where
// README.md, the metric-list collapsible, and unraid-template.xml each advertised
// a different metric count ("90+ gauges", "80+ metrics", "30+ gauges"). All three
// sources MUST agree on the same round-down threshold so users (and downstream
// app stores like Unraid CA / TrueNAS) see a single consistent number.
//
// The canonical phrasing is "<N>+ gauges" / "<N>+ metrics" where N is the
// nearest round-down to the ACTUAL exported metric count (see
// TestPrometheus_ActualMetricCount below).
//
// If you add new metrics and bump the threshold, update ALL three sources in
// lockstep:
//   - README.md line ~205 (Integrations table)
//   - README.md line ~572 (collapsible metric list <summary>)
//   - unraid-template.xml line ~29 (Overview field)
//   - the companion mcdays94/docker-templates/nas-doctor/nas-doctor.xml
func TestReadme_PrometheusCountConsistent(t *testing.T) {
	root := repoRoot(t)

	readmeBytes, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	readme := string(readmeBytes)

	xmlBytes, err := os.ReadFile(filepath.Join(root, "unraid-template.xml"))
	if err != nil {
		t.Fatalf("read unraid-template.xml: %v", err)
	}
	xml := string(xmlBytes)

	// Integrations-table phrasing: "Scrape `/metrics` — <N>+ gauges for ..."
	integRe := regexp.MustCompile(`Scrape ` + "`/metrics`" + ` — (\d+)\+\s*gauges`)
	integMatch := integRe.FindStringSubmatch(readme)
	if integMatch == nil {
		t.Fatalf("integrations-table Prometheus row not found in README.md (looking for `Scrape /metrics — N+ gauges`)")
	}
	integN, _ := strconv.Atoi(integMatch[1])

	// Collapsible-list phrasing: "<summary>Expand metric list (<N>+ metrics)</summary>"
	listRe := regexp.MustCompile(`Expand metric list \((\d+)\+\s*metrics\)`)
	listMatch := listRe.FindStringSubmatch(readme)
	if listMatch == nil {
		t.Fatalf("collapsible metric-list summary not found in README.md (looking for `Expand metric list (N+ metrics)`)")
	}
	listN, _ := strconv.Atoi(listMatch[1])

	// Unraid-template Overview phrasing: "Prometheus /metrics endpoint (<N>+ gauges)"
	xmlRe := regexp.MustCompile(`Prometheus /metrics endpoint \((\d+)\+\s*gauges\)`)
	xmlMatch := xmlRe.FindStringSubmatch(xml)
	if xmlMatch == nil {
		t.Fatalf("Overview field Prometheus line not found in unraid-template.xml (looking for `Prometheus /metrics endpoint (N+ gauges)`)")
	}
	xmlN, _ := strconv.Atoi(xmlMatch[1])

	if integN != listN || listN != xmlN {
		t.Errorf("Prometheus metric count drift detected — README integrations table says %d+, README metric list says %d+, unraid-template.xml says %d+. All three MUST match.",
			integN, listN, xmlN)
	}
}

// TestPrometheus_ActualMetricCount counts the actual metric registrations in
// prometheus.go and asserts the documented threshold is a sensible round-down.
//
// "Sensible" means: the documented N must satisfy N <= actual < N + 50, i.e.
// the README claim is honest (we have at least N) but not stale by more than
// half a hundred (catches "we added 30 metrics and forgot to bump the docs").
func TestPrometheus_ActualMetricCount(t *testing.T) {
	root := repoRoot(t)

	src, err := os.ReadFile(filepath.Join(root, "internal/notifier/prometheus.go"))
	if err != nil {
		t.Fatalf("read prometheus.go: %v", err)
	}

	// Each metric is registered as `m.<field> = gauge(ns, ...)` or `gaugeVec(ns, ...)`.
	// Counting these gives us 1 distinct exported metric NAME per match (subsystem+name
	// pairs are unique across the file — verified by hand at audit time).
	registrationRe := regexp.MustCompile(`(?m)^\s*m\.[A-Za-z0-9_]+ = (gauge|gaugeVec)\(ns,`)
	actual := len(registrationRe.FindAll(src, -1))
	if actual == 0 {
		t.Fatal("found 0 metric registrations — regex is broken or prometheus.go was restructured")
	}

	// Pull the documented threshold from README's integrations-table phrasing.
	readmeBytes, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	integRe := regexp.MustCompile(`Scrape ` + "`/metrics`" + ` — (\d+)\+\s*gauges`)
	m := integRe.FindStringSubmatch(string(readmeBytes))
	if m == nil {
		t.Fatal("integrations-table Prometheus row not found in README.md")
	}
	documented, _ := strconv.Atoi(m[1])

	if actual < documented {
		t.Errorf("README claims %d+ gauges but only %d are registered in prometheus.go — docs overstate reality", documented, actual)
	}
	if actual >= documented+50 {
		t.Errorf("README claims %d+ gauges but %d are registered — docs are stale by 50+, bump the threshold to the next round-down (e.g. %d)",
			documented, actual, (actual/10)*10)
	}
}
