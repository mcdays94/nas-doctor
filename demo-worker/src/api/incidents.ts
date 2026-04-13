import { Platform, PROFILES } from "../data/platforms";
import { hoursAgo } from "../data/noise";

export function generateIncidents(platform: Platform) {
  const p = PROFILES[platform];

  const incidents: Array<{
    id: string;
    type: string;
    severity: string;
    title: string;
    description: string;
    timestamp: string;
    resolved: boolean;
    resolved_at: string | null;
    source: string;
    affected_entity: string;
  }> = [];

  // Disk space warning — recent
  const highDisk = p.drives.find((d) => d.usedPct > 75);
  if (highDisk) {
    incidents.push({
      id: "inc-001",
      type: "threshold_breach",
      severity: "warning",
      title: `Disk usage exceeded 75% on ${highDisk.label}`,
      description: `${highDisk.label} (${highDisk.model}) reached ${highDisk.usedPct}% utilization. Current trajectory suggests full within 90 days.`,
      timestamp: hoursAgo(18),
      resolved: false,
      resolved_at: null,
      source: "disk_analyzer",
      affected_entity: `/dev/${highDisk.device}`,
    });
  }

  // Container restart
  incidents.push({
    id: "inc-002",
    type: "container_event",
    severity: "info",
    title: "Container 'plex' restarted unexpectedly",
    description: "Container exited with code 137 (OOM killed) and was automatically restarted by Docker.",
    timestamp: hoursAgo(48),
    resolved: true,
    resolved_at: hoursAgo(47.9),
    source: "docker_monitor",
    affected_entity: "container:plex",
  });

  // Temperature spike — resolved
  incidents.push({
    id: "inc-003",
    type: "threshold_breach",
    severity: "warning",
    title: "Drive temperature exceeded 50°C threshold",
    description: `NVMe cache drive reached 53°C during heavy transcoding. Temperature normalized after workload decreased.`,
    timestamp: hoursAgo(96),
    resolved: true,
    resolved_at: hoursAgo(94),
    source: "smart_monitor",
    affected_entity: `/dev/${p.drives[p.drives.length - 1].device}`,
  });

  // Network interface flap
  incidents.push({
    id: "inc-004",
    type: "network_event",
    severity: "warning",
    title: "Network interface eth0 link flapped",
    description: "Interface went down for 2.3 seconds then recovered. Possible cable or switch issue.",
    timestamp: hoursAgo(168),
    resolved: true,
    resolved_at: hoursAgo(167.99),
    source: "network_monitor",
    affected_entity: "interface:eth0",
  });

  // UPS event
  if (p.hasUPS) {
    incidents.push({
      id: "inc-005",
      type: "power_event",
      severity: "critical",
      title: "UPS switched to battery — mains power lost",
      description: "Utility power lost at 03:42. UPS ran on battery for 12 minutes before power was restored.",
      timestamp: hoursAgo(360),
      resolved: true,
      resolved_at: hoursAgo(359.8),
      source: "ups_monitor",
      affected_entity: "ups:CyberPower CP1500PFCLCD",
    });
  }

  // Stopped container
  const stopped = p.containers.find((c) => c.state === "exited");
  if (stopped) {
    incidents.push({
      id: "inc-006",
      type: "container_event",
      severity: "info",
      title: `Container '${stopped.name}' stopped`,
      description: `Container ${stopped.name} exited cleanly (code 0) and has not been restarted.`,
      timestamp: hoursAgo(72),
      resolved: false,
      resolved_at: null,
      source: "docker_monitor",
      affected_entity: `container:${stopped.name}`,
    });
  }

  // Scan completed
  incidents.push({
    id: "inc-007",
    type: "system_event",
    severity: "info",
    title: "Full diagnostic scan completed",
    description: "Scheduled 6-hour scan completed successfully. 3 warnings, 0 critical issues found.",
    timestamp: hoursAgo(2),
    resolved: true,
    resolved_at: hoursAgo(1.95),
    source: "scheduler",
    affected_entity: "system",
  });

  // Parity check
  if (p.hasParity) {
    incidents.push({
      id: "inc-008",
      type: "parity_event",
      severity: "info",
      title: "Parity check completed successfully",
      description: "Monthly parity check finished in 14h 22m with 0 errors. Average speed: 95 MB/s.",
      timestamp: hoursAgo(168),
      resolved: true,
      resolved_at: hoursAgo(154),
      source: "parity_monitor",
      affected_entity: "parity:md0",
    });
  }

  return incidents.slice(0, 8);
}
