package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"fmt"
	"path/filepath"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/logfwd"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// ---------- Settings types ----------

// Settings represents the user-configurable application settings stored in the DB.
type Settings struct {
	SettingsVersion   int                     `json:"settings_version"`
	ScanInterval      string                  `json:"scan_interval"`
	SpeedTestInterval string                  `json:"speedtest_interval,omitempty"` // e.g. "4h", "1h", "30m"
	SpeedTestSchedule []string                `json:"speedtest_schedule,omitempty"` // specific times: ["03:00"]
	SpeedTestDay      string                  `json:"speedtest_day,omitempty"`      // "monday"-"sunday" or "1","15" for monthly
	Theme             string                  `json:"theme"`
	Icon              string                  `json:"icon"`
	Notifications     SettingsNotifications   `json:"notifications"`
	ServiceChecks     SettingsServiceChecks   `json:"service_checks"`
	LogPush           SettingsLogForward      `json:"log_push"`
	Retention         RetentionSettings       `json:"retention"`
	Backup            BackupSettings          `json:"backup"`
	Sections          DashboardSections       `json:"sections"`
	Proxmox           SettingsProxmox         `json:"proxmox"`
	Kubernetes        SettingsKubernetes      `json:"kubernetes"`
	APIKey            string                  `json:"api_key,omitempty"` // Instance API key for fleet auth
	Fleet             []internal.RemoteServer `json:"fleet,omitempty"`
	DismissedFindings []string                `json:"dismissed_findings,omitempty"`
	CostPerTB         float64                 `json:"cost_per_tb,omitempty"`     // Drive replacement cost per TB (user's currency)
	ChartRangeHours   int                     `json:"chart_range_hours"`         // Persisted chart time range (1, 24, 168)
	SectionHeights    map[string]int          `json:"section_heights,omitempty"` // Persisted section resize heights (section name → px)
	SectionOrder      map[string][]string     `json:"section_order,omitempty"`   // Persisted drag-and-drop column order ({"cols": [["findings","docker"], ...]})
}

const currentSettingsVersion = 1

// DashboardSections controls which sections appear on the dashboard.
// All default to true (visible). Users can hide sections they don't use.
type DashboardSections struct {
	Findings         bool `json:"findings"`
	DiskSpace        bool `json:"disk_space"`
	SMART            bool `json:"smart"`
	Docker           bool `json:"docker"`
	ZFS              bool `json:"zfs"`
	UPS              bool `json:"ups"`
	Parity           bool `json:"parity"`
	Network          bool `json:"network"`
	Tunnels          bool `json:"tunnels"`
	Proxmox          bool `json:"proxmox"`
	Kubernetes       bool `json:"kubernetes"`
	GPU              bool `json:"gpu"`
	ContainerMetrics bool `json:"container_metrics"`
	MergedContainers bool `json:"merged_containers"` // Combine Docker list + container metrics into one section
	MergedDrives     bool `json:"merged_drives"`     // Combine SMART + storage into one card per drive
	Backup           bool `json:"backup"`
	SpeedTest        bool `json:"speed_test"`
	Processes        bool `json:"processes"`
	DashColumns      int  `json:"dash_columns"` // Dashboard column count: 0=auto (default 2), 1, 2, 3, 4
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

// SettingsNotifications holds the webhook list and notification rules.
type SettingsNotifications struct {
	Webhooks           []internal.WebhookConfig      `json:"webhooks"`
	Rules              []internal.NotificationRule   `json:"rules,omitempty"`
	Policies           []scheduler.AlertPolicy       `json:"policies,omitempty"` // legacy — kept for migration
	QuietHours         scheduler.QuietHours          `json:"quiet_hours,omitempty"`
	MaintenanceWindows []scheduler.MaintenanceWindow `json:"maintenance_windows,omitempty"`
	DefaultCooldownSec int                           `json:"default_cooldown_sec,omitempty"`
}

// SettingsServiceChecks holds configured service checks.
type SettingsServiceChecks struct {
	Checks []internal.ServiceCheckConfig `json:"checks"`
}

// SettingsProxmox holds the Proxmox VE API connection settings.
type SettingsProxmox struct {
	Enabled  bool   `json:"enabled"`
	URL      string `json:"url"`       // e.g. https://192.168.1.10:8006
	TokenID  string `json:"token_id"`  // e.g. root@pam!nas-doctor
	Secret   string `json:"secret"`    // API token UUID secret
	NodeName string `json:"node_name"` // optional: real PVE node name to filter (auto-detected)
	Alias    string `json:"alias"`     // optional: friendly display name
}

// SettingsKubernetes holds the Kubernetes cluster connection settings.
type SettingsKubernetes struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url"`        // e.g. https://192.168.1.10:6443
	Token     string `json:"token"`      // bearer token
	Alias     string `json:"alias"`      // friendly display name
	InCluster bool   `json:"in_cluster"` // auto-detect from mounted service account
}

// SettingsLogForward holds the log-forwarding configuration within settings.
type SettingsLogForward struct {
	Enabled      bool                    `json:"enabled"`
	Destinations []LogForwardDestination `json:"destinations"`
}

// LogForwardDestination represents a single log-forwarding target.
type LogForwardDestination struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // loki, http_json, syslog
	URL     string            `json:"url"`  // endpoint URL (Loki push, HTTP endpoint, syslog host:port)
	Enabled bool              `json:"enabled"`
	Headers map[string]string `json:"headers,omitempty"` // custom HTTP headers (auth tokens, etc.)
	Labels  map[string]string `json:"labels,omitempty"`  // Loki labels / metadata tags
	Format  string            `json:"format,omitempty"`  // full (default), findings_only, summary
}

const settingsConfigKey = "settings"

// defaultSettings returns the default settings used when none are persisted.
func defaultSettings() Settings {
	return Settings{
		SettingsVersion: currentSettingsVersion,
		ScanInterval:    "30m",
		Theme:           ThemeMidnight,
		Icon:            "icon3",
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
		ServiceChecks: SettingsServiceChecks{
			Checks: []internal.ServiceCheckConfig{},
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
			Findings:         true,
			DiskSpace:        true,
			SMART:            true,
			Docker:           true,
			ZFS:              true,
			UPS:              true,
			Parity:           true,
			Network:          true,
			Tunnels:          true,
			GPU:              true,
			ContainerMetrics: false,
			MergedContainers: true,
			MergedDrives:     true,
			Backup:           true,
			SpeedTest:        true,
		},
		ChartRangeHours: 1,
	}
}

// ---------- Route registration ----------

// RegisterExtendedRoutes registers additional API and page routes on the given router.
func (s *Server) RegisterExtendedRoutes(r chi.Router) {
	// API endpoints (registered directly, not as a sub-Route)
	r.Get("/api/v1/settings", s.handleGetSettings)
	r.Put("/api/v1/settings", s.handleUpdateSettings)
	r.Post("/api/v1/settings/test-webhook", s.handleTestWebhook)
	r.Put("/api/v1/settings/chart-range", s.handleSetChartRange)
	r.Put("/api/v1/settings/section-heights", s.handleSetSectionHeights)
	r.Put("/api/v1/settings/section-order", s.handleSetSectionOrder)
	r.Get("/api/v1/disks", s.handleListDisks)
	r.Get("/api/v1/disks/{serial}", s.handleGetDisk)
	r.Get("/api/v1/history/system", s.handleSystemHistory)
	r.Get("/api/v1/history/gpu", s.handleGPUHistory)
	r.Get("/api/v1/history/containers", s.handleContainerHistory)
	r.Get("/api/v1/history/speedtest", s.handleSpeedTestHistory)
	r.Get("/api/v1/notifications/log", s.handleNotificationLog)
	r.Get("/api/v1/service-checks", s.handleServiceChecks)
	r.Get("/api/v1/service-checks/history", s.handleServiceCheckHistory)
	r.Post("/api/v1/service-checks/run", s.handleRunServiceChecks)
	r.Delete("/api/v1/service-checks/{key}", s.handleDeleteServiceCheck)
	r.Get("/api/v1/incidents/timeline", s.handleIncidentTimeline)
	r.Get("/api/v1/incidents/correlation", s.handleIncidentCorrelation)
	r.Get("/api/v1/smart/trends", s.handleSMARTTrends)
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

	// Alerts page
	r.Get("/alerts", s.handleAlertsPage)

	// Fleet test
	r.Post("/api/v1/fleet/test", s.handleTestFleetServer)

	// Kubernetes test
	r.Post("/api/v1/kubernetes/test", s.handleTestKubernetes)

	// Proxmox test
	r.Post("/api/v1/proxmox/test", s.handleTestProxmox)

	// Service Checks page
	r.Get("/service-checks", s.handleServiceChecksPage)

	// Parity page
	r.Get("/parity", s.handleParityPage)

	// Stats page
	r.Get("/stats", s.handleStatsPage)
	r.Post("/api/v1/backup", s.handleCreateBackup)
	r.Get("/api/v1/backup", s.handleListBackups)
	r.Get("/api/v1/db/stats", s.handleDBStats)
	r.Get("/api/v1/sparklines", s.handleSparklines)

	// Replacement planner
	r.Get("/api/v1/replacement-plan", s.handleReplacementPlan)
	r.Get("/replacement-planner", s.handleReplacementPlannerPage)

	// Capacity forecast
	r.Get("/api/v1/capacity-forecast", s.handleCapacityForecast)
	r.Get("/api/v1/disk-usage-history", s.handleDiskUsageHistory)

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
	if settings.ServiceChecks.Checks == nil {
		settings.ServiceChecks.Checks = []internal.ServiceCheckConfig{}
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
			Tunnels: true, MergedDrives: true,
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
		settings.ScanInterval = "30m"
	}
	if _, err := time.ParseDuration(settings.ScanInterval); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scan_interval: " + err.Error()})
		return
	}
	if settings.Theme == "" {
		settings.Theme = DefaultTheme
	}
	switch settings.Theme {
	case ThemeMidnight, ThemeClean:
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
	if settings.ServiceChecks.Checks == nil {
		settings.ServiceChecks.Checks = []internal.ServiceCheckConfig{}
	}
	serviceNames := make(map[string]struct{}, len(settings.ServiceChecks.Checks))
	for i := range settings.ServiceChecks.Checks {
		check := &settings.ServiceChecks.Checks[i]
		check.Name = strings.TrimSpace(check.Name)
		check.Type = strings.ToLower(strings.TrimSpace(check.Type))
		check.Target = strings.TrimSpace(check.Target)
		if check.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service_checks.checks name is required"})
			return
		}
		if check.Target == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service_checks.checks target is required"})
			return
		}
		if _, exists := serviceNames[strings.ToLower(check.Name)]; exists {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "duplicate service check name: " + check.Name})
			return
		}
		serviceNames[strings.ToLower(check.Name)] = struct{}{}
		switch check.Type {
		case internal.ServiceCheckHTTP, internal.ServiceCheckTCP, internal.ServiceCheckDNS, internal.ServiceCheckSMB, internal.ServiceCheckNFS, internal.ServiceCheckPing, internal.ServiceCheckSpeed:
			// valid
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid service check type: " + check.Type})
			return
		}
		if check.IntervalSec <= 0 {
			check.IntervalSec = 300 // default 5 minutes
		}
		if check.IntervalSec < 30 {
			check.IntervalSec = 30 // minimum 30 seconds
		}
		if check.TimeoutSec <= 0 {
			check.TimeoutSec = 5
		}
		if check.TimeoutSec > 30 {
			check.TimeoutSec = 30
		}
		if check.Port < 0 || check.Port > 65535 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service check port must be between 0 and 65535"})
			return
		}
		if check.FailureThreshold <= 0 {
			check.FailureThreshold = 1
		}
		if check.FailureSeverity == "" {
			check.FailureSeverity = internal.SeverityWarning
		}
		switch check.FailureSeverity {
		case internal.SeverityInfo, internal.SeverityWarning, internal.SeverityCritical:
			// valid
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid service check failure_severity"})
			return
		}
		if check.Type == internal.ServiceCheckHTTP {
			if check.ExpectedMin <= 0 {
				check.ExpectedMin = 200
			}
			if check.ExpectedMax <= 0 {
				check.ExpectedMax = 399
			}
			if check.ExpectedMax < check.ExpectedMin {
				check.ExpectedMax = check.ExpectedMin
			}
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
		// Update speed test interval and schedule
		if settings.SpeedTestInterval != "" {
			if d, err := time.ParseDuration(settings.SpeedTestInterval); err == nil {
				s.scheduler.SetSpeedTestInterval(d)
			}
		}
		s.scheduler.SetSpeedTestSchedule(settings.SpeedTestSchedule, settings.SpeedTestDay, settings.SpeedTestInterval)
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
			Rules:              settings.Notifications.Rules,
			Policies:           settings.Notifications.Policies, // legacy compat
			QuietHours:         settings.Notifications.QuietHours,
			MaintenanceWindows: settings.Notifications.MaintenanceWindows,
			DefaultCooldownSec: settings.Notifications.DefaultCooldownSec,
		})
		s.scheduler.UpdateServiceChecks(settings.ServiceChecks.Checks)

		// Auto-enable Kubernetes dashboard section when K8s integration is turned on
		if settings.Kubernetes.Enabled && !settings.Sections.Kubernetes {
			settings.Sections.Kubernetes = true
		}

		// Auto-enable Proxmox dashboard section when PVE integration is turned on
		if settings.Proxmox.Enabled && !settings.Sections.Proxmox {
			settings.Sections.Proxmox = true
		}

		// Update Proxmox config on the collector
		s.collector.SetProxmoxConfig(collector.ProxmoxConfig{
			Enabled:  settings.Proxmox.Enabled,
			URL:      settings.Proxmox.URL,
			TokenID:  settings.Proxmox.TokenID,
			Secret:   settings.Proxmox.Secret,
			NodeName: settings.Proxmox.NodeName,
			Alias:    settings.Proxmox.Alias,
		})

		// Update Kubernetes config on the collector
		s.collector.SetKubeConfig(collector.KubeConfig{
			Enabled:   settings.Kubernetes.Enabled,
			URL:       settings.Kubernetes.URL,
			Token:     settings.Kubernetes.Token,
			Alias:     settings.Kubernetes.Alias,
			InCluster: settings.Kubernetes.InCluster,
		})

		// Update log forwarding
		if settings.LogPush.Enabled && len(settings.LogPush.Destinations) > 0 {
			var dests []logfwd.Destination
			for _, d := range settings.LogPush.Destinations {
				dests = append(dests, logfwd.Destination{
					Name:    d.Name,
					Type:    d.Type,
					URL:     d.URL,
					Enabled: d.Enabled,
					Headers: d.Headers,
					Labels:  d.Labels,
					Format:  d.Format,
				})
			}
			s.scheduler.UpdateLogForwarder(dests)
		} else {
			s.scheduler.UpdateLogForwarder(nil)
		}
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
// Runs one-time migrations when settings_version is behind currentSettingsVersion.
func (s *Server) getSettings() Settings {
	settings := defaultSettings()
	if raw, err := s.store.GetConfig(settingsConfigKey); err == nil && raw != "" {
		json.Unmarshal([]byte(raw), &settings)
		if settings.SettingsVersion < currentSettingsVersion {
			// v0 → v1: merged_drives defaults to true
			if settings.SettingsVersion < 1 {
				settings.Sections.MergedDrives = true
			}
			settings.SettingsVersion = currentSettingsVersion
			if data, err := json.Marshal(settings); err == nil {
				s.store.SetConfig(settingsConfigKey, string(data))
			}
		}
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

	var payload struct {
		internal.WebhookConfig
		Finding *internal.Finding `json:"finding,omitempty"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	wh := payload.WebhookConfig
	if wh.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "webhook url is required"})
		return
	}
	// Force enabled for the test
	wh.Enabled = true

	var testFindings []internal.Finding
	if payload.Finding != nil && payload.Finding.Title != "" {
		// Use the caller-provided finding (from notification rule test)
		f := *payload.Finding
		f.ID = "test-rule-001"
		if f.Severity == "" {
			f.Severity = internal.SeverityWarning
		}
		testFindings = []internal.Finding{f}
	} else {
		// Default generic test finding (from webhook test button)
		testFindings = []internal.Finding{
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
			},
		}
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

// handleGPUHistory returns historical GPU metrics for chart rendering.
// GET /api/v1/history/gpu?hours=1 (default 1, accepts 1/24/168)
func (s *Server) handleGPUHistory(w http.ResponseWriter, r *http.Request) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 1
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}
	if hours > 720 { // cap at 30 days
		hours = 720
	}
	points, err := s.store.GetGPUHistory(hours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get GPU history: " + err.Error()})
		return
	}
	if points == nil {
		points = []storage.GPUHistoryPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}

// handleContainerHistory returns per-container resource metrics history.
// GET /api/v1/history/containers?hours=N
func (s *Server) handleContainerHistory(w http.ResponseWriter, r *http.Request) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 1
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}
	if hours > 720 { // cap at 30 days
		hours = 720
	}
	points, err := s.store.GetContainerHistory(hours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get container history: " + err.Error()})
		return
	}
	if points == nil {
		points = []storage.ContainerHistoryPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}

// handleSpeedTestHistory returns speed test history for chart rendering.
// GET /api/v1/history/speedtest?hours=N
func (s *Server) handleSpeedTestHistory(w http.ResponseWriter, r *http.Request) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 1
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}
	if hours > 720 { // cap at 30 days
		hours = 720
	}
	points, err := s.store.GetSpeedTestHistory(hours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get speed test history: " + err.Error()})
		return
	}
	if points == nil {
		points = []storage.SpeedTestHistoryPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}

// handleSetChartRange persists the user's preferred chart time range (1, 24, 168 hours).
// PUT /api/v1/settings/chart-range
func (s *Server) handleSetChartRange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hours int `json:"hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	// Validate — only allow known ranges
	switch body.Hours {
	case 1, 24, 168:
		// ok
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hours must be 1, 24, or 168"})
		return
	}
	settings := s.getSettings()
	settings.ChartRangeHours = body.Hours
	data, err := json.Marshal(settings)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal settings"})
		return
	}
	if err := s.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"chart_range_hours": body.Hours})
}

// handleSetSectionHeights persists user's section resize heights.
// PUT /api/v1/settings/section-heights
func (s *Server) handleSetSectionHeights(w http.ResponseWriter, r *http.Request) {
	var body map[string]int
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	// Sanitize: only keep positive heights, cap at 2000px
	clean := make(map[string]int, len(body))
	for k, v := range body {
		if v > 0 && v < 2000 && len(k) < 64 {
			clean[k] = v
		}
	}
	settings := s.getSettings()
	settings.SectionHeights = clean
	data, err := json.Marshal(settings)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal settings"})
		return
	}
	if err := s.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, clean)
}

// handleSetSectionOrder persists the dashboard drag-and-drop section order.
// PUT /api/v1/settings/section-order
func (s *Server) handleSetSectionOrder(w http.ResponseWriter, r *http.Request) {
	var body map[string][]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	settings := s.getSettings()
	settings.SectionOrder = body
	data, err := json.Marshal(settings)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal settings"})
		return
	}
	if err := s.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, body)
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

// ---------- Service checks handlers ----------

// handleServiceChecks returns latest status per configured service check.
// GET /api/v1/service-checks
func (s *Server) handleServiceChecks(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = parsed
	}

	entries, err := s.store.ListLatestServiceChecks(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list service checks: " + err.Error()})
		return
	}
	if entries == nil {
		entries = []storage.ServiceCheckEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleServiceCheckHistory returns recent status history for a check key.
// GET /api/v1/service-checks/history?key=<service_check_key>&limit=100
func (s *Server) handleServiceCheckHistory(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param 'key' is required"})
		return
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = parsed
	}

	history, err := s.store.GetServiceCheckHistory(key, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load service check history: " + err.Error()})
		return
	}
	if history == nil {
		history = []storage.ServiceCheckEntry{}
	}
	writeJSON(w, http.StatusOK, history)
}

// handleRunServiceChecks triggers immediate service check execution.
// POST /api/v1/service-checks/run
func (s *Server) handleRunServiceChecks(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service checks unavailable in demo mode"})
		return
	}
	results, err := s.scheduler.RunServiceChecksNow()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "service checks failed: " + err.Error()})
		return
	}
	if results == nil {
		results = []internal.ServiceCheckResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

// handleDeleteServiceCheck removes all history for a service check key.
// DELETE /api/v1/service-checks/{key}
func (s *Server) handleDeleteServiceCheck(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}
	deleted, err := s.store.DeleteServiceCheckByKey(key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": deleted, "key": key})
}

type incidentTimelineEvent struct {
	Time     string `json:"time"`
	Type     string `json:"type"`
	Severity string `json:"severity,omitempty"`
	Status   string `json:"status,omitempty"`
	Title    string `json:"title,omitempty"`
	AlertID  int64  `json:"alert_id,omitempty"`
	Source   string `json:"source,omitempty"`
	Details  string `json:"details,omitempty"`
}

// handleIncidentTimeline returns alert/notification timeline events and downsampled metrics.
// GET /api/v1/incidents/timeline?from=<rfc3339>&to=<rfc3339>&severity=<critical|warning|info>&points=400
func (s *Server) handleIncidentTimeline(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour)
	to := now
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid from timestamp"})
			return
		}
		from = parsed.UTC()
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid to timestamp"})
			return
		}
		to = parsed.UTC()
	}
	if to.Before(from) {
		from, to = to, from
	}

	severityFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("severity")))
	switch severityFilter {
	case "", string(internal.SeverityCritical), string(internal.SeverityWarning), string(internal.SeverityInfo):
		// valid
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid severity filter"})
		return
	}

	points := 400
	if raw := strings.TrimSpace(r.URL.Query().Get("points")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 10 || parsed > 5000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid points value"})
			return
		}
		points = parsed
	}

	eventLimit := 1000
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 10000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit value"})
			return
		}
		eventLimit = parsed
	}

	alerts, err := s.store.ListAlerts("", 5000, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list alerts: " + err.Error()})
		return
	}
	notifications, err := s.store.GetNotificationLogRange(from, to, eventLimit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list notifications: " + err.Error()})
		return
	}
	system, err := s.store.GetSystemHistoryRange(from, to, points)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load system history: " + err.Error()})
		return
	}
	temps, err := s.store.GetAverageDiskTemperatureRange(from, to, points)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load disk temperature history: " + err.Error()})
		return
	}

	type sortableEvent struct {
		when  time.Time
		event incidentTimelineEvent
	}
	events := make([]sortableEvent, 0, len(alerts)*2+len(notifications))
	for _, alert := range alerts {
		if severityFilter != "" && strings.ToLower(alert.Severity) != severityFilter {
			continue
		}
		created, err := time.Parse(time.RFC3339, alert.CreatedAt)
		if err == nil && !created.Before(from) && !created.After(to) {
			events = append(events, sortableEvent{
				when: created,
				event: incidentTimelineEvent{
					Time:     created.UTC().Format(time.RFC3339),
					Type:     "alert_opened",
					Severity: alert.Severity,
					Status:   string(alert.Status),
					Title:    alert.Title,
					AlertID:  alert.ID,
					Source:   "alerts",
				},
			})
		}
		if alert.ResolvedAt != "" {
			if resolved, err := time.Parse(time.RFC3339, alert.ResolvedAt); err == nil && !resolved.Before(from) && !resolved.After(to) {
				events = append(events, sortableEvent{
					when: resolved,
					event: incidentTimelineEvent{
						Time:     resolved.UTC().Format(time.RFC3339),
						Type:     "alert_resolved",
						Severity: alert.Severity,
						Status:   string(alert.Status),
						Title:    alert.Title,
						AlertID:  alert.ID,
						Source:   "alerts",
					},
				})
			}
		}
	}
	for _, n := range notifications {
		if n.CreatedAt.Before(from) || n.CreatedAt.After(to) {
			continue
		}
		events = append(events, sortableEvent{
			when: n.CreatedAt,
			event: incidentTimelineEvent{
				Time:    n.CreatedAt.UTC().Format(time.RFC3339),
				Type:    "notification",
				Status:  n.Status,
				Source:  n.WebhookName,
				Details: n.ErrorMessage,
			},
		})
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].when.After(events[j].when)
	})
	if len(events) > eventLimit {
		events = events[:eventLimit]
	}
	outEvents := make([]incidentTimelineEvent, 0, len(events))
	for _, event := range events {
		outEvents = append(outEvents, event.event)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"from":              from.UTC().Format(time.RFC3339),
		"to":                to.UTC().Format(time.RFC3339),
		"severity":          severityFilter,
		"events":            outEvents,
		"system_metrics":    system,
		"avg_disk_temp":     temps,
		"downsample_points": points,
	})
}

type correlationMetricWindow struct {
	Samples   int     `json:"samples"`
	AvgCPU    float64 `json:"avg_cpu"`
	AvgMemory float64 `json:"avg_memory"`
	AvgIOWait float64 `json:"avg_io_wait"`
	AvgDiskT  float64 `json:"avg_disk_temp"`
}

// handleIncidentCorrelation returns metric shifts around an alert timestamp.
// GET /api/v1/incidents/correlation?alert_id=123&window_hours=24
func (s *Server) handleIncidentCorrelation(w http.ResponseWriter, r *http.Request) {
	alertIDRaw := strings.TrimSpace(r.URL.Query().Get("alert_id"))
	if alertIDRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "alert_id is required"})
		return
	}
	alertID, err := strconv.ParseInt(alertIDRaw, 10, 64)
	if err != nil || alertID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid alert_id"})
		return
	}

	windowHours := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("window_hours")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 168 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid window_hours"})
			return
		}
		windowHours = parsed
	}

	now := time.Now().UTC()
	alert, err := s.store.GetAlert(alertID, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load alert: " + err.Error()})
		return
	}
	if alert == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}
	alertTime, err := time.Parse(time.RFC3339, alert.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "alert timestamp is invalid"})
		return
	}

	window := time.Duration(windowHours) * time.Hour
	duringWindow := window / 4
	if duringWindow > 2*time.Hour {
		duringWindow = 2 * time.Hour
	}
	if duringWindow < 30*time.Minute {
		duringWindow = 30 * time.Minute
	}

	from := alertTime.Add(-window)
	to := alertTime.Add(window)
	system, err := s.store.GetSystemHistoryRange(from, to, 400)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load system metrics: " + err.Error()})
		return
	}
	temps, err := s.store.GetAverageDiskTemperatureRange(from, to, 400)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load temperature metrics: " + err.Error()})
		return
	}

	beforeStart := alertTime.Add(-window)
	beforeEnd := alertTime
	duringStart := alertTime
	duringEnd := alertTime.Add(duringWindow)
	afterStart := duringEnd
	afterEnd := alertTime.Add(window)

	before := summarizeMetrics(system, temps, beforeStart, beforeEnd)
	during := summarizeMetrics(system, temps, duringStart, duringEnd)
	after := summarizeMetrics(system, temps, afterStart, afterEnd)

	writeJSON(w, http.StatusOK, map[string]any{
		"alert":          alert,
		"window_hours":   windowHours,
		"reference_time": alertTime.UTC().Format(time.RFC3339),
		"system_metrics": system,
		"avg_disk_temp":  temps,
		"before":         before,
		"during":         during,
		"after":          after,
		"before_range":   []string{beforeStart.UTC().Format(time.RFC3339), beforeEnd.UTC().Format(time.RFC3339)},
		"during_range":   []string{duringStart.UTC().Format(time.RFC3339), duringEnd.UTC().Format(time.RFC3339)},
		"after_range":    []string{afterStart.UTC().Format(time.RFC3339), afterEnd.UTC().Format(time.RFC3339)},
	})
}

func summarizeMetrics(system []storage.SystemHistoryPoint, temps []storage.NumericPoint, start, end time.Time) correlationMetricWindow {
	var out correlationMetricWindow
	if end.Before(start) {
		start, end = end, start
	}
	var cpu, mem, io float64
	for _, point := range system {
		if point.Timestamp.Before(start) || !point.Timestamp.Before(end) {
			continue
		}
		out.Samples++
		cpu += point.CPUUsage
		mem += point.MemPercent
		io += point.IOWait
	}
	if out.Samples > 0 {
		out.AvgCPU = cpu / float64(out.Samples)
		out.AvgMemory = mem / float64(out.Samples)
		out.AvgIOWait = io / float64(out.Samples)
	}

	var tempSamples int
	var tempSum float64
	for _, point := range temps {
		if point.Timestamp.Before(start) || !point.Timestamp.Before(end) {
			continue
		}
		tempSamples++
		tempSum += point.Value
	}
	if tempSamples > 0 {
		out.AvgDiskT = tempSum / float64(tempSamples)
	}
	return out
}

type smartTrendDrive struct {
	Serial               string  `json:"serial"`
	Device               string  `json:"device"`
	Model                string  `json:"model"`
	Points               int     `json:"points"`
	DaysSpan             float64 `json:"days_span"`
	CurrentTemp          int     `json:"current_temp"`
	TempDelta            int     `json:"temp_delta"`
	CurrentReallocated   int64   `json:"current_reallocated"`
	ReallocatedDelta     int64   `json:"reallocated_delta"`
	CurrentPending       int64   `json:"current_pending"`
	PendingDelta         int64   `json:"pending_delta"`
	CurrentCRC           int64   `json:"current_crc"`
	CRCDelta             int64   `json:"crc_delta"`
	RiskScore            int     `json:"risk_score"`
	Urgency              string  `json:"urgency"`
	Confidence           string  `json:"confidence"`
	Worsening            bool    `json:"worsening"`
	ReplacementCandidate bool    `json:"replacement_candidate"`
	Recommendation       string  `json:"recommendation"`
}

// handleSMARTTrends returns predictive trend summaries for drives and parity.
// GET /api/v1/smart/trends
func (s *Server) handleSMARTTrends(w http.ResponseWriter, r *http.Request) {
	disks, err := s.store.ListDisks()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list disks: " + err.Error()})
		return
	}

	trends := make([]smartTrendDrive, 0, len(disks))
	for _, disk := range disks {
		history, err := s.store.GetDiskHistory(disk.Serial, 120)
		if err != nil || len(history) < 2 {
			continue
		}
		first := history[0]
		last := history[len(history)-1]
		days := last.Timestamp.Sub(first.Timestamp).Hours() / 24.0
		if days < 1 {
			days = 1
		}

		reallocDelta := last.Reallocated - first.Reallocated
		pendingDelta := last.Pending - first.Pending
		crcDelta := last.UDMACRC - first.UDMACRC
		tempDelta := last.Temperature - first.Temperature

		riskScore := 0
		if last.Pending > 0 {
			riskScore += 40
		}
		if pendingDelta > 0 {
			riskScore += 25
		}
		if reallocDelta > 0 {
			riskScore += int(minInt64(reallocDelta, 20))
		}
		if crcDelta > 5 {
			riskScore += 10
		}
		if last.Temperature >= 50 {
			riskScore += 15
		} else if last.Temperature >= 45 {
			riskScore += 8
		}
		if tempDelta >= 4 {
			riskScore += 10
		}

		urgency := "monitor"
		if riskScore >= 70 {
			urgency = "immediate"
		} else if riskScore >= 40 {
			urgency = "short-term"
		}
		confidence := "low"
		if len(history) >= 10 && days >= 7 {
			confidence = "high"
		} else if len(history) >= 5 && days >= 3 {
			confidence = "medium"
		}

		worsening := reallocDelta > 0 || pendingDelta > 0 || crcDelta > 0 || (tempDelta >= 4 && last.Temperature >= 45)
		replacementCandidate := urgency == "immediate" || last.Pending > 0 || reallocDelta >= 20

		recommendation := "Continue monitoring trend slope and keep backups verified."
		if urgency == "short-term" {
			recommendation = "Plan replacement window and inspect cabling/power path to reduce progression risk."
		}
		if urgency == "immediate" {
			recommendation = "Prepare immediate drive replacement and verify restore path before failure escalates."
		}

		trends = append(trends, smartTrendDrive{
			Serial:               disk.Serial,
			Device:               disk.Device,
			Model:                disk.Model,
			Points:               len(history),
			DaysSpan:             days,
			CurrentTemp:          last.Temperature,
			TempDelta:            tempDelta,
			CurrentReallocated:   last.Reallocated,
			ReallocatedDelta:     reallocDelta,
			CurrentPending:       last.Pending,
			PendingDelta:         pendingDelta,
			CurrentCRC:           last.UDMACRC,
			CRCDelta:             crcDelta,
			RiskScore:            riskScore,
			Urgency:              urgency,
			Confidence:           confidence,
			Worsening:            worsening,
			ReplacementCandidate: replacementCandidate,
			Recommendation:       recommendation,
		})
	}

	sort.Slice(trends, func(i, j int) bool {
		if trends[i].RiskScore == trends[j].RiskScore {
			return trends[i].CurrentPending > trends[j].CurrentPending
		}
		return trends[i].RiskScore > trends[j].RiskScore
	})

	var paritySummary map[string]any
	if snap := s.latestSnapshot(); snap != nil && snap.Parity != nil {
		history := snap.Parity.History
		if len(history) > 0 {
			recentErrors := 0
			startIdx := len(history) - 3
			if startIdx < 0 {
				startIdx = 0
			}
			for i := startIdx; i < len(history); i++ {
				recentErrors += history[i].Errors
			}

			degradationPct := 0.0
			if len(history) >= 2 {
				first := history[0]
				last := history[len(history)-1]
				if first.SpeedMBs > 0 && last.SpeedMBs > 0 {
					degradationPct = (first.SpeedMBs - last.SpeedMBs) / first.SpeedMBs * 100.0
				}
			}

			risk := "stable"
			if recentErrors > 0 {
				risk = "critical"
			} else if degradationPct > 30 {
				risk = "warning"
			}

			last := history[len(history)-1]
			paritySummary = map[string]any{
				"checks":                len(history),
				"recent_errors":         recentErrors,
				"speed_degradation_pct": degradationPct,
				"latest_speed_mb_s":     last.SpeedMBs,
				"latest_date":           last.Date,
				"risk":                  risk,
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"drives":       trends,
		"parity":       paritySummary,
	})
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

// handleAlertsPage serves the alerts HTML page.
// GET /alerts
func (s *Server) handleAlertsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(alertsPageHTML))
}

// handleTestFleetServer tests connectivity to a remote NAS Doctor instance.
// POST /api/v1/fleet/test
func (s *Server) handleTestFleetServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string            `json:"url"`
		APIKey  string            `json:"api_key"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL is required"})
		return
	}
	baseURL := strings.TrimRight(req.URL, "/")
	client := &http.Client{Timeout: 10 * time.Second}

	// Build request headers helper
	buildReq := func(path string) (*http.Request, error) {
		r, err := http.NewRequest("GET", baseURL+path, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Set("User-Agent", "nas-doctor-fleet-test/1.0")
		if req.APIKey != "" {
			r.Header.Set("Authorization", "Bearer "+req.APIKey)
		}
		for k, v := range req.Headers {
			r.Header.Set(k, v)
		}
		return r, nil
	}

	// Primary test: hit /api/v1/status (requires API key when set)
	statusReq, err := buildReq("/api/v1/status")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": "invalid URL: " + err.Error()})
		return
	}
	resp, err := client.Do(statusReq)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	// Handle 401 — instance has API key, we don't have it (or wrong key)
	if resp.StatusCode == 401 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": "Authentication failed — this instance requires a valid API key."})
		return
	}
	if resp.StatusCode != 200 {
		// Try health endpoint to distinguish "not NAS Doctor" from "auth issue"
		if healthReq, err := buildReq("/api/v1/health"); err == nil {
			if hResp, err := client.Do(healthReq); err == nil {
				defer hResp.Body.Close()
				var h struct {
					NasDoc bool `json:"nas_doctor"`
				}
				json.NewDecoder(hResp.Body).Decode(&h)
				if !h.NasDoc {
					writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": "Response does not contain NAS Doctor signature — check URL and auth headers."})
					return
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": fmt.Sprintf("HTTP %d from %s", resp.StatusCode, baseURL+"/api/v1/status")})
		return
	}

	// Parse status response
	var st struct {
		Hostname string `json:"hostname"`
		Platform string `json:"platform"`
		Version  string `json:"version"`
		Uptime   string `json:"uptime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": "Response is not valid JSON — likely a proxy login page. Add the required auth headers."})
		return
	}
	if st.Hostname == "" && st.Platform == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": "Response does not look like a NAS Doctor instance — check URL and auth headers."})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"hostname": st.Hostname,
		"platform": st.Platform,
		"version":  st.Version,
		"uptime":   st.Uptime,
	})
}

// handleTestKubernetes tests the Kubernetes API connection.
// POST /api/v1/kubernetes/test
func (s *Server) handleTestKubernetes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL       string `json:"url"`
		Token     string `json:"token"`
		InCluster bool   `json:"in_cluster"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	cfg := collector.KubeConfig{Enabled: true, URL: req.URL, Token: req.Token, InCluster: req.InCluster}
	result := collector.CollectKubernetes(cfg)
	if result == nil || !result.Connected {
		errMsg := "failed to connect"
		if result != nil && result.Error != "" {
			errMsg = result.Error
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": errMsg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"version":     result.Version,
		"platform":    result.Platform,
		"nodes":       len(result.Nodes),
		"pods":        len(result.Pods),
		"namespaces":  len(result.Namespaces),
		"deployments": len(result.Deployments),
	})
}

// handleTestProxmox tests the Proxmox VE API connection.
// POST /api/v1/proxmox/test
func (s *Server) handleTestProxmox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string `json:"url"`
		TokenID string `json:"token_id"`
		Secret  string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.URL == "" || req.TokenID == "" || req.Secret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL, token ID, and secret are required"})
		return
	}
	cfg := collector.ProxmoxConfig{Enabled: true, URL: req.URL, TokenID: req.TokenID, Secret: req.Secret}
	result := collector.CollectProxmox(cfg)
	if result == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": "failed to connect"})
		return
	}
	nodeNames := make([]string, 0, len(result.Nodes))
	for _, n := range result.Nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"version":    result.Version,
		"cluster":    result.ClusterName,
		"nodes":      len(result.Nodes),
		"node_names": nodeNames,
		"guests":     len(result.Guests),
		"storage":    len(result.Storage),
	})
}

// handleServiceChecksPage serves the service checks HTML page.
// GET /service-checks
func (s *Server) handleServiceChecksPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(serviceChecksPageHTML))
}

// handleParityPage serves the parity history HTML page.
// GET /parity
func (s *Server) handleParityPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(parityPageHTML))
}

// handleDiskPage serves the disk detail HTML page.
// GET /disk/{serial}
func (s *Server) handleDiskPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(DiskDetailPage))
}

// ---------- Internal helpers ----------

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

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
