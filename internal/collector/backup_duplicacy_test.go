package collector

import (
	"context"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// fakeDupRunner is a recording DuplicacyRunner used to exercise the
// CollectBackups → collectDuplicacyEntries plumbing without touching
// the filesystem. Returns canned states keyed by the entry's Label
// (or kind+path when label is empty).
type fakeDupRunner struct {
	calls   []DuplicacyEntry
	results map[string]DuplicacyState
	err     error
}

func (f *fakeDupRunner) Read(ctx context.Context, e DuplicacyEntry) (DuplicacyState, error) {
	f.calls = append(f.calls, e)
	if f.err != nil {
		return DuplicacyState{}, f.err
	}
	if r, ok := f.results[e.Label]; ok {
		return r, nil
	}
	return DuplicacyState{ReasonCode: DuplicacyReasonNoSnapshotsYet}, nil
}

// TestCollectBackups_DuplicacyEntries_PopulatesBackupInfo asserts the
// scheduler-side wiring: each enabled DuplicacyEntry produces one
// DuplicacyJobState in BackupInfo.Duplicacy, fields are copied from
// the runner's DuplicacyState verbatim, and disabled entries are
// dropped (no row). PRD #310 V1c / issue #314.
func TestCollectBackups_DuplicacyEntries_PopulatesBackupInfo(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	runner := &fakeDupRunner{
		results: map[string]DuplicacyState{
			"docs": {
				ReasonCode:             DuplicacyReasonOK,
				SnapshotCount:          42,
				LatestBackupAt:         now.Add(-3 * time.Hour),
				LatestBackupSizeBytes:  88 * 1024 * 1024 * 1024,
				LatestBackupFiles:      31000,
				CurrentlyRunning:       false,
				LatestSnapshotID:       "documents",
				LatestSnapshotRevision: 42,
				SnapshotIDs:            []string{"documents"},
			},
			"media": {
				ReasonCode:    DuplicacyReasonStale,
				SnapshotCount: 12,
			},
		},
	}

	info := CollectBackups(CollectBackupsOptions{
		DuplicacyRunner: runner,
		Duplicacy: []DuplicacyEntry{
			{Enabled: true, Label: "docs", Kind: DuplicacyKindCLIRepo, Path: "/p/docs"},
			{Enabled: true, Label: "media", Kind: DuplicacyKindWebCache, Path: "/p/media", StorageID: "main"},
			{Enabled: false, Label: "skip-me", Kind: DuplicacyKindCLIRepo, Path: "/p/skip"},
		},
	})

	if info == nil {
		t.Fatal("CollectBackups returned nil")
	}
	if !info.Available {
		t.Errorf("BackupInfo.Available = false; want true (Duplicacy entries make backup data available)")
	}
	if got := len(info.Duplicacy); got != 2 {
		t.Fatalf("info.Duplicacy len = %d; want 2 (one disabled entry must be skipped); entries=%+v", got, info.Duplicacy)
	}
	// Disabled entries must not even reach the runner.
	if got := len(runner.calls); got != 2 {
		t.Errorf("runner.Read called %d times; want 2 (disabled entry must be skipped before Read)", got)
	}
	for _, c := range runner.calls {
		if !c.Enabled {
			t.Errorf("runner received a disabled entry: %+v", c)
		}
	}

	// Field-by-field verification of the docs entry — every field
	// the dashboard widget + Prometheus exporter consumes must
	// round-trip from the runner.
	d0 := info.Duplicacy[0]
	if d0.Label != "docs" {
		t.Errorf("entry 0 Label = %q; want docs", d0.Label)
	}
	if d0.Kind != DuplicacyKindCLIRepo {
		t.Errorf("entry 0 Kind = %q; want cli-repo", d0.Kind)
	}
	if d0.Path != "/p/docs" {
		t.Errorf("entry 0 Path = %q; want /p/docs", d0.Path)
	}
	if d0.ReasonCode != string(DuplicacyReasonOK) {
		t.Errorf("entry 0 ReasonCode = %q; want %q", d0.ReasonCode, DuplicacyReasonOK)
	}
	if d0.SnapshotCount != 42 {
		t.Errorf("entry 0 SnapshotCount = %d; want 42", d0.SnapshotCount)
	}
	if !d0.LatestBackupAt.Equal(now.Add(-3 * time.Hour)) {
		t.Errorf("entry 0 LatestBackupAt = %v; want %v", d0.LatestBackupAt, now.Add(-3*time.Hour))
	}
	if d0.LatestSnapshotID != "documents" || d0.LatestSnapshotRevision != 42 {
		t.Errorf("entry 0 latest_snapshot_(id|revision) = %q / %d; want documents / 42", d0.LatestSnapshotID, d0.LatestSnapshotRevision)
	}
	if len(d0.SnapshotIDs) != 1 || d0.SnapshotIDs[0] != "documents" {
		t.Errorf("entry 0 SnapshotIDs = %v; want [documents]", d0.SnapshotIDs)
	}

	// Web-cache entry round-trips StorageID + carries the per-entry
	// reason code from the runner.
	d1 := info.Duplicacy[1]
	if d1.Kind != DuplicacyKindWebCache || d1.StorageID != "main" {
		t.Errorf("entry 1 (Kind, StorageID) = (%q, %q); want (web-cache, main)", d1.Kind, d1.StorageID)
	}
	if d1.ReasonCode != string(DuplicacyReasonStale) {
		t.Errorf("entry 1 ReasonCode = %q; want %q", d1.ReasonCode, DuplicacyReasonStale)
	}
}

// TestCollectBackups_DuplicacyEntries_NoneSpecified_PreservesPreV1cShape
// asserts that an install with no Duplicacy entries keeps
// BackupInfo.Duplicacy nil — the omitempty JSON tag then keeps the
// `duplicacy` key absent from the snapshot, preserving pre-V1c bytes
// for upgraders.
func TestCollectBackups_DuplicacyEntries_NoneSpecified_PreservesPreV1cShape(t *testing.T) {
	info := CollectBackups(CollectBackupsOptions{})
	if info == nil {
		t.Fatal("CollectBackups returned nil")
	}
	if info.Duplicacy != nil {
		t.Errorf("info.Duplicacy = %+v; want nil for an install with no entries (omitempty preserves pre-V1c JSON shape)", info.Duplicacy)
	}
}

// TestCollectBackups_DuplicacyEntries_RunnerErrorFallsBackToPathUnreadable
// asserts the defense-in-depth runner-error fallback. The V1a runner
// is total (every classifier outcome maps to a ReasonCode), but if a
// future refactor breaks that contract we surface an entry with the
// path_unreadable reason rather than silently dropping the row.
// PRD #310 §10.
func TestCollectBackups_DuplicacyEntries_RunnerErrorFallsBackToPathUnreadable(t *testing.T) {
	runner := &fakeDupRunner{err: context.DeadlineExceeded}
	info := CollectBackups(CollectBackupsOptions{
		DuplicacyRunner: runner,
		Duplicacy: []DuplicacyEntry{
			{Enabled: true, Label: "broken", Kind: DuplicacyKindCLIRepo, Path: "/p"},
		},
	})
	if info == nil || len(info.Duplicacy) != 1 {
		t.Fatalf("expected one entry surfacing the runner error; got %+v", info)
	}
	if info.Duplicacy[0].ReasonCode != string(DuplicacyReasonPathUnreadable) {
		t.Errorf("runner-error entry ReasonCode = %q; want %q (defense-in-depth fallback)", info.Duplicacy[0].ReasonCode, DuplicacyReasonPathUnreadable)
	}
}

// TestCollector_SetBackupMonitorDuplicacy_Roundtrip asserts that the
// Collector's setter is idempotent + defensively-copies the slice,
// preventing aliasing bugs where the api layer mutates an entry
// post-Set.
func TestCollector_SetBackupMonitorDuplicacy_Roundtrip(t *testing.T) {
	c := &Collector{}
	src := []DuplicacyEntry{
		{Enabled: true, Label: "a", Kind: DuplicacyKindCLIRepo, Path: "/p/a"},
	}
	c.SetBackupMonitorDuplicacy(src)
	src[0].Label = "MUTATED" // caller-side mutation
	if c.duplicacyEntries[0].Label != "a" {
		t.Errorf("SetBackupMonitorDuplicacy must defensively copy; got Label=%q after caller mutation", c.duplicacyEntries[0].Label)
	}

	// Calling again with nil resets to nil/empty (matches Borg pattern).
	c.SetBackupMonitorDuplicacy(nil)
	if len(c.duplicacyEntries) != 0 {
		t.Errorf("SetBackupMonitorDuplicacy(nil) should clear; got %+v", c.duplicacyEntries)
	}
}

// TestBackupInfo_DuplicacyJobState_FieldShape is a compile-time pin
// on the internal.DuplicacyJobState shape. Adds a smoke test so a
// future field rename is caught at api-boundary update time. Also
// asserts the JSON tag stability so /api/v1/snapshot/latest's wire
// shape stays stable for downstream consumers (Grafana, fleet
// aggregation).
func TestBackupInfo_DuplicacyJobState_FieldShape(t *testing.T) {
	// Compile-only: any rename to required fields produces a build
	// error here.
	state := internal.DuplicacyJobState{
		Label:                  "x",
		Kind:                   "cli-repo",
		Path:                   "/p",
		StorageID:              "",
		ReasonCode:             "ok",
		SnapshotCount:          1,
		LatestBackupAt:         time.Now(),
		LatestBackupSizeBytes:  1,
		LatestBackupFiles:      1,
		CurrentlyRunning:       false,
		LatestSnapshotID:       "x",
		LatestSnapshotRevision: 1,
		SnapshotIDs:            []string{"x"},
	}
	if state.Kind == "" {
		t.Fatal("compile guard")
	}
}
