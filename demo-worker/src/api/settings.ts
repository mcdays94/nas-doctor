import { Platform, PROFILES } from "../data/platforms";

export function generateSettings(platform: Platform) {
  const p = PROFILES[platform];

  return {
    settings_version: 1,
    api_key: "",
    theme: "midnight",
    icon: "default",
    scan_interval: "6h",
    chart_range_hours: 1,
    cost_per_tb: 22.50,
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
      tailscale: p.hasTailscale,
      proxmox: p.hasProxmox,
      kubernetes: p.hasKubernetes,
      merged_containers: true,
      merged_drives: true,
    },
    section_heights: {},
    notifications: {
      webhooks: [
        {
          name: "Discord - #nas-alerts",
          type: "discord",
          url: "https://discord-webhook.example.com/api/webhooks/DEMO/TOKEN",
          enabled: true,
          severity_filter: ["critical", "warning"],
        },
        {
          name: "Slack - #infrastructure",
          type: "slack",
          url: "https://slack-webhook.example.com/services/DEMO/WEBHOOK/TOKEN",
          enabled: true,
          severity_filter: ["critical"],
        },
        {
          name: "Ntfy - phone alerts",
          type: "ntfy",
          url: "https://ntfy.sh/nas-doctor-alerts",
          enabled: true,
          severity_filter: ["critical"],
        },
      ],
      policies: [
        {
          name: "Critical — immediate",
          severity: "critical",
          cooldown_seconds: 300,
          channels: ["Discord - #nas-alerts", "Slack - #infrastructure", "Ntfy - phone alerts"],
        },
        {
          name: "Warnings — 15min cooldown",
          severity: "warning",
          cooldown_seconds: 900,
          channels: ["Discord - #nas-alerts"],
        },
      ],
      maintenance_windows: [
        {
          name: "Weekly parity check",
          start_hhmm: "02:00",
          end_hhmm: "18:00",
          days: ["sunday"],
          suppress_severity: ["warning", "info"],
        },
      ],
      default_cooldown_sec: 900,
      quiet_hours: {
        enabled: true,
        start_hhmm: "23:00",
        end_hhmm: "07:00",
        timezone: "Europe/London",
        suppress_severity: ["info", "warning"],
      },
    },
    service_checks: {
      checks: [
        { name: "NAS Doctor Dashboard", url: "http://localhost:8080/api/v1/health", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200 },
        { name: "Plex Media Server", url: "http://localhost:32400/web", method: "GET", interval_seconds: 30, timeout_seconds: 10, expected_status: 200 },
        { name: "Router Admin", url: "http://10.0.1.1", method: "GET", interval_seconds: 60, timeout_seconds: 5, expected_status: 200 },
        { name: "Pi-hole DNS", url: "http://10.0.1.53/admin", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200 },
        { name: "Nextcloud", url: "https://cloud.example.com/status.php", method: "GET", interval_seconds: 60, timeout_seconds: 10, expected_status: 200 },
        { name: "Grafana", url: "http://localhost:3000/api/health", method: "GET", interval_seconds: 30, timeout_seconds: 5, expected_status: 200 },
      ],
    },
    log_push: {
      enabled: true,
      type: "loki",
      url: "http://loki.local:3100/loki/api/v1/push",
      labels: { job: "nas-doctor", instance: p.hostname },
    },
    retention: {
      snapshot_days: 30,
      notify_log_days: 30,
    },
    backup: {
      enabled: true,
      interval: "24h",
      keep: 7,
      path: "/data/backups",
    },
    fleet: [
      { name: "Primary NAS", url: "http://10.0.1.50:8080", api_key: "••••••••", headers: {} },
      { name: "Synology Backup", url: "http://10.0.1.60:8080", api_key: "••••••••", headers: {} },
      { name: "Proxmox Host", url: "http://10.0.1.70:8080", api_key: "••••••••", headers: {} },
      { name: "Remote Backup", url: "http://192.168.50.10:8080", api_key: "••••••••", headers: {} },
    ],
    proxmox: p.hasProxmox
      ? {
          enabled: true,
          url: "https://pve.local:8006",
          token_id: "nas-doctor@pam!monitoring",
          token_secret: "••••••••-••••-••••-••••-••••••••••••",
          node: "pve-node01",
          alias: "PVE Cluster",
          verify_ssl: false,
        }
      : { enabled: false, url: "", token_id: "", token_secret: "", node: "", alias: "", verify_ssl: true },
    kubernetes: p.hasKubernetes
      ? {
          enabled: true,
          mode: "external",
          kubeconfig_path: "/etc/rancher/k3s/k3s.yaml",
          context: "k3s-prod",
          namespace: "",
          alias: "K3s Production",
        }
      : { enabled: false, mode: "in-cluster", kubeconfig_path: "", context: "", namespace: "", alias: "" },
    dismissed_findings: [],
  };
}
