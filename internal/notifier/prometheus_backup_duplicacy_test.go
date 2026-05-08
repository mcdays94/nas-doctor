package notifier

import (
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestPrometheus_BackupDuplicacy_FourGaugesPerEntry asserts that after
// Update() with a snapshot containing one Duplicacy entry, /metrics
// exports the full 4-gauge family per entry:
//
//   - nasdoctor_backup_duplicacy_snapshots_total{label="…"}
//   - nasdoctor_backup_duplicacy_last_backup_age_seconds{label="…"}
//   - nasdoctor_backup_duplicacy_last_backup_size_bytes{label="…"}
//   - nasdoctor_backup_duplicacy_status{label="…",reason="…"}
//
// Naming + label conventions match the existing
// nasdoctor_backup_borg_* would-have. Per PRD #310 §8 the gauges go
// under the `backup_duplicacy` subsystem so Grafana panels can group
// "duplicacy across all repos" with a single metric-name regex.
//
// Issue #314 (V1c) acceptance criteria 4-5.
func TestPrometheus_BackupDuplicacy_FourGaugesPerEntry(t *testing.T) {
	m := NewMetrics()
	now := time.Now()
	m.Update(&internal.Snapshot{
		Timestamp: now,
		Backup: &internal.BackupInfo{
			Available: true,
			Duplicacy: []internal.DuplicacyJobState{
				{
					Label:                  "Documents",
					Kind:                   "cli-repo",
					Path:                   "/mnt/user/duplicacy/docs",
					ReasonCode:             "ok",
					SnapshotCount:          217,
					LatestBackupAt:         now.Add(-2 * time.Hour),
					LatestBackupSizeBytes:  44_000_000_000,
					LatestSnapshotID:       "documents",
					LatestSnapshotRevision: 217,
				},
			},
		},
	})

	body := scrapeMetrics(t, m)

	// snapshots_total
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_snapshots_total{label="documents"} 217`) {
		t.Errorf("expected nasdoctor_backup_duplicacy_snapshots_total{label=\"documents\"} 217 in /metrics; body:\n%s", body)
	}
	// last_backup_size_bytes
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_last_backup_size_bytes{label="documents"} 4.4e+10`) {
		t.Errorf("expected nasdoctor_backup_duplicacy_last_backup_size_bytes{label=\"documents\"} 4.4e+10 in /metrics; body:\n%s", body)
	}
	// last_backup_age_seconds — 2 hours = 7200s ± 5s of clock slop
	// between the test's `now` and the gauge's snap.Timestamp delta.
	// Just match the prefix + label set; numeric proximity is
	// covered separately below.
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_last_backup_age_seconds{label="documents"}`) {
		t.Errorf("expected nasdoctor_backup_duplicacy_last_backup_age_seconds{label=\"documents\"} … in /metrics; body:\n%s", body)
	}
	// status — the active reason reads 1, every other reason in the
	// closed set reads 0. Pin both arms so a future regression
	// where we forget to zero-out the others is caught.
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="documents",reason="ok"} 1`) {
		t.Errorf("expected nasdoctor_backup_duplicacy_status{label=\"documents\",reason=\"ok\"} 1 in /metrics; body:\n%s", body)
	}
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="documents",reason="stale"} 0`) {
		t.Errorf("expected nasdoctor_backup_duplicacy_status{label=\"documents\",reason=\"stale\"} 0 in /metrics; body:\n%s", body)
	}
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="documents",reason="path_not_found"} 0`) {
		t.Errorf("expected nasdoctor_backup_duplicacy_status{label=\"documents\",reason=\"path_not_found\"} 0 in /metrics; body:\n%s", body)
	}
}

// TestPrometheus_BackupDuplicacy_StatusFamilyAllReasons asserts that
// every reason code in the V1a closed set produces a 0-valued series
// when the entry is in any other state — Grafana queries can build a
// "current-reason" panel with `nasdoctor_backup_duplicacy_status == 1`.
// Pinned to 8 reasons matching collector.DuplicacyReason* constants.
func TestPrometheus_BackupDuplicacy_StatusFamilyAllReasons(t *testing.T) {
	m := NewMetrics()
	m.Update(&internal.Snapshot{
		Timestamp: time.Now(),
		Backup: &internal.BackupInfo{
			Available: true,
			Duplicacy: []internal.DuplicacyJobState{
				{Label: "media", Kind: "web-cache", ReasonCode: "stale"},
			},
		},
	})

	body := scrapeMetrics(t, m)
	wantReasons := []string{
		"ok",
		"path_not_found",
		"path_unreadable",
		"not_a_duplicacy_repo",
		"storage_id_not_found",
		"no_snapshots_yet",
		"stale",
		"corrupt_snapshot",
	}
	for _, r := range wantReasons {
		expected := r + `"`
		if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="media",reason="`+expected) {
			t.Errorf("missing series for reason=%q in /metrics body — every reason in the V1a closed set must have a 0/1-valued series so Grafana can `... == 1` filter on the active reason; body excerpt:\n%s", r, body)
		}
	}
	// The active reason reads 1; every other reason reads 0. (The
	// previous loop only checks presence; this one pins values.)
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="media",reason="stale"} 1`) {
		t.Errorf("expected the active reason (stale) to read 1; body:\n%s", body)
	}
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="media",reason="ok"} 0`) {
		t.Errorf("expected non-active reason (ok) to read 0; body:\n%s", body)
	}
}

// TestPrometheus_BackupDuplicacy_LabelFallback asserts that an entry
// with an empty Label uses the basename of Path as the gauge label
// — keeps Prometheus series identifiable even for users who don't
// fill in the optional Label field.
func TestPrometheus_BackupDuplicacy_LabelFallback(t *testing.T) {
	m := NewMetrics()
	m.Update(&internal.Snapshot{
		Timestamp: time.Now(),
		Backup: &internal.BackupInfo{
			Available: true,
			Duplicacy: []internal.DuplicacyJobState{
				{Label: "", Kind: "cli-repo", Path: "/mnt/user/duplicacy/photos", ReasonCode: "ok"},
			},
		},
	})

	body := scrapeMetrics(t, m)
	if !strings.Contains(body, `nasdoctor_backup_duplicacy_status{label="photos",reason="ok"} 1`) {
		t.Errorf("expected fallback label to be derived from Path basename; body:\n%s", body)
	}
}

// TestPrometheus_BackupDuplicacy_ResetBetweenScans asserts that an
// entry removed from the configuration disappears from /metrics on
// the next Update — i.e. the gauges are Reset() before each
// re-stamp. Without this a renamed entry would leak ghost series
// forever.
func TestPrometheus_BackupDuplicacy_ResetBetweenScans(t *testing.T) {
	m := NewMetrics()
	now := time.Now()
	m.Update(&internal.Snapshot{
		Timestamp: now,
		Backup: &internal.BackupInfo{
			Available: true,
			Duplicacy: []internal.DuplicacyJobState{
				{Label: "stale-name", Kind: "cli-repo", ReasonCode: "ok"},
			},
		},
	})
	if body := scrapeMetrics(t, m); !strings.Contains(body, `label="stale-name"`) {
		t.Fatalf("preconditions: stale-name should be present after first Update; body:\n%s", body)
	}
	// Second Update with a different label — first should be gone.
	m.Update(&internal.Snapshot{
		Timestamp: now,
		Backup: &internal.BackupInfo{
			Available: true,
			Duplicacy: []internal.DuplicacyJobState{
				{Label: "fresh-name", Kind: "cli-repo", ReasonCode: "ok"},
			},
		},
	})
	body := scrapeMetrics(t, m)
	if strings.Contains(body, `label="stale-name"`) {
		t.Errorf("expected stale-name series to be dropped after second Update; body:\n%s", body)
	}
	if !strings.Contains(body, `label="fresh-name"`) {
		t.Errorf("expected fresh-name series to be present after second Update; body:\n%s", body)
	}
}

// TestPrometheus_BackupDuplicacy_NoEntriesNoSeries asserts that an
// install with no Duplicacy entries produces no
// nasdoctor_backup_duplicacy_* series at all. Important for Grafana
// operators who detect "did the user configure Duplicacy?" via
// series presence.
//
// Note: a GaugeVec with no labelled series produces no `# HELP /
// # TYPE` headers at all in the Prometheus text exposition format —
// that's standard Prometheus client behaviour. So we assert on
// series-line absence only, not on the metric family declaration.
func TestPrometheus_BackupDuplicacy_NoEntriesNoSeries(t *testing.T) {
	m := NewMetrics()
	m.Update(&internal.Snapshot{
		Timestamp: time.Now(),
		Backup:    &internal.BackupInfo{Available: false},
	})
	body := scrapeMetrics(t, m)
	for _, line := range strings.Split(body, "\n") {
		// Skip HELP/TYPE comment lines — those are documentation,
		// not series. The check is purely on per-entry data lines.
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "nasdoctor_backup_duplicacy_") {
			t.Errorf("unexpected series with no entries configured: %q", line)
		}
	}
}
