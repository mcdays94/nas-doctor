import { Platform, PROFILES } from "../data/platforms";
import { hash, clamp, dayFactor } from "../data/noise";

/**
 * Determine the interval (in minutes) based on the hours window.
 * 1h → 5min, 24h → 30min, 168h (7d) → 120min
 */
function intervalForHours(hours: number): number {
  if (hours <= 1) return 5;
  if (hours <= 24) return 30;
  return 120;
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

function basePlusNoise(base: number, timeSeed: number, offset: number, pctRange: number): number {
  const h = hash(timeSeed * 17 + offset * 53);
  return base * (1 + (h - 0.5) * 2 * (pctRange / 100));
}

// ── GPU History ──
export function generateGPUHistory(hours: number) {
  const interval = intervalForHours(hours);
  const now = Date.now();
  const intervalMs = interval * 60000;
  const count = Math.floor((hours * 60) / interval);
  const points: Array<{
    timestamp: string;
    gpu_index: number;
    usage_percent: number;
    temperature_c: number;
    mem_used_mb: number;
    mem_total_mb: number;
    mem_percent: number;
    power_watts: number;
    encoder_percent: number;
    decoder_percent: number;
  }> = [];

  for (let i = count; i >= 0; i--) {
    const ts = now - i * intervalMs;
    const seed = Math.floor(ts / intervalMs);
    const df = dayFactor(ts);

    // RTX 4060
    const gpuUsage0 = Math.round(clamp(basePlusNoise(28 * df, seed, 2000, 30), 0, 100));
    const gpuTemp0 = Math.round(clamp(basePlusNoise(52, seed, 2001, 10), 30, 85));
    const gpuMem0 = Math.round(clamp(basePlusNoise(2048, seed, 2002, 20), 256, 7800));
    const gpuPower0 = round2(clamp(basePlusNoise(85 * df, seed, 2003, 20), 15, 170));

    points.push({
      timestamp: new Date(ts).toISOString(),
      gpu_index: 0,
      usage_percent: gpuUsage0,
      temperature_c: gpuTemp0,
      mem_used_mb: gpuMem0,
      mem_total_mb: 8192,
      mem_percent: round2((gpuMem0 / 8192) * 100),
      power_watts: gpuPower0,
      encoder_percent: Math.round(clamp(basePlusNoise(15 * df, seed, 2004, 40), 0, 100)),
      decoder_percent: Math.round(clamp(basePlusNoise(8 * df, seed, 2005, 40), 0, 100)),
    });

    // Intel UHD 730
    const gpuUsage1 = Math.round(clamp(basePlusNoise(12, seed, 2010, 30), 0, 100));
    const gpuTemp1 = Math.round(clamp(basePlusNoise(42, seed, 2011, 10), 25, 75));
    const gpuMem1 = Math.round(clamp(basePlusNoise(128, seed, 2012, 25), 32, 512));

    points.push({
      timestamp: new Date(ts).toISOString(),
      gpu_index: 1,
      usage_percent: gpuUsage1,
      temperature_c: gpuTemp1,
      mem_used_mb: gpuMem1,
      mem_total_mb: 512,
      mem_percent: round2((gpuMem1 / 512) * 100),
      power_watts: round2(clamp(basePlusNoise(12, seed, 2013, 20), 3, 25)),
      encoder_percent: Math.round(clamp(basePlusNoise(5, seed, 2014, 50), 0, 100)),
      decoder_percent: Math.round(clamp(basePlusNoise(3, seed, 2015, 50), 0, 100)),
    });
  }

  return points;
}

// ── Container History ──
export function generateContainerHistory(platform: Platform, hours: number) {
  const p = PROFILES[platform];
  const interval = intervalForHours(hours);
  const now = Date.now();
  const intervalMs = interval * 60000;
  const count = Math.floor((hours * 60) / interval);
  const points: Array<{
    timestamp: string;
    name: string;
    image: string;
    cpu_percent: number;
    mem_mb: number;
    mem_percent: number;
    net_in_bytes: number;
    net_out_bytes: number;
    block_read_bytes: number;
    block_write_bytes: number;
  }> = [];

  for (let i = count; i >= 0; i--) {
    const ts = now - i * intervalMs;
    const seed = Math.floor(ts / intervalMs);
    const df = dayFactor(ts);

    for (let ci = 0; ci < p.containers.length; ci++) {
      const c = p.containers[ci];
      if (c.state !== "running") continue;

      points.push({
        timestamp: new Date(ts).toISOString(),
        name: c.name,
        image: c.image,
        cpu_percent: round2(clamp(basePlusNoise(c.baseCPU * df, seed, 3000 + ci, 25), 0, 100)),
        mem_mb: round2(clamp(basePlusNoise(c.baseMem, seed, 3100 + ci, 15), 10, c.baseMem * 2)),
        mem_percent: round2(clamp(basePlusNoise(c.memPct, seed, 3200 + ci, 20), 0, 50)),
        net_in_bytes: Math.round(clamp(basePlusNoise(c.netIn / count, seed, 3300 + ci, 30), 0, c.netIn)),
        net_out_bytes: Math.round(clamp(basePlusNoise(c.netOut / count, seed, 3400 + ci, 30), 0, c.netOut)),
        block_read_bytes: Math.round(clamp(basePlusNoise(c.blockRead / count, seed, 3500 + ci, 30), 0, c.blockRead)),
        block_write_bytes: Math.round(clamp(basePlusNoise(c.blockWrite / count, seed, 3600 + ci, 30), 0, c.blockWrite)),
      });
    }
  }

  return points;
}

// ── System History ──
// Returns 24h of system metrics at 30min intervals
export function generateSystemHistory() {
  const now = Date.now();
  const intervalMs = 30 * 60000;
  const count = 48; // 24h at 30min intervals
  const points: Array<{
    timestamp: string;
    cpu_usage: number;
    mem_percent: number;
    io_wait: number;
    load1: number;
    load5: number;
    load15: number;
  }> = [];

  for (let i = count; i >= 0; i--) {
    const ts = now - i * intervalMs;
    const seed = Math.floor(ts / intervalMs);
    const df = dayFactor(ts);

    points.push({
      timestamp: new Date(ts).toISOString(),
      cpu_usage: round2(clamp(basePlusNoise(22 * df, seed, 4000, 20), 2, 95)),
      mem_percent: round2(clamp(basePlusNoise(70, seed, 4001, 8), 40, 95)),
      io_wait: round2(clamp(basePlusNoise(2.5, seed, 4002, 50), 0, 20)),
      load1: round2(clamp(basePlusNoise(4.5 * df, seed, 4003, 25), 0.1, 32)),
      load5: round2(clamp(basePlusNoise(3.8 * df, seed, 4004, 20), 0.1, 28)),
      load15: round2(clamp(basePlusNoise(3.2 * df, seed, 4005, 15), 0.1, 24)),
    });
  }

  return points;
}
