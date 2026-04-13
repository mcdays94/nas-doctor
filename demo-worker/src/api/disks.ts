import { Platform, PROFILES } from "../data/platforms";
import { jitter, clamp, hash, hoursAgo } from "../data/noise";

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

/** List of disk summaries for the disks page */
export function generateDisks(platform: Platform) {
  const p = PROFILES[platform];

  return p.drives.map((d, i) => {
    const usedPct = clamp(jitter(d.usedPct, 3, 6000 + i), 0, 99);
    const usedGB = round2((usedPct / 100) * d.sizeGB);
    const temp = Math.round(clamp(jitter(d.tempC, 8, 6100 + i), 20, 65));

    return {
      device: `/dev/${d.device}`,
      model: d.model,
      serial: d.serial,
      type: d.type,
      mount_point: d.mountPoint,
      label: d.label,
      total_gb: round2(d.sizeGB),
      used_gb: usedGB,
      free_gb: round2(d.sizeGB - usedGB),
      used_percent: round2(usedPct),
      health_passed: d.healthPassed,
      temperature_c: temp,
      power_on_hours: d.powerOnHours,
      smart_status: d.healthPassed ? "PASSED" : "FAILED",
      assessment: assessDisk(d.usedPct, d.powerOnHours, temp, d.type),
    };
  });
}

function assessDisk(usedPct: number, poh: number, temp: number, type: string): string {
  if (usedPct > 90) return "critical";
  if (usedPct > 80 || (type === "hdd" && poh > 40000) || temp > 50) return "warning";
  return "healthy";
}

/** Detailed single disk view with SMART attributes and history */
export function generateDiskDetail(platform: Platform) {
  const p = PROFILES[platform];
  // Use the first drive with interesting data (highest usage)
  const d = [...p.drives].sort((a, b) => b.usedPct - a.usedPct)[0];

  const usedPct = clamp(jitter(d.usedPct, 3, 6200), 0, 99);
  const usedGB = round2((usedPct / 100) * d.sizeGB);
  const temp = Math.round(clamp(jitter(d.tempC, 8, 6201), 20, 65));
  const poh = d.powerOnHours + Math.floor(Date.now() / 3600000) % 100;

  // Generate 7 days of temperature history (every 2 hours)
  const now = Date.now();
  const tempHistory = Array.from({ length: 84 }, (_, i) => {
    const ts = now - (83 - i) * 7200000;
    const seed = Math.floor(ts / 7200000);
    const h = hash(seed * 17 + 6300);
    const dayTemp = d.tempC * (0.85 + 0.15 * dayFactorAt(ts));
    return {
      timestamp: new Date(ts).toISOString(),
      temperature_c: Math.round(clamp(dayTemp + (h - 0.5) * 6, 20, 65)),
    };
  });

  // Generate 30 days of usage history (daily)
  const usageHistory = Array.from({ length: 30 }, (_, i) => {
    const ts = now - (29 - i) * 86400000;
    // Gradual increase in usage
    const basePct = d.usedPct - (29 - i) * 0.15;
    const h = hash(Math.floor(ts / 86400000) * 31 + 6400);
    return {
      timestamp: new Date(ts).toISOString(),
      used_percent: round2(clamp(basePct + (h - 0.5) * 2, 0, 99)),
      used_gb: round2(clamp((basePct / 100) * d.sizeGB + (h - 0.5) * 50, 0, d.sizeGB)),
    };
  });

  return {
    device: `/dev/${d.device}`,
    model: d.model,
    serial: d.serial,
    firmware: "FW01",
    type: d.type,
    mount_point: d.mountPoint,
    label: d.label,
    total_gb: round2(d.sizeGB),
    used_gb: usedGB,
    free_gb: round2(d.sizeGB - usedGB),
    used_percent: round2(usedPct),
    health_passed: d.healthPassed,
    temperature_c: temp,
    power_on_hours: poh,
    power_cycle_count: Math.floor(poh / 2000) + 12,
    reallocated_sectors: 0,
    pending_sectors: 0,
    offline_uncorrectable: 0,
    udma_crc_errors: 0,
    reads_gb: round2(poh * 0.8),
    writes_gb: round2(poh * 0.3),
    wear_leveling: d.type !== "hdd" ? Math.round(clamp(100 - poh / 500, 70, 100)) : null,
    smart_attributes: generateSmartAttributes(d.type, poh),
    temperature_history: tempHistory,
    usage_history: usageHistory,
    assessment: assessDisk(d.usedPct, poh, temp, d.type),
    backblaze_comparison: {
      model: d.model,
      annual_failure_rate: d.type === "hdd" ? 1.42 : 0.38,
      sample_size: d.type === "hdd" ? 24589 : 8234,
      confidence: "high",
    },
  };
}

function generateSmartAttributes(type: string, poh: number) {
  if (type === "hdd") {
    return [
      { id: 1, name: "Raw_Read_Error_Rate", current: 200, worst: 200, threshold: 51, raw: 0, status: "ok" },
      { id: 3, name: "Spin_Up_Time", current: 184, worst: 183, threshold: 21, raw: 5791, status: "ok" },
      { id: 4, name: "Start_Stop_Count", current: 100, worst: 100, threshold: 0, raw: Math.floor(poh / 2000) + 12, status: "ok" },
      { id: 5, name: "Reallocated_Sector_Ct", current: 200, worst: 200, threshold: 140, raw: 0, status: "ok" },
      { id: 7, name: "Seek_Error_Rate", current: 200, worst: 200, threshold: 0, raw: 0, status: "ok" },
      { id: 9, name: "Power_On_Hours", current: 95, worst: 95, threshold: 0, raw: poh, status: "ok" },
      { id: 10, name: "Spin_Retry_Count", current: 100, worst: 100, threshold: 0, raw: 0, status: "ok" },
      { id: 194, name: "Temperature_Celsius", current: 117, worst: 107, threshold: 0, raw: 33, status: "ok" },
      { id: 197, name: "Current_Pending_Sector", current: 200, worst: 200, threshold: 0, raw: 0, status: "ok" },
      { id: 198, name: "Offline_Uncorrectable", current: 200, worst: 200, threshold: 0, raw: 0, status: "ok" },
    ];
  }

  // SSD/NVMe attributes
  return [
    { id: 1, name: "Raw_Read_Error_Rate", current: 100, worst: 100, threshold: 0, raw: 0, status: "ok" },
    { id: 5, name: "Reallocated_Sector_Ct", current: 100, worst: 100, threshold: 10, raw: 0, status: "ok" },
    { id: 9, name: "Power_On_Hours", current: 99, worst: 99, threshold: 0, raw: poh, status: "ok" },
    { id: 12, name: "Power_Cycle_Count", current: 100, worst: 100, threshold: 0, raw: Math.floor(poh / 2000) + 5, status: "ok" },
    { id: 177, name: "Wear_Leveling_Count", current: Math.round(clamp(100 - poh / 500, 70, 100)), worst: Math.round(clamp(100 - poh / 500, 70, 100)), threshold: 5, raw: Math.round(poh / 500), status: "ok" },
    { id: 194, name: "Temperature_Celsius", current: 67, worst: 55, threshold: 0, raw: 40, status: "ok" },
    { id: 241, name: "Total_LBAs_Written", current: 99, worst: 99, threshold: 0, raw: Math.round(poh * 380), status: "ok" },
    { id: 242, name: "Total_LBAs_Read", current: 99, worst: 99, threshold: 0, raw: Math.round(poh * 920), status: "ok" },
  ];
}

function dayFactorAt(ts: number): number {
  const hour = new Date(ts).getHours();
  if (hour >= 9 && hour <= 23) return 1.4;
  if (hour >= 6 && hour < 9) return 1.1;
  return 0.7;
}
