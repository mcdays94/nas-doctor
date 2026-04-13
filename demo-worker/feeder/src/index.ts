/**
 * NAS Doctor Demo Feeder — Cron Worker
 *
 * Runs every 5 minutes. Reads base seed data from KV (captured from real Go binary),
 * applies time-based jitter to make metrics look live, shifts all timestamps to now,
 * and writes the refreshed data back to KV for the demo Worker to serve.
 *
 * The seed data is in the exact format the real Go app produces because it WAS
 * captured from the real Go binary. The feeder just keeps it fresh.
 */

interface Env {
  DEMO_DATA: KVNamespace;
}

export default {
  async scheduled(event: ScheduledController, env: Env, ctx: ExecutionContext): Promise<void> {
    console.log("Feeder: refreshing demo data at", new Date().toISOString());

    // Platform: for now just refresh unraid (base platform). Others can be added.
    const platform = "unraid";

    // Read and refresh each endpoint
    const endpoints = [
      "status", "snapshot", "sparklines", "fleet", "service_checks",
      "alerts", "incidents", "notifications_log", "gpu_history",
      "container_history", "system_history", "settings", "db_stats",
      "disks", "smart_trends", "replacement_plan", "capacity_forecast",
    ];

    for (const ep of endpoints) {
      try {
        // Read seed data (the original capture from the Go binary)
        const seedKey = `seed:${platform}:${ep}`;
        const apiKey = `api:${platform}:${ep}`;

        let seedData = await env.DEMO_DATA.get(seedKey, "text");

        // If no seed exists yet, copy current api data as seed (first run bootstrap)
        if (!seedData) {
          seedData = await env.DEMO_DATA.get(apiKey, "text");
          if (seedData) {
            await env.DEMO_DATA.put(seedKey, seedData);
            console.log(`Feeder: bootstrapped seed for ${seedKey}`);
          }
        }

        if (!seedData) continue;

        const data = JSON.parse(seedData);
        const refreshed = refreshData(ep, data);
        await env.DEMO_DATA.put(apiKey, JSON.stringify(refreshed));
      } catch (e) {
        console.error(`Feeder: error refreshing ${ep}:`, e);
      }
    }

    console.log("Feeder: done refreshing", endpoints.length, "endpoints");
  },

  // Also respond to HTTP requests for manual trigger / health check
  async fetch(request: Request, env: Env): Promise<Response> {
    if (new URL(request.url).pathname === "/trigger") {
      // Manual trigger: run the same logic as scheduled
      await this.scheduled!({} as ScheduledController, env, { waitUntil: () => {}, passThroughOnException: () => {} } as unknown as ExecutionContext);
      return new Response("Feeder triggered manually", { status: 200 });
    }
    return new Response("NAS Doctor Demo Feeder — runs on cron", { status: 200 });
  },
} satisfies ExportedHandler<Env>;

// ── Deterministic noise ──
function hash(seed: number): number {
  let h = seed | 0;
  h = ((h >> 16) ^ h) * 0x45d9f3b;
  h = ((h >> 16) ^ h) * 0x45d9f3b;
  h = (h >> 16) ^ h;
  return (h & 0x7fffffff) / 0x7fffffff;
}

function jitter(value: number, pctRange: number, seed: number): number {
  const slot = Math.floor(Date.now() / 300000); // changes every 5 min (matches cron)
  const h = hash(slot * 17 + seed * 53 + Math.round(value * 10));
  return value * (1 + (h - 0.5) * 2 * (pctRange / 100));
}

function clamp(v: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, v));
}

function dayFactor(): number {
  const hour = new Date().getUTCHours();
  if (hour >= 8 && hour <= 22) return 1.3;
  if (hour >= 6 && hour < 8) return 1.1;
  return 0.7;
}

// ── Data refreshers ──
// Each function takes the seed data and returns a refreshed copy with shifted timestamps and jittered metrics.

function refreshData(endpoint: string, data: unknown): unknown {
  switch (endpoint) {
    case "status": return refreshStatus(data as Record<string, unknown>);
    case "snapshot": return refreshSnapshot(data as Record<string, unknown>);
    case "sparklines": return refreshSparklines(data as Record<string, unknown>);
    case "fleet": return refreshFleet(data as unknown[]);
    case "service_checks": return refreshServiceChecks(data as unknown[]);
    case "system_history": return refreshHistory(data as unknown[]);
    case "gpu_history": return refreshHistory(data as unknown[]);
    case "container_history": return refreshHistory(data as unknown[]);
    default: return data; // pass through unchanged
  }
}

function refreshStatus(d: Record<string, unknown>): Record<string, unknown> {
  return {
    ...d,
    last_scan: new Date(Date.now() - 120000).toISOString(),
    critical_count: Math.round(clamp(jitter(1, 80, 1), 0, 3)),
    warning_count: Math.round(clamp(jitter(3, 50, 2), 1, 8)),
    info_count: Math.round(clamp(jitter(4, 40, 3), 2, 10)),
  };
}

function refreshSnapshot(d: Record<string, unknown>): Record<string, unknown> {
  const now = new Date().toISOString();
  const df = dayFactor();

  // Refresh system metrics
  const sys = d.system as Record<string, unknown> | undefined;
  if (sys) {
    sys.cpu_usage = round2(clamp(jitter((sys.cpu_usage as number) || 25, 20, 10) * df, 3, 85));
    sys.mem_percent = round2(clamp(jitter((sys.mem_percent as number) || 70, 8, 11), 40, 92));
    sys.io_wait = round2(clamp(jitter((sys.io_wait as number) || 3, 35, 12), 0, 15));
    sys.load_1 = round2(clamp(jitter((sys.load_1 as number) || 2, 25, 13) * df, 0.1, 20));
    sys.load_5 = round2(clamp(jitter((sys.load_5 as number) || 1.5, 20, 14) * df, 0.1, 16));
    sys.load_15 = round2(clamp(jitter((sys.load_15 as number) || 1.2, 15, 15) * df, 0.1, 12));
    sys.uptime_seconds = ((sys.uptime_seconds as number) || 0) + 300; // +5 min each tick
  }

  // Refresh SMART temperatures
  const smart = d.smart as Array<Record<string, unknown>> | undefined;
  if (smart) {
    for (let i = 0; i < smart.length; i++) {
      const s = smart[i];
      s.temperature_c = Math.round(clamp(jitter((s.temperature_c as number) || 35, 8, 100 + i), 22, 60));
    }
  }

  // Refresh Docker container metrics
  const docker = d.docker as Record<string, unknown> | undefined;
  if (docker) {
    const containers = docker.containers as Array<Record<string, unknown>> | undefined;
    if (containers) {
      for (let i = 0; i < containers.length; i++) {
        const c = containers[i];
        if (c.state !== "running") continue;
        c.cpu_percent = round2(clamp(jitter((c.cpu_percent as number) || 5, 25, 200 + i) * df, 0, 100));
        c.mem_mb = round2(clamp(jitter((c.mem_mb as number) || 256, 12, 300 + i), 16, 8192));
        c.mem_percent = round2(clamp(jitter((c.mem_percent as number) || 3, 15, 400 + i), 0.1, 50));
      }
    }
  }

  return { ...d, timestamp: now, id: `demo-${Math.floor(Date.now() / 300000)}` };
}

function refreshSparklines(d: Record<string, unknown>): Record<string, unknown> {
  const df = dayFactor();
  const sys = d.system as Array<Record<string, unknown>> | undefined;
  if (sys && sys.length > 0) {
    // Shift all timestamps forward and jitter the latest points
    const interval = 3600000; // 1 hour between points
    const now = Date.now();
    for (let i = 0; i < sys.length; i++) {
      sys[i].timestamp = new Date(now - (sys.length - 1 - i) * interval).toISOString();
      sys[i].cpu = round2(clamp(jitter((sys[i].cpu as number) || 25, 15, 500 + i) * df, 3, 85));
      sys[i].mem = round2(clamp(jitter((sys[i].mem as number) || 70, 8, 600 + i), 40, 92));
    }
  }
  return d;
}

function refreshFleet(data: unknown[]): unknown[] {
  return data.map((s: unknown, i: number) => {
    const server = s as Record<string, unknown>;
    if (server.online) {
      server.last_poll = new Date(Date.now() - (10000 + i * 5000)).toISOString();
    }
    return server;
  });
}

function refreshServiceChecks(data: unknown[]): unknown[] {
  return data.map((c: unknown, i: number) => {
    const check = c as Record<string, unknown>;
    if (check.status === "up") {
      check.response_ms = Math.round(clamp(jitter((check.response_ms as number) || 20, 30, 700 + i), 1, 500));
      check.checked_at = new Date().toISOString();
    }
    return check;
  });
}

function refreshHistory(data: unknown[]): unknown[] {
  if (!Array.isArray(data) || data.length === 0) return data;

  // Shift all timestamps so the latest is ~now
  const items = data as Array<Record<string, unknown>>;
  const lastTs = new Date(items[items.length - 1].timestamp as string).getTime();
  const offset = Date.now() - lastTs;

  return items.map((item, i) => {
    const shifted = { ...item };
    shifted.timestamp = new Date(new Date(item.timestamp as string).getTime() + offset).toISOString();

    // Jitter numeric values
    for (const key of Object.keys(shifted)) {
      if (key === "timestamp" || key === "name" || key === "image" || key === "gpu_index") continue;
      if (typeof shifted[key] === "number") {
        shifted[key] = round2(clamp(jitter(shifted[key] as number, 10, 800 + i), 0, 1e15));
      }
    }
    return shifted;
  });
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}
