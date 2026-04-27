package collector

import (
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #300 — on a Synology DS918+ Container Manager deployment with
// the README's `/boot:/host/boot:ro`, `/var/log:/host/log:ro`, and
// `/mnt:/host/mnt:ro` bind mounts (and no `/volumeN` bind mounts),
// the dashboard's STORAGE section showed three entries — `/boot`,
// `/log`, `/mnt` — all reporting the system root partition
// `/dev/md0`. `/boot` and `/mnt` had byte-for-byte identical
// Size/Used/Available numbers because they're the same filesystem
// mounted at two paths.
//
// dedupSameFilesystemMounts collapses these duplicate entries so the
// dashboard shows the system-root partition once, freeing vertical
// space for actual user storage. The `/log` entry has a different
// size (smaller, because it's a separate filesystem on the host) and
// must NOT be folded into the system root.

func TestDedupSameFilesystemMounts_CollapsesDS918SystemRootDuplicates(t *testing.T) {
	// Verbatim shape of the issue #300 reporter's API output. /boot
	// and /mnt are byte-identical on /dev/md0; /log is a different
	// filesystem with much smaller size; we want 2 results, not 3.
	in := []internal.DiskInfo{
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2.3, UsedGB: 1.3, FreeGB: 0.82, UsedPct: 62},
		{Device: "/dev/md0", MountPoint: "/log", TotalGB: 0.2, UsedGB: 0.05, FreeGB: 0.14, UsedPct: 28},
		{Device: "/dev/md0", MountPoint: "/mnt", TotalGB: 2.3, UsedGB: 1.3, FreeGB: 0.82, UsedPct: 62},
	}

	got := dedupSameFilesystemMounts(in)

	if len(got) != 2 {
		t.Fatalf("expected 2 entries after dedup (/boot+/log, /mnt collapsed into /boot), got %d:\n%+v", len(got), got)
	}
	// Surviving mounts must include /log and one of {/boot, /mnt}.
	var saw struct{ boot, log, mnt bool }
	for _, d := range got {
		switch d.MountPoint {
		case "/boot":
			saw.boot = true
		case "/log":
			saw.log = true
		case "/mnt":
			saw.mnt = true
		}
	}
	if !saw.log {
		t.Errorf("/log entry must survive dedup (different filesystem); got: %+v", got)
	}
	if saw.boot && saw.mnt {
		t.Errorf("both /boot AND /mnt survived; one should have been collapsed (they have identical Size/Used)")
	}
	if !saw.boot && !saw.mnt {
		t.Errorf("both /boot AND /mnt got collapsed; at least one must survive to represent the system root")
	}
}

func TestDedupSameFilesystemMounts_PrefersUserStoragePathOverSystemPath(t *testing.T) {
	// When a real user-storage mount (/volume1, /mnt/disk1, etc.)
	// shares a filesystem with a system mount, the user-storage
	// path must win — that's what the operator wants to see in the
	// dashboard. Synthetic but it matches the contract documented
	// in preferUserStoragePath.
	in := []internal.DiskInfo{
		{Device: "/dev/sda1", MountPoint: "/host", TotalGB: 18000, UsedGB: 12000, UsedPct: 67},
		{Device: "/dev/sda1", MountPoint: "/volume1", TotalGB: 18000, UsedGB: 12000, UsedPct: 67},
	}

	got := dedupSameFilesystemMounts(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].MountPoint != "/volume1" {
		t.Errorf("/volume1 must beat /host (real user storage); got %q", got[0].MountPoint)
	}
}

func TestDedupSameFilesystemMounts_TieBreakOnShortestPath(t *testing.T) {
	// Two non-user-storage mounts on the same filesystem — neither
	// is a "real" volume. Tie-break is the shorter path so users
	// see /boot rather than /var/lib/something.
	in := []internal.DiskInfo{
		{Device: "/dev/md0", MountPoint: "/var/lib/cache", TotalGB: 2.3, UsedGB: 1.3, UsedPct: 62},
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2.3, UsedGB: 1.3, UsedPct: 62},
	}

	got := dedupSameFilesystemMounts(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].MountPoint != "/boot" {
		t.Errorf("expected shorter path /boot to win; got %q", got[0].MountPoint)
	}
}

func TestDedupSameFilesystemMounts_DoesNotCollapseDifferentFilesystems(t *testing.T) {
	// Two real /volume mounts on different cachedevs (typical
	// Synology setup with multiple storage pools) must both survive.
	// Same Device prefix isn't enough — TotalGB differs, so the
	// dedup key differs.
	in := []internal.DiskInfo{
		{Device: "/dev/mapper/cachedev_0", MountPoint: "/volume1", TotalGB: 23000, UsedGB: 21000, UsedPct: 89},
		{Device: "/dev/mapper/cachedev_1", MountPoint: "/volume2", TotalGB: 1800, UsedGB: 174, UsedPct: 10},
	}

	got := dedupSameFilesystemMounts(in)
	if len(got) != 2 {
		t.Errorf("expected both volumes to survive (different filesystems), got %d:\n%+v", len(got), got)
	}
}

func TestDedupSameFilesystemMounts_PreservesOrderingForDeterministicDashboard(t *testing.T) {
	// The dashboard renders disks in the order returned by the
	// collector, so dedup MUST be order-preserving — otherwise the
	// storage section would shuffle on every scan, which looks
	// alarming and breaks the "did anything change?" eyeball test.
	in := []internal.DiskInfo{
		{Device: "/dev/sda1", MountPoint: "/volume1", TotalGB: 100, UsedGB: 50, UsedPct: 50},
		{Device: "/dev/sdb1", MountPoint: "/volume2", TotalGB: 200, UsedGB: 80, UsedPct: 40},
		{Device: "/dev/md0", MountPoint: "/boot", TotalGB: 2, UsedGB: 1, UsedPct: 50},
	}

	got := dedupSameFilesystemMounts(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries (no duplicates in input), got %d", len(got))
	}
	wantOrder := []string{"/volume1", "/volume2", "/boot"}
	for i, w := range wantOrder {
		if got[i].MountPoint != w {
			t.Errorf("position %d: got %q, want %q (full order: %v)", i, got[i].MountPoint, w, mountPaths(got))
		}
	}
}

func TestDedupSameFilesystemMounts_NoOpOnSingleDisk(t *testing.T) {
	// Edge case: single-disk input must return the same disk
	// without any allocation overhead. Common on TrueNAS SCALE
	// installs where the only relevant mount is the ZFS pool.
	in := []internal.DiskInfo{
		{Device: "tank/data", MountPoint: "/mnt/tank/data", TotalGB: 8000, UsedGB: 4500, UsedPct: 56},
	}
	got := dedupSameFilesystemMounts(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].MountPoint != "/mnt/tank/data" {
		t.Errorf("entry mutated: %+v", got[0])
	}
}

func TestDedupSameFilesystemMounts_NoOpOnEmptyInput(t *testing.T) {
	got := dedupSameFilesystemMounts(nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestIsUserStorageMount_PinsKnownPrefixes(t *testing.T) {
	cases := []struct {
		mount string
		want  bool
	}{
		// Synology DSM
		{"/volume1", true},
		{"/volume2", true},
		{"/volume99", true},
		// Unraid
		{"/mnt/disk1", true},
		{"/mnt/disk12", true},
		{"/mnt/cache", true},
		{"/mnt/cache_pool", true},
		{"/mnt/user", true},
		{"/mnt/user0", true},
		// System / non-storage
		{"/boot", false},
		{"/log", false},
		{"/mnt", false}, // bare /mnt isn't a real volume
		{"/var/log", false},
		{"/", false},
		{"/host/boot", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.mount, func(t *testing.T) {
			if got := isUserStorageMount(tc.mount); got != tc.want {
				t.Errorf("isUserStorageMount(%q) = %v, want %v", tc.mount, got, tc.want)
			}
		})
	}
}

// mountPaths is a small helper used by failure messages above to
// render a disk slice as just its mount paths, joined for readability.
func mountPaths(disks []internal.DiskInfo) string {
	out := make([]string, 0, len(disks))
	for _, d := range disks {
		out = append(out, d.MountPoint)
	}
	return strings.Join(out, ", ")
}
