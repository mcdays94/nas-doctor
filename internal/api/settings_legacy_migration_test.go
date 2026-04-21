package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// putSettings issues a PUT /api/v1/settings against the given test server and
// returns the recorder for inspection.
func putSettings(t *testing.T, srv *Server, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleUpdateSettings(rec, req)
	return rec
}

// getSettings issues a GET /api/v1/settings and parses the response.
func getSettings(t *testing.T, srv *Server) Settings {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec := httptest.NewRecorder()
	srv.handleGetSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/settings: %d %s", rec.Code, rec.Body.String())
	}
	var s Settings
	body, _ := io.ReadAll(rec.Body)
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return s
}

// seedRawSettings writes a JSON blob straight into the store under the
// settings key, bypassing the update handler — this simulates settings.json
// written by an older version that would now fail validation.
func seedRawSettings(t *testing.T, srv *Server, raw string) {
	t.Helper()
	if err := srv.store.SetConfig(settingsConfigKey, raw); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

// TestHandleUpdateSettings_InvalidCheck_ErrorNamesTheCheck — Phase 1:
// when a check fails validation the error must identify which check
// by name so users can self-diagnose. Regression for #169 where users
// upgrading from v0.9.2 with a DNS-IP check received a flat error
// that seemed to reference the check they were currently editing.
func TestHandleUpdateSettings_InvalidCheck_ErrorNamesTheCheck(t *testing.T) {
	srv := newSettingsTestServer()
	rec := putSettings(t, srv, map[string]any{
		"scan_interval": "30m",
		"theme":         "midnight",
		"service_checks": map[string]any{
			"checks": []map[string]any{
				{
					"name":    "my-legacy-dns",
					"type":    "dns",
					"target":  "1.1.1.1",
					"enabled": true,
				},
			},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "my-legacy-dns") {
		t.Fatalf("error must identify failing check by name 'my-legacy-dns', got: %s", rec.Body.String())
	}
}

// TestHandleUpdateSettings_MultipleInvalidChecks_AllReported — Phase 1:
// the validator must accumulate errors across all checks and surface
// all failures, not short-circuit on the first. Users editing an
// unrelated check shouldn't have to fix each broken check one by one.
func TestHandleUpdateSettings_MultipleInvalidChecks_AllReported(t *testing.T) {
	srv := newSettingsTestServer()
	rec := putSettings(t, srv, map[string]any{
		"scan_interval": "30m",
		"theme":         "midnight",
		"service_checks": map[string]any{
			"checks": []map[string]any{
				{"name": "bad-one", "type": "dns", "target": "1.1.1.1", "enabled": true},
				{"name": "bad-two", "type": "dns", "target": "8.8.8.8", "enabled": true},
			},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "bad-one") {
		t.Errorf("expected 'bad-one' in error body, got: %s", body)
	}
	if !strings.Contains(body, "bad-two") {
		t.Errorf("expected 'bad-two' in error body, got: %s", body)
	}
}

// TestSettingsLoad_DNSIPCheck_SoftDisabled — Phase 2: when the stored
// settings contain a legacy DNS check with an IP target (valid under
// v0.9.2 but invalid under v0.9.3+), the load path must mark it
// disabled and surface a user-visible warning, NOT silently drop it.
func TestSettingsLoad_DNSIPCheck_SoftDisabled(t *testing.T) {
	srv := newSettingsTestServer()
	seedRawSettings(t, srv, `{
		"scan_interval": "30m",
		"theme": "midnight",
		"service_checks": {
			"checks": [
				{"name": "legacy-dns", "type": "dns", "target": "1.1.1.1", "enabled": true}
			]
		}
	}`)

	loaded := getSettings(t, srv)
	if len(loaded.ServiceChecks.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(loaded.ServiceChecks.Checks))
	}
	check := loaded.ServiceChecks.Checks[0]
	if check.Enabled {
		t.Errorf("expected legacy DNS-IP check to be auto-disabled on load, Enabled=true")
	}
	if strings.TrimSpace(check.Warning) == "" {
		t.Errorf("expected non-empty Warning on auto-disabled legacy check, got %q", check.Warning)
	}
	// Stored config must remain untouched — we do NOT rewrite disk.
	raw, err := srv.store.GetConfig(settingsConfigKey)
	if err != nil {
		t.Fatalf("load raw stored settings: %v", err)
	}
	if !strings.Contains(raw, `"enabled": true`) && !strings.Contains(raw, `"enabled":true`) {
		t.Errorf("stored settings should not be silently mutated; got: %s", raw)
	}
}

// TestSettingsLoad_Then_UpdateSucceeds_WithLegacyCheck — Phase 2 + save path:
// after load-time soft-disable marks a legacy DNS-IP check with
// Enabled=false, round-tripping that check back through PUT must NOT
// re-trigger the validator (disabled checks can't run, so their
// invalid target is harmless). This is the whole point of #169 —
// unsticking the save path for upgraders.
func TestSettingsLoad_Then_UpdateSucceeds_WithLegacyCheck(t *testing.T) {
	srv := newSettingsTestServer()
	seedRawSettings(t, srv, `{
		"scan_interval": "30m",
		"theme": "midnight",
		"service_checks": {
			"checks": [
				{"name": "legacy-dns", "type": "dns", "target": "1.1.1.1", "enabled": true}
			]
		}
	}`)

	loaded := getSettings(t, srv)

	// Simulate the UI editing an unrelated setting while keeping the
	// (auto-disabled) legacy check in the payload. Also add a fresh
	// valid HTTP check — this is the scenario from the issue.
	loaded.ServiceChecks.Checks = append(loaded.ServiceChecks.Checks, internal.ServiceCheckConfig{
		Name:    "new-http",
		Type:    "http",
		Target:  "https://example.com",
		Enabled: true,
	})

	rec := putSettings(t, srv, loaded)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 — save must succeed with legacy disabled check + fresh valid check; got %d: %s", rec.Code, rec.Body.String())
	}

	// Reload and assert both checks persist.
	after := getSettings(t, srv)
	if len(after.ServiceChecks.Checks) != 2 {
		t.Fatalf("expected 2 checks after save, got %d", len(after.ServiceChecks.Checks))
	}
	names := map[string]bool{}
	var legacy *internal.ServiceCheckConfig
	for i := range after.ServiceChecks.Checks {
		c := &after.ServiceChecks.Checks[i]
		names[c.Name] = true
		if c.Name == "legacy-dns" {
			legacy = c
		}
	}
	if !names["legacy-dns"] || !names["new-http"] {
		t.Fatalf("expected both checks persisted, got %v", names)
	}
	if legacy == nil {
		t.Fatal("legacy check not found")
	}
	if legacy.Enabled {
		t.Errorf("legacy check should remain disabled after save, got Enabled=true")
	}
}

// TestSettingsLoad_ValidDNSCheck_NotTouched — a DNS check with a
// hostname target is valid under the current schema and must pass
// through load+save untouched (no warning, enabled preserved). This
// guards against the Phase 2 migration over-reaching.
func TestSettingsLoad_ValidDNSCheck_NotTouched(t *testing.T) {
	srv := newSettingsTestServer()
	seedRawSettings(t, srv, `{
		"scan_interval": "30m",
		"theme": "midnight",
		"service_checks": {
			"checks": [
				{"name": "good-dns", "type": "dns", "target": "google.com", "enabled": true}
			]
		}
	}`)

	loaded := getSettings(t, srv)
	if len(loaded.ServiceChecks.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(loaded.ServiceChecks.Checks))
	}
	c := loaded.ServiceChecks.Checks[0]
	if !c.Enabled {
		t.Errorf("valid DNS hostname check must stay enabled on load, got Enabled=false")
	}
	if strings.TrimSpace(c.Warning) != "" {
		t.Errorf("valid DNS hostname check must have empty Warning, got %q", c.Warning)
	}

	// Resave — also must round-trip cleanly.
	rec := putSettings(t, srv, loaded)
	if rec.Code != http.StatusOK {
		t.Fatalf("resave failed: %d %s", rec.Code, rec.Body.String())
	}
	after := getSettings(t, srv)
	if !after.ServiceChecks.Checks[0].Enabled {
		t.Errorf("valid DNS check flipped disabled on resave")
	}
	if strings.TrimSpace(after.ServiceChecks.Checks[0].Warning) != "" {
		t.Errorf("valid DNS check acquired Warning on resave: %q", after.ServiceChecks.Checks[0].Warning)
	}
}

// TestSettingsLoad_LegacyCheck_FixingTargetRestoresCheck — when the
// user edits the auto-disabled legacy check to use a hostname target
// and clears the Warning field (as a UI would when the user applies
// the "fix"), the check validates normally on save. Enabled stays
// false until the user explicitly re-enables — we never silently flip
// a user-disabled check back on.
func TestSettingsLoad_LegacyCheck_FixingTargetRestoresCheck(t *testing.T) {
	srv := newSettingsTestServer()
	seedRawSettings(t, srv, `{
		"scan_interval": "30m",
		"theme": "midnight",
		"service_checks": {
			"checks": [
				{"name": "legacy-dns", "type": "dns", "target": "1.1.1.1", "enabled": true}
			]
		}
	}`)

	loaded := getSettings(t, srv)
	// UI-equivalent edit: target fixed, warning cleared, enabled left
	// at false (user must opt back in explicitly).
	loaded.ServiceChecks.Checks[0].Target = "google.com"
	loaded.ServiceChecks.Checks[0].Warning = ""

	rec := putSettings(t, srv, loaded)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after target fix, got %d: %s", rec.Code, rec.Body.String())
	}
	after := getSettings(t, srv)
	if len(after.ServiceChecks.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(after.ServiceChecks.Checks))
	}
	c := after.ServiceChecks.Checks[0]
	if c.Enabled {
		t.Errorf("fixed check must stay disabled until user re-enables, got Enabled=true")
	}
	if strings.TrimSpace(c.Warning) != "" {
		t.Errorf("fixed check must not re-acquire warning, got %q", c.Warning)
	}
	if c.Target != "google.com" {
		t.Errorf("target not persisted: got %q", c.Target)
	}
}

// TestHandleUpdateSettings_FreshDNSIPCheck_StillRejected — do NOT regress #159:
// a freshly submitted DNS check with an IP target (enabled=true) must
// still be rejected with 400. Only pre-existing checks that arrived
// via the load-time migration path (enabled=false) are grandfathered.
func TestHandleUpdateSettings_FreshDNSIPCheck_StillRejected(t *testing.T) {
	srv := newSettingsTestServer()
	rec := putSettings(t, srv, map[string]any{
		"scan_interval": "30m",
		"theme":         "midnight",
		"service_checks": map[string]any{
			"checks": []map[string]any{
				{"name": "brand-new", "type": "dns", "target": "1.1.1.1", "enabled": true},
			},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("fresh DNS-IP check must still be rejected (issue #159); got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "hostname") {
		t.Errorf("expected hostname guidance in error, got: %s", rec.Body.String())
	}
}
