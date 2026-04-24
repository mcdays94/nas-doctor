// NAS Doctor — Local NAS diagnostic and monitoring tool.
//
// Usage:
//
//	nas-doctor [flags]
//
// Flags:
//
//	-listen     HTTP listen address (default ":8080")
//	-data       Data directory for SQLite database (default "/data")
//	-interval   Diagnostic scan interval (default "30m")
//	-config     Path to JSON config file (optional)
//	-demo       Run with realistic mock data (for previewing themes)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/analyzer"
	"github.com/mcdays94/nas-doctor/internal/api"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/demo"
	"github.com/mcdays94/nas-doctor/internal/fleet"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// version is set at build time via -ldflags="-X main.version=x.y.z".
// Falls back to "dev" for local builds.
var version = "dev"

func main() {
	// Flags
	listenAddr := flag.String("listen", normalizeListenAddr(envOr("NAS_DOCTOR_LISTEN", ":8060")), "HTTP listen address")
	dataDir := flag.String("data", envOr("NAS_DOCTOR_DATA", "/tmp/nas-doctor-data"), "Data directory for SQLite DB")
	intervalStr := flag.String("interval", envOr("NAS_DOCTOR_INTERVAL", "30m"), "Diagnostic scan interval")
	configPath := flag.String("config", envOr("NAS_DOCTOR_CONFIG", ""), "Path to JSON config file")
	demoMode := flag.Bool("demo", false, "Run with realistic mock data (for previewing themes)")
	flag.Parse()

	// Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	mode := "production"
	if *demoMode {
		mode = "demo"
	}
	logger.Info("NAS Doctor starting",
		"version", version,
		"mode", mode,
		"listen", *listenAddr,
		"data", *dataDir,
	)

	// Parse scan interval
	interval, err := time.ParseDuration(*intervalStr)
	if err != nil {
		logger.Error("invalid scan interval", "interval", *intervalStr, "error", err)
		os.Exit(1)
	}

	// Load config
	cfg := defaultConfig(*listenAddr, *dataDir)
	if *configPath != "" {
		if err := loadConfig(*configPath, &cfg); err != nil {
			logger.Error("failed to load config", "path", *configPath, "error", err)
			os.Exit(1)
		}
	}
	applyEnvOverrides(&cfg)

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		logger.Error("failed to create data directory", "path", cfg.DataDir, "error", err)
		os.Exit(1)
	}

	// Issue #227 — detect the "/data is not bind-mounted" footgun at startup.
	// Runs only in production (demo mode writes to a tmpfs path by default
	// and we don't want to scold users previewing the dashboard).
	dataPersistent := true
	if !*demoMode {
		dataPersistent = warnIfDataEphemeral(logger, cfg.DataDir)
	}

	// Open database
	dbPath := filepath.Join(cfg.DataDir, "nas-doctor.db")
	store, err := storage.Open(dbPath, logger)
	if err != nil {
		logger.Error("failed to open database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer store.Close()

	persistedSettings := loadPersistedSettings(store, logger)

	// Create Prometheus metrics
	metrics := notifier.NewMetrics()

	var sched *scheduler.Scheduler

	// Create collector (shared between demo and production paths)
	coll := collector.New(cfg.HostPaths, logger)

	if *demoMode {
		// Demo mode: inject fake data with historical snapshots
		logger.Info("demo mode: generating mock diagnostic data with history")

		sched = scheduler.New(coll, store, nil, metrics, logger, 24*time.Hour)

		// Generate 30 days of historical snapshots (one per day) for chart data
		for i := 29; i >= 0; i-- {
			histSnap := demo.GenerateSnapshot()
			histSnap.Timestamp = time.Now().Add(-time.Duration(i) * 24 * time.Hour)
			histSnap.ID = fmt.Sprintf("demo-hist-%02d", i)
			// Vary the data slightly per day to make charts interesting
			histSnap.System.CPUUsage = demo.Jitter(23.4, 40)
			histSnap.System.MemPercent = demo.Jitter(75.0, 15)
			histSnap.System.IOWait = demo.Jitter(18.3, 30)
			histSnap.System.LoadAvg1 = demo.Jitter(2.34, 50)
			histSnap.System.LoadAvg5 = demo.Jitter(1.87, 40)
			for j := range histSnap.SMART {
				histSnap.SMART[j].Temperature = int(demo.Jitter(float64(histSnap.SMART[j].Temperature), 15))
				// Jitter SMART attributes to make trend charts interesting
				if histSnap.SMART[j].UDMACRC > 0 {
					// CRC errors accumulate over time — simulate gradual increase
					histSnap.SMART[j].UDMACRC = int64(float64(histSnap.SMART[j].UDMACRC) * float64(30-i) / 30.0)
				}
				if histSnap.SMART[j].CommandTimeout > 5 {
					histSnap.SMART[j].CommandTimeout = int64(demo.Jitter(float64(histSnap.SMART[j].CommandTimeout)*float64(30-i)/30.0, 25))
					if histSnap.SMART[j].CommandTimeout < 0 {
						histSnap.SMART[j].CommandTimeout = 0
					}
				}
				if histSnap.SMART[j].Reallocated > 0 {
					// Reallocated sectors grow over time
					histSnap.SMART[j].Reallocated = int64(float64(histSnap.SMART[j].Reallocated) * float64(30-i) / 30.0)
					if histSnap.SMART[j].Reallocated < 0 {
						histSnap.SMART[j].Reallocated = 0
					}
				}
				if histSnap.SMART[j].Pending > 0 {
					histSnap.SMART[j].Pending = int64(demo.Jitter(float64(histSnap.SMART[j].Pending)*float64(30-i)/30.0, 30))
					if histSnap.SMART[j].Pending < 0 {
						histSnap.SMART[j].Pending = 0
					}
				}
			}
			// Jitter GPU metrics
			if histSnap.GPU != nil && histSnap.GPU.Available {
				for j := range histSnap.GPU.GPUs {
					histSnap.GPU.GPUs[j].UsagePct = demo.Jitter(34, 60)
					histSnap.GPU.GPUs[j].Temperature = int(demo.Jitter(62, 20))
					histSnap.GPU.GPUs[j].MemUsedMB = demo.Jitter(2048, 40)
					histSnap.GPU.GPUs[j].PowerW = demo.Jitter(85, 30)
					histSnap.GPU.GPUs[j].EncoderPct = demo.Jitter(40, 80)
					histSnap.GPU.GPUs[j].DecoderPct = demo.Jitter(30, 70)
					if histSnap.GPU.GPUs[j].MemTotalMB > 0 {
						histSnap.GPU.GPUs[j].MemPct = (histSnap.GPU.GPUs[j].MemUsedMB / histSnap.GPU.GPUs[j].MemTotalMB) * 100
					}
				}
			}
			histSnap.Findings = analyzer.Analyze(histSnap)
			if err := store.SaveSnapshot(histSnap); err != nil {
				logger.Warn("failed to save historical demo snapshot", "day", i, "error", err)
			}
		}

		// Generate hourly GPU + container snapshots for the past 48h (for 1h/1d chart granularity)
		for h := 47; h >= 1; h-- {
			gpuSnap := demo.GenerateSnapshot()
			gpuSnap.Timestamp = time.Now().Add(-time.Duration(h) * time.Hour)
			gpuSnap.ID = fmt.Sprintf("demo-gpu-h%03d", h)
			// Vary GPU metrics with a time-of-day pattern (higher during "day" hours)
			hourOfDay := gpuSnap.Timestamp.Hour()
			dayFactor := 1.0
			if hourOfDay >= 9 && hourOfDay <= 23 {
				dayFactor = 1.5 // busier during day
			}
			gpuSnap.System.CPUUsage = demo.Jitter(23.4*dayFactor, 30)
			gpuSnap.System.MemPercent = demo.Jitter(75.0, 10)
			gpuSnap.System.IOWait = demo.Jitter(18.3, 25)
			if gpuSnap.GPU != nil {
				for j := range gpuSnap.GPU.GPUs {
					gpuSnap.GPU.GPUs[j].UsagePct = demo.Jitter(30*dayFactor, 50)
					if gpuSnap.GPU.GPUs[j].UsagePct > 100 {
						gpuSnap.GPU.GPUs[j].UsagePct = 100
					}
					gpuSnap.GPU.GPUs[j].Temperature = int(demo.Jitter(55+7*dayFactor, 15))
					gpuSnap.GPU.GPUs[j].MemUsedMB = demo.Jitter(1800*dayFactor, 35)
					gpuSnap.GPU.GPUs[j].PowerW = demo.Jitter(70*dayFactor, 30)
					gpuSnap.GPU.GPUs[j].EncoderPct = demo.Jitter(35*dayFactor, 60)
					gpuSnap.GPU.GPUs[j].DecoderPct = demo.Jitter(25*dayFactor, 50)
					if gpuSnap.GPU.GPUs[j].MemTotalMB > 0 {
						gpuSnap.GPU.GPUs[j].MemPct = (gpuSnap.GPU.GPUs[j].MemUsedMB / gpuSnap.GPU.GPUs[j].MemTotalMB) * 100
					}
				}
			}
			// Vary per-container metrics with time-of-day pattern
			for j := range gpuSnap.Docker.Containers {
				c := &gpuSnap.Docker.Containers[j]
				if c.State != "running" {
					continue
				}
				c.CPU = demo.Jitter(c.CPU*dayFactor, 40)
				if c.CPU < 0 {
					c.CPU = 0.1
				}
				c.MemMB = demo.Jitter(c.MemMB, 20)
				if c.MemMB < 1 {
					c.MemMB = 1
				}
				c.MemPct = demo.Jitter(c.MemPct, 20)
				if c.MemPct < 0.1 {
					c.MemPct = 0.1
				}
				c.NetIn = demo.Jitter(c.NetIn, 15)
				c.NetOut = demo.Jitter(c.NetOut, 15)
				c.BlockRead = demo.Jitter(c.BlockRead, 10)
				c.BlockWrite = demo.Jitter(c.BlockWrite, 10)
			}
			gpuSnap.Findings = analyzer.Analyze(gpuSnap)
			if err := store.SaveSnapshot(gpuSnap); err != nil {
				logger.Warn("failed to save hourly demo snapshot", "hour", h, "error", err)
			}
		}

		// Generate process history for the past 48h (every 5 minutes = 576 snapshots)
		for m := 576; m >= 1; m-- {
			procSnap := demo.GenerateSnapshot()
			ts := time.Now().Add(-time.Duration(m) * 5 * time.Minute)
			hourOfDay := ts.Hour()
			dayFactor := 1.0
			if hourOfDay >= 9 && hourOfDay <= 23 {
				dayFactor = 1.5
			}
			for j := range procSnap.System.TopProcesses {
				p := &procSnap.System.TopProcesses[j]
				p.CPU = demo.Jitter(p.CPU*dayFactor, 40)
				if p.CPU < 0.1 {
					p.CPU = 0.1
				}
				p.Mem = demo.Jitter(p.Mem, 30)
				if p.Mem < 0.1 {
					p.Mem = 0.1
				}
			}
			if err := store.SaveProcessStatsAt(procSnap.System.TopProcesses, ts); err != nil {
				logger.Warn("failed to save demo process history", "m", m, "error", err)
			}
		}

		// Current snapshot (latest)
		snap := demo.GenerateSnapshot()
		snap.Findings = analyzer.Analyze(snap)
		if err := store.SaveSnapshot(snap); err != nil {
			logger.Error("failed to save demo snapshot", "error", err)
		}
		metrics.Update(snap)

		// Inject the demo snapshot into the scheduler's in-memory cache
		// so that Latest() returns it for the report and status endpoints.
		sched.SetLatest(snap)

		// Demo service check configs — persist to settings DB so they appear
		// in the editable list, and push to scheduler for the check loop.
		demoChecks := demo.DemoServiceCheckConfigs()
		sched.UpdateServiceChecks(demoChecks)
		{
			// Build a minimal settings object with just the checks, then
			// merge into whatever exists (or create fresh).
			raw, _ := store.GetConfig("settings")
			s := map[string]interface{}{}
			if raw != "" {
				json.Unmarshal([]byte(raw), &s)
			}
			checksJSON, _ := json.Marshal(demoChecks)
			var checksArr interface{}
			json.Unmarshal(checksJSON, &checksArr)
			s["service_checks"] = map[string]interface{}{"checks": checksArr}
			if data, err := json.Marshal(s); err == nil {
				store.SetConfig("settings", string(data))
			}
		}

		// Generate demo service check history (7 days of data)
		for i := 7 * 24 * 12; i >= 0; i-- { // every 5 minutes for 7 days
			ts := time.Now().Add(-time.Duration(i) * 5 * time.Minute)
			for _, sc := range snap.Services {
				sc.CheckedAt = ts.Format(time.RFC3339)
				sc.ResponseMS = int64(demo.Jitter(float64(sc.ResponseMS), 40))
				if sc.ResponseMS < 1 {
					sc.ResponseMS = 1
				}
				// Simulate occasional failures
				if sc.Status == "down" || (i > 0 && i%97 == 0 && sc.Name == "NFS Share") {
					sc.Status = "down"
					sc.Error = "connection refused"
					sc.ConsecutiveFailures++
				} else {
					sc.Status = "up"
					sc.Error = ""
					sc.ConsecutiveFailures = 0
				}
			}
			_ = store.SaveServiceCheckResults(snap.Services)
		}

		logger.Info("demo data loaded",
			"findings", len(snap.Findings),
			"critical", countSev(snap.Findings, internal.SeverityCritical),
			"warnings", countSev(snap.Findings, internal.SeverityWarning),
		)
	} else {
		// Production mode: real collectors
		interval = intervalFromSettings(persistedSettings, interval, logger)

		webhooks := cfg.Notifications.Webhooks
		if persistedSettings != nil && persistedSettings.Notifications.Webhooks != nil {
			webhooks = persistedSettings.Notifications.Webhooks
		}
		notif := buildNotifier(webhooks, logger, store)

		sched = scheduler.New(coll, store, notif, metrics, logger, interval)
		applySchedulerSettingsFromStore(sched, persistedSettings)
		// Defense-in-depth: guarantee the scheduler's in-memory service check
		// set matches whatever history the DB carries, even when the settings
		// payload failed to load or was nil (empty DB, corrupt JSON). Without
		// this, a prior-boot orphan row in service_checks_history would
		// continue to surface on /api/v1/service-checks as a phantom check
		// the user cannot remove via the settings UI. Issue #181.
		if pruned, err := sched.PurgeOrphanServiceCheckHistory(); err != nil {
			logger.Warn("startup: purge orphan service check history", "error", err)
		} else if pruned > 0 {
			logger.Info("startup: pruned orphan service check history", "rows", pruned)
		}
		// Apply Proxmox config to collector on startup
		if persistedSettings != nil && persistedSettings.Proxmox.Enabled {
			coll.SetProxmoxConfig(collector.ProxmoxConfig{
				Enabled:  true,
				URL:      persistedSettings.Proxmox.URL,
				TokenID:  persistedSettings.Proxmox.TokenID,
				Secret:   persistedSettings.Proxmox.Secret,
				NodeName: persistedSettings.Proxmox.NodeName,
				Alias:    persistedSettings.Proxmox.Alias,
			})
			logger.Info("proxmox integration loaded", "url", persistedSettings.Proxmox.URL)
		}
		if persistedSettings != nil && persistedSettings.Kubernetes.Enabled {
			coll.SetKubeConfig(collector.KubeConfig{
				Enabled:   true,
				URL:       persistedSettings.Kubernetes.URL,
				Token:     persistedSettings.Kubernetes.Token,
				Alias:     persistedSettings.Kubernetes.Alias,
				InCluster: persistedSettings.Kubernetes.InCluster,
			})
			logger.Info("kubernetes integration loaded", "url", persistedSettings.Kubernetes.URL, "in_cluster", persistedSettings.Kubernetes.InCluster)
		}
		// Apply SMART standby-awareness preference on startup (#198). Default
		// (false) uses `-n standby` so spun-down drives aren't woken by scans.
		// Moved from Settings.SMART → Settings.AdvancedScans.SMART in
		// schema v3 (#259).
		if persistedSettings != nil {
			coll.SetSMARTConfig(collector.SMARTConfig{
				WakeDrives: persistedSettings.AdvancedScans.SMART.WakeDrives,
			})
			// Apply the max-age force-wake threshold on startup (#238).
			// Scheduler owns this policy; 0 disables the safety net.
			sched.SetSMARTMaxAgeDays(persistedSettings.AdvancedScans.SMART.MaxAgeDays)
			// Apply per-subsystem scan intervals on startup (#260).
			// The scheduler's dispatcher is the source of truth for
			// "what runs when" — without this push, fresh boots
			// would silently run every subsystem on the global
			// cadence regardless of persisted settings.
			sched.SetDispatcherIntervals(scheduler.DispatcherIntervalsConfig{
				SMARTSec:      persistedSettings.AdvancedScans.SMART.IntervalSec,
				DockerSec:     persistedSettings.AdvancedScans.Docker.IntervalSec,
				ProxmoxSec:    persistedSettings.AdvancedScans.Proxmox.IntervalSec,
				KubernetesSec: persistedSettings.AdvancedScans.Kubernetes.IntervalSec,
				ZFSSec:        persistedSettings.AdvancedScans.ZFS.IntervalSec,
				GPUSec:        persistedSettings.AdvancedScans.GPU.IntervalSec,
			}, interval)
			// Apply external Borg monitor config on startup (#279).
			// The collector reads this on every backup tick — without
			// it, configured repos wouldn't show up until the user
			// re-saved settings.
			if len(persistedSettings.BackupMonitor.Borg) > 0 {
				ext := make([]collector.BorgExternalRepo, 0, len(persistedSettings.BackupMonitor.Borg))
				for _, r := range persistedSettings.BackupMonitor.Borg {
					ext = append(ext, collector.BorgExternalRepo{
						Enabled:       r.Enabled,
						Label:         r.Label,
						RepoPath:      r.RepoPath,
						BinaryPath:    r.BinaryPath,
						PassphraseEnv: r.PassphraseEnv,
						SSHKeyPath:    r.SSHKeyPath,
					})
				}
				coll.SetBackupMonitorBorg(ext)
				logger.Info("external backup monitor loaded", "borg_repos", len(ext))
			}
		}
		sched.Start()
		defer sched.Stop()
	}

	// Fleet manager (multi-server monitoring)
	fleetMgr := fleet.New(logger)
	if *demoMode {
		fleetMgr.SetServers(demo.DemoFleetServers())
		fleetMgr.InjectStatuses(demo.DemoFleetStatuses())
		logger.Info("demo fleet data loaded", "servers", len(demo.DemoFleetServers()))
		// Don't start polling in demo mode — injected statuses are static
	} else {
		if persistedSettings != nil && len(persistedSettings.Fleet) > 0 {
			fleetMgr.SetServers(persistedSettings.Fleet)
			logger.Info("fleet monitoring loaded", "servers", len(persistedSettings.Fleet))
		}
		fleetMgr.Start(60 * time.Second)
		defer fleetMgr.Stop()
	}

	// Create API server
	apiServer := api.New(store, sched, coll, metrics, fleetMgr, logger, version)
	apiServer.SetDataPersistent(dataPersistent)

	// HTTP server
	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      apiServer.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("HTTP server listening", "addr", cfg.ListenAddr)
		fmt.Printf("\n  🏥 NAS Doctor v%s", version)
		if *demoMode {
			fmt.Printf(" [DEMO MODE]")
		}
		fmt.Println()
		fmt.Printf("  Dashboard: http://localhost%s\n", cfg.ListenAddr)
		fmt.Printf("  API:       http://localhost%s/api/v1/health\n", cfg.ListenAddr)
		fmt.Printf("  Metrics:   http://localhost%s/metrics\n\n", cfg.ListenAddr)
		fmt.Printf("  Themes:\n")
		fmt.Printf("    ⚫ Midnight:  http://localhost%s/               (default)\n", cfg.ListenAddr)
		fmt.Printf("    ⚪ Clean:     http://localhost%s/theme/clean\n", cfg.ListenAddr)
		fmt.Printf("  Report:        http://localhost%s/api/v1/report\n\n", cfg.ListenAddr)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("HTTP shutdown error", "error", err)
	}

	logger.Info("NAS Doctor stopped")
}

func defaultConfig(listen, dataDir string) internal.Config {
	return internal.Config{
		ListenAddr:   listen,
		DataDir:      dataDir,
		ScheduleCron: "0 */6 * * *",
		HostPaths: internal.HostPaths{
			Boot: "/host/boot",
			Log:  "/host/log",
			Proc: "/proc",
			Sys:  "/sys",
		},
		Prometheus: internal.PrometheusConfig{
			Enabled: true,
			Path:    "/metrics",
		},
	}
}

func loadConfig(path string, cfg *internal.Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}

func applyEnvOverrides(cfg *internal.Config) {
	if url := os.Getenv("NAS_DOCTOR_WEBHOOK_URL"); url != "" {
		wh := internal.WebhookConfig{
			Name:    "env-webhook",
			URL:     url,
			Type:    envOr("NAS_DOCTOR_WEBHOOK_TYPE", "generic"),
			Enabled: true,
		}
		cfg.Notifications.Webhooks = append(cfg.Notifications.Webhooks, wh)
	}

	if v := os.Getenv("NAS_DOCTOR_HOST_BOOT"); v != "" {
		cfg.HostPaths.Boot = v
	}
	if v := os.Getenv("NAS_DOCTOR_HOST_LOG"); v != "" {
		cfg.HostPaths.Log = v
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// normalizeListenAddr accepts a listen address in either "port" (e.g. "8067"),
// ":port" (e.g. ":8067"), or "host:port" (e.g. "0.0.0.0:8067", "[::1]:8060")
// form and returns a form accepted by net.Listen. Bare port numbers are
// prefixed with ":" so users who type "8067" into the Unraid template's
// NAS_DOCTOR_LISTEN variable still get a working config.
//
// Empty input is returned unchanged so callers see the empty value and can
// fall back to their own default.
func normalizeListenAddr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if !strings.Contains(s, ":") {
		return ":" + s
	}
	return s
}

func countSev(findings []internal.Finding, sev internal.Severity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

func loadPersistedSettings(store *storage.DB, logger *slog.Logger) *api.Settings {
	raw, err := store.GetConfig("settings")
	if err != nil || raw == "" {
		return nil
	}
	var settings api.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		logger.Warn("failed to parse stored settings", "error", err)
		return nil
	}
	return &settings
}

func intervalFromSettings(settings *api.Settings, fallback time.Duration, logger *slog.Logger) time.Duration {
	if settings == nil || settings.ScanInterval == "" {
		return fallback
	}
	d, err := time.ParseDuration(settings.ScanInterval)
	if err != nil {
		logger.Warn("ignoring invalid stored scan interval", "scan_interval", settings.ScanInterval, "error", err)
		return fallback
	}
	return d
}

func buildNotifier(webhooks []internal.WebhookConfig, logger *slog.Logger, store *storage.DB) *notifier.Notifier {
	if len(webhooks) == 0 {
		return nil
	}
	n := notifier.New(webhooks, logger)
	n.SetResultHook(func(name, webhookType, status string, findingsCount int, errMsg string) {
		if err := store.SaveNotificationLog(name, webhookType, status, findingsCount, errMsg); err != nil {
			logger.Warn("failed to save notification log", "error", err)
		}
	})
	return n
}

func applySchedulerSettingsFromStore(sched *scheduler.Scheduler, settings *api.Settings) {
	if sched == nil || settings == nil {
		return
	}

	snapshotDays := settings.Retention.SnapshotDays
	if snapshotDays < 7 {
		snapshotDays = 90
	}
	maxDBSizeMB := settings.Retention.MaxDBSizeMB
	if maxDBSizeMB < 50 {
		maxDBSizeMB = 500
	}
	notifyLogDays := settings.Retention.NotifyLogDays
	if notifyLogDays < 1 {
		notifyLogDays = 30
	}
	sched.UpdateRetention(scheduler.RetentionConfig{
		SnapshotDays:  snapshotDays,
		MaxDBSizeMB:   maxDBSizeMB,
		NotifyLogDays: notifyLogDays,
	})

	keepCount := settings.Backup.KeepCount
	if keepCount <= 0 {
		keepCount = 4
	}
	intervalH := settings.Backup.IntervalH
	if intervalH <= 0 {
		intervalH = 168
	}
	sched.UpdateBackup(scheduler.BackupConfig{
		Enabled:   settings.Backup.Enabled,
		Path:      settings.Backup.Path,
		KeepCount: keepCount,
		IntervalH: intervalH,
	})

	defaultCooldown := settings.Notifications.DefaultCooldownSec
	if defaultCooldown <= 0 {
		defaultCooldown = 900
	}
	sched.UpdateAlerting(scheduler.AlertingConfig{
		Policies:           settings.Notifications.Policies,
		QuietHours:         settings.Notifications.QuietHours,
		MaintenanceWindows: settings.Notifications.MaintenanceWindows,
		DefaultCooldownSec: defaultCooldown,
	})
	sched.UpdateServiceChecks(settings.ServiceChecks.Checks)
}
