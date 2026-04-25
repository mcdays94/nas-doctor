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
	CategorySystem    Category = "system"
	CategoryDisk      Category = "disk"
	CategorySMART     Category = "smart"
	CategoryDocker    Category = "docker"
	CategoryNetwork   Category = "network"
	CategoryService   Category = "service"
	CategoryMemory    Category = "memory"
	CategoryThermal   Category = "thermal"
	CategoryLogs      Category = "logs"
	CategoryParity    Category = "parity"
	CategoryZFS       Category = "zfs"
	CategoryUPS       Category = "ups"
	CategoryGPU       Category = "gpu"
	CategoryBackup    Category = "backup"
	CategorySpeedTest Category = "speedtest"
)

// ---------- Snapshot (one complete diagnostic run) ----------

// Snapshot represents a single point-in-time diagnostic collection.
type Snapshot struct {
	ID         string               `json:"id" db:"id"`
	Timestamp  time.Time            `json:"timestamp" db:"timestamp"`
	Duration   float64              `json:"duration_seconds" db:"duration_seconds"` // how long the collection took
	System     SystemInfo           `json:"system"`
	Disks      []DiskInfo           `json:"disks"`
	SMART      []SMARTInfo          `json:"smart"`
	Docker     DockerInfo           `json:"docker"`
	Network    NetworkInfo          `json:"network"`
	Logs       LogInfo              `json:"logs"`
	Parity     *ParityInfo          `json:"parity,omitempty"`
	ZFS        *ZFSInfo             `json:"zfs,omitempty"`
	UPS        *UPSInfo             `json:"ups,omitempty"`
	Update     *UpdateInfo          `json:"update,omitempty"`
	Tunnels    *TunnelInfo          `json:"tunnels,omitempty"`
	Proxmox    *ProxmoxInfo         `json:"proxmox,omitempty"`
	Kubernetes *KubeInfo            `json:"kubernetes,omitempty"`
	GPU        *GPUInfo             `json:"gpu,omitempty"`
	Backup     *BackupInfo          `json:"backup,omitempty"`
	SpeedTest  *SpeedTestInfo       `json:"speed_test,omitempty"`
	Services   []ServiceCheckResult `json:"service_checks,omitempty"`
	Findings   []Finding            `json:"findings"`

	// SMARTStandbyDevices lists devices that were in standby during this
	// scan's SMART collect pass (i.e. `-n standby` skipped them). The
	// scheduler's StaleSMARTChecker (issue #238) uses this list to
	// decide which drives to force-wake when Settings.SMART.MaxAgeDays
	// has been exceeded. Persisted so historical snapshots retain an
	// accurate "which drives were asleep at this point" audit trail.
	SMARTStandbyDevices []string `json:"smart_standby_devices,omitempty"`

	// SubsystemLastRan maps each configurable subsystem name (smart,
	// docker, proxmox, kubernetes, zfs, gpu) to the RFC3339 timestamp
	// of its most recent successful collection. Introduced by issue
	// #260 so dashboard clients can surface "last scanned 4m ago"
	// per subsystem and distinguish stale vs fresh data. Subsystems
	// that have never run since scheduler start are omitted rather
	// than reported as zero-time. Optional field; omitempty so the
	// key disappears when the dispatcher isn't populated (e.g. demo
	// mode with a synthetic snapshot).
	SubsystemLastRan map[string]string `json:"subsystem_last_ran,omitempty"`
}

// ---------- Proxmox VE ----------

type ProxmoxInfo struct {
	Connected   bool             `json:"connected"`
	Error       string           `json:"error,omitempty"`
	Alias       string           `json:"alias,omitempty"` // user-defined display name
	Version     string           `json:"version,omitempty"`
	ClusterName string           `json:"cluster_name,omitempty"`
	Nodes       []ProxmoxNode    `json:"nodes,omitempty"`
	Guests      []ProxmoxGuest   `json:"guests,omitempty"` // VMs + LXC combined
	Storage     []ProxmoxStorage `json:"storage,omitempty"`
	Tasks       []ProxmoxTask    `json:"tasks,omitempty"`
	HAServices  []ProxmoxHA      `json:"ha_services,omitempty"`
}

type ProxmoxNode struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`     // online, offline
	Uptime     int64   `json:"uptime"`     // seconds
	CPUUsage   float64 `json:"cpu_usage"`  // 0.0-1.0
	MemUsed    int64   `json:"mem_used"`   // bytes
	MemTotal   int64   `json:"mem_total"`  // bytes
	DiskUsed   int64   `json:"disk_used"`  // bytes (root fs)
	DiskTotal  int64   `json:"disk_total"` // bytes
	PVEVersion string  `json:"pve_version,omitempty"`
	KernelVer  string  `json:"kernel_version,omitempty"`
	CPUModel   string  `json:"cpu_model,omitempty"`
	CPUCores   int     `json:"cpu_cores,omitempty"`
}

type ProxmoxGuest struct {
	VMID     int     `json:"vmid"`
	Name     string  `json:"name"`
	Node     string  `json:"node"`
	Type     string  `json:"type"`      // qemu, lxc
	Status   string  `json:"status"`    // running, stopped, paused
	Uptime   int64   `json:"uptime"`    // seconds
	CPUUsage float64 `json:"cpu_usage"` // 0.0-1.0
	CPUs     int     `json:"cpus"`
	MemUsed  int64   `json:"mem_used"`  // bytes
	MemMax   int64   `json:"mem_max"`   // bytes
	DiskUsed int64   `json:"disk_used"` // bytes
	DiskMax  int64   `json:"disk_max"`  // bytes
	NetIn    int64   `json:"net_in"`    // bytes
	NetOut   int64   `json:"net_out"`   // bytes
	Tags     string  `json:"tags,omitempty"`
	Template bool    `json:"template,omitempty"`
	HAState  string  `json:"ha_state,omitempty"` // managed, started, error, etc.
}

type ProxmoxStorage struct {
	Storage string  `json:"storage"`
	Node    string  `json:"node"`
	Type    string  `json:"type"`   // dir, lvm, lvmthin, zfspool, nfs, cifs, ceph
	Status  string  `json:"status"` // available, unavailable
	Used    int64   `json:"used"`   // bytes
	Total   int64   `json:"total"`  // bytes
	UsedPct float64 `json:"used_pct"`
	Shared  bool    `json:"shared"`
	Content string  `json:"content"` // images,rootdir,vztmpl,iso,backup
}

type ProxmoxTask struct {
	UPID      string `json:"upid"`
	Node      string `json:"node"`
	Type      string `json:"type"`   // vzdump, qmigrate, vzmigrate, etc.
	Status    string `json:"status"` // OK, Error, running
	User      string `json:"user"`
	StartTime int64  `json:"start_time"` // unix epoch
	EndTime   int64  `json:"end_time"`
	VMID      int    `json:"vmid,omitempty"`
}

type ProxmoxHA struct {
	SID    string `json:"sid"`   // e.g., "vm:100"
	State  string `json:"state"` // started, stopped, error, fence
	Node   string `json:"node"`
	Status string `json:"status"` // OK, error message
}

// ---------- Tunnels (Cloudflared / Tailscale) ----------

type TunnelInfo struct {
	Cloudflared *CloudflaredInfo `json:"cloudflared,omitempty"`
	Tailscale   *TailscaleInfo   `json:"tailscale,omitempty"`
}

type CloudflaredInfo struct {
	Installed bool                `json:"installed"`
	Version   string              `json:"version,omitempty"`
	Tunnels   []CloudflaredTunnel `json:"tunnels,omitempty"`
	Hint      string              `json:"hint,omitempty"` // Operator hint when detection is partial (e.g. cloudflared CLI not bundled in default image)
}

type CloudflaredTunnel struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"` // healthy, degraded, down, inactive
	CreatedAt   string   `json:"created_at,omitempty"`
	Connections int      `json:"connections"`
	Routes      []string `json:"routes,omitempty"` // ingress hostnames
	OriginIP    string   `json:"origin_ip,omitempty"`
}

type TailscaleInfo struct {
	Installed    bool            `json:"installed"`
	Version      string          `json:"version,omitempty"`
	BackendState string          `json:"backend_state,omitempty"` // Running, Stopped, NeedsLogin, Unreachable
	Self         *TailscaleNode  `json:"self,omitempty"`
	Peers        []TailscaleNode `json:"peers,omitempty"`
	MagicDNS     bool            `json:"magic_dns,omitempty"`
	TailnetName  string          `json:"tailnet_name,omitempty"`
	Hint         string          `json:"hint,omitempty"` // Operator hint when detection is partial (e.g. missing socket mount)
}

type TailscaleNode struct {
	Name     string   `json:"name"`
	DNSName  string   `json:"dns_name,omitempty"`
	IP       string   `json:"ip"`
	OS       string   `json:"os,omitempty"`
	Online   bool     `json:"online"`
	ExitNode bool     `json:"exit_node,omitempty"`
	Relay    string   `json:"relay,omitempty"` // DERP relay region
	TxBytes  int64    `json:"tx_bytes,omitempty"`
	RxBytes  int64    `json:"rx_bytes,omitempty"`
	LastSeen string   `json:"last_seen,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// ---------- Service Checks ----------

const (
	ServiceCheckHTTP       = "http"
	ServiceCheckTCP        = "tcp"
	ServiceCheckDNS        = "dns"
	ServiceCheckSMB        = "smb"
	ServiceCheckNFS        = "nfs"
	ServiceCheckPing       = "ping"
	ServiceCheckSpeed      = "speed"
	ServiceCheckTraceroute = "traceroute"
)

type ServiceCheckConfig struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
	// Instance is the fleet server ID this check is "associated with"
	// (empty = local instance). Currently decorative UI metadata ONLY —
	// the scheduler's RunDueChecks / RunCheck dispatch does not filter
	// on Instance, so every check runs on whichever NAS Doctor instance
	// owns the configuration (the one whose scheduler picked it up).
	//
	// Fleet aggregation works by each peer independently running its
	// own checks and the hub reading pre-computed results from peer
	// snapshots (see internal/fleet/fleet.go) — NOT by remote
	// dispatch. A check marked Instance=X running on the local
	// instance therefore still reads local state (speedtest_history,
	// DNS resolvers, TCP routes, etc.) rather than exercising X.
	//
	// This is a known limitation tracked in #215. Proper fleet-aware
	// dispatch would require a real fleet-target API call on the
	// owning scheduler and is worth doing alongside #205 (Uptime
	// Kuma federation) — both need the same primitive.
	Instance         string            `json:"instance,omitempty"`
	IntervalSec      int               `json:"interval_sec,omitempty"` // Per-check interval in seconds (default 300 = 5min)
	TimeoutSec       int               `json:"timeout_sec,omitempty"`
	Port             int               `json:"port,omitempty"`
	FailureThreshold int               `json:"failure_threshold,omitempty"`
	FailureSeverity  Severity          `json:"failure_severity,omitempty"`
	ExpectedMin      int               `json:"expected_status_min,omitempty"`
	ExpectedMax      int               `json:"expected_status_max,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"` // custom request headers for HTTP checks

	// Speed check specific fields
	ContractedDownMbps float64 `json:"contracted_down_mbps,omitempty"` // expected minimum download speed
	ContractedUpMbps   float64 `json:"contracted_up_mbps,omitempty"`   // expected minimum upload speed
	MarginPct          float64 `json:"margin_pct,omitempty"`           // acceptable margin of error (e.g. 10 = ±10%)

	// DNS check specific fields
	// DNSServer is an optional resolver to use for DNS-type checks (e.g.
	// "1.1.1.1", "8.8.8.8:53", "192.168.1.1:1053"). Empty means use the
	// system resolver. Port defaults to 53 when unspecified.
	DNSServer string `json:"dns_server,omitempty"`

	// Traceroute check specific fields
	// MaxLossPct is the optional end-to-end packet loss threshold (in
	// percent) above which a traceroute check reports "degraded" rather
	// than "up". nil means reachability-only: if the final hop responds
	// the check is up regardless of loss. Pointer type so we can
	// distinguish "unset" from "explicitly 0". See issue #189.
	MaxLossPct *float64 `json:"max_loss_pct,omitempty"`

	// Warning is a transient, load-time-populated message shown when the
	// stored check configuration is invalid under the current schema but
	// was valid under a previous version (e.g. issue #169 — a DNS check
	// with an IP target saved under v0.9.2 is invalid under v0.9.3+).
	// The load path sets Enabled=false and populates this so the UI can
	// surface the issue; it is NOT persisted back to the store
	// automatically — user must acknowledge and resave.
	Warning string `json:"warning,omitempty"`
}

type ServiceCheckResult struct {
	Key                 string   `json:"key"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	Target              string   `json:"target"`
	Status              string   `json:"status"` // up, degraded, down
	ResponseMS          int64    `json:"response_ms"`
	Error               string   `json:"error,omitempty"`
	CheckedAt           string   `json:"checked_at"`
	ConsecutiveFailures int      `json:"consecutive_failures"`
	FailureThreshold    int      `json:"failure_threshold"`
	FailureSeverity     Severity `json:"failure_severity"`

	// Speed check specific fields (populated when Type == "speed")
	DownloadMbps float64 `json:"download_mbps,omitempty"`
	UploadMbps   float64 `json:"upload_mbps,omitempty"`
	LatencyMs    float64 `json:"latency_ms,omitempty"`
	DownloadOK   *bool   `json:"download_ok,omitempty"` // nil for non-speed checks
	UploadOK     *bool   `json:"upload_ok,omitempty"`

	// Details carries per-check-type diagnostic context (HTTP status code,
	// resolved IPs, DNS records, ping RTT, failure stage, etc.). Populated
	// when the parent ServiceChecker has SetCollectDetails(true) — both
	// the ad-hoc Test-button flow (#154) and the scheduled path (#182,
	// since v0.9.4) opt in. The scheduled path persists this map in the
	// service_checks_history.details_json column so the log UI can render
	// the same rich context the Test button already shows.
	Details map[string]any `json:"details,omitempty"`
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
	PID           int     `json:"pid"`
	User          string  `json:"user"`
	CPU           float64 `json:"cpu_percent"`
	Mem           float64 `json:"mem_percent"`
	Command       string  `json:"command"`
	ContainerName string  `json:"container_name"`
	ContainerID   string  `json:"container_id"`
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
	DataAvailable  bool    `json:"data_available"` // true if SMART attributes were successfully read
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
	// HiddenCount reports how many running containers were filtered out
	// by the user's DockerHiddenContainers setting when serving the
	// dashboard snapshot. Not persisted in the stored snapshot — it is
	// stamped onto the response only. See issue #204.
	HiddenCount int `json:"hidden_count,omitempty"`
}

type ContainerInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Image      string  `json:"image"`
	Status     string  `json:"status"` // running, exited, etc.
	State      string  `json:"state"`
	CPU        float64 `json:"cpu_percent"`
	MemMB      float64 `json:"mem_mb"`
	MemPct     float64 `json:"mem_percent"`
	NetIn      float64 `json:"net_in_bytes"`      // cumulative bytes received
	NetOut     float64 `json:"net_out_bytes"`     // cumulative bytes sent
	BlockRead  float64 `json:"block_read_bytes"`  // cumulative bytes read
	BlockWrite float64 `json:"block_write_bytes"` // cumulative bytes written
	Uptime     string  `json:"uptime"`
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
	AvgTempC float64 `json:"avg_temp_c,omitempty"` // average array temperature during this check (computed from smart_history)
	MaxTempC float64 `json:"max_temp_c,omitempty"` // peak temperature during this check
}

// ---------- Fleet / Multi-Server ----------

type RemoteServer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"` // friendly name ("Backup NAS", "Proxmox")
	URL     string            `json:"url"`  // base URL ("http://192.168.1.50:8080" or "https://nas.example.com")
	Enabled bool              `json:"enabled"`
	APIKey  string            `json:"api_key,omitempty"` // NAS Doctor API key auth
	Headers map[string]string `json:"headers,omitempty"` // custom request headers (e.g. CF-Access-Client-Id, Authorization)
}

type RemoteServerStatus struct {
	Server        RemoteServer        `json:"server"`
	Online        bool                `json:"online"`
	LastPoll      string              `json:"last_poll"` // ISO timestamp
	Hostname      string              `json:"hostname"`
	Platform      string              `json:"platform"`
	Version       string              `json:"version"` // NAS Doctor version
	Uptime        string              `json:"uptime"`
	OverallHealth string              `json:"overall_health"` // healthy, warning, critical
	CriticalCount int                 `json:"critical_count"`
	WarningCount  int                 `json:"warning_count"`
	InfoCount     int                 `json:"info_count"`
	Error         string              `json:"error,omitempty"`
	Summary       *FleetServerSummary `json:"summary,omitempty"`
}

// FleetServerSummary holds condensed snapshot data fetched from a remote instance.
type FleetServerSummary struct {
	// System
	CPUUsage   float64 `json:"cpu_usage"`
	MemPercent float64 `json:"mem_percent"`
	MemTotalMB int     `json:"mem_total_mb"`
	MemUsedMB  int     `json:"mem_used_mb"`
	CPUModel   string  `json:"cpu_model"`
	CPUCores   int     `json:"cpu_cores"`
	LoadAvg1   float64 `json:"load_avg_1"`
	IOWait     float64 `json:"io_wait"`

	// Drives
	DriveCount     int     `json:"drive_count"`
	DrivesHealthy  int     `json:"drives_healthy"`
	DrivesWarning  int     `json:"drives_warning"`
	DrivesCritical int     `json:"drives_critical"`
	TotalStorageTB float64 `json:"total_storage_tb"`

	// Docker
	DockerAvailable   bool `json:"docker_available"`
	ContainersRunning int  `json:"containers_running"`
	ContainersTotal   int  `json:"containers_total"`

	// Service checks
	ServiceChecksUp    int `json:"service_checks_up"`
	ServiceChecksDown  int `json:"service_checks_down"`
	ServiceChecksTotal int `json:"service_checks_total"`

	// Findings (actual text for unified view)
	Findings []FleetFinding `json:"findings,omitempty"`
}

// FleetFinding is a single finding from a remote instance.
type FleetFinding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Category string `json:"category"`
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

// ---------- GPU ----------

// GPUInfo aggregates all detected GPU devices.
type GPUInfo struct {
	Available bool        `json:"available"`
	GPUs      []GPUDevice `json:"gpus"`
}

// GPUDevice represents a single GPU (Nvidia, AMD, or Intel).
type GPUDevice struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`            // "NVIDIA GeForce RTX 3090"
	Vendor      string  `json:"vendor"`          // "nvidia", "amd", "intel"
	Driver      string  `json:"driver"`          // driver version
	UsagePct    float64 `json:"usage_percent"`   // GPU core utilization 0-100
	MemUsedMB   float64 `json:"mem_used_mb"`     // VRAM used
	MemTotalMB  float64 `json:"mem_total_mb"`    // VRAM total
	MemPct      float64 `json:"mem_percent"`     // VRAM utilization 0-100
	Temperature int     `json:"temperature_c"`   // core temperature
	FanPct      float64 `json:"fan_percent"`     // fan speed 0-100
	PowerW      float64 `json:"power_watts"`     // current power draw
	PowerMaxW   float64 `json:"power_max_watts"` // TDP / power limit
	ClockMHz    int     `json:"clock_mhz"`       // current GPU clock
	MemClockMHz int     `json:"mem_clock_mhz"`   // current memory clock
	PCIeBus     string  `json:"pcie_bus"`        // "00:02.0"
	EncoderPct  float64 `json:"encoder_percent"` // video encoder utilization (transcoding)
	DecoderPct  float64 `json:"decoder_percent"` // video decoder utilization
}

// ---------- Backup Monitoring ----------

type BackupInfo struct {
	Available bool        `json:"available"`
	Jobs      []BackupJob `json:"jobs"`
}

type BackupJob struct {
	Provider      string    `json:"provider"`       // "borg", "restic", "pbs", "duplicati", "rclone"
	Name          string    `json:"name"`           // job/repo name
	Repository    string    `json:"repository"`     // repo path or URL
	LastRun       time.Time `json:"last_run"`       // timestamp of last backup attempt
	LastSuccess   time.Time `json:"last_success"`   // timestamp of last successful backup
	Status        string    `json:"status"`         // "ok", "warning", "stale", "failed", "running"
	SizeBytes     int64     `json:"size_bytes"`     // total repo/backup size
	FilesCount    int       `json:"files_count"`    // number of files in last snapshot
	Duration      float64   `json:"duration_secs"`  // how long the last backup took
	SnapshotCount int       `json:"snapshot_count"` // number of snapshots/archives in the repo
	ErrorMessage  string    `json:"error_message,omitempty"`
	Schedule      string    `json:"schedule,omitempty"`    // cron expression or "daily", "hourly"
	Compression   string    `json:"compression,omitempty"` // "lz4", "zstd", "none"
	Encrypted     bool      `json:"encrypted"`
	// Label is the user-supplied display name for explicitly-configured
	// external backup repos (issue #279). Empty for auto-detected repos;
	// non-empty entries render as "<label>" on the dashboard card in
	// place of the repo basename.
	Label string `json:"label,omitempty"`
	// Configured distinguishes explicitly user-configured external
	// repos from auto-detected ones. Controls the "Configured" pill
	// on the dashboard card. Issue #279.
	Configured bool `json:"configured,omitempty"`
	// Error is a non-empty string when a backup probe failed — signals
	// the dashboard to render the card in error state. Empty on
	// healthy/stale/warning jobs. Issue #279.
	Error string `json:"error,omitempty"`
	// ErrorReason is a short stable category matching one of the
	// BorgErr* constants in internal/collector. Used by the dashboard
	// widget for user-visible messaging. Issue #279.
	ErrorReason string `json:"error_reason,omitempty"`
}

// ---------- Network Speed Test ----------

type SpeedTestInfo struct {
	Available bool             `json:"available"`
	Latest    *SpeedTestResult `json:"latest,omitempty"`
	// LastAttempt is the scheduler's most recent speed-test outcome,
	// carried alongside Latest so the dashboard widget can render
	// "Running initial speed test…" when the first-ever test is in
	// flight (status=pending with no Latest yet) and so the widget
	// can distinguish a truly-broken state from a never-ran state.
	// Populated on every runSpeedTest tick (success/failed/pending/
	// disabled). See #210.
	LastAttempt *SpeedTestAttempt `json:"last_attempt,omitempty"`
}

// SpeedTestAttempt mirrors storage.LastSpeedTestAttempt as an API-facing
// type. The status values are a closed set: "success", "failed",
// "pending", "disabled". Widget + scheduled type=speed check switch on
// Status to decide what to render / report. See #210.
type SpeedTestAttempt struct {
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
	ErrorMsg  string    `json:"error_msg,omitempty"`
}

type SpeedTestResult struct {
	Timestamp    time.Time `json:"timestamp"`
	DownloadMbps float64   `json:"download_mbps"`
	UploadMbps   float64   `json:"upload_mbps"`
	LatencyMs    float64   `json:"latency_ms"`
	JitterMs     float64   `json:"jitter_ms"`
	ServerName   string    `json:"server_name"`
	ServerID     int       `json:"server_id"`
	ISP          string    `json:"isp"`
	ExternalIP   string    `json:"external_ip"`
	ResultURL    string    `json:"result_url,omitempty"` // speedtest.net result link
	// Engine identifies which speed-test engine produced this result.
	// Closed set: SpeedTestEngineSpeedTestGo ("speedtest_go") for the
	// showwin/speedtest-go primary path, SpeedTestEngineOoklaCLI
	// ("ookla_cli") for the bundled Ookla CLI fallback path. See PRD
	// #283 / issue #284 for the engine-swap rationale and the
	// historical chart's "engine switchover" annotation. Persisted to
	// speedtest_history.engine.
	Engine string `json:"engine,omitempty"`
}

// Speed-test engine identifier constants. Values are stable strings —
// they are persisted into speedtest_history.engine and exported as the
// nasdoctor_speedtest_engine{engine="…"} Prometheus label, so renaming
// them is a breaking change. See issue #284.
const (
	SpeedTestEngineSpeedTestGo = "speedtest_go"
	SpeedTestEngineOoklaCLI    = "ookla_cli"
)

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
	DetectedAt  string   `json:"detected_at,omitempty"`  // ISO timestamp when first detected
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
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Type    string            `json:"type"` // discord, slack, gotify, ntfy, generic
	Enabled bool              `json:"enabled"`
	Headers map[string]string `json:"headers,omitempty"`
	Secret  string            `json:"secret,omitempty"` // for HMAC signing
}

// NotificationRule defines a single, user-created alert rule.
// WHEN [Category] + [Condition] on [Target] crosses [Operator] [Value]
// THEN notify via [Webhook].
type NotificationRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Webhook     string `json:"webhook"`            // webhook name
	Category    string `json:"category"`           // disk_space, disk_temp, smart, service, parity, ups, docker, system, zfs, tunnels, findings, update
	Condition   string `json:"condition"`          // sub-condition within category
	Target      string `json:"target,omitempty"`   // specific drive/service/container (empty = any)
	Operator    string `json:"operator,omitempty"` // gt, lt, eq, any
	Value       string `json:"value,omitempty"`    // threshold value
	CooldownSec int    `json:"cooldown_sec"`       // min seconds between notifications
}

// ---------- Kubernetes ----------

type KubeInfo struct {
	Connected   bool             `json:"connected"`
	Error       string           `json:"error,omitempty"`
	Alias       string           `json:"alias,omitempty"`
	Version     string           `json:"version,omitempty"`  // server version (e.g. v1.31.4+k3s1)
	Platform    string           `json:"platform,omitempty"` // k8s, k3s, eks, gke, aks, etc.
	ClusterName string           `json:"cluster_name,omitempty"`
	Nodes       []KubeNode       `json:"nodes,omitempty"`
	Namespaces  []KubeNamespace  `json:"namespaces,omitempty"`
	Pods        []KubePod        `json:"pods,omitempty"`
	Deployments []KubeDeployment `json:"deployments,omitempty"`
	Services    []KubeService    `json:"services,omitempty"`
	PVCs        []KubePVC        `json:"pvcs,omitempty"`
	Events      []KubeEvent      `json:"events,omitempty"`
}

type KubeNode struct {
	Name             string   `json:"name"`
	Status           string   `json:"status"`                      // Ready, NotReady, Unknown
	Roles            string   `json:"roles,omitempty"`             // control-plane, worker
	Version          string   `json:"version,omitempty"`           // kubelet version
	OS               string   `json:"os,omitempty"`                // linux, windows
	Arch             string   `json:"arch,omitempty"`              // amd64, arm64
	ContainerRuntime string   `json:"container_runtime,omitempty"` // containerd, docker
	InternalIP       string   `json:"internal_ip,omitempty"`
	CPUCores         int      `json:"cpu_cores"`
	CPUUsage         float64  `json:"cpu_usage,omitempty"` // 0.0-1.0 (from metrics API)
	MemTotal         int64    `json:"mem_total"`           // bytes
	MemUsage         int64    `json:"mem_usage,omitempty"` // bytes (from metrics API)
	PodCount         int      `json:"pod_count"`
	PodCapacity      int      `json:"pod_capacity"`
	DiskTotal        int64    `json:"disk_total,omitempty"`       // ephemeral-storage capacity (bytes)
	DiskAllocatable  int64    `json:"disk_allocatable,omitempty"` // ephemeral-storage allocatable (bytes)
	Conditions       []string `json:"conditions,omitempty"`       // MemoryPressure, DiskPressure, PIDPressure
	Age              string   `json:"age,omitempty"`
	Unschedulable    bool     `json:"unschedulable,omitempty"`
}

type KubeNamespace struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // Active, Terminating
	PodCount int    `json:"pod_count"`
	Age      string `json:"age,omitempty"`
}

type KubePod struct {
	Name       string          `json:"name"`
	Namespace  string          `json:"namespace"`
	Node       string          `json:"node,omitempty"`
	Status     string          `json:"status"` // Running, Pending, Succeeded, Failed, CrashLoopBackOff, etc.
	Phase      string          `json:"phase"`  // raw phase
	Ready      string          `json:"ready"`  // e.g. "1/1", "0/1"
	Restarts   int             `json:"restarts"`
	CPUUsage   int64           `json:"cpu_usage_millicores,omitempty"`
	MemUsage   int64           `json:"mem_usage_bytes,omitempty"`
	Age        string          `json:"age,omitempty"`
	IP         string          `json:"ip,omitempty"`
	Containers []KubeContainer `json:"containers,omitempty"`
}

type KubeContainer struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Ready        bool   `json:"ready"`
	RestartCount int    `json:"restart_count"`
	State        string `json:"state"`            // running, waiting, terminated
	Reason       string `json:"reason,omitempty"` // CrashLoopBackOff, OOMKilled, etc.
	LastTermMsg  string `json:"last_term_msg,omitempty"`
}

type KubeDeployment struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Replicas      int    `json:"replicas"`
	ReadyReplicas int    `json:"ready_replicas"`
	Available     int    `json:"available"`
	Unavailable   int    `json:"unavailable"`
	Age           string `json:"age,omitempty"`
	Strategy      string `json:"strategy,omitempty"` // RollingUpdate, Recreate
}

type KubeService struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Type       string   `json:"type"` // ClusterIP, NodePort, LoadBalancer, ExternalName
	ClusterIP  string   `json:"cluster_ip,omitempty"`
	ExternalIP string   `json:"external_ip,omitempty"`
	Ports      []string `json:"ports,omitempty"` // e.g. ["80/TCP", "443/TCP"]
}

type KubePVC struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"` // Bound, Pending, Lost
	StorageClass string `json:"storage_class,omitempty"`
	Capacity     string `json:"capacity,omitempty"`     // e.g. "50Gi"
	AccessModes  string `json:"access_modes,omitempty"` // e.g. "ReadWriteOnce"
	VolumeName   string `json:"volume_name,omitempty"`
	Age          string `json:"age,omitempty"`
}

type KubeEvent struct {
	Type      string `json:"type"` // Normal, Warning
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Object    string `json:"object"` // e.g. "Pod/my-app-xyz"
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
}

type PrometheusConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"` // /metrics
}
