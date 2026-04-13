import { jitter, clamp } from "../data/noise";

export function generateServiceChecks() {
  const now = new Date().toISOString();
  const fiveMinAgo = new Date(Date.now() - 300000).toISOString();

  return [
    {
      key: "sc-nas-doctor",
      name: "NAS Doctor Dashboard",
      url: "http://localhost:8080/api/v1/health",
      method: "GET",
      status: "up",
      latency_ms: Math.round(clamp(jitter(12, 30, 5000), 2, 80)),
      status_code: 200,
      last_check: now,
      consecutive_failures: 0,
    },
    {
      key: "sc-plex",
      name: "Plex Media Server",
      url: "http://localhost:32400/web",
      method: "GET",
      status: "up",
      latency_ms: Math.round(clamp(jitter(45, 25, 5001), 10, 200)),
      status_code: 200,
      last_check: now,
      consecutive_failures: 0,
    },
    {
      key: "sc-router",
      name: "Router Admin",
      url: "http://10.0.1.1",
      method: "GET",
      status: "up",
      latency_ms: Math.round(clamp(jitter(8, 40, 5002), 1, 50)),
      status_code: 200,
      last_check: now,
      consecutive_failures: 0,
    },
    {
      key: "sc-pihole",
      name: "Pi-hole DNS",
      url: "http://10.0.1.53/admin",
      method: "GET",
      status: "down",
      latency_ms: 0,
      status_code: 0,
      last_check: fiveMinAgo,
      consecutive_failures: 12,
    },
    {
      key: "sc-nextcloud",
      name: "Nextcloud",
      url: "https://cloud.example.com/status.php",
      method: "GET",
      status: "up",
      latency_ms: Math.round(clamp(jitter(185, 20, 5004), 50, 500)),
      status_code: 200,
      last_check: now,
      consecutive_failures: 0,
    },
    {
      key: "sc-grafana",
      name: "Grafana",
      url: "http://localhost:3000/api/health",
      method: "GET",
      status: "up",
      latency_ms: Math.round(clamp(jitter(22, 25, 5005), 5, 100)),
      status_code: 200,
      last_check: now,
      consecutive_failures: 0,
    },
  ];
}
