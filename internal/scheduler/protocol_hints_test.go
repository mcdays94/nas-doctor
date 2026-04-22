// Package scheduler — protocol_hints_test.go covers ProtocolHint, the
// well-known-port → label map surfaced on TCP service check Details so
// the dashboard can render a small protocol badge (e.g. "SSH" for :22,
// "HTTPS" for :443) in the expanded log entry and the Test button
// toast. Purely informational; does not affect check execution.
//
// See issue #188.
package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestProtocolHint_KnownPorts walks the published port → badge table
// from the issue body. Any drift between the table here and the Go
// implementation means the docs lie. Kept verbose on purpose: one
// case per row so a failure points at the exact offending port.
func TestProtocolHint_KnownPorts(t *testing.T) {
	cases := []struct {
		port int
		want string
	}{
		{22, "SSH"},
		{25, "SMTP"},
		{53, "DNS"},
		{80, "HTTP"},
		{110, "POP3"},
		{143, "IMAP"},
		{389, "LDAP"},
		{443, "HTTPS"},
		{445, "SMB"},
		{465, "SMTPS"},
		{587, "SMTP (submission)"},
		{636, "LDAPS"},
		{993, "IMAPS"},
		{995, "POP3S"},
		{1433, "MSSQL"},
		{3306, "MySQL"},
		{3389, "RDP"},
		{5432, "PostgreSQL"},
		{5672, "AMQP"},
		{6379, "Redis"},
		{8080, "HTTP (alt)"},
		{8443, "HTTPS (alt)"},
		{9200, "Elasticsearch"},
		{27017, "MongoDB"},
	}
	for _, c := range cases {
		if got := ProtocolHint(c.port); got != c.want {
			t.Errorf("ProtocolHint(%d) = %q, want %q", c.port, got, c.want)
		}
	}
}

// TestProtocolHint_UnknownPorts — any port not in the published table
// must return "" so the UI can skip the badge entirely. We exercise a
// spread (privileged, ephemeral, nonsense) to keep the implementation
// honest: no "default to HTTP" shortcuts.
func TestProtocolHint_UnknownPorts(t *testing.T) {
	for _, port := range []int{0, -1, 1, 23, 21, 999, 8000, 8081, 9999, 65535, 65536} {
		if got := ProtocolHint(port); got != "" {
			t.Errorf("ProtocolHint(%d) = %q, want empty string", port, got)
		}
	}
}

// TestRunCheck_TCP_PopulatesProtocolHint — when a TCP check resolves
// to a well-known port, the Details map carries protocol_hint so the
// dashboard and Test toast can render a badge. Uses a real listener
// on 127.0.0.1:<ephemeral> for the happy path, and a hardcoded
// target ending in :22 for the hint-set path (connect will fail —
// the hint is computed from the TARGET port, not the connection
// result, so failure_stage coexists with protocol_hint).
func TestRunCheck_TCP_PopulatesProtocolHint(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)

	// Target port 22 is well-known (SSH). We point at 127.0.0.1:22
	// rather than a real SSH server — the dial will almost certainly
	// fail on CI, which is fine: the hint is derived from the
	// resolved_address, not from the connection outcome.
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "tcp-ssh-hint",
		Type:       internal.ServiceCheckTCP,
		Target:     "127.0.0.1:22",
		Enabled:    true,
		TimeoutSec: 1,
	}, time.Now().UTC())

	hint, ok := result.Details["protocol_hint"].(string)
	if !ok {
		t.Fatalf("expected protocol_hint to be a string in Details, got %T (%v); full Details=%+v",
			result.Details["protocol_hint"], result.Details["protocol_hint"], result.Details)
	}
	if hint != "SSH" {
		t.Fatalf("expected protocol_hint=SSH for port 22, got %q", hint)
	}
}

// TestRunCheck_TCP_UnknownPort_NoProtocolHint — the hint key must be
// ABSENT (not empty-string) when the port is not in the well-known
// table. The UI renderer uses key-presence to decide whether to draw
// the badge, so an empty-string value would render an empty badge.
func TestRunCheck_TCP_UnknownPort_NoProtocolHint(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)

	// Port 9999 is not in the table; pick a target that will also
	// not connect so we don't need to stand up a listener.
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "tcp-no-hint",
		Type:       internal.ServiceCheckTCP,
		Target:     "127.0.0.1:9999",
		Enabled:    true,
		TimeoutSec: 1,
	}, time.Now().UTC())

	if v, present := result.Details["protocol_hint"]; present {
		t.Fatalf("expected protocol_hint to be absent for port 9999, got %v", v)
	}
}

// TestRunCheck_TCP_SMB_ImpliedPortHint — SMB checks default to port
// 445 when none is specified. The hint should reflect the defaulted
// port (SMB) so users get a badge even for port-less SMB targets.
func TestRunCheck_TCP_SMB_ImpliedPortHint(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetCollectDetails(true)

	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "smb-implied-445",
		Type:       internal.ServiceCheckSMB,
		Target:     "127.0.0.1", // no explicit port → runner defaults to 445
		Enabled:    true,
		TimeoutSec: 1,
	}, time.Now().UTC())

	hint, ok := result.Details["protocol_hint"].(string)
	if !ok || hint != "SMB" {
		t.Fatalf("expected protocol_hint=SMB for defaulted SMB port 445, got %v", result.Details["protocol_hint"])
	}
}
