// Package internal contains shared types used across all nas-doctor packages.
package internal

import "time"

// Severity levels for findings.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
	SeverityOK       Severity = "ok"
)

// Category classifies a diagnostic area.
type Category string

const (
	CategorySystem  Category = "system"
	CategoryDisk    Category = "disk"
	CategorySMART   Category = "smart"
	CategoryDocker  Category = "docker"
	CategoryNetwork Category = "network"
	CategoryMemory  Category = "memory"
	CategoryThermal Category = "thermal"
	CategoryLogs    Category = "logs"
	CategoryParity  Category = "parity"
	CategoryZFS     Category = "zfs"
	CategoryUPS     Category = "ups"
)

// ---------- Snapshot (one complete diagnostic run) ----------

// Snapshot represents a single point-in-time diagnostic collection.
type Snapshot struct {
	ID        string      `json:"id" db:"id"`
	Timestamp time.Time   `json:"timestamp" db:"timestamp"`
	Duration  float64     `json:"duration_seconds" db:"duration_seconds"` // how long the collection took
	System    SystemInfo  `json:"system"`
	Disks     []DiskInfo  `json:"disks"`
	SMART     []SMARTInfo `json:"smart"`
	Docker    DockerInfo  `json:"docker"`
	Network   NetworkInfo `json:"network"`
	Logs      LogInfo     `json:"logs"`
	Parity    *ParityInfo `json:"parity,omitempty"`
	ZFS       *ZFSInfo    `json:"zfs,omitempty"`
	UPS       *UPSInfo    `json:"ups,omitempty"`
	Update    *UpdateInfo `json:"update,omitempty"`
	Findings  []Finding   `json:"findings"`
}

// ---------- System ----------

type SystemInfo struct {
	Hostname     string        `json:"hostname"`
	OS           string        `json:"os"`
	Kernel       string        `json:"kernel"`
	Platform     string        `json:"platform"` // unraid, truenas, synology, linux, etc.
	PlatformVer  string        `json:"platform_version"`
	CPUModel     string        `json:"cpu_model"`
	CPUCores     int           `json:"cpu_cores"`
	CPUUsage     float64       `json:"cpu_usage_percent"`
	LoadAvg1     float64       `json:"load_avg_1"`
	LoadAvg5     float64       `json:"load_avg_5"`
	LoadAvg15    float64       `json:"load_avg_15"`
	MemTotalMB   int64         `json:"mem_total_mb"`
	MemUsedMB    int64         `json:"mem_used_mb"`
	MemPercent   float64       `json:"mem_percent"`
	SwapTotalMB  int64         `json:"swap_total_mb"`
	SwapUsedMB   int64         `json:"swap_used_mb"`
	IOWait       float64       `json:"io_wait_percent"`
	UptimeSecs   int64         `json:"uptime_seconds"`
	Motherboard  string        `json:"motherboard"`
	TopProcesses []ProcessInfo `json:"top_processes"`
}

type ProcessInfo struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu_percent"`
	Mem     float64 `json:"mem_percent"`
	Command string  `json:"command"`
}

// ---------- Disk ----------

type DiskInfo struct {
	Device     string  `json:"device"`      // /dev/sda
	MountPoint string  `json:"mount_point"` // /mnt/disk1
	Label      string  `json:"label"`       // Disk 1, Cache, etc.
	FSType     string  `json:"fs_type"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
	FreeGB     float64 `json:"free_gb"`
	UsedPct    float64 `json:"used_percent"`
}

// ---------- SMART ----------

type SMARTInfo struct {
	Device         string  `json:"device"`
	Model          string  `json:"model"`
	Serial         string  `json:"serial"`
	Firmware       string  `json:"firmware"`
	SizeGB         float64 `json:"size_gb"`
	HealthPassed   bool    `json:"health_passed"`
	PowerOnHours   int64   `json:"power_on_hours"`
	Temperature    int     `json:"temperature_c"`
	TempMax        int     `json:"temperature_max_c"`
	Reallocated    int64   `json:"reallocated_sectors"`
	Pending        int64   `json:"pending_sectors"`
	Offline        int64   `json:"offline_uncorrectable"`
	UDMACRC        int64   `json:"udma_crc_errors"`
	CommandTimeout int64   `json:"command_timeout"`
	SpinRetry      int64   `json:"spin_retry_count"`
	RawReadError   int64   `json:"raw_read_error_rate"`
	SeekError      int64   `json:"seek_error_rate"`
	DiskType       string  `json:"disk_type"`  // hdd, ssd, nvme
	ATAPort        string  `json:"ata_port"`   // ata1, ata2, etc.
	ArraySlot      string  `json:"array_slot"` // disk1, parity, cache, etc.
}

// ---------- Docker ----------

type DockerInfo struct {
	Available  bool            `json:"available"`
	Containers []ContainerInfo `json:"containers"`
}

type ContainerInfo struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Image  string  `json:"image"`
	Status string  `json:"status"` // running, exited, etc.
	State  string  `json:"state"`
	CPU    float64 `json:"cpu_percent"`
	MemMB  float64 `json:"mem_mb"`
	MemPct float64 `json:"mem_percent"`
	Uptime string  `json:"uptime"`
}

// ---------- Network ----------

type NetworkInfo struct {
	Interfaces []NetInterface `json:"interfaces"`
}

type NetInterface struct {
	Name  string `json:"name"`
	Speed string `json:"speed"` // "1000Mb/s", "10000Mb/s"
	State string `json:"state"` // UP, DOWN
	MTU   int    `json:"mtu"`
	IPv4  string `json:"ipv4"`
}

// ---------- Logs ----------

type LogInfo struct {
	DmesgErrors  []LogEntry `json:"dmesg_errors"`
	SyslogErrors []LogEntry `json:"syslog_errors"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"` // error, warning, critical
	Message   string `json:"message"`
	Source    string `json:"source"` // dmesg, syslog, etc.
}

// ---------- Parity (Unraid-specific) ----------

type ParityInfo struct {
	Status       string        `json:"status"` // idle, running, paused
	History      []ParityCheck `json:"history"`
	CurrentSpeed float64       `json:"current_speed_mb_s"`
	Elapsed      string        `json:"elapsed"`
}

type ParityCheck struct {
	Date     string  `json:"date"`
	Duration int64   `json:"duration_seconds"`
	SpeedMBs float64 `json:"speed_mb_s"`
	Errors   int     `json:"errors"`
	ExitCode int     `json:"exit_code"`
	Action   string  `json:"action"` // check, correct, recon
	SizeGB   float64 `json:"size_gb"`
}

// ---------- Fleet / Multi-Server ----------

type RemoteServer struct {
	ID      string `json:"id"`
	Name    string `json:"name"` // friendly name ("Backup NAS", "Proxmox")
	URL     string `json:"url"`  // base URL ("http://192.168.1.50:8080")
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key,omitempty"` // for future auth
}

type RemoteServerStatus struct {
	Server        RemoteServer `json:"server"`
	Online        bool         `json:"online"`
	LastPoll      string       `json:"last_poll"` // ISO timestamp
	Hostname      string       `json:"hostname"`
	Platform      string       `json:"platform"`
	Uptime        string       `json:"uptime"`
	OverallHealth string       `json:"overall_health"` // healthy, warning, critical
	CriticalCount int          `json:"critical_count"`
	WarningCount  int          `json:"warning_count"`
	InfoCount     int          `json:"info_count"`
	Error         string       `json:"error,omitempty"`
}

// ---------- OS Update ----------

type UpdateInfo struct {
	Platform         string `json:"platform"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	ReleaseName      string `json:"release_name,omitempty"`
	ReleaseURL       string `json:"release_url,omitempty"`
	CheckedAt        string `json:"checked_at,omitempty"`
	Error            string `json:"error,omitempty"`
}

// ---------- UPS ----------

type UPSInfo struct {
	Available    bool    `json:"available"`
	Source       string  `json:"source"` // "nut", "apcupsd"
	Name         string  `json:"name"`   // UPS model/name
	Model        string  `json:"model"`
	Status       string  `json:"status"`       // "OL" (online), "OB" (on battery), "LB" (low battery), "OL CHRG", etc.
	StatusHuman  string  `json:"status_human"` // "Online", "On Battery", "Low Battery", etc.
	BatteryPct   float64 `json:"battery_percent"`
	BatteryV     float64 `json:"battery_voltage"`
	InputV       float64 `json:"input_voltage"`
	OutputV      float64 `json:"output_voltage"`
	LoadPct      float64 `json:"load_percent"`
	RuntimeMins  float64 `json:"runtime_minutes"`
	WattageW     float64 `json:"wattage_watts"`
	NominalW     float64 `json:"nominal_watts"`
	Temperature  float64 `json:"temperature_c"`
	OnBattery    bool    `json:"on_battery"`
	LowBattery   bool    `json:"low_battery"`
	LastTransfer string  `json:"last_transfer"` // reason for last transfer to battery
	LastEvent    string  `json:"last_event"`
}

// ---------- ZFS ----------

type ZFSInfo struct {
	Available bool         `json:"available"`
	Pools     []ZPool      `json:"pools"`
	Datasets  []ZDataset   `json:"datasets,omitempty"`
	ARC       *ZFSARCStats `json:"arc,omitempty"`
}

type ZPool struct {
	Name          string      `json:"name"`         // "tank", "rpool"
	State         string      `json:"state"`        // ONLINE, DEGRADED, FAULTED, OFFLINE, REMOVED, UNAVAIL
	Status        string      `json:"status"`       // human-readable status message
	Action        string      `json:"action"`       // recommended action from zpool status
	ScanStatus    string      `json:"scan_status"`  // "scrub repaired 0B ... with 0 errors", "resilver in progress", "none requested"
	ScanType      string      `json:"scan_type"`    // "scrub", "resilver", "none"
	ScanPct       float64     `json:"scan_percent"` // 0-100 if in progress
	ScanErrors    int         `json:"scan_errors"`
	ScanDate      string      `json:"scan_date"` // last scrub/resilver completion date
	TotalGB       float64     `json:"total_gb"`
	UsedGB        float64     `json:"used_gb"`
	FreeGB        float64     `json:"free_gb"`
	UsedPct       float64     `json:"used_percent"`
	Fragmentation int         `json:"fragmentation_percent"`
	VDevs         []ZVDev     `json:"vdevs"`
	Errors        ZPoolErrors `json:"errors"`
}

type ZVDev struct {
	Name     string  `json:"name"`  // "mirror-0", "raidz1-0", "/dev/sda"
	Type     string  `json:"type"`  // "mirror", "raidz1", "raidz2", "raidz3", "disk", "spare", "log", "cache", "special"
	State    string  `json:"state"` // ONLINE, DEGRADED, FAULTED, OFFLINE, REMOVED, UNAVAIL
	Children []ZVDev `json:"children,omitempty"`
	ReadErr  int64   `json:"read_errors"`
	WriteErr int64   `json:"write_errors"`
	CksumErr int64   `json:"checksum_errors"`
}

type ZPoolErrors struct {
	Data     string `json:"data"` // "No known data errors" or description
	Read     int64  `json:"read"`
	Write    int64  `json:"write"`
	Checksum int64  `json:"checksum"`
}

type ZDataset struct {
	Name        string  `json:"name"` // "tank/data", "rpool/ROOT"
	Pool        string  `json:"pool"`
	Type        string  `json:"type"` // "filesystem", "volume", "snapshot"
	UsedGB      float64 `json:"used_gb"`
	AvailGB     float64 `json:"avail_gb"`
	ReferGB     float64 `json:"refer_gb"`
	MountPoint  string  `json:"mount_point"`
	Compression string  `json:"compression"`
	CompRatio   float64 `json:"compression_ratio"`
	Snapshots   int     `json:"snapshot_count"`
}

type ZFSARCStats struct {
	SizeMB    float64 `json:"size_mb"`
	MaxSizeMB float64 `json:"max_size_mb"`
	HitRate   float64 `json:"hit_rate_percent"`
	MissRate  float64 `json:"miss_rate_percent"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	L2SizeMB  float64 `json:"l2_size_mb"`
	L2HitRate float64 `json:"l2_hit_rate_percent"`
}

// ---------- Findings (analysis output) ----------

type Finding struct {
	ID          string   `json:"id"`
	Severity    Severity `json:"severity"`
	Category    Category `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Evidence    []string `json:"evidence"`               // raw log lines, data points
	Impact      string   `json:"impact"`                 // what happens if ignored
	Action      string   `json:"action"`                 // recommended fix
	Priority    string   `json:"priority"`               // immediate, short-term, medium-term
	Cost        string   `json:"cost"`                   // "$10-20", "Free", etc.
	RelatedDisk string   `json:"related_disk,omitempty"` // if disk-specific
}

// ---------- Configuration ----------

type Config struct {
	ListenAddr    string             `json:"listen_addr"`   // :8080
	DataDir       string             `json:"data_dir"`      // /data
	ScheduleCron  string             `json:"schedule_cron"` // "0 */6 * * *"
	HostPaths     HostPaths          `json:"host_paths"`
	Notifications NotificationConfig `json:"notifications"`
	Prometheus    PrometheusConfig   `json:"prometheus"`
}

type HostPaths struct {
	Boot string `json:"boot"` // /host/boot (Unraid config)
	Log  string `json:"log"`  // /host/log (system logs)
	Proc string `json:"proc"` // /proc
	Sys  string `json:"sys"`  // /sys
}

type NotificationConfig struct {
	Webhooks []WebhookConfig `json:"webhooks"`
}

type WebhookConfig struct {
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Type     string            `json:"type"` // discord, slack, gotify, ntfy, generic
	Enabled  bool              `json:"enabled"`
	MinLevel Severity          `json:"min_level"` // minimum severity to notify
	Headers  map[string]string `json:"headers,omitempty"`
	Secret   string            `json:"secret,omitempty"` // for HMAC signing
}

type PrometheusConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"` // /metrics
}
