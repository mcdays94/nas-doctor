// Package analyzer — Storage Capacity Forecasting.
//
// Uses linear regression on historical disk usage to project when volumes
// will reach critical capacity thresholds.
package analyzer

import (
	"math"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// CapacityForecast is the forecast for a single mount point.
type CapacityForecast struct {
	MountPoint  string  `json:"mount_point"`
	Label       string  `json:"label"`
	Device      string  `json:"device"`
	TotalGB     float64 `json:"total_gb"`
	CurrentPct  float64 `json:"current_pct"`
	CurrentGB   float64 `json:"current_used_gb"`
	GrowthGBDay float64 `json:"growth_gb_per_day"` // avg GB added per day
	DaysTo90    int     `json:"days_to_90"`        // -1 = never / shrinking
	DaysTo95    int     `json:"days_to_95"`
	DaysTo100   int     `json:"days_to_100"`
	Confidence  float64 `json:"confidence"`  // R² value 0-1
	DataPoints  int     `json:"data_points"` // number of historical points
	Trend       string  `json:"trend"`       // "growing", "stable", "shrinking"
	Urgency     string  `json:"urgency"`     // "critical", "warning", "ok"
}

// CapacityReport is the full forecast for all volumes.
type CapacityReport struct {
	Volumes      []CapacityForecast `json:"volumes"`
	TotalVolumes int                `json:"total_volumes"`
	Critical     int                `json:"critical"` // < 30 days to full
	Warning      int                `json:"warning"`  // < 90 days to full
	OK           int                `json:"ok"`
}

// BuildCapacityReport generates forecasts for all disk volumes.
func BuildCapacityReport(series []storage.DiskUsageSeries) *CapacityReport {
	report := &CapacityReport{}

	for _, s := range series {
		if len(s.Points) < 2 || s.TotalGB <= 1 {
			continue // skip virtual/tiny/empty volumes
		}
		f := forecastVolume(s)
		report.Volumes = append(report.Volumes, f)
	}

	report.TotalVolumes = len(report.Volumes)
	for _, f := range report.Volumes {
		switch f.Urgency {
		case "critical":
			report.Critical++
		case "warning":
			report.Warning++
		default:
			report.OK++
		}
	}

	return report
}

func forecastVolume(s storage.DiskUsageSeries) CapacityForecast {
	f := CapacityForecast{
		MountPoint: s.MountPoint,
		Label:      s.Label,
		Device:     s.Device,
		TotalGB:    s.TotalGB,
		CurrentPct: s.CurrentPct,
		DataPoints: len(s.Points),
	}

	if len(s.Points) > 0 {
		f.CurrentGB = s.Points[len(s.Points)-1].UsedGB
	}

	// Convert timestamps to days-from-first for regression
	xs := make([]float64, len(s.Points))
	ys := make([]float64, len(s.Points))
	t0, _ := time.Parse(time.RFC3339Nano, s.Points[0].Timestamp)
	for i, p := range s.Points {
		t, err := time.Parse(time.RFC3339Nano, p.Timestamp)
		if err != nil {
			t, _ = time.Parse("2006-01-02T15:04:05Z", p.Timestamp)
		}
		xs[i] = t.Sub(t0).Hours() / 24.0 // days
		ys[i] = p.UsedGB
	}

	// Linear regression: y = slope*x + intercept
	slope, intercept, r2 := linearRegression(xs, ys)
	f.Confidence = r2
	f.GrowthGBDay = slope

	// Determine trend
	if slope > 0.01 {
		f.Trend = "growing"
	} else if slope < -0.01 {
		f.Trend = "shrinking"
	} else {
		f.Trend = "stable"
	}

	// Project days to thresholds
	lastDay := xs[len(xs)-1]
	currentProjected := slope*lastDay + intercept

	threshold90 := s.TotalGB * 0.90
	threshold95 := s.TotalGB * 0.95
	threshold100 := s.TotalGB

	f.DaysTo90 = capDays(daysToThreshold(currentProjected, slope, threshold90))
	f.DaysTo95 = capDays(daysToThreshold(currentProjected, slope, threshold95))
	f.DaysTo100 = capDays(daysToThreshold(currentProjected, slope, threshold100))

	// Determine urgency
	if f.DaysTo100 >= 0 && f.DaysTo100 < 30 {
		f.Urgency = "critical"
	} else if f.DaysTo95 >= 0 && f.DaysTo95 < 90 {
		f.Urgency = "warning"
	} else {
		f.Urgency = "ok"
	}

	// Override: if already above threshold, urgency escalates
	if f.CurrentPct >= 95 {
		f.Urgency = "critical"
	} else if f.CurrentPct >= 90 {
		if f.Urgency == "ok" {
			f.Urgency = "warning"
		}
	}

	return f
}

// capDays caps day forecasts at 10 years (3650 days). Anything beyond is effectively "never".
func capDays(d int) int {
	if d > 3650 {
		return -1 // treat as "never" if more than 10 years out
	}
	return d
}

// daysToThreshold calculates how many days from current until usage reaches threshold.
// Returns -1 if usage is shrinking/stable and will never reach it, or already past it.
func daysToThreshold(current, slopePerDay, threshold float64) int {
	if current >= threshold {
		return 0 // already past
	}
	if slopePerDay <= 0 {
		return -1 // never
	}
	days := (threshold - current) / slopePerDay
	return int(math.Ceil(days))
}

// linearRegression computes slope, intercept, and R² for y = slope*x + intercept.
func linearRegression(xs, ys []float64) (slope, intercept, r2 float64) {
	n := float64(len(xs))
	if n < 2 {
		return 0, 0, 0
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
	}

	denom := n*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-12 {
		return 0, sumY / n, 0
	}

	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n

	// R² (coefficient of determination)
	meanY := sumY / n
	var ssRes, ssTot float64
	for i := range xs {
		predicted := slope*xs[i] + intercept
		ssRes += (ys[i] - predicted) * (ys[i] - predicted)
		ssTot += (ys[i] - meanY) * (ys[i] - meanY)
	}
	if ssTot > 1e-12 {
		r2 = 1 - ssRes/ssTot
	}
	if r2 < 0 {
		r2 = 0
	}

	return slope, intercept, r2
}
