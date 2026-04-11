// Package fleet manages polling and aggregation of remote NAS Doctor instances.
package fleet

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Manager handles remote server polling and status aggregation.
type Manager struct {
	mu       sync.RWMutex
	servers  []internal.RemoteServer
	statuses map[string]*internal.RemoteServerStatus // keyed by server ID
	logger   *slog.Logger
	client   *http.Client
	stop     chan struct{}
	stopOnce sync.Once
}

// New creates a new fleet manager.
func New(logger *slog.Logger) *Manager {
	return &Manager{
		statuses: make(map[string]*internal.RemoteServerStatus),
		logger:   logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		stop: make(chan struct{}),
	}
}

// SetServers updates the list of remote servers to monitor.
func (m *Manager) SetServers(servers []internal.RemoteServer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = servers
	// Clean up statuses for removed servers
	active := make(map[string]bool)
	for _, s := range servers {
		active[s.ID] = true
	}
	for id := range m.statuses {
		if !active[id] {
			delete(m.statuses, id)
		}
	}
}

// InjectStatuses allows demo mode to set pre-built statuses directly.
func (m *Manager) InjectStatuses(statuses []internal.RemoteServerStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range statuses {
		s := statuses[i]
		m.statuses[s.Server.ID] = &s
	}
}

// Start begins periodic polling of remote servers.
func (m *Manager) Start(interval time.Duration) {
	go func() {
		// Poll immediately
		m.PollAll()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.PollAll()
			case <-m.stop:
				return
			}
		}
	}()
}

// Stop halts the polling loop. Safe to call multiple times.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stop) })
}

// PollAll polls all enabled remote servers concurrently.
func (m *Manager) PollAll() {
	m.mu.RLock()
	servers := make([]internal.RemoteServer, len(m.servers))
	copy(servers, m.servers)
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		wg.Add(1)
		go func(s internal.RemoteServer) {
			defer wg.Done()
			status := m.pollServer(s)
			m.mu.Lock()
			m.statuses[s.ID] = status
			m.mu.Unlock()
		}(srv)
	}
	wg.Wait()
}

// pollServer fetches status from a single remote NAS Doctor instance.
func (m *Manager) pollServer(srv internal.RemoteServer) *internal.RemoteServerStatus {
	result := &internal.RemoteServerStatus{
		Server:   srv,
		LastPoll: time.Now().Format(time.RFC3339),
	}

	url := strings.TrimRight(srv.URL, "/") + "/api/v1/status"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("invalid URL: %v", err)
		return result
	}
	req.Header.Set("User-Agent", "nas-doctor-fleet/1.0")
	if srv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+srv.APIKey)
	}
	// Inject custom headers (e.g. Cloudflare Access service tokens)
	for k, v := range srv.Headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("connection failed: %v", err)
		m.logger.Warn("fleet poll failed", "server", srv.Name, "url", srv.URL, "error", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}

	// Parse the status response (same shape as our own /api/v1/status)
	var statusResp struct {
		Hostname      string `json:"hostname"`
		Platform      string `json:"platform"`
		Version       string `json:"version"`
		Uptime        string `json:"uptime"`
		OverallHealth string `json:"overall_health"`
		CriticalCount int    `json:"critical_count"`
		WarningCount  int    `json:"warning_count"`
		InfoCount     int    `json:"info_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		result.Error = "response is not valid JSON — likely a proxy login page (check auth headers)"
		return result
	}
	// Validate this is a NAS Doctor instance
	if statusResp.Hostname == "" && statusResp.Platform == "" {
		result.Error = "response does not look like a NAS Doctor instance — check URL and auth headers"
		return result
	}

	result.Online = true
	result.Hostname = statusResp.Hostname
	result.Platform = statusResp.Platform
	result.Version = statusResp.Version
	result.Uptime = statusResp.Uptime
	result.OverallHealth = statusResp.OverallHealth
	result.CriticalCount = statusResp.CriticalCount
	result.WarningCount = statusResp.WarningCount
	result.InfoCount = statusResp.InfoCount

	// Fetch rich snapshot data (best-effort — older instances may not support this)
	result.Summary = m.fetchSummary(srv)

	return result
}

// fetchSummary fetches /api/v1/snapshot/latest from a remote instance and
// extracts a condensed summary. Returns nil on any failure (graceful degradation).
func (m *Manager) fetchSummary(srv internal.RemoteServer) *internal.FleetServerSummary {
	url := strings.TrimRight(srv.URL, "/") + "/api/v1/snapshot/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "nas-doctor-fleet/1.0")
	if srv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+srv.APIKey)
	}
	for k, v := range srv.Headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}

	var snap struct {
		System struct {
			CPUUsage   float64 `json:"cpu_usage_percent"`
			MemPercent float64 `json:"mem_percent"`
			MemTotalMB int     `json:"mem_total_mb"`
			MemUsedMB  int     `json:"mem_used_mb"`
			CPUModel   string  `json:"cpu_model"`
			CPUCores   int     `json:"cpu_cores"`
			LoadAvg1   float64 `json:"load_avg_1"`
			IOWait     float64 `json:"io_wait_percent"`
		} `json:"system"`
		SMART []struct {
			HealthPassed       bool    `json:"health_passed"`
			SizeGB             float64 `json:"size_gb"`
			ReallocatedSectors int64   `json:"reallocated_sectors"`
			PendingSectors     int64   `json:"pending_sectors"`
		} `json:"smart"`
		Docker struct {
			Available  bool `json:"available"`
			Containers []struct {
				State string `json:"state"`
			} `json:"containers"`
		} `json:"docker"`
		ServiceChecks []struct {
			Status string `json:"status"` // "up" or "down"
		} `json:"service_checks"`
		Findings []struct {
			Severity string `json:"severity"`
			Title    string `json:"title"`
			Category string `json:"category"`
		} `json:"findings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil
	}

	s := &internal.FleetServerSummary{
		CPUUsage:   snap.System.CPUUsage,
		MemPercent: snap.System.MemPercent,
		MemTotalMB: snap.System.MemTotalMB,
		MemUsedMB:  snap.System.MemUsedMB,
		CPUModel:   snap.System.CPUModel,
		CPUCores:   snap.System.CPUCores,
		LoadAvg1:   snap.System.LoadAvg1,
		IOWait:     snap.System.IOWait,
	}

	// Drives
	s.DriveCount = len(snap.SMART)
	var totalGB float64
	for _, d := range snap.SMART {
		totalGB += d.SizeGB
		if !d.HealthPassed || d.PendingSectors > 4 || d.ReallocatedSectors > 19 {
			s.DrivesCritical++
		} else if d.ReallocatedSectors > 0 || d.PendingSectors > 0 {
			s.DrivesWarning++
		} else {
			s.DrivesHealthy++
		}
	}
	s.TotalStorageTB = totalGB / 1000

	// Docker
	s.DockerAvailable = snap.Docker.Available
	for _, c := range snap.Docker.Containers {
		s.ContainersTotal++
		if c.State == "running" {
			s.ContainersRunning++
		}
	}

	// Service checks
	for _, sc := range snap.ServiceChecks {
		s.ServiceChecksTotal++
		if sc.Status == "up" {
			s.ServiceChecksUp++
		} else {
			s.ServiceChecksDown++
		}
	}

	// Findings
	for _, f := range snap.Findings {
		s.Findings = append(s.Findings, internal.FleetFinding{
			Severity: f.Severity,
			Title:    f.Title,
			Category: f.Category,
		})
	}

	return s
}

// GetStatuses returns the current status of all remote servers.
func (m *Manager) GetStatuses() []internal.RemoteServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []internal.RemoteServerStatus
	for _, srv := range m.servers {
		if status, ok := m.statuses[srv.ID]; ok {
			results = append(results, *status)
		} else {
			results = append(results, internal.RemoteServerStatus{
				Server: srv,
				Error:  "not yet polled",
			})
		}
	}
	return results
}

// GetServers returns the configured remote servers.
func (m *Manager) GetServers() []internal.RemoteServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]internal.RemoteServer, len(m.servers))
	copy(result, m.servers)
	return result
}

// TestServer polls a single server and returns the result immediately.
func (m *Manager) TestServer(srv internal.RemoteServer) *internal.RemoteServerStatus {
	return m.pollServer(srv)
}
