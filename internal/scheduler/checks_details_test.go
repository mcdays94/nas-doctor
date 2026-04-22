package scheduler

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// These tests cover the opt-in "collect details" code path added for the
// Test-button flow (issue #154). The scheduled-check path (RunDueChecks)
// must remain unchanged — no extra detail overhead on every 30s run.

// ── Phase 1: opt-in toggle ─────────────────────────────────────────────

// TestRunCheck_Details_OmittedByDefault — the Details field must be empty
// (nil or zero-length) when the collect-details toggle has not been enabled,
// so scheduled checks carry zero detail overhead over the wire.
func TestRunCheck_Details_OmittedByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:    "default-no-details",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}, time.Now().UTC())

	if result.Status != "up" {
		t.Fatalf("expected status up, got %q (error=%q)", result.Status, result.Error)
	}
	if len(result.Details) != 0 {
		t.Fatalf("expected no details on scheduled path, got %d keys: %+v", len(result.Details), result.Details)
	}
}

// TestRunCheck_Details_PopulatedWhenEnabled — once SetCollectDetails(true)
// has been called, subsequent RunCheck calls populate the Details map.
func TestRunCheck_Details_PopulatedWhenEnabled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)

	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:    "with-details",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}, time.Now().UTC())

	if result.Status != "up" {
		t.Fatalf("expected up, got %q (error=%q)", result.Status, result.Error)
	}
	if len(result.Details) == 0 {
		t.Fatal("expected non-empty Details when SetCollectDetails(true)")
	}
}

// TestRunDueChecks_OmitsDetailsWhenCollectorOff — with SetCollectDetails
// left at its default (false), scheduled check rows must NOT carry
// Details. This preserves the pre-#182 on-the-wire shape for integrations
// that haven't opted in (e.g. unit tests constructing a bare checker via
// NewServiceChecker). Production wires details on — see scheduler.go —
// but the default still round-trips cleanly with no payload bloat.
//
// Replaces the old TestRunDueChecks_NeverPopulatesDetails which asserted
// the stricter invariant that RunDueChecks unconditionally stripped the
// map. Issue #182 relaxed that: the scheduler now persists details so
// the /service-checks log UI can render them on expanded log rows.
func TestRunDueChecks_OmitsDetailsWhenCollectorOff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	// NB: SetCollectDetails deliberately not called — the default is
	// false so a bare NewServiceChecker stays details-free.

	checks := []internal.ServiceCheckConfig{{
		Name:    "scheduled",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}}
	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected 1 scheduled result, got %d", len(results))
	}
	if len(results[0].Details) != 0 {
		t.Fatalf("RunDueChecks with collectDetails=false must not carry Details, got: %+v", results[0].Details)
	}
}

// TestRunDueChecks_CarriesDetailsWhenCollectorOn — #182: when the parent
// checker has collectDetails enabled (as the production scheduler does
// via scheduler.go), RunDueChecks must now propagate the Details map
// out of RunCheck instead of stripping it as it did under #154.
func TestRunDueChecks_CarriesDetailsWhenCollectorOn(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	sc.SetCollectDetails(true) // mirrors scheduler.go's production wiring

	checks := []internal.ServiceCheckConfig{{
		Name:    "scheduled-with-details",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}}
	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected 1 scheduled result, got %d", len(results))
	}
	if len(results[0].Details) == 0 {
		t.Fatalf("RunDueChecks with collectDetails=true must propagate Details, got empty map")
	}
	if code, _ := results[0].Details["status_code"].(int); code != 200 {
		t.Fatalf("expected status_code=200 in scheduled Details, got %v", results[0].Details["status_code"])
	}
}

// ── Phase 2: HTTP details ──────────────────────────────────────────────

// TestRunCheck_Details_HTTP_Success_StatusCode — a successful HTTP test
// records status_code, content_type, body_bytes, and final_url so users
// can see what the server actually returned.
func TestRunCheck_Details_HTTP_Success_StatusCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:    "http-details",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected up, got %q (error=%q)", result.Status, result.Error)
	}
	code, ok := result.Details["status_code"].(int)
	if !ok || code != 200 {
		t.Fatalf("expected status_code=200 (int), got %v (%T)", result.Details["status_code"], result.Details["status_code"])
	}
	if ct, _ := result.Details["content_type"].(string); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected content_type containing application/json, got %v", result.Details["content_type"])
	}
	if size, ok := result.Details["body_bytes"].(int64); !ok || size < 1 {
		t.Fatalf("expected body_bytes > 0 int64, got %v (%T)", result.Details["body_bytes"], result.Details["body_bytes"])
	}
	if final, _ := result.Details["final_url"].(string); final == "" {
		t.Fatalf("expected final_url populated, got %v", result.Details["final_url"])
	}
}

// TestRunCheck_Details_HTTP_404_StillRecordsCode — failure path: a 404
// reports status_code=404 in Details (not just the generic "unexpected
// HTTP status" error). This is the core diagnostic value the issue calls
// out — distinguishing a 404 from a connection refused.
func TestRunCheck_Details_HTTP_404_StillRecordsCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:    "http-404",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}, time.Now().UTC())

	if result.Status != "down" {
		t.Fatalf("expected down, got %q", result.Status)
	}
	code, ok := result.Details["status_code"].(int)
	if !ok || code != 404 {
		t.Fatalf("expected status_code=404 on down, got %v", result.Details["status_code"])
	}
}

// TestRunCheck_Details_HTTP_Unreachable_NoStatusCode — if we never got a
// response (connection refused / TCP error), there is no status code to
// record. The Details map may be empty or only contain failure_stage.
// We assert status_code is NOT present so the UI doesn't lie about the
// server's reply.
func TestRunCheck_Details_HTTP_Unreachable_NoStatusCode(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "http-unreach",
		Type:       internal.ServiceCheckHTTP,
		Target:     "http://192.0.2.1:1",
		Enabled:    true,
		TimeoutSec: 1,
	}, time.Now().UTC())

	if result.Status != "down" {
		t.Fatalf("expected down, got %q", result.Status)
	}
	if _, present := result.Details["status_code"]; present {
		t.Fatalf("unreachable should have no status_code, got %v", result.Details["status_code"])
	}
	// failure_stage lets the UI say "connection refused" vs "HTTP 4xx" vs "TLS failed".
	stage, _ := result.Details["failure_stage"].(string)
	if stage == "" {
		t.Fatalf("expected failure_stage to be populated for unreachable target, got %+v", result.Details)
	}
}

// ── Phase 2: TCP details ───────────────────────────────────────────────

// TestRunCheck_Details_TCP_ResolvedAddress — on success, the TCP runner
// records the resolved_address so users see which IP:port they actually
// connected to (e.g. "1.2.3.4:445" when target was "nas.local").
func TestRunCheck_Details_TCP_ResolvedAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:    "tcp-details",
		Type:    internal.ServiceCheckTCP,
		Target:  "127.0.0.1:" + port,
		Enabled: true,
	}, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected up, got %q (error=%q)", result.Status, result.Error)
	}
	addr, _ := result.Details["resolved_address"].(string)
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("expected resolved_address prefix 127.0.0.1:, got %v", addr)
	}
}

// TestRunCheck_Details_TCP_Closed_RecordsAddress — on down, the address
// we tried is still recorded (so the user can see which host:port the
// failure references — useful when testing an SMB check with a hostname).
func TestRunCheck_Details_TCP_Closed_RecordsAddress(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "tcp-closed-details",
		Type:       internal.ServiceCheckTCP,
		Target:     "127.0.0.1:1",
		Enabled:    true,
		TimeoutSec: 1,
	}, time.Now().UTC())

	if result.Status != "down" {
		t.Fatalf("expected down, got %q", result.Status)
	}
	if addr, _ := result.Details["resolved_address"].(string); addr != "127.0.0.1:1" {
		t.Fatalf("expected resolved_address=127.0.0.1:1, got %v", result.Details["resolved_address"])
	}
}

// ── Phase 2: DNS details ───────────────────────────────────────────────

// TestRunCheck_Details_DNS_Records — a successful DNS lookup records the
// resolved A/AAAA records so users see the actual addresses their
// resolver returned (the #1 debugging need per the issue).
func TestRunCheck_Details_DNS_Records(t *testing.T) {
	addr, stop := startFakeDNS(t)
	defer stop()

	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "dns-details",
		Type:       internal.ServiceCheckDNS,
		Target:     "example.test.",
		DNSServer:  addr,
		TimeoutSec: 3,
		Enabled:    true,
	}, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected up, got %q (error=%q)", result.Status, result.Error)
	}
	records, ok := result.Details["records"].([]string)
	if !ok {
		t.Fatalf("expected records []string, got %T: %v", result.Details["records"], result.Details["records"])
	}
	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}
	if records[0] != "127.0.0.1" {
		t.Fatalf("expected first record 127.0.0.1, got %v", records[0])
	}
	if server, _ := result.Details["dns_server"].(string); server == "" {
		t.Fatalf("expected dns_server recorded, got %v", result.Details["dns_server"])
	}
}

// TestRunCheck_Details_DNS_NXDOMAIN_RecordsQueryHost — on failure, the
// host we queried is still recorded so users see what was looked up.
func TestRunCheck_Details_DNS_NXDOMAIN_RecordsQueryHost(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "dns-fail",
		Type:       internal.ServiceCheckDNS,
		Target:     "this.domain.definitely.does.not.exist.invalid.",
		Enabled:    true,
		TimeoutSec: 3,
	}, time.Now().UTC())

	if result.Status != "down" {
		t.Fatalf("expected down, got %q", result.Status)
	}
	host, _ := result.Details["query_host"].(string)
	if host == "" {
		t.Fatalf("expected query_host populated on failure, got %+v", result.Details)
	}
}

// ── Phase 2: Ping details ──────────────────────────────────────────────

// TestRunCheck_Details_Ping_RecordsRTT — a successful ping records the
// parsed rtt_ms (float) and the resolved host so users see what was hit.
// Uses the loopback so we don't flake on CI.
func TestRunCheck_Details_Ping_RecordsRTT(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "ping-details",
		Type:       internal.ServiceCheckPing,
		Target:     "127.0.0.1",
		Enabled:    true,
		TimeoutSec: 2,
	}, time.Now().UTC())

	// ping might not be available in all CI sandboxes — if it fails,
	// we still want the query_host recorded so the UI can explain.
	if result.Status == "down" {
		t.Skipf("ping not available in this environment: %q", result.Error)
	}
	if _, ok := result.Details["rtt_ms"].(float64); !ok {
		t.Fatalf("expected rtt_ms float64, got %v (%T)", result.Details["rtt_ms"], result.Details["rtt_ms"])
	}
	if host, _ := result.Details["query_host"].(string); host != "127.0.0.1" {
		t.Fatalf("expected query_host=127.0.0.1, got %v", result.Details["query_host"])
	}
}
