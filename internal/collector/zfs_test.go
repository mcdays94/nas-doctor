package collector

import (
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// ── zpool list -Hp parsing ──

const sampleZPoolList = `tank	21474836480	16106127360	5368709120	42	75	ONLINE
rpool	10737418240	4294967296	6442450944	12	40	ONLINE
backup	5368709120	3221225472	2147483648	8	60	DEGRADED
`

func TestParseZPoolList(t *testing.T) {
	pools := parseZPoolList(sampleZPoolList)
	if len(pools) != 3 {
		t.Fatalf("expected 3 pools, got %d", len(pools))
	}
	if pools[0].Name != "tank" {
		t.Errorf("pool[0] name: got %q, want %q", pools[0].Name, "tank")
	}
	if pools[0].State != "ONLINE" {
		t.Errorf("pool[0] state: got %q, want %q", pools[0].State, "ONLINE")
	}
	if pools[0].UsedPct != 75 {
		t.Errorf("pool[0] used%%: got %.0f, want 75", pools[0].UsedPct)
	}
	if pools[0].Fragmentation != 42 {
		t.Errorf("pool[0] frag: got %d, want 42", pools[0].Fragmentation)
	}
	if pools[2].State != "DEGRADED" {
		t.Errorf("pool[2] state: got %q, want %q", pools[2].State, "DEGRADED")
	}
}

// ── zpool status — healthy pool with mirrors ──

const sampleStatusHealthy = `  pool: tank
 state: ONLINE
  scan: scrub repaired 0B in 12:34:56 with 0 errors on Sun Apr  6 02:00:00 2026
config:

	NAME                                      STATE     READ WRITE CKSUM
	tank                                      ONLINE       0     0     0
	  mirror-0                                ONLINE       0     0     0
	    /dev/disk/by-id/scsi-SATA_WDC_1234    ONLINE       0     0     0
	    /dev/disk/by-id/scsi-SATA_WDC_5678    ONLINE       0     0     0
	  mirror-1                                ONLINE       0     0     0
	    /dev/disk/by-id/scsi-SATA_SEA_AAAA    ONLINE       0     0     0
	    /dev/disk/by-id/scsi-SATA_SEA_BBBB    ONLINE       0     0     0

errors: No known data errors
`

func TestParsePoolStatusHealthy(t *testing.T) {
	mockPools := parseZPoolList("tank\t21474836480\t16106127360\t5368709120\t42\t75\tONLINE\n")
	enrichPoolsFromStatus(mockPools, sampleStatusHealthy)

	if len(mockPools) == 0 {
		t.Fatal("no pools parsed")
	}
	p := mockPools[0]
	if p.State != "ONLINE" {
		t.Errorf("state: got %q, want ONLINE", p.State)
	}
	if p.ScanType != "scrub" {
		t.Errorf("scan type: got %q, want scrub", p.ScanType)
	}
	if p.ScanErrors != 0 {
		t.Errorf("scan errors: got %d, want 0", p.ScanErrors)
	}
	if p.Errors.Data != "No known data errors" {
		t.Errorf("errors: got %q", p.Errors.Data)
	}
	if len(p.VDevs) < 2 {
		t.Fatalf("expected >=2 vdevs, got %d", len(p.VDevs))
	}
	if p.VDevs[0].Type != "mirror" {
		t.Errorf("vdev[0] type: got %q, want mirror", p.VDevs[0].Type)
	}
	if len(p.VDevs[0].Children) != 2 {
		t.Errorf("vdev[0] children: got %d, want 2", len(p.VDevs[0].Children))
	}
}

// ── zpool status — degraded pool with raidz ──

const sampleStatusDegraded = `  pool: backup
 state: DEGRADED
status: One or more devices has been removed by the administrator.
	Sufficient replicas exist for the pool to continue functioning in a
	degraded state.
action: Online the device using 'zpool online' or replace the device with
	'zpool replace'.
  scan: scrub repaired 0B in 08:12:33 with 0 errors on Sat Apr  5 04:00:00 2026
config:

	NAME                        STATE     READ WRITE CKSUM
	backup                      DEGRADED     0     0     0
	  raidz1-0                  DEGRADED     0     0     0
	    /dev/sda                ONLINE       0     0     0
	    /dev/sdb                REMOVED      0     0     0
	    /dev/sdc                ONLINE       0     0     0

errors: No known data errors
`

func TestParsePoolStatusDegraded(t *testing.T) {
	mockPools := parseZPoolList("backup\t5368709120\t3221225472\t2147483648\t8\t60\tDEGRADED\n")
	enrichPoolsFromStatus(mockPools, sampleStatusDegraded)

	p := mockPools[0]
	if p.State != "DEGRADED" {
		t.Errorf("state: got %q, want DEGRADED", p.State)
	}
	if p.Status == "" {
		t.Error("expected non-empty status message")
	}
	if p.Action == "" {
		t.Error("expected non-empty action message")
	}
	if len(p.VDevs) < 1 {
		t.Fatal("expected >=1 vdev")
	}
	if p.VDevs[0].Type != "raidz1" {
		t.Errorf("vdev type: got %q, want raidz1", p.VDevs[0].Type)
	}
	found := false
	for _, child := range p.VDevs[0].Children {
		if child.State == "REMOVED" {
			found = true
		}
	}
	if !found {
		t.Error("expected a REMOVED child in raidz1-0")
	}
}

// ── zpool status — resilver in progress ──

const sampleStatusResilver = `  pool: tank
 state: DEGRADED
status: One or more devices is currently being resilvered.
action: Wait for the resilver to complete.
  scan: resilver in progress since Sun Apr  6 10:00:00 2026
    1.23T scanned at 456M/s, 800G issued at 300M/s, 2.00T total
    45.2% done, 0 days 03:12:45 to go
config:

	NAME                        STATE     READ WRITE CKSUM
	tank                        DEGRADED     0     0     0
	  mirror-0                  DEGRADED     0     0     0
	    /dev/sda                ONLINE       0     0     0
	    /dev/sdb                ONLINE       0     0     0

errors: No known data errors
`

func TestParsePoolStatusResilver(t *testing.T) {
	mockPools := parseZPoolList("tank\t21474836480\t16106127360\t5368709120\t42\t75\tDEGRADED\n")
	enrichPoolsFromStatus(mockPools, sampleStatusResilver)

	p := mockPools[0]
	if p.ScanType != "resilver" {
		t.Errorf("scan type: got %q, want resilver", p.ScanType)
	}
	if p.ScanPct < 40 || p.ScanPct > 50 {
		t.Errorf("scan pct: got %.1f, want ~45.2", p.ScanPct)
	}
}

// ── zpool status — scrub with errors ──

const sampleStatusErrors = `  pool: data
 state: ONLINE
status: One or more devices has experienced an unrecoverable error.
  scan: scrub repaired 4K in 06:22:11 with 3 errors on Fri Apr  4 02:00:00 2026
config:

	NAME                        STATE     READ WRITE CKSUM
	data                        ONLINE       0     0     0
	  /dev/sda                  ONLINE       2     0     1

errors: 3 data errors, use '-v' for a list
`

func TestParsePoolStatusWithErrors(t *testing.T) {
	mockPools := parseZPoolList("data\t10737418240\t8589934592\t2147483648\t15\t80\tONLINE\n")
	enrichPoolsFromStatus(mockPools, sampleStatusErrors)

	p := mockPools[0]
	if p.ScanErrors != 3 {
		t.Errorf("scan errors: got %d, want 3", p.ScanErrors)
	}
	if p.Errors.Data == "No known data errors" {
		t.Errorf("expected error description, got %q", p.Errors.Data)
	}
	if len(p.VDevs) > 0 {
		v := p.VDevs[0]
		if v.ReadErr != 2 {
			t.Errorf("vdev read errors: got %d, want 2", v.ReadErr)
		}
		if v.CksumErr != 1 {
			t.Errorf("vdev cksum errors: got %d, want 1", v.CksumErr)
		}
	}
}

// ── zfs list parsing ──

const sampleZFSList = `tank	8589934592	5368709120	262144	/tank	lz4	1.52x	filesystem
tank/data	7516192768	5368709120	7516192768	/tank/data	lz4	1.52x	filesystem
tank/docker	536870912	5368709120	536870912	/tank/docker	lz4	2.10x	filesystem
tank/vms	536870912	5368709120	536870912	/tank/vms	off	1.00x	filesystem
`

func TestParseZFSList(t *testing.T) {
	datasets := parseZFSList(sampleZFSList)
	if len(datasets) != 4 {
		t.Fatalf("expected 4 datasets, got %d", len(datasets))
	}
	if datasets[0].Pool != "tank" {
		t.Errorf("ds[0] pool: got %q, want tank", datasets[0].Pool)
	}
	if datasets[1].Compression != "lz4" {
		t.Errorf("ds[1] compression: got %q, want lz4", datasets[1].Compression)
	}
	if datasets[2].CompRatio != 2.10 {
		t.Errorf("ds[2] comp ratio: got %.2f, want 2.10", datasets[2].CompRatio)
	}
	if datasets[3].Type != "filesystem" {
		t.Errorf("ds[3] type: got %q, want filesystem", datasets[3].Type)
	}
}

// ── vdev classification ──

func TestClassifyVDev(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"mirror-0", "mirror"},
		{"raidz1-0", "raidz1"},
		{"raidz2-1", "raidz2"},
		{"raidz3-0", "raidz3"},
		{"/dev/sda", "disk"},
		{"sda", "disk"},
		{"nvme0n1", "disk"},
		{"cache", "cache"},
		{"logs", "log"},
		{"spares", "spare"},
		{"special", "special"},
	}
	for _, tt := range tests {
		got := classifyVDev(tt.name)
		if got != tt.expected {
			t.Errorf("classifyVDev(%q): got %q, want %q", tt.name, got, tt.expected)
		}
	}
}

// Suppress unused import warning
var _ = internal.ZFSInfo{}
