// Package scheduler — checks.go extracts service check execution into a
// standalone, testable ServiceChecker module.
//
// This file is ADDITIVE: it exists alongside the existing scheduler.go
// service check code. Wiring into the scheduler happens in a later issue.
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// defaultInterval is the fallback per-check interval when none is specified.
const defaultInterval = 300 // 5 minutes

// SpeedTestRunner is a function that executes a speed test and returns
// the result. The default implementation calls the Ookla CLI; tests
// inject a stub.
type SpeedTestRunner func() *internal.SpeedTestResult

// ServiceChecker owns execution of the 7 service check types, consecutive
// failure tracking, and per-check interval management.
type ServiceChecker struct {
	store          storage.ServiceCheckStore
	logger         *slog.Logger
	lastRun        map[string]time.Time
	mu             sync.Mutex
	speedTestRunFn SpeedTestRunner // nil → speed checks return "no tool"
}

// NewServiceChecker creates a ready-to-use ServiceChecker.
func NewServiceChecker(store storage.ServiceCheckStore, logger *slog.Logger) *ServiceChecker {
	return &ServiceChecker{
		store:   store,
		logger:  logger,
		lastRun: make(map[string]time.Time),
	}
}

// SetSpeedTestRunner injects a speed-test function (useful for testing).
func (sc *ServiceChecker) SetSpeedTestRunner(fn SpeedTestRunner) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.speedTestRunFn = fn
}

// RunDueChecks evaluates which checks are due based on their intervals,
// executes them, tracks consecutive failures, and persists results.
// It returns the slice of results for checks that were actually executed.
func (sc *ServiceChecker) RunDueChecks(checks []internal.ServiceCheckConfig, now time.Time) []internal.ServiceCheckResult {
	var due []internal.ServiceCheckConfig
	for _, check := range checks {
		if !check.Enabled {
			continue
		}
		if !IsSupportedCheckType(check.Type) {
			continue
		}
		key := CheckKey(check)
		interval := check.IntervalSec
		if interval <= 0 {
			interval = defaultInterval
		}
		sc.mu.Lock()
		last, exists := sc.lastRun[key]
		sc.mu.Unlock()
		if !exists || now.Sub(last) >= time.Duration(interval)*time.Second {
			due = append(due, check)
		}
	}
	if len(due) == 0 {
		return nil
	}

	results := make([]internal.ServiceCheckResult, 0, len(due))
	for _, check := range due {
		result := sc.RunCheck(check, now)

		// Track consecutive failures from store history.
		state, ok, err := sc.store.GetLatestServiceCheckState(result.Key)
		if err != nil {
			sc.logger.Warn("service check state read failed", "check", result.Name, "error", err)
		}
		if result.Status == "down" {
			if ok && state.Status == "down" {
				result.ConsecutiveFailures = state.ConsecutiveFailures + 1
			} else {
				result.ConsecutiveFailures = 1
			}
		}
		// If status is "up" (or "degraded"), ConsecutiveFailures stays 0.

		results = append(results, result)

		sc.mu.Lock()
		sc.lastRun[result.Key] = now
		sc.mu.Unlock()
	}

	// Persist results.
	if err := sc.store.SaveServiceCheckResults(results); err != nil {
		sc.logger.Warn("failed to save service check results", "error", err)
	}

	return results
}

// RunCheck executes a single service check by type and returns the result.
// It does NOT track consecutive failures or persist — that is RunDueChecks' job.
func (sc *ServiceChecker) RunCheck(check internal.ServiceCheckConfig, now time.Time) internal.ServiceCheckResult {
	typeName := strings.ToLower(strings.TrimSpace(check.Type))
	timeoutSec := check.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	threshold := check.FailureThreshold
	if threshold <= 0 {
		threshold = 1
	}
	severity := check.FailureSeverity
	if severity == "" {
		severity = internal.SeverityWarning
	}

	result := internal.ServiceCheckResult{
		Key:              CheckKey(check),
		Name:             strings.TrimSpace(check.Name),
		Type:             typeName,
		Target:           strings.TrimSpace(check.Target),
		Status:           "down",
		CheckedAt:        now.UTC().Format(time.RFC3339),
		FailureThreshold: threshold,
		FailureSeverity:  severity,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	switch typeName {
	case internal.ServiceCheckHTTP:
		sc.runHTTPCheck(ctx, check, &result, start, timeoutSec)
	case internal.ServiceCheckDNS:
		sc.runDNSCheck(ctx, check, &result, start)
	case internal.ServiceCheckTCP, internal.ServiceCheckSMB, internal.ServiceCheckNFS:
		sc.runTCPCheck(ctx, check, &result, start, timeoutSec)
	case internal.ServiceCheckPing:
		sc.runPingCheck(ctx, check, &result, start, timeoutSec)
	case internal.ServiceCheckSpeed:
		sc.runSpeedCheck(check, &result, start)
	default:
		result.Error = "unsupported service check type"
	}

	if result.ResponseMS == 0 {
		result.ResponseMS = time.Since(start).Milliseconds()
	}
	if result.Status == "up" {
		result.Error = ""
	}
	return result
}

// ── Check type implementations ─────────────────────────────────────────

func (sc *ServiceChecker) runHTTPCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time, timeoutSec int) {
	urlValue := NormalizeHTTPURL(check.Target)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlValue, nil)
	if err != nil {
		result.Error = err.Error()
		return
	}
	req.Header.Set("User-Agent", "nas-doctor-service-check/1.0")
	for k, v := range check.Headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: time.Duration(timeoutSec) * time.Second}).Do(req)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return
	}
	_ = resp.Body.Close()

	minStatus := check.ExpectedMin
	maxStatus := check.ExpectedMax
	if minStatus <= 0 {
		minStatus = 200
	}
	if maxStatus <= 0 {
		maxStatus = 399
	}
	if maxStatus < minStatus {
		maxStatus = minStatus
	}
	if resp.StatusCode < minStatus || resp.StatusCode > maxStatus {
		result.Error = fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode)
		return
	}
	result.Status = "up"
}

func (sc *ServiceChecker) runDNSCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time) {
	host := NormalizeDNSHost(check.Target)
	if host == "" {
		result.Error = "empty DNS target"
		return
	}
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return
	}
	if len(addrs) == 0 {
		result.Error = "no DNS records found"
		return
	}
	result.Status = "up"
}

func (sc *ServiceChecker) runTCPCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time, timeoutSec int) {
	addr, err := NormalizeTCPAddress(check)
	if err != nil {
		result.Error = err.Error()
		return
	}
	dialer := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return
	}
	_ = conn.Close()
	result.Status = "up"
}

func (sc *ServiceChecker) runPingCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time, timeoutSec int) {
	host := NormalizeDNSHost(check.Target)
	if host == "" {
		result.Error = "empty ping target"
		return
	}
	countArg := "-c"
	timeoutArg := "-W"
	timeoutVal := fmt.Sprintf("%d", timeoutSec)
	if runtime.GOOS == "darwin" {
		timeoutArg = "-t"
	}
	cmd := exec.CommandContext(ctx, "ping", countArg, "1", timeoutArg, timeoutVal, host)
	out, err := cmd.CombinedOutput()
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = "host unreachable"
		return
	}
	// Parse round-trip time from ping output if available.
	outStr := string(out)
	if idx := strings.Index(outStr, "time="); idx >= 0 {
		sub := outStr[idx+5:]
		if sp := strings.IndexAny(sub, " m\n"); sp > 0 {
			if ms, parseErr := strconv.ParseFloat(sub[:sp], 64); parseErr == nil {
				result.ResponseMS = int64(ms)
			}
		}
	}
	result.Status = "up"
}

func (sc *ServiceChecker) runSpeedCheck(check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time) {
	sc.mu.Lock()
	runner := sc.speedTestRunFn
	sc.mu.Unlock()

	if runner == nil {
		result.Error = "no speedtest tool available (install speedtest or speedtest-cli)"
		return
	}

	stResult := runner()
	if stResult == nil {
		result.Error = "no speedtest tool available (install speedtest or speedtest-cli)"
		return
	}

	result.DownloadMbps = stResult.DownloadMbps
	result.UploadMbps = stResult.UploadMbps
	result.LatencyMs = stResult.LatencyMs
	result.ResponseMS = int64(stResult.LatencyMs)

	// Apply margin of error (default 10%).
	margin := check.MarginPct
	if margin <= 0 {
		margin = 10
	}
	marginFactor := 1 - (margin / 100)

	dlThreshold := check.ContractedDownMbps * marginFactor
	ulThreshold := check.ContractedUpMbps * marginFactor

	dlOK := check.ContractedDownMbps <= 0 || stResult.DownloadMbps >= dlThreshold
	ulOK := check.ContractedUpMbps <= 0 || stResult.UploadMbps >= ulThreshold
	result.DownloadOK = &dlOK
	result.UploadOK = &ulOK

	switch {
	case dlOK && ulOK:
		result.Status = "up"
	case dlOK || ulOK:
		result.Status = "degraded"
		which := "upload"
		if !dlOK {
			which = "download"
		}
		result.Error = fmt.Sprintf("%s below contracted speed (%.0f/%.0f Mbps, threshold %.0f with %.0f%% margin)",
			which, stResult.DownloadMbps, stResult.UploadMbps,
			check.ContractedDownMbps, margin)
	default:
		result.Error = fmt.Sprintf("both download and upload below contracted speed (%.0f/%.0f Mbps, contracted %.0f/%.0f with %.0f%% margin)",
			stResult.DownloadMbps, stResult.UploadMbps,
			check.ContractedDownMbps, check.ContractedUpMbps, margin)
	}
}

// ── Helpers (exported so tests and future consumers can use them) ───────

// IsSupportedCheckType returns true if the given check type string is a
// recognised service check type.
func IsSupportedCheckType(checkType string) bool {
	switch strings.ToLower(strings.TrimSpace(checkType)) {
	case internal.ServiceCheckHTTP,
		internal.ServiceCheckTCP,
		internal.ServiceCheckDNS,
		internal.ServiceCheckSMB,
		internal.ServiceCheckNFS,
		internal.ServiceCheckPing,
		internal.ServiceCheckSpeed:
		return true
	default:
		return false
	}
}

// CheckKey produces a deterministic hash key for a service check config.
func CheckKey(check internal.ServiceCheckConfig) string {
	raw := strings.ToLower(strings.TrimSpace(check.Name)) + "|" +
		strings.ToLower(strings.TrimSpace(check.Type)) + "|" +
		strings.ToLower(strings.TrimSpace(check.Target)) + "|" +
		fmt.Sprintf("%d", check.Port)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NormalizeHTTPURL ensures the target has an http:// or https:// scheme.
func NormalizeHTTPURL(rawTarget string) string {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return target
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	return "http://" + target
}

// NormalizeDNSHost strips URL schemes and port numbers to extract a bare hostname.
func NormalizeDNSHost(rawTarget string) string {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return ""
	}
	if strings.Contains(target, "://") {
		if parsed, err := url.Parse(target); err == nil {
			if host := strings.TrimSpace(parsed.Hostname()); host != "" {
				return host
			}
		}
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		return host
	}
	return target
}

// NormalizeTCPAddress resolves the host:port address for a TCP-family check
// (tcp, smb, nfs), using default ports where appropriate.
func NormalizeTCPAddress(check internal.ServiceCheckConfig) (string, error) {
	target := strings.TrimSpace(check.Target)
	if target == "" {
		return "", fmt.Errorf("empty target")
	}
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err == nil && parsed.Host != "" {
			target = parsed.Host
		}
	}

	if _, _, err := net.SplitHostPort(target); err == nil {
		return target, nil
	}

	port := check.Port
	if port <= 0 {
		switch strings.ToLower(strings.TrimSpace(check.Type)) {
		case internal.ServiceCheckSMB:
			port = 445
		case internal.ServiceCheckNFS:
			port = 2049
		}
	}
	if port <= 0 {
		return "", fmt.Errorf("missing port")
	}
	host := NormalizeDNSHost(target)
	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}
