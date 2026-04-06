// Package analyzer implements rule-based diagnostic analysis.
// It encodes the same pattern-matching intelligence as the OpenCode skill,
// but runs deterministically without an LLM.
package analyzer

import (
	"fmt"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// Analyze takes a snapshot and produces findings by running all rule sets.
func Analyze(snap *internal.Snapshot) []internal.Finding {
	var findings []internal.Finding

	findings = append(findings, analyzeSMART(snap.SMART)...)
	findings = append(findings, analyzeThermal(snap.SMART)...)
	findings = append(findings, analyzeMemory(snap.System)...)
	findings = append(findings, analyzeIOWait(snap.System)...)
	findings = append(findings, analyzeDiskSpace(snap.Disks)...)
	findings = append(findings, analyzeDocker(snap.Docker)...)
	findings = append(findings, analyzeNetwork(snap.Network)...)
	findings = append(findings, analyzeLogs(snap.Logs)...)

	if snap.Parity != nil {
		findings = append(findings, analyzeParity(snap.Parity)...)
	}

	// Cross-correlation: combine related findings
	findings = correlate(findings, snap)

	// Assign IDs
	for i := range findings {
		findings[i].ID = fmt.Sprintf("F%03d", i+1)
	}

	return findings
}

// ---------- SMART Rules (Backblaze-informed thresholds) ----------
// Thresholds from: backblaze_thresholds.go (data version: Q4-2025)

func analyzeSMART(drives []internal.SMARTInfo) []internal.Finding {
	var findings []internal.Finding
	for _, d := range drives {
		// SMART health self-assessment failed
		if !d.HealthPassed {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategorySMART,
				Title:       fmt.Sprintf("SMART Health FAILED: %s (%s)", d.Device, d.Model),
				Description: fmt.Sprintf("Drive %s (S/N: %s) has FAILED its SMART self-assessment. This drive is at imminent risk of failure.", d.Device, d.Serial),
				Evidence:    []string{"SMART overall-health self-assessment: FAILED"},
				Impact:      "Data loss if the drive fails before data is migrated",
				Action:      "Replace this drive immediately. Back up any unique data NOW.",
				Priority:    "immediate",
				Cost:        estimateDriveCost(d),
				RelatedDisk: d.ArraySlot,
			})
		}

		// Reallocated sectors — Backblaze tiered thresholds
		if tier := GetReallocatedTier(d.Reallocated); tier != nil {
			sev := internal.SeverityWarning
			if tier.Severity == "critical" {
				sev = internal.SeverityCritical
			}
			findings = append(findings, internal.Finding{
				Severity: sev,
				Category: internal.CategorySMART,
				Title:    fmt.Sprintf("Reallocated Sectors on %s (%s)", d.Device, d.Model),
				Description: fmt.Sprintf(
					"Drive %s has %d reallocated sectors. %s — Backblaze data (%s) shows drives at this level fail at %.1fx the baseline rate.",
					d.Device, d.Reallocated, tier.Label, BackblazeDataVersion, tier.FailureMult,
				),
				Evidence: []string{
					fmt.Sprintf("Reallocated_Sector_Ct: %d", d.Reallocated),
					fmt.Sprintf("Backblaze failure multiplier: %.1fx baseline (data: %s)", tier.FailureMult, BackblazeDataVersion),
				},
				Impact:      "Progressive drive failure, potential data loss",
				Action:      "Monitor closely. Plan replacement if count is increasing.",
				Priority:    priorityFromSeverity(sev),
				Cost:        estimateDriveCost(d),
				RelatedDisk: d.ArraySlot,
			})
		}

		// Pending sectors — Backblaze tiered thresholds
		if tier := GetPendingTier(d.Pending); tier != nil {
			findings = append(findings, internal.Finding{
				Severity: internal.SeverityCritical,
				Category: internal.CategorySMART,
				Title:    fmt.Sprintf("Pending Sectors on %s (%s)", d.Device, d.Model),
				Description: fmt.Sprintf(
					"Drive %s has %d pending sectors. %s — Backblaze data shows drives with pending sectors fail at %.1fx the baseline rate.",
					d.Device, d.Pending, tier.Label, tier.FailureMult,
				),
				Evidence: []string{
					fmt.Sprintf("Current_Pending_Sector: %d", d.Pending),
					fmt.Sprintf("Backblaze failure multiplier: %.1fx baseline", tier.FailureMult),
				},
				Impact:      "Active read errors, data corruption risk",
				Action:      "Run an extended SMART self-test. Plan drive replacement.",
				Priority:    "immediate",
				Cost:        estimateDriveCost(d),
				RelatedDisk: d.ArraySlot,
			})
		}

		// UDMA CRC errors — Backblaze tiered thresholds
		if tier := GetCRCTier(d.UDMACRC); tier != nil {
			sev := internal.SeverityInfo
			if tier.Severity == "warning" {
				sev = internal.SeverityWarning
			}
			findings = append(findings, internal.Finding{
				Severity: sev,
				Category: internal.CategorySMART,
				Title:    fmt.Sprintf("SATA Cable Issue on %s (%s)", d.Device, d.Model),
				Description: fmt.Sprintf(
					"Drive %s has %d UDMA CRC errors. %s. CRC errors indicate data transfer corruption — almost always a cable/connection issue, not the drive itself.",
					d.Device, d.UDMACRC, tier.Label,
				),
				Evidence: []string{
					fmt.Sprintf("UDMA_CRC_Error_Count: %d", d.UDMACRC),
					fmt.Sprintf("ATA Port: %s", d.ATAPort),
				},
				Impact:      "Slow I/O, transfer errors, parity check slowdowns",
				Action:      fmt.Sprintf("Replace the SATA cable on port %s. Use a certified SATA III cable.", d.ATAPort),
				Priority:    priorityFromSeverity(sev),
				Cost:        "$5-15 for a new SATA cable",
				RelatedDisk: d.ArraySlot,
			})
		}

		// Command timeouts — Backblaze tiered thresholds
		if tier := GetCmdTimeoutTier(d.CommandTimeout); tier != nil {
			sev := internal.SeverityInfo
			if tier.Severity == "warning" {
				sev = internal.SeverityWarning
			} else if tier.Severity == "critical" {
				sev = internal.SeverityCritical
			}
			findings = append(findings, internal.Finding{
				Severity: sev,
				Category: internal.CategorySMART,
				Title:    fmt.Sprintf("Command Timeouts on %s (%s)", d.Device, d.Model),
				Description: fmt.Sprintf(
					"Drive %s has %d command timeouts. %s.",
					d.Device, d.CommandTimeout, tier.Label,
				),
				Evidence:    []string{fmt.Sprintf("Command_Timeout: %d", d.CommandTimeout)},
				Impact:      "System hangs, slow I/O operations",
				Action:      "Check SATA cable and power connections. May indicate controller or drive issues.",
				Priority:    priorityFromSeverity(sev),
				Cost:        "$5-15 (cable) or " + estimateDriveCost(d) + " (replacement)",
				RelatedDisk: d.ArraySlot,
			})
		}

		// Drive age — Backblaze bathtub curve data
		if tier := GetAgeTier(d.PowerOnHours); tier != nil && tier.Severity != "ok" {
			years := float64(d.PowerOnHours) / 8766
			sev := internal.SeverityInfo
			if tier.Severity == "warning" {
				sev = internal.SeverityWarning
			}
			findings = append(findings, internal.Finding{
				Severity: sev,
				Category: internal.CategorySMART,
				Title:    fmt.Sprintf("Aging Drive: %s (%s)", d.Device, d.Model),
				Description: fmt.Sprintf(
					"Drive %s has %d power-on hours (%.1f years). %s. Backblaze data shows failure rate at %.1fx baseline for drives at this age.",
					d.Device, d.PowerOnHours, years, tier.Label, tier.Mult,
				),
				Evidence: []string{
					fmt.Sprintf("Power_On_Hours: %d (%.1f years)", d.PowerOnHours, years),
					fmt.Sprintf("Backblaze age-based failure multiplier: %.1fx", tier.Mult),
				},
				Impact:      "Increased probability of failure over time",
				Action:      "Ensure backups are current. Consider proactive replacement.",
				Priority:    priorityFromSeverity(sev),
				Cost:        estimateDriveCost(d),
				RelatedDisk: d.ArraySlot,
			})
		}

		// Composite health score for info
		score := ComputeHealthScore(d.Reallocated, d.Pending, d.UDMACRC, d.CommandTimeout, d.Temperature, d.PowerOnHours, d.HealthPassed)
		if score < 50 && d.HealthPassed { // Only add if SMART test itself didn't fail (avoid duplicate)
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategorySMART,
				Title:       fmt.Sprintf("Low Health Score: %s at %d/100", d.Device, score),
				Description: fmt.Sprintf("Drive %s (%s) has a composite health score of %d/100 based on Backblaze failure rate data. Multiple risk factors are combining to create elevated failure probability.", d.Device, d.Model, score),
				Evidence: []string{
					fmt.Sprintf("Health Score: %d/100", score),
					fmt.Sprintf("Based on: Backblaze Drive Stats %s (337,000+ drives)", BackblazeDataVersion),
				},
				Impact:      "High probability of drive failure",
				Action:      "Plan replacement. Ensure backups are current and verified.",
				Priority:    "immediate",
				Cost:        estimateDriveCost(d),
				RelatedDisk: d.ArraySlot,
			})
		}
	}
	return findings
}

// ---------- Thermal Rules (Backblaze + Google research) ----------

func analyzeThermal(drives []internal.SMARTInfo) []internal.Finding {
	var findings []internal.Finding
	var hotDrives []string

	for _, d := range drives {
		tier := GetTempTier(d.Temperature)
		if tier == nil || tier.Severity == "ok" {
			continue
		}

		sev := internal.SeverityInfo
		if tier.Severity == "warning" {
			sev = internal.SeverityWarning
			hotDrives = append(hotDrives, d.Device)
		} else if tier.Severity == "critical" {
			sev = internal.SeverityCritical
			hotDrives = append(hotDrives, d.Device)
		}

		findings = append(findings, internal.Finding{
			Severity: sev,
			Category: internal.CategoryThermal,
			Title:    fmt.Sprintf("Drive Temperature: %s at %d°C", d.Device, d.Temperature),
			Description: fmt.Sprintf(
				"Drive %s (%s) is at %d°C. %s — Backblaze + Google research shows failure rate at %.1fx baseline at this temperature.",
				d.Device, d.Model, d.Temperature, tier.Label, tier.Mult,
			),
			Evidence: []string{
				fmt.Sprintf("Current temperature: %d°C", d.Temperature),
				fmt.Sprintf("Failure rate multiplier: %.1fx (Backblaze/Google data)", tier.Mult),
			},
			Impact:      "Reduced drive lifespan, increased error rate",
			Action:      "Improve case airflow. Add/replace fans. Check that existing fans are working.",
			Priority:    priorityFromSeverity(sev),
			Cost:        "$20-50 for case fans",
			RelatedDisk: d.ArraySlot,
		})

		// Max temperature ever — historical damage check
		if d.TempMax >= 60 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryThermal,
				Title:       fmt.Sprintf("Historical Overheating on %s (max %d°C)", d.Device, d.TempMax),
				Description: fmt.Sprintf("Drive %s has reached %d°C at some point in its lifetime. At this temperature, failure rate is ~%.1fx baseline. Thermal damage may be permanent.", d.Device, d.TempMax, GetTempTier(d.TempMax).Mult),
				Evidence:    []string{fmt.Sprintf("Airflow_Temperature_Max: %d°C", d.TempMax)},
				Impact:      "Possible latent damage from thermal stress",
				Action:      "Monitor SMART attributes closely for degradation.",
				Priority:    "medium-term",
				RelatedDisk: d.ArraySlot,
			})
		}
	}

	// Systemic thermal issue
	if len(hotDrives) >= 3 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityCritical,
			Category:    internal.CategoryThermal,
			Title:       "Systemic Thermal Issue",
			Description: fmt.Sprintf("%d drives are running hot (%s). This suggests a case-level airflow problem rather than individual drive issues.", len(hotDrives), strings.Join(hotDrives, ", ")),
			Evidence:    []string{fmt.Sprintf("%d of %d drives above safe temperature", len(hotDrives), len(drives))},
			Impact:      "Accelerated wear across the entire array",
			Action:      "Check all case fans are operational. Consider adding intake/exhaust fans. Clean dust filters.",
			Priority:    "immediate",
			Cost:        "$50-100 for fan upgrades",
		})
	}

	return findings
}

// ---------- Memory Rules ----------

func analyzeMemory(sys internal.SystemInfo) []internal.Finding {
	var findings []internal.Finding

	if sys.MemPercent >= 95 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityCritical,
			Category:    internal.CategoryMemory,
			Title:       "Critical Memory Pressure",
			Description: fmt.Sprintf("Memory usage is at %.1f%% (%d MB / %d MB). The system may be swapping heavily or at risk of OOM kills.", sys.MemPercent, sys.MemUsedMB, sys.MemTotalMB),
			Evidence:    []string{fmt.Sprintf("Memory: %d/%d MB (%.1f%%)", sys.MemUsedMB, sys.MemTotalMB, sys.MemPercent)},
			Impact:      "Application crashes, severe performance degradation",
			Action:      "Identify memory-hungry processes. Consider adding RAM or reducing Docker container count.",
			Priority:    "immediate",
			Cost:        "$30-100 for RAM upgrade",
		})
	} else if sys.MemPercent >= 85 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityWarning,
			Category:    internal.CategoryMemory,
			Title:       "High Memory Usage",
			Description: fmt.Sprintf("Memory usage is at %.1f%% (%d MB / %d MB).", sys.MemPercent, sys.MemUsedMB, sys.MemTotalMB),
			Evidence:    []string{fmt.Sprintf("Memory: %d/%d MB (%.1f%%)", sys.MemUsedMB, sys.MemTotalMB, sys.MemPercent)},
			Impact:      "May cause slowdowns under additional load",
			Action:      "Review container memory limits. Consider RAM upgrade if usage keeps growing.",
			Priority:    "short-term",
			Cost:        "$30-100 for RAM upgrade",
		})
	}

	// Swap usage
	if sys.SwapUsedMB > 0 && sys.SwapTotalMB > 0 {
		swapPct := float64(sys.SwapUsedMB) / float64(sys.SwapTotalMB) * 100
		if swapPct > 50 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryMemory,
				Title:       "Heavy Swap Usage",
				Description: fmt.Sprintf("Swap is %d/%d MB (%.0f%% used). Active swapping causes severe I/O performance degradation.", sys.SwapUsedMB, sys.SwapTotalMB, swapPct),
				Evidence:    []string{fmt.Sprintf("Swap: %d/%d MB", sys.SwapUsedMB, sys.SwapTotalMB)},
				Impact:      "Significantly increased I/O load, overall system slowness",
				Action:      "Add more RAM. Review which processes are consuming the most memory.",
				Priority:    "short-term",
				Cost:        "$30-100 for RAM upgrade",
			})
		}
	}

	return findings
}

// ---------- I/O Wait Rules ----------

func analyzeIOWait(sys internal.SystemInfo) []internal.Finding {
	var findings []internal.Finding

	if sys.IOWait >= 30 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityCritical,
			Category:    internal.CategoryDisk,
			Title:       "Severe Disk I/O Bottleneck",
			Description: fmt.Sprintf("I/O wait is at %.1f%%. CPUs are spending a large portion of time waiting for disk operations.", sys.IOWait),
			Evidence:    []string{fmt.Sprintf("I/O Wait: %.1f%%", sys.IOWait)},
			Impact:      "Everything on the system feels slow — file transfers, Docker containers, application responsiveness",
			Action:      "Add an SSD cache drive. Check for failing disks or bad SATA cables causing retries.",
			Priority:    "immediate",
			Cost:        "$50-150 for SSD cache drive",
		})
	} else if sys.IOWait >= 15 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityWarning,
			Category:    internal.CategoryDisk,
			Title:       "Elevated Disk I/O Wait",
			Description: fmt.Sprintf("I/O wait is at %.1f%%. This is above the ideal threshold of <10%%.", sys.IOWait),
			Evidence:    []string{fmt.Sprintf("I/O Wait: %.1f%%", sys.IOWait)},
			Impact:      "Noticeable performance degradation during heavy disk activity",
			Action:      "Consider adding an SSD cache for Docker containers and frequently-accessed data.",
			Priority:    "short-term",
			Cost:        "$50-150 for SSD cache drive",
		})
	}

	// High load average relative to CPU count
	if sys.LoadAvg5 > float64(sys.CPUCores)*2 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityWarning,
			Category:    internal.CategorySystem,
			Title:       "High System Load",
			Description: fmt.Sprintf("5-minute load average (%.2f) is more than 2x the CPU core count (%d). The system is overloaded.", sys.LoadAvg5, sys.CPUCores),
			Evidence:    []string{fmt.Sprintf("Load Avg (1/5/15): %.2f / %.2f / %.2f", sys.LoadAvg1, sys.LoadAvg5, sys.LoadAvg15), fmt.Sprintf("CPU Cores: %d", sys.CPUCores)},
			Impact:      "Process scheduling delays, overall sluggishness",
			Action:      "Identify CPU-heavy processes. Reduce concurrent workloads or upgrade CPU.",
			Priority:    "short-term",
		})
	}

	return findings
}

// ---------- Disk Space Rules ----------

func analyzeDiskSpace(disks []internal.DiskInfo) []internal.Finding {
	var findings []internal.Finding
	for _, d := range disks {
		if d.UsedPct >= 97 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryDisk,
				Title:       fmt.Sprintf("Disk Almost Full: %s (%.0f%%)", d.MountPoint, d.UsedPct),
				Description: fmt.Sprintf("%s is at %.0f%% capacity (%.1f GB free of %.1f GB).", d.MountPoint, d.UsedPct, d.FreeGB, d.TotalGB),
				Evidence:    []string{fmt.Sprintf("Usage: %.1f/%.1f GB (%.0f%%)", d.UsedGB, d.TotalGB, d.UsedPct)},
				Impact:      "Services may fail if disk fills completely. Write operations will fail.",
				Action:      "Free space immediately or expand storage.",
				Priority:    "immediate",
			})
		} else if d.UsedPct >= 90 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDisk,
				Title:       fmt.Sprintf("Low Disk Space: %s (%.0f%%)", d.MountPoint, d.UsedPct),
				Description: fmt.Sprintf("%s is at %.0f%% capacity (%.1f GB free of %.1f GB).", d.MountPoint, d.UsedPct, d.FreeGB, d.TotalGB),
				Evidence:    []string{fmt.Sprintf("Usage: %.1f/%.1f GB (%.0f%%)", d.UsedGB, d.TotalGB, d.UsedPct)},
				Impact:      "May run out of space soon if growth continues",
				Action:      "Monitor growth rate. Plan storage expansion or cleanup.",
				Priority:    "short-term",
			})
		}
	}
	return findings
}

// ---------- Docker Rules ----------

func analyzeDocker(docker internal.DockerInfo) []internal.Finding {
	var findings []internal.Finding
	if !docker.Available {
		return findings
	}

	exitedCount := 0
	highCPU := 0
	highMem := 0

	for _, c := range docker.Containers {
		if c.State == "exited" || c.State == "dead" {
			exitedCount++
		}
		if c.CPU > 80 {
			highCPU++
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("High CPU: Container '%s' (%.0f%%)", c.Name, c.CPU),
				Description: fmt.Sprintf("Container '%s' (%s) is using %.1f%% CPU.", c.Name, c.Image, c.CPU),
				Evidence:    []string{fmt.Sprintf("CPU: %.1f%%, Memory: %.0f MB", c.CPU, c.MemMB)},
				Impact:      "May starve other containers and system processes",
				Action:      "Check if the container is healthy. Set CPU limits if needed.",
				Priority:    "short-term",
			})
		}
		if c.MemPct > 80 {
			highMem++
		}
	}

	if exitedCount > 3 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityInfo,
			Category:    internal.CategoryDocker,
			Title:       fmt.Sprintf("%d Stopped Containers", exitedCount),
			Description: fmt.Sprintf("%d containers are in exited/dead state. These may be failed or unused.", exitedCount),
			Impact:      "Wasted disk space, potential confusion in management",
			Action:      "Review stopped containers. Remove ones that aren't needed: docker container prune",
			Priority:    "medium-term",
		})
	}

	return findings
}

// ---------- Network Rules ----------

func analyzeNetwork(net internal.NetworkInfo) []internal.Finding {
	var findings []internal.Finding
	for _, iface := range net.Interfaces {
		if iface.State == "DOWN" && !strings.HasPrefix(iface.Name, "wl") { // ignore wifi being down
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryNetwork,
				Title:       fmt.Sprintf("Network Interface Down: %s", iface.Name),
				Description: fmt.Sprintf("Interface %s is in DOWN state.", iface.Name),
				Impact:      "Network connectivity may be affected",
				Action:      "Check cable connection and switch port.",
				Priority:    "short-term",
			})
		}
		// Check for 100Mb/s on what should be GbE
		if iface.Speed == "100Mb/s" && (strings.HasPrefix(iface.Name, "eth") || strings.HasPrefix(iface.Name, "en")) {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryNetwork,
				Title:       fmt.Sprintf("Slow Link Speed: %s at 100Mb/s", iface.Name),
				Description: fmt.Sprintf("Interface %s is negotiated at 100Mb/s instead of 1Gb/s or higher. This is usually caused by a bad cable or switch port.", iface.Name),
				Evidence:    []string{fmt.Sprintf("Speed: %s", iface.Speed)},
				Impact:      "Network transfers capped at ~12 MB/s instead of ~120 MB/s",
				Action:      "Replace Ethernet cable. Check switch port.",
				Priority:    "short-term",
				Cost:        "$5-15 for Cat6 cable",
			})
		}
	}
	return findings
}

// ---------- Log Rules ----------

func analyzeLogs(logs internal.LogInfo) []internal.Finding {
	var findings []internal.Finding

	// Count ATA/SATA errors in dmesg
	ataErrors := 0
	ioErrors := 0
	for _, entry := range logs.DmesgErrors {
		msg := strings.ToLower(entry.Message)
		if strings.Contains(msg, "ata") && (strings.Contains(msg, "error") || strings.Contains(msg, "reset") || strings.Contains(msg, "timeout")) {
			ataErrors++
		}
		if strings.Contains(msg, "i/o error") || strings.Contains(msg, "medium error") {
			ioErrors++
		}
	}

	if ataErrors > 10 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityCritical,
			Category:    internal.CategoryLogs,
			Title:       fmt.Sprintf("Frequent ATA/SATA Errors (%d occurrences)", ataErrors),
			Description: "Kernel logs show repeated ATA/SATA errors. This indicates a hardware issue — typically a failing SATA cable, disk, or controller.",
			Evidence:    collectEvidence(logs.DmesgErrors, "ata", 5),
			Impact:      "Data corruption risk, slow I/O, system instability",
			Action:      "Check SATA cables and connections. Cross-reference with SMART data to identify the affected drive.",
			Priority:    "immediate",
		})
	}

	if ioErrors > 5 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityCritical,
			Category:    internal.CategoryLogs,
			Title:       fmt.Sprintf("I/O Errors Detected (%d occurrences)", ioErrors),
			Description: "Kernel logs show I/O errors. This means the system is unable to read or write to a disk.",
			Evidence:    collectEvidence(logs.DmesgErrors, "i/o", 5),
			Impact:      "Data loss risk, application failures",
			Action:      "Identify the affected drive from the error messages. Check SMART health.",
			Priority:    "immediate",
		})
	}

	return findings
}

func collectEvidence(entries []internal.LogEntry, keyword string, max int) []string {
	var evidence []string
	keyword = strings.ToLower(keyword)
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Message), keyword) {
			evidence = append(evidence, e.Message)
			if len(evidence) >= max {
				break
			}
		}
	}
	return evidence
}

// ---------- Parity Rules (Unraid) ----------

func analyzeParity(parity *internal.ParityInfo) []internal.Finding {
	var findings []internal.Finding
	if parity == nil || len(parity.History) < 2 {
		return findings
	}

	// Compare first and last parity check speeds
	first := parity.History[0]
	last := parity.History[len(parity.History)-1]

	if first.SpeedMBs > 0 && last.SpeedMBs > 0 {
		degradation := (first.SpeedMBs - last.SpeedMBs) / first.SpeedMBs * 100
		if degradation > 50 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryParity,
				Title:       "Severe Parity Check Speed Degradation",
				Description: fmt.Sprintf("Parity check speed has degraded by %.0f%% (from %.0f MB/s to %.0f MB/s). This is a strong indicator of a hardware issue.", degradation, first.SpeedMBs, last.SpeedMBs),
				Evidence:    []string{fmt.Sprintf("Earliest: %.0f MB/s (%s)", first.SpeedMBs, first.Date), fmt.Sprintf("Latest: %.0f MB/s (%s)", last.SpeedMBs, last.Date)},
				Impact:      "Parity checks take much longer, array is unprotected for extended periods",
				Action:      "Check SATA cables, drive health, and controller. The slowest drive/cable is the bottleneck.",
				Priority:    "immediate",
			})
		} else if degradation > 25 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryParity,
				Title:       "Parity Check Speed Declining",
				Description: fmt.Sprintf("Parity check speed has dropped %.0f%% (from %.0f MB/s to %.0f MB/s).", degradation, first.SpeedMBs, last.SpeedMBs),
				Evidence:    []string{fmt.Sprintf("Earliest: %.0f MB/s (%s)", first.SpeedMBs, first.Date), fmt.Sprintf("Latest: %.0f MB/s (%s)", last.SpeedMBs, last.Date)},
				Impact:      "Longer parity checks, reduced array performance",
				Action:      "Monitor trend. Check SATA cables if degradation continues.",
				Priority:    "short-term",
			})
		}
	}

	// Check for errors in recent checks
	for _, check := range parity.History[max(0, len(parity.History)-3):] {
		if check.Errors > 0 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryParity,
				Title:       fmt.Sprintf("Parity Errors on %s", check.Date),
				Description: fmt.Sprintf("Parity check on %s found %d errors (action: %s).", check.Date, check.Errors, check.Action),
				Evidence:    []string{fmt.Sprintf("Errors: %d, Duration: %ds, Speed: %.0f MB/s", check.Errors, check.Duration, check.SpeedMBs)},
				Impact:      "Parity data is inconsistent. Array protection is compromised.",
				Action:      "Run a correcting parity check. Investigate which drive has bad data.",
				Priority:    "immediate",
			})
		}
	}

	return findings
}

// ---------- Cross-Correlation ----------

func correlate(findings []internal.Finding, snap *internal.Snapshot) []internal.Finding {
	hasSATACable := false
	hasHighTemp := false
	hasHighIOWait := false
	hasNoCache := true
	hasSlowParity := false

	for _, f := range findings {
		if strings.Contains(f.Title, "SATA Cable") || strings.Contains(f.Title, "UDMA CRC") {
			hasSATACable = true
		}
		if f.Category == internal.CategoryThermal && f.Severity == internal.SeverityCritical {
			hasHighTemp = true
		}
		if strings.Contains(f.Title, "I/O") && f.Category == internal.CategoryDisk {
			hasHighIOWait = true
		}
		if strings.Contains(f.Title, "Parity") && strings.Contains(f.Title, "Degradation") {
			hasSlowParity = true
		}
	}

	// Check for cache drive
	for _, d := range snap.Disks {
		if strings.Contains(strings.ToLower(d.Label), "cache") || strings.Contains(d.MountPoint, "cache") {
			hasNoCache = false
		}
	}
	for _, d := range snap.SMART {
		if d.DiskType == "ssd" || d.DiskType == "nvme" {
			hasNoCache = false
		}
	}

	// SATA cable + slow parity = cable is the root cause
	if hasSATACable && hasSlowParity {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityCritical,
			Category:    internal.CategorySystem,
			Title:       "Root Cause: SATA Cable Failure Causing Parity Degradation",
			Description: "UDMA CRC errors are directly correlated with parity check speed degradation. A failing SATA cable forces the controller to retry operations, dramatically slowing array-wide operations.",
			Impact:      "Until the cable is replaced, parity checks and array performance will continue to degrade.",
			Action:      "Replace the affected SATA cable(s). This is the #1 priority fix.",
			Priority:    "immediate",
			Cost:        "$5-15",
		})
	}

	// High temps + slow parity = thermal throttling
	if hasHighTemp && hasSlowParity {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityWarning,
			Category:    internal.CategorySystem,
			Title:       "Correlation: High Temperatures May Be Affecting Parity Speed",
			Description: "Multiple drives are running hot, which can cause thermal throttling and reduced I/O performance.",
			Impact:      "Drives may throttle to protect themselves, slowing array operations.",
			Action:      "Address cooling before evaluating parity performance further.",
			Priority:    "immediate",
			Cost:        "$20-50 for fans",
		})
	}

	// No cache + high IO wait + Docker = I/O starvation
	if hasNoCache && hasHighIOWait && snap.Docker.Available && len(snap.Docker.Containers) > 3 {
		findings = append(findings, internal.Finding{
			Severity:    internal.SeverityWarning,
			Category:    internal.CategorySystem,
			Title:       "No SSD Cache with Docker Workloads",
			Description: fmt.Sprintf("Running %d Docker containers without an SSD cache drive. All container I/O goes to the array's spinning disks, creating I/O contention.", len(snap.Docker.Containers)),
			Impact:      "Docker containers compete with array operations for disk I/O, causing overall slowness.",
			Action:      "Add an SSD or NVMe cache drive. Move Docker appdata to the cache.",
			Priority:    "short-term",
			Cost:        "$50-150 for SSD cache",
		})
	}

	return findings
}

// ---------- Helpers ----------

func estimateDriveCost(d internal.SMARTInfo) string {
	if d.DiskType == "nvme" {
		if d.SizeGB > 1500 {
			return "$100-200"
		}
		return "$50-100"
	}
	if d.SizeGB > 10000 {
		return "$200-350"
	}
	if d.SizeGB > 4000 {
		return "$100-180"
	}
	return "$60-120"
}

func priorityFromSeverity(s internal.Severity) string {
	switch s {
	case internal.SeverityCritical:
		return "immediate"
	case internal.SeverityWarning:
		return "short-term"
	default:
		return "medium-term"
	}
}
