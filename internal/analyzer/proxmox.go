package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

func analyzeProxmox(pve *internal.ProxmoxInfo) []internal.Finding {
	var findings []internal.Finding

	// Node offline
	for _, n := range pve.Nodes {
		if n.Status != "online" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("Proxmox node offline: %s", n.Name),
				Description: fmt.Sprintf("Node %s is reporting status '%s'. This may indicate a hardware failure, network issue, or planned maintenance.", n.Name, n.Status),
				Impact:      "VMs and containers on this node are unavailable",
				Action:      "Check physical server power, network connectivity, and PVE cluster logs",
				Priority:    "immediate",
			})
		}
		// Node memory high (>90%)
		if n.MemTotal > 0 {
			memPct := float64(n.MemUsed) / float64(n.MemTotal) * 100
			if memPct > 95 {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityCritical,
					Category:    internal.CategoryMemory,
					Title:       fmt.Sprintf("Proxmox node memory critical: %s (%.0f%%)", n.Name, memPct),
					Description: fmt.Sprintf("Node %s is using %.0f%% of %.0f GB RAM. VMs may be killed by the OOM killer.", n.Name, memPct, float64(n.MemTotal)/1073741824),
					Impact:      "Risk of VM/container termination due to out-of-memory",
					Action:      "Migrate VMs to other nodes, increase RAM, or reduce VM memory allocations",
					Priority:    "immediate",
				})
			} else if memPct > 90 {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityWarning,
					Category:    internal.CategoryMemory,
					Title:       fmt.Sprintf("Proxmox node memory high: %s (%.0f%%)", n.Name, memPct),
					Description: fmt.Sprintf("Node %s is using %.0f%% of %.0f GB RAM.", n.Name, memPct, float64(n.MemTotal)/1073741824),
					Impact:      "Performance degradation, risk of OOM if usage increases",
					Action:      "Consider migrating workloads or adding more RAM",
					Priority:    "short-term",
				})
			}
		}
		// Node CPU sustained high (>90%)
		if n.CPUUsage > 0.9 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("Proxmox node CPU high: %s (%.0f%%)", n.Name, n.CPUUsage*100),
				Description: fmt.Sprintf("Node %s CPU at %.0f%% across %d cores (%s)", n.Name, n.CPUUsage*100, n.CPUCores, n.CPUModel),
				Impact:      "VM performance degradation",
				Action:      "Identify high-CPU VMs, consider migrating workloads",
				Priority:    "short-term",
			})
		}
	}

	// Guest issues
	for _, g := range pve.Guests {
		// Stopped VMs/LXCs that might be unexpected
		if g.Status == "stopped" && g.HAState == "started" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("HA-managed guest stopped: %s (VMID %d)", g.Name, g.VMID),
				Description: fmt.Sprintf("%s %s on node %s is stopped but configured for HA with state 'started'. This indicates an HA failure.", strings.ToUpper(g.Type), g.Name, g.Node),
				Impact:      "Service outage for applications running in this guest",
				Action:      "Check PVE HA logs, verify guest can start, check for resource constraints",
				Priority:    "immediate",
			})
		}
		// Guest memory high (>95%)
		if g.Status == "running" && g.MemMax > 0 {
			memPct := float64(g.MemUsed) / float64(g.MemMax) * 100
			if memPct > 95 {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityWarning,
					Category:    internal.CategoryMemory,
					Title:       fmt.Sprintf("Guest memory critical: %s (%.0f%%)", g.Name, memPct),
					Description: fmt.Sprintf("VMID %d (%s) is using %.0f%% of %.1f GB allocated memory", g.VMID, g.Name, memPct, float64(g.MemMax)/1073741824),
					Impact:      "Application performance issues or crashes inside the guest",
					Action:      "Increase memory allocation or optimize applications",
					Priority:    "short-term",
				})
			}
		}
	}

	// Storage pools high usage
	for _, s := range pve.Storage {
		if s.Total <= 0 || s.Status != "available" {
			continue
		}
		if s.UsedPct > 95 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryDisk,
				Title:       fmt.Sprintf("PVE storage critical: %s on %s (%.0f%%)", s.Storage, s.Node, s.UsedPct),
				Description: fmt.Sprintf("Storage pool '%s' (%s) on node %s is %.0f%% full. Total: %.0f GB", s.Storage, s.Type, s.Node, s.UsedPct, float64(s.Total)/1073741824),
				Impact:      "Cannot create snapshots, backups, or new VMs. Running VMs may fail on disk writes.",
				Action:      "Free space immediately: remove old backups, snapshots, or unused disk images",
				Priority:    "immediate",
			})
		} else if s.UsedPct > 85 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDisk,
				Title:       fmt.Sprintf("PVE storage high: %s on %s (%.0f%%)", s.Storage, s.Node, s.UsedPct),
				Description: fmt.Sprintf("Storage pool '%s' (%s) on node %s is %.0f%% full", s.Storage, s.Type, s.Node, s.UsedPct),
				Impact:      "May run out of space for backups and snapshots",
				Action:      "Plan storage cleanup or expansion",
				Priority:    "short-term",
			})
		}
	}

	// Backup freshness — check if any backup task ran in the last 48 hours
	if len(pve.Tasks) > 0 {
		lastBackup := int64(0)
		for _, t := range pve.Tasks {
			if t.Type == "vzdump" && t.EndTime > lastBackup {
				lastBackup = t.EndTime
			}
		}
		if lastBackup > 0 {
			age := time.Now().Unix() - lastBackup
			if age > 172800 { // >48 hours
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityWarning,
					Category:    internal.CategorySystem,
					Title:       "Proxmox backups may be stale",
					Description: fmt.Sprintf("Last successful backup was %.0f hours ago. Consider verifying your backup schedule.", float64(age)/3600),
					Impact:      "Data loss risk if a VM fails without recent backup",
					Action:      "Check Datacenter → Backup in PVE to verify backup jobs are scheduled and running",
					Priority:    "short-term",
				})
			}
		}
		// Failed tasks
		for _, t := range pve.Tasks {
			if t.Status != "" && t.Status != "OK" && t.Status != "running" && t.EndTime > time.Now().Add(-24*time.Hour).Unix() {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityWarning,
					Category:    internal.CategorySystem,
					Title:       fmt.Sprintf("PVE task failed: %s on %s", t.Type, t.Node),
					Description: fmt.Sprintf("Task %s for VMID %d on node %s finished with status: %s", t.Type, t.VMID, t.Node, t.Status),
					Impact:      "Backup/migration may not have completed",
					Action:      "Check task log in PVE for details",
					Priority:    "short-term",
				})
			}
		}
	}

	// HA service errors
	for _, ha := range pve.HAServices {
		if ha.State == "error" || ha.State == "fence" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("PVE HA service error: %s", ha.SID),
				Description: fmt.Sprintf("HA service %s on node %s is in state '%s': %s", ha.SID, ha.Node, ha.State, ha.Status),
				Impact:      "HA-managed service may be unavailable",
				Action:      "Check HA logs, verify fencing configuration, check node health",
				Priority:    "immediate",
			})
		}
	}

	return findings
}
