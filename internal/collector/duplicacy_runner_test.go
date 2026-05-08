package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixtureNow is the deterministic "now" used by the verbatim-fixture
// tests below. Pinned a few hours after the healthy fixture's latest
// snapshot end_time (1714867315 = 2024-05-04 22:01:55 UTC) so:
//
//   - healthy fixtures (rev 1 + rev 2) classify as ok (well under 30d)
//   - stale fixture (rev 45, end 2024-03-01) classifies as stale (>30d)
//
// Pinning the clock keeps the verbatim Unix-timestamp fixtures from
// drifting into staleness as wall-clock time advances after they
// were committed. Issue #311.
var fixtureNow = time.Unix(1714900000, 0).UTC()

// TestDiskDuplicacyRunner_CLIRepo_Healthy exercises the canonical
// happy path: a fully-populated CLI repo with two snapshot revisions
// for the "documents" snapshot id. Asserts ReasonCode=ok, both revs
// counted, latest rev's metadata surfaced.
func TestDiskDuplicacyRunner_CLIRepo_Healthy(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/healthy",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonOK {
		t.Errorf("ReasonCode = %q; want ok", got.ReasonCode)
	}
	if got.SnapshotCount != 2 {
		t.Errorf("SnapshotCount = %d; want 2", got.SnapshotCount)
	}
	if got.LatestSnapshotRevision != 2 {
		t.Errorf("LatestSnapshotRevision = %d; want 2", got.LatestSnapshotRevision)
	}
	if got.LatestSnapshotID != "documents" {
		t.Errorf("LatestSnapshotID = %q; want documents", got.LatestSnapshotID)
	}
	if got.LatestBackupSizeBytes != 104923000 {
		t.Errorf("LatestBackupSizeBytes = %d; want 104923000", got.LatestBackupSizeBytes)
	}
	if got.LatestBackupFiles != 1240 {
		t.Errorf("LatestBackupFiles = %d; want 1240", got.LatestBackupFiles)
	}
	if got.LatestBackupAt.Unix() != 1714867315 {
		t.Errorf("LatestBackupAt = %v (unix=%d); want unix=1714867315", got.LatestBackupAt, got.LatestBackupAt.Unix())
	}
	if got.CurrentlyRunning {
		t.Error("CurrentlyRunning = true; want false (no lock present)")
	}
	if len(got.SnapshotIDs) != 1 || got.SnapshotIDs[0] != "documents" {
		t.Errorf("SnapshotIDs = %v; want [documents]", got.SnapshotIDs)
	}
}

// TestDiskDuplicacyRunner_CLIRepo_NoSnapshotsYet exercises the
// fresh-init path: .duplicacy/preferences exists but no snapshot
// files have been written yet.
func TestDiskDuplicacyRunner_CLIRepo_NoSnapshotsYet(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/no-snapshots",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonNoSnapshotsYet {
		t.Errorf("ReasonCode = %q; want no_snapshots_yet", got.ReasonCode)
	}
	if got.SnapshotCount != 0 {
		t.Errorf("SnapshotCount = %d; want 0", got.SnapshotCount)
	}
	if !got.LatestBackupAt.IsZero() {
		t.Errorf("LatestBackupAt = %v; want zero", got.LatestBackupAt)
	}
}

// TestDiskDuplicacyRunner_CLIRepo_Stale exercises the staleness
// classifier. Fixture's only snapshot has end_time=2024-03-01;
// fixtureNow is 2024-05-05; default StaleAfter (30d) means this
// repo classifies as stale.
func TestDiskDuplicacyRunner_CLIRepo_Stale(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/stale",
		// StaleAfter omitted — default 30d applies via runner.
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonStale {
		t.Errorf("ReasonCode = %q; want stale", got.ReasonCode)
	}
	if got.SnapshotCount != 1 {
		t.Errorf("SnapshotCount = %d; want 1 (the stale snapshot still counted)", got.SnapshotCount)
	}
	if got.LatestSnapshotRevision != 45 {
		t.Errorf("LatestSnapshotRevision = %d; want 45", got.LatestSnapshotRevision)
	}
}

// TestDiskDuplicacyRunner_CLIRepo_StaleAfterRespectsCustomThreshold
// pins that an explicitly-supplied StaleAfter overrides the 30-day
// default. The "stale" fixture has a snapshot ~64 days old; with
// StaleAfter=90 it should classify as ok instead of stale.
func TestDiskDuplicacyRunner_CLIRepo_StaleAfterRespectsCustomThreshold(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled:    true,
		Kind:       DuplicacyKindCLIRepo,
		Path:       "testdata/duplicacy/cli-repo/stale",
		StaleAfter: 90,
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonOK {
		t.Errorf("StaleAfter=90 ReasonCode = %q; want ok (snapshot ~64d old)", got.ReasonCode)
	}
}

// TestDiskDuplicacyRunner_CLIRepo_NotARepo exercises the path-exists-
// but-no-.duplicacy-subdir path. Distinguishes "not mounted" from
// "mounted but not a Duplicacy repo" per PRD #310 user story 15.
func TestDiskDuplicacyRunner_CLIRepo_NotARepo(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/not-a-repo",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonNotARepo {
		t.Errorf("ReasonCode = %q; want not_a_duplicacy_repo", got.ReasonCode)
	}
}

// TestDiskDuplicacyRunner_WebCache_Healthy exercises the saspus-style
// layout. Two snapshot ids ("media", "documents") under the named
// storage; latest end_time on the documents id (later than media).
func TestDiskDuplicacyRunner_WebCache_Healthy(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled:   true,
		Kind:      DuplicacyKindWebCache,
		Path:      "testdata/duplicacy/web-cache/healthy",
		StorageID: "storage-main",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonOK {
		t.Errorf("ReasonCode = %q; want ok", got.ReasonCode)
	}
	if got.SnapshotCount != 2 {
		t.Errorf("SnapshotCount = %d; want 2 (one per snapshot id)", got.SnapshotCount)
	}
	// documents/1 has end_time=1714867253; media/1 has end_time=1714781902.
	// documents wins as latest.
	if got.LatestSnapshotID != "documents" {
		t.Errorf("LatestSnapshotID = %q; want documents (later end_time wins)", got.LatestSnapshotID)
	}
	if len(got.SnapshotIDs) != 2 {
		t.Errorf("SnapshotIDs len = %d; want 2", len(got.SnapshotIDs))
	}
	// Sorted alphabetically.
	if got.SnapshotIDs[0] != "documents" || got.SnapshotIDs[1] != "media" {
		t.Errorf("SnapshotIDs = %v; want [documents media]", got.SnapshotIDs)
	}
}

// TestDiskDuplicacyRunner_WebCache_StorageIDNotFound exercises the
// typo'd-storage_id path. Path resolves but the named subdir is
// absent.
func TestDiskDuplicacyRunner_WebCache_StorageIDNotFound(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled:   true,
		Kind:      DuplicacyKindWebCache,
		Path:      "testdata/duplicacy/web-cache/healthy",
		StorageID: "storage-typo-nonexistent",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonStorageIDNotFound {
		t.Errorf("ReasonCode = %q; want storage_id_not_found", got.ReasonCode)
	}
}

// TestDiskDuplicacyRunner_PathNotFound exercises the universal
// precondition: a Path that doesn't exist on disk produces
// path_not_found regardless of Kind.
func TestDiskDuplicacyRunner_PathNotFound(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	cases := []struct {
		name string
		kind string
	}{
		{"cli-repo", DuplicacyKindCLIRepo},
		{"web-cache", DuplicacyKindWebCache},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Read(context.Background(), DuplicacyEntry{
				Enabled:   true,
				Kind:      tc.kind,
				Path:      "testdata/duplicacy/this/path/does/not/exist",
				StorageID: "irrelevant",
			})
			if err != nil {
				t.Fatalf("Read err = %v; want nil", err)
			}
			if got.ReasonCode != DuplicacyReasonPathNotFound {
				t.Errorf("ReasonCode = %q; want path_not_found", got.ReasonCode)
			}
		})
	}
}

// TestDiskDuplicacyRunner_PathUnreadable exercises the precondition's
// other failure mode: Path exists but stat() can't traverse it. We
// simulate this with a chmod-zeroed dir (Linux/Darwin); on rare
// permission-permissive setups we skip rather than fail.
func TestDiskDuplicacyRunner_PathUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0 would not deny stat()")
	}
	tmp := t.TempDir()
	denied := filepath.Join(tmp, "denied")
	if err := os.MkdirAll(denied, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Inner path is unreachable because the parent has mode 0
	// (no execute bit). os.Stat on innerPath returns EACCES.
	innerPath := filepath.Join(denied, "repo")
	if err := os.MkdirAll(innerPath, 0o700); err != nil {
		t.Fatalf("mkdir inner: %v", err)
	}
	if err := os.Chmod(denied, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0o700) })

	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    innerPath,
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	// Either path_not_found or path_unreadable is acceptable here —
	// some kernels return EACCES (→ path_unreadable), others surface
	// ENOENT after the EACCES on parent (→ path_not_found). The
	// actionable user-facing distinction is the same: "fix the
	// permission/mount on the parent." Pin that we DON'T classify
	// as ok or no_snapshots_yet.
	if got.ReasonCode != DuplicacyReasonPathUnreadable && got.ReasonCode != DuplicacyReasonPathNotFound {
		t.Errorf("ReasonCode = %q; want path_unreadable or path_not_found", got.ReasonCode)
	}
}

// TestDiskDuplicacyRunner_CorruptSnapshot exercises the
// graceful-failure path. The fixture mixes a corrupt snapshot file
// with an otherwise-valid repo structure; the corrupt signal wins
// over any count-based outcome (PRD #310 user story 14).
func TestDiskDuplicacyRunner_CorruptSnapshot(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/corrupt",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonCorruptSnapshot {
		t.Errorf("ReasonCode = %q; want corrupt_snapshot", got.ReasonCode)
	}
}

// TestDiskDuplicacyRunner_CurrentlyRunning_LockPresent exercises the
// best-effort lock-detection. The "running" fixture carries a healthy
// snapshot (so ReasonCode=ok) AND a non-empty .duplicacy/locks/ dir
// (so CurrentlyRunning=true). Pins both axes orthogonally.
func TestDiskDuplicacyRunner_CurrentlyRunning_LockPresent(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/running",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.ReasonCode != DuplicacyReasonOK {
		t.Errorf("ReasonCode = %q; want ok (lock orthogonal to reason)", got.ReasonCode)
	}
	if !got.CurrentlyRunning {
		t.Error("CurrentlyRunning = false; want true (locks dir non-empty)")
	}
}

// TestDiskDuplicacyRunner_CurrentlyRunning_LockAbsent is the
// inverse: a healthy repo with no lock should report
// CurrentlyRunning=false. Re-uses the healthy fixture which has no
// locks/ dir.
func TestDiskDuplicacyRunner_CurrentlyRunning_LockAbsent(t *testing.T) {
	r := newDiskDuplicacyRunnerAt(fixtureNow)
	got, err := r.Read(context.Background(), DuplicacyEntry{
		Enabled: true,
		Kind:    DuplicacyKindCLIRepo,
		Path:    "testdata/duplicacy/cli-repo/healthy",
	})
	if err != nil {
		t.Fatalf("Read err = %v; want nil", err)
	}
	if got.CurrentlyRunning {
		t.Error("CurrentlyRunning = true; want false (no lock present)")
	}
}

// TestDiskDuplicacyRunner_DefaultsAndKinds covers small contract
// invariants that don't need their own fixture trees.
func TestDiskDuplicacyRunner_DefaultsAndKinds(t *testing.T) {
	t.Run("DefaultStaleAfterDays is 30", func(t *testing.T) {
		if DuplicacyDefaultStaleAfterDays != 30 {
			t.Errorf("DuplicacyDefaultStaleAfterDays = %d; want 30", DuplicacyDefaultStaleAfterDays)
		}
	})
	t.Run("kind constants are stable strings", func(t *testing.T) {
		if DuplicacyKindCLIRepo != "cli-repo" {
			t.Errorf("DuplicacyKindCLIRepo = %q; want cli-repo", DuplicacyKindCLIRepo)
		}
		if DuplicacyKindWebCache != "web-cache" {
			t.Errorf("DuplicacyKindWebCache = %q; want web-cache", DuplicacyKindWebCache)
		}
	})
	t.Run("unknown kind classifies as not_a_repo", func(t *testing.T) {
		r := newDiskDuplicacyRunnerAt(fixtureNow)
		// Path that exists so the precondition passes.
		got, err := r.Read(context.Background(), DuplicacyEntry{
			Enabled: true,
			Kind:    "rclone-style-future-kind",
			Path:    "testdata/duplicacy/cli-repo/healthy",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.ReasonCode != DuplicacyReasonNotARepo {
			t.Errorf("ReasonCode = %q; want not_a_duplicacy_repo (unknown kind defense-in-depth)", got.ReasonCode)
		}
	})
	t.Run("empty path classifies as path_not_found", func(t *testing.T) {
		r := newDiskDuplicacyRunnerAt(fixtureNow)
		got, err := r.Read(context.Background(), DuplicacyEntry{
			Enabled: true,
			Kind:    DuplicacyKindCLIRepo,
			Path:    "",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.ReasonCode != DuplicacyReasonPathNotFound {
			t.Errorf("ReasonCode = %q; want path_not_found", got.ReasonCode)
		}
	})
}

// TestParseRevisionFromName pins the filename-to-revision parser
// across the formats real Duplicacy emits (decimal integer) plus a
// couple of defensive variants.
func TestParseRevisionFromName(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOk bool
	}{
		{"1", 1, true},
		{"42", 42, true},
		{"1023", 1023, true},
		{"v1", 1, true},
		{"007", 7, true},
		{"0", 0, true},
		{"00", 0, true},
		{"", 0, false},
		{"abc", 0, false},
		{"1.json", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parseRevisionFromName(tc.in)
			if ok != tc.wantOk {
				t.Errorf("parseRevisionFromName(%q) ok = %v; want %v", tc.in, ok, tc.wantOk)
			}
			if got != tc.want {
				t.Errorf("parseRevisionFromName(%q) = %d; want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestReadCLIRepoStorageName_FallbackToDefault pins the runner's
// resilience: a missing/malformed preferences file falls back to
// "default" so the snapshots-dir walk still runs and surfaces
// no_snapshots_yet (the right user-facing outcome).
func TestReadCLIRepoStorageName_FallbackToDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Run("missing file", func(t *testing.T) {
		got := readCLIRepoStorageName(filepath.Join(tmp, "nonexistent"))
		if got != "default" {
			t.Errorf("missing file → %q; want default", got)
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		bad := filepath.Join(tmp, "bad-prefs")
		if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := readCLIRepoStorageName(bad)
		if got != "default" {
			t.Errorf("malformed → %q; want default", got)
		}
	})
	t.Run("custom name", func(t *testing.T) {
		good := filepath.Join(tmp, "good-prefs")
		body := `[{"name":"offsite-b2","id":"x","storage":"b2://bucket"}]`
		if err := os.WriteFile(good, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		got := readCLIRepoStorageName(good)
		if got != "offsite-b2" {
			t.Errorf("custom-name → %q; want offsite-b2", got)
		}
	})
	t.Run("empty array falls back", func(t *testing.T) {
		empty := filepath.Join(tmp, "empty-prefs")
		if err := os.WriteFile(empty, []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := readCLIRepoStorageName(empty)
		if got != "default" {
			t.Errorf("empty array → %q; want default", got)
		}
	})
}

// TestDuplicacySnapshotJSON_EndTimeFallback covers the end_time/
// start_time fallback used when EndTime is unset (a backup that
// started but never finished).
func TestDuplicacySnapshotJSON_EndTimeFallback(t *testing.T) {
	cases := []struct {
		name string
		in   duplicacySnapshotJSON
		zero bool
	}{
		{"end_time set", duplicacySnapshotJSON{StartTime: 100, EndTime: 200}, false},
		{"only start_time", duplicacySnapshotJSON{StartTime: 100, EndTime: 0}, false},
		{"both zero", duplicacySnapshotJSON{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.endTime()
			if got.IsZero() != tc.zero {
				t.Errorf("endTime IsZero = %v; want %v", got.IsZero(), tc.zero)
			}
		})
	}
}

// ---------- pure-function classifier table-test ----------

// TestClassifySnapshotWalk_TableDriven is the centrepiece pure-
// function classifier test. One row per reason code that the
// classifier emits (path_*/not_a_repo/storage_id_not_found are
// emitted by the resolvers, NOT this function — they're covered by
// the runner-level tests above).
func TestClassifySnapshotWalk_TableDriven(t *testing.T) {
	now := time.Unix(1714900000, 0).UTC()
	older30d := now.Add(-31 * 24 * time.Hour)
	fresh := now.Add(-2 * time.Hour)

	cases := []struct {
		name      string
		walk      snapshotWalkResult
		staleDays int
		want      DuplicacyReason
	}{
		{
			name:      "ok — fresh snapshot under threshold",
			walk:      snapshotWalkResult{count: 3, latest: fresh},
			staleDays: 30,
			want:      DuplicacyReasonOK,
		},
		{
			name:      "no_snapshots_yet — count zero, no latest",
			walk:      snapshotWalkResult{},
			staleDays: 30,
			want:      DuplicacyReasonNoSnapshotsYet,
		},
		{
			name:      "no_snapshots_yet — count nonzero but latest zero (defensive)",
			walk:      snapshotWalkResult{count: 1},
			staleDays: 30,
			want:      DuplicacyReasonNoSnapshotsYet,
		},
		{
			name:      "stale — older than 30d default",
			walk:      snapshotWalkResult{count: 1, latest: older30d},
			staleDays: 30,
			want:      DuplicacyReasonStale,
		},
		{
			name:      "ok — older than 30d but custom threshold 90d",
			walk:      snapshotWalkResult{count: 1, latest: older30d},
			staleDays: 90,
			want:      DuplicacyReasonOK,
		},
		{
			name:      "corrupt wins over count-zero",
			walk:      snapshotWalkResult{corrupt: true, count: 0},
			staleDays: 30,
			want:      DuplicacyReasonCorruptSnapshot,
		},
		{
			name:      "corrupt wins over fresh ok",
			walk:      snapshotWalkResult{corrupt: true, count: 5, latest: fresh},
			staleDays: 30,
			want:      DuplicacyReasonCorruptSnapshot,
		},
		{
			name:      "corrupt wins over stale",
			walk:      snapshotWalkResult{corrupt: true, count: 1, latest: older30d},
			staleDays: 30,
			want:      DuplicacyReasonCorruptSnapshot,
		},
		{
			name:      "staleDays<=0 disables stale check (defensive)",
			walk:      snapshotWalkResult{count: 1, latest: older30d},
			staleDays: 0,
			want:      DuplicacyReasonOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifySnapshotWalkAt(tc.walk, tc.staleDays, now)
			if got.ReasonCode != tc.want {
				t.Errorf("ReasonCode = %q; want %q", got.ReasonCode, tc.want)
			}
			// Pin that the classifier doesn't drop walk-derived
			// fields — V1b/V1c rely on them.
			if got.SnapshotCount != tc.walk.count {
				t.Errorf("SnapshotCount = %d; want %d", got.SnapshotCount, tc.walk.count)
			}
			if !got.LatestBackupAt.Equal(tc.walk.latest) {
				t.Errorf("LatestBackupAt = %v; want %v", got.LatestBackupAt, tc.walk.latest)
			}
		})
	}
}

// TestClassifyPathPrecondition_TableDriven exercises the universal-
// precondition classifier as a pure function. Same pattern as the
// snapshot-walk classifier — fast, fixture-free.
func TestClassifyPathPrecondition_TableDriven(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "exists-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(tmp, "exists-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
		want DuplicacyReason
		pass bool
	}{
		{"empty", "", DuplicacyReasonPathNotFound, false},
		{"whitespace-only", "   ", DuplicacyReasonPathNotFound, false},
		{"missing", filepath.Join(tmp, "nope"), DuplicacyReasonPathNotFound, false},
		{"exists-dir → pass", dir, DuplicacyReason(""), true},
		{"exists-file (not a directory)", file, DuplicacyReasonPathUnreadable, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pass := classifyPathPrecondition(tc.path)
			if pass != tc.pass {
				t.Errorf("pass = %v; want %v (got reason %q)", pass, tc.pass, got)
			}
			if !tc.pass && got != tc.want {
				t.Errorf("reason = %q; want %q", got, tc.want)
			}
		})
	}
}

// ---------- DuplicacyRunner contract test (parallel to BorgRunner) ----------

// fakeDuplicacyRunner is an in-process DuplicacyRunner implementation
// usable by V1b/V1c tests when fixture files would be heavyweight.
// Mirrors the fakeBorgRunner pattern from borg_runner_test.go.
type fakeDuplicacyRunner struct {
	byPath map[string]fakeDuplicacyScenario
}

type fakeDuplicacyScenario struct {
	State DuplicacyState
	Err   error
}

func newFakeDuplicacyRunner() *fakeDuplicacyRunner {
	return &fakeDuplicacyRunner{byPath: make(map[string]fakeDuplicacyScenario)}
}

func (r *fakeDuplicacyRunner) set(path string, s fakeDuplicacyScenario) {
	r.byPath[path] = s
}

func (r *fakeDuplicacyRunner) Read(ctx context.Context, entry DuplicacyEntry) (DuplicacyState, error) {
	s, ok := r.byPath[entry.Path]
	if !ok {
		return DuplicacyState{}, errors.New("fake: no scenario configured for path " + entry.Path)
	}
	return s.State, s.Err
}

// TestDuplicacyRunner_Contract_FakeIsAcceptedAtInterface pins that
// the test fake satisfies the interface (compile-time + behaviour).
// Future V1b/V1c work will rely on this.
func TestDuplicacyRunner_Contract_FakeIsAcceptedAtInterface(t *testing.T) {
	var r DuplicacyRunner = newFakeDuplicacyRunner()
	r.(*fakeDuplicacyRunner).set("/x", fakeDuplicacyScenario{State: DuplicacyState{ReasonCode: DuplicacyReasonOK, SnapshotCount: 7}})
	got, err := r.Read(context.Background(), DuplicacyEntry{Path: "/x"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.ReasonCode != DuplicacyReasonOK || got.SnapshotCount != 7 {
		t.Errorf("got %+v; want ReasonCode=ok SnapshotCount=7", got)
	}
}

// TestDuplicacyReason_AllConstantsExportedAndStable pins that all
// eight reason codes are exported and that their string values
// match the closed-set definitions in the issue body. Renaming
// these would break the V1b/V1c dashboard and Prometheus
// downstream consumers.
func TestDuplicacyReason_AllConstantsExportedAndStable(t *testing.T) {
	cases := []struct {
		c    DuplicacyReason
		want string
	}{
		{DuplicacyReasonOK, "ok"},
		{DuplicacyReasonPathNotFound, "path_not_found"},
		{DuplicacyReasonPathUnreadable, "path_unreadable"},
		{DuplicacyReasonNotARepo, "not_a_duplicacy_repo"},
		{DuplicacyReasonStorageIDNotFound, "storage_id_not_found"},
		{DuplicacyReasonNoSnapshotsYet, "no_snapshots_yet"},
		{DuplicacyReasonStale, "stale"},
		{DuplicacyReasonCorruptSnapshot, "corrupt_snapshot"},
	}
	for _, tc := range cases {
		if string(tc.c) != tc.want {
			t.Errorf("%v = %q; want %q", tc.c, string(tc.c), tc.want)
		}
	}
}
