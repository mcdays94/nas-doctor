import { Platform, PROFILES } from "../data/platforms";
import { hash, clamp, dayFactor } from "../data/noise";

export function generateSparklines(platform: Platform) {
  const p = PROFILES[platform];
  const now = Date.now();

  // 24 system data points, 1 per hour going back 24h
  const system = Array.from({ length: 24 }, (_, i) => {
    const ts = now - (23 - i) * 3600000;
    const df = dayFactor(ts);
    const seed = Math.floor(ts / 3600000);

    return {
      timestamp: new Date(ts).toISOString(),
      cpu: round2(clamp(basePlusNoise(22 * df, seed, 10, 15), 2, 95)),
      mem: round2(clamp(basePlusNoise(70, seed, 20, 8), 40, 95)),
      io_wait: round2(clamp(basePlusNoise(2.5, seed, 30, 60), 0, 20)),
    };
  });

  // Temperature sparklines per drive, 24 points each
  const disks = p.drives.map((d, di) => ({
    serial: d.serial,
    device: `/dev/${d.device}`,
    model: d.model,
    temps: Array.from({ length: 24 }, (_, i) => {
      const ts = now - (23 - i) * 3600000;
      const seed = Math.floor(ts / 3600000);
      // Drives are warmer during the day
      const df = dayFactor(ts);
      const temp = clamp(
        basePlusNoise(d.tempC * (0.85 + 0.15 * df), seed, 100 + di, 5),
        20,
        65
      );
      return {
        timestamp: new Date(ts).toISOString(),
        temp: Math.round(temp),
      };
    }),
  }));

  return { system, disks };
}

/** Deterministic noise around a base value */
function basePlusNoise(base: number, timeSeed: number, offset: number, pctRange: number): number {
  const h = hash(timeSeed * 17 + offset * 53);
  return base * (1 + (h - 0.5) * 2 * (pctRange / 100));
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}
