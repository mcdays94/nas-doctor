/**
 * Widget-coverage regression test for the demo feeder.
 *
 * The public demo at https://nasdoctordemo.mdias.info relies on
 * `transformSnapshot` producing JSON with the exact field names that
 * the dashboard widgets (in `internal/api/dashboard.go`) consume. When
 * the feeder forgets to generate data for a widget, the widget renders
 * empty and the dashboard's auto-column layout collapses sections into
 * too few columns.
 *
 * This suite walks every known dashboard widget and asserts that for at
 * least one supported platform the required JSON keys are present and
 * non-null. It's the guard that caught #262 and will catch the next
 * time someone ships a new widget in the Go binary without updating
 * the feeder.
 *
 * These tests are pure unit tests — no KV, no worker env. They call
 * the exported `transformSnapshot` with the captured unraid snapshot
 * seed shape plus the platform profile.
 */

import { describe, it, expect } from "vitest";
import { transformSnapshot, PROFILES, type Platform } from "./index";

// A minimal seed that resembles what `seed:unraid:snapshot` looks like
// after the Go binary's `--demo` capture. The feeder's
// `transformSnapshot` reads some fields from the seed (e.g. ups, gpu,
// parity, tunnels, proxmox, kubernetes) and rebuilds others from the
// platform profile. For the widgets this test cares about (speed_test,
// backup, top_processes, gpu, container metrics) the seed can be
// mostly empty — the feeder is expected to synthesise realistic data.
const SEED: Record<string, unknown> = {
  timestamp: "2026-04-23T12:00:00Z",
  id: "seed",
  system: {
    hostname: "seed",
    platform: "seed",
    cpu_model: "seed",
    cpu_cores: 8,
    mem_total_gb: 64,
    mem_used_gb: 32,
    mem_percent: 50,
    cpu_usage: 25,
    uptime_seconds: 100000,
  },
  disks: [],
  smart: [],
  docker: { available: true, version: "24.0.7", containers: [] },
  ups: { available: true, name: "Seed UPS", model: "Seed", battery_percent: 90, load_percent: 25, runtime_minutes: 45, on_battery: false, status_human: "Online" },
  gpu: { available: false, gpus: [] },
  parity: { available: true, history: [{ date: "2026-04-01", duration_seconds: 43200, speed_mb_s: 120, errors: 0 }] },
  tunnels: { available: true, cloudflared: [{ name: "demo", status: "healthy" }] },
  proxmox: { available: false },
  kubernetes: { available: false },
  network: { interfaces: [] },
  service_checks: [],
  zfs: { available: false, pools: [] },
  findings: [],
  logs: [],
  update: {},
  duration_seconds: 1.0,
};

// Helper: read a dotted / numeric-index path out of a JSON-ish value.
// Supports `foo.bar`, `repos.0.name`, etc. Returns undefined on any
// missing / non-traversable segment.
function getPath(obj: unknown, path: string): unknown {
  const parts = path.split(".");
  let cur: unknown = obj;
  for (const p of parts) {
    if (cur === null || cur === undefined) return undefined;
    const idx = /^\d+$/.test(p) ? Number(p) : p;
    cur = (cur as Record<string | number, unknown>)[idx];
  }
  return cur;
}

// The shape of a widget-coverage expectation: for each widget, the
// dotted paths that MUST be present + non-null after transformSnapshot
// runs on the listed platforms. Paths map exactly to what the
// dashboard.go `sections.*` JS consumes.
interface WidgetExpectation {
  widget: string;
  requiredKeys: string[];
  platforms: Platform[];
}

const EXPECTED_WIDGETS: WidgetExpectation[] = [
  // sections.speedtest in dashboard.go L782 — reads
  //   snapshot.speed_test.{available, latest.{download_mbps, upload_mbps, latency_ms, server_name, isp}, last_attempt.{status, timestamp}}
  {
    widget: "speed_test",
    requiredKeys: [
      "speed_test.available",
      "speed_test.latest.download_mbps",
      "speed_test.latest.upload_mbps",
      "speed_test.latest.latency_ms",
      "speed_test.latest.server_name",
      "speed_test.latest.isp",
      "speed_test.last_attempt.status",
      "speed_test.last_attempt.timestamp",
    ],
    platforms: ["unraid", "synology", "truenas", "proxmox", "kubernetes"],
  },
  // sections.backup in dashboard.go L705 — reads
  //   snapshot.backup.{available, jobs[].{provider, name, status, snapshot_count, size_bytes, last_success, encrypted}}
  {
    widget: "backup",
    requiredKeys: [
      "backup.available",
      "backup.jobs.0.provider",
      "backup.jobs.0.name",
      "backup.jobs.0.status",
      "backup.jobs.1.provider",
    ],
    // k8s uses its own backup story (velero, etc) and hides the widget.
    platforms: ["unraid", "synology", "truenas", "proxmox"],
  },
  // sections.processes in dashboard.go L1144 — reads
  //   snapshot.system.top_processes[].{command, cpu_percent, mem_percent, user, container_name}
  {
    widget: "top_processes",
    requiredKeys: [
      "system.top_processes.0.command",
      "system.top_processes.0.cpu_percent",
      "system.top_processes.0.mem_percent",
      "system.top_processes.0.user",
    ],
    platforms: ["unraid", "synology", "truenas", "proxmox", "kubernetes"],
  },
  // sections.gpu in dashboard.go L613 — reads
  //   snapshot.gpu.{available, gpus[].{name, vendor, usage_percent, temperature_c, mem_used_mb, mem_total_mb, power_watts}}
  // Only required for platforms with hasGPU: true (Unraid + Proxmox).
  {
    widget: "gpu",
    requiredKeys: [
      "gpu.available",
      "gpu.gpus.0.name",
      "gpu.gpus.0.vendor",
      "gpu.gpus.0.usage_percent",
      "gpu.gpus.0.temperature_c",
      "gpu.gpus.0.mem_total_mb",
    ],
    platforms: ["unraid", "proxmox"],
  },
  // sections.docker exists for every platform — covered by the default
  // feeder already, but pin it here so a future refactor can't delete it.
  {
    widget: "docker_containers",
    requiredKeys: [
      "docker.available",
      "docker.containers.0.name",
      "docker.containers.0.state",
      "docker.containers.0.cpu_percent",
    ],
    platforms: ["unraid", "synology", "truenas", "proxmox", "kubernetes"],
  },
];

describe("demo feeder widget coverage", () => {
  for (const { widget, requiredKeys, platforms } of EXPECTED_WIDGETS) {
    for (const platform of platforms) {
      it(`populates ${widget} for ${platform}`, () => {
        const profile = PROFILES[platform];
        const snap = transformSnapshot(SEED, profile, platform);
        for (const keyPath of requiredKeys) {
          const value = getPath(snap, keyPath);
          expect(
            value,
            `expected transformSnapshot(${platform}).${keyPath} to be defined, got: ${JSON.stringify(value)}`,
          ).toBeDefined();
          expect(
            value,
            `expected transformSnapshot(${platform}).${keyPath} to be non-null`,
          ).not.toBeNull();
        }
      });
    }
  }

  it("hides GPU widget on platforms without GPUs (synology, truenas, kubernetes)", () => {
    for (const platform of ["synology", "truenas", "kubernetes"] as Platform[]) {
      const snap = transformSnapshot(SEED, PROFILES[platform], platform);
      expect(getPath(snap, "gpu.available"), `${platform} must mark gpu.available=false`).toBe(false);
    }
  });

  it("uses platform-specific ISPs so the demo looks realistic per region", () => {
    const isps = (["unraid", "synology", "truenas", "proxmox", "kubernetes"] as Platform[]).map((p) => {
      const snap = transformSnapshot(SEED, PROFILES[p], p);
      return getPath(snap, "speed_test.latest.isp");
    });
    // Five distinct ISPs — no platform should share with another.
    expect(new Set(isps).size).toBe(5);
    for (const isp of isps) expect(typeof isp).toBe("string");
  });
});
