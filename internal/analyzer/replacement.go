// Package analyzer — Drive Replacement Planner.
//
// Uses Backblaze failure data and current SMART health to predict replacement
// urgency per drive and estimate fleet-wide replacement costs.
package analyzer

import (
	"fmt"
	"math"
	"sort"

	"github.com/mcdays94/nas-doctor/internal"
)

// ReplacementUrgency describes how urgently a drive should be replaced.
type ReplacementUrgency string

const (
	UrgencyReplaceNow  ReplacementUrgency = "replace_now"
	UrgencyReplaceSoon ReplacementUrgency = "replace_soon"
	UrgencyMonitor     ReplacementUrgency = "monitor"
	UrgencyHealthy     ReplacementUrgency = "healthy"
)

// DrivePlan is a single drive's replacement assessment.
type DrivePlan struct {
	Device         string             `json:"device"`
	Model          string             `json:"model"`
	Serial         string             `json:"serial"`
	ArraySlot      string             `json:"array_slot,omitempty"`
	DiskType       string             `json:"disk_type"`
	SizeGB         float64            `json:"size_gb"`
	HealthScore    int                `json:"health_score"`
	HealthPassed   bool               `json:"health_passed"`
	Urgency        ReplacementUrgency `json:"urgency"`
	UrgencyLabel   string             `json:"urgency_label"`
	RiskFactors    []string           `json:"risk_factors"`
	FailureMult    float64            `json:"failure_mult"`    // combined failure multiplier
	RemainingYears float64            `json:"remaining_years"` // estimated remaining life
	LifeUsedPct    float64            `json:"life_used_pct"`   // % of expected life used
	AgeBracket     string             `json:"age_bracket"`
	TempRating     string             `json:"temp_rating"`
	CostEstimate   float64            `json:"cost_estimate"` // replacement cost in user's currency
	PowerOnHours   int64              `json:"power_on_hours"`
	TempC          int                `json:"temp_c"`
	Reallocated    int64              `json:"reallocated"`
	Pending        int64              `json:"pending"`
	CRCErrors      int64              `json:"crc_errors"`
}

// ReplacementPlan is the full fleet replacement assessment.
type ReplacementPlan struct {
	Drives         []DrivePlan `json:"drives"`
	TotalDrives    int         `json:"total_drives"`
	ReplaceNow     int         `json:"replace_now"`
	ReplaceSoon    int         `json:"replace_soon"`
	Monitor        int         `json:"monitor"`
	Healthy        int         `json:"healthy"`
	TotalCost      float64     `json:"total_cost"`      // cost for replace_now + replace_soon
	TotalCostAll   float64     `json:"total_cost_all"`  // cost for all drives
	CostConfigured bool        `json:"cost_configured"` // whether user set cost_per_tb
	DataVersion    string      `json:"data_version"`    // Backblaze data version
}

// avgHDDLifeHours is the average HDD lifespan based on Backblaze fleet data.
// Most enterprise/NAS drives last 5-7 years; we use 6 as midpoint.
const avgHDDLifeHours = 52560 // 6 years
const avgSSDLifeHours = 70080 // 8 years

// BuildReplacementPlan generates a replacement assessment for all drives.
func BuildReplacementPlan(drives []internal.SMARTInfo, costPerTB float64) *ReplacementPlan {
	plan := &ReplacementPlan{
		CostConfigured: costPerTB > 0,
		DataVersion:    BackblazeDataVersion,
	}

	for _, d := range drives {
		dp := assessDrive(d, costPerTB)
		plan.Drives = append(plan.Drives, dp)
	}

	// Sort by urgency (worst first), then by health score (lowest first)
	sort.Slice(plan.Drives, func(i, j int) bool {
		ui, uj := urgencyRank(plan.Drives[i].Urgency), urgencyRank(plan.Drives[j].Urgency)
		if ui != uj {
			return ui < uj
		}
		return plan.Drives[i].HealthScore < plan.Drives[j].HealthScore
	})

	// Compute totals
	plan.TotalDrives = len(plan.Drives)
	for _, dp := range plan.Drives {
		switch dp.Urgency {
		case UrgencyReplaceNow:
			plan.ReplaceNow++
			plan.TotalCost += dp.CostEstimate
		case UrgencyReplaceSoon:
			plan.ReplaceSoon++
			plan.TotalCost += dp.CostEstimate
		case UrgencyMonitor:
			plan.Monitor++
		case UrgencyHealthy:
			plan.Healthy++
		}
		plan.TotalCostAll += dp.CostEstimate
	}

	return plan
}

func assessDrive(d internal.SMARTInfo, costPerTB float64) DrivePlan {
	dp := DrivePlan{
		Device:       d.Device,
		Model:        d.Model,
		Serial:       d.Serial,
		ArraySlot:    d.ArraySlot,
		DiskType:     d.DiskType,
		SizeGB:       d.SizeGB,
		HealthPassed: d.HealthPassed,
		PowerOnHours: d.PowerOnHours,
		TempC:        d.Temperature,
		Reallocated:  d.Reallocated,
		Pending:      d.Pending,
		CRCErrors:    d.UDMACRC,
	}

	// Health score
	dp.HealthScore = ComputeHealthScore(
		d.Reallocated, d.Pending, d.UDMACRC,
		d.CommandTimeout, d.Temperature, d.PowerOnHours, d.HealthPassed,
	)

	// Combined failure multiplier from all risk factors
	dp.FailureMult = 1.0
	if !d.HealthPassed {
		dp.FailureMult *= 10.0
		dp.RiskFactors = append(dp.RiskFactors, "SMART self-test FAILED")
	}
	if t := GetReallocatedTier(d.Reallocated); t != nil {
		dp.FailureMult *= t.FailureMult
		dp.RiskFactors = append(dp.RiskFactors, fmt.Sprintf("%d reallocated sectors (%s)", d.Reallocated, t.Label))
	}
	if t := GetPendingTier(d.Pending); t != nil {
		dp.FailureMult *= t.FailureMult
		dp.RiskFactors = append(dp.RiskFactors, fmt.Sprintf("%d pending sectors (%s)", d.Pending, t.Label))
	}
	if t := GetTempTier(d.Temperature); t != nil && t.Mult > 1.0 {
		dp.FailureMult *= t.Mult
		dp.RiskFactors = append(dp.RiskFactors, fmt.Sprintf("Temperature %d°C (%s)", d.Temperature, t.Label))
	}
	if t := GetAgeTier(d.PowerOnHours); t != nil && t.Mult > 1.0 {
		dp.FailureMult *= t.Mult
		dp.RiskFactors = append(dp.RiskFactors, fmt.Sprintf("Age %s (%s)", formatHours(d.PowerOnHours), t.Label))
	}

	// Age bracket + temp rating
	if at := GetAgeTier(d.PowerOnHours); at != nil {
		dp.AgeBracket = at.Label
	}
	if tt := GetTempTier(d.Temperature); tt != nil {
		dp.TempRating = tt.Label
	}

	// Life estimation
	avgLife := float64(avgHDDLifeHours)
	if d.DiskType == "ssd" || d.DiskType == "nvme" {
		avgLife = float64(avgSSDLifeHours)
	}
	dp.LifeUsedPct = float64(d.PowerOnHours) / avgLife * 100
	if dp.LifeUsedPct > 100 {
		dp.LifeUsedPct = math.Min(dp.LifeUsedPct, 200) // cap at 200%
	}
	// Remaining life adjusted by failure multiplier
	rawRemaining := (avgLife - float64(d.PowerOnHours)) / 8766 // years
	if rawRemaining < 0 {
		rawRemaining = 0
	}
	dp.RemainingYears = rawRemaining / dp.FailureMult
	if dp.RemainingYears < 0 {
		dp.RemainingYears = 0
	}

	// Determine urgency
	dp.Urgency, dp.UrgencyLabel = determineUrgency(dp)

	// Cost estimate
	if costPerTB > 0 {
		dp.CostEstimate = (d.SizeGB / 1000) * costPerTB
		dp.CostEstimate = math.Round(dp.CostEstimate*100) / 100
	}

	return dp
}

func determineUrgency(dp DrivePlan) (ReplacementUrgency, string) {
	// Replace Now: SMART failed, critical health, or extreme failure multiplier
	if !dp.HealthPassed {
		return UrgencyReplaceNow, "SMART self-test failed — replace immediately"
	}
	if dp.HealthScore < 30 {
		return UrgencyReplaceNow, "Critical health score — high failure risk"
	}
	if dp.Pending > 0 && dp.Reallocated > 19 {
		return UrgencyReplaceNow, "Active media degradation with significant reallocations"
	}
	if dp.FailureMult >= 15.0 {
		return UrgencyReplaceNow, fmt.Sprintf("%.0fx failure risk — imminent failure likely", dp.FailureMult)
	}

	// Replace Soon: degraded health, aging with issues
	if dp.HealthScore < 60 {
		return UrgencyReplaceSoon, "Degraded health — plan replacement within 3 months"
	}
	if dp.RemainingYears < 1.0 && dp.HealthScore < 80 {
		return UrgencyReplaceSoon, fmt.Sprintf("~%.1f years remaining at current degradation rate", dp.RemainingYears)
	}
	if dp.FailureMult >= 5.0 {
		return UrgencyReplaceSoon, fmt.Sprintf("%.1fx failure risk — schedule replacement", dp.FailureMult)
	}

	// Monitor: some issues but not urgent
	if dp.HealthScore < 80 {
		return UrgencyMonitor, "Minor issues — monitor closely"
	}
	if dp.LifeUsedPct > 80 {
		return UrgencyMonitor, fmt.Sprintf("%.0f%% of expected life used — entering end-of-life window", dp.LifeUsedPct)
	}
	if dp.FailureMult >= 2.0 {
		return UrgencyMonitor, "Elevated risk factors — watch for changes"
	}

	return UrgencyHealthy, "No replacement needed"
}

func urgencyRank(u ReplacementUrgency) int {
	switch u {
	case UrgencyReplaceNow:
		return 0
	case UrgencyReplaceSoon:
		return 1
	case UrgencyMonitor:
		return 2
	default:
		return 3
	}
}

func formatHours(h int64) string {
	if h <= 0 {
		return "0h"
	}
	days := h / 24
	if days > 365 {
		years := float64(days) / 365.25
		return fmt.Sprintf("%.1fy", years)
	}
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dh", h)
}
