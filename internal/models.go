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
