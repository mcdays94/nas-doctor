package main

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/api"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestApplySchedulerSettingsFromStore_AppliesSpeedTestAtBoot is the boot-seam
// regression guard for issue #333: a persisted speed-test setting must take
// effect at startup, not be silently left at the scheduler's New() default of
// 4h. Before the fix the startup helper applied retention/backup/alerting but
// never touched the speed-test cadence, so a user who had set "Disabled" (or a
// weekly schedule) still got a full-bandwidth test every 4h after each restart.
func TestApplySchedulerSettingsFromStore_AppliesSpeedTestAtBoot(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	coll := collector.New(internal.HostPaths{}, logger)
	sched := scheduler.New(coll, storage.NewFakeStore(), nil, nil, logger, 30*time.Minute)

	settings := &api.Settings{SpeedTestInterval: "disabled"}
	applySchedulerSettingsFromStore(sched, coll, settings, 30*time.Minute)

	iv, _, _, _ := sched.SpeedTestConfig()
	if iv != scheduler.SpeedTestIntervalDisabled {
		t.Fatalf("startup did not apply the persisted speed-test setting: interval = %v, want disabled (issue #333)", iv)
	}
}

// TestApplySchedulerSettingsFromStore_NilSettingsIsNoop confirms the helper
// tolerates a nil settings blob (empty DB / unreadable config) without panic,
// leaving the scheduler at its constructor defaults.
func TestApplySchedulerSettingsFromStore_NilSettingsIsNoop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	coll := collector.New(internal.HostPaths{}, logger)
	sched := scheduler.New(coll, storage.NewFakeStore(), nil, nil, logger, 30*time.Minute)

	applySchedulerSettingsFromStore(sched, coll, nil, 30*time.Minute)

	if iv, _, _, _ := sched.SpeedTestConfig(); iv != 4*time.Hour {
		t.Fatalf("nil settings should leave the New() default in place: interval = %v, want 4h", iv)
	}
}

// TestApplySettingsAtBoot: saved settings are always applied; a brand-new
// data directory gets the defaults its Settings page shows; an install
// that already has scan history but never saved a setting is left on the
// scheduler defaults it has been running with.
func TestApplySettingsAtBoot(t *testing.T) {
	cases := []struct {
		stored, history, want bool
	}{
		{stored: true, history: true, want: true},
		{stored: true, history: false, want: true},
		{stored: false, history: false, want: true},
		{stored: false, history: true, want: false},
	}
	for _, c := range cases {
		if got := applySettingsAtBoot(c.stored, c.history); got != c.want {
			t.Errorf("applySettingsAtBoot(stored=%v, history=%v) = %v, want %v", c.stored, c.history, got, c.want)
		}
	}
}
