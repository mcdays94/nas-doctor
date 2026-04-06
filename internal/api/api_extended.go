package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"path/filepath"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// ---------- Settings types ----------

// Settings represents the user-configurable application settings stored in the DB.
type Settings struct {
	ScanInterval  string                `json:"scan_interval"`
	Theme         string                `json:"theme"`
	Notifications SettingsNotifications `json:"notifications"`
	LogPush       SettingsLogForward    `json:"log_push"`
	Retention     RetentionSettings     `json:"retention"`
	Backup        BackupSettings        `json:"backup"`
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
	Webhooks []internal.WebhookConfig `json:"webhooks"`
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
		Notifications: SettingsNotifications{
			Webhooks: []internal.WebhookConfig{},
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

// ---------- Page handlers ----------

// SettingsPage is the self-contained HTML settings page (Midnight theme).
var SettingsPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>NAS Doctor — Settings</title>
<link rel="icon" type="image/png" href="/icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
/* Midnight theme (default) */
:root, body.theme-midnight{
  --bg:#0f1011;--surface:#191a1b;--text:#f7f8f8;--text2:#8a8f98;
  --accent:#5e6ad2;--accent-hover:#7170ff;--success:#27a644;--error:#dc2626;
  --border:rgba(255,255,255,0.08);--radius:8px;
  --input-bg:rgba(255,255,255,0.04);--code-bg:rgba(255,255,255,0.06);
}
/* Clean theme */
body.theme-clean{
  --bg:#ffffff;--surface:#ffffff;--text:#171717;--text2:#808080;
  --accent:#171717;--accent-hover:#404040;--success:#16a34a;--error:#dc2626;
  --border:rgba(0,0,0,0.08);--radius:8px;
  --input-bg:rgba(0,0,0,0.03);--code-bg:rgba(0,0,0,0.04);
}
body.theme-clean a{color:#171717}
body.theme-clean a:hover{color:#404040}
body.theme-clean .btn-primary{background:#171717;border-color:#171717}
body.theme-clean .btn-primary:hover{background:#333}
body.theme-clean .btn-secondary{background:#fff;color:#4d4d4d;border-color:rgba(0,0,0,0.15)}
body.theme-clean .btn-secondary:hover{color:#171717;background:#fafafa;border-color:rgba(0,0,0,0.2)}
body.theme-clean .btn-danger{background:rgba(220,38,38,0.06);border-color:rgba(220,38,38,0.15)}
body.theme-clean .btn-success{background:rgba(22,163,74,0.06);color:var(--success);border-color:rgba(22,163,74,0.15)}
body.theme-clean .card{border-color:rgba(0,0,0,0.08);box-shadow:0 1px 3px rgba(0,0,0,0.04)}
body.theme-clean .toggle{background:rgba(0,0,0,0.1)}
body.theme-clean .toggle.on{background:var(--accent)}
body.theme-clean .webhook-item{background:rgba(0,0,0,0.01);border-color:rgba(0,0,0,0.08)}
body.theme-clean .webhook-item:hover{background:rgba(0,0,0,0.03)}
body.theme-clean .webhook-form{border-color:var(--accent);background:rgba(0,0,0,0.02)}
body.theme-clean .theme-option{border-color:rgba(0,0,0,0.12)}
body.theme-clean .theme-option:hover{border-color:rgba(0,0,0,0.2)}
body.theme-clean .theme-option.active{border-color:var(--accent);background:rgba(0,0,0,0.03)}
body.theme-clean .log-table th{color:#808080;border-bottom-color:rgba(0,0,0,0.08)}
body.theme-clean .log-table td{color:#171717;border-bottom-color:rgba(0,0,0,0.04)}
body.theme-clean .log-table tr:hover td{background:rgba(0,0,0,0.02)}
body.theme-clean .swatch{border-color:rgba(0,0,0,0.1)}
body.theme-clean select,body.theme-clean input[type="text"],body.theme-clean input[type="url"],body.theme-clean input[type="password"],body.theme-clean input[type="number"]{
  background:var(--input-bg);border-color:rgba(0,0,0,0.12);color:var(--text)}
body.theme-clean select:focus,body.theme-clean input:focus{border-color:var(--accent)}
body.theme-clean .back-link{color:#808080;border-color:rgba(0,0,0,0.12)}
body.theme-clean .back-link:hover{color:#171717;border-color:rgba(0,0,0,0.2);background:rgba(0,0,0,0.02)}
body.theme-clean .coming-soon::after{color:var(--accent);background:rgba(0,0,0,0.05)}
/* Ember theme */
body.theme-ember{
  --bg:#07080a;--surface:#101111;--text:#f9f9f9;--text2:#9c9c9d;
  --accent:#55b3ff;--accent-hover:#7ec8ff;--success:#5fc992;--error:#FF6363;
  --border:rgba(255,255,255,0.06);--radius:8px;
  --input-bg:rgba(255,255,255,0.04);--code-bg:rgba(255,255,255,0.06);
}
body.theme-ember .btn-primary{background:var(--accent);border-color:var(--accent)}
body.theme-ember .btn-primary:hover{background:var(--accent-hover)}
body.theme-ember .coming-soon::after{color:var(--accent);background:rgba(85,179,255,0.12)}
html{background:var(--bg);color:var(--text);font-family:'Inter',system-ui,-apple-system,sans-serif;font-size:14px;line-height:1.5;-webkit-font-smoothing:antialiased}
body{min-height:100vh;padding:24px;transition:background 0.2s ease,color 0.2s ease}
a{color:var(--accent);text-decoration:none}
a:hover{color:var(--accent-hover)}

.container{max-width:860px;margin:0 auto}

/* Header */
.header{display:flex;align-items:center;justify-content:space-between;padding:16px 0;margin-bottom:32px;border-bottom:1px solid var(--border)}
.header-left{display:flex;align-items:center;gap:16px}
.logo{display:flex;align-items:center;gap:8px;font-size:20px;font-weight:600;letter-spacing:-0.5px;color:var(--text)}
.logo img{width:24px;height:24px;border-radius:4px}
.page-title{font-size:20px;font-weight:600;color:var(--text2)}
.back-link{font-size:13px;font-weight:500;padding:6px 14px;border-radius:var(--radius);border:1px solid var(--border);color:var(--text2);transition:all 0.15s}
.back-link:hover{color:var(--text);border-color:rgba(255,255,255,0.15);background:rgba(255,255,255,0.03)}

/* Cards */
.card{background:var(--surface);border:1px solid var(--border);border-radius:12px;padding:24px;margin-bottom:24px;transition:background 0.2s ease,border-color 0.2s ease}
.card-title{font-size:16px;font-weight:600;margin-bottom:4px}
.card-desc{font-size:13px;color:var(--text2);margin-bottom:20px}

/* Form elements */
label{display:block;font-size:12px;font-weight:600;color:var(--text2);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:6px}
select,input[type="text"],input[type="url"],input[type="password"],input[type="number"]{
  width:100%;padding:8px 12px;background:var(--input-bg);border:1px solid var(--border);
  border-radius:var(--radius);color:var(--text);font-size:14px;font-family:inherit;outline:none;transition:border 0.15s}
select:focus,input:focus{border-color:var(--accent)}
select{cursor:pointer;-webkit-appearance:none;appearance:none;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' fill='%238a8f98'%3E%3Cpath d='M6 8.5L1 3.5h10z'/%3E%3C/svg%3E");
  background-repeat:no-repeat;background-position:right 10px center}
input:disabled,select:disabled{opacity:0.4;cursor:not-allowed}

.form-row{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:16px}
@media(max-width:600px){.form-row{grid-template-columns:1fr}}
.form-group{margin-bottom:16px}

/* Buttons */
.btn{display:inline-flex;align-items:center;gap:6px;padding:8px 16px;border-radius:var(--radius);font-size:13px;font-weight:600;font-family:inherit;cursor:pointer;border:1px solid transparent;transition:all 0.15s}
.btn-primary{background:var(--accent);color:#fff;border-color:var(--accent)}
.btn-primary:hover{background:var(--accent-hover)}
.btn-secondary{background:rgba(255,255,255,0.04);color:var(--text2);border-color:var(--border)}
.btn-secondary:hover{color:var(--text);border-color:rgba(255,255,255,0.15);background:rgba(255,255,255,0.06)}
.btn-danger{background:rgba(220,38,38,0.1);color:var(--error);border-color:rgba(220,38,38,0.2)}
.btn-danger:hover{background:rgba(220,38,38,0.18)}
.btn-success{background:rgba(39,166,68,0.1);color:var(--success);border-color:rgba(39,166,68,0.2)}
.btn:disabled{opacity:0.4;cursor:not-allowed}
.btn-sm{padding:5px 10px;font-size:12px}

/* Theme swatches */
.theme-options{display:flex;gap:12px;margin-bottom:16px}
.theme-option{display:flex;align-items:center;gap:8px;padding:10px 16px;border-radius:var(--radius);border:2px solid var(--border);cursor:pointer;transition:all 0.15s;background:transparent}
.theme-option:hover{border-color:rgba(255,255,255,0.15)}
.theme-option.active{border-color:var(--accent);background:rgba(94,106,210,0.06)}
.theme-option input{display:none}
.swatch{width:24px;height:24px;border-radius:6px;border:1px solid rgba(255,255,255,0.1);flex-shrink:0}
.swatch-midnight{background:linear-gradient(135deg,#0f1011 50%,#5e6ad2 50%)}
.swatch-clean{background:linear-gradient(135deg,#ffffff 50%,#000000 50%)}
.swatch-ember{background:linear-gradient(135deg,#1a1a1a 50%,#f97316 50%)}
.theme-name{font-size:13px;font-weight:500;color:var(--text)}

/* Toggle */
.toggle-wrap{display:flex;align-items:center;gap:10px;cursor:pointer}
.toggle{position:relative;width:40px;height:22px;background:rgba(255,255,255,0.1);border-radius:11px;transition:background 0.2s;flex-shrink:0}
.toggle.on{background:var(--accent)}
.toggle-knob{position:absolute;top:2px;left:2px;width:18px;height:18px;background:#fff;border-radius:50%;transition:left 0.2s}
.toggle.on .toggle-knob{left:20px}
.toggle-label{font-size:13px;color:var(--text2)}

/* Webhooks list */
.webhook-item{display:flex;align-items:center;gap:12px;padding:12px 16px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:8px;background:rgba(255,255,255,0.02);transition:background 0.15s}
.webhook-item:hover{background:rgba(255,255,255,0.04)}
.webhook-info{flex:1;min-width:0}
.webhook-name{font-size:14px;font-weight:600;color:var(--text)}
.webhook-url{font-size:12px;color:var(--text2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:300px}
.badge{display:inline-block;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:0.5px;padding:2px 8px;border-radius:4px;margin-left:8px}
.badge-discord{background:rgba(88,101,242,0.15);color:#5865f2}
.badge-slack{background:rgba(74,21,75,0.15);color:#e01e5a}
.badge-gotify{background:rgba(16,185,129,0.15);color:#10b981}
.badge-ntfy{background:rgba(59,130,246,0.15);color:#3b82f6}
.badge-generic{background:rgba(255,255,255,0.08);color:var(--text2)}
.webhook-actions{display:flex;align-items:center;gap:8px;flex-shrink:0}

/* Webhook form */
.webhook-form{border:1px solid var(--accent);border-radius:var(--radius);padding:20px;margin-top:12px;background:rgba(94,106,210,0.04);display:none}
.webhook-form.visible{display:block}
.webhook-form-actions{display:flex;gap:8px;margin-top:16px}

/* Coming soon overlay */
.coming-soon{position:relative}
.coming-soon::after{content:"Coming Soon";position:absolute;top:12px;right:12px;font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:0.5px;color:var(--accent);background:rgba(94,106,210,0.12);padding:3px 10px;border-radius:4px}
.coming-soon .form-group{opacity:0.4;pointer-events:none}

/* Notification log table */
.log-table{width:100%;border-collapse:collapse}
.log-table th{text-align:left;font-size:11px;font-weight:600;color:var(--text2);text-transform:uppercase;letter-spacing:0.5px;padding:8px 12px;border-bottom:1px solid var(--border)}
.log-table td{padding:8px 12px;font-size:13px;color:var(--text);border-bottom:1px solid rgba(255,255,255,0.04);white-space:nowrap}
.log-table tr:hover td{background:rgba(255,255,255,0.02)}
.status-badge{font-size:11px;font-weight:600;padding:2px 8px;border-radius:4px}
.status-success{color:var(--success);background:rgba(39,166,68,0.12)}
.status-failed{color:var(--error);background:rgba(220,38,38,0.12)}
.log-empty{text-align:center;padding:32px;color:var(--text2);font-size:13px}
.log-refresh{font-size:12px;color:var(--text2);margin-top:8px}
.overflow-x{overflow-x:auto}

/* Toasts */
.toast-container{position:fixed;top:20px;right:20px;z-index:9999;display:flex;flex-direction:column;gap:8px}
.toast{padding:10px 18px;border-radius:var(--radius);font-size:13px;font-weight:500;color:#fff;animation:toast-in 0.25s ease;pointer-events:none}
.toast-success{background:var(--success)}
.toast-error{background:var(--error)}
.toast-info{background:var(--accent)}
@keyframes toast-in{from{opacity:0;transform:translateY(-8px)}to{opacity:1;transform:translateY(0)}}
@keyframes toast-out{from{opacity:1}to{opacity:0;transform:translateY(-8px)}}
</style>
</head>
<body>
<div class="toast-container" id="toasts"></div>
<div class="container">

  <!-- Header -->
  <div class="header">
    <div class="header-left">
      <div class="logo"><img src="/icon.png" alt="">NAS Doctor</div>
      <span class="page-title">Settings</span>
    </div>
    <a href="/" class="back-link">&#8592; Dashboard</a>
  </div>

  <!-- 1. General -->
  <div class="card" id="card-general">
    <div class="card-title">General</div>
    <div class="card-desc">Configure scan interval and appearance.</div>
    <div class="form-row">
      <div>
        <label for="scan-preset">Scan Interval</label>
        <select id="scan-preset" onchange="onPresetChange()">
          <option value="30m">Every 30 minutes</option>
          <option value="1h">Every 1 hour</option>
          <option value="2h">Every 2 hours</option>
          <option value="6h" selected>Every 6 hours</option>
          <option value="12h">Every 12 hours</option>
          <option value="24h">Every 24 hours</option>
          <option value="custom">Custom...</option>
        </select>
      </div>
      <div></div>
    </div>
    <div id="custom-interval-panel" style="display:none;margin-bottom:16px;padding:16px;background:rgba(255,255,255,0.02);border:1px solid var(--border);border-radius:var(--radius)">
      <div style="display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap;margin-bottom:12px">
        <div style="flex:1;min-width:70px">
          <label for="scan-days" style="font-size:11px;margin-bottom:4px">Days</label>
          <input type="number" id="scan-days" min="0" max="365" value="0" style="text-align:center">
        </div>
        <div style="flex:1;min-width:70px">
          <label for="scan-hours" style="font-size:11px;margin-bottom:4px">Hours</label>
          <input type="number" id="scan-hours" min="0" max="23" value="0" style="text-align:center">
        </div>
        <div style="flex:1;min-width:70px">
          <label for="scan-minutes" style="font-size:11px;margin-bottom:4px">Minutes</label>
          <input type="number" id="scan-minutes" min="0" max="59" value="30" style="text-align:center">
        </div>
        <div style="flex:1;min-width:70px">
          <label for="scan-seconds" style="font-size:11px;margin-bottom:4px">Seconds</label>
          <input type="number" id="scan-seconds" min="0" max="59" value="0" style="text-align:center">
        </div>
      </div>
      <div id="scan-interval-preview" style="font-size:12px;color:var(--text2);margin-bottom:4px">Scans every 30 minutes</div>
      <div id="scan-cron-preview" style="font-size:11px;font-family:monospace;color:var(--accent);background:rgba(94,106,210,0.06);padding:4px 10px;border-radius:4px;display:inline-block"></div>
    </div>
    <label>Theme</label>
    <div class="theme-options" id="theme-options">
      <div class="theme-option active" data-theme="midnight" onclick="selectTheme(this)">
        <input type="radio" name="theme" value="midnight" checked>
        <div class="swatch swatch-midnight"></div>
        <span class="theme-name">Midnight</span>
      </div>
      <div class="theme-option" data-theme="clean" onclick="selectTheme(this)">
        <input type="radio" name="theme" value="clean">
        <div class="swatch swatch-clean"></div>
        <span class="theme-name">Clean</span>
      </div>
      <div class="theme-option" data-theme="ember" onclick="selectTheme(this)">
        <input type="radio" name="theme" value="ember">
        <div class="swatch swatch-ember"></div>
        <span class="theme-name">Ember</span>
      </div>
    </div>
    <button class="btn btn-primary" onclick="saveSettings()">Save General Settings</button>
  </div>

  <!-- 2. Notifications — Webhooks -->
  <div class="card" id="card-webhooks">
    <div class="card-title">Notifications &mdash; Webhooks</div>
    <div class="card-desc">Configure webhook endpoints for scan result notifications.</div>
    <div id="webhook-list"></div>
    <button class="btn btn-secondary" id="btn-add-webhook" onclick="toggleWebhookForm()" style="margin-top:8px">+ Add Webhook</button>
    <div class="webhook-form" id="webhook-form">
      <input type="hidden" id="wh-edit-index" value="-1">
      <div class="form-row">
        <div>
          <label for="wh-name">Name</label>
          <input type="text" id="wh-name" placeholder="e.g. My Discord">
        </div>
        <div>
          <label for="wh-type">Type</label>
          <select id="wh-type">
            <option value="discord">Discord</option>
            <option value="slack">Slack</option>
            <option value="gotify">Gotify</option>
            <option value="ntfy">Ntfy</option>
            <option value="generic">Generic</option>
          </select>
        </div>
      </div>
      <div class="form-group">
        <label for="wh-url">URL</label>
        <input type="url" id="wh-url" placeholder="https://...">
      </div>
      <div class="form-row">
        <div>
          <label for="wh-level">Minimum Level</label>
          <select id="wh-level">
            <option value="critical">Critical</option>
            <option value="warning">Warning</option>
            <option value="info" selected>Info</option>
          </select>
        </div>
        <div>
          <label for="wh-secret">Secret (optional, HMAC signing)</label>
          <input type="password" id="wh-secret" placeholder="Leave blank if not needed">
        </div>
      </div>
      <div class="webhook-form-actions">
        <button class="btn btn-primary" onclick="saveWebhook()">Save Webhook</button>
        <button class="btn btn-success btn-sm" onclick="testWebhook()">Test</button>
        <button class="btn btn-secondary" onclick="cancelWebhookForm()">Cancel</button>
      </div>
    </div>
  </div>

  <!-- 3. Log Forwarding (coming soon) -->
  <div class="card coming-soon" id="card-logfwd">
    <div class="card-title">Log Forwarding</div>
    <div class="card-desc">Forward scan results and metrics to external logging or observability endpoints (e.g. Grafana via Prometheus).</div>
    <div class="form-group">
      <div class="toggle-wrap" style="margin-bottom:16px">
        <div class="toggle" id="logfwd-toggle"><div class="toggle-knob"></div></div>
        <span class="toggle-label">Enable Log Forwarding</span>
      </div>
      <label for="logfwd-url">Destination URL</label>
      <input type="url" id="logfwd-url" placeholder="https://..." disabled>
    </div>
    <div class="form-group">
      <label for="logfwd-format">Format</label>
      <select id="logfwd-format" disabled>
        <option value="json">JSON</option>
        <option value="syslog">Syslog</option>
      </select>
    </div>
    <div class="form-group" style="margin-top:12px">
      <p style="font-size:12px;color:var(--text2);line-height:1.6">Prometheus metrics are already available at <code style="background:rgba(255,255,255,0.06);padding:2px 6px;border-radius:4px;font-size:11px">/metrics</code> for Grafana integration. Configure your Prometheus scraper to target this endpoint.</p>
    </div>
  </div>

  <!-- 4. Data Lifecycle -->
  <div class="card" id="card-retention">
    <div class="card-title">Data Lifecycle</div>
    <div class="card-desc">Control how long diagnostic data is stored. Older data is automatically pruned after each scan.</div>
    <div class="form-row">
      <div>
        <label for="ret-snapshot-days">Snapshot retention (days)</label>
        <input type="number" id="ret-snapshot-days" min="7" max="365" value="90" style="text-align:center">
        <p style="font-size:11px;color:var(--text2);margin-top:4px">Snapshots, SMART history, and system metrics older than this are deleted.</p>
      </div>
      <div>
        <label for="ret-notify-days">Notification log (days)</label>
        <input type="number" id="ret-notify-days" min="1" max="365" value="30" style="text-align:center">
      </div>
    </div>
    <div class="form-row" style="margin-top:12px">
      <div>
        <label for="ret-max-db">Max database size (MB)</label>
        <input type="number" id="ret-max-db" min="50" max="10000" value="500" style="text-align:center">
        <p style="font-size:11px;color:var(--text2);margin-top:4px">Hard cap. Oldest data is aggressively deleted if exceeded.</p>
      </div>
      <div>
        <div id="db-stats" style="font-size:12px;color:var(--text2);line-height:1.8;padding-top:20px">Loading database stats...</div>
      </div>
    </div>
  </div>

  <!-- 5. Backup -->
  <div class="card" id="card-backup">
    <div class="card-title">Automatic Backup</div>
    <div class="card-desc">Periodically back up settings and historical data so you can restore after a Docker reinstall.</div>
    <div class="form-group">
      <div class="toggle-wrap" style="margin-bottom:16px">
        <div class="toggle on" id="backup-toggle" onclick="toggleBackup()"><div class="toggle-knob"></div></div>
        <span class="toggle-label">Enable automatic backups</span>
      </div>
    </div>
    <div class="form-row">
      <div>
        <label for="backup-path">Backup directory</label>
        <input type="text" id="backup-path" placeholder="Leave empty for default (data/backups/)">
        <p style="font-size:11px;color:var(--text2);margin-top:4px">Set a path to a mounted share for off-server backups.</p>
      </div>
      <div>
        <label for="backup-keep">Keep last N backups</label>
        <input type="number" id="backup-keep" min="1" max="52" value="4" style="text-align:center">
      </div>
    </div>
    <div class="form-row" style="margin-top:12px">
      <div>
        <label for="backup-interval">Backup every (hours)</label>
        <input type="number" id="backup-interval" min="1" max="8760" value="168" style="text-align:center">
        <p style="font-size:11px;color:var(--text2);margin-top:4px">168 = weekly, 24 = daily</p>
      </div>
      <div>
        <div id="backup-info" style="font-size:12px;color:var(--text2);line-height:1.8;padding-top:20px">Loading backup info...</div>
      </div>
    </div>
    <div style="margin-top:12px">
      <button class="btn-secondary" onclick="triggerBackup()">Backup Now</button>
    </div>
  </div>

  <!-- 6. Notification History -->
  <div class="card" id="card-history">
    <div class="card-title">Notification History</div>
    <div class="card-desc">Recent webhook delivery attempts. <span class="log-refresh">Auto-refreshes every 30s.</span></div>
    <div class="overflow-x">
      <table class="log-table">
        <thead>
          <tr><th>Time</th><th>Webhook</th><th>Type</th><th>Status</th><th>Findings</th><th>Error</th></tr>
        </thead>
        <tbody id="log-body">
          <tr><td colspan="6" class="log-empty">Loading...</td></tr>
        </tbody>
      </table>
    </div>
  </div>

</div>

<script>
/* ---------- State ---------- */
var settings = null;
var webhooks = [];

/* ---------- Toast ---------- */
function showToast(msg, type) {
  var c = document.getElementById("toasts");
  var t = document.createElement("div");
  t.className = "toast toast-" + (type || "info");
  t.textContent = msg;
  c.appendChild(t);
  setTimeout(function() {
    t.style.animation = "toast-out 0.25s ease forwards";
    setTimeout(function() { t.remove(); }, 260);
  }, 3000);
}

/* ---------- Theme selection ---------- */
function selectTheme(el) {
  var opts = document.querySelectorAll(".theme-option");
  for (var i = 0; i < opts.length; i++) {
    opts[i].classList.remove("active");
    opts[i].querySelector("input").checked = false;
  }
  el.classList.add("active");
  el.querySelector("input").checked = true;
}

function getSelectedTheme() {
  var checked = document.querySelector('input[name="theme"]:checked');
  return checked ? checked.value : "midnight";
}

/* ---------- Interval helpers ---------- */
var presetValues = ["30m", "1h", "2h", "6h", "12h", "24h"];

function parseDurationToFields(dur) {
  var days = 0, hours = 0, minutes = 0, seconds = 0;
  if (!dur) return { days: 0, hours: 6, minutes: 0, seconds: 0 };
  var m;
  m = dur.match(/(\d+)h/);
  if (m) { var h = parseInt(m[1], 10); days = Math.floor(h / 24); hours = h % 24; }
  m = dur.match(/(\d+)m(?!s)/);
  if (m) { minutes = parseInt(m[1], 10); }
  m = dur.match(/(\d+)s/);
  if (m) { seconds = parseInt(m[1], 10); }
  return { days: days, hours: hours, minutes: minutes, seconds: seconds };
}

function fieldsToDuration() {
  var d = parseInt(document.getElementById("scan-days").value, 10) || 0;
  var h = parseInt(document.getElementById("scan-hours").value, 10) || 0;
  var m = parseInt(document.getElementById("scan-minutes").value, 10) || 0;
  var s = parseInt(document.getElementById("scan-seconds").value, 10) || 0;
  var totalH = d * 24 + h;
  var parts = [];
  if (totalH > 0) parts.push(totalH + "h");
  if (m > 0) parts.push(m + "m");
  if (s > 0) parts.push(s + "s");
  if (parts.length === 0) return "6h";
  return parts.join("");
}

function getScanInterval() {
  var preset = document.getElementById("scan-preset").value;
  if (preset !== "custom") return preset;
  return fieldsToDuration();
}

function isPresetValue(dur) {
  for (var i = 0; i < presetValues.length; i++) {
    if (presetValues[i] === dur) return true;
  }
  return false;
}

function durationToCron(dur) {
  var f = parseDurationToFields(dur);
  var totalSecs = f.days * 86400 + f.hours * 3600 + f.minutes * 60 + f.seconds;
  if (totalSecs <= 0) return "";
  if (totalSecs < 60) return "*/" + totalSecs + "s (every " + totalSecs + " seconds)";
  var totalMin = Math.round(totalSecs / 60);
  if (totalMin <= 0) totalMin = 1;
  if (totalMin < 60) return "*/" + totalMin + " * * * *";
  var totalHrs = f.days * 24 + f.hours;
  var extraMin = f.minutes;
  if (totalHrs > 0 && extraMin === 0 && f.seconds === 0) {
    if (totalHrs < 24) return "0 */" + totalHrs + " * * *";
    if (totalHrs % 24 === 0) {
      var d = totalHrs / 24;
      return "0 0 */" + d + " * *";
    }
    return "0 */" + totalHrs + " * * *";
  }
  if (totalSecs < 3600) return "*/" + totalMin + " * * * *";
  return "0 */" + totalHrs + " * * *";
}

function onPresetChange() {
  var preset = document.getElementById("scan-preset").value;
  var panel = document.getElementById("custom-interval-panel");
  if (preset === "custom") {
    panel.style.display = "block";
    updateIntervalPreview();
  } else {
    panel.style.display = "none";
  }
}

function updateIntervalPreview() {
  var d = parseInt(document.getElementById("scan-days").value, 10) || 0;
  var h = parseInt(document.getElementById("scan-hours").value, 10) || 0;
  var m = parseInt(document.getElementById("scan-minutes").value, 10) || 0;
  var s = parseInt(document.getElementById("scan-seconds").value, 10) || 0;
  var parts = [];
  if (d > 0) parts.push(d + (d === 1 ? " day" : " days"));
  if (h > 0) parts.push(h + (h === 1 ? " hour" : " hours"));
  if (m > 0) parts.push(m + (m === 1 ? " minute" : " minutes"));
  if (s > 0) parts.push(s + (s === 1 ? " second" : " seconds"));
  var el = document.getElementById("scan-interval-preview");
  var cronEl = document.getElementById("scan-cron-preview");
  if (parts.length === 0) {
    el.textContent = "Invalid: set at least 1 second";
    el.style.color = "var(--error)";
    cronEl.textContent = "";
  } else {
    el.textContent = "Scans every " + parts.join(", ");
    el.style.color = "var(--text2)";
    var dur = fieldsToDuration();
    var cron = durationToCron(dur);
    cronEl.textContent = cron ? "cron: " + cron : "";
  }
}

document.getElementById("scan-days").addEventListener("input", updateIntervalPreview);
document.getElementById("scan-hours").addEventListener("input", updateIntervalPreview);
document.getElementById("scan-minutes").addEventListener("input", updateIntervalPreview);
document.getElementById("scan-seconds").addEventListener("input", updateIntervalPreview);

/* ---------- Load settings ---------- */
function loadSettings() {
  fetch("/api/v1/settings")
    .then(function(r) { return r.json(); })
    .then(function(data) {
      settings = data;
      /* Scan interval — check if it matches a preset */
      var sel = document.getElementById("scan-preset");
      if (isPresetValue(data.scan_interval)) {
        sel.value = data.scan_interval;
        document.getElementById("custom-interval-panel").style.display = "none";
      } else {
        sel.value = "custom";
        document.getElementById("custom-interval-panel").style.display = "block";
        var f = parseDurationToFields(data.scan_interval);
        document.getElementById("scan-days").value = f.days;
        document.getElementById("scan-hours").value = f.hours;
        document.getElementById("scan-minutes").value = f.minutes;
        document.getElementById("scan-seconds").value = f.seconds;
        updateIntervalPreview();
      }
      /* Theme */
      var opts = document.querySelectorAll(".theme-option");
      for (var j = 0; j < opts.length; j++) {
        var isActive = opts[j].getAttribute("data-theme") === data.theme;
        opts[j].classList.toggle("active", isActive);
        opts[j].querySelector("input").checked = isActive;
      }
      /* Apply theme to settings page */
      applyTheme(data.theme);
      try { localStorage.setItem("nas-doctor-theme", data.theme); } catch(e) {}
      /* Webhooks */
      webhooks = (data.notifications && data.notifications.webhooks) ? data.notifications.webhooks : [];
      renderWebhooks();
      /* Retention */
      var ret = data.retention || {};
      document.getElementById("ret-snapshot-days").value = ret.snapshot_days || 90;
      document.getElementById("ret-notify-days").value = ret.notify_log_days || 30;
      document.getElementById("ret-max-db").value = ret.max_db_size_mb || 500;
      /* Backup */
      var bk = data.backup || {};
      var bkToggle = document.getElementById("backup-toggle");
      if (bk.enabled === false) { bkToggle.classList.remove("on"); } else { bkToggle.classList.add("on"); }
      document.getElementById("backup-path").value = bk.path || "";
      document.getElementById("backup-keep").value = bk.keep_count || 4;
      document.getElementById("backup-interval").value = bk.interval_hours || 168;
    })
    .catch(function(e) { showToast("Failed to load settings: " + e, "error"); });
}

/* ---------- Save settings ---------- */
function buildSettingsPayload() {
  return {
    scan_interval: getScanInterval(),
    theme: getSelectedTheme(),
    notifications: { webhooks: webhooks },
    log_push: { enabled: false, destinations: [] },
    retention: {
      snapshot_days: parseInt(document.getElementById("ret-snapshot-days").value, 10) || 90,
      max_db_size_mb: parseInt(document.getElementById("ret-max-db").value, 10) || 500,
      notify_log_days: parseInt(document.getElementById("ret-notify-days").value, 10) || 30
    },
    backup: {
      enabled: document.getElementById("backup-toggle").classList.contains("on"),
      path: document.getElementById("backup-path").value.trim(),
      keep_count: parseInt(document.getElementById("backup-keep").value, 10) || 4,
      interval_hours: parseInt(document.getElementById("backup-interval").value, 10) || 168
    }
  };
}

function saveSettings() {
  var payload = buildSettingsPayload();
  fetch("/api/v1/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  })
  .then(function(r) {
    if (!r.ok) return r.json().then(function(e) { throw new Error(e.error || "Save failed"); });
    return r.json();
  })
  .then(function() {
    showToast("Settings saved", "success");
    /* Apply theme live */
    applyTheme(payload.theme);
    try { localStorage.setItem("nas-doctor-theme", payload.theme); } catch(e) {}
  })
  .catch(function(e) { showToast("Error: " + e.message, "error"); });
}

/* ---------- Webhooks ---------- */
function maskUrl(url) {
  if (!url) return "";
  try {
    var u = new URL(url);
    var host = u.hostname;
    var path = u.pathname;
    if (path.length > 12) path = path.substring(0, 12) + "...";
    return u.protocol + "//" + host + path;
  } catch(e) {
    if (url.length > 40) return url.substring(0, 40) + "...";
    return url;
  }
}

function renderWebhooks() {
  var container = document.getElementById("webhook-list");
  if (!webhooks || webhooks.length === 0) {
    container.innerHTML = '<div style="padding:16px;text-align:center;color:var(--text2);font-size:13px">No webhooks configured yet.</div>';
    return;
  }
  var html = "";
  for (var i = 0; i < webhooks.length; i++) {
    var wh = webhooks[i];
    var badgeClass = "badge-" + (wh.type || "generic");
    var typeName = (wh.type || "generic").charAt(0).toUpperCase() + (wh.type || "generic").slice(1);
    html += '<div class="webhook-item">';
    html += '  <div class="webhook-info">';
    html += '    <div class="webhook-name">' + escapeHtml(wh.name || "Unnamed") + '<span class="badge ' + badgeClass + '">' + typeName + '</span></div>';
    html += '    <div class="webhook-url">' + escapeHtml(maskUrl(wh.url)) + '</div>';
    html += '  </div>';
    html += '  <div class="webhook-actions">';
    html += '    <div class="toggle-wrap" onclick="toggleWebhookEnabled(' + i + ')">';
    html += '      <div class="toggle' + (wh.enabled ? " on" : "") + '"><div class="toggle-knob"></div></div>';
    html += '    </div>';
    html += '    <button class="btn btn-secondary btn-sm" onclick="editWebhook(' + i + ')">Edit</button>';
    html += '    <button class="btn btn-danger btn-sm" onclick="removeWebhook(' + i + ')">Remove</button>';
    html += '  </div>';
    html += '</div>';
  }
  container.innerHTML = html;
}

function toggleWebhookEnabled(idx) {
  webhooks[idx].enabled = !webhooks[idx].enabled;
  renderWebhooks();
  saveSettings();
}

function toggleWebhookForm() {
  var form = document.getElementById("webhook-form");
  if (form.classList.contains("visible")) {
    cancelWebhookForm();
    return;
  }
  document.getElementById("wh-edit-index").value = "-1";
  document.getElementById("wh-name").value = "";
  document.getElementById("wh-url").value = "";
  document.getElementById("wh-type").value = "discord";
  document.getElementById("wh-level").value = "info";
  document.getElementById("wh-secret").value = "";
  form.classList.add("visible");
}

function editWebhook(idx) {
  var wh = webhooks[idx];
  document.getElementById("wh-edit-index").value = String(idx);
  document.getElementById("wh-name").value = wh.name || "";
  document.getElementById("wh-url").value = wh.url || "";
  document.getElementById("wh-type").value = wh.type || "generic";
  document.getElementById("wh-level").value = wh.min_level || "info";
  document.getElementById("wh-secret").value = wh.secret || "";
  document.getElementById("webhook-form").classList.add("visible");
}

function cancelWebhookForm() {
  document.getElementById("webhook-form").classList.remove("visible");
}

function saveWebhook() {
  var name = document.getElementById("wh-name").value.trim();
  var url = document.getElementById("wh-url").value.trim();
  if (!name || !url) {
    showToast("Name and URL are required", "error");
    return;
  }
  var wh = {
    name: name,
    url: url,
    type: document.getElementById("wh-type").value,
    enabled: true,
    min_level: document.getElementById("wh-level").value,
    secret: document.getElementById("wh-secret").value.trim() || ""
  };
  var editIdx = parseInt(document.getElementById("wh-edit-index").value, 10);
  if (editIdx >= 0 && editIdx < webhooks.length) {
    wh.enabled = webhooks[editIdx].enabled;
    webhooks[editIdx] = wh;
  } else {
    webhooks.push(wh);
  }
  renderWebhooks();
  cancelWebhookForm();
  saveSettings();
}

function removeWebhook(idx) {
  if (!confirm("Remove webhook \"" + (webhooks[idx].name || "Unnamed") + "\"?")) return;
  webhooks.splice(idx, 1);
  renderWebhooks();
  saveSettings();
}

function testWebhook() {
  var url = document.getElementById("wh-url").value.trim();
  if (!url) {
    showToast("Enter a URL first", "error");
    return;
  }
  var wh = {
    name: document.getElementById("wh-name").value.trim() || "Test",
    url: url,
    type: document.getElementById("wh-type").value,
    enabled: true,
    min_level: document.getElementById("wh-level").value,
    secret: document.getElementById("wh-secret").value.trim() || ""
  };
  showToast("Sending test...", "info");
  fetch("/api/v1/settings/test-webhook", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(wh)
  })
  .then(function(r) {
    if (!r.ok) return r.json().then(function(e) { throw new Error(e.error || "Test failed"); });
    return r.json();
  })
  .then(function(data) { showToast("Test sent! Check your destination.", "success"); })
  .catch(function(e) { showToast("Error: " + e.message, "error"); });
}

/* ---------- Notification log ---------- */
function loadNotificationLog() {
  fetch("/api/v1/notifications/log")
    .then(function(r) { return r.json(); })
    .then(function(entries) {
      var tbody = document.getElementById("log-body");
      if (!entries || entries.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="log-empty">No notifications sent yet.</td></tr>';
        return;
      }
      var html = "";
      for (var i = 0; i < entries.length; i++) {
        var e = entries[i];
        var ts = e.created_at ? new Date(e.created_at).toLocaleString() : "—";
        var statusCls = e.status === "success" ? "status-success" : "status-failed";
        var badgeClass = "badge-" + (e.webhook_type || "generic");
        var typeName = (e.webhook_type || "generic").charAt(0).toUpperCase() + (e.webhook_type || "generic").slice(1);
        html += "<tr>";
        html += "<td>" + escapeHtml(ts) + "</td>";
        html += "<td>" + escapeHtml(e.webhook_name || "—") + "</td>";
        html += '<td><span class="badge ' + badgeClass + '">' + typeName + "</span></td>";
        html += '<td><span class="status-badge ' + statusCls + '">' + escapeHtml(e.status || "unknown") + "</span></td>";
        html += "<td>" + (e.findings_count != null ? e.findings_count : "—") + "</td>";
        html += "<td>" + escapeHtml(e.error_message || "—") + "</td>";
        html += "</tr>";
      }
      tbody.innerHTML = html;
    })
    .catch(function() {
      document.getElementById("log-body").innerHTML = '<tr><td colspan="6" class="log-empty">Failed to load log.</td></tr>';
    });
}

/* ---------- Helpers ---------- */
function escapeHtml(str) {
  if (!str) return "";
  return String(str).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;");
}

/* ---------- Apply saved theme ---------- */
function applyTheme(theme) {
  document.body.classList.remove("theme-midnight", "theme-clean", "theme-ember");
  if (theme === "clean" || theme === "ember") {
    document.body.classList.add("theme-" + theme);
  } else {
    document.body.classList.add("theme-midnight");
  }
}

/* Load theme before anything else to avoid flash */
(function() {
  try {
    var stored = localStorage.getItem("nas-doctor-theme");
    if (stored) applyTheme(stored);
  } catch(e) {}
})();

/* ---------- DB Stats ---------- */
function loadDBStats() {
  fetch("/api/v1/db/stats")
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var el = document.getElementById("db-stats");
      var lines = [];
      lines.push("<strong>" + d.file_size_mb.toFixed(1) + " MB</strong> database size");
      lines.push(d.snapshot_count + " snapshots stored");
      lines.push(d.smart_history_rows + " SMART history rows");
      lines.push(d.system_history_rows + " system history rows");
      if (d.oldest_snapshot) {
        var oldest = d.oldest_snapshot.substring(0, 10);
        var newest = d.newest_snapshot.substring(0, 10);
        lines.push("Range: " + oldest + " → " + newest);
      }
      el.innerHTML = lines.join("<br>");
    })
    .catch(function() {
      document.getElementById("db-stats").textContent = "Failed to load stats";
    });
}

/* ---------- Backup ---------- */
function toggleBackup() {
  var el = document.getElementById("backup-toggle");
  el.classList.toggle("on");
}

function loadBackupInfo() {
  fetch("/api/v1/backup")
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var el = document.getElementById("backup-info");
      var backups = d.backups || [];
      if (backups.length === 0) {
        el.textContent = "No backups yet";
      } else {
        var lines = [];
        lines.push("<strong>" + backups.length + "</strong> backup(s) stored");
        lines.push("Latest: " + backups[0].timestamp.substring(0, 10));
        lines.push("Size: " + backups[0].size_mb.toFixed(1) + " MB");
        el.innerHTML = lines.join("<br>");
      }
    })
    .catch(function() {
      document.getElementById("backup-info").textContent = "Failed to load";
    });
}

function triggerBackup() {
  showToast("Creating backup...", "success");
  fetch("/api/v1/backup", { method: "POST" })
    .then(function(r) { return r.json(); })
    .then(function(d) {
      if (d.error) { showToast("Backup failed: " + d.error, "error"); return; }
      showToast("Backup created: " + d.size_mb.toFixed(1) + " MB", "success");
      loadBackupInfo();
    })
    .catch(function(e) { showToast("Backup error: " + e.message, "error"); });
}

/* ---------- Init ---------- */
loadSettings();
loadNotificationLog();
loadDBStats();
loadBackupInfo();
setInterval(loadNotificationLog, 30000);
</script>
</body>
</html>`

// handleSettingsPage serves the settings HTML page.
// GET /settings
func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(SettingsPage))
}

// DiskDetailPage is a self-contained HTML page for viewing detailed per-disk SMART analysis.
var DiskDetailPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Disk Detail — NAS Doctor</title>
<link rel="icon" type="image/png" href="/icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<script src="/js/charts.js"></script>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg-base:#0f1011;--bg-surface:#191a1b;--bg-elevated:#242526;
  --text-primary:#f7f8f8;--text-secondary:#d0d6e0;--text-tertiary:#8a8f98;
  --accent:#5e6ad2;--accent-hover:#7170ff;
  --green:#10b981;--green-bg:rgba(16,185,129,0.1);
  --amber:#d97706;--amber-bg:rgba(217,119,6,0.1);
  --red:#dc2626;--red-bg:rgba(220,38,38,0.1);
  --border:rgba(255,255,255,0.08);
  --radius:8px;--sp:8px;
}
html{background:var(--bg-base);color:var(--text-primary);font-family:"Inter",system-ui,-apple-system,sans-serif;font-feature-settings:"cv01","ss03";font-size:14px;line-height:1.5;-webkit-font-smoothing:antialiased}
body{min-height:100vh;padding:calc(var(--sp)*3)}
a{color:var(--accent);text-decoration:none}
a:hover{color:var(--accent-hover)}

.container{max-width:1200px;margin:0 auto}

/* ── Loading / Error ─────────────────────── */
.loading{display:flex;align-items:center;justify-content:center;height:60vh;flex-direction:column;gap:16px}
.loading-spinner{width:32px;height:32px;border:3px solid var(--border);border-top-color:var(--accent);border-radius:50%;animation:spin 0.8s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
.error-state{text-align:center;color:var(--red);padding:80px 0}
.error-state h2{margin-bottom:8px}

/* ── Header ──────────────────────────────── */
.page-header{display:flex;align-items:flex-start;justify-content:space-between;gap:32px;padding-bottom:calc(var(--sp)*3);margin-bottom:calc(var(--sp)*3);border-bottom:1px solid var(--border)}
.header-left{flex:1;min-width:0}
.back-link{display:inline-flex;align-items:center;gap:6px;font-size:13px;color:var(--text-tertiary);margin-bottom:12px;transition:color 0.15s}
.back-link:hover{color:var(--accent)}
.drive-title{font-size:28px;font-weight:700;letter-spacing:-0.8px;margin-bottom:8px;color:var(--text-primary)}
.badge-row{display:flex;flex-wrap:wrap;gap:8px}
.badge{display:inline-flex;align-items:center;padding:4px 10px;font-size:12px;font-weight:500;color:var(--text-tertiary);background:var(--bg-elevated);border:1px solid var(--border);border-radius:var(--radius)}
.badge-accent{color:var(--accent);background:rgba(94,106,210,0.08);border-color:rgba(94,106,210,0.2)}

/* ── Health Score Card ───────────────────── */
.health-score-card{flex-shrink:0;width:200px;background:var(--bg-surface);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:20px;text-align:center}
.health-gauge-wrap{width:160px;height:100px;margin:0 auto 8px}
.health-gauge-wrap canvas{width:160px;height:100px}
.health-status{font-size:14px;font-weight:700;letter-spacing:0.5px;margin-bottom:8px}
.health-status.passed{color:var(--green)}
.health-status.failed{color:var(--red)}
.health-meta{font-size:12px;color:var(--text-tertiary);line-height:1.6}
.drive-type-badge{display:inline-block;padding:2px 8px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;border-radius:4px;margin-top:6px;color:var(--accent);background:rgba(94,106,210,0.1)}

/* ── Section headings ────────────────────── */
.section-heading{font-size:13px;font-weight:600;color:var(--text-tertiary);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:calc(var(--sp)*2);margin-top:calc(var(--sp)*5)}

/* ── SMART Attributes Table ──────────────── */
.smart-table-wrap{background:var(--bg-surface);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden;margin-bottom:calc(var(--sp)*4)}
.smart-table{width:100%;border-collapse:collapse}
.smart-table th{font-size:11px;font-weight:600;color:var(--text-tertiary);text-transform:uppercase;letter-spacing:0.4px;padding:12px 16px;text-align:left;background:var(--bg-elevated);border-bottom:1px solid var(--border)}
.smart-table td{padding:12px 16px;border-bottom:1px solid var(--border);vertical-align:middle}
.smart-table tr:last-child td{border-bottom:none}
.smart-table tr:hover{background:rgba(255,255,255,0.02)}
.attr-name{font-weight:500;color:var(--text-primary)}
.attr-value{font-size:15px;font-weight:600;font-variant-numeric:tabular-nums}
.status-pill{display:inline-flex;align-items:center;gap:6px;padding:3px 10px;border-radius:20px;font-size:12px;font-weight:500}
.status-good{color:var(--green);background:var(--green-bg)}
.status-warn{color:var(--amber);background:var(--amber-bg)}
.status-bad{color:var(--red);background:var(--red-bg)}
.status-info{color:var(--accent);background:rgba(94,106,210,0.1)}
.sparkline-cell{width:140px}
.sparkline-cell canvas{display:block}

/* ── Chart sections ──────────────────────── */
.chart-card{background:var(--bg-surface);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);padding:20px;margin-bottom:calc(var(--sp)*3)}
.chart-card-title{font-size:14px;font-weight:600;color:var(--text-secondary);margin-bottom:16px}
.chart-wrap{width:100%;position:relative}
.chart-wrap canvas{width:100%;display:block}
.chart-grid-2{display:grid;grid-template-columns:1fr 1fr;gap:calc(var(--sp)*3);margin-bottom:calc(var(--sp)*4)}
@media(max-width:768px){.chart-grid-2{grid-template-columns:1fr}}

/* ── Findings ────────────────────────────── */
.findings-section{margin-bottom:calc(var(--sp)*4)}
.finding-card{border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-bottom:8px;transition:all 200ms ease}
.finding-card:hover{border-color:rgba(255,255,255,0.12)}
.finding-critical{background:rgba(220,38,38,0.06)}
.finding-critical:hover{background:rgba(220,38,38,0.10)}
.finding-warning{background:rgba(217,119,6,0.06)}
.finding-warning:hover{background:rgba(217,119,6,0.10)}
.finding-info{background:rgba(94,106,210,0.06)}
.finding-info:hover{background:rgba(94,106,210,0.10)}
.sev-dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:8px;flex-shrink:0}
.sev-critical{background:var(--red)}
.sev-warning{background:var(--amber)}
.sev-info{background:var(--accent)}
.finding-title-row{display:flex;align-items:center;margin-bottom:6px}
.finding-title{font-size:14px;font-weight:600;color:var(--text-primary)}
.finding-desc{font-size:13px;color:var(--text-secondary);margin-bottom:8px;line-height:1.5}
.finding-action-text{font-size:13px;color:var(--accent);line-height:1.5}
.finding-sev-tag{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;padding:2px 8px;border-radius:4px;margin-left:auto}
.finding-sev-tag.tag-critical{color:var(--red);background:var(--red-bg)}
.finding-sev-tag.tag-warning{color:var(--amber);background:var(--amber-bg)}
.finding-sev-tag.tag-info{color:var(--accent);background:rgba(94,106,210,0.1)}

/* ── Drive Info Table ────────────────────── */
.info-table-wrap{background:var(--bg-surface);border:1px solid var(--border);border-radius:calc(var(--radius)*1.5);overflow:hidden;margin-bottom:calc(var(--sp)*6)}
.info-table{width:100%;border-collapse:collapse}
.info-table td{padding:10px 16px;border-bottom:1px solid var(--border);font-size:13px}
.info-table tr:last-child td{border-bottom:none}
.info-table tr:hover{background:rgba(255,255,255,0.02)}
.info-table .label-cell{color:var(--text-tertiary);font-weight:500;width:200px}
.info-table .value-cell{color:var(--text-primary);font-variant-numeric:tabular-nums}

/* ── Empty states ────────────────────────── */
.no-data{text-align:center;color:var(--text-tertiary);padding:40px;font-size:13px}

/* ── Responsive ──────────────────────────── */
@media(max-width:700px){
  .page-header{flex-direction:column}
  .health-score-card{width:100%}
  .health-gauge-wrap{width:200px}
  .drive-title{font-size:22px}
}
</style>
</head>
<body>

<div class="container" id="app">
  <div class="loading" id="loadingState">
    <div class="loading-spinner"></div>
    <div style="color:var(--text-tertiary);font-size:13px">Loading disk data...</div>
  </div>
</div>

<script>
(function(){
  "use strict";

  /* ── Extract serial from URL path ─────────────────────────────── */
  var pathParts = window.location.pathname.split("/");
  var serial = "";
  for (var i = 0; i < pathParts.length; i++) {
    if (pathParts[i] === "disk" && i + 1 < pathParts.length) {
      serial = decodeURIComponent(pathParts[i + 1]);
      break;
    }
  }

  if (!serial) {
    showError("No disk serial found in URL.");
    return;
  }

  /* ── Fetch disk data ──────────────────────────────────────────── */
  fetch("/api/v1/disks/" + encodeURIComponent(serial))
    .then(function(resp) {
      if (!resp.ok) throw new Error("HTTP " + resp.status);
      return resp.json();
    })
    .then(function(data) {
      renderPage(data);
    })
    .catch(function(err) {
      showError("Failed to load disk data: " + err.message);
    });

  /* ── Error state ──────────────────────────────────────────────── */
  function showError(msg) {
    var app = document.getElementById("app");
    app.innerHTML = "<div class=\"error-state\">" +
      "<h2>Error</h2>" +
      "<p>" + escHtml(msg) + "</p>" +
      "<p style=\"margin-top:16px\"><a href=\"/\">Back to Dashboard</a></p>" +
      "</div>";
  }

  function escHtml(s) {
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }

  /* ── Compute health score ─────────────────────────────────────── */
  function computeHealthScore(disk) {
    var score = 100;
    if (!disk) return 0;
    /* SMART test failed = -40 */
    if (disk.health_passed === false) score -= 40;
    /* Reallocated sectors: -5 per sector, max -30 */
    var realloc = disk.reallocated || 0;
    if (realloc > 0) score -= Math.min(30, realloc * 5);
    /* Pending sectors: -4 per sector, max -20 */
    var pending = disk.pending || 0;
    if (pending > 0) score -= Math.min(20, pending * 4);
    /* UDMA CRC errors: -1 per 10, max -10 */
    var crc = disk.udma_crc || 0;
    if (crc > 0) score -= Math.min(10, Math.ceil(crc / 10));
    /* Command timeout: -1 per 5, max -10 */
    var ct = disk.command_timeout || 0;
    if (ct > 5) score -= Math.min(10, Math.ceil((ct - 5) / 5));
    /* High temperature: subtract if >= 50 */
    var temp = disk.temperature || 0;
    if (temp >= 55) score -= 15;
    else if (temp >= 50) score -= 8;
    else if (temp >= 45) score -= 3;
    /* Offline uncorrectable */
    var offline = disk.offline_uncorrectable || 0;
    if (offline > 0) score -= Math.min(15, offline * 5);
    return Math.max(0, Math.min(100, score));
  }

  /* ── Format helpers ───────────────────────────────────────────── */
  function formatHours(h) {
    if (!h && h !== 0) return "—";
    var years = (h / 8766).toFixed(1);
    return h.toLocaleString() + " hrs (" + years + " years)";
  }

  function formatSize(gb) {
    if (!gb) return "—";
    if (gb >= 1000) return (gb / 1000).toFixed(1) + " TB";
    return gb + " GB";
  }

  function tempStatus(t) {
    if (t < 40) return { cls: "status-good", label: "Normal" };
    if (t < 50) return { cls: "status-warn", label: "Warm" };
    return { cls: "status-bad", label: "Hot" };
  }

  function reallocStatus(v) {
    if (v === 0) return { cls: "status-good", label: "OK" };
    return { cls: "status-bad", label: "Bad" };
  }

  function pendingStatus(v) {
    if (v === 0) return { cls: "status-good", label: "OK" };
    return { cls: "status-warn", label: "Warning" };
  }

  function crcStatus(v) {
    if (v === 0) return { cls: "status-good", label: "OK" };
    return { cls: "status-warn", label: "Warning" };
  }

  function ctStatus(v) {
    if (v <= 5) return { cls: "status-good", label: "OK" };
    if (v <= 20) return { cls: "status-warn", label: "Warning" };
    return { cls: "status-bad", label: "Bad" };
  }

  function formatTimestamp(ts) {
    if (!ts) return "";
    var d = new Date(ts);
    var mo = d.getMonth() + 1;
    var dy = d.getDate();
    var hr = d.getHours();
    var mn = d.getMinutes();
    return mo + "/" + dy + " " + (hr < 10 ? "0" : "") + hr + ":" + (mn < 10 ? "0" : "") + mn;
  }

  function driveTypeLabel(dt) {
    if (!dt) return "HDD";
    var t = dt.toLowerCase();
    if (t.indexOf("nvme") >= 0) return "NVMe";
    if (t.indexOf("ssd") >= 0 || t.indexOf("solid") >= 0) return "SSD";
    return "HDD";
  }

  /* ── Extract history arrays ───────────────────────────────────── */
  function extractHistory(hist, key) {
    var arr = [];
    for (var i = 0; i < hist.length; i++) {
      arr.push(hist[i][key] !== undefined && hist[i][key] !== null ? hist[i][key] : 0);
    }
    return arr;
  }

  function extractLabels(hist) {
    var arr = [];
    for (var i = 0; i < hist.length; i++) {
      arr.push(formatTimestamp(hist[i].timestamp));
    }
    return arr;
  }

  /* ── Render the full page ─────────────────────────────────────── */
  function renderPage(data) {
    var disk = data.current || {};
    var history = data.history || [];
    var findings = data.findings || [];
    var score = computeHealthScore(disk);

    var html = "";

    /* ── Header ────────────────────────────────────────────────── */
    html += "<div class=\"page-header\">";
    html += "<div class=\"header-left\">";
    html += "<a href=\"/\" class=\"back-link\">";
    html += "<svg width=\"16\" height=\"16\" viewBox=\"0 0 16 16\" fill=\"none\" xmlns=\"http://www.w3.org/2000/svg\">";
    html += "<path d=\"M10 12L6 8L10 4\" stroke=\"currentColor\" stroke-width=\"1.5\" stroke-linecap=\"round\" stroke-linejoin=\"round\"/>";
    html += "</svg> Dashboard</a>";
    html += "<h1 class=\"drive-title\">" + escHtml(disk.model || "Unknown Drive") + "</h1>";
    html += "<div class=\"badge-row\">";
    html += "<span class=\"badge\">" + escHtml(disk.serial || serial) + "</span>";
    if (disk.device) html += "<span class=\"badge\">" + escHtml(disk.device) + "</span>";
    if (disk.array_slot) html += "<span class=\"badge badge-accent\">" + escHtml(disk.array_slot) + "</span>";
    if (disk.firmware) html += "<span class=\"badge\">FW: " + escHtml(disk.firmware) + "</span>";
    if (disk.size_gb) html += "<span class=\"badge\">" + formatSize(disk.size_gb) + "</span>";
    html += "</div>";
    html += "</div>";

    /* ── Health Score Card ─────────────────────────────────────── */
    html += "<div class=\"health-score-card\">";
    html += "<div class=\"health-gauge-wrap\"><canvas id=\"healthGauge\" width=\"160\" height=\"100\"></canvas></div>";
    var passed = disk.health_passed !== false;
    html += "<div class=\"health-status " + (passed ? "passed" : "failed") + "\">" + (passed ? "PASSED" : "FAILED") + "</div>";
    html += "<div class=\"health-meta\">";
    if (disk.power_on_hours !== undefined && disk.power_on_hours !== null) {
      var ageYears = (disk.power_on_hours / 8766).toFixed(1);
      html += "Age: " + ageYears + " years<br>";
    }
    html += "</div>";
    html += "<div class=\"drive-type-badge\">" + driveTypeLabel(disk.disk_type) + "</div>";
    html += "</div>";
    html += "</div>"; /* end page-header */

    /* ── SMART Attributes Table ────────────────────────────────── */
    html += "<div class=\"section-heading\">SMART Attributes</div>";
    html += "<div class=\"smart-table-wrap\">";
    html += "<table class=\"smart-table\">";
    html += "<thead><tr>";
    html += "<th>Attribute</th><th>Current Value</th><th>Status</th><th>Trend</th>";
    html += "</tr></thead>";
    html += "<tbody>";

    var tempVal = disk.temperature || 0;
    var tempSt = tempStatus(tempVal);
    html += "<tr>";
    html += "<td class=\"attr-name\">Temperature</td>";
    html += "<td class=\"attr-value\">" + tempVal + "&deg;C</td>";
    html += "<td><span class=\"status-pill " + tempSt.cls + "\">" + tempSt.label + "</span></td>";
    html += "<td class=\"sparkline-cell\"><canvas id=\"sparkTemp\"></canvas></td>";
    html += "</tr>";

    var reallocVal = disk.reallocated || 0;
    var reallocSt = reallocStatus(reallocVal);
    html += "<tr>";
    html += "<td class=\"attr-name\">Reallocated Sectors</td>";
    html += "<td class=\"attr-value\">" + reallocVal.toLocaleString() + "</td>";
    html += "<td><span class=\"status-pill " + reallocSt.cls + "\">" + reallocSt.label + "</span></td>";
    html += "<td class=\"sparkline-cell\"><canvas id=\"sparkRealloc\"></canvas></td>";
    html += "</tr>";

    var pendingVal = disk.pending || 0;
    var pendingSt = pendingStatus(pendingVal);
    html += "<tr>";
    html += "<td class=\"attr-name\">Pending Sectors</td>";
    html += "<td class=\"attr-value\">" + pendingVal.toLocaleString() + "</td>";
    html += "<td><span class=\"status-pill " + pendingSt.cls + "\">" + pendingSt.label + "</span></td>";
    html += "<td class=\"sparkline-cell\"><canvas id=\"sparkPending\"></canvas></td>";
    html += "</tr>";

    var crcVal = disk.udma_crc || 0;
    var crcSt = crcStatus(crcVal);
    html += "<tr>";
    html += "<td class=\"attr-name\">UDMA CRC Errors</td>";
    html += "<td class=\"attr-value\">" + crcVal.toLocaleString() + "</td>";
    html += "<td><span class=\"status-pill " + crcSt.cls + "\">" + crcSt.label + "</span></td>";
    html += "<td class=\"sparkline-cell\"><canvas id=\"sparkCrc\"></canvas></td>";
    html += "</tr>";

    var ctVal = disk.command_timeout || 0;
    var ctSt = ctStatus(ctVal);
    html += "<tr>";
    html += "<td class=\"attr-name\">Command Timeout</td>";
    html += "<td class=\"attr-value\">" + ctVal.toLocaleString() + "</td>";
    html += "<td><span class=\"status-pill " + ctSt.cls + "\">" + ctSt.label + "</span></td>";
    html += "<td class=\"sparkline-cell\"><canvas id=\"sparkCt\"></canvas></td>";
    html += "</tr>";

    var offlineVal = disk.offline_uncorrectable || 0;
    var offlineSt = offlineVal === 0 ? { cls: "status-good", label: "OK" } : { cls: "status-bad", label: "Bad" };
    html += "<tr>";
    html += "<td class=\"attr-name\">Offline Uncorrectable</td>";
    html += "<td class=\"attr-value\">" + offlineVal.toLocaleString() + "</td>";
    html += "<td><span class=\"status-pill " + offlineSt.cls + "\">" + offlineSt.label + "</span></td>";
    html += "<td class=\"sparkline-cell\">—</td>";
    html += "</tr>";

    var pohVal = disk.power_on_hours || 0;
    var pohYears = (pohVal / 8766).toFixed(1);
    html += "<tr>";
    html += "<td class=\"attr-name\">Power On Hours</td>";
    html += "<td class=\"attr-value\">" + pohVal.toLocaleString() + "</td>";
    html += "<td><span class=\"status-pill status-info\">" + pohYears + " years</span></td>";
    html += "<td class=\"sparkline-cell\">—</td>";
    html += "</tr>";

    html += "</tbody></table></div>";

    /* ── Temperature History Chart ─────────────────────────────── */
    html += "<div class=\"section-heading\">Temperature History</div>";
    if (history.length > 1) {
      html += "<div class=\"chart-card\">";
      html += "<div class=\"chart-wrap\"><canvas id=\"chartTemp\" height=\"220\"></canvas></div>";
      html += "</div>";
    } else {
      html += "<div class=\"chart-card\"><div class=\"no-data\">Not enough history data for temperature chart</div></div>";
    }

    /* ── SMART Trend Charts (2-col grid) ───────────────────────── */
    html += "<div class=\"section-heading\">SMART Trends</div>";
    if (history.length > 1) {
      html += "<div class=\"chart-grid-2\">";
      html += "<div class=\"chart-card\">";
      html += "<div class=\"chart-card-title\">Reallocated Sectors</div>";
      html += "<div class=\"chart-wrap\"><canvas id=\"chartRealloc\" height=\"180\"></canvas></div>";
      html += "</div>";
      html += "<div class=\"chart-card\">";
      html += "<div class=\"chart-card-title\">Pending Sectors</div>";
      html += "<div class=\"chart-wrap\"><canvas id=\"chartPending\" height=\"180\"></canvas></div>";
      html += "</div>";
      html += "<div class=\"chart-card\">";
      html += "<div class=\"chart-card-title\">UDMA CRC Errors</div>";
      html += "<div class=\"chart-wrap\"><canvas id=\"chartCrc\" height=\"180\"></canvas></div>";
      html += "</div>";
      html += "<div class=\"chart-card\">";
      html += "<div class=\"chart-card-title\">Command Timeout</div>";
      html += "<div class=\"chart-wrap\"><canvas id=\"chartCt\" height=\"180\"></canvas></div>";
      html += "</div>";
      html += "</div>";
    } else {
      html += "<div class=\"chart-card\"><div class=\"no-data\">Not enough history data for trend charts</div></div>";
    }

    /* ── Findings ──────────────────────────────────────────────── */
    if (findings.length > 0) {
      html += "<div class=\"section-heading\">Related Findings</div>";
      html += "<div class=\"findings-section\">";
      for (var fi = 0; fi < findings.length; fi++) {
        var f = findings[fi];
        var sev = f.severity || "info";
        var cardCls = "finding-card finding-" + sev;
        var dotCls = "sev-dot sev-" + sev;
        var tagCls = "finding-sev-tag tag-" + sev;
        html += "<div class=\"" + cardCls + "\">";
        html += "<div class=\"finding-title-row\">";
        html += "<span class=\"" + dotCls + "\"></span>";
        html += "<span class=\"finding-title\">" + escHtml(f.title || "") + "</span>";
        html += "<span class=\"" + tagCls + "\">" + escHtml(sev) + "</span>";
        html += "</div>";
        if (f.description) html += "<div class=\"finding-desc\">" + escHtml(f.description) + "</div>";
        if (f.action) html += "<div class=\"finding-action-text\">" + escHtml(f.action) + "</div>";
        html += "</div>";
      }
      html += "</div>";
    }

    /* ── Drive Identification ──────────────────────────────────── */
    html += "<div class=\"section-heading\">Drive Identification</div>";
    html += "<div class=\"info-table-wrap\"><table class=\"info-table\">";
    var rows = [
      ["Device", disk.device || "—"],
      ["Model", disk.model || "—"],
      ["Serial Number", disk.serial || serial],
      ["Firmware", disk.firmware || "—"],
      ["Size", formatSize(disk.size_gb)],
      ["Type", driveTypeLabel(disk.disk_type)],
      ["ATA Port", disk.ata_port || "—"],
      ["Array Slot", disk.array_slot || "—"],
      ["Power On Hours", pohVal.toLocaleString()],
      ["Age", pohYears + " years"],
      ["Temperature", tempVal + "\u00B0C"],
      ["Max Temperature", (disk.temperature_max_c || "—") + (disk.temperature_max_c ? "\u00B0C" : "")]
    ];
    for (var ri = 0; ri < rows.length; ri++) {
      html += "<tr><td class=\"label-cell\">" + escHtml(rows[ri][0]) + "</td><td class=\"value-cell\">" + escHtml(String(rows[ri][1])) + "</td></tr>";
    }
    html += "</table></div>";

    /* ── Inject ────────────────────────────────────────────────── */
    document.getElementById("app").innerHTML = html;
    document.title = (disk.model || "Disk") + " — NAS Doctor";

    /* ── Render charts after DOM update ────────────────────────── */
    setTimeout(function() { renderCharts(disk, history); }, 50);
  }

  /* ── Render all charts ────────────────────────────────────────── */
  function renderCharts(disk, history) {
    var score = computeHealthScore(disk);

    /* Health Gauge */
    NasChart.gauge("healthGauge", {
      value: score,
      max: 100,
      label: "Health Score",
      width: 160,
      height: 100,
      thresholds: { good: 80, warn: 50 }
    });

    /* Sparklines */
    if (history.length > 1) {
      var tempData = extractHistory(history, "temperature");
      var reallocData = extractHistory(history, "reallocated");
      var pendingData = extractHistory(history, "pending");
      var crcData = extractHistory(history, "udma_crc");
      var ctData = extractHistory(history, "command_timeout");

      var sparkColor = function(data, zeroGood) {
        var mx = Math.max.apply(null, data);
        if (zeroGood) return mx > 0 ? "#d97706" : "#10b981";
        return "#5e6ad2";
      };

      NasChart.sparkline("sparkTemp", { data: tempData, color: disk.temperature >= 50 ? "#dc2626" : disk.temperature >= 40 ? "#d97706" : "#10b981", width: 120, height: 28 });
      NasChart.sparkline("sparkRealloc", { data: reallocData, color: sparkColor(reallocData, true), width: 120, height: 28 });
      NasChart.sparkline("sparkPending", { data: pendingData, color: sparkColor(pendingData, true), width: 120, height: 28 });
      NasChart.sparkline("sparkCrc", { data: crcData, color: sparkColor(crcData, true), width: 120, height: 28 });
      NasChart.sparkline("sparkCt", { data: ctData, color: sparkColor(ctData, true), width: 120, height: 28 });

      /* ── Temperature Line Chart ────────────────────────────── */
      var labels = extractLabels(history);

      /* Build warning/critical threshold line arrays */
      var warnLine = [];
      var critLine = [];
      for (var ti = 0; ti < tempData.length; ti++) {
        warnLine.push(45);
        critLine.push(55);
      }

      NasChart.line("chartTemp", {
        datasets: [
          { data: tempData, color: "#5e6ad2", label: "Temperature" },
          { data: warnLine, color: "#d97706", label: "Warning (45\u00B0C)", dashed: true },
          { data: critLine, color: "#dc2626", label: "Critical (55\u00B0C)", dashed: true }
        ],
        labels: labels,
        yLabel: "\u00B0C"
      });

      /* ── Trend Charts ──────────────────────────────────────── */
      var hasNonZeroRealloc = Math.max.apply(null, reallocData) > 0;
      NasChart.area("chartRealloc", {
        datasets: [{ data: reallocData, color: hasNonZeroRealloc ? "#dc2626" : "#10b981", label: "Reallocated" }],
        labels: labels,
        yLabel: "Sectors"
      });

      NasChart.area("chartPending", {
        datasets: [{ data: pendingData, color: Math.max.apply(null, pendingData) > 0 ? "#d97706" : "#10b981", label: "Pending" }],
        labels: labels,
        yLabel: "Sectors"
      });

      NasChart.line("chartCrc", {
        datasets: [{ data: crcData, color: "#d97706", label: "UDMA CRC Errors" }],
        labels: labels,
        yLabel: "Errors"
      });

      NasChart.line("chartCt", {
        datasets: [{ data: ctData, color: "#d97706", label: "Command Timeout" }],
        labels: labels,
        yLabel: "Count"
      });
    }
  }

})();
</script>
</body>
</html>`

// handleDiskPage serves the disk detail HTML page.
// GET /disk/{serial}
func (s *Server) handleDiskPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(DiskDetailPage))
}

// ---------- Internal helpers ----------

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
