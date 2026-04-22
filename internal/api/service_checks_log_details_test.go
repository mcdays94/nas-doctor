package api

import (
	"strings"
	"testing"
)

// TestServiceChecksHTML_LogDetail_RendersPerTypeDetails guards the UI
// changes for issue #182. When a user expands a log entry on the
// /service-checks page, the detail panel must render the per-type
// Details map (HTTP status_code, DNS records, TCP resolved_address,
// Ping rtt_ms + packet_loss_pct, failure_stage for any type) so the
// same rich context the Test button already shows is visible on
// persisted log rows.
//
// We verify by cross-reference — the template must mention the exact
// Details keys the scheduler persists. Keeps the test hermetic (no
// headless browser), catches regressions where a refactor deletes the
// rendering branch for one type.
func TestServiceChecksHTML_LogDetail_RendersPerTypeDetails(t *testing.T) {
	html := loadServiceChecksHTML(t)

	// The renderer helper must exist and be invoked from the log expand
	// path. Naming follows #154's settings.html helper so a future refactor
	// can promote it without renaming callers.
	mustContain(t, html, "renderServiceCheckDetails",
		"log expand path must reference the renderServiceCheckDetails helper")

	// The entry's details payload must be consumed. ServiceCheckEntry's
	// JSON tag is "details,omitempty"; the template reads e.details.
	mustContain(t, html, "e.details",
		"log expand path must read e.details from the persisted entry")

	// Per-type keys — match the set the scheduler runners populate.
	// One failure here pinpoints exactly which branch regressed.
	perTypeKeys := map[string]string{
		"HTTP status_code":     "status_code",
		"HTTP content_type":    "content_type",
		"HTTP body_bytes":      "body_bytes",
		"HTTP final_url":       "final_url",
		"DNS records":          "records",
		"DNS query_host":       "query_host",
		"DNS dns_server":       "dns_server",
		"TCP resolved_address": "resolved_address",
		"Ping rtt_ms":          "rtt_ms",
		"Ping packet_loss_pct": "packet_loss_pct",
		"Shared failure_stage": "failure_stage",
	}
	for label, key := range perTypeKeys {
		if !strings.Contains(html, key) {
			t.Errorf("service_checks.html missing %s key %q — log detail panel won't render it", label, key)
		}
	}
}

// TestServiceChecksHTML_LogDetail_ExistingFieldsStillRendered — sanity
// guard against accidentally losing the original fields in the detail
// panel while wiring up the new helper. These were shipped by the
// expand-row UI prior to #182 (Check Name, Type, Target, Status, Response
// Time, Checked At, etc.); they must still appear.
func TestServiceChecksHTML_LogDetail_ExistingFieldsStillRendered(t *testing.T) {
	html := loadServiceChecksHTML(t)
	required := []string{
		"Check Name",
		"Target",
		"Response Time",
		"Checked At",
		"Consecutive Failures",
		"Check Key",
	}
	for _, label := range required {
		if !strings.Contains(html, label) {
			t.Errorf("service_checks.html lost legacy detail-panel label %q during #182 wiring", label)
		}
	}
}

func mustContain(t *testing.T, haystack, needle, why string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s\n  expected substring: %q", why, needle)
	}
}
