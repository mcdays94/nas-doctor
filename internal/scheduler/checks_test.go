package scheduler

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newTestChecker returns a ServiceChecker backed by a FakeStore.
func newTestChecker() (*ServiceChecker, *storage.FakeStore) {
	store := storage.NewFakeStore()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	sc := NewServiceChecker(store, logger)
	return sc, store
}

// ── HTTP check tests ───────────────────────────────────────────────────

func TestRunCheck_HTTP_Up(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:    "web",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up, got %s (error=%q)", result.Status, result.Error)
	}
	if result.Error != "" {
		t.Fatalf("expected no error, got %q", result.Error)
	}
	if result.ResponseMS < 0 {
		t.Fatalf("expected non-negative response time, got %d", result.ResponseMS)
	}
	if result.Key == "" {
		t.Fatal("expected non-empty check key")
	}
}

func TestRunCheck_HTTP_500_Down(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:    "api",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down, got %s", result.Status)
	}
	if !strings.Contains(result.Error, "unexpected HTTP status") {
		t.Fatalf("expected 'unexpected HTTP status' error, got %q", result.Error)
	}
}

func TestRunCheck_HTTP_Unreachable(t *testing.T) {
	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:       "unreachable",
		Type:       internal.ServiceCheckHTTP,
		Target:     "http://192.0.2.1:1", // RFC 5737 TEST-NET, guaranteed unreachable
		Enabled:    true,
		TimeoutSec: 1,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down, got %s", result.Status)
	}
	if result.Error == "" {
		t.Fatal("expected an error message for unreachable target")
	}
}

func TestRunCheck_HTTP_CustomStatusRange(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent) // 204
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:        "custom-range",
		Type:        internal.ServiceCheckHTTP,
		Target:      ts.URL,
		Enabled:     true,
		ExpectedMin: 200,
		ExpectedMax: 204,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up for 204 in range 200-204, got %s (error=%q)", result.Status, result.Error)
	}
}

func TestRunCheck_HTTP_CustomHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "test-value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:    "with-headers",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
		Headers: map[string]string{"X-Custom": "test-value"},
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up with custom header, got %s (error=%q)", result.Status, result.Error)
	}
}

// ── TCP check tests ────────────────────────────────────────────────────

func TestRunCheck_TCP_Up(t *testing.T) {
	// Start a TCP listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:    "tcp-svc",
		Type:    internal.ServiceCheckTCP,
		Target:  "127.0.0.1:" + port,
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up, got %s (error=%q)", result.Status, result.Error)
	}
}

func TestRunCheck_TCP_Down(t *testing.T) {
	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:       "tcp-closed",
		Type:       internal.ServiceCheckTCP,
		Target:     "127.0.0.1:1", // port 1 should be closed on localhost
		Enabled:    true,
		TimeoutSec: 1,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down, got %s", result.Status)
	}
	if result.Error == "" {
		t.Fatal("expected an error for closed port")
	}
}

// ── DNS check tests ────────────────────────────────────────────────────

func TestRunCheck_DNS_Up(t *testing.T) {
	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:    "dns-google",
		Type:    internal.ServiceCheckDNS,
		Target:  "google.com",
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up for google.com DNS, got %s (error=%q)", result.Status, result.Error)
	}
}

func TestRunCheck_DNS_Fail(t *testing.T) {
	sc, _ := newTestChecker()
	check := internal.ServiceCheckConfig{
		Name:       "dns-nonexistent",
		Type:       internal.ServiceCheckDNS,
		Target:     "this.domain.definitely.does.not.exist.invalid.",
		Enabled:    true,
		TimeoutSec: 3,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down for non-existent domain, got %s", result.Status)
	}
	if result.Error == "" {
		t.Fatal("expected an error for failed DNS resolution")
	}
}

// ── SMB address normalization ──────────────────────────────────────────

func TestNormalizeTCPAddress_SMB_DefaultPort(t *testing.T) {
	addr, err := NormalizeTCPAddress(internal.ServiceCheckConfig{
		Type:   internal.ServiceCheckSMB,
		Target: "nas.local",
	})
	if err != nil {
		t.Fatalf("NormalizeTCPAddress failed: %v", err)
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

func TestNormalizeTCPAddress_NFS_DefaultPort(t *testing.T) {
	addr, err := NormalizeTCPAddress(internal.ServiceCheckConfig{
		Type:   internal.ServiceCheckNFS,
		Target: "fileserver.local",
	})
	if err != nil {
		t.Fatalf("NormalizeTCPAddress failed: %v", err)
	}
	_, port, _ := net.SplitHostPort(addr)
	if port != "2049" {
		t.Fatalf("expected NFS default port 2049, got %s", port)
	}
}

func TestNormalizeTCPAddress_WithURL(t *testing.T) {
	addr, err := NormalizeTCPAddress(internal.ServiceCheckConfig{
		Type:   internal.ServiceCheckSMB,
		Target: "smb://nas.local",
	})
	if err != nil {
		t.Fatalf("NormalizeTCPAddress failed: %v", err)
	}
	host, port, _ := net.SplitHostPort(addr)
	if host != "nas.local" {
		t.Fatalf("expected host nas.local, got %s", host)
	}
	if port != "445" {
		t.Fatalf("expected SMB default port 445, got %s", port)
	}
}

// ── Consecutive failure tracking ───────────────────────────────────────

func TestConsecutiveFailures_FirstFailure(t *testing.T) {
	sc, store := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:    "fail-svc",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}}

	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ConsecutiveFailures != 1 {
		t.Fatalf("expected consecutiveFailures=1 on first failure, got %d", results[0].ConsecutiveFailures)
	}

	// Verify it was persisted.
	entries, err := store.ListLatestServiceChecks(10)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 persisted entry, got %d", len(entries))
	}
	if entries[0].ConsecutiveFailures != 1 {
		t.Fatalf("expected persisted consecutiveFailures=1, got %d", entries[0].ConsecutiveFailures)
	}
}

func TestConsecutiveFailures_SecondFailure(t *testing.T) {
	sc, _ := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:        "fail2-svc",
		Type:        internal.ServiceCheckHTTP,
		Target:      ts.URL,
		Enabled:     true,
		IntervalSec: 1, // allow immediate re-run
	}}

	now := time.Now().UTC()

	// First failure.
	results1 := sc.RunDueChecks(checks, now)
	if len(results1) != 1 || results1[0].ConsecutiveFailures != 1 {
		t.Fatalf("first run: expected consecutiveFailures=1, got %d", results1[0].ConsecutiveFailures)
	}

	// Second failure — advance time past the interval.
	results2 := sc.RunDueChecks(checks, now.Add(2*time.Second))
	if len(results2) != 1 {
		t.Fatalf("second run: expected 1 result, got %d", len(results2))
	}
	if results2[0].ConsecutiveFailures != 2 {
		t.Fatalf("second run: expected consecutiveFailures=2, got %d", results2[0].ConsecutiveFailures)
	}
}

func TestConsecutiveFailures_Recovery(t *testing.T) {
	callCount := 0
	sc, _ := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:        "recover-svc",
		Type:        internal.ServiceCheckHTTP,
		Target:      ts.URL,
		Enabled:     true,
		IntervalSec: 1,
	}}

	now := time.Now().UTC()

	// Two failures.
	sc.RunDueChecks(checks, now)
	results := sc.RunDueChecks(checks, now.Add(2*time.Second))
	if results[0].ConsecutiveFailures != 2 {
		t.Fatalf("expected consecutiveFailures=2 before recovery, got %d", results[0].ConsecutiveFailures)
	}

	// Recovery.
	results = sc.RunDueChecks(checks, now.Add(4*time.Second))
	if len(results) != 1 {
		t.Fatalf("expected 1 result after recovery, got %d", len(results))
	}
	if results[0].Status != "up" {
		t.Fatalf("expected status up after recovery, got %s", results[0].Status)
	}
	if results[0].ConsecutiveFailures != 0 {
		t.Fatalf("expected consecutiveFailures=0 after recovery, got %d", results[0].ConsecutiveFailures)
	}
}

// ── Interval management tests ──────────────────────────────────────────

func TestRunDueChecks_NotDue_Skipped(t *testing.T) {
	sc, _ := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:        "interval-svc",
		Type:        internal.ServiceCheckHTTP,
		Target:      ts.URL,
		Enabled:     true,
		IntervalSec: 300, // 5 minutes
	}}

	now := time.Now().UTC()

	// First run — should execute (never run before).
	results := sc.RunDueChecks(checks, now)
	if len(results) != 1 {
		t.Fatalf("first run: expected 1 result, got %d", len(results))
	}

	// Second run 10 seconds later — should be skipped (interval=300s).
	results = sc.RunDueChecks(checks, now.Add(10*time.Second))
	if len(results) != 0 {
		t.Fatalf("expected 0 results (check not due), got %d", len(results))
	}
}

func TestRunDueChecks_Overdue_Executed(t *testing.T) {
	sc, _ := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:        "overdue-svc",
		Type:        internal.ServiceCheckHTTP,
		Target:      ts.URL,
		Enabled:     true,
		IntervalSec: 60, // 1 minute
	}}

	now := time.Now().UTC()

	// First run.
	sc.RunDueChecks(checks, now)

	// 90 seconds later — overdue, should execute.
	results := sc.RunDueChecks(checks, now.Add(90*time.Second))
	if len(results) != 1 {
		t.Fatalf("expected 1 result (check overdue), got %d", len(results))
	}
	if results[0].Status != "up" {
		t.Fatalf("expected status up, got %s", results[0].Status)
	}
}

func TestRunDueChecks_DisabledCheck_Skipped(t *testing.T) {
	sc, _ := newTestChecker()
	checks := []internal.ServiceCheckConfig{{
		Name:    "disabled-svc",
		Type:    internal.ServiceCheckHTTP,
		Target:  "http://example.com",
		Enabled: false,
	}}

	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 0 {
		t.Fatalf("expected 0 results for disabled check, got %d", len(results))
	}
}

func TestRunDueChecks_UnsupportedType_Skipped(t *testing.T) {
	sc, _ := newTestChecker()
	checks := []internal.ServiceCheckConfig{{
		Name:    "bad-type",
		Type:    "ftp", // not a supported type
		Target:  "ftp://example.com",
		Enabled: true,
	}}

	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unsupported check type, got %d", len(results))
	}
}

// ── Speed check tests ──────────────────────────────────────────────────

func TestRunCheck_Speed_AboveThreshold_Up(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{
			DownloadMbps: 500,
			UploadMbps:   100,
			LatencyMs:    5,
		}
	})

	check := internal.ServiceCheckConfig{
		Name:               "speed-ok",
		Type:               internal.ServiceCheckSpeed,
		Target:             "speedtest",
		Enabled:            true,
		ContractedDownMbps: 400,
		ContractedUpMbps:   80,
		MarginPct:          10,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up, got %s (error=%q)", result.Status, result.Error)
	}
	if result.DownloadMbps != 500 {
		t.Fatalf("expected download 500, got %.0f", result.DownloadMbps)
	}
	if result.UploadMbps != 100 {
		t.Fatalf("expected upload 100, got %.0f", result.UploadMbps)
	}
	if result.DownloadOK == nil || !*result.DownloadOK {
		t.Fatal("expected downloadOK=true")
	}
	if result.UploadOK == nil || !*result.UploadOK {
		t.Fatal("expected uploadOK=true")
	}
}

func TestRunCheck_Speed_BelowThreshold_Degraded(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{
			DownloadMbps: 500,
			UploadMbps:   30, // below 80 * 0.9 = 72
			LatencyMs:    5,
		}
	})

	check := internal.ServiceCheckConfig{
		Name:               "speed-degraded",
		Type:               internal.ServiceCheckSpeed,
		Target:             "speedtest",
		Enabled:            true,
		ContractedDownMbps: 400,
		ContractedUpMbps:   80,
		MarginPct:          10,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "degraded" {
		t.Fatalf("expected status degraded, got %s (error=%q)", result.Status, result.Error)
	}
	if !strings.Contains(result.Error, "upload below contracted speed") {
		t.Fatalf("expected upload-related error, got %q", result.Error)
	}
}

func TestRunCheck_Speed_BothBelow_Down(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{
			DownloadMbps: 50, // below 400 * 0.9 = 360
			UploadMbps:   10, // below 80 * 0.9 = 72
			LatencyMs:    100,
		}
	})

	check := internal.ServiceCheckConfig{
		Name:               "speed-down",
		Type:               internal.ServiceCheckSpeed,
		Target:             "speedtest",
		Enabled:            true,
		ContractedDownMbps: 400,
		ContractedUpMbps:   80,
		MarginPct:          10,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	// When both are below threshold, the status is "down" (not "degraded" — no side passes).
	// Actually from the code: default case when neither dlOK nor ulOK → no explicit "down" set,
	// it stays as the initial "down".
	if result.Status != "down" {
		t.Fatalf("expected status down, got %s (error=%q)", result.Status, result.Error)
	}
	if !strings.Contains(result.Error, "both download and upload below") {
		t.Fatalf("expected 'both below' error, got %q", result.Error)
	}
}

func TestRunCheck_Speed_NoRunner(t *testing.T) {
	sc, _ := newTestChecker()
	// No speed test runner set (default nil).

	check := internal.ServiceCheckConfig{
		Name:    "speed-no-tool",
		Type:    internal.ServiceCheckSpeed,
		Target:  "speedtest",
		Enabled: true,
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status != "down" {
		t.Fatalf("expected status down when no speed test runner, got %s", result.Status)
	}
	if !strings.Contains(result.Error, "no speedtest tool available") {
		t.Fatalf("expected 'no speedtest tool available' error, got %q", result.Error)
	}
}

// TestRunCheck_Speed_ZeroThroughput_ReportsDown verifies that a runner
// returning an all-zero SpeedTestResult is NOT reported as "up (1 ms)".
// Regression for #170: when contracted speeds aren't set, the threshold
// logic (`ContractedDownMbps <= 0 || ...`) unconditionally passed any
// non-nil result, including a zero-valued struct, producing a misleading
// UP status with response_ms=1 (from the sub-ms floor in RunCheck).
func TestRunCheck_Speed_ZeroThroughput_ReportsDown(t *testing.T) {
	sc, _ := newTestChecker()
	sc.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		return &internal.SpeedTestResult{
			DownloadMbps: 0,
			UploadMbps:   0,
			LatencyMs:    0,
		}
	})

	check := internal.ServiceCheckConfig{
		Name:    "speed-zero",
		Type:    internal.ServiceCheckSpeed,
		Target:  "speedtest",
		Enabled: true,
		// Contracted speeds deliberately NOT set — this is the default
		// configuration that triggered the original bug.
	}

	result := sc.RunCheck(check, time.Now().UTC())
	if result.Status == "up" {
		t.Fatalf("expected non-up status for zero-throughput result, got up (error=%q, response_ms=%d)",
			result.Error, result.ResponseMS)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error explaining zero-throughput result")
	}
	if !strings.Contains(strings.ToLower(result.Error), "no measurements") &&
		!strings.Contains(strings.ToLower(result.Error), "speedtest") {
		t.Fatalf("expected error to mention missing measurements or speedtest, got %q", result.Error)
	}
}

// ── Helper function tests ──────────────────────────────────────────────

func TestIsSupportedCheckType(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"http", true},
		{"HTTP", true},
		{" tcp ", true},
		{"dns", true},
		{"smb", true},
		{"nfs", true},
		{"ping", true},
		{"speed", true},
		{"ftp", false},
		{"", false},
		{"grpc", false},
	}
	for _, tc := range cases {
		if got := IsSupportedCheckType(tc.input); got != tc.want {
			t.Errorf("IsSupportedCheckType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCheckKey_Deterministic(t *testing.T) {
	check := internal.ServiceCheckConfig{
		Name:   "web",
		Type:   "http",
		Target: "https://example.com",
		Port:   443,
	}
	key1 := CheckKey(check)
	key2 := CheckKey(check)
	if key1 != key2 {
		t.Fatalf("expected deterministic key, got %s vs %s", key1, key2)
	}
	if len(key1) != 64 { // SHA-256 hex
		t.Fatalf("expected 64-char hex key, got len=%d", len(key1))
	}
}

func TestCheckKey_Consistency(t *testing.T) {
	// Verify that CheckKey is deterministic and produces the expected
	// SHA-256 hash format for a given input.
	check := internal.ServiceCheckConfig{
		Name:   "My Service",
		Type:   "HTTP",
		Target: "https://example.com",
		Port:   443,
	}
	key1 := CheckKey(check)
	key2 := CheckKey(check)
	if key1 != key2 {
		t.Fatalf("CheckKey is not deterministic:\n  first:  %s\n  second: %s", key1, key2)
	}
	if len(key1) != 64 {
		t.Fatalf("expected 64-char hex key, got len=%d", len(key1))
	}
}

func TestNormalizeHTTPURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://example.com", "https://example.com"},
		{"http://example.com", "http://example.com"},
		{"example.com", "http://example.com"},
		{"", ""},
		{"  example.com  ", "http://example.com"},
	}
	for _, tc := range cases {
		if got := NormalizeHTTPURL(tc.input); got != tc.want {
			t.Errorf("NormalizeHTTPURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeDNSHost(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com"},
		{"http://example.com/path", "example.com"},
		{"https://example.com:8443", "example.com"},
		{"example.com:80", "example.com"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeDNSHost(tc.input); got != tc.want {
			t.Errorf("NormalizeDNSHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── Persistence integration test ───────────────────────────────────────

func TestRunDueChecks_PersistsResults(t *testing.T) {
	sc, store := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:    "persist-svc",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}}

	sc.RunDueChecks(checks, time.Now().UTC())

	// Verify store has the result.
	entries, err := store.ListLatestServiceChecks(10)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 persisted entry, got %d", len(entries))
	}
	if entries[0].Status != "up" {
		t.Fatalf("expected persisted status up, got %s", entries[0].Status)
	}
}

func TestRunDueChecks_DefaultInterval(t *testing.T) {
	// When IntervalSec is 0, defaults to 300s.
	sc, _ := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checks := []internal.ServiceCheckConfig{{
		Name:        "default-interval",
		Type:        internal.ServiceCheckHTTP,
		Target:      ts.URL,
		Enabled:     true,
		IntervalSec: 0, // should default to 300
	}}

	now := time.Now().UTC()
	// First run — executes.
	results := sc.RunDueChecks(checks, now)
	if len(results) != 1 {
		t.Fatalf("first run: expected 1 result, got %d", len(results))
	}

	// 60 seconds later — should be skipped (default interval = 300s).
	results = sc.RunDueChecks(checks, now.Add(60*time.Second))
	if len(results) != 0 {
		t.Fatalf("expected 0 results (not due, default 300s interval), got %d", len(results))
	}

	// 301 seconds later — should execute.
	results = sc.RunDueChecks(checks, now.Add(301*time.Second))
	if len(results) != 1 {
		t.Fatalf("expected 1 result (past default 300s interval), got %d", len(results))
	}
}

// ── Multiple checks in one run ─────────────────────────────────────────

func TestRunDueChecks_MultipleChecks(t *testing.T) {
	sc, store := newTestChecker()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	checks := []internal.ServiceCheckConfig{
		{
			Name:    "http-svc",
			Type:    internal.ServiceCheckHTTP,
			Target:  ts.URL,
			Enabled: true,
		},
		{
			Name:    "tcp-svc",
			Type:    internal.ServiceCheckTCP,
			Target:  "127.0.0.1:" + port,
			Enabled: true,
		},
	}

	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != "up" {
			t.Errorf("check %s: expected up, got %s (error=%q)", r.Name, r.Status, r.Error)
		}
	}

	entries, err := store.ListLatestServiceChecks(10)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 persisted entries, got %d", len(entries))
	}
}

// ── DNS check — #159 bug fixes ─────────────────────────────────────────

// TestRunDNSCheck_IPTarget_IsRejected — bug #159: a DNS check whose target
// parses as a literal IP must be rejected at check-time, because Go's
// resolver short-circuits on IP literals and never sends a packet,
// causing the check to always appear "up" in 0ms regardless of whether
// any resolver is reachable.
func TestRunDNSCheck_IPTarget_IsRejected(t *testing.T) {
	sc, _ := newTestChecker()
	cases := []string{"1.1.1.1", "8.8.8.8", "192.168.1.1", "::1", "2606:4700:4700::1111"}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			cfg := internal.ServiceCheckConfig{
				Name:    "ip-dns",
				Type:    internal.ServiceCheckDNS,
				Target:  target,
				Enabled: true,
			}
			result := sc.RunCheck(cfg, time.Now().UTC())
			if result.Status == "up" {
				t.Errorf("expected DNS check of IP %q to not report up, got status=%q", target, result.Status)
			}
			if !strings.Contains(strings.ToLower(result.Error), "hostname") {
				t.Errorf("expected error message to mention hostname guidance, got: %q", result.Error)
			}
		})
	}
}

// TestRunCheck_SuccessfulSubMs_FlooredAt1ms — bug #159: a check that
// completes in under a millisecond should report 1ms, not 0ms. LAN-local
// DNS resolvers and loopback HTTP genuinely run faster than 1ms; raw
// int64 millisecond truncation displays "0ms" which users interpret as
// "didn't run". Floor at 1ms so the UI shows a sane value.
func TestRunCheck_SuccessfulSubMs_FlooredAt1ms(t *testing.T) {
	// A loopback HTTP server responds in well under 1ms on most hosts.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sc, _ := newTestChecker()
	// Run the check many times; at least one iteration should complete in
	// sub-millisecond time on any reasonable CI. We only need ONE success
	// with the floor applied to pass.
	var observed bool
	for i := 0; i < 20; i++ {
		result := sc.RunCheck(internal.ServiceCheckConfig{
			Name:    "loopback",
			Type:    internal.ServiceCheckHTTP,
			Target:  ts.URL,
			Enabled: true,
		}, time.Now().UTC())
		if result.Status != "up" {
			t.Fatalf("loopback HTTP check unexpectedly failed: %q", result.Error)
		}
		if result.ResponseMS < 1 {
			t.Fatalf("successful check reported ResponseMS=%d; floor should guarantee >= 1", result.ResponseMS)
		}
		if result.ResponseMS == 1 {
			observed = true
		}
	}
	// If none of the iterations hit the floor, note it but don't fail —
	// the important guarantee is the >= 1 invariant above.
	if !observed {
		t.Logf("note: none of 20 loopback iterations hit the sub-ms floor (all were >= 2ms); invariant still holds")
	}
}

// ── DNS check — #160 custom resolver ───────────────────────────────────

// startFakeDNS stands up a minimal UDP DNS server that replies to every
// A query with 127.0.0.1. Returns the host:port address it is listening on
// and a teardown function. Used to exercise the custom-resolver code path
// without depending on the host's network.
func startFakeDNS(t *testing.T) (string, func()) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 12 {
				continue
			}
			// Build a response reusing the question section verbatim.
			// We locate the end of the QNAME+QTYPE+QCLASS, then append a
			// single A record answer pointing at 127.0.0.1.
			resp := make([]byte, 0, 64)
			// Header: copy ID, set QR=1 (response), RD+RA=1, ANCOUNT=1.
			resp = append(resp, buf[0], buf[1]) // ID
			resp = append(resp, 0x81, 0x80)     // flags: response, recursion available
			resp = append(resp, 0x00, 0x01)     // QDCOUNT=1
			resp = append(resp, 0x00, 0x01)     // ANCOUNT=1
			resp = append(resp, 0x00, 0x00)     // NSCOUNT=0
			resp = append(resp, 0x00, 0x00)     // ARCOUNT=0
			// Copy the question section verbatim.
			qStart := 12
			qEnd := qStart
			for qEnd < n && buf[qEnd] != 0 {
				qEnd += int(buf[qEnd]) + 1
				if qEnd >= n {
					break
				}
			}
			qEnd++ // consume the terminating zero
			// QTYPE(2) + QCLASS(2)
			qEnd += 4
			if qEnd > n {
				qEnd = n
			}
			resp = append(resp, buf[qStart:qEnd]...)
			// Answer: pointer to the question's name (0xC00C), type A,
			// class IN, TTL 60, rdlength 4, rdata 127.0.0.1.
			resp = append(resp, 0xC0, 0x0C)
			resp = append(resp, 0x00, 0x01)             // TYPE A
			resp = append(resp, 0x00, 0x01)             // CLASS IN
			resp = append(resp, 0x00, 0x00, 0x00, 0x3C) // TTL 60
			resp = append(resp, 0x00, 0x04)             // RDLENGTH 4
			resp = append(resp, 127, 0, 0, 1)           // RDATA 127.0.0.1
			_, _ = conn.WriteTo(resp, addr)
		}
	}()
	stop := func() {
		_ = conn.Close()
		<-done
	}
	return conn.LocalAddr().String(), stop
}

// TestRunDNSCheck_CustomDNSServer_UsesIt verifies that when DNSServer is
// set, the check queries that server (not the system resolver) and
// succeeds if the server responds.
func TestRunDNSCheck_CustomDNSServer_UsesIt(t *testing.T) {
	addr, stop := startFakeDNS(t)
	defer stop()

	sc, _ := newTestChecker()
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "custom-dns",
		Type:       internal.ServiceCheckDNS,
		Target:     "example.test.", // trailing dot to avoid search-path expansion
		DNSServer:  addr,
		TimeoutSec: 3,
		Enabled:    true,
	}, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up, got %q (error=%q)", result.Status, result.Error)
	}
}

// TestRunDNSCheck_CustomDNSServer_Unreachable_Fails verifies that
// pointing at an unreachable IP fails with a timeout error rather than
// falling back to the system resolver.
func TestRunDNSCheck_CustomDNSServer_Unreachable_Fails(t *testing.T) {
	sc, _ := newTestChecker()
	// Use RFC 5737 TEST-NET-1 (192.0.2.0/24) plus a closed-looking port.
	// Note: on Linux, sending a UDP packet to an unreachable address may
	// return ICMP port unreachable quickly rather than timing out; both
	// outcomes produce an error, which is what we require here.
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "unreachable-dns",
		Type:       internal.ServiceCheckDNS,
		Target:     "example.com.",
		DNSServer:  "192.0.2.1:53",
		TimeoutSec: 2,
		Enabled:    true,
	}, time.Now().UTC())
	if result.Status == "up" {
		t.Fatalf("expected status down with unreachable DNS server, got up")
	}
	if result.Error == "" {
		t.Fatal("expected a non-empty error from unreachable DNS server")
	}
}

// TestRunDNSCheck_CustomDNSServer_PortlessDefaultsTo53 verifies that a
// bare-IP DNSServer ("1.1.1.1") is treated as "1.1.1.1:53". We don't
// actually want to hit 1.1.1.1 in CI, so we exercise the normalisation
// path by pointing at our fake server with the port stripped back on —
// i.e. we construct a fake that listens on port 53 is infeasible here,
// so we instead assert via an unreachable IP: the dial target must
// include :53 or the check would fail with an address format error
// rather than a timeout.
func TestRunDNSCheck_CustomDNSServer_PortlessDefaultsTo53(t *testing.T) {
	sc, _ := newTestChecker()
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:       "portless-dns",
		Type:       internal.ServiceCheckDNS,
		Target:     "example.com.",
		DNSServer:  "192.0.2.1", // no port — must default to :53
		TimeoutSec: 1,
		Enabled:    true,
	}, time.Now().UTC())
	// We expect a down/error outcome (unreachable), not an address-parse
	// panic or a "missing port" error. Any error that ISN'T about port
	// formatting is acceptable.
	if result.Status == "up" {
		t.Fatalf("expected status down, got up")
	}
	if strings.Contains(strings.ToLower(result.Error), "missing port") ||
		strings.Contains(strings.ToLower(result.Error), "invalid port") {
		t.Fatalf("bare-IP DNSServer should have been defaulted to :53; got port error: %q", result.Error)
	}
}

// TestRunDNSCheck_NoCustomServer_UsesDefault verifies backwards
// compatibility: when DNSServer is empty, behaviour is unchanged and a
// lookup against a hostname that resolves via /etc/hosts (localhost)
// reports up.
func TestRunDNSCheck_NoCustomServer_UsesDefault(t *testing.T) {
	sc, _ := newTestChecker()
	result := sc.RunCheck(internal.ServiceCheckConfig{
		Name:    "default-dns",
		Type:    internal.ServiceCheckDNS,
		Target:  "localhost",
		Enabled: true,
	}, time.Now().UTC())
	if result.Status != "up" {
		t.Fatalf("expected status up for localhost, got %q (error=%q)", result.Status, result.Error)
	}
}
