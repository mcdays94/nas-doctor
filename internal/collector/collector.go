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
}

// SetProxmoxConfig updates the Proxmox VE API connection settings.
func (c *Collector) SetProxmoxConfig(cfg ProxmoxConfig) {
	c.proxmoxConfig = cfg
}

// SetKubeConfig updates the Kubernetes cluster connection settings.
func (c *Collector) SetKubeConfig(cfg KubeConfig) {
	c.kubeConfig = cfg
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
	c.logger.Info("collecting SMART data")
	smart, err := collectSMART()
	if err != nil {
		c.logger.Warn("SMART collection partial failure", "error", err)
	}
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

// CollectDockerStats runs a lightweight Docker stats collection (no full scan).
// Used by the scheduler's independent container stats loop for chart history.
func (c *Collector) CollectDockerStats() (*internal.DockerInfo, error) {
	info, err := collectDocker()
	if err != nil {
		return nil, err
	}
	return &info, nil
}
