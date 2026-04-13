/**
 * Platform profiles for the demo.
 * Each platform has different drives, containers, sections, and system characteristics.
 */

export type Platform = "unraid" | "synology" | "proxmox" | "kubernetes";

export function getPlatformFromRequest(request: Request, url: URL): Platform {
  // Check query param first
  const qp = url.searchParams.get("platform");
  if (qp && isValidPlatform(qp)) return qp;

  // Check cookie
  const cookie = request.headers.get("Cookie") || "";
  const match = cookie.match(/nas_demo_platform=(\w+)/);
  if (match && isValidPlatform(match[1])) return match[1] as Platform;

  return "unraid"; // default
}

function isValidPlatform(s: string): s is Platform {
  return ["unraid", "synology", "proxmox", "kubernetes"].includes(s);
}

export interface PlatformProfile {
  hostname: string;
  platform: string;
  osVersion: string;
  cpuModel: string;
  cpuCores: number;
  ramGB: number;
  uptimeSecs: number;
  drives: DriveProfile[];
  containers: ContainerProfile[];
  hasZFS: boolean;
  hasUPS: boolean;
  hasParity: boolean;
  hasProxmox: boolean;
  hasKubernetes: boolean;
  hasTunnels: boolean;
  hasTailscale: boolean;
  hasGPU: boolean;
}

export interface DriveProfile {
  device: string;
  model: string;
  serial: string;
  sizeGB: number;
  type: "hdd" | "ssd" | "nvme";
  mountPoint: string;
  label: string;
  usedPct: number;
  tempC: number;
  powerOnHours: number;
  healthPassed: boolean;
}

export interface ContainerProfile {
  id: string;
  name: string;
  image: string;
  state: "running" | "exited";
  baseCPU: number;
  baseMem: number;
  memPct: number;
  netIn: number;
  netOut: number;
  blockRead: number;
  blockWrite: number;
}

export const PROFILES: Record<Platform, PlatformProfile> = {
  unraid: {
    hostname: "unraid-tower",
    platform: "Unraid 7.0.1",
    osVersion: "6.12.13",
    cpuModel: "AMD Ryzen 9 5950X",
    cpuCores: 16,
    ramGB: 64,
    uptimeSecs: 30 * 86400,
    drives: [
      { device: "sda", model: "WDC WD180EDGZ", serial: "WD-WX12345678", sizeGB: 18000, type: "hdd", mountPoint: "/mnt/disk1", label: "Disk 1", usedPct: 72, tempC: 35, powerOnHours: 28000, healthPassed: true },
      { device: "sdb", model: "WDC WD180EDGZ", serial: "WD-WX23456789", sizeGB: 18000, type: "hdd", mountPoint: "/mnt/disk2", label: "Disk 2", usedPct: 65, tempC: 34, powerOnHours: 28000, healthPassed: true },
      { device: "sdc", model: "Seagate IronWolf 16TB", serial: "ST-ZL34567890", sizeGB: 16000, type: "hdd", mountPoint: "/mnt/disk3", label: "Disk 3", usedPct: 88, tempC: 38, powerOnHours: 42000, healthPassed: true },
      { device: "sdd", model: "Seagate IronWolf 16TB", serial: "ST-ZL45678901", sizeGB: 16000, type: "hdd", mountPoint: "/mnt/disk4", label: "Disk 4", usedPct: 45, tempC: 36, powerOnHours: 42000, healthPassed: true },
      { device: "sde", model: "WDC WD180EDGZ", serial: "WD-WX56789012", sizeGB: 18000, type: "hdd", mountPoint: "/mnt/parity", label: "Parity", usedPct: 0, tempC: 37, powerOnHours: 28000, healthPassed: true },
      { device: "nvme0n1", model: "Samsung 990 Pro 2TB", serial: "S6XNNS0T123456", sizeGB: 2000, type: "nvme", mountPoint: "/mnt/cache", label: "Cache", usedPct: 42, tempC: 45, powerOnHours: 8000, healthPassed: true },
    ],
    containers: [
      { id: "a1b2c3", name: "plex", image: "linuxserver/plex:latest", state: "running", baseCPU: 12.3, baseMem: 1340, memPct: 4.1, netIn: 154e9, netOut: 892e9, blockRead: 1099e9, blockWrite: 52e9 },
      { id: "d4e5f6", name: "emby", image: "emby/embyserver:latest", state: "running", baseCPU: 8.7, baseMem: 2028, memPct: 6.2, netIn: 98e9, netOut: 543e9, blockRead: 824e9, blockWrite: 41e9 },
      { id: "g7h8i9", name: "tdarr", image: "haveagitgat/tdarr:latest", state: "running", baseCPU: 5.4, baseMem: 916, memPct: 2.8, netIn: 12e9, netOut: 9e9, blockRead: 2199e9, blockWrite: 1649e9 },
      { id: "j0k1l2", name: "nginx-proxy", image: "nginx:alpine", state: "running", baseCPU: 0.3, baseMem: 42, memPct: 0.1, netIn: 456e9, netOut: 567e9, blockRead: 104e6, blockWrite: 52e6 },
      { id: "m3n4o5", name: "wireguard", image: "linuxserver/wireguard:latest", state: "running", baseCPU: 0.1, baseMem: 28, memPct: 0.1, netIn: 234e9, netOut: 198e9, blockRead: 10e6, blockWrite: 5e6 },
      { id: "p6q7r8", name: "home-assistant", image: "homeassistant/home-assistant:latest", state: "running", baseCPU: 3.2, baseMem: 512, memPct: 1.6, netIn: 8e9, netOut: 4e9, blockRead: 21e9, blockWrite: 10e9 },
      { id: "s9t0u1", name: "grafana", image: "grafana/grafana:latest", state: "running", baseCPU: 1.1, baseMem: 186, memPct: 0.6, netIn: 5e9, netOut: 12e9, blockRead: 1e9, blockWrite: 536e6 },
      { id: "v2w3x4", name: "radarr", image: "linuxserver/radarr:latest", state: "running", baseCPU: 0.4, baseMem: 312, memPct: 1.0, netIn: 2e9, netOut: 1e9, blockRead: 2e9, blockWrite: 1e9 },
      { id: "y5z6a7", name: "sonarr", image: "linuxserver/sonarr:latest", state: "running", baseCPU: 0.3, baseMem: 298, memPct: 0.9, netIn: 2e9, netOut: 1e9, blockRead: 2e9, blockWrite: 1e9 },
      { id: "b8c9d0", name: "sabnzbd", image: "linuxserver/sabnzbd:latest", state: "running", baseCPU: 0.1, baseMem: 95, memPct: 0.3, netIn: 43e9, netOut: 2e9, blockRead: 549e9, blockWrite: 274e9 },
      { id: "e1f2g3", name: "pihole", image: "pihole/pihole:latest", state: "exited", baseCPU: 0, baseMem: 0, memPct: 0, netIn: 0, netOut: 0, blockRead: 0, blockWrite: 0 },
    ],
    hasZFS: false,
    hasUPS: true,
    hasParity: true,
    hasProxmox: false,
    hasKubernetes: false,
    hasTunnels: true,
    hasTailscale: true,
    hasGPU: true,
  },

  synology: {
    hostname: "synology-nas",
    platform: "Synology DSM 7.2.2",
    osVersion: "DSM 7.2.2-72806",
    cpuModel: "Intel Celeron J4125",
    cpuCores: 4,
    ramGB: 8,
    uptimeSecs: 90 * 86400,
    drives: [
      { device: "sata1", model: "Seagate IronWolf 8TB", serial: "ST-ZA12345678", sizeGB: 8000, type: "hdd", mountPoint: "/volume1", label: "Volume 1", usedPct: 78, tempC: 36, powerOnHours: 52000, healthPassed: true },
      { device: "sata2", model: "Seagate IronWolf 8TB", serial: "ST-ZA23456789", sizeGB: 8000, type: "hdd", mountPoint: "/volume1", label: "Volume 1 (RAID)", usedPct: 78, tempC: 37, powerOnHours: 52000, healthPassed: true },
      { device: "sata3", model: "WDC WD80EFZZ", serial: "WD-CA34567890", sizeGB: 8000, type: "hdd", mountPoint: "/volume2", label: "Volume 2", usedPct: 45, tempC: 35, powerOnHours: 18000, healthPassed: true },
      { device: "nvme0n1", model: "Samsung 970 EVO Plus 500GB", serial: "S4EWNS0M123456", sizeGB: 500, type: "nvme", mountPoint: "/volume1/@docker", label: "SSD Cache", usedPct: 61, tempC: 48, powerOnHours: 12000, healthPassed: true },
    ],
    containers: [
      { id: "syn01", name: "synology-photos", image: "synology/photos:latest", state: "running", baseCPU: 2.1, baseMem: 384, memPct: 4.8, netIn: 5e9, netOut: 12e9, blockRead: 45e9, blockWrite: 8e9 },
      { id: "syn02", name: "synology-drive", image: "synology/drive:latest", state: "running", baseCPU: 1.5, baseMem: 256, memPct: 3.2, netIn: 18e9, netOut: 22e9, blockRead: 30e9, blockWrite: 15e9 },
      { id: "syn03", name: "plex", image: "linuxserver/plex:latest", state: "running", baseCPU: 8.5, baseMem: 768, memPct: 9.6, netIn: 80e9, netOut: 400e9, blockRead: 500e9, blockWrite: 20e9 },
      { id: "syn04", name: "homebridge", image: "homebridge/homebridge:latest", state: "running", baseCPU: 0.8, baseMem: 128, memPct: 1.6, netIn: 1e9, netOut: 0.5e9, blockRead: 2e9, blockWrite: 1e9 },
      { id: "syn05", name: "watchtower", image: "containrrr/watchtower:latest", state: "running", baseCPU: 0.1, baseMem: 32, memPct: 0.4, netIn: 0.2e9, netOut: 0.1e9, blockRead: 0.5e9, blockWrite: 0.1e9 },
    ],
    hasZFS: false,
    hasUPS: true,
    hasParity: false,
    hasProxmox: false,
    hasKubernetes: false,
    hasTunnels: false,
    hasTailscale: true,
    hasGPU: false,
  },

  proxmox: {
    hostname: "pve-node01",
    platform: "Proxmox VE 8.3.2",
    osVersion: "Debian 12.8",
    cpuModel: "Intel Xeon E-2388G",
    cpuCores: 8,
    ramGB: 128,
    uptimeSecs: 45 * 86400,
    drives: [
      { device: "sda", model: "Samsung PM893 960GB", serial: "S6XNNS0T567890", sizeGB: 960, type: "ssd", mountPoint: "/", label: "Boot SSD", usedPct: 18, tempC: 32, powerOnHours: 15000, healthPassed: true },
      { device: "sdb", model: "Samsung PM893 960GB", serial: "S6XNNS0T678901", sizeGB: 960, type: "ssd", mountPoint: "/", label: "Boot SSD Mirror", usedPct: 18, tempC: 33, powerOnHours: 15000, healthPassed: true },
      { device: "nvme0n1", model: "Samsung 990 Pro 4TB", serial: "S6XNNS0T789012", sizeGB: 4000, type: "nvme", mountPoint: "/mnt/vm-storage", label: "VM Storage", usedPct: 62, tempC: 42, powerOnHours: 8000, healthPassed: true },
      { device: "nvme1n1", model: "Samsung 990 Pro 4TB", serial: "S6XNNS0T890123", sizeGB: 4000, type: "nvme", mountPoint: "/mnt/vm-storage", label: "VM Storage Mirror", usedPct: 62, tempC: 43, powerOnHours: 8000, healthPassed: true },
    ],
    containers: [
      { id: "pve01", name: "nas-doctor", image: "ghcr.io/mcdays94/nas-doctor:latest", state: "running", baseCPU: 0.5, baseMem: 64, memPct: 0.05, netIn: 1e9, netOut: 2e9, blockRead: 5e9, blockWrite: 2e9 },
      { id: "pve02", name: "traefik", image: "traefik:v3.0", state: "running", baseCPU: 0.3, baseMem: 48, memPct: 0.04, netIn: 120e9, netOut: 180e9, blockRead: 0.5e9, blockWrite: 0.2e9 },
      { id: "pve03", name: "portainer", image: "portainer/portainer-ce:latest", state: "running", baseCPU: 0.2, baseMem: 96, memPct: 0.07, netIn: 0.8e9, netOut: 1.5e9, blockRead: 3e9, blockWrite: 1e9 },
    ],
    hasZFS: true,
    hasUPS: true,
    hasParity: false,
    hasProxmox: true,
    hasKubernetes: false,
    hasTunnels: true,
    hasTailscale: false,
    hasGPU: true,
  },

  kubernetes: {
    hostname: "k3s-master-01",
    platform: "K3s v1.31.3+k3s1",
    osVersion: "Ubuntu 24.04 LTS",
    cpuModel: "AMD EPYC 7543P",
    cpuCores: 32,
    ramGB: 256,
    uptimeSecs: 60 * 86400,
    drives: [
      { device: "sda", model: "Samsung PM9A3 3.84TB", serial: "S6XNNS0T901234", sizeGB: 3840, type: "nvme", mountPoint: "/", label: "System", usedPct: 12, tempC: 38, powerOnHours: 6000, healthPassed: true },
      { device: "sdb", model: "Samsung PM9A3 3.84TB", serial: "S6XNNS0T012345", sizeGB: 3840, type: "nvme", mountPoint: "/var/lib/longhorn", label: "Longhorn Storage", usedPct: 54, tempC: 40, powerOnHours: 6000, healthPassed: true },
    ],
    containers: [
      { id: "k8s01", name: "coredns", image: "rancher/mirrored-coredns-coredns:1.11.3", state: "running", baseCPU: 0.2, baseMem: 32, memPct: 0.01, netIn: 50e9, netOut: 48e9, blockRead: 0.1e9, blockWrite: 0.05e9 },
      { id: "k8s02", name: "traefik", image: "rancher/mirrored-library-traefik:2.11.0", state: "running", baseCPU: 0.4, baseMem: 64, memPct: 0.02, netIn: 200e9, netOut: 250e9, blockRead: 1e9, blockWrite: 0.5e9 },
      { id: "k8s03", name: "longhorn-manager", image: "longhornio/longhorn-manager:v1.7.0", state: "running", baseCPU: 1.2, baseMem: 256, memPct: 0.1, netIn: 30e9, netOut: 35e9, blockRead: 80e9, blockWrite: 60e9 },
      { id: "k8s04", name: "nas-doctor", image: "ghcr.io/mcdays94/nas-doctor:latest", state: "running", baseCPU: 0.3, baseMem: 48, memPct: 0.02, netIn: 1e9, netOut: 2e9, blockRead: 3e9, blockWrite: 1e9 },
    ],
    hasZFS: false,
    hasUPS: false,
    hasParity: false,
    hasProxmox: false,
    hasKubernetes: true,
    hasTunnels: false,
    hasTailscale: true,
    hasGPU: false,
  },
};
