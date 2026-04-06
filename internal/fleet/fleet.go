// Package fleet manages polling and aggregation of remote NAS Doctor instances.
package fleet

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

// Stop halts the polling loop.
func (m *Manager) Stop() {
	close(m.stop)
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

	url := srv.URL + "/api/v1/status"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("invalid URL: %v", err)
		return result
	}
	req.Header.Set("User-Agent", "nas-doctor-fleet/1.0")
	if srv.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+srv.APIKey)
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
		Uptime        string `json:"uptime"`
		OverallHealth string `json:"overall_health"`
		CriticalCount int    `json:"critical_count"`
		WarningCount  int    `json:"warning_count"`
		InfoCount     int    `json:"info_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		result.Error = fmt.Sprintf("parse error: %v", err)
		return result
	}

	result.Online = true
	result.Hostname = statusResp.Hostname
	result.Platform = statusResp.Platform
	result.Uptime = statusResp.Uptime
	result.OverallHealth = statusResp.OverallHealth
	result.CriticalCount = statusResp.CriticalCount
	result.WarningCount = statusResp.WarningCount
	result.InfoCount = statusResp.InfoCount

	return result
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
