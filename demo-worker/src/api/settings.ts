import { Platform, PROFILES } from "../data/platforms";

export function generateSettings(platform: Platform) {
  const p = PROFILES[platform];

  return {
    api_key: "",
    theme: "midnight",
    scan_interval: "6h",
    chart_range_hours: 1,
    sections: {
      system: true,
      disks: true,
      smart: true,
      docker: true,
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
    webhooks: [],
    notification_policies: [],
    fleet_servers: [],
    service_checks: [],
    log_forwarding: {
      enabled: false,
      destination: "",
      format: "",
      url: "",
    },
    proxmox_config: p.hasProxmox
      ? {
          enabled: true,
          host: "https://pve.local:8006",
          token_id: "nas-doctor@pam!demo",
          token_secret: "********",
          node: "pve-node01",
          alias: "PVE Cluster",
          verify_ssl: false,
        }
      : {
          enabled: false,
          host: "",
          token_id: "",
          token_secret: "",
          node: "",
          alias: "",
          verify_ssl: true,
        },
    kubernetes_config: p.hasKubernetes
      ? {
          enabled: true,
          mode: "external",
          kubeconfig: "",
          context: "k3s-prod",
          namespace: "",
          alias: "K3s Production",
        }
      : {
          enabled: false,
          mode: "in-cluster",
          kubeconfig: "",
          context: "",
          namespace: "",
          alias: "",
        },
    dismissed_findings: [],
  };
}
