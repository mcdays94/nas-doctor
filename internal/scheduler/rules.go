package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// evaluateRule checks a single notification rule against the snapshot and
// returns matching findings (real or synthetic).
func evaluateRule(rule internal.NotificationRule, snap *internal.Snapshot) []internal.Finding {
	cat := strings.ToLower(rule.Category)
	cond := strings.ToLower(rule.Condition)
	target := strings.ToLower(strings.TrimSpace(rule.Target))
	val := parseFloat(rule.Value)

	switch cat {
	case "findings":
		return evalFindings(cond, target, snap.Findings)
	case "disk_space":
		return evalDiskSpace(cond, target, val, snap.Disks)
	case "disk_temp":
		return evalDiskTemp(cond, target, int(val), snap.SMART)
	case "smart":
		return evalSMART(cond, target, int64(val), snap.SMART)
	case "service":
		return evalService(cond, target, val, snap.Services)
	case "parity":
		return evalParity(cond, val, snap.Parity)
	case "ups":
		return evalUPS(cond, val, snap.UPS)
	case "docker":
		return evalDocker(cond, target, snap.Docker)
	case "system":
		return evalSystem(cond, val, snap.System)
	case "zfs":
		return evalZFS(cond, target, val, snap.ZFS)
	case "tunnels":
		return evalTunnels(cond, target, snap.Tunnels)
	case "update":
		return evalUpdate(snap.Update)
	}
	return nil
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func synth(id string, sev internal.Severity, cat internal.Category, title, desc string) internal.Finding {
	return internal.Finding{ID: id, Severity: sev, Category: cat, Title: title, Description: desc}
}

func matchTarget(target, candidate string) bool {
	if target == "" {
		return true
	}
	return strings.EqualFold(target, strings.TrimSpace(candidate))
}

// -- Category evaluators --

func evalFindings(cond, target string, findings []internal.Finding) []internal.Finding {
	var out []internal.Finding
	for _, f := range findings {
		switch cond {
		case "critical":
			if f.Severity == internal.SeverityCritical {
				out = append(out, f)
			}
		case "warning":
			if f.Severity == internal.SeverityWarning || f.Severity == internal.SeverityCritical {
				out = append(out, f)
			}
		case "category":
			if strings.EqualFold(string(f.Category), target) {
				out = append(out, f)
			}
		case "any":
			out = append(out, f)
		}
	}
	return out
}

func evalDiskSpace(cond, target string, val float64, disks []internal.DiskInfo) []internal.Finding {
	if val <= 0 {
		return nil
	}
	var out []internal.Finding
	for _, d := range disks {
		if !matchTarget(target, d.MountPoint) && !matchTarget(target, d.Label) {
			continue
		}
		free := 100.0 - d.UsedPct
		if free < val {
			out = append(out, synth("rule:disk-space:"+d.MountPoint, internal.SeverityWarning, internal.CategoryDisk,
				"Disk space low: "+d.MountPoint, fmt.Sprintf("Free space %.1f%% is below threshold %.0f%%", free, val)))
		}
	}
	return out
}

func evalDiskTemp(cond, target string, val int, smart []internal.SMARTInfo) []internal.Finding {
	if val <= 0 {
		return nil
	}
	var out []internal.Finding
	switch cond {
	case "above", "single_above":
		for _, s := range smart {
			if !matchTarget(target, s.Serial) && !matchTarget(target, s.Device) {
				continue
			}
			if s.Temperature > val {
				out = append(out, synth("rule:disk-temp:"+s.Serial, internal.SeverityWarning, internal.CategoryThermal,
					"Disk temperature high: "+s.Device, fmt.Sprintf("%d°C exceeds threshold %d°C", s.Temperature, val)))
			}
		}
	case "avg_above":
		if len(smart) == 0 {
			return nil
		}
		sum := 0
		for _, s := range smart {
			sum += s.Temperature
		}
		avg := sum / len(smart)
		if avg > val {
			out = append(out, synth("rule:avg-disk-temp", internal.SeverityWarning, internal.CategoryThermal,
				"Average disk temperature high", fmt.Sprintf("Average %d°C exceeds threshold %d°C", avg, val)))
		}
	}
	return out
}

func evalSMART(cond, target string, val int64, smart []internal.SMARTInfo) []internal.Finding {
	var out []internal.Finding
	for _, s := range smart {
		if !matchTarget(target, s.Serial) && !matchTarget(target, s.Device) {
			continue
		}
		switch cond {
		case "health_fails":
			if !s.HealthPassed {
				out = append(out, synth("rule:smart-fail:"+s.Serial, internal.SeverityCritical, internal.CategorySMART,
					"SMART health failed: "+s.Device, s.Model+" (S/N: "+s.Serial+")"))
			}
		case "reallocated_above":
			if val > 0 && s.Reallocated > val {
				out = append(out, synth("rule:smart-realloc:"+s.Serial, internal.SeverityCritical, internal.CategorySMART,
					"Reallocated sectors high: "+s.Device, fmt.Sprintf("%d exceeds threshold %d", s.Reallocated, val)))
			}
		case "pending_above":
			if val > 0 && s.Pending > val {
				out = append(out, synth("rule:smart-pending:"+s.Serial, internal.SeverityWarning, internal.CategorySMART,
					"Pending sectors: "+s.Device, fmt.Sprintf("%d exceeds threshold %d", s.Pending, val)))
			}
		case "crc_above":
			if val > 0 && s.UDMACRC > val {
				out = append(out, synth("rule:smart-crc:"+s.Serial, internal.SeverityWarning, internal.CategorySMART,
					"UDMA CRC errors: "+s.Device, fmt.Sprintf("%d exceeds threshold %d", s.UDMACRC, val)))
			}
		case "power_hours_above":
			if val > 0 && s.PowerOnHours > val {
				out = append(out, synth("rule:smart-age:"+s.Serial, internal.SeverityInfo, internal.CategorySMART,
					"Drive age warning: "+s.Device, fmt.Sprintf("%.1f years (%.0f hours threshold)", float64(s.PowerOnHours)/8766, float64(val))))
			}
		}
	}
	return out
}

func evalService(cond, target string, val float64, services []internal.ServiceCheckResult) []internal.Finding {
	var out []internal.Finding
	for _, sc := range services {
		if !matchTarget(target, sc.Name) && !matchTarget(target, sc.Key) {
			continue
		}
		switch cond {
		case "down":
			if sc.Status == "down" {
				out = append(out, synth("rule:svc-down:"+sc.Key, internal.SeverityWarning, internal.CategoryService,
					"Service down: "+sc.Name, fmt.Sprintf("%s (%s) — %s", sc.Target, sc.Type, sc.Error)))
			}
		case "degraded":
			if sc.Status == "degraded" {
				out = append(out, synth("rule:svc-degraded:"+sc.Key, internal.SeverityWarning, internal.CategoryService,
					"Service degraded: "+sc.Name, fmt.Sprintf("%s (%s) — %s", sc.Target, sc.Type, sc.Error)))
			}
		case "latency_above":
			if val > 0 && float64(sc.ResponseMS) > val {
				out = append(out, synth("rule:svc-latency:"+sc.Key, internal.SeverityWarning, internal.CategoryService,
					"Service latency high: "+sc.Name, fmt.Sprintf("%dms exceeds threshold %.0fms", sc.ResponseMS, val)))
			}
		case "download_below":
			if sc.Type == internal.ServiceCheckSpeed && val > 0 && sc.DownloadMbps < val {
				out = append(out, synth("rule:svc-dl:"+sc.Key, internal.SeverityWarning, internal.CategorySpeedTest,
					"Download speed below threshold: "+sc.Name, fmt.Sprintf("%.0f Mbps < %.0f Mbps threshold", sc.DownloadMbps, val)))
			}
		case "upload_below":
			if sc.Type == internal.ServiceCheckSpeed && val > 0 && sc.UploadMbps < val {
				out = append(out, synth("rule:svc-ul:"+sc.Key, internal.SeverityWarning, internal.CategorySpeedTest,
					"Upload speed below threshold: "+sc.Name, fmt.Sprintf("%.0f Mbps < %.0f Mbps threshold", sc.UploadMbps, val)))
			}
		}
	}
	return out
}

func evalParity(cond string, val float64, parity *internal.ParityInfo) []internal.Finding {
	if parity == nil || len(parity.History) == 0 {
		return nil
	}
	last := parity.History[len(parity.History)-1]
	switch cond {
	case "errors":
		if last.Errors > 0 {
			return []internal.Finding{synth("rule:parity-err:"+last.Date, internal.SeverityCritical, internal.CategoryParity,
				"Parity check errors: "+last.Date, fmt.Sprintf("%d errors found", last.Errors))}
		}
	case "speed_below":
		if val > 0 && last.SpeedMBs < val {
			return []internal.Finding{synth("rule:parity-slow:"+last.Date, internal.SeverityWarning, internal.CategoryParity,
				"Parity check slow", fmt.Sprintf("%.1f MB/s below threshold %.0f MB/s", last.SpeedMBs, val))}
		}
	}
	return nil
}

func evalUPS(cond string, val float64, ups *internal.UPSInfo) []internal.Finding {
	if ups == nil || !ups.Available {
		return nil
	}
	switch cond {
	case "on_battery":
		if ups.OnBattery {
			return []internal.Finding{synth("rule:ups-battery", internal.SeverityCritical, internal.CategoryUPS,
				"UPS on battery", fmt.Sprintf("Battery %.0f%%, runtime %.0f min", ups.BatteryPct, ups.RuntimeMins))}
		}
	case "battery_below":
		if val > 0 && ups.BatteryPct < val {
			return []internal.Finding{synth("rule:ups-low", internal.SeverityCritical, internal.CategoryUPS,
				"UPS battery low", fmt.Sprintf("%.0f%% below threshold %.0f%%", ups.BatteryPct, val))}
		}
	case "load_above":
		if val > 0 && ups.LoadPct > val {
			return []internal.Finding{synth("rule:ups-load", internal.SeverityWarning, internal.CategoryUPS,
				"UPS load high", fmt.Sprintf("%.0f%% exceeds threshold %.0f%%", ups.LoadPct, val))}
		}
	}
	return nil
}

func evalDocker(cond, target string, docker internal.DockerInfo) []internal.Finding {
	var out []internal.Finding
	for _, c := range docker.Containers {
		if !matchTarget(target, c.Name) {
			continue
		}
		switch cond {
		case "stopped":
			if c.State != "running" {
				out = append(out, synth("rule:docker-stop:"+c.Name, internal.SeverityWarning, internal.CategoryDocker,
					"Container stopped: "+c.Name, c.Image+" — state: "+c.State))
			}
		}
	}
	return out
}

func evalSystem(cond string, val float64, sys internal.SystemInfo) []internal.Finding {
	if val <= 0 {
		return nil
	}
	switch cond {
	case "cpu_above":
		if sys.CPUUsage > val {
			return []internal.Finding{synth("rule:sys-cpu", internal.SeverityWarning, internal.CategorySystem,
				"CPU usage high", fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", sys.CPUUsage, val))}
		}
	case "mem_above":
		if sys.MemPercent > val {
			return []internal.Finding{synth("rule:sys-mem", internal.SeverityWarning, internal.CategorySystem,
				"Memory usage high", fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", sys.MemPercent, val))}
		}
	case "load_above":
		if sys.LoadAvg1 > val {
			return []internal.Finding{synth("rule:sys-load", internal.SeverityWarning, internal.CategorySystem,
				"Load average high", fmt.Sprintf("%.2f exceeds threshold %.0f", sys.LoadAvg1, val))}
		}
	case "iowait_above":
		if sys.IOWait > val {
			return []internal.Finding{synth("rule:sys-iowait", internal.SeverityWarning, internal.CategorySystem,
				"I/O wait high", fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", sys.IOWait, val))}
		}
	}
	return nil
}

func evalZFS(cond, target string, val float64, zfs *internal.ZFSInfo) []internal.Finding {
	if zfs == nil || !zfs.Available {
		return nil
	}
	var out []internal.Finding
	for _, pool := range zfs.Pools {
		if !matchTarget(target, pool.Name) {
			continue
		}
		switch cond {
		case "degraded":
			if !strings.EqualFold(pool.State, "ONLINE") {
				out = append(out, synth("rule:zfs-degraded:"+pool.Name, internal.SeverityCritical, internal.CategoryZFS,
					"ZFS pool degraded: "+pool.Name, "State: "+pool.State))
			}
		case "scrub_errors":
			if pool.ScanErrors > 0 {
				out = append(out, synth("rule:zfs-scrub-err:"+pool.Name, internal.SeverityCritical, internal.CategoryZFS,
					"ZFS scrub errors: "+pool.Name, fmt.Sprintf("%d errors found", pool.ScanErrors)))
			}
		case "usage_above":
			if val > 0 && pool.UsedPct > val {
				out = append(out, synth("rule:zfs-usage:"+pool.Name, internal.SeverityWarning, internal.CategoryZFS,
					"ZFS pool usage high: "+pool.Name, fmt.Sprintf("%.1f%% exceeds threshold %.0f%%", pool.UsedPct, val)))
			}
		}
	}
	return out
}

func evalTunnels(cond, target string, tunnels *internal.TunnelInfo) []internal.Finding {
	if tunnels == nil {
		return nil
	}
	var out []internal.Finding
	switch cond {
	case "cloudflared_down":
		if tunnels.Cloudflared != nil {
			for _, t := range tunnels.Cloudflared.Tunnels {
				if !matchTarget(target, t.Name) {
					continue
				}
				if t.Status != "healthy" {
					out = append(out, synth("rule:cf-down:"+t.Name, internal.SeverityWarning, internal.CategoryNetwork,
						"Cloudflared tunnel down: "+t.Name, "Status: "+t.Status))
				}
			}
		}
	case "tailscale_offline":
		if tunnels.Tailscale != nil {
			all := tunnels.Tailscale.Peers
			for _, nd := range all {
				if !matchTarget(target, nd.Name) {
					continue
				}
				if !nd.Online {
					out = append(out, synth("rule:ts-offline:"+nd.Name, internal.SeverityWarning, internal.CategoryNetwork,
						"Tailscale node offline: "+nd.Name, "IP: "+nd.IP))
				}
			}
		}
	}
	return out
}

func evalUpdate(update *internal.UpdateInfo) []internal.Finding {
	if update != nil && update.UpdateAvailable {
		return []internal.Finding{synth("rule:update", internal.SeverityInfo, internal.CategorySystem,
			"Platform update available", fmt.Sprintf("%s → %s", update.InstalledVersion, update.LatestVersion))}
	}
	return nil
}

// -- Suppression helpers --

func matchesHostname(hosts []string, hostname string) bool {
	if len(hosts) == 0 {
		return true
	}
	h := strings.ToLower(strings.TrimSpace(hostname))
	for _, c := range hosts {
		if strings.ToLower(strings.TrimSpace(c)) == h {
			return true
		}
	}
	return false
}

func severityRank(sev internal.Severity) int {
	switch sev {
	case internal.SeverityCritical:
		return 3
	case internal.SeverityWarning:
		return 2
	case internal.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func inQuietHours(cfg QuietHours, now time.Time) bool {
	if !cfg.Enabled {
		return false
	}
	start, err := parseHHMM(cfg.StartHHMM)
	if err != nil {
		return false
	}
	end, err := parseHHMM(cfg.EndHHMM)
	if err != nil {
		return false
	}
	if start == end {
		return false
	}

	loc := time.UTC
	if cfg.Timezone != "" {
		if loaded, err := time.LoadLocation(cfg.Timezone); err == nil {
			loc = loaded
		}
	}
	localNow := now.In(loc)
	mins := localNow.Hour()*60 + localNow.Minute()

	if start < end {
		return mins >= start && mins < end
	}
	return mins >= start || mins < end
}

func parseHHMM(v string) (int, error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time")
	}
	h, err := time.Parse("15:04", fmt.Sprintf("%s:%s", parts[0], parts[1]))
	if err != nil {
		return 0, err
	}
	return h.Hour()*60 + h.Minute(), nil
}

func inMaintenanceWindow(windows []MaintenanceWindow, hostname string, now time.Time) bool {
	for _, w := range windows {
		if !w.Enabled {
			continue
		}
		if !matchesHostname(w.Hostnames, hostname) {
			continue
		}
		start, err1 := time.Parse(time.RFC3339, strings.TrimSpace(w.StartISO))
		end, err2 := time.Parse(time.RFC3339, strings.TrimSpace(w.EndISO))
		if err1 != nil || err2 != nil || !end.After(start) {
			continue
		}
		if (now.Equal(start) || now.After(start)) && now.Before(end) {
			return true
		}
	}
	return false
}

// -- Fingerprinting & counting --

func findingFingerprint(f internal.Finding) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(string(f.Category))),
		strings.ToLower(strings.TrimSpace(f.Title)),
		strings.ToLower(strings.TrimSpace(f.RelatedDisk)),
	}
	raw := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func countSeverity(findings []internal.Finding, sev internal.Severity) int {
	count := 0
	for _, f := range findings {
		if f.Severity == sev {
			count++
		}
	}
	return count
}

// -- SMART trend helpers --

func estimateTrendCost(urgency string) string {
	switch urgency {
	case "immediate":
		return "$80-350 for replacement drive"
	case "short-term":
		return "$5-15 (cable) or drive replacement planning"
	default:
		return "none"
	}
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
