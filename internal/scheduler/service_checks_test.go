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

func TestExecuteServiceCheckHTTPUp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	check := internal.ServiceCheckConfig{
		Name:    "ui-health",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}

	result := executeServiceCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up, got %s (error=%q)", result.Status, result.Error)
	}
	if result.Error != "" {
		t.Fatalf("expected no error, got %q", result.Error)
	}
	if result.Key == "" {
		t.Fatal("expected non-empty check key")
	}
}

func TestExecuteServiceCheckHTTPUnexpectedStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	check := internal.ServiceCheckConfig{
		Name:    "api-health",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}

	result := executeServiceCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down, got %s", result.Status)
	}
	if !strings.Contains(result.Error, "unexpected HTTP status") {
		t.Fatalf("expected unexpected status error, got %q", result.Error)
	}
}

func TestNormalizeTCPAddressSMBDefaultPort(t *testing.T) {
	addr, err := normalizeTCPAddress(internal.ServiceCheckConfig{
		Type:   internal.ServiceCheckSMB,
		Target: "nas.local",
	})
	if err != nil {
		t.Fatalf("normalizeTCPAddress failed: %v", err)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("invalid normalized address %q: %v", addr, err)
	}
	if host != "nas.local" {
		t.Fatalf("expected host nas.local, got %s", host)
	}
	if port != "445" {
		t.Fatalf("expected SMB default port 445, got %s", port)
	}
}
