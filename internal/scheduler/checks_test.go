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

func TestCheckKey_MatchesSchedulerKey(t *testing.T) {
	// Verify that CheckKey produces the same result as the original
	// serviceCheckKey function in scheduler.go.
	check := internal.ServiceCheckConfig{
		Name:   "My Service",
		Type:   "HTTP",
		Target: "https://example.com",
		Port:   443,
	}
	newKey := CheckKey(check)
	oldKey := serviceCheckKey(check)
	if newKey != oldKey {
		t.Fatalf("CheckKey and serviceCheckKey produce different results:\n  new: %s\n  old: %s", newKey, oldKey)
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
