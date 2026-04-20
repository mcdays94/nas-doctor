package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestNormalizeServiceCheckConfig_DNS_IPTarget_Rejected — issue #159:
// a DNS check whose target is a literal IP is a silent no-op because Go's
// resolver short-circuits without sending a packet. Reject at save time.
func TestNormalizeServiceCheckConfig_DNS_IPTarget_Rejected(t *testing.T) {
	cases := []string{"1.1.1.1", "8.8.8.8", "192.168.1.1", "::1", "2606:4700:4700::1111"}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			check := &internal.ServiceCheckConfig{
				Name:   "ip-dns",
				Type:   internal.ServiceCheckDNS,
				Target: target,
			}
			err := normalizeServiceCheckConfig(check)
			if err == nil {
				t.Fatalf("expected error for DNS target %q, got nil", target)
			}
			if !strings.Contains(strings.ToLower(err.Error()), "hostname") {
				t.Errorf("error should mention hostname guidance, got: %q", err.Error())
			}
		})
	}
}

// TestNormalizeServiceCheckConfig_DNS_HostnameTarget_Allowed — a hostname
// target is fine and normalisation succeeds.
func TestNormalizeServiceCheckConfig_DNS_HostnameTarget_Allowed(t *testing.T) {
	check := &internal.ServiceCheckConfig{
		Name:   "hostname-dns",
		Type:   internal.ServiceCheckDNS,
		Target: "google.com",
	}
	if err := normalizeServiceCheckConfig(check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestNormalizeServiceCheckConfig_DNSServer_ValidHostPort — a valid DNS
// server (bare IP or host:port) passes validation.
func TestNormalizeServiceCheckConfig_DNSServer_ValidHostPort(t *testing.T) {
	cases := []string{
		"1.1.1.1",
		"8.8.8.8:53",
		"192.168.1.1:1053",
		"dns.example.com",
		"dns.example.com:5353",
		"  1.1.1.1  ", // whitespace gets trimmed
	}
	for _, server := range cases {
		t.Run(server, func(t *testing.T) {
			check := &internal.ServiceCheckConfig{
				Name:      "custom-dns",
				Type:      internal.ServiceCheckDNS,
				Target:    "google.com",
				DNSServer: server,
			}
			if err := normalizeServiceCheckConfig(check); err != nil {
				t.Fatalf("unexpected error for server %q: %v", server, err)
			}
			if strings.TrimSpace(server) != check.DNSServer {
				// Acceptable: whitespace-trimmed variant should match.
				if strings.TrimSpace(server) != strings.TrimSpace(check.DNSServer) {
					t.Errorf("expected DNSServer to be trimmed; got %q", check.DNSServer)
				}
			}
		})
	}
}

// TestNormalizeServiceCheckConfig_DNSServer_InvalidPort — a port outside
// 1..65535 is rejected.
func TestNormalizeServiceCheckConfig_DNSServer_InvalidPort(t *testing.T) {
	cases := []string{
		"1.1.1.1:0",
		"1.1.1.1:99999",
		"1.1.1.1:-1",
		"1.1.1.1:abc",
	}
	for _, server := range cases {
		t.Run(server, func(t *testing.T) {
			check := &internal.ServiceCheckConfig{
				Name:      "bad-port",
				Type:      internal.ServiceCheckDNS,
				Target:    "google.com",
				DNSServer: server,
			}
			if err := normalizeServiceCheckConfig(check); err == nil {
				t.Fatalf("expected error for DNSServer %q, got nil", server)
			}
		})
	}
}

// TestNormalizeServiceCheckConfig_DNSServer_OnNonDNSTypeErrors — dns_server
// is only valid for DNS-type checks; setting it on HTTP/TCP/etc. is
// rejected with an actionable error rather than silently dropped.
func TestNormalizeServiceCheckConfig_DNSServer_OnNonDNSTypeErrors(t *testing.T) {
	for _, typ := range []string{
		internal.ServiceCheckHTTP,
		internal.ServiceCheckTCP,
		internal.ServiceCheckSMB,
		internal.ServiceCheckNFS,
		internal.ServiceCheckPing,
	} {
		t.Run(typ, func(t *testing.T) {
			target := "example.com"
			if typ == internal.ServiceCheckHTTP {
				target = "http://example.com"
			}
			if typ == internal.ServiceCheckTCP || typ == internal.ServiceCheckSMB || typ == internal.ServiceCheckNFS {
				target = "example.com:1234"
			}
			check := &internal.ServiceCheckConfig{
				Name:      "wrong-type",
				Type:      typ,
				Target:    target,
				DNSServer: "1.1.1.1",
			}
			if err := normalizeServiceCheckConfig(check); err == nil {
				t.Fatalf("expected dns_server to be rejected on %s check, got nil", typ)
			}
		})
	}
}

// TestNormalizeServiceCheckConfig_DNSServer_EmptyIsFine — empty DNSServer
// is the default and must not error (backwards-compat with existing
// saved checks).
func TestNormalizeServiceCheckConfig_DNSServer_EmptyIsFine(t *testing.T) {
	check := &internal.ServiceCheckConfig{
		Name:      "default-resolver",
		Type:      internal.ServiceCheckDNS,
		Target:    "google.com",
		DNSServer: "",
	}
	if err := normalizeServiceCheckConfig(check); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleTestServiceCheck_DNS_IPTarget_Rejected — the Test button
// endpoint should reject IP DNS targets with 400 rather than silently
// reporting up in 0ms. Closes the loop on bug #159 from the UI side.
func TestHandleTestServiceCheck_DNS_IPTarget_Rejected(t *testing.T) {
	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "ip-dns",
		"type":   "dns",
		"target": "1.1.1.1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for DNS check of IP target, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "hostname") {
		t.Fatalf("expected hostname guidance in error, got: %s", rec.Body.String())
	}
}
