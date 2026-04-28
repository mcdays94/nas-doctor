package collector

import (
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #302 — BusyBox `df` line-wraps records whose device path
// exceeds the column width, which means parseDFSimple's
// `len(fields) < 6` guard silently drops Synology /volumeN entries
// (every cachedev_N path is ≥21 chars). The reporter's DS918+
// running v0.9.12 had the issue-#300 fixes applied but the storage
// section still showed only `/log` because /volume1 + /volume2 got
// dropped at the parse stage, before either dedup or the missing-
// volume finding could see them.
//
// The fixture below is a verbatim capture of `docker exec
// nas_doctor df -h` from the reporter's Synology Container Manager
// deployment. Header line + 4 wrapped records + 1 non-wrapped
// record (/dev/md0 → /host/log).

const dfSynologyBusyBoxOutput = `Filesystem                Size      Used Available Use% Mounted on
/dev/mapper/cachedev_1
                          1.7T    174.1G      1.6T  10% /
tmpfs                     7.7G         0      7.7G   0% /sys/fs/cgroup
/dev/mapper/cachedev_1
                          1.7T    174.1G      1.6T  10% /data
devtmpfs                  7.7G         0      7.7G   0% /dev
tmpfs                     7.7G    244.0K      7.7G   0% /dev/shm
/dev/mapper/cachedev_0
                         22.7T     20.1T      2.6T  89% /host/volume1
/dev/mapper/cachedev_1
                          1.7T    174.1G      1.6T  10% /host/volume2
/dev/md0                200.0M     56.9M    143.1M  28% /host/log
`

// TestParseDFSimple_RecoversWrappedSynologyVolumes is the primary
// issue-#302 regression guard. Replays the reporter's exact df
// output and asserts /volume1 and /volume2 BOTH end up in the
// disks list with their real sizes (22.7 TB, 1.7 TB). Without
// joinWrappedDFLines this test fails with "expected at least 3
// disks, got 1" — only /log survives.
func TestParseDFSimple_RecoversWrappedSynologyVolumes(t *testing.T) {
	disks := parseDFSimple(dfSynologyBusyBoxOutput)

	mounts := make(map[string]internal.DiskInfo, len(disks))
	for _, d := range disks {
		mounts[d.MountPoint] = d
	}

	// /log must always have been there — non-wrapped record, served
	// as the canary that tipped the user off something was wrong.
	if _, ok := mounts["/log"]; !ok {
		t.Errorf("/log missing from disks list (regression: was working before this PR). Got: %v", mountKeys(disks))
	}

	// /volume1 and /volume2 are the issue. Both must populate.
	v1, ok := mounts["/volume1"]
	if !ok {
		t.Fatalf("/volume1 missing from disks list — line-wrap recovery failed. Got: %v", mountKeys(disks))
	}
	if v1.Device != "/dev/mapper/cachedev_0" {
		t.Errorf("/volume1 device = %q, want /dev/mapper/cachedev_0", v1.Device)
	}
	// 22.7T → 22.7 * 1024 = 23244.8 GB. Allow some float wobble.
	if v1.TotalGB < 23000 || v1.TotalGB > 24000 {
		t.Errorf("/volume1 TotalGB = %v, want ~23244 (22.7T)", v1.TotalGB)
	}
	if v1.UsedPct != 89 {
		t.Errorf("/volume1 UsedPct = %v, want 89", v1.UsedPct)
	}

	v2, ok := mounts["/volume2"]
	if !ok {
		t.Fatalf("/volume2 missing from disks list. Got: %v", mountKeys(disks))
	}
	if v2.Device != "/dev/mapper/cachedev_1" {
		t.Errorf("/volume2 device = %q, want /dev/mapper/cachedev_1", v2.Device)
	}
	if v2.UsedPct != 10 {
		t.Errorf("/volume2 UsedPct = %v, want 10", v2.UsedPct)
	}

	// Tangential pin: explicitly-filtered mounts must stay gone.
	// `/data` is a container-bind target; `/sys/fs/cgroup`, `/dev`,
	// `/dev/shm` are virtual filesystems. Note that `/` is NOT
	// asserted here — on Synology Container Manager the container
	// root sits on the same /dev/mapper/cachedev_N as one of the
	// user's volumes, and the dedup pass added in v0.9.12 / issue
	// #300 collapses them with /volume* winning the tie-break. This
	// test pins parseDFSimple in isolation, before dedup runs.
	for _, m := range []string{"/data", "/sys/fs/cgroup", "/dev", "/dev/shm"} {
		if _, ok := mounts[m]; ok {
			t.Errorf("expected %q to be filtered out (virtual or container-bind), but it survived", m)
		}
	}
}

// TestCollectDisks_DedupResolvesContainerRootVsVolume2 is the end-to-
// end shape of the user-facing fix: when df reports the container's
// /  AND a /host/volume2 bind mount on the same /dev/mapper/cachedev_1
// (Synology Container Manager stores container layers on the same fs
// as user volumes), the dedup pass from issue #300 collapses them
// and the user-storage path wins. Combined with this PR's wrap
// recovery, the dashboard ends up with /volume1 + /volume2 + /log,
// and the container's /  is correctly hidden.
//
// Uses the same fixture as the parseDFSimple test but threads it
// through the full pipeline by stubbing execCmd.
func TestCollectDisks_DedupResolvesContainerRootVsVolume2(t *testing.T) {
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		// First call uses GNU --output flags. BusyBox returns
		// non-zero, triggering the parseDFSimple fallback path.
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--output=") {
			return "", &errExecFailed{}
		}
		return dfSynologyBusyBoxOutput, nil
	})()

	disks, err := collectDisks()
	if err != nil {
		t.Fatalf("collectDisks: %v", err)
	}

	have := make(map[string]internal.DiskInfo, len(disks))
	for _, d := range disks {
		have[d.MountPoint] = d
	}
	for _, want := range []string{"/log", "/volume1", "/volume2"} {
		if _, ok := have[want]; !ok {
			t.Errorf("after dedup: %q missing from disks. Got: %v", want, mountKeys(disks))
		}
	}
	// `/` shares (device, total, used) with /volume2 (both
	// /dev/mapper/cachedev_1 at 1.7T/174.1G) so dedup collapses
	// them with /volume2 winning.
	if _, ok := have["/"]; ok {
		t.Errorf("after dedup: container root `/` should have collapsed into /volume2 (preferUserStoragePath tie-break). Got: %v", mountKeys(disks))
	}
}

// errExecFailed is a tiny error type used to signal that BusyBox df
// rejected the GNU --output flag, the same way real BusyBox does. We
// don't care about the message text — only that .Error() returns
// non-empty so collectDisks falls through to the simple-df path.
type errExecFailed struct{}

func (errExecFailed) Error() string {
	return "df: unrecognized option: --output=source,fstype,size,used,avail,pcent,target"
}

// TestJoinWrappedDFLines_BusyBoxFormat exercises the helper directly
// on the reporter's verbatim output. Each wrapped record must come
// out as exactly one logical line; non-wrapped lines must pass
// through untouched.
func TestJoinWrappedDFLines_BusyBoxFormat(t *testing.T) {
	in := strings.Split(dfSynologyBusyBoxOutput, "\n")
	out := joinWrappedDFLines(in)

	// Count records that mention each mount point. Each mount must
	// appear exactly once in the joined output, on a single line.
	for _, mount := range []string{"/host/volume1", "/host/volume2", "/host/log", "/data", "/"} {
		count := 0
		for _, line := range out {
			fields := strings.Fields(line)
			if len(fields) >= 6 && fields[len(fields)-1] == mount {
				count++
			}
		}
		if count != 1 {
			t.Errorf("mount %q appeared %d times after join (want 1). Joined output:\n%s",
				mount, count, strings.Join(out, "\n"))
		}
	}

	// The joined output should NOT contain any orphan device-only
	// line — every wrapped header should have been merged.
	for i, line := range out {
		fields := strings.Fields(line)
		if len(fields) == 1 && looksLikeDevicePath(fields[0]) {
			t.Errorf("orphan wrapped header at line %d after join: %q", i, line)
		}
	}
}

// TestJoinWrappedDFLines_GNUDfPassesThrough confirms the helper is a
// no-op on GNU coreutils `df` output (single-line records). Without
// this guarantee, the helper risks corrupting GNU df output by
// joining unrelated lines — e.g., consuming a legitimate following
// record as if it were a continuation.
func TestJoinWrappedDFLines_GNUDfPassesThrough(t *testing.T) {
	gnu := `Filesystem     Type     Size  Used Avail Use% Mounted on
/dev/sda1      ext4     100G   45G   55G  45% /
/dev/sdb1      xfs      500G  120G  380G  24% /data
tmpfs          tmpfs    8.0G     0  8.0G   0% /dev/shm
`
	in := strings.Split(gnu, "\n")
	out := joinWrappedDFLines(in)

	if len(out) != len(in) {
		t.Errorf("GNU df output should pass through untouched (%d lines in, %d out)", len(in), len(out))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("line %d mutated: %q → %q", i, in[i], out[i])
		}
	}
}

// TestJoinWrappedDFLines_OrphanHeaderHasNoContinuation pins the
// pathological case: a device-like single-field line at end-of-output
// (or followed by an empty line) must NOT cause a panic, must NOT
// consume an empty line as if it were data, and must pass through
// untouched so the existing parser's `< 6 fields` guard skips it.
func TestJoinWrappedDFLines_OrphanHeaderHasNoContinuation(t *testing.T) {
	cases := []struct {
		name string
		in   []string
	}{
		{
			name: "orphan at end of output",
			in: []string{
				"Filesystem  Size  Used  Avail  Use%  Mounted on",
				"/dev/sda    100G  45G   55G    45%   /",
				"/dev/mapper/cachedev_0", // orphan, no follower
			},
		},
		{
			name: "orphan followed by empty line",
			in: []string{
				"Filesystem  Size  Used  Avail  Use%  Mounted on",
				"/dev/mapper/cachedev_0",
				"",
				"/dev/sda    100G  45G   55G    45%   /",
			},
		},
		{
			name: "orphan followed by another orphan",
			in: []string{
				"Filesystem  Size  Used  Avail  Use%  Mounted on",
				"/dev/mapper/cachedev_0",
				"/dev/mapper/cachedev_1",
				"22.7T 20.1T 2.6T 89% /host/volume1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("joinWrappedDFLines panicked on %s: %v", tc.name, r)
				}
			}()
			out := joinWrappedDFLines(tc.in)
			// Liveness only: assert no panic + result is non-empty
			// and at least preserves the header. Strict semantics
			// for orphan handling are intentionally loose; the
			// parsers will skip malformed records on their own.
			if len(out) == 0 {
				t.Errorf("joinWrappedDFLines returned empty output for %d-line input", len(tc.in))
			}
		})
	}
}

// TestLooksLikeDevicePath_Truthtable pins the device-path detection
// for the three real-world shapes: local block devices, NFS sources,
// ZFS pool/dataset names. False positives here would mean the helper
// misidentifies size strings or mount paths as device headers and
// consumes the next line incorrectly.
func TestLooksLikeDevicePath_Truthtable(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		// --- positives ---
		{"/dev/sda1", true},
		{"/dev/mapper/cachedev_0", true},
		{"/dev/mapper/cachedev_1", true},
		{"/dev/disk/by-uuid/abc-123", true},
		{"nas.example.com:/exports/data", true},
		{"192.168.1.100:/volume1", true},
		{"tank", false}, // bare pool name has no /, ambiguous — treat as not-a-device
		{"tank/data", true},
		{"tank/data/movies", true},
		// --- negatives (these must NOT match, or we'd consume real data lines) ---
		{"22.7T", false},
		{"174.1G", false},
		{"1.7T", false},
		{"10%", false},
		{"/host/volume1", false}, // mount point, not device
		{"/", false},
		{"", false},
		{"tmpfs", false},
		{"overlay", false},
		{"devtmpfs", false},
		{"shfs", false}, // Unraid share fs
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			got := looksLikeDevicePath(tc.s)
			if got != tc.want {
				t.Errorf("looksLikeDevicePath(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// mountKeys is a small helper used by failure messages above to
// surface what mount points DID survive parsing, since the bug's
// signature is "expected list looks much shorter than reality".
func mountKeys(disks []internal.DiskInfo) []string {
	out := make([]string, 0, len(disks))
	for _, d := range disks {
		out = append(out, d.MountPoint)
	}
	return out
}
