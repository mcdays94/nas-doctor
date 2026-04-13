import { jitter, clamp, hash } from "../data/noise";

const CHECKS = [
  { key: "sc-nas-doctor", name: "NAS Doctor", type: "http", target: "http://localhost:8060/api/v1/health", baseLatency: 12, up: true, severity: "critical" },
  { key: "sc-plex", name: "Plex Media Server", type: "http", target: "http://localhost:32400/web", baseLatency: 45, up: true, severity: "critical" },
  { key: "sc-gateway", name: "Gateway", type: "ping", target: "10.0.1.1", baseLatency: 2, up: true, severity: "critical" },
  { key: "sc-pihole", name: "Pi-hole DNS", type: "http", target: "http://10.0.1.53/admin", baseLatency: 0, up: false, severity: "critical" },
  { key: "sc-nextcloud", name: "Nextcloud", type: "http", target: "https://cloud.example.com/status.php", baseLatency: 185, up: true, severity: "warning" },
  { key: "sc-grafana", name: "Grafana", type: "http", target: "http://localhost:3000/api/health", baseLatency: 22, up: true, severity: "warning" },
];

export function generateServiceChecks() {
  const now = new Date().toISOString();
  return CHECKS.map((c) => ({
    key: c.key,
    name: c.name,
    type: c.type,
    target: c.target,
    status: c.up ? "up" : "down",
    response_ms: c.up ? Math.round(clamp(jitter(c.baseLatency, 30, c.key.length * 100), 1, 500)) : 0,
    consecutive_failures: c.up ? 0 : 12,
    failure_threshold: 5,
    failure_severity: c.severity,
    checked_at: c.up ? now : new Date(Date.now() - 300000).toISOString(),
  }));
}

/** Generate uptime history for a specific service check key */
export function generateServiceCheckHistory(key: string): Array<{
  key: string; name: string; type: string; target: string;
  status: string; response_ms: number; checked_at: string;
  consecutive_failures: number; failure_threshold: number; failure_severity: string;
}> {
  const check = CHECKS.find((c) => c.key === key) || CHECKS[0];
  const history: ReturnType<typeof generateServiceCheckHistory> = [];
  const now = Date.now();
  const interval = 30000; // 30 seconds between checks
  const count = 500;

  for (let i = count; i >= 0; i--) {
    const ts = now - i * interval;
    const slot = Math.floor(ts / interval);
    const h = hash(slot * 31 + check.key.length * 7);

    // Pi-hole is down for the last ~60 min (last 120 checks)
    let isUp = check.up;
    if (check.key === "sc-pihole") {
      isUp = i > 120; // was up before the last 120 intervals
    }

    // Occasional blips for other checks (0.5% failure rate)
    if (isUp && h < 0.005) isUp = false;

    const latency = isUp
      ? Math.round(clamp(check.baseLatency + (hash(slot * 17 + 3) - 0.5) * check.baseLatency * 0.6, 1, check.baseLatency * 3))
      : 0;

    history.push({
      key: check.key,
      name: check.name,
      type: check.type,
      target: check.target,
      status: isUp ? "up" : "down",
      response_ms: latency,
      checked_at: new Date(ts).toISOString(),
      consecutive_failures: isUp ? 0 : (check.key === "sc-pihole" && i <= 120 ? 120 - i : 1),
      failure_threshold: 5,
      failure_severity: check.severity,
    });
  }

  return history;
}
