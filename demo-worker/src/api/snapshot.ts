import { Platform, PROFILES } from "../data/platforms";
import { hash, jitter, clamp, dayFactor, hoursAgo } from "../data/noise";

export function generateSnapshot(platform: Platform) {
  const p = PROFILES[platform];
  const now = new Date().toISOString();
  const df = dayFactor();

  // ── System ──
  const baseCpu = 25 * df;
  const cpuUsage = clamp(jitter(baseCpu, 20, 1), 3, 95);
  const memUsedGB = clamp(jitter(p.ramGB * 0.72, 8, 2), p.ramGB * 0.3, p.ramGB * 0.95);
  const memPercent = (memUsedGB / p.ramGB) * 100;
  const swapUsedGB = clamp(jitter(0.4, 50, 3), 0, 2);
  const swapTotalGB = 4;

  const system = {
    hostname: p.hostname,
    platform: p.platform,
    os_version: p.osVersion,
    cpu_usage: round2(cpuUsage),
    mem_used_gb: round2(memUsedGB),
    mem_total_gb: p.ramGB,
    mem_percent: round2(memPercent),
    swap_used_gb: round2(swapUsedGB),
    swap_total_gb: swapTotalGB,
    swap_percent: round2((swapUsedGB / swapTotalGB) * 100),
    load_1: round2(clamp(jitter(p.cpuCores * 0.3 * df, 25, 10), 0.1, p.cpuCores * 2)),
    load_5: round2(clamp(jitter(p.cpuCores * 0.25 * df, 20, 11), 0.1, p.cpuCores * 1.5)),
    load_15: round2(clamp(jitter(p.cpuCores * 0.2 * df, 15, 12), 0.1, p.cpuCores * 1.2)),
    io_wait: round2(clamp(jitter(2.5, 40, 13), 0, 15)),
    uptime_seconds: p.uptimeSecs + Math.floor(Date.now() / 1000) % 86400,
    cpu_model: p.cpuModel,
    cpu_cores: p.cpuCores,
  };

  // ── Disks ──
  const disks = p.drives.map((d, i) => {
    const usedPct = clamp(jitter(d.usedPct, 3, 100 + i), 0, 99);
    const usedGB = round2((usedPct / 100) * d.sizeGB);
    return {
      device: `/dev/${d.device}`,
      mount_point: d.mountPoint,
      label: d.label,
      fs_type: d.type === "nvme" ? "ext4" : "xfs",
      total_gb: round2(d.sizeGB),
      used_gb: usedGB,
      free_gb: round2(d.sizeGB - usedGB),
      used_percent: round2(usedPct),
    };
  });

  // ── SMART ──
  const smart = p.drives.map((d, i) => ({
    device: `/dev/${d.device}`,
    model: d.model,
    serial: d.serial,
    firmware: "FW01",
    type: d.type,
    health_passed: d.healthPassed,
    temperature_c: Math.round(clamp(jitter(d.tempC, 8, 200 + i), 20, 65)),
    power_on_hours: d.powerOnHours + Math.floor(Date.now() / 3600000) % 100,
    power_cycle_count: Math.floor(d.powerOnHours / 2000) + 12,
    reallocated_sectors: d.healthPassed ? 0 : 8,
    pending_sectors: 0,
    offline_uncorrectable: 0,
    udma_crc_errors: 0,
    wear_leveling: d.type !== "hdd" ? Math.round(clamp(100 - d.powerOnHours / 500, 70, 100)) : undefined,
    reads_gb: round2(jitter(d.powerOnHours * 0.8, 10, 300 + i)),
    writes_gb: round2(jitter(d.powerOnHours * 0.3, 10, 400 + i)),
  }));

  // ── Docker ──
  const docker = {
    available: true,
    version: "24.0.7",
    containers: p.containers.map((c, i) => ({
      id: c.id,
      name: c.name,
      image: c.image,
      state: c.state,
      status: c.state === "running" ? "Up 14 days" : "Exited (0) 3 days ago",
      created: hoursAgo(14 * 24),
      cpu_percent: c.state === "running" ? round2(clamp(jitter(c.baseCPU * df, 25, 500 + i), 0, 100)) : 0,
      mem_mb: c.state === "running" ? round2(clamp(jitter(c.baseMem, 15, 600 + i), 10, c.baseMem * 2)) : 0,
      mem_percent: c.state === "running" ? round2(clamp(jitter(c.memPct, 20, 700 + i), 0, 50)) : 0,
      net_in_bytes: c.state === "running" ? Math.round(jitter(c.netIn, 10, 800 + i)) : 0,
      net_out_bytes: c.state === "running" ? Math.round(jitter(c.netOut, 10, 900 + i)) : 0,
      block_read_bytes: c.state === "running" ? Math.round(jitter(c.blockRead, 10, 1000 + i)) : 0,
      block_write_bytes: c.state === "running" ? Math.round(jitter(c.blockWrite, 10, 1100 + i)) : 0,
    })),
  };

  // ── Network ──
  const network = {
    interfaces: [
      {
        name: "eth0",
        state: "up",
        speed_mbps: 2500,
        mac: "00:1a:2b:3c:4d:5e",
        ipv4: "10.0.1.50",
        ipv6: "fe80::21a:2bff:fe3c:4d5e",
        rx_bytes: Math.round(jitter(458e9, 5, 1200)),
        tx_bytes: Math.round(jitter(312e9, 5, 1201)),
        rx_errors: 0,
        tx_errors: 0,
      },
      {
        name: "br0",
        state: "up",
        speed_mbps: 2500,
        mac: "00:1a:2b:3c:4d:5f",
        ipv4: "10.0.1.51",
        ipv6: "",
        rx_bytes: Math.round(jitter(120e9, 5, 1202)),
        tx_bytes: Math.round(jitter(85e9, 5, 1203)),
        rx_errors: 0,
        tx_errors: 0,
      },
      {
        name: "wlan0",
        state: "down",
        speed_mbps: 0,
        mac: "00:1a:2b:3c:4d:60",
        ipv4: "",
        ipv6: "",
        rx_bytes: 0,
        tx_bytes: 0,
        rx_errors: 0,
        tx_errors: 0,
      },
    ],
  };

  // ── UPS ──
  const ups = p.hasUPS
    ? {
        available: true,
        name: "CyberPower CP1500PFCLCD",
        status: "OL",
        battery_charge: Math.round(clamp(jitter(98, 2, 1300), 90, 100)),
        battery_voltage: round2(clamp(jitter(13.4, 2, 1301), 12.8, 14.0)),
        input_voltage: round2(clamp(jitter(121.5, 3, 1302), 110, 130)),
        output_voltage: round2(clamp(jitter(120.0, 2, 1303), 118, 122)),
        load_percent: Math.round(clamp(jitter(35, 15, 1304), 10, 60)),
        runtime_seconds: 2700,
        nominal_power: 1000,
      }
    : { available: false };

  // ── ZFS ──
  const zfs = p.hasZFS
    ? {
        available: true,
        pools: [
          {
            name: "rpool",
            state: "ONLINE",
            size_bytes: 960 * 1e9 * 2,
            allocated_bytes: Math.round(jitter(960 * 1e9 * 0.35, 5, 1400)),
            free_bytes: Math.round(960 * 1e9 * 2 * 0.65),
            fragmentation: Math.round(clamp(jitter(8, 30, 1401), 1, 30)),
            capacity_percent: Math.round(clamp(jitter(35, 10, 1402), 10, 80)),
            dedup_ratio: 1.0,
            vdevs: [
              {
                name: "mirror-0",
                type: "mirror",
                state: "ONLINE",
                devices: [
                  { name: "sda", state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 },
                  { name: "sdb", state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 },
                ],
              },
              {
                name: "mirror-1",
                type: "mirror",
                state: "ONLINE",
                devices: [
                  { name: "nvme0n1", state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 },
                  { name: "nvme1n1", state: "ONLINE", read_errors: 0, write_errors: 0, checksum_errors: 0 },
                ],
              },
            ],
            scan: {
              type: "scrub",
              state: "completed",
              start_time: hoursAgo(72),
              end_time: hoursAgo(68),
              errors: 0,
              bytes_scanned: 960 * 1e9 * 0.7,
              bytes_total: 960 * 1e9 * 0.7,
              percent: 100,
            },
          },
        ],
      }
    : { available: false, pools: [] };

  // ── GPU ──
  const gpu = p.hasGPU
    ? {
        available: true,
        devices: [
          {
            index: 0,
            name: "NVIDIA GeForce RTX 4060",
            vendor: "nvidia",
            driver_version: "550.120",
            gpu_usage_percent: Math.round(clamp(jitter(28 * df, 30, 1500), 0, 100)),
            temperature_c: Math.round(clamp(jitter(52, 10, 1501), 30, 85)),
            mem_used_mb: Math.round(clamp(jitter(2048, 20, 1502), 256, 7800)),
            mem_total_mb: 8192,
            mem_percent: 0,
            power_watts: round2(clamp(jitter(85, 15, 1503), 15, 170)),
            power_limit_watts: 170,
            fan_speed_percent: Math.round(clamp(jitter(45, 20, 1504), 0, 100)),
            encoder_percent: Math.round(clamp(jitter(15 * df, 40, 1505), 0, 100)),
            decoder_percent: Math.round(clamp(jitter(8 * df, 40, 1506), 0, 100)),
            pci_bus: "0000:01:00.0",
          },
          {
            index: 1,
            name: "Intel UHD Graphics 730",
            vendor: "intel",
            driver_version: "i915",
            gpu_usage_percent: Math.round(clamp(jitter(12, 30, 1510), 0, 100)),
            temperature_c: Math.round(clamp(jitter(42, 10, 1511), 25, 75)),
            mem_used_mb: Math.round(clamp(jitter(128, 25, 1512), 32, 512)),
            mem_total_mb: 512,
            mem_percent: 0,
            power_watts: round2(clamp(jitter(12, 20, 1513), 3, 25)),
            power_limit_watts: 25,
            fan_speed_percent: 0,
            encoder_percent: Math.round(clamp(jitter(5, 50, 1515), 0, 100)),
            decoder_percent: Math.round(clamp(jitter(3, 50, 1516), 0, 100)),
            pci_bus: "0000:00:02.0",
          },
        ],
      }
    : { available: false, devices: [] };

  // Fix mem_percent for GPU devices
  if (gpu.available && gpu.devices) {
    for (const dev of gpu.devices) {
      dev.mem_percent = round2((dev.mem_used_mb / dev.mem_total_mb) * 100);
    }
  }

  // ── Parity ──
  const parity = p.hasParity
    ? {
        available: true,
        status: "idle",
        history: Array.from({ length: 5 }, (_, i) => {
          const startH = (i + 1) * 168;
          const durationH = 12 + Math.floor(hash(i * 99) * 8);
          const errors = i === 0 ? 0 : Math.floor(hash(i * 77) * 2);
          return {
            start_time: hoursAgo(startH),
            end_time: hoursAgo(startH - durationH),
            duration_seconds: durationH * 3600,
            speed_mb_sec: round2(80 + hash(i * 55) * 40),
            errors: errors,
            status: errors > 0 ? "completed_with_errors" : "completed",
            percent: 100,
            size_bytes: 18000 * 1e9,
          };
        }),
      }
    : { available: false, history: [] };

  // ── Tunnels ──
  const tunnels = p.hasTunnels
    ? {
        available: true,
        cloudflared: [
          {
            tunnel_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
            name: `${p.hostname}-tunnel`,
            status: "healthy",
            created: hoursAgo(30 * 24),
            connections: [
              { colo: "LIS", id: "conn-001", is_pending_reconnect: false, origin_ip: "10.0.1.50", opened_at: hoursAgo(48) },
              { colo: "MAD", id: "conn-002", is_pending_reconnect: false, origin_ip: "10.0.1.50", opened_at: hoursAgo(48) },
            ],
            routes: [
              { hostname: `${p.hostname}.example.com`, service: "http://localhost:8080" },
              { hostname: `plex.example.com`, service: "http://localhost:32400" },
            ],
          },
        ],
      }
    : { available: false, cloudflared: [] };

  // ── Tailscale ──
  const tailscale = p.hasTailscale
    ? {
        available: true,
        self: {
          name: `${p.hostname}.tail1234.ts.net`,
          ip: "100.64.0.1",
          os: platform === "kubernetes" ? "linux" : platform === "synology" ? "linux" : "linux",
          online: true,
          exit_node: false,
        },
        peers: [
          { name: "macbook-pro.tail1234.ts.net", ip: "100.64.0.2", os: "macOS", online: true, exit_node: false, rx_bytes: Math.round(jitter(2.4e9, 10, 1600)), tx_bytes: Math.round(jitter(1.8e9, 10, 1601)), last_seen: hoursAgo(0.01) },
          { name: "iphone-15.tail1234.ts.net", ip: "100.64.0.3", os: "iOS", online: true, exit_node: false, rx_bytes: Math.round(jitter(800e6, 15, 1602)), tx_bytes: Math.round(jitter(420e6, 15, 1603)), last_seen: hoursAgo(0.1) },
          { name: "exit-node-us.tail1234.ts.net", ip: "100.64.0.10", os: "linux", online: true, exit_node: true, rx_bytes: Math.round(jitter(45e9, 8, 1604)), tx_bytes: Math.round(jitter(38e9, 8, 1605)), last_seen: hoursAgo(0.01) },
          { name: "pi-hole.tail1234.ts.net", ip: "100.64.0.20", os: "linux", online: false, exit_node: false, rx_bytes: 0, tx_bytes: 0, last_seen: hoursAgo(72) },
        ],
      }
    : { available: false, peers: [] };

  // ── Proxmox ──
  const proxmox = p.hasProxmox
    ? {
        available: true,
        alias: "PVE Cluster",
        nodes: [
          {
            node: "pve-node01",
            status: "online",
            cpu: round2(clamp(jitter(0.28 * df, 20, 1700), 0.05, 0.95)),
            maxcpu: 8,
            mem: Math.round(jitter(64e9, 10, 1701)),
            maxmem: 128 * 1e9,
            disk: Math.round(jitter(400e9, 5, 1702)),
            maxdisk: 960 * 1e9,
            uptime: p.uptimeSecs,
          },
          {
            node: "pve-node02",
            status: "online",
            cpu: round2(clamp(jitter(0.42 * df, 20, 1710), 0.05, 0.95)),
            maxcpu: 8,
            mem: Math.round(jitter(82e9, 10, 1711)),
            maxmem: 128 * 1e9,
            disk: Math.round(jitter(520e9, 5, 1712)),
            maxdisk: 960 * 1e9,
            uptime: p.uptimeSecs - 86400 * 5,
          },
        ],
        guests: [
          { vmid: 100, name: "docker-host", type: "qemu", status: "running", node: "pve-node01", cpu: round2(jitter(0.15, 25, 1720)), mem: Math.round(jitter(8e9, 10, 1721)), maxmem: 16e9, disk: Math.round(jitter(120e9, 5, 1722)), maxdisk: 200e9, uptime: 30 * 86400 },
          { vmid: 101, name: "truenas-vm", type: "qemu", status: "running", node: "pve-node01", cpu: round2(jitter(0.08, 30, 1730)), mem: Math.round(jitter(24e9, 8, 1731)), maxmem: 32e9, disk: Math.round(jitter(60e9, 5, 1732)), maxdisk: 100e9, uptime: 30 * 86400 },
          { vmid: 102, name: "windows-desktop", type: "qemu", status: "stopped", node: "pve-node01", cpu: 0, mem: 0, maxmem: 16e9, disk: Math.round(jitter(85e9, 5, 1742)), maxdisk: 150e9, uptime: 0 },
          { vmid: 200, name: "pihole-lxc", type: "lxc", status: "running", node: "pve-node02", cpu: round2(jitter(0.02, 40, 1750)), mem: Math.round(jitter(256e6, 15, 1751)), maxmem: 512e6, disk: Math.round(jitter(2e9, 10, 1752)), maxdisk: 8e9, uptime: 45 * 86400 },
          { vmid: 201, name: "nginx-proxy-lxc", type: "lxc", status: "running", node: "pve-node02", cpu: round2(jitter(0.03, 35, 1760)), mem: Math.round(jitter(128e6, 15, 1761)), maxmem: 256e6, disk: Math.round(jitter(1e9, 10, 1762)), maxdisk: 4e9, uptime: 45 * 86400 },
          { vmid: 202, name: "dev-env-lxc", type: "lxc", status: "running", node: "pve-node02", cpu: round2(jitter(0.12, 25, 1770)), mem: Math.round(jitter(2e9, 10, 1771)), maxmem: 4e9, disk: Math.round(jitter(15e9, 8, 1772)), maxdisk: 30e9, uptime: 20 * 86400 },
        ],
        storage: [
          { storage: "local", type: "dir", content: "images,rootdir", total: 960e9, used: Math.round(jitter(180e9, 5, 1780)), avail: 780e9, active: true, shared: false, node: "pve-node01" },
          { storage: "local-zfs", type: "zfspool", content: "images,rootdir", total: 4000e9, used: Math.round(jitter(2480e9, 5, 1781)), avail: 1520e9, active: true, shared: false, node: "pve-node01" },
          { storage: "ceph-pool", type: "rbd", content: "images", total: 8000e9, used: Math.round(jitter(3200e9, 5, 1782)), avail: 4800e9, active: true, shared: true, node: "" },
        ],
        ha: {
          groups: [{ group: "ha-group-1", nodes: "pve-node01,pve-node02", restricted: false, nofailback: false }],
          resources: [
            { sid: "vm:100", state: "started", group: "ha-group-1", node: "pve-node01", status: "active" },
            { sid: "vm:101", state: "started", group: "ha-group-1", node: "pve-node01", status: "active" },
          ],
        },
        tasks: [
          { upid: "UPID:pve-node01:00001234:0A1B2C3D:00000001:vzdump:100:root@pam:", type: "vzdump", status: "OK", starttime: Math.floor(Date.now() / 1000) - 7200, endtime: Math.floor(Date.now() / 1000) - 3600, node: "pve-node01", user: "root@pam" },
          { upid: "UPID:pve-node02:00005678:0E5F6A7B:00000002:apt-update::root@pam:", type: "apt-update", status: "OK", starttime: Math.floor(Date.now() / 1000) - 14400, endtime: Math.floor(Date.now() / 1000) - 14100, node: "pve-node02", user: "root@pam" },
        ],
      }
    : { available: false };

  // ── Kubernetes ──
  const kubernetes = p.hasKubernetes
    ? {
        available: true,
        alias: "K3s Production",
        cluster_version: "v1.31.3+k3s1",
        nodes: [
          { name: "k3s-master-01", status: "Ready", roles: ["control-plane", "master"], version: "v1.31.3+k3s1", os: "Ubuntu 24.04", cpu_capacity: 32, cpu_usage: round2(clamp(jitter(8.5 * df, 20, 1800), 1, 30)), mem_capacity_gb: 256, mem_usage_gb: round2(clamp(jitter(92, 10, 1801), 40, 220)), pods_count: 42, pods_capacity: 110, conditions: [{ type: "Ready", status: "True" }] },
          { name: "k3s-worker-01", status: "Ready", roles: ["worker"], version: "v1.31.3+k3s1", os: "Ubuntu 24.04", cpu_capacity: 32, cpu_usage: round2(clamp(jitter(14.2 * df, 20, 1810), 2, 30)), mem_capacity_gb: 256, mem_usage_gb: round2(clamp(jitter(118, 10, 1811), 50, 230)), pods_count: 38, pods_capacity: 110, conditions: [{ type: "Ready", status: "True" }] },
          { name: "k3s-worker-02", status: "Ready", roles: ["worker"], version: "v1.31.3+k3s1", os: "Ubuntu 24.04", cpu_capacity: 32, cpu_usage: round2(clamp(jitter(11.8 * df, 20, 1820), 1, 30)), mem_capacity_gb: 256, mem_usage_gb: round2(clamp(jitter(85, 10, 1821), 40, 220)), pods_count: 35, pods_capacity: 110, conditions: [{ type: "Ready", status: "True" }] },
        ],
        pods: [
          makePod("nas-doctor", "monitoring", "Running", "k3s-master-01", 1850),
          makePod("coredns-7db6d8c5d4-abc12", "kube-system", "Running", "k3s-master-01", 1851),
          makePod("traefik-6b8f5d8c9-xyz34", "kube-system", "Running", "k3s-worker-01", 1852),
          makePod("longhorn-manager-abc12", "longhorn-system", "Running", "k3s-worker-01", 1853),
          makePod("longhorn-manager-def34", "longhorn-system", "Running", "k3s-worker-02", 1854),
          makePod("longhorn-manager-ghi56", "longhorn-system", "Running", "k3s-master-01", 1855),
          makePod("prometheus-server-0", "monitoring", "Running", "k3s-worker-01", 1856),
          makePod("grafana-5c9d8f7b6-jkl78", "monitoring", "Running", "k3s-worker-01", 1857),
          makePod("cert-manager-5b9c8d7f6-mno90", "cert-manager", "Running", "k3s-master-01", 1858),
          makePod("metallb-speaker-pqr12", "metallb-system", "Running", "k3s-worker-01", 1859),
          makePod("metallb-speaker-stu34", "metallb-system", "Running", "k3s-worker-02", 1860),
          makePod("app-backend-6f8a9b-vwx56", "default", "Running", "k3s-worker-02", 1861),
        ],
        deployments: [
          { name: "traefik", namespace: "kube-system", replicas: 1, ready_replicas: 1, available_replicas: 1, updated_replicas: 1, conditions: [{ type: "Available", status: "True" }] },
          { name: "coredns", namespace: "kube-system", replicas: 1, ready_replicas: 1, available_replicas: 1, updated_replicas: 1, conditions: [{ type: "Available", status: "True" }] },
          { name: "nas-doctor", namespace: "monitoring", replicas: 1, ready_replicas: 1, available_replicas: 1, updated_replicas: 1, conditions: [{ type: "Available", status: "True" }] },
          { name: "app-backend", namespace: "default", replicas: 2, ready_replicas: 2, available_replicas: 2, updated_replicas: 2, conditions: [{ type: "Available", status: "True" }] },
        ],
        services: [
          { name: "traefik", namespace: "kube-system", type: "LoadBalancer", cluster_ip: "10.43.0.100", external_ip: "10.0.1.200", ports: [{ port: 80, target_port: 8000, protocol: "TCP" }, { port: 443, target_port: 8443, protocol: "TCP" }] },
          { name: "coredns", namespace: "kube-system", type: "ClusterIP", cluster_ip: "10.43.0.10", external_ip: "", ports: [{ port: 53, target_port: 53, protocol: "UDP" }] },
          { name: "nas-doctor", namespace: "monitoring", type: "ClusterIP", cluster_ip: "10.43.0.150", external_ip: "", ports: [{ port: 8080, target_port: 8080, protocol: "TCP" }] },
        ],
        pvcs: [
          { name: "nas-doctor-data", namespace: "monitoring", status: "Bound", volume: "pvc-abc123", storage_class: "longhorn", capacity: "5Gi", access_modes: ["ReadWriteOnce"] },
          { name: "prometheus-data", namespace: "monitoring", status: "Bound", volume: "pvc-def456", storage_class: "longhorn", capacity: "50Gi", access_modes: ["ReadWriteOnce"] },
          { name: "grafana-data", namespace: "monitoring", status: "Bound", volume: "pvc-ghi789", storage_class: "longhorn", capacity: "10Gi", access_modes: ["ReadWriteOnce"] },
        ],
        events: [
          { type: "Normal", reason: "Scheduled", message: "Successfully assigned monitoring/nas-doctor to k3s-master-01", object: "pod/nas-doctor", timestamp: hoursAgo(48) },
          { type: "Normal", reason: "Pulled", message: "Container image already present on machine", object: "pod/nas-doctor", timestamp: hoursAgo(48) },
          { type: "Normal", reason: "Started", message: "Started container nas-doctor", object: "pod/nas-doctor", timestamp: hoursAgo(48) },
          { type: "Warning", reason: "BackOff", message: "Back-off restarting failed container", object: "pod/test-pod-xyz", timestamp: hoursAgo(12) },
        ],
      }
    : { available: false };

  // ── Findings ──
  const findings = generateFindings(platform, disks, smart, docker);

  return {
    id: Math.floor(Date.now() / 21600000),
    timestamp: now,
    system,
    disks,
    smart,
    docker,
    network,
    ups,
    zfs,
    gpu,
    parity,
    tunnels,
    tailscale,
    proxmox,
    kubernetes,
    findings,
  };
}

function makePod(name: string, namespace: string, phase: string, node: string, seed: number) {
  const df = dayFactor();
  return {
    name,
    namespace,
    phase,
    node,
    restart_count: Math.floor(hash(seed) * 3),
    cpu_usage: round2(clamp(jitter(0.15 * df, 30, seed), 0.001, 4)),
    mem_usage_mb: Math.round(clamp(jitter(128, 25, seed + 1), 16, 2048)),
    start_time: hoursAgo(48 + hash(seed + 2) * 240),
    conditions: [{ type: "Ready", status: "True" }],
  };
}

function generateFindings(
  platform: Platform,
  disks: { device: string; label: string; used_percent: number }[],
  smart: { device: string; model: string; temperature_c: number; power_on_hours: number }[],
  docker: { containers: { name: string; state: string; cpu_percent: number; mem_percent: number }[] }
) {
  type Finding = { id: string; severity: string; category: string; title: string; description: string; evidence: string[]; impact: string; action: string; priority: string };
  const findings: Finding[] = [];
  let idx = 1;
  const f = (severity: string, category: string, title: string, description: string, evidence: string[], impact: string, action: string, priority: string) => {
    findings.push({ id: `finding-${idx++}`, severity, category, title, description, evidence, impact, action, priority });
  };

  // ── Disk usage (data-driven) ──
  for (const d of disks) {
    if (d.used_percent > 85) {
      f("critical", "disk", `High disk usage on ${d.label} (${d.used_percent.toFixed(0)}%)`,
        `${d.label} (${d.device}) is ${d.used_percent.toFixed(1)}% full. At current growth rate, the volume will be completely full within 45 days.`,
        [`Used: ${d.used_percent.toFixed(1)}%`, `Device: ${d.device}`],
        "Write operations will fail when the volume is full, potentially causing data corruption in databases and application crashes.",
        "Free up space by removing old snapshots, clearing temp files, or expanding the volume. Consider migrating large datasets to a larger drive.",
        "immediate");
    } else if (d.used_percent > 70) {
      f("warning", "disk", `Disk usage elevated on ${d.label} (${d.used_percent.toFixed(0)}%)`,
        `${d.label} (${d.device}) is ${d.used_percent.toFixed(1)}% full.`,
        [`Used: ${d.used_percent.toFixed(1)}%`, `Device: ${d.device}`],
        "May run out of space within 90 days if growth continues at the current rate.",
        "Monitor disk usage trends and plan for expansion or cleanup.",
        "short-term");
    }
  }

  // ── SMART warnings (data-driven) ──
  for (const s of smart) {
    if (s.temperature_c > 48) {
      f("warning", "smart", `Elevated temperature on ${s.model} (${s.temperature_c}°C)`,
        `${s.device} is running at ${s.temperature_c}°C, above the recommended 45°C threshold for long-term reliability.`,
        [`Temperature: ${s.temperature_c}°C`, `Device: ${s.device}`, `Power-on: ${s.power_on_hours.toLocaleString()}h`],
        "Sustained high temperatures reduce drive lifespan and increase the risk of data errors.",
        "Check case airflow, fan speeds, and ambient temperature. Consider adding a drive cooling fan.",
        "short-term");
    }
    if (s.power_on_hours > 35000) {
      f("info", "smart", `High power-on hours: ${s.model} (${s.power_on_hours.toLocaleString()}h)`,
        `${s.device} has been running for ${s.power_on_hours.toLocaleString()} hours. Drives of this model have a Backblaze annualized failure rate of 1.8% after 40,000 hours.`,
        [`Power-on hours: ${s.power_on_hours.toLocaleString()}`, `Model: ${s.model}`],
        "Risk of unexpected failure increases with age. Data loss possible if no redundancy.",
        "Consider proactive replacement. Ensure backups are current and tested.",
        "medium-term");
    }
  }

  // ── Docker (data-driven) ──
  for (const c of docker.containers) {
    if (c.state === "exited") {
      f("warning", "docker", `Container '${c.name}' is stopped`,
        `Container ${c.name} has been in exited state for over 3 days. It may have crashed or been manually stopped.`,
        [`State: exited`, `Container: ${c.name}`],
        "If this container provides a service (DNS, monitoring, etc.), that service is currently unavailable.",
        "Check container logs with 'docker logs " + c.name + "'. Restart if needed.",
        "short-term");
    }
    if (c.cpu_percent > 80) {
      f("warning", "docker", `High CPU: Container '${c.name}' (${c.cpu_percent.toFixed(0)}%)`,
        `Container ${c.name} is using ${c.cpu_percent.toFixed(1)}% CPU.`,
        [`CPU: ${c.cpu_percent.toFixed(1)}%`, `Memory: ${c.mem_percent.toFixed(1)}%`],
        "May starve other containers and system processes.",
        "Check if the container is healthy. Set CPU limits if needed.",
        "short-term");
    }
  }

  // ── Guaranteed findings (ensure every platform has rich data) ──
  // Always add a critical finding for service check failure
  f("critical", "service_check", "Service check failed: Pi-hole DNS (12 consecutive failures)",
    "Pi-hole DNS at http://10.0.1.53/admin has been unreachable for 60 minutes. DNS resolution may be affected for devices using Pi-hole.",
    ["URL: http://10.0.1.53/admin", "Failures: 12 consecutive", "Last success: 60 min ago"],
    "Devices configured to use Pi-hole for DNS will fail to resolve domains, causing widespread connectivity issues.",
    "Check if the Pi-hole container/VM is running. Verify network connectivity to 10.0.1.53. Restart the service if needed.",
    "immediate");

  // Always add a warning about fleet server offline
  f("warning", "fleet", "Fleet server 'Remote Backup' is offline",
    "The fleet server at http://192.168.50.10:8080 has not responded in 48 hours. Last successful poll was 2 days ago.",
    ["Server: Remote Backup", "URL: http://192.168.50.10:8080", "Last seen: 48h ago"],
    "Cannot monitor the remote backup server's health. Backup integrity is unknown.",
    "Check network connectivity to the remote site. Verify the NAS Doctor instance is running on the backup server.",
    "short-term");

  // Always add an info about successful scan
  f("info", "system", "Diagnostic scan completed successfully",
    "Scheduled 6-hour diagnostic scan completed. All subsystems were checked including SMART, Docker, network, and storage.",
    [`Duration: 4.2s`, `Checks: ${14 + disks.length + smart.length + docker.containers.length}`, `Next scan: in 6h`],
    "No immediate action required. System is being monitored.",
    "Review findings above and address any warnings or critical issues.",
    "none");

  // Always add info about NVMe wear
  const nvme = smart.find(s => s.device.includes("nvme"));
  if (nvme) {
    f("info", "smart", `NVMe wear leveling at ${Math.round(100 - (nvme.power_on_hours || 0) / 500)}% remaining life`,
      `${nvme.device} (${nvme.model}) has used approximately ${Math.round((nvme.power_on_hours || 0) / 500)}% of its rated write endurance.`,
      [`Wear: ${Math.round((nvme.power_on_hours || 0) / 500)}%`, `Power-on: ${(nvme.power_on_hours || 0).toLocaleString()}h`],
      "NVMe drives have a finite write endurance. Monitoring ensures timely replacement.",
      "No action needed at current wear level. Plan replacement when wear exceeds 80%.",
      "long-term");
  }

  return findings;
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}
