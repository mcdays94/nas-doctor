import { Platform, PROFILES } from "../data/platforms";
import { hoursAgo } from "../data/noise";

export function generateIncidents(platform: Platform) {
  const p = PROFILES[platform];

  return [
    // ── Active: service check failure ──
    {
      id: "inc-001",
      type: "service_check",
      severity: "critical",
      title: "Service check failed: Pi-hole DNS",
      description: "Pi-hole DNS at http://10.0.1.53/admin has been unreachable for 60 minutes. 12 consecutive failures detected. DNS resolution may be affected for clients using Pi-hole.",
      timestamp: hoursAgo(1),
      resolved: false,
      resolved_at: null,
      source: "service_checks",
      affected_entity: "service:Pi-hole DNS",
    },
    // ── Active: disk space ──
    {
      id: "inc-002",
      type: "threshold_breach",
      severity: "warning",
      title: `Disk usage exceeded threshold on ${p.drives[0].label}`,
      description: `${p.drives[0].label} (${p.drives[0].model}) reached ${p.drives[0].usedPct}% utilization. Current growth rate: ~0.3% per day. Projected full in approximately 60 days.`,
      timestamp: hoursAgo(18),
      resolved: false,
      resolved_at: null,
      source: "disk_analyzer",
      affected_entity: `/dev/${p.drives[0].device}`,
    },
    // ── Active: fleet server offline ──
    {
      id: "inc-003",
      type: "fleet_event",
      severity: "warning",
      title: "Fleet server 'Remote Backup' went offline",
      description: "The fleet server at http://192.168.50.10:8080 stopped responding 48 hours ago. Possible causes: network outage, server down, or NAS Doctor service stopped.",
      timestamp: hoursAgo(48),
      resolved: false,
      resolved_at: null,
      source: "fleet_poller",
      affected_entity: "fleet:Remote Backup",
    },
    // ── Resolved: container OOM restart ──
    {
      id: "inc-004",
      type: "container_event",
      severity: "warning",
      title: `Container '${p.containers[0]?.name || "plex"}' restarted (OOM killed)`,
      description: `Container exited with code 137 (out of memory). Docker automatically restarted it. The container used 2.8 GB of its 3 GB limit before being killed.`,
      timestamp: hoursAgo(36),
      resolved: true,
      resolved_at: hoursAgo(35.9),
      source: "docker_monitor",
      affected_entity: `container:${p.containers[0]?.name || "plex"}`,
    },
    // ── Resolved: temperature spike ──
    {
      id: "inc-005",
      type: "threshold_breach",
      severity: "warning",
      title: "NVMe temperature exceeded 50°C threshold",
      description: `Cache drive reached 53°C during heavy I/O (transcoding + backup). Temperature normalized to 42°C after workload decreased.`,
      timestamp: hoursAgo(96),
      resolved: true,
      resolved_at: hoursAgo(94),
      source: "smart_monitor",
      affected_entity: `/dev/${p.drives[p.drives.length - 1].device}`,
    },
    // ── Resolved: network flap ──
    {
      id: "inc-006",
      type: "network_event",
      severity: "warning",
      title: "Network interface eth0 link flapped",
      description: "Interface went down for 2.3 seconds then recovered. Correlated with switch firmware upgrade in progress on the upstream Ubiquiti switch.",
      timestamp: hoursAgo(168),
      resolved: true,
      resolved_at: hoursAgo(167.99),
      source: "network_monitor",
      affected_entity: "interface:eth0",
    },
    // ── Resolved: UPS event ──
    {
      id: "inc-007",
      type: "power_event",
      severity: "critical",
      title: "UPS switched to battery — mains power lost",
      description: "Utility power lost at 03:42 AM. UPS (CyberPower CP1500PFCLCD) ran on battery for 12 minutes at 35% load before mains power was restored.",
      timestamp: hoursAgo(360),
      resolved: true,
      resolved_at: hoursAgo(359.8),
      source: "ups_monitor",
      affected_entity: "ups:CyberPower CP1500PFCLCD",
    },
    // ── Info: successful parity check ──
    {
      id: "inc-008",
      type: "system_event",
      severity: "info",
      title: "Scheduled diagnostic scan completed",
      description: "6-hour diagnostic scan completed successfully in 4.2 seconds. All subsystems checked. 2 warnings, 1 critical issue detected.",
      timestamp: hoursAgo(2),
      resolved: true,
      resolved_at: hoursAgo(1.95),
      source: "scheduler",
      affected_entity: "system",
    },
    // ── Info: webhook delivery ──
    {
      id: "inc-009",
      type: "notification_event",
      severity: "info",
      title: "Alert notification delivered to Discord",
      description: "Critical alert 'Service check failed: Pi-hole DNS' was successfully delivered to Discord webhook '#nas-alerts'. Delivery took 245ms.",
      timestamp: hoursAgo(0.9),
      resolved: true,
      resolved_at: hoursAgo(0.89),
      source: "notifier",
      affected_entity: "webhook:Discord - #nas-alerts",
    },
  ];
}
