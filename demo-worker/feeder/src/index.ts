/**
 * NAS Doctor Demo Feeder — Cron Worker
 *
 * Runs every 5 minutes. Reads the unraid seed data from KV (captured from the
 * real Go binary), then generates platform-specific variants for synology,
 * proxmox, and kubernetes by transforming the seed data with platform-appropriate
 * system specs, drives, containers, and section toggles.
 *
 * All platform variants maintain the exact same JSON structure as the Go app
 * produces — only the values change to match each platform's characteristics.
 */

interface Env {
  DEMO_DATA: KVNamespace;
}

export const PLATFORMS = ["unraid", "synology", "truenas", "proxmox", "kubernetes"] as const;
export type Platform = (typeof PLATFORMS)[number];

const ENDPOINTS = [
  "status", "snapshot", "sparklines", "fleet", "service_checks",
  "alerts", "incidents", "notifications_log", "gpu_history",
  "container_history", "system_history", "process_history", "settings", "db_stats",
  "disks", "smart_trends", "replacement_plan", "capacity_forecast",
  "speedtest_history",
];

// ── Platform profiles: define what makes each platform unique ──
export interface PlatformProfile {
  hostname: string;
  platformName: string;
  cpuModel: string;
  cpuCores: number;
  ramGB: number;
  uptimeDays: number;
  hasZFS: boolean;
  hasUPS: boolean;
  hasParity: boolean;
  hasProxmox: boolean;
  hasKubernetes: boolean;
  hasTunnels: boolean;
  hasGPU: boolean;
  drives: { device: string; model: string; serial: string; sizeGB: number; type: string; mount: string; label: string; usedPct: number; temp: number; poh: number }[];
  containers: { name: string; image: string; state: string; cpu: number; mem: number }[];
  // Speed-test profile: realistic per-platform throughput. Feeds
  // buildSpeedTest + buildSpeedTestHistory. See #262.
  speedTest: {
    downloadMbps: number;
    uploadMbps: number;
    latencyMs: number;
    jitterMs: number;
    serverName: string;
    isp: string;
  };
  // Optional GPU device profile, populated when hasGPU === true. See
  // buildGPU / #262.
  gpuDevice?: {
    name: string;
    vendor: string;
    driver: string;
    memTotalMB: number;
    memUsedPct: number;
    powerMaxW: number;
    usagePct: number;
    tempC: number;
  };
}

export const PROFILES: Record<Platform, PlatformProfile> = {
  unraid: {
    hostname: "unraid-tower", platformName: "Unraid 7.0.1", cpuModel: "AMD Ryzen 9 5950X", cpuCores: 16, ramGB: 64, uptimeDays: 30,
    hasZFS: false, hasUPS: true, hasParity: true, hasProxmox: false, hasKubernetes: false, hasTunnels: true, hasGPU: true,
    drives: [
      { device: "sda", model: "WDC WD180EDGZ", serial: "WD-WX12345678", sizeGB: 18000, type: "hdd", mount: "/mnt/disk1", label: "Disk 1", usedPct: 72, temp: 35, poh: 28000 },
      { device: "sdb", model: "WDC WD180EDGZ", serial: "WD-WX23456789", sizeGB: 18000, type: "hdd", mount: "/mnt/disk2", label: "Disk 2", usedPct: 65, temp: 34, poh: 28000 },
      { device: "sdc", model: "Seagate IronWolf 16TB", serial: "ST-ZL34567890", sizeGB: 16000, type: "hdd", mount: "/mnt/disk3", label: "Disk 3", usedPct: 88, temp: 38, poh: 42000 },
      { device: "sdd", model: "Seagate IronWolf 16TB", serial: "ST-ZL45678901", sizeGB: 16000, type: "hdd", mount: "/mnt/disk4", label: "Disk 4", usedPct: 45, temp: 36, poh: 42000 },
      { device: "sde", model: "WDC WD180EDGZ", serial: "WD-WX56789012", sizeGB: 18000, type: "hdd", mount: "/mnt/parity", label: "Parity", usedPct: 0, temp: 37, poh: 28000 },
      { device: "nvme0n1", model: "Samsung 990 Pro 2TB", serial: "S6XNNS0T123456", sizeGB: 2000, type: "nvme", mount: "/mnt/cache", label: "Cache", usedPct: 42, temp: 45, poh: 8000 },
    ],
    containers: [
      { name: "plex", image: "linuxserver/plex:latest", state: "running", cpu: 12.3, mem: 1340 },
      { name: "emby", image: "emby/embyserver:latest", state: "running", cpu: 8.7, mem: 2028 },
      { name: "tdarr", image: "haveagitgat/tdarr:latest", state: "running", cpu: 5.4, mem: 916 },
      { name: "nginx-proxy", image: "nginx:alpine", state: "running", cpu: 0.3, mem: 42 },
      { name: "wireguard", image: "linuxserver/wireguard:latest", state: "running", cpu: 0.1, mem: 28 },
      { name: "home-assistant", image: "homeassistant/home-assistant:latest", state: "running", cpu: 3.2, mem: 512 },
      { name: "grafana", image: "grafana/grafana:latest", state: "running", cpu: 1.1, mem: 186 },
      { name: "radarr", image: "linuxserver/radarr:latest", state: "running", cpu: 0.4, mem: 312 },
      { name: "sonarr", image: "linuxserver/sonarr:latest", state: "running", cpu: 0.3, mem: 298 },
      { name: "sabnzbd", image: "linuxserver/sabnzbd:latest", state: "running", cpu: 0.1, mem: 95 },
      { name: "pihole", image: "pihole/pihole:latest", state: "exited", cpu: 0, mem: 0 },
    ],
    speedTest: { downloadMbps: 920, uploadMbps: 880, latencyMs: 7.8, jitterMs: 1.6, serverName: "Virgin Media London", isp: "Virgin Media" },
    gpuDevice: { name: "NVIDIA RTX A2000", vendor: "nvidia", driver: "555.58", memTotalMB: 6144, memUsedPct: 42, powerMaxW: 70, usagePct: 28, tempC: 52 },
  },
  synology: {
    hostname: "synology-nas", platformName: "Synology DSM 7.2.2", cpuModel: "Intel Celeron J4125", cpuCores: 4, ramGB: 8, uptimeDays: 90,
    hasZFS: false, hasUPS: true, hasParity: false, hasProxmox: false, hasKubernetes: false, hasTunnels: false, hasGPU: false,
    drives: [
      { device: "sata1", model: "Seagate IronWolf 8TB", serial: "ST-ZA12345678", sizeGB: 8000, type: "hdd", mount: "/volume1", label: "Volume 1", usedPct: 78, temp: 36, poh: 52000 },
      { device: "sata2", model: "Seagate IronWolf 8TB", serial: "ST-ZA23456789", sizeGB: 8000, type: "hdd", mount: "/volume1", label: "Volume 1 (RAID)", usedPct: 78, temp: 37, poh: 52000 },
      { device: "sata3", model: "WDC WD80EFZZ", serial: "WD-CA34567890", sizeGB: 8000, type: "hdd", mount: "/volume2", label: "Volume 2", usedPct: 45, temp: 35, poh: 18000 },
      { device: "nvme0n1", model: "Samsung 970 EVO Plus 500GB", serial: "S4EWNS0M123456", sizeGB: 500, type: "nvme", mount: "/volume1/@docker", label: "SSD Cache", usedPct: 61, temp: 48, poh: 12000 },
    ],
    containers: [
      { name: "synology-photos", image: "synology/photos:latest", state: "running", cpu: 2.1, mem: 384 },
      { name: "synology-drive", image: "synology/drive:latest", state: "running", cpu: 1.5, mem: 256 },
      { name: "plex", image: "linuxserver/plex:latest", state: "running", cpu: 8.5, mem: 768 },
      { name: "homebridge", image: "homebridge/homebridge:latest", state: "running", cpu: 0.8, mem: 128 },
      { name: "watchtower", image: "containrrr/watchtower:latest", state: "running", cpu: 0.1, mem: 32 },
    ],
    speedTest: { downloadMbps: 450, uploadMbps: 42, latencyMs: 14.2, jitterMs: 2.8, serverName: "BT Wholesale Manchester", isp: "BT Broadband" },
  },
  truenas: {
    hostname: "truenas-scale", platformName: "TrueNAS SCALE 24.10", cpuModel: "Intel Xeon E-2278G", cpuCores: 8, ramGB: 64, uptimeDays: 120,
    hasZFS: true, hasUPS: true, hasParity: false, hasProxmox: false, hasKubernetes: false, hasTunnels: false, hasGPU: false,
    drives: [
      { device: "sda", model: "WDC WD120EMFZ", serial: "WD-WMC540123456", sizeGB: 12000, type: "hdd", mount: "/mnt/tank", label: "tank (raidz2)", usedPct: 68, temp: 34, poh: 38000 },
      { device: "sdb", model: "WDC WD120EMFZ", serial: "WD-WMC540234567", sizeGB: 12000, type: "hdd", mount: "/mnt/tank", label: "tank (raidz2)", usedPct: 68, temp: 35, poh: 38000 },
      { device: "sdc", model: "WDC WD120EMFZ", serial: "WD-WMC540345678", sizeGB: 12000, type: "hdd", mount: "/mnt/tank", label: "tank (raidz2)", usedPct: 68, temp: 33, poh: 38000 },
      { device: "sdd", model: "WDC WD120EMFZ", serial: "WD-WMC540456789", sizeGB: 12000, type: "hdd", mount: "/mnt/tank", label: "tank (raidz2)", usedPct: 68, temp: 36, poh: 38000 },
      { device: "sde", model: "WDC WD120EMFZ", serial: "WD-WMC540567890", sizeGB: 12000, type: "hdd", mount: "/mnt/tank", label: "tank (raidz2)", usedPct: 68, temp: 34, poh: 25000 },
      { device: "sdf", model: "WDC WD120EMFZ", serial: "WD-WMC540678901", sizeGB: 12000, type: "hdd", mount: "/mnt/tank", label: "tank (raidz2)", usedPct: 68, temp: 35, poh: 25000 },
      { device: "nvme0n1", model: "Intel Optane 900P 480GB", serial: "INTEL-OPTANE-001", sizeGB: 480, type: "nvme", mount: "/mnt/tank", label: "SLOG", usedPct: 5, temp: 42, poh: 22000 },
      { device: "nvme1n1", model: "Samsung 980 Pro 1TB", serial: "S6XNNS0T555555", sizeGB: 1000, type: "nvme", mount: "/mnt/tank", label: "L2ARC", usedPct: 82, temp: 46, poh: 14000 },
    ],
    containers: [
      { name: "plex", image: "plexinc/pms-docker:latest", state: "running", cpu: 15.2, mem: 2048 },
      { name: "nextcloud", image: "nextcloud:latest", state: "running", cpu: 2.1, mem: 512 },
      { name: "minio", image: "minio/minio:latest", state: "running", cpu: 0.8, mem: 256 },
      { name: "syncthing", image: "syncthing/syncthing:latest", state: "running", cpu: 0.4, mem: 128 },
      { name: "prometheus", image: "prom/prometheus:latest", state: "running", cpu: 0.6, mem: 384 },
      { name: "grafana", image: "grafana/grafana:latest", state: "running", cpu: 0.3, mem: 192 },
      { name: "nas-doctor", image: "ghcr.io/mcdays94/nas-doctor:latest", state: "running", cpu: 0.2, mem: 48 },
    ],
    speedTest: { downloadMbps: 980, uploadMbps: 960, latencyMs: 4.5, jitterMs: 0.9, serverName: "OVH Roubaix", isp: "OVH" },
  },
  proxmox: {
    hostname: "pve-node01", platformName: "Proxmox VE 8.3.2", cpuModel: "Intel Xeon E-2388G", cpuCores: 8, ramGB: 128, uptimeDays: 45,
    hasZFS: true, hasUPS: true, hasParity: false, hasProxmox: true, hasKubernetes: false, hasTunnels: true, hasGPU: true,
    drives: [
      { device: "sda", model: "Samsung PM893 960GB", serial: "S6XNNS0T567890", sizeGB: 960, type: "ssd", mount: "/", label: "Boot SSD", usedPct: 18, temp: 32, poh: 15000 },
      { device: "sdb", model: "Samsung PM893 960GB", serial: "S6XNNS0T678901", sizeGB: 960, type: "ssd", mount: "/", label: "Boot Mirror", usedPct: 18, temp: 33, poh: 15000 },
      { device: "nvme0n1", model: "Samsung 990 Pro 4TB", serial: "S6XNNS0T789012", sizeGB: 4000, type: "nvme", mount: "/mnt/vm-storage", label: "VM Storage", usedPct: 62, temp: 42, poh: 8000 },
      { device: "nvme1n1", model: "Samsung 990 Pro 4TB", serial: "S6XNNS0T890123", sizeGB: 4000, type: "nvme", mount: "/mnt/vm-storage", label: "VM Mirror", usedPct: 62, temp: 43, poh: 8000 },
    ],
    containers: [
      { name: "nas-doctor", image: "ghcr.io/mcdays94/nas-doctor:latest", state: "running", cpu: 0.5, mem: 64 },
      { name: "traefik", image: "traefik:v3.0", state: "running", cpu: 0.3, mem: 48 },
      { name: "portainer", image: "portainer/portainer-ce:latest", state: "running", cpu: 0.2, mem: 96 },
    ],
    speedTest: { downloadMbps: 1850, uploadMbps: 1820, latencyMs: 2.1, jitterMs: 0.4, serverName: "Cogent Amsterdam", isp: "Cogent" },
    gpuDevice: { name: "NVIDIA Tesla P4", vendor: "nvidia", driver: "535.216", memTotalMB: 8192, memUsedPct: 58, powerMaxW: 75, usagePct: 62, tempC: 48 },
  },
  kubernetes: {
    hostname: "k3s-master-01", platformName: "K3s v1.31.3+k3s1", cpuModel: "AMD EPYC 7543P", cpuCores: 32, ramGB: 256, uptimeDays: 60,
    hasZFS: false, hasUPS: false, hasParity: false, hasProxmox: false, hasKubernetes: true, hasTunnels: false, hasGPU: false,
    drives: [
      { device: "sda", model: "Samsung PM9A3 3.84TB", serial: "S6XNNS0T901234", sizeGB: 3840, type: "nvme", mount: "/", label: "System", usedPct: 12, temp: 38, poh: 6000 },
      { device: "sdb", model: "Samsung PM9A3 3.84TB", serial: "S6XNNS0T012345", sizeGB: 3840, type: "nvme", mount: "/var/lib/longhorn", label: "Longhorn", usedPct: 54, temp: 40, poh: 6000 },
    ],
    containers: [
      { name: "coredns", image: "rancher/mirrored-coredns-coredns:1.11.3", state: "running", cpu: 0.2, mem: 32 },
      { name: "traefik", image: "rancher/mirrored-library-traefik:2.11.0", state: "running", cpu: 0.4, mem: 64 },
      { name: "longhorn-manager", image: "longhornio/longhorn-manager:v1.7.0", state: "running", cpu: 1.2, mem: 256 },
      { name: "nas-doctor", image: "ghcr.io/mcdays94/nas-doctor:latest", state: "running", cpu: 0.3, mem: 48 },
    ],
    speedTest: { downloadMbps: 2450, uploadMbps: 2410, latencyMs: 1.8, jitterMs: 0.3, serverName: "Google Cloud us-central1", isp: "Google Cloud" },
  },
};

export default {
  async scheduled(event: ScheduledController, env: Env, ctx: ExecutionContext): Promise<void> {
    console.log("Feeder: refreshing all platforms at", new Date().toISOString());

    for (const platform of PLATFORMS) {
      for (const ep of ENDPOINTS) {
        try {
          // Read unraid seed (the base from the real Go binary)
          const seedKey = `seed:unraid:${ep}`;
          const apiKey = `api:${platform}:${ep}`;

          let seedData = await env.DEMO_DATA.get(seedKey, "text");
          if (!seedData) {
            // Bootstrap: copy from api key if seed doesn't exist yet
            seedData = await env.DEMO_DATA.get(`api:unraid:${ep}`, "text");
            if (seedData) await env.DEMO_DATA.put(seedKey, seedData);
          }
          if (!seedData) continue;

          const data = JSON.parse(seedData);
          // First apply time-based refresh (jitter, timestamp shift)
          const refreshed = refreshData(ep, data);
          // Then apply platform transformation (hostname, drives, containers, sections)
          const transformed = transformForPlatform(ep, refreshed, platform);
          await env.DEMO_DATA.put(apiKey, JSON.stringify(transformed));
        } catch (e) {
          console.error(`Feeder: error ${platform}/${ep}:`, e);
        }
      }
    }
    console.log("Feeder: done refreshing", PLATFORMS.length, "platforms x", ENDPOINTS.length, "endpoints");
  },

  async fetch(request: Request, env: Env): Promise<Response> {
    if (new URL(request.url).pathname === "/trigger") {
      await this.scheduled!({} as ScheduledController, env, { waitUntil: () => {}, passThroughOnException: () => {} } as unknown as ExecutionContext);
      return new Response("Feeder triggered manually — all platforms refreshed", { status: 200 });
    }
    return new Response("NAS Doctor Demo Feeder — cron every 5 min", { status: 200 });
  },
} satisfies ExportedHandler<Env>;

// ── Platform transformation ──
// Takes refreshed data (from unraid seed) and transforms it for a specific platform.

function transformForPlatform(endpoint: string, data: unknown, platform: Platform): unknown {
  // These are always generated fresh for ALL platforms (not from seed)
  if (endpoint === "service_checks") return buildServiceChecks();
  if (endpoint === "alerts") return buildAlerts(PROFILES[platform]);
  if (endpoint === "incidents") return buildIncidents(PROFILES[platform]);
  if (endpoint === "notifications_log") return buildNotificationLog();
  if (endpoint === "settings") return transformSettings(data as Record<string, unknown>, PROFILES[platform]);
  if (endpoint === "status") return transformStatus(data as Record<string, unknown>, PROFILES[platform]);
  if (endpoint === "disks") return buildDisksAPI(PROFILES[platform]);
  if (endpoint === "smart_trends") return buildSmartTrends(PROFILES[platform]);
  if (endpoint === "replacement_plan") return buildReplacementPlan(PROFILES[platform]);
  if (endpoint === "capacity_forecast") return buildCapacityForecast(PROFILES[platform]);
  if (endpoint === "sparklines") return transformSparklines(data as Record<string, unknown>, PROFILES[platform]);
  if (endpoint === "snapshot") return transformSnapshot(data as Record<string, unknown>, PROFILES[platform], platform);
  if (endpoint === "speedtest_history") return buildSpeedTestHistory(PROFILES[platform], 24);

  return data; // everything else passed through from seed
}

function transformStatus(d: Record<string, unknown>, p: PlatformProfile): Record<string, unknown> {
  const sec = d.sections as Record<string, unknown> || {};
  return {
    ...d,
    hostname: p.hostname,
    platform: p.platformName,
    sections: {
      ...sec,
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
  };
}

function transformSettings(d: Record<string, unknown>, p: PlatformProfile): Record<string, unknown> {
  const sec = (d as any).sections || {};
  return {
    ...d,
    theme: "midnight",
    sections: {
      ...sec,
      findings: true, disk_space: true, smart: true, docker: true,
      container_metrics: false, network: true,
      ups: p.hasUPS, zfs: p.hasZFS, gpu: p.hasGPU,
      parity: p.hasParity, tunnels: p.hasTunnels,
      proxmox: p.hasProxmox, kubernetes: p.hasKubernetes,
      merged_containers: true, merged_drives: true,
    },
    service_checks: {
      checks: [
        { name: "Gateway", url: "10.0.1.1", method: "ping", interval_seconds: 30, timeout_seconds: 5, expected_status: 0, severity: "critical" },
        { name: "NAS Doctor", url: "http://localhost:8060/api/v1/health", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200, severity: "critical" },
        { name: "Plex Media Server", url: "http://localhost:32400/web", method: "GET", interval_seconds: 30, timeout_seconds: 10, expected_status: 200, severity: "warning" },
        { name: "Pi-hole DNS", url: "http://10.0.1.53/admin", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200, severity: "critical" },
        { name: "Nextcloud", url: "https://cloud.example.com/status.php", method: "GET", interval_seconds: 60, timeout_seconds: 10, expected_status: 200, severity: "warning" },
        { name: "Grafana", url: "http://localhost:3000/api/health", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200, severity: "warning" },
        { name: "Router Admin", url: "http://10.0.1.1", method: "GET", interval_seconds: 60, timeout_seconds: 5, expected_status: 200, severity: "warning" },
        { name: "External DNS", url: "1.1.1.1", method: "ping", interval_seconds: 60, timeout_seconds: 5, expected_status: 0, severity: "critical" },
        { name: "Google DNS", url: "8.8.8.8", method: "ping", interval_seconds: 60, timeout_seconds: 5, expected_status: 0, severity: "info" },
        { name: "Home Assistant", url: "http://10.0.1.55:8123", method: "GET", interval_seconds: 60, timeout_seconds: 10, expected_status: 200, severity: "warning" },
        { name: "AdGuard Home", url: "http://10.0.1.53:3000", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200, severity: "warning" },
      ],
    },
  };
}

export function transformSnapshot(d: Record<string, unknown>, p: PlatformProfile, platform: Platform): Record<string, unknown> {
  const df = dayFactor();

  // System
  const sys = { ...(d.system as Record<string, unknown> || {}) };
  sys.hostname = p.hostname;
  sys.platform = p.platformName;
  sys.cpu_model = p.cpuModel;
  sys.cpu_cores = p.cpuCores;
  sys.mem_total_gb = p.ramGB;
  sys.mem_used_gb = round2(clamp(jitter(p.ramGB * 0.68, 10, hashStr(platform) + 1), p.ramGB * 0.3, p.ramGB * 0.92));
  sys.mem_percent = round2(((sys.mem_used_gb as number) / p.ramGB) * 100);
  sys.cpu_usage = round2(clamp(jitter(22 * df, 20, hashStr(platform) + 2), 3, 85));
  sys.uptime_seconds = p.uptimeDays * 86400 + Math.floor(Date.now() / 1000) % 86400;

  // Disks
  const disks = p.drives.map((dr, i) => ({
    device: `/dev/${dr.device}`, mount_point: dr.mount, label: dr.label,
    fs_type: dr.type === "nvme" ? "ext4" : "xfs",
    total_gb: round2(dr.sizeGB),
    used_gb: round2(clamp(jitter(dr.usedPct, 3, hashStr(platform) + 100 + i), 0, 99) / 100 * dr.sizeGB),
    free_gb: 0, used_percent: 0,
  }));
  for (const dk of disks) { dk.free_gb = round2(dk.total_gb - dk.used_gb); dk.used_percent = round2((dk.used_gb / dk.total_gb) * 100); }

  // SMART — must include ALL fields the stats page reads
  const temp = (dr: typeof p.drives[0], i: number) => Math.round(clamp(jitter(dr.temp, 8, hashStr(platform) + 200 + i), 22, 60));
  const smart = p.drives.map((dr, i) => ({
    device: `/dev/${dr.device}`, model: dr.model, serial: dr.serial,
    firmware: "FW01",
    size_gb: round2(dr.sizeGB),
    health_passed: true,
    data_available: true,
    power_on_hours: dr.poh + Math.floor(Date.now() / 3600000) % 100,
    temperature_c: temp(dr, i),
    temperature_max_c: temp(dr, i) + 8,
    reallocated_sectors: 0,
    pending_sectors: 0,
    offline_uncorrectable: 0,
    udma_crc_errors: 0,
    command_timeout: 0,
    spin_retry_count: 0,
    raw_read_error_rate: 0,
    seek_error_rate: 0,
    disk_type: dr.type,
    ata_port: dr.type === "nvme" ? "" : `ata${i + 1}`,
    array_slot: dr.label,
    power_cycle_count: Math.floor(dr.poh / 2000) + 12,
    wear_leveling: dr.type !== "hdd" ? Math.round(clamp(100 - dr.poh / 500, 70, 100)) : undefined,
    reads_gb: round2(jitter(dr.poh * 0.8, 10, hashStr(platform) + 300 + i)),
    writes_gb: round2(jitter(dr.poh * 0.3, 10, hashStr(platform) + 400 + i)),
  }));

  // Docker containers
  const containers = p.containers.map((c, i) => ({
    id: `${platform.slice(0, 3)}-${i}`, name: c.name, image: c.image,
    state: c.state, status: c.state === "running" ? `Up ${p.uptimeDays}d` : "Exited (0) 3 days ago",
    cpu_percent: c.state === "running" ? round2(clamp(jitter(c.cpu * df, 25, hashStr(platform) + 500 + i), 0, 100)) : 0,
    mem_mb: c.state === "running" ? round2(clamp(jitter(c.mem, 15, hashStr(platform) + 600 + i), 10, c.mem * 2)) : 0,
    mem_percent: c.state === "running" ? round2(clamp(jitter(c.mem / (p.ramGB * 10.24), 20, hashStr(platform) + 700 + i), 0.1, 50)) : 0,
    net_in_bytes: c.state === "running" ? Math.round(jitter(5e9, 30, hashStr(platform) + 800 + i)) : 0,
    net_out_bytes: c.state === "running" ? Math.round(jitter(3e9, 30, hashStr(platform) + 900 + i)) : 0,
    block_read_bytes: c.state === "running" ? Math.round(jitter(10e9, 25, hashStr(platform) + 1000 + i)) : 0,
    block_write_bytes: c.state === "running" ? Math.round(jitter(4e9, 25, hashStr(platform) + 1100 + i)) : 0,
    uptime: c.state === "running" ? `${p.uptimeDays} days` : "Exited",
  }));

  // Sections that differ per platform
  const ups = p.hasUPS ? d.ups : { available: false };
  const zfs = p.hasZFS ? buildZFS(p) : { available: false, pools: [] };
  // GPU is synthesised from the profile's gpuDevice spec rather than
  // passed through from the seed: the captured unraid snapshot may
  // not have rich GPU data, and we want per-platform variety
  // (RTX A2000 for Unraid transcoding, Tesla P4 for Proxmox VM
  // passthrough). See #262 and dashboard.go L613 for the fields the
  // widget consumes (gpus[].{name, vendor, usage_percent, …}).
  const gpu = p.hasGPU ? buildGPU(p, platform) : { available: false, gpus: [] };
  const parity = p.hasParity ? d.parity : { available: false, history: [] };
  const tunnels = p.hasTunnels ? d.tunnels : { available: false, cloudflared: [] };
  const proxmox = p.hasProxmox ? d.proxmox : { available: false };
  const kubernetes = p.hasKubernetes ? d.kubernetes : { available: false };

  // Widgets added in #262 — feeder generates data for these even
  // though the captured live-binary snapshot doesn't include them.
  const speed_test = buildSpeedTest(p);
  const backup = buildBackup(p, platform);
  const top_processes = buildTopProcesses(p, platform);

  // system.top_processes is how the Processes widget reads its data
  // (sections.processes in dashboard.go L1144 reads sn.system.top_processes).
  sys.top_processes = top_processes;

  // Rebuild findings for this platform's data
  const findings = buildFindings(p, disks, smart, containers);

  return {
    ...d,
    system: sys,
    disks,
    smart,
    docker: { available: true, version: "24.0.7", containers },
    ups, zfs, gpu, parity, tunnels, proxmox, kubernetes,
    speed_test, backup,
    findings,
  };
}

function transformSparklines(d: Record<string, unknown>, p: PlatformProfile): Record<string, unknown> {
  // Adjust disk temp sparklines to match this platform's drives
  const diskSparklines = p.drives.map((dr, i) => ({
    serial: dr.serial,
    temps: Array.from({ length: 24 }, (_, j) => ({
      timestamp: new Date(Date.now() - (23 - j) * 3600000).toISOString(),
      temp: Math.round(clamp(jitter(dr.temp, 8, hashStr(dr.serial) + j), 22, 58)),
    })),
  }));
  return { ...d, disks: diskSparklines };
}

// /api/v1/disks — matches real Go app format
function buildDisksAPI(p: PlatformProfile): unknown[] {
  return p.drives.map((dr, i) => ({
    device: `/dev/${dr.device}`,
    serial: dr.serial,
    model: dr.model,
    last_temperature: Math.round(clamp(jitter(dr.temp, 8, hashStr(dr.serial) + 50 + i), 22, 58)),
    last_health_passed: true,
    power_on_hours: dr.poh + Math.floor(Date.now() / 3600000) % 100,
    data_points: 78,
  }));
}

// /api/v1/smart/trends — matches real Go app format
function buildSmartTrends(p: PlatformProfile): unknown[] {
  return p.drives.map((dr) => ({
    serial: dr.serial,
    model: dr.model,
    device: `/dev/${dr.device}`,
    type: dr.type,
    attributes: [
      { id: 194, name: "temperature_celsius", values: Array.from({ length: 30 }, (_, d) => ({ date: new Date(Date.now() - (29 - d) * 86400000).toISOString().split("T")[0], raw: Math.round(clamp(jitter(dr.temp, 6, hashStr(dr.serial) + d), 22, 55)) })) },
      { id: 9, name: "power_on_hours", values: Array.from({ length: 30 }, (_, d) => ({ date: new Date(Date.now() - (29 - d) * 86400000).toISOString().split("T")[0], raw: dr.poh + d * 24 })) },
      { id: 5, name: "reallocated_sector_ct", values: Array.from({ length: 30 }, (_, d) => ({ date: new Date(Date.now() - (29 - d) * 86400000).toISOString().split("T")[0], raw: 0 })) },
      ...(dr.type !== "hdd" ? [{ id: 177, name: "wear_leveling_count", values: Array.from({ length: 30 }, (_, d) => ({ date: new Date(Date.now() - (29 - d) * 86400000).toISOString().split("T")[0], raw: Math.round(100 - dr.poh / 500) })) }] : []),
    ],
  }));
}

// /api/v1/replacement-plan — matches real Go app format
function buildReplacementPlan(p: PlatformProfile): unknown {
  const drives = p.drives.map((dr, i) => {
    const years = dr.poh / 8766;
    const healthScore = Math.round(clamp(100 - years * 8 - (dr.temp > 40 ? 10 : 0), 20, 100));
    const urgency = healthScore < 40 ? "replace_now" : healthScore < 60 ? "replace_soon" : healthScore < 80 ? "monitor" : "healthy";
    return {
      device: `/dev/${dr.device}`, model: dr.model, serial: dr.serial,
      array_slot: dr.label, disk_type: dr.type,
      size_gb: round2(dr.sizeGB), health_score: healthScore, health_passed: true,
      urgency, urgency_label: urgency.replace("_", " "),
      risk_factors: [
        ...(years > 3 ? [`Age: ${years.toFixed(1)} years`] : []),
        ...(dr.temp > 40 ? [`Temperature: ${dr.temp}°C`] : []),
        ...(dr.poh > 40000 ? ["High power-on hours"] : []),
      ],
      failure_mult: round2(years > 4 ? 2.5 : years > 3 ? 1.5 : 1.0),
      remaining_years: round2(Math.max(0, 6 - years)),
      life_used_pct: round2(clamp(years / 6 * 100, 0, 100)),
      age_bracket: years > 4 ? "wear-out" : years > 1 ? "normal" : "infant",
      temp_rating: dr.temp > 45 ? "hot" : dr.temp > 38 ? "warm" : "cool",
      cost_estimate: round2(dr.type === "nvme" ? dr.sizeGB * 0.06 : dr.type === "ssd" ? dr.sizeGB * 0.05 : dr.sizeGB * 0.015),
      power_on_hours: dr.poh, temp_c: dr.temp,
      reallocated: 0, pending: 0, crc_errors: 0,
    };
  });
  drives.sort((a, b) => a.health_score - b.health_score);
  return {
    drives,
    total_drives: drives.length,
    replace_now: drives.filter(d => d.urgency === "replace_now").length,
    replace_soon: drives.filter(d => d.urgency === "replace_soon").length,
    monitor: drives.filter(d => d.urgency === "monitor").length,
    healthy: drives.filter(d => d.urgency === "healthy").length,
    total_cost: round2(drives.filter(d => d.urgency !== "healthy").reduce((s, d) => s + d.cost_estimate, 0)),
    total_cost_all: round2(drives.reduce((s, d) => s + d.cost_estimate, 0)),
    cost_configured: true,
    data_version: 1,
  };
}

// /api/v1/capacity-forecast — matches real Go app format exactly
function buildCapacityForecast(p: PlatformProfile): unknown {
  const volumes = p.drives.filter(d => d.usedPct > 0).map((dr) => {
    const growthGBPerDay = round2(dr.sizeGB * (0.001 + hash(hashStr(dr.serial) + 42) * 0.003));
    const currentUsedGB = round2(dr.usedPct / 100 * dr.sizeGB);
    const remainingGB = dr.sizeGB - currentUsedGB;
    const daysTo = (threshold: number) => {
      if (dr.usedPct >= threshold) return 0;
      const targetGB = (threshold / 100) * dr.sizeGB;
      return growthGBPerDay > 0.001 ? Math.round((targetGB - currentUsedGB) / growthGBPerDay) : -1;
    };
    const d90 = daysTo(90), d95 = daysTo(95), d100 = daysTo(100);
    const urgency = d100 >= 0 && d100 < 30 ? "critical" : d100 >= 0 && d100 < 90 ? "warning" : "ok";
    const trend = growthGBPerDay > 0.5 ? "growing" : growthGBPerDay < -0.5 ? "shrinking" : "stable";
    return {
      mount_point: dr.mount, label: dr.label, device: `/dev/${dr.device}`,
      total_gb: round2(dr.sizeGB),
      current_pct: round2(dr.usedPct),
      current_used_gb: currentUsedGB,
      growth_gb_per_day: growthGBPerDay,
      days_to_90: d90,
      days_to_95: d95,
      days_to_100: d100,
      confidence: 78,
      data_points: 78,
      trend,
      urgency,
    };
  });
  return {
    volumes,
    total_volumes: volumes.length,
    critical: volumes.filter(v => v.urgency === "critical").length,
    warning: volumes.filter(v => v.urgency === "warning").length,
    ok: volumes.filter(v => v.urgency === "ok").length,
  };
}

function hoursAgo(h: number): string {
  return new Date(Date.now() - h * 3600000).toISOString();
}

function buildAlerts(p: PlatformProfile): unknown[] {
  const alerts: unknown[] = [];
  let id = 1;
  const a = (title: string, sev: string, cat: string, status: string, firstH: number, lastH: number, count: number, ack: boolean, snoozed: boolean, snoozedUntil: string | null = null) => {
    alerts.push({ id: `alert-${id++}`, title, severity: sev, category: cat, status, first_seen: hoursAgo(firstH), last_seen: hoursAgo(lastH), count, acknowledged: ack, snoozed, snoozed_until: snoozedUntil });
  };

  // Active critical
  a("Service check failed: Pi-hole Admin (12 consecutive failures)", "critical", "service_check", "active", 1, 0.08, 12, false, false);
  a("Service check failed: Local DNS (8 consecutive failures)", "critical", "service_check", "active", 0.8, 0.08, 8, false, false);

  // Active critical — disk
  const highDisk = p.drives.find(d => d.usedPct > 80) || p.drives[0];
  a(`Disk usage critical on ${highDisk.label} (${highDisk.usedPct}%)`, highDisk.usedPct > 80 ? "critical" : "warning", "disk", "active", 72, 0.5, 14, false, false);

  // Active warning — fleet
  a("Fleet server 'Remote Backup' offline for 48 hours", "warning", "fleet", "active", 48, 0.5, 96, false, false);

  // Active warning — SMART
  const oldDrive = p.drives.find(d => d.poh > 25000) || p.drives[0];
  a(`Drive aging: ${oldDrive.model} has ${oldDrive.poh.toLocaleString()} power-on hours`, "warning", "smart", "active", 168, 6, 7, true, false);

  // Active warning — container stopped
  const stopped = p.containers.find(c => c.state === "exited");
  if (stopped) a(`Container '${stopped.name}' has been stopped for 3+ days`, "warning", "docker", "active", 96, 1, 48, false, false);

  // Active warning — snoozed
  a(`Container '${p.containers[0]?.name || "plex"}' memory usage above 80%`, "warning", "docker", "active", 48, 2, 24, false, true, hoursAgo(-4));

  // Resolved critical — UPS
  a("UPS switched to battery power — mains restored after 12 min", "critical", "ups", "resolved", 360, 359.8, 1, true, false);

  // Resolved warning — temperature
  a("NVMe temperature exceeded 50°C threshold", "warning", "smart", "resolved", 240, 192, 3, true, false);

  // Resolved warning — network
  a("Network interface eth0 link flapped (2.3s outage)", "warning", "network", "resolved", 168, 167.99, 1, true, false);

  // Resolved info — scan
  a("Diagnostic scan completed — 3 warnings found", "info", "system", "resolved", 2, 1.95, 1, false, false);

  return alerts;
}

function buildIncidents(p: PlatformProfile): unknown[] {
  return [
    { id: "inc-001", type: "service_check", severity: "critical", title: "Service check failed: Pi-hole Admin", description: "Pi-hole at http://10.0.1.53/admin unreachable for 60 min. 12 consecutive failures. DNS resolution affected for Pi-hole clients.", timestamp: hoursAgo(1), resolved: false, resolved_at: null, source: "service_checks", affected_entity: "service:Pi-hole Admin" },
    { id: "inc-002", type: "service_check", severity: "critical", title: "DNS resolution check failed: Local DNS", description: "Local DNS at 10.0.1.53 is not resolving queries. 8 consecutive failures.", timestamp: hoursAgo(0.8), resolved: false, resolved_at: null, source: "service_checks", affected_entity: "service:Local DNS (Pi-hole)" },
    { id: "inc-003", type: "threshold_breach", severity: "warning", title: `Disk usage exceeded threshold on ${p.drives[0].label}`, description: `${p.drives[0].label} (${p.drives[0].model}) at ${p.drives[0].usedPct}%. Growth rate ~0.3%/day. Projected full in ~60 days.`, timestamp: hoursAgo(18), resolved: false, resolved_at: null, source: "disk_analyzer", affected_entity: `/dev/${p.drives[0].device}` },
    { id: "inc-004", type: "fleet_event", severity: "warning", title: "Fleet server 'Remote Backup' went offline", description: "Fleet server at http://192.168.50.10:8060 stopped responding 48h ago.", timestamp: hoursAgo(48), resolved: false, resolved_at: null, source: "fleet_poller", affected_entity: "fleet:Remote Backup" },
    { id: "inc-005", type: "container_event", severity: "warning", title: `Container '${p.containers[0]?.name || "plex"}' restarted (OOM killed)`, description: "Exited with code 137 (out of memory). Auto-restarted by Docker.", timestamp: hoursAgo(36), resolved: true, resolved_at: hoursAgo(35.9), source: "docker_monitor", affected_entity: `container:${p.containers[0]?.name || "plex"}` },
    { id: "inc-006", type: "threshold_breach", severity: "warning", title: "NVMe temperature exceeded 50°C", description: "Cache drive hit 53°C during heavy I/O. Normalized to 42°C after load decreased.", timestamp: hoursAgo(96), resolved: true, resolved_at: hoursAgo(94), source: "smart_monitor", affected_entity: `/dev/${p.drives[p.drives.length - 1].device}` },
    { id: "inc-007", type: "network_event", severity: "warning", title: "Network interface eth0 link flapped", description: "Interface down for 2.3s then recovered. Correlated with upstream switch firmware upgrade.", timestamp: hoursAgo(168), resolved: true, resolved_at: hoursAgo(167.99), source: "network_monitor", affected_entity: "interface:eth0" },
    { id: "inc-008", type: "power_event", severity: "critical", title: "UPS switched to battery — mains power lost", description: "Utility power lost at 03:42 AM. UPS ran on battery for 12 min at 35% load before mains restored.", timestamp: hoursAgo(360), resolved: true, resolved_at: hoursAgo(359.8), source: "ups_monitor", affected_entity: "ups:CyberPower CP1500PFCLCD" },
    { id: "inc-009", type: "notification_event", severity: "info", title: "Alert delivered to Discord", description: "Critical alert 'Pi-hole Admin failed' delivered to Discord #nas-alerts. Latency: 245ms.", timestamp: hoursAgo(0.9), resolved: true, resolved_at: hoursAgo(0.89), source: "notifier", affected_entity: "webhook:Discord - #nas-alerts" },
    { id: "inc-010", type: "system_event", severity: "info", title: "Diagnostic scan completed", description: "6-hour scan completed in 4.2s. All subsystems checked. 3 warnings, 2 critical issues found.", timestamp: hoursAgo(2), resolved: true, resolved_at: hoursAgo(1.95), source: "scheduler", affected_entity: "system" },
  ];
}

function buildNotificationLog(): unknown[] {
  return [
    { id: 1, webhook_name: "Discord - #nas-alerts", webhook_type: "discord", status: "success", findings_count: 3, error: "", timestamp: hoursAgo(0.9), latency_ms: 245 },
    { id: 2, webhook_name: "Slack - #infrastructure", webhook_type: "slack", status: "success", findings_count: 1, error: "", timestamp: hoursAgo(0.9), latency_ms: 312 },
    { id: 3, webhook_name: "Ntfy - phone alerts", webhook_type: "ntfy", status: "success", findings_count: 1, error: "", timestamp: hoursAgo(0.9), latency_ms: 189 },
    { id: 4, webhook_name: "Discord - #nas-alerts", webhook_type: "discord", status: "success", findings_count: 2, error: "", timestamp: hoursAgo(6.9), latency_ms: 198 },
    { id: 5, webhook_name: "Discord - #nas-alerts", webhook_type: "discord", status: "failed", findings_count: 4, error: "HTTP 429: rate limited", timestamp: hoursAgo(12.5), latency_ms: 0 },
    { id: 6, webhook_name: "Slack - #infrastructure", webhook_type: "slack", status: "success", findings_count: 1, error: "", timestamp: hoursAgo(24.1), latency_ms: 287 },
    { id: 7, webhook_name: "Ntfy - phone alerts", webhook_type: "ntfy", status: "success", findings_count: 2, error: "", timestamp: hoursAgo(24.2), latency_ms: 201 },
    { id: 8, webhook_name: "Discord - #nas-alerts", webhook_type: "discord", status: "success", findings_count: 5, error: "", timestamp: hoursAgo(48), latency_ms: 178 },
  ];
}

function buildServiceChecks(): unknown[] {
  const now = new Date().toISOString();
  const checks: unknown[] = [];

  // Helper to create a check entry
  const sc = (key: string, name: string, type: string, target: string, up: boolean, baseLat: number, severity: string, failures = 0) => {
    checks.push({
      key, name, type, target,
      status: up ? "up" : "down",
      response_ms: up ? Math.round(clamp(jitter(baseLat, 30, hashStr(key)), 1, baseLat * 3)) : 0,
      consecutive_failures: failures,
      failure_threshold: 5,
      failure_severity: severity,
      checked_at: up ? now : new Date(Date.now() - 300000).toISOString(),
    });
  };

  // ── Ping checks ──
  sc("sc-gateway", "Gateway", "ping", "10.0.1.1", true, 1, "critical");
  sc("sc-dns-cf", "Cloudflare DNS", "ping", "1.1.1.1", true, 12, "critical");
  sc("sc-dns-google", "Google DNS", "ping", "8.8.8.8", true, 18, "info");
  sc("sc-switch", "Core Switch", "ping", "10.0.1.2", true, 1, "warning");

  // ── HTTP checks ──
  sc("sc-nas-doctor", "NAS Doctor", "http", "http://localhost:8060/api/v1/health", true, 8, "critical");
  sc("sc-plex", "Plex Media Server", "http", "http://localhost:32400/web", true, 42, "warning");
  sc("sc-nextcloud", "Nextcloud", "http", "https://cloud.example.com/status.php", true, 185, "warning");
  sc("sc-grafana", "Grafana", "http", "http://localhost:3000/api/health", true, 18, "warning");
  sc("sc-router", "Router Admin", "http", "http://10.0.1.1", true, 5, "warning");
  sc("sc-home-assistant", "Home Assistant", "http", "http://10.0.1.55:8123", true, 35, "warning");
  sc("sc-pihole", "Pi-hole Admin", "http", "http://10.0.1.53/admin", false, 0, "critical", 12);

  // ── TCP checks ──
  sc("sc-ssh", "SSH Server", "tcp", "10.0.1.50:22", true, 3, "critical");
  sc("sc-mariadb", "MariaDB", "tcp", "10.0.1.50:3306", true, 2, "warning");
  sc("sc-redis", "Redis Cache", "tcp", "10.0.1.50:6379", true, 1, "info");
  sc("sc-mqtt", "MQTT Broker", "tcp", "10.0.1.55:1883", true, 2, "warning");

  // ── DNS checks ──
  sc("sc-dns-local", "Local DNS (Pi-hole)", "dns", "10.0.1.53", false, 0, "critical", 8);
  sc("sc-dns-resolve", "Public DNS Resolution", "dns", "1.1.1.1", true, 15, "critical");

  // ── SMB / NFS checks ──
  sc("sc-smb-media", "SMB: Media Share", "smb", "//10.0.1.50/media", true, 8, "warning");
  sc("sc-smb-backup", "SMB: Backup Share", "smb", "//10.0.1.50/backups", true, 12, "warning");
  sc("sc-nfs-docker", "NFS: Docker Volumes", "nfs", "10.0.1.50:/mnt/cache/appdata", true, 5, "critical");

  // ── Fleet-auto-created checks ──
  sc("fleet-http-192.168.1.50:8060", "Fleet: Backup NAS", "http", "http://192.168.1.50:8060/api/v1/health", true, 22, "critical");
  sc("fleet-http-192.168.1.51:8060", "Fleet: Media Server", "http", "http://192.168.1.51:8060/api/v1/health", true, 18, "critical");
  sc("fleet-http-10.0.0.10:8060", "Fleet: Proxmox Node 1", "http", "http://10.0.0.10:8060/api/v1/health", true, 28, "critical");
  sc("fleet-http-192.168.50.10:8060", "Fleet: Remote Backup", "http", "http://192.168.50.10:8060/api/v1/health", false, 0, "critical", 576);

  return checks;
}

function buildZFS(p: PlatformProfile): unknown {
  const isTrueNAS = p.platformName.includes("TrueNAS");
  const isProxmox = p.platformName.includes("Proxmox");

  if (isTrueNAS) {
    const hddDrives = p.drives.filter(d => d.type === "hdd");
    const totalBytes = hddDrives.reduce((s, d) => s + d.sizeGB * 1e9, 0);
    const usedBytes = Math.round(totalBytes * (p.drives[0]?.usedPct || 60) / 100);
    return {
      available: true,
      pools: [{
        name: "tank",
        state: "ONLINE",
        size_bytes: totalBytes,
        allocated_bytes: usedBytes,
        free_bytes: totalBytes - usedBytes,
        fragmentation: Math.round(clamp(jitter(12, 30, hashStr("truenas-zfs")), 1, 35)),
        capacity_percent: Math.round(usedBytes / totalBytes * 100),
        dedup_ratio: 1.0,
        vdevs: [{
          name: "raidz2-0",
          type: "raidz2",
          state: "ONLINE",
          devices: hddDrives.map(d => ({ name: d.device, state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 })),
        }],
        scan: {
          type: "scrub",
          state: "completed",
          start_time: hoursAgo(168),
          end_time: hoursAgo(154),
          errors: 0,
          bytes_scanned: usedBytes,
          bytes_total: usedBytes,
          percent: 100,
        },
      }],
    };
  }

  // Proxmox ZFS
  const ssdDrives = p.drives.filter(d => d.type === "ssd");
  const nvmeDrives = p.drives.filter(d => d.type === "nvme");
  return {
    available: true,
    pools: [{
      name: "rpool",
      state: "ONLINE",
      size_bytes: ssdDrives.reduce((s, d) => s + d.sizeGB * 1e9, 0),
      allocated_bytes: Math.round(ssdDrives.reduce((s, d) => s + d.sizeGB * 1e9 * d.usedPct / 100, 0)),
      free_bytes: Math.round(ssdDrives.reduce((s, d) => s + d.sizeGB * 1e9 * (1 - d.usedPct / 100), 0)),
      fragmentation: 8,
      capacity_percent: ssdDrives[0]?.usedPct || 18,
      dedup_ratio: 1.0,
      vdevs: [{
        name: "mirror-0",
        type: "mirror",
        state: "ONLINE",
        devices: ssdDrives.map(d => ({ name: d.device, state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 })),
      }, ...(nvmeDrives.length >= 2 ? [{
        name: "mirror-1",
        type: "mirror",
        state: "ONLINE",
        devices: nvmeDrives.map(d => ({ name: d.device, state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 })),
      }] : [])],
      scan: {
        type: "scrub",
        state: "completed",
        start_time: hoursAgo(72),
        end_time: hoursAgo(68),
        errors: 0,
        bytes_scanned: Math.round(ssdDrives.reduce((s, d) => s + d.sizeGB * 1e9 * d.usedPct / 100, 0)),
        bytes_total: Math.round(ssdDrives.reduce((s, d) => s + d.sizeGB * 1e9 * d.usedPct / 100, 0)),
        percent: 100,
      },
    }],
  };
}

// ── Widget builders added for #262 ─────────────────────────────
//
// These produce JSON in the exact shape `internal/api/dashboard.go`
// consumes. Field names are pinned by `widget-coverage.test.ts` —
// renaming any of them in the Go binary without updating the feeder
// (or vice-versa) will turn that test RED.

// buildSpeedTest — produces snapshot.speed_test matching
// internal.SpeedTestInfo: {available, latest: SpeedTestResult,
// last_attempt: SpeedTestAttempt}.
// dashboard.go L782 (sections.speedtest) reads
// spd.latest.{download_mbps, upload_mbps, latency_ms, jitter_ms,
// server_name, isp, timestamp} and spd.last_attempt.{status,
// timestamp}. See #210 for the state model.
function buildSpeedTest(p: PlatformProfile): Record<string, unknown> {
  const t = p.speedTest;
  // Jitter the throughput a bit so multiple cron ticks in the same
  // day produce visibly different bars in the chart. Use a stable
  // seed per platform + current 5min slot so the 24h history series
  // can mirror it.
  const seed = hashStr(p.hostname + "-speed");
  const downJ = jitter(t.downloadMbps, 6, seed);
  const upJ = jitter(t.uploadMbps, 8, seed + 1);
  const latJ = jitter(t.latencyMs, 20, seed + 2);
  const jitJ = jitter(t.jitterMs, 40, seed + 3);
  const now = new Date().toISOString();
  return {
    available: true,
    latest: {
      timestamp: now,
      download_mbps: round2(clamp(downJ, t.downloadMbps * 0.7, t.downloadMbps * 1.15)),
      upload_mbps: round2(clamp(upJ, t.uploadMbps * 0.7, t.uploadMbps * 1.15)),
      latency_ms: round2(clamp(latJ, Math.max(0.5, t.latencyMs * 0.5), t.latencyMs * 2)),
      jitter_ms: round2(clamp(jitJ, 0.1, t.jitterMs * 3)),
      server_name: t.serverName,
      server_id: Math.abs(seed) % 100000,
      isp: t.isp,
      external_ip: "203.0.113." + (Math.abs(seed) % 200 + 10),
      result_url: "",
    },
    last_attempt: {
      timestamp: now,
      status: "success",
      error_msg: "",
    },
  };
}

// buildSpeedTestHistory — produces /api/v1/history/speedtest payload.
// The Go endpoint returns an array of {timestamp, download_mbps,
// upload_mbps, latency_ms, jitter_ms, server_name, isp}; the widget's
// mini-chart consumes download_mbps for each point. See
// internal/api/handlers_history.go and dashboard.go L784 ("Speed Test"
// range buttons which call loadSpeedTestChart).
function buildSpeedTestHistory(p: PlatformProfile, hours: number): unknown[] {
  const t = p.speedTest;
  const seed = hashStr(p.hostname + "-speedhist");
  const now = Date.now();
  const points: unknown[] = [];
  // One sample per hour going back `hours`.
  for (let h = hours - 1; h >= 0; h--) {
    const ts = new Date(now - h * 3600000).toISOString();
    // Time-of-day modulation: residential ISPs slow down in the
    // evening, datacentre links are flat.
    const tod = new Date(now - h * 3600000).getUTCHours();
    const eveningDip = (tod >= 18 && tod <= 23) ? 0.85 : 1.0;
    const s = seed + h;
    points.push({
      timestamp: ts,
      download_mbps: round2(clamp(jitter(t.downloadMbps * eveningDip, 10, s), t.downloadMbps * 0.5, t.downloadMbps * 1.15)),
      upload_mbps: round2(clamp(jitter(t.uploadMbps * eveningDip, 12, s + 1), t.uploadMbps * 0.5, t.uploadMbps * 1.15)),
      latency_ms: round2(clamp(jitter(t.latencyMs, 25, s + 2), Math.max(0.5, t.latencyMs * 0.5), t.latencyMs * 2.5)),
      jitter_ms: round2(clamp(jitter(t.jitterMs, 50, s + 3), 0.1, t.jitterMs * 4)),
      server_name: t.serverName,
      isp: t.isp,
    });
  }
  return points;
}

// buildBackup — produces snapshot.backup matching internal.BackupInfo:
// {available, jobs: BackupJob[]}.
// dashboard.go L705 (sections.backup) reads bj.{provider, name, status,
// snapshot_count, size_bytes, last_success, encrypted}.
function buildBackup(p: PlatformProfile, platform: Platform): Record<string, unknown> {
  const nowMs = Date.now();
  const hoursAgoIso = (h: number) => new Date(nowMs - h * 3600000).toISOString();
  // Platform-appropriate repos. Every platform gets ≥2 repos so
  // visual variety works: one healthy + one stale/warning.
  const repos: Record<string, unknown>[] = [];
  if (platform === "unraid") {
    repos.push(
      { provider: "borg", name: "appdata-nightly", repository: "/mnt/user/backups/borg/appdata", status: "ok", snapshot_count: 127, size_bytes: 184_000_000_000, files_count: 215000, last_run: hoursAgoIso(6.2), last_success: hoursAgoIso(6.2), duration_secs: 428, schedule: "0 3 * * *", compression: "zstd", encrypted: true, error_message: "" },
      { provider: "restic", name: "media-weekly", repository: "rclone:b2:tower-media", status: "ok", snapshot_count: 42, size_bytes: 3_400_000_000_000, files_count: 1_820_000, last_run: hoursAgoIso(18.5), last_success: hoursAgoIso(18.5), duration_secs: 7842, schedule: "0 2 * * 0", compression: "auto", encrypted: true, error_message: "" },
      { provider: "duplicati", name: "documents-offsite", repository: "s3://offsite-docs-2026", status: "warning", snapshot_count: 89, size_bytes: 42_000_000_000, files_count: 11500, last_run: hoursAgoIso(56), last_success: hoursAgoIso(56), duration_secs: 610, schedule: "0 4 * * *", compression: "zstd", encrypted: true, error_message: "Last attempt completed with warnings (3 files skipped)" },
    );
  } else if (platform === "synology") {
    repos.push(
      { provider: "restic", name: "home-docs", repository: "/volume1/Backup/restic", status: "ok", snapshot_count: 58, size_bytes: 92_000_000_000, files_count: 87000, last_run: hoursAgoIso(4.8), last_success: hoursAgoIso(4.8), duration_secs: 312, schedule: "0 2 * * *", compression: "auto", encrypted: true, error_message: "" },
      { provider: "borg", name: "photos-archive", repository: "/volume2/borg/photos", status: "stale", snapshot_count: 36, size_bytes: 680_000_000_000, files_count: 220000, last_run: hoursAgoIso(96), last_success: hoursAgoIso(96), duration_secs: 1850, schedule: "0 3 * * 0", compression: "zstd", encrypted: true, error_message: "Last run was 4 days ago — expected weekly" },
    );
  } else if (platform === "truenas") {
    repos.push(
      { provider: "pbs", name: "vm-backups", repository: "pbs-01:datastore1", status: "ok", snapshot_count: 184, size_bytes: 2_100_000_000_000, files_count: 0, last_run: hoursAgoIso(2.1), last_success: hoursAgoIso(2.1), duration_secs: 1240, schedule: "0 */6 * * *", compression: "zstd", encrypted: true, error_message: "" },
      { provider: "restic", name: "jail-configs", repository: "/mnt/tank/backups/restic-jails", status: "ok", snapshot_count: 67, size_bytes: 8_400_000_000, files_count: 9200, last_run: hoursAgoIso(8.3), last_success: hoursAgoIso(8.3), duration_secs: 42, schedule: "0 1 * * *", compression: "auto", encrypted: true, error_message: "" },
      { provider: "rclone", name: "nextcloud-offsite", repository: "rclone:wasabi:truenas-offsite", status: "failed", snapshot_count: 45, size_bytes: 540_000_000_000, files_count: 180000, last_run: hoursAgoIso(12.4), last_success: hoursAgoIso(36), duration_secs: 0, schedule: "0 4 * * *", compression: "none", encrypted: true, error_message: "wasabi: auth token expired — refresh credentials" },
    );
  } else if (platform === "proxmox") {
    repos.push(
      { provider: "pbs", name: "datacenter-vms", repository: "pbs-primary:main", status: "ok", snapshot_count: 412, size_bytes: 5_800_000_000_000, files_count: 0, last_run: hoursAgoIso(1.2), last_success: hoursAgoIso(1.2), duration_secs: 2180, schedule: "0 */4 * * *", compression: "zstd", encrypted: true, error_message: "" },
      { provider: "pbs", name: "offsite-replica", repository: "pbs-remote:datastore2", status: "warning", snapshot_count: 320, size_bytes: 5_700_000_000_000, files_count: 0, last_run: hoursAgoIso(9.1), last_success: hoursAgoIso(9.1), duration_secs: 3600, schedule: "0 5 * * *", compression: "zstd", encrypted: true, error_message: "Replication lag: 8h behind primary" },
    );
  } else {
    // Kubernetes: Velero is the norm, but the demo keeps this
    // widget hidden. Returning available: false here means the
    // dashboard renders the "no backup provider detected" empty
    // state, which is what a Velero-managed cluster would show
    // to NAS Doctor.
    return { available: false, jobs: [] };
  }
  return { available: true, jobs: repos };
}

// buildTopProcesses — produces snapshot.system.top_processes matching
// internal.ProcessInfo: {pid, user, cpu_percent, mem_percent, command,
// container_name, container_id}.
// dashboard.go L1144 (sections.processes) reads p.{command, cpu_percent,
// mem_percent, user, container_name}. Returns 8-10 realistic processes
// matching the platform's container list.
function buildTopProcesses(p: PlatformProfile, platform: Platform): Record<string, unknown>[] {
  const seed = hashStr(platform + "-procs");
  const j = (base: number, pct: number, s: number) => round2(clamp(jitter(base, pct, seed + s), 0.1, 99));
  // Start with host processes common across all platforms, then splice
  // in container-owned processes matching the profile's containers.
  const procs: Record<string, unknown>[] = [];
  let pid = 1000;
  // Host kernel / init
  procs.push({ pid: 1, user: "root", cpu_percent: j(0.3, 50, 1), mem_percent: j(0.1, 30, 2), command: "/sbin/init", container_name: "", container_id: "" });
  // The NAS Doctor binary itself.
  procs.push({ pid: pid++, user: platform === "synology" ? "admin" : "root", cpu_percent: j(1.2, 40, 3), mem_percent: j(0.6, 20, 4), command: "/usr/bin/nas-doctor --http :8060", container_name: "", container_id: "" });
  // dockerd / containerd (not on k8s)
  if (platform !== "kubernetes") {
    procs.push({ pid: pid++, user: "root", cpu_percent: j(1.8, 50, 5), mem_percent: j(2.1, 25, 6), command: "/usr/bin/dockerd -H fd:// --containerd=/run/containerd/containerd.sock", container_name: "", container_id: "" });
  } else {
    procs.push({ pid: pid++, user: "root", cpu_percent: j(3.4, 40, 5), mem_percent: j(2.8, 20, 6), command: "/usr/local/bin/k3s server", container_name: "", container_id: "" });
  }
  // Pick the heaviest running containers from the profile (max 6).
  const running = p.containers.filter((c) => c.state === "running").sort((a, b) => b.cpu + b.mem / 100 - (a.cpu + a.mem / 100)).slice(0, 6);
  for (let i = 0; i < running.length; i++) {
    const c = running[i];
    const cmd = commandFor(c.name, c.image);
    procs.push({
      pid: pid++,
      user: platform === "kubernetes" ? "nonroot" : "root",
      cpu_percent: round2(clamp(jitter(c.cpu, 15, seed + 100 + i), 0.1, 100)),
      mem_percent: round2(clamp(jitter(c.mem / (p.ramGB * 10.24), 12, seed + 200 + i), 0.1, 50)),
      command: cmd,
      container_name: c.name,
      container_id: `${platform.slice(0, 3)}-${i}`,
    });
  }
  // Round out to ~9 entries with a couple of small always-on host
  // processes.
  procs.push({ pid: pid++, user: "root", cpu_percent: j(0.1, 30, 20), mem_percent: j(0.2, 25, 21), command: "/usr/sbin/sshd -D", container_name: "", container_id: "" });
  procs.push({ pid: pid++, user: "root", cpu_percent: j(0.2, 50, 22), mem_percent: j(0.05, 40, 23), command: "/usr/sbin/cron -f", container_name: "", container_id: "" });
  // Sort by cpu_percent descending so the widget's default order is
  // "heaviest at the top".
  procs.sort((a, b) => (b.cpu_percent as number) - (a.cpu_percent as number));
  return procs;
}

function commandFor(name: string, image: string): string {
  // Best-effort realistic command line for a running container.
  // Used by buildTopProcesses to give the Processes widget a
  // recognisable command string.
  if (name === "plex") return "/usr/lib/plexmediaserver/Plex Media Server";
  if (name === "emby") return "/system/EmbyServer -programdata /config";
  if (name.startsWith("nginx")) return "nginx: master process";
  if (name === "home-assistant") return "python3 -m homeassistant --config /config";
  if (name === "grafana") return "/usr/share/grafana/bin/grafana-server";
  if (name === "prometheus") return "/bin/prometheus --config.file=/etc/prometheus/prometheus.yml";
  if (name === "nextcloud") return "apache2 -DFOREGROUND";
  if (name === "minio") return "/usr/bin/minio server /data";
  if (name === "coredns") return "/coredns -conf /etc/coredns/Corefile";
  if (name === "traefik") return "/traefik --providers.kubernetesingress";
  if (name === "longhorn-manager") return "longhorn-manager daemon --engine-image=longhornio/longhorn-engine:v1.7.0";
  if (name === "tdarr") return "node /app/Tdarr_Server/index.js";
  if (name === "wireguard") return "wg-quick up wg0";
  if (name === "radarr") return "/app/bin/Radarr -nobrowser -data=/config";
  if (name === "sonarr") return "/app/bin/Sonarr -nobrowser -data=/config";
  if (name === "sabnzbd") return "python3 /app/SABnzbd.py -s 0.0.0.0:8080";
  if (name === "syncthing") return "/bin/syncthing --no-browser --home=/var/syncthing";
  if (name === "nas-doctor") return "/usr/bin/nas-doctor --http :8060";
  if (name === "portainer") return "/portainer --host=unix:///var/run/docker.sock";
  if (name === "homebridge") return "/usr/local/bin/homebridge -U /homebridge";
  return `${image.split(":")[0].split("/").pop()}`;
}

// buildGPU — produces snapshot.gpu matching internal.GPUInfo:
// {available, gpus: GPUDevice[]}. dashboard.go L613 (sections.gpu)
// consumes gpus[].{name, vendor, driver, usage_percent, temperature_c,
// mem_used_mb, mem_total_mb, power_watts, power_max_watts, fan_percent,
// encoder_percent, decoder_percent}.
function buildGPU(p: PlatformProfile, platform: Platform): Record<string, unknown> {
  const g = p.gpuDevice;
  if (!g) return { available: false, gpus: [] };
  const seed = hashStr(platform + "-gpu");
  const usage = clamp(jitter(g.usagePct, 30, seed), 1, 100);
  const temp = Math.round(clamp(jitter(g.tempC, 15, seed + 1), 35, 88));
  const memUsedMB = Math.round(g.memTotalMB * clamp(jitter(g.memUsedPct / 100, 15, seed + 2), 0.1, 0.95));
  const memPct = round2((memUsedMB / g.memTotalMB) * 100);
  const powerW = round2(clamp(jitter(g.powerMaxW * (usage / 100) * 0.9, 20, seed + 3), 5, g.powerMaxW));
  // Unraid's RTX A2000 is used for Plex transcoding → encoder
  // activity. Proxmox's Tesla P4 is used for compute → no encoder.
  const encoderPct = platform === "unraid" ? round2(clamp(jitter(45, 40, seed + 4), 0, 100)) : 0;
  const decoderPct = platform === "unraid" ? round2(clamp(jitter(28, 40, seed + 5), 0, 100)) : 0;
  return {
    available: true,
    gpus: [{
      index: 0,
      name: g.name,
      vendor: g.vendor,
      driver: g.driver,
      usage_percent: round2(usage),
      mem_used_mb: memUsedMB,
      mem_total_mb: g.memTotalMB,
      mem_percent: memPct,
      temperature_c: temp,
      fan_percent: g.vendor === "nvidia" && g.powerMaxW > 70 ? round2(clamp(jitter(55, 20, seed + 6), 20, 100)) : 0,
      power_watts: powerW,
      power_max_watts: g.powerMaxW,
      clock_mhz: Math.round(clamp(jitter(1800, 10, seed + 7), 1400, 2100)),
      mem_clock_mhz: Math.round(clamp(jitter(7000, 5, seed + 8), 6500, 7500)),
      pcie_bus: "0a:00.0",
      encoder_percent: encoderPct,
      decoder_percent: decoderPct,
    }],
  };
}

function buildFindings(p: PlatformProfile, disks: any[], smart: any[], containers: any[]): unknown[] {
  const findings: any[] = [];
  let idx = 1;
  const f = (sev: string, cat: string, title: string, desc: string, evidence: string[], impact: string, action: string, priority: string) => {
    findings.push({ id: `finding-${idx++}`, severity: sev, category: cat, title, description: desc, evidence, impact, action, priority });
  };

  for (const d of disks) {
    if (d.used_percent > 85) f("critical", "disk", `High disk usage on ${d.label} (${d.used_percent.toFixed(0)}%)`, `${d.label} is ${d.used_percent.toFixed(1)}% full.`, [`Used: ${d.used_percent.toFixed(1)}%`], "Write operations will fail when full.", "Free up space or expand the volume.", "immediate");
    else if (d.used_percent > 70) f("warning", "disk", `Disk usage elevated on ${d.label} (${d.used_percent.toFixed(0)}%)`, `${d.label} is ${d.used_percent.toFixed(1)}% full.`, [`Used: ${d.used_percent.toFixed(1)}%`], "May run out of space within 90 days.", "Monitor usage and plan expansion.", "short-term");
  }
  for (const s of smart) {
    if (s.temperature_c > 48) f("warning", "smart", `Elevated temperature: ${s.model} (${s.temperature_c}°C)`, `${s.device} at ${s.temperature_c}°C.`, [`Temp: ${s.temperature_c}°C`], "Reduces drive lifespan.", "Check airflow and cooling.", "short-term");
    if (s.power_on_hours > 30000) {
      const years = (s.power_on_hours / 8766).toFixed(1);
      f("info", "smart", `Drive aging: ${s.model} — ${years} years (Backblaze AFR: 1.8%)`, `${s.device} has ${s.power_on_hours.toLocaleString()} hours.`, [`Power-on: ${s.power_on_hours.toLocaleString()}h`, `Backblaze AFR: 1.8%`], "Failure risk increases with age.", "Plan proactive replacement.", "medium-term");
    }
  }
  for (const c of containers) { if (c.state === "exited") f("warning", "docker", `Container '${c.name}' is stopped`, `${c.name} has been stopped.`, [`State: exited`], "Service may be unavailable.", "Check logs and restart.", "short-term"); }

  f("critical", "service_check", "Service check failed: Pi-hole DNS (12 consecutive failures)", "Pi-hole at http://10.0.1.53/admin unreachable for 60 min.", ["Failures: 12", "URL: http://10.0.1.53/admin"], "DNS resolution affected.", "Restart the Pi-hole service.", "immediate");
  f("warning", "fleet", "Fleet server 'Remote Backup' is offline", "Remote backup server at http://192.168.50.10:8060 has not responded in 48 hours.", ["Last seen: 48h ago"], "Cannot verify backup integrity.", "Check remote site connectivity.", "short-term");
  f("info", "system", "Diagnostic scan completed", "All subsystems checked.", [`Drives: ${p.drives.length}`, `Containers: ${p.containers.length}`], "No action required.", "Review findings above.", "none");

  return findings;
}

// ── Noise utilities ──
function hash(seed: number): number {
  let h = seed | 0;
  h = ((h >> 16) ^ h) * 0x45d9f3b;
  h = ((h >> 16) ^ h) * 0x45d9f3b;
  h = (h >> 16) ^ h;
  return (h & 0x7fffffff) / 0x7fffffff;
}

function hashStr(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  return Math.abs(h);
}

function jitter(value: number, pctRange: number, seed: number): number {
  const slot = Math.floor(Date.now() / 300000);
  const h = hash(slot * 17 + seed * 53 + Math.round(value * 10));
  return value * (1 + (h - 0.5) * 2 * (pctRange / 100));
}

function clamp(v: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, v));
}

function dayFactor(): number {
  const hour = new Date().getUTCHours();
  return hour >= 8 && hour <= 22 ? 1.3 : hour >= 6 ? 1.1 : 0.7;
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

function refreshData(endpoint: string, data: unknown): unknown {
  switch (endpoint) {
    case "status": return refreshStatus(data as Record<string, unknown>);
    case "snapshot": return refreshSnapshot(data as Record<string, unknown>);
    case "sparklines": return refreshSparklines(data as Record<string, unknown>);
    case "fleet": return refreshFleet(data as unknown[]);
    case "service_checks": return refreshServiceChecks(data as unknown[]);
    case "system_history": case "gpu_history": case "container_history": case "process_history": return refreshHistory(data as unknown[]);
    default: return data;
  }
}

function refreshStatus(d: Record<string, unknown>): Record<string, unknown> {
  return { ...d, last_scan: new Date(Date.now() - 120000).toISOString(), critical_count: Math.round(clamp(jitter(1, 80, 1), 0, 3)), warning_count: Math.round(clamp(jitter(3, 50, 2), 1, 8)), info_count: Math.round(clamp(jitter(4, 40, 3), 2, 10)) };
}

function refreshSnapshot(d: Record<string, unknown>): Record<string, unknown> {
  const now = new Date().toISOString();
  const df = dayFactor();
  const sys = d.system as Record<string, unknown> | undefined;
  if (sys) {
    sys.cpu_usage = round2(clamp(jitter((sys.cpu_usage as number) || 25, 20, 10) * df, 3, 85));
    sys.mem_percent = round2(clamp(jitter((sys.mem_percent as number) || 70, 8, 11), 40, 92));
    sys.io_wait = round2(clamp(jitter((sys.io_wait as number) || 3, 35, 12), 0, 15));
  }
  const smart = d.smart as Array<Record<string, unknown>> | undefined;
  if (smart) { for (let i = 0; i < smart.length; i++) smart[i].temperature_c = Math.round(clamp(jitter((smart[i].temperature_c as number) || 35, 8, 100 + i), 22, 60)); }
  return { ...d, timestamp: now, id: `demo-${Math.floor(Date.now() / 300000)}` };
}

function refreshSparklines(d: Record<string, unknown>): Record<string, unknown> {
  const df = dayFactor();
  const sys = d.system as Array<Record<string, unknown>> | undefined;
  if (sys) { const now = Date.now(); for (let i = 0; i < sys.length; i++) { sys[i].timestamp = new Date(now - (sys.length - 1 - i) * 3600000).toISOString(); sys[i].cpu = round2(clamp(jitter((sys[i].cpu as number) || 25, 15, 500 + i) * df, 3, 85)); } }
  return d;
}

function refreshFleet(data: unknown[]): unknown[] {
  return data.map((s: unknown, i: number) => { const sv = s as Record<string, unknown>; if (sv.online) sv.last_poll = new Date(Date.now() - 10000 - i * 5000).toISOString(); return sv; });
}

function refreshServiceChecks(data: unknown[]): unknown[] {
  return data.map((c: unknown, i: number) => { const ch = c as Record<string, unknown>; if (ch.status === "up") { ch.response_ms = Math.round(clamp(jitter((ch.response_ms as number) || 20, 30, 700 + i), 1, 500)); ch.checked_at = new Date().toISOString(); } return ch; });
}

function refreshHistory(data: unknown[]): unknown[] {
  if (!Array.isArray(data) || data.length === 0) return data;
  const items = data as Array<Record<string, unknown>>;
  const lastTs = new Date(items[items.length - 1].timestamp as string).getTime();
  const offset = Date.now() - lastTs;
  return items.map((item, i) => {
    const shifted = { ...item };
    shifted.timestamp = new Date(new Date(item.timestamp as string).getTime() + offset).toISOString();
    for (const key of Object.keys(shifted)) { if (key !== "timestamp" && key !== "name" && key !== "image" && key !== "gpu_index" && typeof shifted[key] === "number") shifted[key] = round2(clamp(jitter(shifted[key] as number, 10, 800 + i), 0, 1e15)); }
    return shifted;
  });
}
