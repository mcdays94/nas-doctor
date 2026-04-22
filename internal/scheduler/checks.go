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
	"io"
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
	// collectDetails, when true, enriches RunCheck results with the
	// per-check-type Details map (HTTP status code, resolved IPs, DNS
	// records, etc.). Both the ad-hoc Test-button endpoint (#154) and
	// the scheduled path (#182, since v0.9.4) set this to true. The
	// /service-checks log UI reads the persisted blob via
	// service_checks_history.details_json.
	collectDetails bool
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

// SetCollectDetails toggles rich-details output. When true, RunCheck
// populates the per-type Details map (HTTP status code, resolved IPs,
// DNS records, failure stage, etc.) alongside the usual
// status/response_ms/error triple. Both the scheduler (issue #182) and
// the ad-hoc /service-checks/test endpoint (issue #154) call this with
// true; the scheduled path additionally persists the map into
// service_checks_history.details_json so the /service-checks log UI
// can render the rich context on expanded log rows.
func (sc *ServiceChecker) SetCollectDetails(enabled bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.collectDetails = enabled
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
		// Relaxation of the #154 invariant: the scheduled path now
		// carries the per-type Details map through to the store so
		// the /service-checks log UI can show HTTP status codes, DNS
		// records, resolved addresses, etc. in expanded log rows.
		// See issue #182. The map is only populated when the parent
		// ServiceChecker has SetCollectDetails(true) — otherwise
		// RunCheck returns Details == nil and this loop is a no-op.

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

	sc.mu.Lock()
	collect := sc.collectDetails
	sc.mu.Unlock()
	if collect {
		result.Details = make(map[string]any, 4)
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

	// If collection is off, ensure Details is nil (zero-length maps would
	// still serialise as "details":{}). This guarantees backward-compat
	// JSON output for the scheduled path.
	if !collect {
		result.Details = nil
	}

	if result.ResponseMS == 0 {
		result.ResponseMS = time.Since(start).Milliseconds()
	}
	// Floor successful sub-millisecond checks at 1ms. time.Since().Milliseconds()
	// truncates fractional values (int64), so LAN-local DNS (Pi-hole, router)
	// and loopback HTTP genuinely finishing in 200-800µs would display as "0ms"
	// — which users read as "didn't run" rather than "near-instant". See #159.
	if (result.Status == "up" || result.Status == "degraded") && result.ResponseMS <= 0 {
		result.ResponseMS = 1
	}
	if result.Status == "up" {
		result.Error = ""
	}
	return result
}

// ── Check type implementations ─────────────────────────────────────────

func (sc *ServiceChecker) runHTTPCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time, timeoutSec int) {
	urlValue := NormalizeHTTPURL(check.Target)
	if result.Details != nil {
		result.Details["request_url"] = urlValue
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlValue, nil)
	if err != nil {
		result.Error = err.Error()
		if result.Details != nil {
			result.Details["failure_stage"] = "request_build"
		}
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
		if result.Details != nil {
			result.Details["failure_stage"] = classifyHTTPError(err)
		}
		return
	}
	defer resp.Body.Close()

	if result.Details != nil {
		result.Details["status_code"] = resp.StatusCode
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			result.Details["content_type"] = ct
		}
		if resp.Request != nil && resp.Request.URL != nil {
			result.Details["final_url"] = resp.Request.URL.String()
		} else {
			result.Details["final_url"] = urlValue
		}
		// Read body (capped) so we can report size AND release the
		// connection cleanly. Cap at 1 MiB to bound memory for users
		// who accidentally test a file-server URL.
		n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		result.Details["body_bytes"] = n
	}

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
		if result.Details != nil {
			result.Details["failure_stage"] = "http_status"
		}
		return
	}
	result.Status = "up"
}

// classifyHTTPError maps the common failure shapes to a short diagnostic
// label the UI can render (e.g. "connection refused", "dns lookup failed",
// "tls handshake failed", "timeout"). We look at both the error text and
// the typed wrappers Go's net stack produces; best-effort, not exhaustive.
func classifyHTTPError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "x509") || strings.Contains(msg, "tls"):
		return "tls"
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return "dns"
	case strings.Contains(msg, "connection refused"):
		return "connection_refused"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return "timeout"
	case strings.Contains(msg, "unreachable") || strings.Contains(msg, "no route"):
		return "network_unreachable"
	default:
		return "connect"
	}
}

func (sc *ServiceChecker) runDNSCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time) {
	host := NormalizeDNSHost(check.Target)
	if result.Details != nil {
		result.Details["query_host"] = host
	}
	if host == "" {
		result.Error = "empty DNS target"
		return
	}
	// Guard against IP targets here too (validation should reject them
	// earlier, but the test-button endpoint and pre-existing saved checks
	// may not flow through the validator). DNS resolution of a literal IP
	// is a silent no-op in Go's resolver — it returns immediately without
	// querying any server, so the check would always appear "up" in 0ms.
	// See issue #159.
	if net.ParseIP(host) != nil {
		result.Error = "DNS checks need a hostname like google.com; to test IP reachability use a Ping or TCP check"
		if result.Details != nil {
			result.Details["failure_stage"] = "ip_target_rejected"
		}
		return
	}

	resolver := net.DefaultResolver
	resolvedServer := "system"
	if dnsServer := strings.TrimSpace(check.DNSServer); dnsServer != "" {
		server := dnsServer
		if _, _, err := net.SplitHostPort(server); err != nil {
			server = net.JoinHostPort(server, "53")
		}
		resolvedServer = server
		timeoutSec := check.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 5
		}
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
				return d.DialContext(ctx, "udp", server)
			},
		}
	}
	if result.Details != nil {
		result.Details["dns_server"] = resolvedServer
	}
	addrs, err := resolver.LookupHost(ctx, host)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		if result.Details != nil {
			result.Details["failure_stage"] = classifyDNSError(err)
		}
		return
	}
	if len(addrs) == 0 {
		result.Error = "no DNS records found"
		if result.Details != nil {
			result.Details["failure_stage"] = "empty_answer"
		}
		return
	}
	if result.Details != nil {
		result.Details["records"] = addrs
	}
	result.Status = "up"
}

// classifyDNSError buckets the common lookup failures so the UI can
// explain why a DNS check is down.
func classifyDNSError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no such host"):
		return "nxdomain"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return "timeout"
	case strings.Contains(msg, "refused"):
		return "refused"
	case strings.Contains(msg, "server misbehaving"):
		return "servfail"
	default:
		return "resolver_error"
	}
}

func (sc *ServiceChecker) runTCPCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time, timeoutSec int) {
	addr, err := NormalizeTCPAddress(check)
	if err != nil {
		result.Error = err.Error()
		if result.Details != nil {
			result.Details["failure_stage"] = "address_parse"
		}
		return
	}
	if result.Details != nil {
		result.Details["resolved_address"] = addr
		// protocol_hint is purely informational — drives the small
		// badge in the expanded log entry + Test toast (issue #188).
		// Absent from the map when the port is not in the
		// well-known table so the JS renderer can use key-presence
		// to decide whether to draw the badge.
		if _, portStr, splitErr := net.SplitHostPort(addr); splitErr == nil {
			if portNum, convErr := strconv.Atoi(portStr); convErr == nil {
				if hint := ProtocolHint(portNum); hint != "" {
					result.Details["protocol_hint"] = hint
				}
			}
		}
	}
	dialer := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	result.ResponseMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		if result.Details != nil {
			result.Details["failure_stage"] = classifyHTTPError(err) // reuse: same net.Error families
		}
		return
	}
	_ = conn.Close()
	result.Status = "up"
}

func (sc *ServiceChecker) runPingCheck(ctx context.Context, check internal.ServiceCheckConfig, result *internal.ServiceCheckResult, start time.Time, timeoutSec int) {
	host := NormalizeDNSHost(check.Target)
	if result.Details != nil {
		result.Details["query_host"] = host
	}
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
		if result.Details != nil {
			result.Details["failure_stage"] = "unreachable"
		}
		return
	}
	// Parse round-trip time from ping output if available.
	outStr := string(out)
	if idx := strings.Index(outStr, "time="); idx >= 0 {
		sub := outStr[idx+5:]
		if sp := strings.IndexAny(sub, " m\n"); sp > 0 {
			if ms, parseErr := strconv.ParseFloat(sub[:sp], 64); parseErr == nil {
				result.ResponseMS = int64(ms)
				if result.Details != nil {
					result.Details["rtt_ms"] = ms
				}
			}
		}
	}
	// Best-effort loss parsing — ping prints e.g. "0% packet loss" on both
	// Linux and Darwin. Not all ping implementations include a space before
	// "packet loss"; we look for the "%" and walk backwards.
	if result.Details != nil {
		if pct := extractPacketLossPercent(outStr); pct >= 0 {
			result.Details["packet_loss_pct"] = pct
		}
	}
	result.Status = "up"
}

// extractPacketLossPercent parses "0% packet loss" / "0.0% packet loss" out
// of ping stdout. Returns -1 when the token is absent so callers can skip
// recording the key rather than lying about zero loss.
func extractPacketLossPercent(out string) float64 {
	idx := strings.Index(out, "% packet loss")
	if idx <= 0 {
		return -1
	}
	// Walk back until we hit a non-digit/non-dot character.
	i := idx - 1
	for i >= 0 {
		c := out[i]
		if (c >= '0' && c <= '9') || c == '.' {
			i--
			continue
		}
		break
	}
	num := out[i+1 : idx]
	if num == "" {
		return -1
	}
	if f, err := strconv.ParseFloat(num, 64); err == nil {
		return f
	}
	return -1
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
