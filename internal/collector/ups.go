// UPS monitoring via NUT (upsc) or apcupsd (apcaccess).
package collector

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// collectUPS tries NUT first, then apcupsd. Returns nil if neither is available.
func collectUPS() (*internal.UPSInfo, error) {
	// Try NUT first (more common on TrueNAS, Synology)
	if info, err := collectNUT(); err == nil && info != nil {
		return info, nil
	}

	// Try apcupsd (common on Unraid)
	if info, err := collectApcupsd(); err == nil && info != nil {
		return info, nil
	}

	return &internal.UPSInfo{Available: false}, nil
}

// ── NUT (Network UPS Tools) via `upsc` ──────────────────────────────

func collectNUT() (*internal.UPSInfo, error) {
	// Get list of UPS devices
	if _, err := exec.LookPath("upsc"); err != nil {
		return nil, err
	}

	// Try to list UPS devices
	listOut, err := exec.Command("upsc", "-l").Output()
	if err != nil {
		return nil, err
	}

	upsName := ""
	for _, line := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			upsName = name
			break // use first UPS
		}
	}
	if upsName == "" {
		return nil, nil
	}

	// Get UPS data
	out, err := exec.Command("upsc", upsName).Output()
	if err != nil {
		return nil, err
	}

	return parseNUT(upsName, string(out)), nil
}

// parseNUT parses `upsc <name>` output (key: value pairs).
func parseNUT(name, output string) *internal.UPSInfo {
	vals := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			vals[key] = val
		}
	}

	info := &internal.UPSInfo{
		Available: true,
		Source:    "nut",
		Name:      name,
		Model:     vals["ups.model"],
	}

	// Status
	info.Status = vals["ups.status"]
	info.StatusHuman = nutStatusToHuman(info.Status)
	info.OnBattery = strings.Contains(info.Status, "OB")
	info.LowBattery = strings.Contains(info.Status, "LB")

	// Battery
	info.BatteryPct = parseF(vals["battery.charge"])
	info.BatteryV = parseF(vals["battery.voltage"])
	info.RuntimeMins = parseF(vals["battery.runtime"]) / 60 // NUT reports in seconds

	// Input/Output voltage
	info.InputV = parseF(vals["input.voltage"])
	info.OutputV = parseF(vals["output.voltage"])

	// Load
	info.LoadPct = parseF(vals["ups.load"])

	// Power
	info.NominalW = parseF(vals["ups.realpower.nominal"])
	if info.NominalW > 0 && info.LoadPct > 0 {
		info.WattageW = info.NominalW * info.LoadPct / 100
	}

	// Temperature
	info.Temperature = parseF(vals["ups.temperature"])

	// Transfer reason
	info.LastTransfer = vals["input.transfer.reason"]

	return info
}

func nutStatusToHuman(status string) string {
	parts := strings.Fields(status)
	var labels []string
	for _, p := range parts {
		switch p {
		case "OL":
			labels = append(labels, "Online")
		case "OB":
			labels = append(labels, "On Battery")
		case "LB":
			labels = append(labels, "Low Battery")
		case "HB":
			labels = append(labels, "High Battery")
		case "RB":
			labels = append(labels, "Replace Battery")
		case "CHRG":
			labels = append(labels, "Charging")
		case "DISCHRG":
			labels = append(labels, "Discharging")
		case "BYPASS":
			labels = append(labels, "Bypass")
		case "CAL":
			labels = append(labels, "Calibrating")
		case "OFF":
			labels = append(labels, "Offline")
		case "OVER":
			labels = append(labels, "Overloaded")
		case "TRIM":
			labels = append(labels, "Trimming")
		case "BOOST":
			labels = append(labels, "Boosting")
		case "FSD":
			labels = append(labels, "Forced Shutdown")
		default:
			labels = append(labels, p)
		}
	}
	if len(labels) == 0 {
		return "Unknown"
	}
	return strings.Join(labels, ", ")
}

// ── apcupsd via `apcaccess` ─────────────────────────────────────────

func collectApcupsd() (*internal.UPSInfo, error) {
	if _, err := exec.LookPath("apcaccess"); err != nil {
		return nil, err
	}

	out, err := exec.Command("apcaccess").Output()
	if err != nil {
		return nil, err
	}

	return parseApcaccess(string(out)), nil
}

// parseApcaccess parses `apcaccess` output (KEY : VALUE pairs).
func parseApcaccess(output string) *internal.UPSInfo {
	vals := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			vals[key] = val
		}
	}

	info := &internal.UPSInfo{
		Available: true,
		Source:    "apcupsd",
		Name:      strings.TrimSpace(vals["UPSNAME"]),
		Model:     strings.TrimSpace(vals["MODEL"]),
	}

	// Status
	info.Status = strings.TrimSpace(vals["STATUS"])
	switch info.Status {
	case "ONLINE":
		info.StatusHuman = "Online"
	case "ONBATT":
		info.StatusHuman = "On Battery"
		info.OnBattery = true
	case "LOWBATT":
		info.StatusHuman = "Low Battery"
		info.OnBattery = true
		info.LowBattery = true
	case "COMMLOST":
		info.StatusHuman = "Communication Lost"
	default:
		info.StatusHuman = info.Status
	}

	// Battery
	info.BatteryPct = parseApcFloat(vals["BCHARGE"])
	info.BatteryV = parseApcFloat(vals["BATTV"])
	info.RuntimeMins = parseApcFloat(vals["TIMELEFT"])

	// Voltage
	info.InputV = parseApcFloat(vals["LINEV"])
	info.OutputV = parseApcFloat(vals["OUTPUTV"])

	// Load
	info.LoadPct = parseApcFloat(vals["LOADPCT"])

	// Power
	info.NominalW = parseApcFloat(vals["NOMPOWER"])
	if info.NominalW > 0 && info.LoadPct > 0 {
		info.WattageW = info.NominalW * info.LoadPct / 100
	}

	// Temperature
	info.Temperature = parseApcFloat(vals["ITEMP"])

	// Transfer reason
	info.LastTransfer = strings.TrimSpace(vals["LASTXFER"])

	return info
}

// parseApcFloat extracts a float from apcaccess values like "100.0 Percent" or "13.8 Volts".
func parseApcFloat(val string) float64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	// Take the first field (the number)
	fields := strings.Fields(val)
	if len(fields) == 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(fields[0], 64)
	return f
}

func parseF(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
