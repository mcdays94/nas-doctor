// Package demo provides realistic mock diagnostic data for local preview.
package demo

import (
	"math/rand"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// GenerateSnapshot returns a realistic fake snapshot resembling a
// mid-range Unraid server with a handful of issues.
func GenerateSnapshot() *internal.Snapshot {
	now := time.Now()
	snap := &internal.Snapshot{
		ID:        "demo-" + now.Format("20060102-150405"),
		Timestamp: now,
		Duration:  4.7,
	}

	snap.System = demoSystem()
	snap.Disks = demoDisks()
	snap.SMART = demoSMART()
	snap.Docker = demoDocker()
	snap.Network = demoNetwork()
	snap.Logs = demoLogs()
	snap.Parity = demoParity()
	snap.ZFS = demoZFS()
	snap.UPS = demoUPS()

	return snap
}

func demoSystem() internal.SystemInfo {
	return internal.SystemInfo{
		Hostname:    "Tower",
		OS:          "unraid 7.1.4 (kernel 6.12.10-Unraid)",
		Kernel:      "6.12.10-Unraid",
		Platform:    "unraid",
		PlatformVer: "7.1.4",
		CPUModel:    "Intel Core i7-10700 @ 2.90GHz",
		CPUCores:    16,
		CPUUsage:    23.4,
		LoadAvg1:    2.34,
		LoadAvg5:    1.87,
		LoadAvg15:   1.52,
		MemTotalMB:  32768,
		MemUsedMB:   24576,
		MemPercent:  75.0,
		SwapTotalMB: 4096,
		SwapUsedMB:  128,
		IOWait:      18.3,
		UptimeSecs:  2592000, // 30 days
		Motherboard: "ASRock Z490M-ITX/ac",
		TopProcesses: []internal.ProcessInfo{
			{PID: 1842, User: "root", CPU: 12.3, Mem: 4.1, Command: "/usr/bin/plex-media-server"},
			{PID: 2901, User: "root", CPU: 8.7, Mem: 6.2, Command: "emby-server --ffmpeg /usr/bin/ffmpeg"},
			{PID: 3156, User: "nobody", CPU: 5.4, Mem: 2.8, Command: "python3 /app/tdarr/Tdarr_Node"},
			{PID: 4201, User: "root", CPU: 3.2, Mem: 1.5, Command: "/usr/sbin/shfs"},
			{PID: 5678, User: "root", CPU: 2.1, Mem: 3.4, Command: "mongod --config /etc/mongod.conf"},
			{PID: 6789, User: "root", CPU: 1.8, Mem: 0.9, Command: "nginx: master process"},
			{PID: 7890, User: "nobody", CPU: 1.2, Mem: 0.4, Command: "/usr/bin/wireguard-go wg0"},
			{PID: 8901, User: "root", CPU: 0.8, Mem: 2.1, Command: "postgres: writer process"},
			{PID: 9012, User: "root", CPU: 0.5, Mem: 0.3, Command: "/usr/sbin/sshd -D"},
			{PID: 1234, User: "root", CPU: 0.3, Mem: 0.2, Command: "/sbin/agetty --noclear tty1 linux"},
		},
	}
}

func demoDisks() []internal.DiskInfo {
	return []internal.DiskInfo{
		{Device: "/dev/md1", MountPoint: "/mnt/disk1", Label: "Disk 1", FSType: "xfs", TotalGB: 14000, UsedGB: 12180, FreeGB: 1820, UsedPct: 87.0},
		{Device: "/dev/md2", MountPoint: "/mnt/disk2", Label: "Disk 2", FSType: "xfs", TotalGB: 14000, UsedGB: 11340, FreeGB: 2660, UsedPct: 81.0},
		{Device: "/dev/md3", MountPoint: "/mnt/disk3", Label: "Disk 3", FSType: "xfs", TotalGB: 8000, UsedGB: 7680, FreeGB: 320, UsedPct: 96.0},
		{Device: "/dev/md4", MountPoint: "/mnt/disk4", Label: "Disk 4", FSType: "xfs", TotalGB: 8000, UsedGB: 5600, FreeGB: 2400, UsedPct: 70.0},
		{Device: "/dev/md5", MountPoint: "/mnt/disk5", Label: "Disk 5", FSType: "xfs", TotalGB: 4000, UsedGB: 3920, FreeGB: 80, UsedPct: 98.0},
		{Device: "/dev/nvme0n1p1", MountPoint: "/mnt/cache", Label: "Cache (NVMe)", FSType: "btrfs", TotalGB: 1000, UsedGB: 680, FreeGB: 320, UsedPct: 68.0},
		{Device: "/dev/sda1", MountPoint: "/boot", Label: "Flash", FSType: "vfat", TotalGB: 32, UsedGB: 2, FreeGB: 30, UsedPct: 6.0},
	}
}

func demoSMART() []internal.SMARTInfo {
	return []internal.SMARTInfo{
		{
			Device: "/dev/sdb", Model: "WDC WD140EDGZ-11B1PA0", Serial: "9LHWA2JC",
			SizeGB: 14000, HealthPassed: true, PowerOnHours: 38420, Temperature: 36, TempMax: 44,
			Reallocated: 0, Pending: 0, Offline: 0, UDMACRC: 0, CommandTimeout: 0,
			DiskType: "hdd", ATAPort: "ata1", ArraySlot: "parity",
		},
		{
			Device: "/dev/sdc", Model: "WDC WD140EDGZ-11B1PA0", Serial: "2CGHV7BD",
			SizeGB: 14000, HealthPassed: true, PowerOnHours: 42150, Temperature: 38, TempMax: 46,
			Reallocated: 0, Pending: 0, Offline: 0, UDMACRC: 47, CommandTimeout: 3,
			DiskType: "hdd", ATAPort: "ata2", ArraySlot: "disk1",
		},
		{
			Device: "/dev/sdd", Model: "Seagate ST14000NM001G-2KJ", Serial: "ZL20BQNT",
			SizeGB: 14000, HealthPassed: true, PowerOnHours: 35800, Temperature: 37, TempMax: 43,
			Reallocated: 0, Pending: 0, Offline: 0, UDMACRC: 0, CommandTimeout: 0,
			DiskType: "hdd", ATAPort: "ata3", ArraySlot: "disk2",
		},
		{
			Device: "/dev/sde", Model: "Seagate ST8000VN004-2M2101", Serial: "WSD0K1PV",
			SizeGB: 8000, HealthPassed: true, PowerOnHours: 51200, Temperature: 42, TempMax: 52,
			Reallocated: 8, Pending: 2, Offline: 0, UDMACRC: 0, CommandTimeout: 12,
			DiskType: "hdd", ATAPort: "ata4", ArraySlot: "disk3",
		},
		{
			Device: "/dev/sdf", Model: "Seagate ST8000VN004-2M2101", Serial: "WSD0K3TH",
			SizeGB: 8000, HealthPassed: true, PowerOnHours: 44800, Temperature: 39, TempMax: 48,
			Reallocated: 0, Pending: 0, Offline: 0, UDMACRC: 0, CommandTimeout: 0,
			DiskType: "hdd", ATAPort: "ata5", ArraySlot: "disk4",
		},
		{
			Device: "/dev/sdg", Model: "WDC WD40EFAX-68JH4N1", Serial: "WD-WX22D31N6TDV",
			SizeGB: 4000, HealthPassed: false, PowerOnHours: 62400, Temperature: 47, TempMax: 69,
			Reallocated: 184, Pending: 24, Offline: 16, UDMACRC: 0, CommandTimeout: 87,
			DiskType: "hdd", ATAPort: "ata6", ArraySlot: "disk5",
		},
		{
			Device: "/dev/nvme0n1", Model: "Samsung 970 EVO Plus 1TB", Serial: "S4EWNF0MC01234",
			SizeGB: 1000, HealthPassed: true, PowerOnHours: 18200, Temperature: 41, TempMax: 55,
			Reallocated: 0, Pending: 0, Offline: 0, UDMACRC: 0, CommandTimeout: 0,
			DiskType: "nvme", ATAPort: "", ArraySlot: "cache",
		},
	}
}

func demoDocker() internal.DockerInfo {
	return internal.DockerInfo{
		Available: true,
		Containers: []internal.ContainerInfo{
			{ID: "a1b2c3d4", Name: "plex", Image: "linuxserver/plex:latest", Status: "Up 30 days", State: "running", CPU: 12.3, MemMB: 1340, MemPct: 4.1, Uptime: "30 days"},
			{ID: "e5f6g7h8", Name: "emby", Image: "emby/embyserver:latest", Status: "Up 30 days", State: "running", CPU: 8.7, MemMB: 2028, MemPct: 6.2, Uptime: "30 days"},
			{ID: "i9j0k1l2", Name: "tdarr", Image: "haveagitgat/tdarr:latest", Status: "Up 30 days", State: "running", CPU: 5.4, MemMB: 916, MemPct: 2.8, Uptime: "30 days"},
			{ID: "m3n4o5p6", Name: "nginx-proxy", Image: "nginx:alpine", Status: "Up 30 days", State: "running", CPU: 0.3, MemMB: 42, MemPct: 0.1, Uptime: "30 days"},
			{ID: "q7r8s9t0", Name: "wireguard", Image: "linuxserver/wireguard:latest", Status: "Up 30 days", State: "running", CPU: 0.1, MemMB: 28, MemPct: 0.1, Uptime: "30 days"},
			{ID: "u1v2w3x4", Name: "home-assistant", Image: "homeassistant/home-assistant:latest", Status: "Up 15 days", State: "running", CPU: 3.2, MemMB: 512, MemPct: 1.6, Uptime: "15 days"},
			{ID: "y5z6a7b8", Name: "grafana", Image: "grafana/grafana:latest", Status: "Up 30 days", State: "running", CPU: 1.1, MemMB: 186, MemPct: 0.6, Uptime: "30 days"},
			{ID: "c9d0e1f2", Name: "prometheus", Image: "prom/prometheus:latest", Status: "Up 30 days", State: "running", CPU: 0.8, MemMB: 256, MemPct: 0.8, Uptime: "30 days"},
			{ID: "g3h4i5j6", Name: "radarr", Image: "linuxserver/radarr:latest", Status: "Up 30 days", State: "running", CPU: 0.4, MemMB: 312, MemPct: 1.0, Uptime: "30 days"},
			{ID: "k7l8m9n0", Name: "sonarr", Image: "linuxserver/sonarr:latest", Status: "Up 30 days", State: "running", CPU: 0.3, MemMB: 298, MemPct: 0.9, Uptime: "30 days"},
			{ID: "o1p2q3r4", Name: "overseerr", Image: "linuxserver/overseerr:latest", Status: "Up 30 days", State: "running", CPU: 0.2, MemMB: 148, MemPct: 0.5, Uptime: "30 days"},
			{ID: "s5t6u7v8", Name: "sabnzbd", Image: "linuxserver/sabnzbd:latest", Status: "Up 30 days", State: "running", CPU: 0.1, MemMB: 95, MemPct: 0.3, Uptime: "30 days"},
			{ID: "w9x0y1z2", Name: "pihole", Image: "pihole/pihole:latest", Status: "Exited (1) 3 days ago", State: "exited", CPU: 0, MemMB: 0, MemPct: 0, Uptime: "Exited"},
			{ID: "a3b4c5d6", Name: "mariadb-old", Image: "mariadb:10.5", Status: "Exited (0) 14 days ago", State: "exited", CPU: 0, MemMB: 0, MemPct: 0, Uptime: "Exited"},
		},
	}
}

func demoNetwork() internal.NetworkInfo {
	return internal.NetworkInfo{
		Interfaces: []internal.NetInterface{
			{Name: "eth0", Speed: "1000Mb/s", State: "UP", MTU: 1500, IPv4: "192.168.1.38/24"},
			{Name: "bond0", Speed: "2000Mb/s", State: "UP", MTU: 1500, IPv4: "192.168.1.38/24"},
			{Name: "wg0", Speed: "", State: "UP", MTU: 1420, IPv4: "10.13.13.1/24"},
		},
	}
}

func demoLogs() internal.LogInfo {
	return internal.LogInfo{
		DmesgErrors: []internal.LogEntry{
			{Timestamp: "2026-04-05T23:14:22", Level: "error", Message: "ata6.00: failed command: READ FPDMA QUEUED", Source: "dmesg"},
			{Timestamp: "2026-04-05T23:14:22", Level: "error", Message: "ata6.00: status: { DRDY ERR }", Source: "dmesg"},
			{Timestamp: "2026-04-05T23:14:22", Level: "error", Message: "ata6.00: error: { UNC }", Source: "dmesg"},
			{Timestamp: "2026-04-05T22:47:11", Level: "error", Message: "ata6: SError: { CommWake }", Source: "dmesg"},
			{Timestamp: "2026-04-05T22:47:11", Level: "error", Message: "ata6: hard resetting link", Source: "dmesg"},
			{Timestamp: "2026-04-05T20:33:05", Level: "error", Message: "sd 5:0:0:0: [sdg] tag#12 FAILED Result: hostbyte=DID_OK driverbyte=DRIVER_SENSE", Source: "dmesg"},
			{Timestamp: "2026-04-05T20:33:05", Level: "error", Message: "sd 5:0:0:0: [sdg] tag#12 Sense Key : Medium Error [current]", Source: "dmesg"},
			{Timestamp: "2026-04-05T20:33:05", Level: "error", Message: "sd 5:0:0:0: [sdg] tag#12 Add. Sense: Unrecovered read error", Source: "dmesg"},
			{Timestamp: "2026-04-05T18:21:44", Level: "error", Message: "ata2.00: exception Emask 0x10 SAct 0x20400000 SErr 0x400100 action 0x6 frozen", Source: "dmesg"},
			{Timestamp: "2026-04-05T18:21:44", Level: "error", Message: "ata2: SError: { UnrecovData Handshk }", Source: "dmesg"},
			{Timestamp: "2026-04-05T18:21:44", Level: "error", Message: "ata2.00: failed command: WRITE FPDMA QUEUED", Source: "dmesg"},
			{Timestamp: "2026-04-05T14:02:18", Level: "warning", Message: "ata2: limiting SATA link speed to 3.0 Gbps", Source: "dmesg"},
			{Timestamp: "2026-04-04T09:15:32", Level: "error", Message: "BTRFS warning (device nvme0n1p1): csum failed root 5 ino 14582 off 0 csum 0x97e4c59f expected 0xfa64b4c2 mirror 1", Source: "dmesg"},
		},
		SyslogErrors: []internal.LogEntry{
			{Timestamp: "2026-04-05T23:15:00", Level: "error", Message: "emhttpd: error: mdcmd, Input/output error (5): write", Source: "syslog"},
			{Timestamp: "2026-04-05T22:48:00", Level: "warning", Message: "kernel: sd 5:0:0:0: [sdg] Synchronize Cache(10) failed: Result: hostbyte=DID_BAD_TARGET", Source: "syslog"},
			{Timestamp: "2026-04-05T20:34:00", Level: "error", Message: "emhttpd: read error on /dev/sdg sector 7831236: Input/output error", Source: "syslog"},
		},
	}
}

func demoParity() *internal.ParityInfo {
	return &internal.ParityInfo{
		Status: "idle",
		History: []internal.ParityCheck{
			{Date: "2024-01-15", Duration: 54000, SpeedMBs: 142.5, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2024-04-12", Duration: 57600, SpeedMBs: 134.2, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2024-07-20", Duration: 63000, SpeedMBs: 121.8, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2024-10-05", Duration: 72000, SpeedMBs: 106.3, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2025-01-11", Duration: 86400, SpeedMBs: 88.7, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2025-04-19", Duration: 97200, SpeedMBs: 78.4, Errors: 2, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2025-07-26", Duration: 108000, SpeedMBs: 71.1, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2025-10-18", Duration: 126000, SpeedMBs: 60.2, Errors: 0, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2026-01-22", Duration: 151200, SpeedMBs: 50.5, Errors: 5, ExitCode: 0, Action: "check", SizeGB: 28000},
			{Date: "2026-03-30", Duration: 172800, SpeedMBs: 44.1, Errors: 12, ExitCode: 0, Action: "check", SizeGB: 28000},
		},
	}
}

func demoUPS() *internal.UPSInfo {
	return &internal.UPSInfo{
		Available:    true,
		Source:       "apcupsd",
		Name:         "ServerUPS",
		Model:        "APC Back-UPS XS 1400U",
		Status:       "ONLINE",
		StatusHuman:  "Online",
		BatteryPct:   100,
		BatteryV:     27.2,
		InputV:       122,
		OutputV:      122,
		LoadPct:      34,
		RuntimeMins:  48,
		WattageW:     294,
		NominalW:     865,
		Temperature:  31,
		OnBattery:    false,
		LowBattery:   false,
		LastTransfer: "Low line voltage",
	}
}

func demoZFS() *internal.ZFSInfo {
	return &internal.ZFSInfo{
		Available: true,
		Pools: []internal.ZPool{
			{
				Name: "tank", State: "ONLINE",
				TotalGB: 28000, UsedGB: 21280, FreeGB: 6720, UsedPct: 76,
				Fragmentation: 18,
				ScanType:      "scrub", ScanErrors: 0,
				ScanStatus: "scrub repaired 0B in 14:22:08 with 0 errors on Sun Apr 6 02:00:00 2026",
				ScanDate:   "Sun Apr 6 02:00:00 2026",
				Errors:     internal.ZPoolErrors{Data: "No known data errors"},
				VDevs: []internal.ZVDev{
					{Name: "mirror-0", Type: "mirror", State: "ONLINE", Children: []internal.ZVDev{
						{Name: "/dev/sdb", Type: "disk", State: "ONLINE"},
						{Name: "/dev/sdc", Type: "disk", State: "ONLINE"},
					}},
					{Name: "mirror-1", Type: "mirror", State: "ONLINE", Children: []internal.ZVDev{
						{Name: "/dev/sdd", Type: "disk", State: "ONLINE"},
						{Name: "/dev/sde", Type: "disk", State: "ONLINE"},
					}},
				},
			},
			{
				Name: "backup", State: "DEGRADED",
				Status:  "One or more devices has been removed by the administrator.",
				Action:  "Online the device using 'zpool online' or replace the device with 'zpool replace'.",
				TotalGB: 8000, UsedGB: 5440, FreeGB: 2560, UsedPct: 68,
				Fragmentation: 8,
				ScanType:      "scrub", ScanErrors: 0,
				ScanStatus: "scrub repaired 0B in 06:18:44 with 0 errors on Sat Apr 5 04:00:00 2026",
				ScanDate:   "Sat Apr 5 04:00:00 2026",
				Errors:     internal.ZPoolErrors{Data: "No known data errors"},
				VDevs: []internal.ZVDev{
					{Name: "raidz1-0", Type: "raidz1", State: "DEGRADED", Children: []internal.ZVDev{
						{Name: "/dev/sdf", Type: "disk", State: "ONLINE"},
						{Name: "/dev/sdg", Type: "disk", State: "REMOVED"},
						{Name: "/dev/sdh", Type: "disk", State: "ONLINE"},
					}},
				},
			},
		},
		Datasets: []internal.ZDataset{
			{Name: "tank", Pool: "tank", Type: "filesystem", UsedGB: 21280, AvailGB: 6720, ReferGB: 256, MountPoint: "/tank", Compression: "lz4", CompRatio: 1.42},
			{Name: "tank/data", Pool: "tank", Type: "filesystem", UsedGB: 16800, AvailGB: 6720, ReferGB: 16800, MountPoint: "/tank/data", Compression: "lz4", CompRatio: 1.38},
			{Name: "tank/media", Pool: "tank", Type: "filesystem", UsedGB: 3200, AvailGB: 6720, ReferGB: 3200, MountPoint: "/tank/media", Compression: "lz4", CompRatio: 1.02},
			{Name: "tank/docker", Pool: "tank", Type: "filesystem", UsedGB: 680, AvailGB: 6720, ReferGB: 680, MountPoint: "/tank/docker", Compression: "lz4", CompRatio: 2.15},
			{Name: "tank/vms", Pool: "tank", Type: "filesystem", UsedGB: 600, AvailGB: 6720, ReferGB: 600, MountPoint: "/tank/vms", Compression: "off", CompRatio: 1.0},
			{Name: "backup", Pool: "backup", Type: "filesystem", UsedGB: 5440, AvailGB: 2560, ReferGB: 128, MountPoint: "/backup", Compression: "zstd", CompRatio: 1.85},
			{Name: "backup/snapshots", Pool: "backup", Type: "filesystem", UsedGB: 5312, AvailGB: 2560, ReferGB: 5312, MountPoint: "/backup/snapshots", Compression: "zstd", CompRatio: 1.92},
		},
		ARC: &internal.ZFSARCStats{
			SizeMB:    12288,
			MaxSizeMB: 16384,
			HitRate:   94.2,
			MissRate:  5.8,
			Hits:      847293156,
			Misses:    51482304,
			L2SizeMB:  245760,
			L2HitRate: 78.5,
		},
	}
}

// Jitter adds small random variation to a float to simulate live data.
func Jitter(base float64, pct float64) float64 {
	delta := base * pct / 100
	return base + (rand.Float64()*2-1)*delta
}
