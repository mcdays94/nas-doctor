// Package collector orchestrates all diagnostic data collection.
package collector

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mcdays94/nas-doctor/internal"
)

// Collector runs all diagnostic sub-collectors and assembles a Snapshot.
type Collector struct {
	hostPaths     internal.HostPaths
	logger        *slog.Logger
	proxmoxConfig ProxmoxConfig
	kubeConfig    KubeConfig
	smartConfig   SMARTConfig
}

// SetProxmoxConfig updates the Proxmox VE API connection settings.
func (c *Collector) SetProxmoxConfig(cfg ProxmoxConfig) {
	c.proxmoxConfig = cfg
}

// SetKubeConfig updates the Kubernetes cluster connection settings.
func (c *Collector) SetKubeConfig(cfg KubeConfig) {
	c.kubeConfig = cfg
}

// SetSMARTConfig updates SMART-collector behaviour flags — primarily the
// WakeDrives toggle introduced for issue #198.
func (c *Collector) SetSMARTConfig(cfg SMARTConfig) {
	c.smartConfig = cfg
}

// New creates a new Collector with the given host path mappings.
func New(hostPaths internal.HostPaths, logger *slog.Logger) *Collector {
	return &Collector{
		hostPaths: hostPaths,
		logger:    logger,
	}
}

// Collect runs all diagnostic collectors and returns a complete Snapshot.
func (c *Collector) Collect() (*internal.Snapshot, error) {
	start := time.Now()
	snap := &internal.Snapshot{
		ID:        uuid.New().String(),
		Timestamp: start,
	}

	// System info
	c.logger.Info("collecting system info")
	sys, err := collectSystem(c.hostPaths)
	if err != nil {
		c.logger.Warn("system collection partial failure", "error", err)
	}
	snap.System = sys

	// Disk usage
	c.logger.Info("collecting disk info")
	disks, err := collectDisks()
	if err != nil {
		c.logger.Warn("disk collection partial failure", "error", err)
	}
	snap.Disks = disks

	// SMART data
	c.logger.Info("collecting SMART data", "wake_drives", c.smartConfig.WakeDrives)
	smart, standbyDevices, err := collectSMART(c.smartConfig, c.logger)
	if err != nil {
		c.logger.Warn("SMART collection partial failure", "error", err)
	}
	// Issue #238: surface standby device list to the scheduler so the
	// StaleSMARTChecker can evaluate max-age and force-wake if overdue.
	snap.SMARTStandbyDevices = standbyDevices
	// Enrich SMART data with Unraid array slot mapping (md -> physical device)
	if smart != nil && sys.Platform == "unraid" {
		mdMap := buildMDToPhysicalMap() // "sdb" -> "1" (for /mnt/disk1)
		for i := range smart {
			devName := strings.TrimPrefix(smart[i].Device, "/dev/")
			if mdNum, ok := mdMap[devName]; ok {
				smart[i].ArraySlot = "disk" + mdNum
			}
		}
	}
	snap.SMART = smart

	// Docker containers
	c.logger.Info("collecting Docker info")
	docker, err := collectDocker()
	if err != nil {
		c.logger.Warn("Docker collection partial failure", "error", err)
	}
	snap.Docker = docker

	// Enrich top processes with container attribution (requires Docker data)
	if docker.Available && len(docker.Containers) > 0 && len(sys.TopProcesses) > 0 {
		containerIDMap := buildContainerIDMap(docker.Containers)
		enrichProcessContainers(sys.TopProcesses, containerIDMap, "/proc")
		snap.System.TopProcesses = sys.TopProcesses
	}

	// Network
	c.logger.Info("collecting network info")
	net, err := collectNetwork()
	if err != nil {
		c.logger.Warn("network collection partial failure", "error", err)
	}
	snap.Network = net

	// Logs (dmesg, syslog)
	c.logger.Info("collecting log entries")
	logs, err := collectLogs(c.hostPaths)
	if err != nil {
		c.logger.Warn("log collection partial failure", "error", err)
	}
	snap.Logs = logs

	// Parity (Unraid-specific)
	if sys.Platform == "unraid" {
		c.logger.Info("collecting Unraid parity info")
		parity, err := collectParity(c.hostPaths)
		if err != nil {
			c.logger.Warn("parity collection partial failure", "error", err)
		}
		snap.Parity = parity
	}

	// UPS (NUT or apcupsd)
	c.logger.Info("collecting UPS info")
	upsInfo, err := collectUPS()
	if err != nil {
		c.logger.Warn("UPS collection partial failure", "error", err)
	}
	if upsInfo != nil && upsInfo.Available {
		snap.UPS = upsInfo
	}

	// OS update check (cached, runs max once per 24h)
	c.logger.Info("checking OS update status")
	if sys.Platform != "" && sys.PlatformVer != "" {
		updateInfo := collectUpdateInfo(sys.Platform, sys.PlatformVer)
		if updateInfo != nil {
			snap.Update = updateInfo
		}
	}

	// Tunnels (cloudflared / tailscale) — checks host binaries + Docker containers
	c.logger.Info("collecting tunnel info")
	tunnelInfo := collectTunnels(docker)
	if tunnelInfo != nil {
		snap.Tunnels = tunnelInfo
	}

	// Proxmox VE (if configured)
	if c.proxmoxConfig.Enabled {
		c.logger.Info("collecting Proxmox VE data")
		pveInfo := CollectProxmox(c.proxmoxConfig)
		if pveInfo != nil {
			pveInfo.Alias = c.proxmoxConfig.Alias
			snap.Proxmox = pveInfo
			if pveInfo.Error != "" {
				c.logger.Warn("Proxmox VE collection error", "error", pveInfo.Error)
			} else {
				c.logger.Info("Proxmox VE data collected", "nodes", len(pveInfo.Nodes), "guests", len(pveInfo.Guests))
			}
		}
	}

	// Kubernetes (if configured)
	if c.kubeConfig.Enabled {
		c.logger.Info("collecting Kubernetes data")
		kubeInfo := CollectKubernetes(c.kubeConfig)
		if kubeInfo != nil {
			kubeInfo.Alias = c.kubeConfig.Alias
			snap.Kubernetes = kubeInfo
			if kubeInfo.Error != "" {
				c.logger.Warn("Kubernetes collection error", "error", kubeInfo.Error)
			} else {
				c.logger.Info("Kubernetes data collected", "nodes", len(kubeInfo.Nodes), "pods", len(kubeInfo.Pods))
			}
		}
	}

	// GPU (Nvidia / AMD / Intel)
	c.logger.Info("collecting GPU info")
	gpuInfo := collectGPU()
	if gpuInfo != nil && gpuInfo.Available {
		snap.GPU = gpuInfo
	}

	// ZFS (if available)
	c.logger.Info("collecting ZFS info")
	zfsInfo, err := collectZFS()
	if err != nil {
		c.logger.Warn("ZFS collection partial failure", "error", err)
	}
	if zfsInfo != nil && zfsInfo.Available {
		snap.ZFS = zfsInfo
	}

	// Backup monitoring (Borg, Restic, PBS, Duplicati)
	c.logger.Info("collecting backup info")
	backupInfo := collectBackups()
	if backupInfo != nil && backupInfo.Available {
		snap.Backup = backupInfo
	}

	// Speed test: not collected here (runs on its own schedule via scheduler)
	// snap.SpeedTest is populated by the scheduler's speed test loop

	snap.Duration = time.Since(start).Seconds()
	c.logger.Info("collection complete", "duration", fmt.Sprintf("%.1fs", snap.Duration))
	return snap, nil
}

// CollectSMARTForced reads SMART for the given devices without the
// `-n standby` guard. Used by the scheduler's StaleSMARTChecker (issue
// #238) as the seam for the force-wake path. Thin wrapper around the
// package-level function so the scheduler doesn't need to import
// collector internals directly.
func (c *Collector) CollectSMARTForced(devices []string) ([]internal.SMARTInfo, error) {
	return CollectSMARTForced(devices, c.logger)
}

// ---------- Per-subsystem public collect methods (issue #260) ----------
//
// These are the thin wrappers the scheduler's ScanDispatcher calls
// when a given subsystem's interval elapses. They preserve the exact
// logging + error handling of the monolithic Collect() flow, and
// return the same types produced by the existing internal collectX
// functions. The monolithic Collect() is retained but now only
// invokes the 9 non-configurable subsystems; the 6 configurable ones
// (SMART, Docker, Proxmox, Kubernetes, ZFS, GPU) are invoked
// independently from the scheduler's main loop per their configured
// cadence.

// CollectSMART performs the SMART subsystem pass. Returns the per-
// drive SMARTInfo list, the device names that were in standby (so
// the scheduler's StaleSMARTChecker can evaluate max-age), and any
// error surfaced by the underlying collector. Platform-specific
// array-slot enrichment for Unraid is applied here so the wrapper
// produces the same shape as the monolithic Collect() path.
func (c *Collector) CollectSMART(platform string) ([]internal.SMARTInfo, []string, error) {
	c.logger.Info("collecting SMART data", "wake_drives", c.smartConfig.WakeDrives)
	smart, standbyDevices, err := collectSMART(c.smartConfig, c.logger)
	if err != nil {
		c.logger.Warn("SMART collection partial failure", "error", err)
	}
	if smart != nil && platform == "unraid" {
		mdMap := buildMDToPhysicalMap()
		for i := range smart {
			devName := strings.TrimPrefix(smart[i].Device, "/dev/")
			if mdNum, ok := mdMap[devName]; ok {
				smart[i].ArraySlot = "disk" + mdNum
			}
		}
	}
	return smart, standbyDevices, err
}

// CollectDocker performs the Docker subsystem pass. Returns the full
// DockerInfo as a value (matching the monolithic Collect()'s
// snap.Docker field type).
func (c *Collector) CollectDocker() (internal.DockerInfo, error) {
	c.logger.Info("collecting Docker info")
	docker, err := collectDocker()
	if err != nil {
		c.logger.Warn("Docker collection partial failure", "error", err)
	}
	return docker, err
}

// CollectProxmox performs the Proxmox subsystem pass. Returns nil
// (and no error) when Proxmox integration is not configured — the
// dispatcher can still fire this subsystem on its cadence without
// producing stale data.
func (c *Collector) CollectProxmox() (*internal.ProxmoxInfo, error) {
	if !c.proxmoxConfig.Enabled {
		return nil, nil
	}
	c.logger.Info("collecting Proxmox VE data")
	pveInfo := CollectProxmox(c.proxmoxConfig)
	if pveInfo != nil {
		pveInfo.Alias = c.proxmoxConfig.Alias
		if pveInfo.Error != "" {
			c.logger.Warn("Proxmox VE collection error", "error", pveInfo.Error)
		} else {
			c.logger.Info("Proxmox VE data collected", "nodes", len(pveInfo.Nodes), "guests", len(pveInfo.Guests))
		}
	}
	return pveInfo, nil
}

// CollectKubernetes performs the Kubernetes subsystem pass. Returns
// nil when K8s integration is not configured.
func (c *Collector) CollectKubernetes() (*internal.KubeInfo, error) {
	if !c.kubeConfig.Enabled {
		return nil, nil
	}
	c.logger.Info("collecting Kubernetes data")
	kubeInfo := CollectKubernetes(c.kubeConfig)
	if kubeInfo != nil {
		kubeInfo.Alias = c.kubeConfig.Alias
		if kubeInfo.Error != "" {
			c.logger.Warn("Kubernetes collection error", "error", kubeInfo.Error)
		} else {
			c.logger.Info("Kubernetes data collected", "nodes", len(kubeInfo.Nodes), "pods", len(kubeInfo.Pods))
		}
	}
	return kubeInfo, nil
}

// CollectZFS performs the ZFS subsystem pass.
func (c *Collector) CollectZFS() (*internal.ZFSInfo, error) {
	c.logger.Info("collecting ZFS info")
	zfsInfo, err := collectZFS()
	if err != nil {
		c.logger.Warn("ZFS collection partial failure", "error", err)
	}
	return zfsInfo, err
}

// CollectGPU performs the GPU subsystem pass.
func (c *Collector) CollectGPU() *internal.GPUInfo {
	c.logger.Info("collecting GPU info")
	return collectGPU()
}

// CollectDockerStats runs a lightweight Docker stats collection (no full scan).
// Used by the scheduler's independent container stats loop for chart history.
func (c *Collector) CollectDockerStats() (*internal.DockerInfo, error) {
	info, err := collectDocker()
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// CollectTopProcesses returns the top n processes by CPU usage.
// Used by the scheduler's process stats loop for standalone collection.
func (c *Collector) CollectTopProcesses(n int) []internal.ProcessInfo {
	return collectTopProcesses(n)
}

// EnrichProcessContainers populates ContainerID and ContainerName on each
// ProcessInfo by reading cgroup data and matching against known containers.
// procRoot allows overriding the /proc filesystem root for testing (pass ""
// for the default "/proc").
func EnrichProcessContainers(procs []internal.ProcessInfo, containers []internal.ContainerInfo, procRoot string) {
	if len(procs) == 0 || len(containers) == 0 {
		return
	}
	containerIDMap := buildContainerIDMap(containers)
	if procRoot == "" {
		procRoot = "/proc"
	}
	enrichProcessContainers(procs, containerIDMap, procRoot)
}
