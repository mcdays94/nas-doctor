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
  }> = [];

  // Always include: high disk usage warning
  const highDisk = p.drives.find((d) => d.usedPct > 80);
  if (highDisk) {
    alerts.push({
      id: "alert-001",
      title: `Disk usage critical on ${highDisk.label} (${highDisk.usedPct}%)`,
      severity: "critical",
      category: "disk",
      status: "active",
      first_seen: hoursAgo(72),
      last_seen: hoursAgo(0.5),
      count: 14,
      acknowledged: false,
      snoozed: false,
    });
  }

  // High power-on hours
  const oldDrive = p.drives.find((d) => d.powerOnHours > 35000);
  if (oldDrive) {
    alerts.push({
      id: "alert-002",
      title: `Drive ${oldDrive.model} has ${oldDrive.powerOnHours.toLocaleString()} power-on hours`,
      severity: "warning",
      category: "smart",
      status: "active",
      first_seen: hoursAgo(168),
      last_seen: hoursAgo(6),
      count: 7,
      acknowledged: true,
      snoozed: false,
    });
  }

  // Stopped container alert
  const stopped = p.containers.find((c) => c.state === "exited");
  if (stopped) {
    alerts.push({
      id: "alert-003",
      title: `Container "${stopped.name}" has been stopped for 3+ days`,
      severity: "warning",
      category: "docker",
      status: "active",
      first_seen: hoursAgo(96),
      last_seen: hoursAgo(1),
      count: 48,
      acknowledged: false,
      snoozed: false,
    });
  }

  // Temperature alert — resolved
  alerts.push({
    id: "alert-004",
    title: "NVMe temperature exceeded 50°C threshold",
    severity: "warning",
    category: "smart",
    status: "resolved",
    first_seen: hoursAgo(240),
    last_seen: hoursAgo(192),
    count: 3,
    acknowledged: true,
    snoozed: false,
  });

  // UPS on battery — resolved
  if (p.hasUPS) {
    alerts.push({
      id: "alert-005",
      title: "UPS switched to battery power",
      severity: "critical",
      category: "ups",
      status: "resolved",
      first_seen: hoursAgo(360),
      last_seen: hoursAgo(359.8),
      count: 1,
      acknowledged: true,
      snoozed: false,
    });
  }

  return alerts.slice(0, 5);
}
