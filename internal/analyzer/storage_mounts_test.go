package analyzer

import (
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #300 — when running on a Synology DSM host but no /volume*
// bind mounts are configured in the container, the dashboard's
// STORAGE section silently shows only DSM system partitions
// (/dev/md0 mounted at /boot, /log, /mnt). The user has no signal
// telling them anything is misconfigured. analyzeStorageMounts
// surfaces this as a Warning finding so the bind-mount gap appears
// alongside other actionable issues.

// snapshotWith is a small builder that produces a minimally-populated
// snapshot for the storage-mount analyzer. Other analyzer rules need
// other fields populated (SMART, Docker, etc.) but those are
// independent — they short-circuit on empty input.
func snapshotWith(platform string, disks []internal.DiskInfo) *internal.Snapshot {
	return &internal.Snapshot{
		System: internal.SystemInfo{Platform: platform},
		Disks:  disks,
	}
}

// TestAnalyzeStorageMounts_FiresOnSynologyWithNoVolumes is the
// primary issue-#300 regression guard. Replays the reporter's
// post-platform-detection-fix state: platform = "synology",
// snap.disks contains only /boot, /log, /mnt entries — no /volume*
// in sight. The finding must fire with Warning severity and a
// description that names the exact bind-mount the user needs.
func TestAnalyzeStorageMounts_FiresOnSynologyWithNoVolumes(t *testing.T) {
	snap := snapshotWith("synology", []internal.DiskInfo{
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2.3, UsedGB: 1.3, UsedPct: 62},
		{Device: "/dev/md0", MountPoint: "/log", TotalGB: 0.2, UsedGB: 0.05, UsedPct: 28},
	})

	findings := analyzeStorageMounts(snap)
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d:\n%+v", len(findings), findings)
	}
	f := findings[0]
	if f.Severity != internal.SeverityWarning {
		t.Errorf("severity = %v, want Warning", f.Severity)
	}
	if f.Category != internal.CategoryDisk {
		t.Errorf("category = %v, want disk", f.Category)
	}
	// The Action text should mention the literal bind-mount syntax
	// so the user can copy-paste rather than guess. Pin this loosely
	// (substring) so future copy edits don't break the test.
	if !strings.Contains(f.Action, "/volume1:/host/volume1:ro") {
		t.Errorf("Action should give the user a copy-pasteable bind-mount hint; got: %q", f.Action)
	}
	// Evidence should list the mounts the container CAN see, so the
	// operator immediately knows what's there vs what should be.
	if len(f.Evidence) == 0 {
		t.Errorf("Evidence is empty; should list visible bind mounts so the operator can compare to README")
	}
}

// TestAnalyzeStorageMounts_DoesNotFireWhenAtLeastOneVolumeMounted
// pins the "user has at least started setting this up" case: as
// soon as one /volume* mount appears, the finding stops firing.
// We don't try to detect "missing /volume2 but /volume1 is there"
// — too much false-positive risk (the user might genuinely only
// have one volume).
func TestAnalyzeStorageMounts_DoesNotFireWhenAtLeastOneVolumeMounted(t *testing.T) {
	snap := snapshotWith("synology", []internal.DiskInfo{
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2.3, UsedGB: 1.3, UsedPct: 62},
		{Device: "/dev/mapper/cachedev_0", MountPoint: "/volume1", TotalGB: 23000, UsedGB: 21000, UsedPct: 89},
	})

	findings := analyzeStorageMounts(snap)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (user has /volume1), got %d:\n%+v", len(findings), findings)
	}
}

// TestAnalyzeStorageMounts_NoOpOnNonSynologyPlatforms is the
// platform-gating guard. Unraid users WANT to see /boot (their
// flash drive) without being told to mount /volume* (an unrelated
// concept). Same for TrueNAS, QNAP, plain Linux. The finding is
// strictly Synology-specific.
func TestAnalyzeStorageMounts_NoOpOnNonSynologyPlatforms(t *testing.T) {
	disks := []internal.DiskInfo{
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2, UsedGB: 1, UsedPct: 50},
	}
	for _, plat := range []string{"unraid", "truenas", "qnap", "proxmox", "alpine", "linux", ""} {
		t.Run(plat, func(t *testing.T) {
			snap := snapshotWith(plat, disks)
			findings := analyzeStorageMounts(snap)
			if len(findings) != 0 {
				t.Errorf("platform %q should not emit Synology-specific finding; got: %+v", plat, findings)
			}
		})
	}
}

// TestAnalyzeStorageMounts_NoOpWhenDiskCollectionFailed pins the
// "df itself failed" case. An empty disk list usually means a
// different failure mode (df errored, no bind mounts at all, etc.)
// and a separate finding will surface elsewhere. Don't pile on a
// confusing "missing volumes" hint when we have nothing to compare
// against.
func TestAnalyzeStorageMounts_NoOpWhenDiskCollectionFailed(t *testing.T) {
	snap := snapshotWith("synology", nil)
	findings := analyzeStorageMounts(snap)
	if len(findings) != 0 {
		t.Errorf("empty disks list should not fire the finding (no signal to act on); got: %+v", findings)
	}
}

// TestAnalyzeStorageMounts_FiringFindingIsPicked UpByAnalyze is the
// integration-level guard: the new analyzer must actually be wired
// into the top-level Analyze() call. Without the wire-up the
// per-rule unit tests above all pass but the dashboard never sees
// the finding. Mirrors the structural guard pattern from the
// existing #206 tests.
func TestAnalyze_IncludesStorageMountsFinding(t *testing.T) {
	snap := snapshotWith("synology", []internal.DiskInfo{
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2.3, UsedGB: 1.3, UsedPct: 62},
	})

	findings := Analyze(snap)
	var saw bool
	for _, f := range findings {
		if strings.Contains(f.Title, "Synology storage volumes not bind-mounted") {
			saw = true
			break
		}
	}
	if !saw {
		titles := make([]string, 0, len(findings))
		for _, f := range findings {
			titles = append(titles, f.Title)
		}
		t.Errorf("Analyze() did not include the Synology storage-mounts finding; titles seen: %v", titles)
	}
}
