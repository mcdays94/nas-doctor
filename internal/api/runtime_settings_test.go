package api

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newSchedForApplyTest builds a real scheduler backed by the in-memory fake
// store so ApplyRuntimeSettings can be exercised end-to-end and its effect
// observed via the scheduler's read accessors.
func newSchedForApplyTest(t *testing.T) *scheduler.Scheduler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	coll := collector.New(internal.HostPaths{}, logger)
	// nil metrics is supported by scheduler.New (guarded internally) and keeps
	// the test off the Prometheus registry.
	return scheduler.New(coll, storage.NewFakeStore(), nil, nil, logger, 30*time.Minute)
}

// TestApplyRuntimeSettings_SpeedTestCadence is the core regression guard for
// issue #333: persisted speed-test settings must take effect through
// ApplyRuntimeSettings, not stay at the scheduler's New() default of 4h.
func TestApplyRuntimeSettings_SpeedTestCadence(t *testing.T) {
	cases := []struct {
		name         string
		interval     string
		schedule     []string
		day          string
		wantInterval time.Duration
		wantSchedule []string
		wantDay      string
		wantFreq     string
	}{
		{
			name:         "disabled sentinel is honoured",
			interval:     "disabled",
			schedule:     []string{},
			wantInterval: scheduler.SpeedTestIntervalDisabled,
			wantSchedule: nil,
			wantFreq:     "disabled", // SetSpeedTestSchedule records the raw freq; unused while schedule is empty
		},
		{
			name:         "weekly scheduled cadence",
			interval:     "weekly",
			schedule:     []string{"03:00"},
			day:          "sunday",
			wantInterval: 4 * time.Hour, // "weekly" is not a duration; interval left at default, schedule drives the loop
			wantSchedule: []string{"03:00"},
			wantDay:      "sunday",
			wantFreq:     "weekly",
		},
		{
			name:         "interval mode with explicit empty schedule",
			interval:     "6h",
			schedule:     []string{},
			wantInterval: 6 * time.Hour,
			wantSchedule: nil,
			wantFreq:     "6h", // recorded but unused while schedule is empty (interval mode)
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sched := newSchedForApplyTest(t)
			s := Settings{
				SpeedTestInterval: tc.interval,
				SpeedTestSchedule: tc.schedule,
				SpeedTestDay:      tc.day,
			}
			ApplyRuntimeSettings(sched, nil, s, 30*time.Minute)

			gotInterval, gotSchedule, gotDay, gotFreq := sched.SpeedTestConfig()
			if gotInterval != tc.wantInterval {
				t.Errorf("interval = %v; want %v", gotInterval, tc.wantInterval)
			}
			if len(gotSchedule) != len(tc.wantSchedule) {
				t.Errorf("schedule = %v; want %v", gotSchedule, tc.wantSchedule)
			} else {
				for i := range gotSchedule {
					if gotSchedule[i] != tc.wantSchedule[i] {
						t.Errorf("schedule[%d] = %q; want %q", i, gotSchedule[i], tc.wantSchedule[i])
					}
				}
			}
			if gotDay != tc.wantDay {
				t.Errorf("day = %q; want %q", gotDay, tc.wantDay)
			}
			if gotFreq != tc.wantFreq {
				t.Errorf("freq = %q; want %q", gotFreq, tc.wantFreq)
			}
		})
	}
}

// TestApplyRuntimeSettings_AppliesNotificationRules guards the second boot
// regression: notification routing rules were dropped at startup, silently
// falling back to the legacy "every finding to every webhook" path until the
// next save.
func TestApplyRuntimeSettings_AppliesNotificationRules(t *testing.T) {
	sched := newSchedForApplyTest(t)
	s := Settings{}
	s.Notifications.Rules = []internal.NotificationRule{
		{ID: "r1", Name: "criticals to discord"},
	}
	ApplyRuntimeSettings(sched, nil, s, 30*time.Minute)

	got := sched.AlertingConfig()
	if len(got.Rules) != 1 || got.Rules[0].ID != "r1" {
		t.Fatalf("alerting rules not applied: got %+v; want one rule with ID r1", got.Rules)
	}
}

// TestLogForwardDestinations covers the pure conversion helper that both the
// save handler and startup use to wire log forwarding — the third boot
// regression (forwarding was never wired at startup).
func TestLogForwardDestinations(t *testing.T) {
	t.Run("disabled returns nil (clears the forwarder)", func(t *testing.T) {
		got := logForwardDestinations(SettingsLogForward{
			Enabled:      false,
			Destinations: []LogForwardDestination{{Name: "loki", Type: "loki", URL: "http://x", Enabled: true}},
		})
		if got != nil {
			t.Errorf("disabled config should yield nil; got %v", got)
		}
	})

	t.Run("enabled with no destinations returns nil", func(t *testing.T) {
		if got := logForwardDestinations(SettingsLogForward{Enabled: true}); got != nil {
			t.Errorf("no destinations should yield nil; got %v", got)
		}
	})

	t.Run("enabled destinations are converted", func(t *testing.T) {
		got := logForwardDestinations(SettingsLogForward{
			Enabled: true,
			Destinations: []LogForwardDestination{{
				Name: "loki", Type: "loki", URL: "http://loki:3100", Enabled: true,
				Headers: map[string]string{"X-Scope-OrgID": "1"}, Format: "full",
			}},
		})
		if len(got) != 1 {
			t.Fatalf("want 1 destination, got %d", len(got))
		}
		if got[0].Name != "loki" || got[0].URL != "http://loki:3100" || got[0].Headers["X-Scope-OrgID"] != "1" {
			t.Errorf("destination not converted faithfully: %+v", got[0])
		}
	})
}

// TestSpeedTestScheduleRoundTripsWhenEmpty is the guard against re-flipping
// interval-mode users to daily 03:00. With omitempty on SpeedTestSchedule, an
// explicit empty schedule (interval mode) was dropped from the persisted JSON,
// so a re-read layered the "03:00" default back on. The key must now survive
// serialization so an empty schedule round-trips as empty.
func TestSpeedTestScheduleRoundTripsWhenEmpty(t *testing.T) {
	// An interval-mode save carries an explicit empty schedule.
	saved := Settings{SpeedTestInterval: "6h", SpeedTestSchedule: []string{}}
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The key must be present (not omitted) so the emptiness is recorded.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := asMap["speedtest_schedule"]; !ok {
		t.Fatal("speedtest_schedule key was omitted; an empty (interval-mode) schedule must persist explicitly (issue #333 regression)")
	}

	// Re-reading over defaults must NOT reintroduce the "03:00" default.
	reread := defaultSettings()
	if err := json.Unmarshal(raw, &reread); err != nil {
		t.Fatalf("unmarshal over defaults: %v", err)
	}
	if len(reread.SpeedTestSchedule) != 0 {
		t.Errorf("empty schedule did not round-trip; got %v (interval-mode user would be flipped to scheduled mode)", reread.SpeedTestSchedule)
	}
}
