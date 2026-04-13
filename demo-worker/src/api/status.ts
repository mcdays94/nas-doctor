import { Platform, PROFILES } from "../data/platforms";
import { timeNoise, clamp } from "../data/noise";

export function generateStatus(platform: Platform) {
  const p = PROFILES[platform];
  const critical = Math.round(clamp(timeNoise(1, 3600000, 1, 1), 0, 3));
  const warning = Math.round(clamp(timeNoise(3, 3600000, 2, 2), 1, 6));
  const info = Math.round(clamp(timeNoise(4, 3600000, 2, 3), 2, 8));

  const overall =
    critical > 0 ? "critical" : warning > 2 ? "warning" : "healthy";

  const uptimeDays = Math.floor(p.uptimeSecs / 86400);
  const uptimeHrs = Math.floor((p.uptimeSecs % 86400) / 3600);

  return {
    hostname: p.hostname,
    platform: p.platform,
    version: "demo",
    uptime: `${uptimeDays}d ${uptimeHrs}h`,
    last_scan: new Date(Date.now() - 120000).toISOString(),
    scan_interval_secs: 21600,
    scan_running: false,
    critical_count: critical,
    warning_count: warning,
    info_count: info,
    overall_health: overall,
    // Field names must match the real Go app exactly
    sections: {
      findings: true,
      disk_space: true,
      smart: true,
      docker: true,
      container_metrics: false,
      network: true,
      ups: p.hasUPS,
      zfs: p.hasZFS,
      gpu: p.hasGPU,
      parity: p.hasParity,
      tunnels: p.hasTunnels,
      proxmox: p.hasProxmox,
      kubernetes: p.hasKubernetes,
      merged_containers: true,
      merged_drives: true,
    },
    chart_range_hours: 1,
    section_heights: null,
    dismissed_findings: [],
  };
}
