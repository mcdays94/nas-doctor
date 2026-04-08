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
	hostPaths internal.HostPaths
	logger    *slog.Logger
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

	// ZFS (if available)
	c.logger.Info("collecting ZFS info")
	zfsInfo, err := collectZFS()
	if err != nil {
		c.logger.Warn("ZFS collection partial failure", "error", err)
	}
	if zfsInfo != nil && zfsInfo.Available {
		snap.ZFS = zfsInfo
	}

	snap.Duration = time.Since(start).Seconds()
	c.logger.Info("collection complete", "duration", fmt.Sprintf("%.1fs", snap.Duration))
	return snap, nil
}
