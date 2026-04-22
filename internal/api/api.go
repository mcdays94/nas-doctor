// Package api provides the HTTP API and web dashboard for nas-doctor.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mcdays94/nas-doctor/internal/collector"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/fleet"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// Theme names
const (
	ThemeMidnight = "midnight" // dark precision
	ThemeClean    = "clean"    // light minimal
	DefaultTheme  = ThemeMidnight
)

// Server holds dependencies for the HTTP API.
type Server struct {
	store     storage.Store
	scheduler *scheduler.Scheduler
	collector *collector.Collector
	metrics   *notifier.Metrics
	fleet     *fleet.Manager
	logger    *slog.Logger
	version   string
	startTime time.Time
	// speedTestRunner is the function invoked by handleTestServiceCheck for
	// speed-type checks. Nil means the handler will fall back to
	// collector.RunSpeedTest (the default). Tests override this to inject
	// deterministic results without needing the Ookla CLI.
	speedTestRunner scheduler.SpeedTestRunner
}

// New creates a new API server.
func New(store storage.Store, sched *scheduler.Scheduler, coll *collector.Collector, metrics *notifier.Metrics, fleetMgr *fleet.Manager, logger *slog.Logger, version string) *Server {
	return &Server{
		store:     store,
		scheduler: sched,
		collector: coll,
		metrics:   metrics,
		fleet:     fleetMgr,
		logger:    logger,
		version:   version,
		startTime: time.Now(),
	}
}

// Router returns the configured chi router with all routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Baseline middleware — cheap, applied to every route.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Long-running test endpoints — registered BEFORE the Timeout group.
	// Speed-type service-check tests invoke the Ookla CLI, which can take
	// 10-60s, so we can't subject them to the 30s router-wide timeout.
	// RunCheck uses its own per-check context from cfg.TimeoutSec.
	r.Post("/api/v1/service-checks/test", s.handleTestServiceCheck)

	// Standard-latency routes — 30s soft timeout via a Group so we don't
	// apply it to the long-running route above.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(30 * time.Second))

		// Health endpoint — always public (Docker HEALTHCHECK, K8s probes, load balancers)
		r.Get("/api/v1/health", s.handleHealth)

		// API routes — protected by API key when set
		r.Route("/api/v1", func(r chi.Router) {
			r.Use(s.apiKeyMiddleware)
			r.Get("/status", s.handleStatus)
			r.Get("/snapshot/latest", s.handleLatestSnapshot)
			r.Get("/snapshot/{id}", s.handleGetSnapshot)
			r.Get("/snapshots", s.handleListSnapshots)
			r.Post("/scan", s.handleTriggerScan)
			r.Get("/report", s.handleReport)
		})

		// Prometheus metrics
		if s.metrics != nil {
			r.Handle("/metrics", promhttp.HandlerFor(s.metrics.Registry(), promhttp.HandlerOpts{}))
		}

		// Extended API routes (settings, disks, history, notifications)
		s.RegisterExtendedRoutes(r)
	})

	// Chart library JS
	r.Get("/js/charts.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte(ChartJS))
	})

	// Shared dashboard rendering JS
	r.Get("/js/dashboard.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte(DashboardJS))
	})

	// Shared design system CSS
	r.Get("/css/shared.css", serveSharedCSS)

	// Icons
	r.Get("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		settings := s.getSettings()
		icon := settings.Icon
		if icon == "" {
			icon = "icon3"
		}
		serveIcon(w, icon)
	})
	r.Get("/favicon.png", func(w http.ResponseWriter, r *http.Request) {
		settings := s.getSettings()
		icon := settings.Icon
		if icon == "" {
			icon = "icon3"
		}
		serveIcon(w, icon)
	})
	r.Get("/icons/{name}.png", func(w http.ResponseWriter, r *http.Request) {
		serveIcon(w, chi.URLParam(r, "name"))
	})
	r.Get("/api/v1/icons", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"default":   "icon3",
			"available": ListIcons(),
		})
	})

	// Web dashboard
	r.Get("/", s.handleDashboard)
	r.Get("/theme/{theme}", s.handleDashboardTheme)

	return r
}

// ---------- Handlers ----------

// apiKeyMiddleware checks for a valid API key when one is configured.
// Requests from the same origin (HTML pages) are exempt — only external
// API calls need the key. When no key is set, all requests pass through.
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := s.getSettings()
		if settings.APIKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Also check query param for convenience (e.g. browser testing)
			auth = "Bearer " + r.URL.Query().Get("api_key")
		}
		expected := "Bearer " + settings.APIKey
		if auth != expected {
			// Allow requests from same origin (Referer contains our host)
			referer := r.Header.Get("Referer")
			if referer != "" && (strings.Contains(referer, r.Host) || strings.Contains(referer, "localhost")) {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing API key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-NAS-Doctor", "true")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"nas_doctor": true,
		"version":    s.version,
		"uptime":     time.Since(s.startTime).String(),
		"themes":     []string{ThemeMidnight, ThemeClean},
	})
}

type statusResponse struct {
	Hostname          string              `json:"hostname"`
	Platform          string              `json:"platform"`
	Version           string              `json:"version"`
	Uptime            string              `json:"uptime"`
	LastScan          string              `json:"last_scan"`
	ScanIntervalSecs  int                 `json:"scan_interval_secs"`
	ScanRunning       bool                `json:"scan_running"`
	CriticalCount     int                 `json:"critical_count"`
	WarningCount      int                 `json:"warning_count"`
	InfoCount         int                 `json:"info_count"`
	OverallHealth     string              `json:"overall_health"`
	Sections          *DashboardSections  `json:"sections,omitempty"`
	ChartRangeHours   int                 `json:"chart_range_hours"`
	SectionHeights    map[string]int      `json:"section_heights,omitempty"`
	SectionOrder      map[string][]string `json:"section_order,omitempty"`
	DismissedFindings []string            `json:"dismissed_findings,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{Version: s.version}
	settings := s.getSettings()
	dismissed := make(map[string]struct{}, len(settings.DismissedFindings))
	for _, title := range settings.DismissedFindings {
		dismissed[title] = struct{}{}
	}

	if s.scheduler != nil {
		resp.ScanRunning = s.scheduler.IsRunning()
		resp.ScanIntervalSecs = int(s.scheduler.Interval().Seconds())
	}

	var snap *internal.Snapshot
	if s.scheduler != nil {
		snap = s.scheduler.Latest()
	}
	if snap == nil {
		snap, _ = s.store.GetLatestSnapshot()
	}
	if snap != nil {
		resp.Hostname = snap.System.Hostname
		resp.Platform = snap.System.Platform
		resp.Uptime = formatDuration(time.Duration(snap.System.UptimeSecs) * time.Second)
		resp.LastScan = snap.Timestamp.Format(time.RFC3339)

		for _, f := range snap.Findings {
			if _, ok := dismissed[f.Title]; ok {
				continue
			}
			switch f.Severity {
			case "critical":
				resp.CriticalCount++
			case "warning":
				resp.WarningCount++
			case "info":
				resp.InfoCount++
			}
		}

		if resp.CriticalCount > 0 {
			resp.OverallHealth = "critical"
		} else if resp.WarningCount > 0 {
			resp.OverallHealth = "warning"
		} else {
			resp.OverallHealth = "healthy"
		}
	}

	// Include section visibility, chart range, and dismissed findings from settings
	resp.Sections = &settings.Sections
	resp.ChartRangeHours = settings.ChartRangeHours
	if resp.ChartRangeHours == 0 {
		resp.ChartRangeHours = 1
	}
	resp.SectionHeights = settings.SectionHeights
	resp.SectionOrder = settings.SectionOrder
	resp.DismissedFindings = settings.DismissedFindings

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	var snap *internal.Snapshot
	if s.scheduler != nil {
		snap = s.scheduler.Latest()
	}
	if snap == nil {
		var err error
		snap, err = s.store.GetLatestSnapshot()
		if err != nil || snap == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no snapshots available"})
			return
		}
	}
	// Enrich parity checks with avg/max array temperature from smart_history
	s.enrichParityTemps(snap)
	writeJSON(w, http.StatusOK, snap)
}

// enrichParityTemps computes avg/max array temperature during each parity check
// by querying smart_history for the check's time window.
func (s *Server) enrichParityTemps(snap *internal.Snapshot) {
	if snap == nil || snap.Parity == nil || len(snap.Parity.History) == 0 {
		return
	}
	for i := range snap.Parity.History {
		pc := &snap.Parity.History[i]
		if pc.Duration <= 0 || pc.Date == "" {
			continue
		}
		start, err := time.Parse("2006 Jan  2 15:04:05", pc.Date)
		if err != nil {
			start, err = time.Parse("2006 Jan 2 15:04:05", pc.Date)
		}
		if err != nil {
			continue
		}
		end := start.Add(time.Duration(pc.Duration) * time.Second)
		avg, max, err := s.store.GetAvgTempDuringRange(start, end)
		if err == nil && avg > 0 {
			pc.AvgTempC = math.Round(avg*10) / 10
			pc.MaxTempC = math.Round(max*10) / 10
		}
	}
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	snap, err := s.store.GetSnapshot(id)
	if err != nil || snap == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.store.ListSnapshots(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if summaries == nil {
		summaries = []storage.SnapshotSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (s *Server) handleTriggerScan(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "scanner not available in demo mode"})
		return
	}
	if s.scheduler.IsRunning() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "scan already in progress"})
		return
	}
	go s.scheduler.RunOnce()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var snap *internal.Snapshot
	if s.scheduler != nil {
		snap = s.scheduler.Latest()
	}
	if snap == nil {
		snap, _ = s.store.GetLatestSnapshot()
	}
	if snap == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no snapshot available for report"})
		return
	}
	// Fetch sparkline data for inline SVG charts
	var sparks ReportSparklines
	if s.store != nil {
		if sysH, err := s.store.GetSystemSparkline(60); err == nil {
			sparks.System = sysH
		}
		if diskH, err := s.store.GetAllDiskSparklines(60); err == nil {
			sparks.Disks = diskH
		}
	}
	html := GenerateReport(snap, sparks)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\"nas-doctor-report.html\"")
	w.Write([]byte(html))
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Serve the theme saved in settings (not hardcoded)
	settings := s.getSettings()
	s.serveDashboard(w, settings.Theme)
}

func (s *Server) handleDashboardTheme(w http.ResponseWriter, r *http.Request) {
	theme := chi.URLParam(r, "theme")
	if theme == ThemeMidnight || theme == ThemeClean {
		settings := s.getSettings()
		settings.Theme = theme
		if data, err := json.Marshal(settings); err == nil {
			s.store.SetConfig(settingsConfigKey, string(data))
		}
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) serveDashboard(w http.ResponseWriter, theme string) {
	var html string
	switch theme {
	case ThemeClean:
		html = DashboardClean
	default:
		html = DashboardMidnight
	}
	html = strings.Replace(html, "__VERSION__", s.version, -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// ---------- Helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
}
