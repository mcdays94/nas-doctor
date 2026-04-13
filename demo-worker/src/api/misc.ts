import { Platform, PROFILES } from "../data/platforms";
import { hash, clamp, hoursAgo } from "../data/noise";

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

export function generateMisc(type: string, platform: Platform) {
  if (type === "replacement-plan") {
    return generateReplacementPlan(platform);
  }
  if (type === "capacity-forecast") {
    return generateCapacityForecast(platform);
  }
  return { error: "unknown type" };
}

function generateReplacementPlan(platform: Platform) {
  const p = PROFILES[platform];

  const drives = p.drives.map((d, i) => {
    const riskScore = calculateRiskScore(d.powerOnHours, d.type, d.healthPassed, d.usedPct);
    const yearsRemaining = estimateYearsRemaining(d.powerOnHours, d.type);

    return {
      device: `/dev/${d.device}`,
      model: d.model,
      serial: d.serial,
      type: d.type,
      power_on_hours: d.powerOnHours,
      power_on_years: round2(d.powerOnHours / 8766),
      health_passed: d.healthPassed,
      risk_score: riskScore,
      risk_level: riskScore > 70 ? "high" : riskScore > 40 ? "medium" : "low",
      estimated_years_remaining: yearsRemaining,
      estimated_replacement_date: futureDate(yearsRemaining),
      replacement_priority: i + 1,
      estimated_cost_usd: estimateCost(d.sizeGB, d.type),
      recommended_replacement: recommendReplacement(d.sizeGB, d.type),
    };
  });

  // Sort by risk score descending
  drives.sort((a, b) => b.risk_score - a.risk_score);
  drives.forEach((d, i) => (d.replacement_priority = i + 1));

  const totalCost = drives.reduce((sum, d) => sum + d.estimated_cost_usd, 0);

  return {
    generated_at: new Date().toISOString(),
    total_drives: drives.length,
    drives_at_risk: drives.filter((d) => d.risk_level !== "low").length,
    estimated_total_cost_usd: totalCost,
    replacement_schedule: drives,
    recommendations: [
      "Replace high-risk drives within the next 6 months",
      "Keep spare drives on hand for emergency replacements",
      "Consider upgrading to larger capacity when replacing aging drives",
      "Maintain current backup strategy — verified backups are critical during drive replacements",
    ],
  };
}

function calculateRiskScore(poh: number, type: string, healthPassed: boolean, usedPct: number): number {
  let score = 0;
  if (!healthPassed) score += 50;

  if (type === "hdd") {
    score += clamp((poh / 50000) * 40, 0, 40);
  } else {
    score += clamp((poh / 30000) * 30, 0, 30);
  }

  if (usedPct > 85) score += 10;

  return Math.round(clamp(score, 0, 100));
}

function estimateYearsRemaining(poh: number, type: string): number {
  const maxHours = type === "hdd" ? 60000 : 40000;
  const remaining = Math.max(0, maxHours - poh);
  return round2(remaining / 8766);
}

function futureDate(years: number): string {
  const ms = years * 365.25 * 86400000;
  return new Date(Date.now() + ms).toISOString().split("T")[0];
}

function estimateCost(sizeGB: number, type: string): number {
  if (type === "nvme") return Math.round(sizeGB * 0.06);
  if (type === "ssd") return Math.round(sizeGB * 0.05);
  return Math.round(sizeGB * 0.015); // HDD
}

function recommendReplacement(sizeGB: number, type: string): string {
  if (type === "nvme") {
    if (sizeGB >= 2000) return "Samsung 990 Pro 4TB";
    return "Samsung 990 Pro 2TB";
  }
  if (type === "ssd") {
    return "Samsung PM893 1.92TB";
  }
  if (sizeGB >= 16000) {
    return "WDC WD200EFGX 20TB";
  }
  return "Seagate IronWolf Pro 16TB";
}

function generateCapacityForecast(platform: Platform) {
  const p = PROFILES[platform];
  const now = Date.now();

  const volumes = p.drives
    .filter((d) => d.usedPct > 0) // exclude parity
    .map((d, i) => {
      // Calculate growth rate: ~0.3-0.8% per day
      const dailyGrowthPct = 0.3 + hash(i * 77 + 8000) * 0.5;
      const currentUsedGB = (d.usedPct / 100) * d.sizeGB;
      const dailyGrowthGB = (dailyGrowthPct / 100) * d.sizeGB;
      const remainingGB = d.sizeGB - currentUsedGB;
      const daysToFull = remainingGB > 0 ? Math.round(remainingGB / dailyGrowthGB) : 0;

      // Generate 90-day forecast points
      const forecast = Array.from({ length: 90 }, (_, day) => {
        const projectedUsed = currentUsedGB + dailyGrowthGB * day;
        const projectedPct = clamp((projectedUsed / d.sizeGB) * 100, 0, 100);
        return {
          date: new Date(now + day * 86400000).toISOString().split("T")[0],
          projected_used_gb: round2(Math.min(projectedUsed, d.sizeGB)),
          projected_used_percent: round2(projectedPct),
        };
      });

      // 30-day historical (going back)
      const history = Array.from({ length: 30 }, (_, day) => {
        const pastUsed = currentUsedGB - dailyGrowthGB * (30 - day);
        return {
          date: new Date(now - (30 - day) * 86400000).toISOString().split("T")[0],
          used_gb: round2(Math.max(pastUsed, 0)),
          used_percent: round2(clamp((pastUsed / d.sizeGB) * 100, 0, 100)),
        };
      });

      return {
        device: `/dev/${d.device}`,
        label: d.label,
        total_gb: round2(d.sizeGB),
        current_used_gb: round2(currentUsedGB),
        current_used_percent: round2(d.usedPct),
        daily_growth_gb: round2(dailyGrowthGB),
        daily_growth_percent: round2(dailyGrowthPct),
        days_to_full: daysToFull,
        estimated_full_date: daysToFull > 0 ? new Date(now + daysToFull * 86400000).toISOString().split("T")[0] : null,
        days_to_80_percent:
          d.usedPct < 80
            ? Math.round(((0.8 * d.sizeGB - currentUsedGB) / dailyGrowthGB))
            : 0,
        forecast,
        history,
      };
    });

  return {
    generated_at: new Date().toISOString(),
    volumes,
    summary: {
      total_capacity_gb: round2(volumes.reduce((s, v) => s + v.total_gb, 0)),
      total_used_gb: round2(volumes.reduce((s, v) => s + v.current_used_gb, 0)),
      overall_used_percent: round2(
        (volumes.reduce((s, v) => s + v.current_used_gb, 0) /
          volumes.reduce((s, v) => s + v.total_gb, 0)) *
          100
      ),
      earliest_full_date: volumes
        .filter((v) => v.estimated_full_date)
        .sort((a, b) => (a.days_to_full || Infinity) - (b.days_to_full || Infinity))[0]
        ?.estimated_full_date || null,
    },
  };
}
