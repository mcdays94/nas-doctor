import { Platform, PROFILES } from "../data/platforms";
import { hoursAgo } from "../data/noise";

export function generateAlerts(platform: Platform) {
  const p = PROFILES[platform];

  const alerts: Array<{
    id: string;
    title: string;
    severity: string;
    category: string;
    status: string;
    first_seen: string;
    last_seen: string;
    count: number;
    acknowledged: boolean;
    snoozed: boolean;
    snoozed_until: string | null;
  }> = [];

  // ── Critical: service check failure (always present) ──
  alerts.push({
    id: "alert-001",
    title: "Service check failed: Pi-hole DNS (12 consecutive failures)",
    severity: "critical",
    category: "service_check",
    status: "active",
    first_seen: hoursAgo(1),
    last_seen: hoursAgo(0.08),
    count: 12,
    acknowledged: false,
    snoozed: false,
    snoozed_until: null,
  });

  // ── Critical: disk usage (data-driven, but guaranteed) ──
  const highDisk = p.drives.find((d) => d.usedPct > 75) || p.drives[0];
  alerts.push({
    id: "alert-002",
    title: `Disk usage critical on ${highDisk.label} (${highDisk.usedPct}%)`,
    severity: highDisk.usedPct > 80 ? "critical" : "warning",
    category: "disk",
    status: "active",
    first_seen: hoursAgo(72),
    last_seen: hoursAgo(0.5),
    count: 14,
    acknowledged: false,
    snoozed: false,
    snoozed_until: null,
  });

  // ── Warning: fleet server offline (always present) ──
  alerts.push({
    id: "alert-003",
    title: "Fleet server 'Remote Backup' offline for 48 hours",
    severity: "warning",
    category: "fleet",
    status: "active",
    first_seen: hoursAgo(48),
    last_seen: hoursAgo(0.5),
    count: 96,
    acknowledged: false,
    snoozed: false,
    snoozed_until: null,
  });

  // ── Warning: high power-on hours ──
  const oldDrive = p.drives.find((d) => d.powerOnHours > 25000) || p.drives[0];
  alerts.push({
    id: "alert-004",
    title: `Drive aging: ${oldDrive.model} has ${oldDrive.powerOnHours.toLocaleString()} power-on hours`,
    severity: "warning",
    category: "smart",
    status: "active",
    first_seen: hoursAgo(168),
    last_seen: hoursAgo(6),
    count: 7,
    acknowledged: true,
    snoozed: false,
    snoozed_until: null,
  });

  // ── Warning: stopped container ──
  const stopped = p.containers.find((c) => c.state === "exited");
  if (stopped) {
    alerts.push({
      id: "alert-005",
      title: `Container '${stopped.name}' has been stopped for 3+ days`,
      severity: "warning",
      category: "docker",
      status: "active",
      first_seen: hoursAgo(96),
      last_seen: hoursAgo(1),
      count: 48,
      acknowledged: false,
      snoozed: false,
      snoozed_until: null,
    });
  }

  // ── Resolved: NVMe temperature spike ──
  alerts.push({
    id: "alert-006",
    title: "NVMe temperature exceeded 50°C threshold",
    severity: "warning",
    category: "smart",
    status: "resolved",
    first_seen: hoursAgo(240),
    last_seen: hoursAgo(192),
    count: 3,
    acknowledged: true,
    snoozed: false,
    snoozed_until: null,
  });

  // ── Resolved: UPS battery event ──
  alerts.push({
    id: "alert-007",
    title: "UPS switched to battery power — mains restored after 12 min",
    severity: "critical",
    category: "ups",
    status: "resolved",
    first_seen: hoursAgo(360),
    last_seen: hoursAgo(359.8),
    count: 1,
    acknowledged: true,
    snoozed: false,
    snoozed_until: null,
  });

  // ── Snoozed: container high memory ──
  alerts.push({
    id: "alert-008",
    title: `Container '${p.containers[0]?.name || "plex"}' memory usage above 80%`,
    severity: "warning",
    category: "docker",
    status: "active",
    first_seen: hoursAgo(48),
    last_seen: hoursAgo(2),
    count: 24,
    acknowledged: false,
    snoozed: true,
    snoozed_until: hoursAgo(-4), // 4 hours from now
  });

  return alerts;
}
