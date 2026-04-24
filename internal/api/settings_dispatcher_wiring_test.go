package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/notifier"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestHandleUpdateSettings_AdvancedScans_PushesIntervalsToScheduler
// confirms the handler invokes SetDispatcherIntervals with the
// user-submitted values. Without this wiring, slice 2a persists the
// config but the scheduler never picks up the cadence change —
// issue #260 user stories 1-6 all fail silently.
func TestHandleUpdateSettings_AdvancedScans_PushesIntervalsToScheduler(t *testing.T) {
	store := storage.NewFakeStore()
	silent := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	col := collector.New(internal.HostPaths{}, silent)
	sched := scheduler.New(col, store, &notifier.Notifier{}, nil, silent, 30*time.Minute)

	srv := &Server{
		store:     store,
		scheduler: sched,
		collector: col,
		logger:    silent,
		version:   "test",
		startTime: time.Now(),
	}

	// Precondition: dispatcher starts with everything on global
	// (30m). FastestInterval = 30m.
	if got := sched.Dispatcher().FastestInterval(); got != 30*time.Minute {
		t.Fatalf("precondition: FastestInterval = %v, want 30m", got)
	}

	// Seed settings to provide the required default values PUT expects
	// to find on disk (otherwise the handler's validation path for
	// nested fields may behave unexpectedly).
	_ = store.SetConfig(settingsConfigKey, func() string {
		d := defaultSettings()
		b, _ := json.Marshal(d)
		return string(b)
	}())

	// Save settings with Docker=300 (5 min override) + SMART=86400
	// (1 day). FastestInterval should drop to 5 min after the save.
	body, _ := json.Marshal(map[string]interface{}{
		"settings_version": 3,
		"scan_interval":    "30m",
		"theme":            "midnight",
		"advanced_scans": map[string]interface{}{
			"smart":      map[string]interface{}{"wake_drives": false, "max_age_days": 7, "interval_sec": 86400},
			"docker":     map[string]interface{}{"interval_sec": 300},
			"proxmox":    map[string]interface{}{"interval_sec": 0},
			"kubernetes": map[string]interface{}{"interval_sec": 0},
			"zfs":        map[string]interface{}{"interval_sec": 0},
			"gpu":        map[string]interface{}{"interval_sec": 0},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleUpdateSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/settings returned %d: %s", rec.Code, rec.Body.String())
	}

	// After save, dispatcher should see Docker at 5m as fastest.
	if got, want := sched.Dispatcher().FastestInterval(), 5*time.Minute; got != want {
		t.Errorf("after save, FastestInterval = %v, want %v (Docker=300s should be fastest)", got, want)
	}
}
