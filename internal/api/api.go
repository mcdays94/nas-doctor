// Package api provides the HTTP API and web dashboard for nas-doctor.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	ThemeEmber    = "ember"    // serif with depth
	DefaultTheme  = ThemeMidnight
)

// Server holds dependencies for the HTTP API.
type Server struct {
	store     *storage.DB
	scheduler *scheduler.Scheduler
	collector *collector.Collector
	metrics   *notifier.Metrics
	fleet     *fleet.Manager
	logger    *slog.Logger
	version   string
	startTime time.Time
}

// New creates a new API server.
func New(store *storage.DB, sched *scheduler.Scheduler, coll *collector.Collector, metrics *notifier.Metrics, fleetMgr *fleet.Manager, logger *slog.Logger, version string) *Server {
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

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// API routes — all protected by API key when set
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.apiKeyMiddleware)
		r.Get("/health", s.handleHealth)
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

	// Chart library JS
	r.Get("/js/charts.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write([]byte(ChartJS))
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
		"themes":     []string{ThemeMidnight, ThemeClean, ThemeEmber},
	})
}

type statusResponse struct {
	Hostname          string             `json:"hostname"`
	Platform          string             `json:"platform"`
	Version           string             `json:"version"`
	Uptime            string             `json:"uptime"`
	LastScan          string             `json:"last_scan"`
	ScanIntervalSecs  int                `json:"scan_interval_secs"`
	ScanRunning       bool               `json:"scan_running"`
	CriticalCount     int                `json:"critical_count"`
	WarningCount      int                `json:"warning_count"`
	InfoCount         int                `json:"info_count"`
	OverallHealth     string             `json:"overall_health"`
	Sections          *DashboardSections `json:"sections,omitempty"`
	DismissedFindings []string           `json:"dismissed_findings,omitempty"`
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

	// Include section visibility and dismissed findings from settings
	resp.Sections = &settings.Sections
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
	writeJSON(w, http.StatusOK, snap)
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
	if theme == ThemeMidnight || theme == ThemeClean || theme == ThemeEmber {
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
	case ThemeEmber:
		html = DashboardEmber
	default:
		html = DashboardMidnight
	}
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
