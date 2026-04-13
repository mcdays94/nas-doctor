import { Platform, PROFILES } from "../data/platforms";
import { hash, clamp } from "../data/noise";

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

/** SMART attribute trends per drive — 30 daily data points per attribute */
export function generateSmartTrends(platform: Platform) {
  const p = PROFILES[platform];
  const now = Date.now();

  return p.drives.map((d, di) => {
    // Generate 30 days of daily trend data for key attributes
    const days = 30;
    const temperature: Array<{ timestamp: string; value: number }> = [];
    const reallocated: Array<{ timestamp: string; value: number }> = [];
    const pending: Array<{ timestamp: string; value: number }> = [];
    const powerOnHours: Array<{ timestamp: string; value: number }> = [];
    const wearLeveling: Array<{ timestamp: string; value: number }> = [];

    for (let day = days; day >= 0; day--) {
      const ts = now - day * 86400000;
      const daySeed = Math.floor(ts / 86400000);
      const timestamp = new Date(ts).toISOString();

      // Temperature varies by day — simulate seasonal/ambient fluctuations
      const h = hash(daySeed * 31 + di * 97 + 7000);
      const tempVal = Math.round(clamp(d.tempC + (h - 0.5) * 6, 20, 65));
      temperature.push({ timestamp, value: tempVal });

      // Reallocated sectors — stable at 0 for healthy drives
      reallocated.push({ timestamp, value: d.healthPassed ? 0 : Math.floor(hash(daySeed + di * 3 + 7100) * 2) + 7 });

      // Pending sectors — always 0 for demo
      pending.push({ timestamp, value: 0 });

      // Power-on hours — linearly increasing
      const pohBase = d.powerOnHours - (days - (days - day)) * 24;
      powerOnHours.push({ timestamp, value: Math.max(0, pohBase + (days - day) * 24) });

      // Wear leveling (SSDs/NVMe only) — very slowly decreasing
      if (d.type !== "hdd") {
        const wlBase = 100 - d.powerOnHours / 500;
        const wlVal = clamp(wlBase - (days - day) * 0.02, 70, 100);
        wearLeveling.push({ timestamp, value: Math.round(wlVal) });
      }
    }

    const trends: Record<string, Array<{ timestamp: string; value: number }>> = {
      temperature,
      reallocated_sectors: reallocated,
      pending_sectors: pending,
      power_on_hours: powerOnHours,
    };

    if (d.type !== "hdd") {
      trends.wear_leveling = wearLeveling;
    }

    return {
      device: `/dev/${d.device}`,
      model: d.model,
      serial: d.serial,
      type: d.type,
      health_passed: d.healthPassed,
      trends,
    };
  });
}
