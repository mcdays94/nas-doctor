package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"fmt"
	"path/filepath"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// ---------- Settings types ----------

// Settings represents the user-configurable application settings stored in the DB.
type Settings struct {
	ScanInterval      string                  `json:"scan_interval"`
	Theme             string                  `json:"theme"`
	Icon              string                  `json:"icon"`
	Notifications     SettingsNotifications   `json:"notifications"`
	LogPush           SettingsLogForward      `json:"log_push"`
	Retention         RetentionSettings       `json:"retention"`
	Backup            BackupSettings          `json:"backup"`
	Sections          DashboardSections       `json:"sections"`
	Fleet             []internal.RemoteServer `json:"fleet,omitempty"`
	DismissedFindings []string                `json:"dismissed_findings,omitempty"`
}

// DashboardSections controls which sections appear on the dashboard.
// All default to true (visible). Users can hide sections they don't use.
type DashboardSections struct {
	Findings  bool `json:"findings"`
	DiskSpace bool `json:"disk_space"`
	SMART     bool `json:"smart"`
	Docker    bool `json:"docker"`
	ZFS       bool `json:"zfs"`
	UPS       bool `json:"ups"`
	Parity    bool `json:"parity"`
	Network   bool `json:"network"`
}

// BackupSettings controls automatic backup of the application database.
type BackupSettings struct {
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path"`                  // Custom backup directory (empty = default: <data_dir>/backups)
	KeepCount  int    `json:"keep_count"`            // Number of backups to retain (0 = default 4)
	IntervalH  int    `json:"interval_hours"`        // Hours between backups (0 = default 168 = weekly)
	LastBackup string `json:"last_backup,omitempty"` // ISO timestamp of last successful backup
}

// RetentionSettings controls data lifecycle / auto-pruning.
type RetentionSettings struct {
	SnapshotDays  int `json:"snapshot_days"`   // Keep snapshots for N days (0 = default 90)
	MaxDBSizeMB   int `json:"max_db_size_mb"`  // Hard cap on DB file size in MB (0 = default 500)
	NotifyLogDays int `json:"notify_log_days"` // Keep notification log entries for N days (0 = default 30)
}

// SettingsNotifications holds the webhook list within settings.
type SettingsNotifications struct {
	Webhooks           []internal.WebhookConfig      `json:"webhooks"`
	Policies           []scheduler.AlertPolicy       `json:"policies,omitempty"`
	QuietHours         scheduler.QuietHours          `json:"quiet_hours,omitempty"`
	MaintenanceWindows []scheduler.MaintenanceWindow `json:"maintenance_windows,omitempty"`
	DefaultCooldownSec int                           `json:"default_cooldown_sec,omitempty"`
}

// SettingsLogForward holds the log-forwarding configuration within settings.
type SettingsLogForward struct {
	Enabled      bool                    `json:"enabled"`
	Destinations []LogForwardDestination `json:"destinations"`
}

// LogForwardDestination represents a single log-forwarding target.
type LogForwardDestination struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

const settingsConfigKey = "settings"

// defaultSettings returns the default settings used when none are persisted.
func defaultSettings() Settings {
	return Settings{
		ScanInterval: "6h",
		Theme:        ThemeMidnight,
		Icon:         "icon3",
		Notifications: SettingsNotifications{
			Webhooks:           []internal.WebhookConfig{},
			Policies:           []scheduler.AlertPolicy{},
			MaintenanceWindows: []scheduler.MaintenanceWindow{},
			QuietHours: scheduler.QuietHours{
				Enabled:   false,
				Timezone:  "UTC",
				StartHHMM: "22:00",
				EndHHMM:   "07:00",
			},
			DefaultCooldownSec: 900,
		},
		LogPush: SettingsLogForward{
			Enabled:      false,
			Destinations: []LogForwardDestination{},
		},
		Retention: RetentionSettings{
			SnapshotDays:  90,
			MaxDBSizeMB:   500,
			NotifyLogDays: 30,
		},
		Backup: BackupSettings{
			Enabled:   true,
			KeepCount: 4,
			IntervalH: 168, // weekly
		},
		Sections: DashboardSections{
			Findings:  true,
			DiskSpace: true,
			SMART:     true,
			Docker:    true,
			ZFS:       true,
			UPS:       true,
			Parity:    true,
			Network:   true,
		},
	}
}

// ---------- Route registration ----------

// RegisterExtendedRoutes registers additional API and page routes on the given router.
func (s *Server) RegisterExtendedRoutes(r chi.Router) {
	// API endpoints (registered directly, not as a sub-Route)
	r.Get("/api/v1/settings", s.handleGetSettings)
	r.Put("/api/v1/settings", s.handleUpdateSettings)
	r.Post("/api/v1/settings/test-webhook", s.handleTestWebhook)
	r.Get("/api/v1/disks", s.handleListDisks)
	r.Get("/api/v1/disks/{serial}", s.handleGetDisk)
	r.Get("/api/v1/history/system", s.handleSystemHistory)
	r.Get("/api/v1/notifications/log", s.handleNotificationLog)
	r.Get("/api/v1/alerts", s.handleListAlerts)
	r.Get("/api/v1/alerts/{id}", s.handleGetAlert)
	r.Get("/api/v1/alerts/{id}/events", s.handleGetAlertEvents)
	r.Post("/api/v1/alerts/{id}/ack", s.handleAckAlert)
	r.Post("/api/v1/alerts/{id}/unack", s.handleUnackAlert)
	r.Post("/api/v1/alerts/{id}/snooze", s.handleSnoozeAlert)
	r.Post("/api/v1/alerts/{id}/unsnooze", s.handleUnsnoozeAlert)
	r.Get("/api/v1/fleet", s.handleFleetStatus)
	r.Get("/api/v1/fleet/servers", s.handleFleetServers)
	r.Put("/api/v1/fleet/servers", s.handleFleetUpdateServers)
	r.Post("/api/v1/fleet/test", s.handleFleetTestServer)
	r.Post("/api/v1/findings/dismiss", s.handleDismissFinding)
	r.Post("/api/v1/findings/restore", s.handleRestoreFinding)

	// Fleet dashboard page
	r.Get("/fleet", s.handleFleetPage)

	// Stats page
	r.Get("/stats", s.handleStatsPage)
	r.Post("/api/v1/backup", s.handleCreateBackup)
	r.Get("/api/v1/backup", s.handleListBackups)
	r.Get("/api/v1/db/stats", s.handleDBStats)
	r.Get("/api/v1/sparklines", s.handleSparklines)

	// Pages
	r.Get("/settings", s.handleSettingsPage)
	r.Get("/disk/{serial}", s.handleDiskPage)
}

// ---------- Settings handlers ----------

// handleGetSettings returns the full settings JSON from the config table.
// GET /api/v1/settings
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	raw, err := s.store.GetConfig(settingsConfigKey)
	if err != nil {
		// No settings stored yet — return defaults.
		writeJSON(w, http.StatusOK, defaultSettings())
		return
	}

	var settings Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		s.logger.Error("failed to parse stored settings", "error", err)
		writeJSON(w, http.StatusOK, defaultSettings())
		return
	}

	// Ensure slice fields are never null in JSON output.
	if settings.Notifications.Webhooks == nil {
		settings.Notifications.Webhooks = []internal.WebhookConfig{}
	}
	if settings.Notifications.Policies == nil {
		settings.Notifications.Policies = []scheduler.AlertPolicy{}
	}
	if settings.Notifications.MaintenanceWindows == nil {
		settings.Notifications.MaintenanceWindows = []scheduler.MaintenanceWindow{}
	}
	if settings.Notifications.QuietHours.Timezone == "" {
		settings.Notifications.QuietHours.Timezone = "UTC"
	}
	if settings.Notifications.QuietHours.StartHHMM == "" {
		settings.Notifications.QuietHours.StartHHMM = "22:00"
	}
	if settings.Notifications.QuietHours.EndHHMM == "" {
		settings.Notifications.QuietHours.EndHHMM = "07:00"
	}
	if settings.Notifications.DefaultCooldownSec <= 0 {
		settings.Notifications.DefaultCooldownSec = 900
	}
	if settings.LogPush.Destinations == nil {
		settings.LogPush.Destinations = []LogForwardDestination{}
	}
	// Apply retention defaults for settings that predate this field.
	if settings.Retention.SnapshotDays == 0 {
		settings.Retention.SnapshotDays = 90
	}
	if settings.Retention.MaxDBSizeMB == 0 {
		settings.Retention.MaxDBSizeMB = 500
	}
	if settings.Retention.NotifyLogDays == 0 {
		settings.Retention.NotifyLogDays = 30
	}
	// Section defaults: if all are false (zero-value from old settings), set all to true
	s_sec := settings.Sections
	if !s_sec.Findings && !s_sec.DiskSpace && !s_sec.SMART && !s_sec.Docker && !s_sec.ZFS && !s_sec.UPS {
		settings.Sections = DashboardSections{
			Findings: true, DiskSpace: true, SMART: true, Docker: true,
			ZFS: true, UPS: true, Parity: true, Network: true,
		}
	}

	writeJSON(w, http.StatusOK, settings)
}

// handleUpdateSettings validates and persists the settings JSON.
// PUT /api/v1/settings
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer r.Body.Close()

	var settings Settings
	if err := json.Unmarshal(body, &settings); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	// Basic validation
	if settings.ScanInterval == "" {
		settings.ScanInterval = "6h"
	}
	if _, err := time.ParseDuration(settings.ScanInterval); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scan_interval: " + err.Error()})
		return
	}
	if settings.Theme == "" {
		settings.Theme = DefaultTheme
	}
	switch settings.Theme {
	case ThemeMidnight, ThemeClean, ThemeEmber:
		// valid
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid theme: " + settings.Theme})
		return
	}
	if settings.Notifications.Webhooks == nil {
		settings.Notifications.Webhooks = []internal.WebhookConfig{}
	}
	if settings.Notifications.Policies == nil {
		settings.Notifications.Policies = []scheduler.AlertPolicy{}
	}
	if settings.Notifications.MaintenanceWindows == nil {
		settings.Notifications.MaintenanceWindows = []scheduler.MaintenanceWindow{}
	}
	if settings.Notifications.DefaultCooldownSec <= 0 {
		settings.Notifications.DefaultCooldownSec = 900
	}
	if settings.Notifications.QuietHours.Timezone == "" {
		settings.Notifications.QuietHours.Timezone = "UTC"
	}
	if settings.Notifications.QuietHours.StartHHMM == "" {
		settings.Notifications.QuietHours.StartHHMM = "22:00"
	}
	if settings.Notifications.QuietHours.EndHHMM == "" {
		settings.Notifications.QuietHours.EndHHMM = "07:00"
	}
	if settings.Notifications.QuietHours.Enabled {
		if _, err := time.Parse("15:04", settings.Notifications.QuietHours.StartHHMM); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid quiet_hours.start_hhmm (expected HH:MM)"})
			return
		}
		if _, err := time.Parse("15:04", settings.Notifications.QuietHours.EndHHMM); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid quiet_hours.end_hhmm (expected HH:MM)"})
			return
		}
		if _, err := time.LoadLocation(settings.Notifications.QuietHours.Timezone); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid quiet_hours.timezone"})
			return
		}
	}
	knownWebhooks := make(map[string]struct{}, len(settings.Notifications.Webhooks))
	for _, wh := range settings.Notifications.Webhooks {
		knownWebhooks[strings.ToLower(strings.TrimSpace(wh.Name))] = struct{}{}
	}
	for i := range settings.Notifications.Policies {
		p := &settings.Notifications.Policies[i]
		if p.MinSeverity == "" {
			p.MinSeverity = internal.SeverityWarning
		}
		if p.CooldownSec < 0 {
			p.CooldownSec = 0
		}
		if p.Name == "" {
			p.Name = fmt.Sprintf("policy-%d", i+1)
		}
		if p.WebhookName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "notifications.policies webhook_name is required"})
			return
		}
		if _, ok := knownWebhooks[strings.ToLower(strings.TrimSpace(p.WebhookName))]; !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "notifications.policies references unknown webhook: " + p.WebhookName})
			return
		}
	}
	for _, mw := range settings.Notifications.MaintenanceWindows {
		if !mw.Enabled {
			continue
		}
		start, err1 := time.Parse(time.RFC3339, strings.TrimSpace(mw.StartISO))
		end, err2 := time.Parse(time.RFC3339, strings.TrimSpace(mw.EndISO))
		if err1 != nil || err2 != nil || !end.After(start) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid maintenance window (start_iso/end_iso must be RFC3339 and end > start)"})
			return
		}
	}
	if settings.LogPush.Destinations == nil {
		settings.LogPush.Destinations = []LogForwardDestination{}
	}
	// Retention defaults and bounds
	if settings.Retention.SnapshotDays < 7 {
		settings.Retention.SnapshotDays = 90
	}
	if settings.Retention.MaxDBSizeMB < 50 {
		settings.Retention.MaxDBSizeMB = 500
	}
	if settings.Retention.NotifyLogDays < 1 {
		settings.Retention.NotifyLogDays = 30
	}

	// Persist
	data, err := json.Marshal(settings)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal settings"})
		return
	}
	if err := s.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings: " + err.Error()})
		return
	}

	// Dynamically update the scheduler if changed.
	if s.scheduler != nil {
		if d, err := time.ParseDuration(settings.ScanInterval); err == nil {
			s.scheduler.UpdateInterval(d)
		}
		// Update retention config
		s.scheduler.UpdateRetention(scheduler.RetentionConfig{
			SnapshotDays:  settings.Retention.SnapshotDays,
			MaxDBSizeMB:   settings.Retention.MaxDBSizeMB,
			NotifyLogDays: settings.Retention.NotifyLogDays,
		})
		// Update backup config
		keepCount := settings.Backup.KeepCount
		if keepCount <= 0 {
			keepCount = 4
		}
		intervalH := settings.Backup.IntervalH
		if intervalH <= 0 {
			intervalH = 168
		}
		s.scheduler.UpdateBackup(scheduler.BackupConfig{
			Enabled:   settings.Backup.Enabled,
			Path:      settings.Backup.Path,
			KeepCount: keepCount,
			IntervalH: intervalH,
		})

		// Update notifier webhooks at runtime.
		s.scheduler.UpdateNotifier(s.buildNotifier(settings.Notifications.Webhooks))
		s.scheduler.UpdateAlerting(scheduler.AlertingConfig{
			Policies:           settings.Notifications.Policies,
			QuietHours:         settings.Notifications.QuietHours,
			MaintenanceWindows: settings.Notifications.MaintenanceWindows,
			DefaultCooldownSec: settings.Notifications.DefaultCooldownSec,
		})
	}

	writeJSON(w, http.StatusOK, settings)
}

// handleSparklines returns condensed history data for dashboard sparklines.
// GET /api/v1/sparklines
func (s *Server) handleSparklines(w http.ResponseWriter, r *http.Request) {
	type sparklineResponse struct {
		System []storage.SystemHistoryPoint `json:"system"`
		Disks  []storage.DiskSparklines     `json:"disks"`
	}

	sysHistory, err := s.store.GetSystemSparkline(60)
	if err != nil {
		sysHistory = nil
	}
	diskHistory, err := s.store.GetAllDiskSparklines(60)
	if err != nil {
		diskHistory = nil
	}

	writeJSON(w, http.StatusOK, sparklineResponse{
		System: sysHistory,
		Disks:  diskHistory,
	})
}

// ---------- Fleet handlers ----------

// handleFleetStatus returns aggregated status of all remote servers.
func (s *Server) handleFleetStatus(w http.ResponseWriter, r *http.Request) {
	if s.fleet == nil {
		writeJSON(w, http.StatusOK, []internal.RemoteServerStatus{})
		return
	}
	writeJSON(w, http.StatusOK, s.fleet.GetStatuses())
}

// handleFleetServers returns the configured remote servers.
func (s *Server) handleFleetServers(w http.ResponseWriter, r *http.Request) {
	if s.fleet == nil {
		writeJSON(w, http.StatusOK, []internal.RemoteServer{})
		return
	}
	writeJSON(w, http.StatusOK, s.fleet.GetServers())
}

// handleFleetUpdateServers replaces the list of remote servers.
func (s *Server) handleFleetUpdateServers(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var servers []internal.RemoteServer
	if err := json.Unmarshal(body, &servers); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Assign IDs to new servers
	for i := range servers {
		if servers[i].ID == "" {
			servers[i].ID = fmt.Sprintf("srv-%d", time.Now().UnixNano()+int64(i))
		}
	}

	// Persist in settings
	settings := s.getSettings()
	settings.Fleet = servers
	data, _ := json.Marshal(settings)
	s.store.SetConfig(settingsConfigKey, string(data))

	// Update fleet manager
	if s.fleet != nil {
		s.fleet.SetServers(servers)
		go s.fleet.PollAll() // poll immediately
	}

	writeJSON(w, http.StatusOK, servers)
}

// handleFleetTestServer tests connectivity to a single server.
func (s *Server) handleFleetTestServer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var srv internal.RemoteServer
	if err := json.Unmarshal(body, &srv); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if s.fleet == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "fleet not initialized"})
		return
	}
	result := s.fleet.TestServer(srv)
	writeJSON(w, http.StatusOK, result)
}

// handleDismissFinding adds a finding title to the dismissed list.
func (s *Server) handleDismissFinding(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	var req struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(body, &req) != nil || req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}
	settings := s.getSettings()
	// Check if already dismissed
	for _, d := range settings.DismissedFindings {
		if d == req.Title {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already dismissed"})
			return
		}
	}
	settings.DismissedFindings = append(settings.DismissedFindings, req.Title)
	data, _ := json.Marshal(settings)
	s.store.SetConfig(settingsConfigKey, string(data))
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

// handleRestoreFinding removes a finding title from the dismissed list.
func (s *Server) handleRestoreFinding(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	var req struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(body, &req) != nil || req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}
	settings := s.getSettings()
	var updated []string
	for _, d := range settings.DismissedFindings {
		if d != req.Title {
			updated = append(updated, d)
		}
	}
	settings.DismissedFindings = updated
	data, _ := json.Marshal(settings)
	s.store.SetConfig(settingsConfigKey, string(data))
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// handleCreateBackup triggers an immediate backup.
// POST /api/v1/backup
func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	settings := s.getSettings()
	backupDir := settings.Backup.Path
	result, err := s.store.CreateBackup(backupDir, s.logger)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "backup failed: " + err.Error()})
		return
	}
	keepCount := settings.Backup.KeepCount
	if keepCount <= 0 {
		keepCount = 4
	}
	dir := backupDir
	if dir == "" {
		dir = filepath.Dir(result.Path)
	}
	storage.PruneBackups(dir, keepCount, s.logger)
	writeJSON(w, http.StatusOK, result)
}

// handleListBackups returns existing backups.
// GET /api/v1/backup
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	settings := s.getSettings()
	backupDir := settings.Backup.Path
	if backupDir == "" {
		backupDir = filepath.Join(s.store.DataDir(), "backups")
	}
	backups, err := storage.ListBackups(backupDir)
	if err != nil {
		writeJSON(w, http.StatusOK, storage.BackupInfo{Backups: nil})
		return
	}
	writeJSON(w, http.StatusOK, storage.BackupInfo{Backups: backups})
}

// getSettings loads and returns the current settings with defaults applied.
func (s *Server) getSettings() Settings {
	settings := defaultSettings()
	if raw, err := s.store.GetConfig(settingsConfigKey); err == nil && raw != "" {
		json.Unmarshal([]byte(raw), &settings)
	}
	return settings
}

func (s *Server) buildNotifier(webhooks []internal.WebhookConfig) *notifier.Notifier {
	if len(webhooks) == 0 {
		return nil
	}
	n := notifier.New(webhooks, s.logger)
	n.SetResultHook(func(name, webhookType, status string, findingsCount int, errMsg string) {
		if err := s.store.SaveNotificationLog(name, webhookType, status, findingsCount, errMsg); err != nil {
			s.logger.Warn("failed to save notification log", "error", err)
		}
	})
	return n
}

// handleDBStats returns database size and row count statistics.
// GET /api/v1/db/stats
func (s *Server) handleDBStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetDBStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleTestWebhook sends a test notification through a single webhook config.
// POST /api/v1/settings/test-webhook
func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer r.Body.Close()

	var wh internal.WebhookConfig
	if err := json.Unmarshal(body, &wh); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if wh.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "webhook url is required"})
		return
	}
	// Force enabled for the test
	wh.Enabled = true
	if wh.MinLevel == "" {
		wh.MinLevel = internal.SeverityInfo
	}

	testFindings := []internal.Finding{
		{
			ID:          "test-001",
			Severity:    internal.SeverityInfo,
			Category:    internal.CategorySystem,
			Title:       "Test Notification",
			Description: "This is a test notification from NAS Doctor to verify your webhook configuration is working correctly.",
			Evidence:    []string{"Triggered manually via settings page"},
			Impact:      "None — this is a test.",
			Action:      "No action required.",
			Priority:    "low",
			Cost:        "none",
		},
	}

	// Create a temporary notifier with just this webhook.
	testLogger := s.logger.With("handler", "test-webhook")
	n := notifier.New([]internal.WebhookConfig{wh}, testLogger)
	n.SetResultHook(func(name, webhookType, status string, findingsCount int, errMsg string) {
		if err := s.store.SaveNotificationLog(name, webhookType, status, findingsCount, errMsg); err != nil {
			s.logger.Warn("failed to save notification log", "error", err)
		}
	})
	n.NotifyFindings(testFindings, "nas-doctor-test")

	// NotifyFindings does not return an error (it logs internally).
	// We optimistically report success; the user should check the target.
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "sent",
		"message": "Test notification dispatched. Check your webhook destination to confirm delivery.",
	})
}

// ---------- Disk handlers ----------

// handleListDisks returns all known disks from SMART history.
// GET /api/v1/disks
func (s *Server) handleListDisks(w http.ResponseWriter, r *http.Request) {
	disks, err := s.store.ListDisks()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list disks: " + err.Error()})
		return
	}
	if disks == nil {
		disks = []storage.DiskSummary{}
	}
	writeJSON(w, http.StatusOK, disks)
}

// diskDetailResponse is the shape returned by GET /api/v1/disks/{serial}.
type diskDetailResponse struct {
	Current  *internal.SMARTInfo        `json:"current"`
	History  []storage.DiskHistoryPoint `json:"history"`
	Findings []internal.Finding         `json:"findings"`
}

// handleGetDisk returns SMART data, history, and related findings for a specific disk.
// GET /api/v1/disks/{serial}
func (s *Server) handleGetDisk(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	if serial == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "serial is required"})
		return
	}

	resp := diskDetailResponse{}

	// Get history from the dedicated SMART history table.
	history, err := s.store.GetDiskHistory(serial, 500)
	if err != nil {
		s.logger.Error("failed to get disk history", "serial", serial, "error", err)
	}
	if history == nil {
		history = []storage.DiskHistoryPoint{}
	}
	resp.History = history

	// Get current SMART data and related findings from the latest snapshot.
	snap := s.latestSnapshot()
	if snap != nil {
		// Find the matching SMART entry by serial.
		for i := range snap.SMART {
			if snap.SMART[i].Serial == serial {
				resp.Current = &snap.SMART[i]
				break
			}
		}

		// Collect findings related to this disk.
		var related []internal.Finding
		for _, f := range snap.Findings {
			if f.RelatedDisk == serial {
				related = append(related, f)
				continue
			}
			// Also match if the finding title contains the device name.
			if resp.Current != nil && resp.Current.Device != "" &&
				strings.Contains(f.Title, resp.Current.Device) {
				related = append(related, f)
			}
		}
		if related == nil {
			related = []internal.Finding{}
		}
		resp.Findings = related
	} else {
		resp.Findings = []internal.Finding{}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------- System history handler ----------

// handleSystemHistory returns historical system metrics for chart rendering.
// GET /api/v1/history/system
func (s *Server) handleSystemHistory(w http.ResponseWriter, r *http.Request) {
	points, err := s.store.GetSystemHistory(500)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get system history: " + err.Error()})
		return
	}
	if points == nil {
		points = []storage.SystemHistoryPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}

// ---------- Notification log handler ----------

// handleNotificationLog returns recent notification delivery attempts.
// GET /api/v1/notifications/log
func (s *Server) handleNotificationLog(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.GetNotificationLog(100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get notification log: " + err.Error()})
		return
	}
	if entries == nil {
		entries = []storage.NotificationLogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// ---------- Alerts handlers ----------

// handleListAlerts returns recent alerts with optional status filtering.
// GET /api/v1/alerts
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", string(storage.AlertStatusOpen), string(storage.AlertStatusAcknowledged), string(storage.AlertStatusSnoozed), string(storage.AlertStatusResolved):
		// valid
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status filter"})
		return
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = parsed
	}

	alerts, err := s.store.ListAlerts(status, limit, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list alerts: " + err.Error()})
		return
	}
	if alerts == nil {
		alerts = []storage.AlertRecord{}
	}
	writeJSON(w, http.StatusOK, alerts)
}

// handleGetAlert returns a single alert by ID.
// GET /api/v1/alerts/{id}
func (s *Server) handleGetAlert(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseAlertID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	alert, err := s.store.GetAlert(alertID, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get alert: " + err.Error()})
		return
	}
	if alert == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

// handleGetAlertEvents returns timeline events for an alert.
// GET /api/v1/alerts/{id}/events
func (s *Server) handleGetAlertEvents(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseAlertID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	alert, err := s.store.GetAlert(alertID, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get alert: " + err.Error()})
		return
	}
	if alert == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = parsed
	}

	events, err := s.store.GetAlertEvents(alertID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get alert events: " + err.Error()})
		return
	}
	if events == nil {
		events = []storage.AlertEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// handleAckAlert acknowledges an alert.
// POST /api/v1/alerts/{id}/ack
func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseAlertID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, err := parseAlertActor(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	found, err := s.store.AcknowledgeAlert(alertID, actor, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to acknowledge alert: " + err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found or already resolved"})
		return
	}
	alert, _ := s.store.GetAlert(alertID, now)
	writeJSON(w, http.StatusOK, alert)
}

// handleUnackAlert clears acknowledgment for an alert.
// POST /api/v1/alerts/{id}/unack
func (s *Server) handleUnackAlert(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseAlertID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, err := parseAlertActor(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	found, err := s.store.UnacknowledgeAlert(alertID, actor, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear acknowledgment: " + err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found or already resolved"})
		return
	}
	alert, _ := s.store.GetAlert(alertID, now)
	writeJSON(w, http.StatusOK, alert)
}

// handleSnoozeAlert snoozes notifications for an alert until a timestamp.
// POST /api/v1/alerts/{id}/snooze
func (s *Server) handleSnoozeAlert(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseAlertID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer r.Body.Close()

	var req struct {
		Until string `json:"until"`
		Actor string `json:"actor"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(req.Until))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "until must be RFC3339"})
		return
	}
	now := time.Now().UTC()
	until = until.UTC()
	if !until.After(now) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "until must be in the future"})
		return
	}
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "manual"
	}
	found, err := s.store.SnoozeAlert(alertID, until, actor, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to snooze alert: " + err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found or already resolved"})
		return
	}
	alert, _ := s.store.GetAlert(alertID, now)
	writeJSON(w, http.StatusOK, alert)
}

// handleUnsnoozeAlert clears alert snooze state.
// POST /api/v1/alerts/{id}/unsnooze
func (s *Server) handleUnsnoozeAlert(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseAlertID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, err := parseAlertActor(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	found, err := s.store.UnsnoozeAlert(alertID, actor, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unsnooze alert: " + err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found or already resolved"})
		return
	}
	alert, _ := s.store.GetAlert(alertID, now)
	writeJSON(w, http.StatusOK, alert)
}

// ---------- Page handlers ----------

// handleSettingsPage serves the settings HTML page.
// GET /settings
func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(SettingsPage))
}

// handleDiskPage serves the disk detail HTML page.
// GET /disk/{serial}
func (s *Server) handleDiskPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(DiskDetailPage))
}

// ---------- Internal helpers ----------

func parseAlertID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	if raw == "" {
		return 0, fmt.Errorf("alert id is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid alert id")
	}
	return id, nil
}

func parseAlertActor(r *http.Request) (string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("failed to read request body")
	}
	defer r.Body.Close()
	if len(strings.TrimSpace(string(body))) == 0 {
		return "manual", nil
	}
	var req struct {
		Actor string `json:"actor"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("invalid JSON")
	}
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "manual"
	}
	return actor, nil
}

// latestSnapshot retrieves the most recent snapshot, preferring the in-memory
// scheduler copy and falling back to the database.
func (s *Server) latestSnapshot() *internal.Snapshot {
	if s.scheduler != nil {
		if snap := s.scheduler.Latest(); snap != nil {
			return snap
		}
	}
	snap, _ := s.store.GetLatestSnapshot()
	return snap
}
