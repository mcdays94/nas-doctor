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
//	-interval   Diagnostic scan interval (default "6h")
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

const version = "0.1.0"

func main() {
	// Flags
	listenAddr := flag.String("listen", envOr("NAS_DOCTOR_LISTEN", ":8060"), "HTTP listen address")
	dataDir := flag.String("data", envOr("NAS_DOCTOR_DATA", "/tmp/nas-doctor-data"), "Data directory for SQLite DB")
	intervalStr := flag.String("interval", envOr("NAS_DOCTOR_INTERVAL", "6h"), "Diagnostic scan interval")
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

	// Open database
	dbPath := filepath.Join(cfg.DataDir, "nas-doctor.db")
	store, err := storage.Open(dbPath, logger)
	if err != nil {
		logger.Error("failed to open database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create Prometheus metrics
	metrics := notifier.NewMetrics()

	var sched *scheduler.Scheduler

	if *demoMode {
		// Demo mode: inject fake data with historical snapshots
		logger.Info("demo mode: generating mock diagnostic data with history")

		col := collector.New(cfg.HostPaths, logger)
		sched = scheduler.New(col, store, nil, metrics, logger, 24*time.Hour)

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
			histSnap.Findings = analyzer.Analyze(histSnap)
			if err := store.SaveSnapshot(histSnap); err != nil {
				logger.Warn("failed to save historical demo snapshot", "day", i, "error", err)
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
		logger.Info("demo data loaded",
			"findings", len(snap.Findings),
			"critical", countSev(snap.Findings, internal.SeverityCritical),
			"warnings", countSev(snap.Findings, internal.SeverityWarning),
		)
	} else {
		// Production mode: real collectors
		col := collector.New(cfg.HostPaths, logger)

		var notif *notifier.Notifier
		if len(cfg.Notifications.Webhooks) > 0 {
			notif = notifier.New(cfg.Notifications.Webhooks, logger)
		}

		sched = scheduler.New(col, store, notif, metrics, logger, interval)
		sched.Start()
		defer sched.Stop()
	}

	// Fleet manager (multi-server monitoring)
	fleetMgr := fleet.New(logger)
	// Load fleet servers from settings
	if raw, err := store.GetConfig("settings"); err == nil && raw != "" {
		var settingsData struct {
			Fleet []internal.RemoteServer `json:"fleet"`
		}
		if json.Unmarshal([]byte(raw), &settingsData) == nil && len(settingsData.Fleet) > 0 {
			fleetMgr.SetServers(settingsData.Fleet)
			fleetMgr.Start(60 * time.Second) // poll every 60s
			logger.Info("fleet monitoring started", "servers", len(settingsData.Fleet))
		}
	}

	// Create API server
	apiServer := api.New(store, sched, metrics, fleetMgr, logger)

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
		fmt.Printf("    🔴 Ember:     http://localhost%s/theme/ember\n", cfg.ListenAddr)
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
			Name:     "env-webhook",
			URL:      url,
			Type:     envOr("NAS_DOCTOR_WEBHOOK_TYPE", "generic"),
			Enabled:  true,
			MinLevel: internal.Severity(envOr("NAS_DOCTOR_WEBHOOK_MIN_LEVEL", "warning")),
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

func countSev(findings []internal.Finding, sev internal.Severity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}
