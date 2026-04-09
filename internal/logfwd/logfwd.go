// Package logfwd forwards diagnostic scan results to external logging/observability systems.
package logfwd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Destination mirrors the settings model for a single forwarding target.
type Destination struct {
	Name    string
	Type    string // loki, http_json, syslog
	URL     string
	Enabled bool
	Headers map[string]string
	Labels  map[string]string
	Format  string // full, findings_only, summary
}

// Forwarder sends scan results to configured destinations.
type Forwarder struct {
	destinations []Destination
	logger       *slog.Logger
	client       *http.Client
}

// New creates a new log forwarder.
func New(logger *slog.Logger) *Forwarder {
	return &Forwarder{
		logger: logger,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// SetDestinations updates the forwarding destinations.
func (f *Forwarder) SetDestinations(dests []Destination) {
	f.destinations = dests
}

// Forward sends the snapshot to all enabled destinations.
func (f *Forwarder) Forward(snap *internal.Snapshot, hostname string) {
	for _, dest := range f.destinations {
		if !dest.Enabled {
			continue
		}
		var err error
		switch strings.ToLower(dest.Type) {
		case "loki":
			err = f.sendLoki(dest, snap, hostname)
		case "http_json", "http":
			err = f.sendHTTPJSON(dest, snap, hostname)
		case "syslog":
			err = f.sendSyslog(dest, snap, hostname)
		default:
			f.logger.Warn("unknown log forward type", "name", dest.Name, "type", dest.Type)
			continue
		}
		if err != nil {
			f.logger.Warn("log forward failed", "name", dest.Name, "type", dest.Type, "error", err)
		} else {
			f.logger.Info("log forward sent", "name", dest.Name, "type", dest.Type, "findings", len(snap.Findings))
		}
	}
}

// ── Loki Push API ──

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

func (f *Forwarder) sendLoki(dest Destination, snap *internal.Snapshot, hostname string) error {
	labels := map[string]string{
		"job":      "nasdoctor",
		"hostname": hostname,
		"platform": snap.System.Platform,
	}
	for k, v := range dest.Labels {
		labels[k] = v
	}

	nowNano := strconv.FormatInt(time.Now().UnixNano(), 10)
	var values [][]string

	format := strings.ToLower(dest.Format)
	if format == "" {
		format = "full"
	}

	switch format {
	case "findings_only":
		for _, finding := range snap.Findings {
			line := fmt.Sprintf("level=%s category=%s title=%q description=%q",
				finding.Severity, finding.Category, finding.Title, finding.Description)
			values = append(values, []string{nowNano, line})
		}
	case "summary":
		critical, warnings, infos := countFindings(snap.Findings)
		line := fmt.Sprintf("scan_complete host=%s findings=%d critical=%d warning=%d info=%d duration=%.1fs",
			hostname, len(snap.Findings), critical, warnings, infos, snap.Duration)
		values = append(values, []string{nowNano, line})
	default: // full
		for _, finding := range snap.Findings {
			line := fmt.Sprintf("level=%s category=%s title=%q description=%q",
				finding.Severity, finding.Category, finding.Title, finding.Description)
			values = append(values, []string{nowNano, line})
		}
		// Also push a summary line
		critical, warnings, infos := countFindings(snap.Findings)
		summary := fmt.Sprintf("scan_complete host=%s findings=%d critical=%d warning=%d info=%d duration=%.1fs smart_drives=%d disks=%d containers=%d",
			hostname, len(snap.Findings), critical, warnings, infos, snap.Duration,
			len(snap.SMART), len(snap.Disks), len(snap.Docker.Containers))
		values = append(values, []string{nowNano, summary})
	}

	if len(values) == 0 {
		values = append(values, []string{nowNano, fmt.Sprintf("scan_complete host=%s findings=0 duration=%.1fs", hostname, snap.Duration)})
	}

	payload := lokiPushRequest{
		Streams: []lokiStream{{Stream: labels, Values: values}},
	}
	return f.postJSON(dest, payload)
}

// ── HTTP JSON ──

type httpJSONPayload struct {
	Timestamp string                        `json:"timestamp"`
	Hostname  string                        `json:"hostname"`
	Platform  string                        `json:"platform"`
	Duration  float64                       `json:"duration_seconds"`
	Summary   httpJSONSummary               `json:"summary"`
	Findings  []internal.Finding            `json:"findings,omitempty"`
	Services  []internal.ServiceCheckResult `json:"service_checks,omitempty"`
}

type httpJSONSummary struct {
	TotalFindings int     `json:"total_findings"`
	Critical      int     `json:"critical"`
	Warning       int     `json:"warning"`
	Info          int     `json:"info"`
	DiskCount     int     `json:"disk_count"`
	SMARTCount    int     `json:"smart_drive_count"`
	Containers    int     `json:"container_count"`
	CPUUsage      float64 `json:"cpu_usage_percent"`
	MemPercent    float64 `json:"mem_percent"`
}

func (f *Forwarder) sendHTTPJSON(dest Destination, snap *internal.Snapshot, hostname string) error {
	critical, warnings, infos := countFindings(snap.Findings)

	payload := httpJSONPayload{
		Timestamp: snap.Timestamp.Format(time.RFC3339),
		Hostname:  hostname,
		Platform:  snap.System.Platform,
		Duration:  snap.Duration,
		Summary: httpJSONSummary{
			TotalFindings: len(snap.Findings),
			Critical:      critical,
			Warning:       warnings,
			Info:          infos,
			DiskCount:     len(snap.Disks),
			SMARTCount:    len(snap.SMART),
			Containers:    len(snap.Docker.Containers),
			CPUUsage:      snap.System.CPUUsage,
			MemPercent:    snap.System.MemPercent,
		},
	}

	format := strings.ToLower(dest.Format)
	if format != "summary" {
		payload.Findings = snap.Findings
		payload.Services = snap.Services
	}

	return f.postJSON(dest, payload)
}

// ── Syslog (RFC 5424 over TCP/UDP) ──

func (f *Forwarder) sendSyslog(dest Destination, snap *internal.Snapshot, hostname string) error {
	addr := dest.URL
	if !strings.Contains(addr, ":") {
		addr += ":514"
	}

	proto := "udp"
	if strings.HasPrefix(strings.ToLower(addr), "tcp://") {
		proto = "tcp"
		addr = strings.TrimPrefix(addr, "tcp://")
	} else {
		addr = strings.TrimPrefix(addr, "udp://")
	}

	conn, err := net.DialTimeout(proto, addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("syslog dial %s: %w", addr, err)
	}
	defer conn.Close()

	now := time.Now().Format(time.RFC3339)
	appName := "nasdoctor"

	for _, finding := range snap.Findings {
		pri := syslogPriority(finding.Severity)
		msg := fmt.Sprintf("<%d>1 %s %s %s - - - [%s] %s: %s",
			pri, now, hostname, appName, finding.Category, finding.Title, finding.Description)
		if _, err := fmt.Fprintf(conn, "%s\n", msg); err != nil {
			return fmt.Errorf("syslog write: %w", err)
		}
	}

	// Summary line
	critical, warnings, infos := countFindings(snap.Findings)
	msg := fmt.Sprintf("<14>1 %s %s %s - - - scan_complete findings=%d critical=%d warning=%d info=%d duration=%.1fs",
		now, hostname, appName, len(snap.Findings), critical, warnings, infos, snap.Duration)
	_, err = fmt.Fprintf(conn, "%s\n", msg)
	return err
}

func syslogPriority(sev internal.Severity) int {
	// facility=1 (user-level) + severity
	switch strings.ToLower(string(sev)) {
	case "critical":
		return 8 + 2 // user.crit
	case "warning":
		return 8 + 4 // user.warning
	case "info":
		return 8 + 6 // user.info
	default:
		return 8 + 6
	}
}

// ── Helpers ──

func (f *Forwarder) postJSON(dest Destination, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := dest.URL
	if strings.ToLower(dest.Type) == "loki" && !strings.HasSuffix(url, "/loki/api/v1/push") {
		url = strings.TrimRight(url, "/") + "/loki/api/v1/push"
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range dest.Headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return nil
}

func countFindings(findings []internal.Finding) (critical, warnings, infos int) {
	for _, f := range findings {
		switch strings.ToLower(string(f.Severity)) {
		case "critical":
			critical++
		case "warning":
			warnings++
		case "info":
			infos++
		}
	}
	return
}
