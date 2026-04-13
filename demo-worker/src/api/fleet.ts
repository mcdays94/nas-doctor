import { hoursAgo } from "../data/noise";

export function generateFleet() {
  return [
    {
      name: "Primary NAS",
      url: "http://10.0.1.50:8080",
      status: "online",
      hostname: "unraid-tower",
      platform: "Unraid 7.0.1",
      version: "demo",
      last_seen: hoursAgo(0.01),
      critical_count: 1,
      warning_count: 3,
      overall_health: "warning",
    },
    {
      name: "Synology Backup",
      url: "http://10.0.1.60:8080",
      status: "online",
      hostname: "synology-nas",
      platform: "Synology DSM 7.2.2",
      version: "demo",
      last_seen: hoursAgo(0.02),
      critical_count: 0,
      warning_count: 1,
      overall_health: "healthy",
    },
    {
      name: "Proxmox Host",
      url: "http://10.0.1.70:8080",
      status: "online",
      hostname: "pve-node01",
      platform: "Proxmox VE 8.3.2",
      version: "demo",
      last_seen: hoursAgo(0.01),
      critical_count: 0,
      warning_count: 2,
      overall_health: "warning",
    },
    {
      name: "Remote Backup",
      url: "http://192.168.50.10:8080",
      status: "offline",
      hostname: "offsite-nas",
      platform: "Unknown",
      version: "",
      last_seen: hoursAgo(48),
      critical_count: 0,
      warning_count: 0,
      overall_health: "unknown",
    },
  ];
}
