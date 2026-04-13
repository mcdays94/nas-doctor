/**
 * Deterministic noise and time-varying data generation.
 * Uses timestamp-based seeding so the same moment always produces the same value,
 * but values change over time to simulate live metrics.
 */

/** Simple deterministic hash for seeding */
export function hash(seed: number): number {
  let h = seed | 0;
  h = ((h >> 16) ^ h) * 0x45d9f3b;
  h = ((h >> 16) ^ h) * 0x45d9f3b;
  h = (h >> 16) ^ h;
  return (h & 0x7fffffff) / 0x7fffffff; // 0..1
}

/** Get a deterministic value that changes every `intervalMs` */
export function timeNoise(base: number, intervalMs: number, amplitude: number, offset = 0): number {
  const slot = Math.floor(Date.now() / intervalMs);
  const h = hash(slot * 31 + offset * 7);
  return base + (h - 0.5) * 2 * amplitude;
}

/** Jitter a value by ±pct% using current time */
export function jitter(value: number, pctRange: number, seed = 0): number {
  const slot = Math.floor(Date.now() / 60000); // changes every minute
  const h = hash(slot * 17 + seed * 53 + Math.round(value * 100));
  const factor = 1 + (h - 0.5) * 2 * (pctRange / 100);
  return value * factor;
}

/** Clamp a number between min and max */
export function clamp(v: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, v));
}

/** Day-of-hour factor: higher during "day" hours (9-23), lower at night */
export function dayFactor(ts?: number): number {
  const hour = new Date(ts || Date.now()).getHours();
  if (hour >= 9 && hour <= 23) return 1.4;
  if (hour >= 6 && hour < 9) return 1.1;
  return 0.7; // night
}

/** Generate an array of historical data points going back `hours` from now */
export function generateTimeSeries(
  hours: number,
  intervalMinutes: number,
  generator: (ts: number, index: number) => number
): { timestamp: string; value: number }[] {
  const points: { timestamp: string; value: number }[] = [];
  const now = Date.now();
  const intervalMs = intervalMinutes * 60000;
  const count = Math.floor((hours * 60) / intervalMinutes);

  for (let i = count; i >= 0; i--) {
    const ts = now - i * intervalMs;
    points.push({
      timestamp: new Date(ts).toISOString(),
      value: generator(ts, i),
    });
  }
  return points;
}

/** ISO timestamp for N hours ago */
export function hoursAgo(h: number): string {
  return new Date(Date.now() - h * 3600000).toISOString();
}
