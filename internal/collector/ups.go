// UPS monitoring via NUT (upsc) or apcupsd (apcaccess).
//
// Environment variables for cross-platform / remote UPS support:
//
//	NAS_DOCTOR_UPS_NAME    — NUT UPS name (default: auto-detect first from `upsc -l`)
//	NAS_DOCTOR_NUT_HOST    — NUT remote host (e.g. "192.168.1.10", used as upsname@host)
//	NAS_DOCTOR_APCUPSD_HOST — apcupsd remote host:port (e.g. "192.168.1.10:3551")
package collector

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// UPSRunner abstracts binary discovery (LookPath) and command execution (Output)
// so the UPS collectors can be exercised from tests without real exec calls.
type UPSRunner interface {
	LookPath(name string) (string, error)
	Output(name string, args ...string) ([]byte, error)
}

// execRunner is the production UPSRunner backed by os/exec.
type execRunner struct{}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }
func (execRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// defaultRunner is used by the exported collectUPS / collectNUT / collectApcupsd entrypoints.
var defaultRunner UPSRunner = execRunner{}

// collectUPS tries NUT first, then apcupsd. Returns Available=false if neither is available.
func collectUPS() (*internal.UPSInfo, error) {
	// Try NUT first (works on TrueNAS, Synology, generic Linux, FreeBSD, macOS with brew)
	if info, err := collectNUTWith(defaultRunner); err == nil && info != nil {
		return info, nil
	}

	// Try apcupsd (common on Unraid, available on all Linux/FreeBSD/macOS)
	if info, err := collectApcupsdWith(defaultRunner); err == nil && info != nil {
		return info, nil
	}

	return &internal.UPSInfo{Available: false}, nil
}

// ── NUT (Network UPS Tools) via `upsc` ──────────────────────────────

func collectNUT() (*internal.UPSInfo, error) { return collectNUTWith(defaultRunner) }

func collectNUTWith(r UPSRunner) (*internal.UPSInfo, error) {
	if _, err := r.LookPath("upsc"); err != nil {
		return nil, err
	}

	nutHost := os.Getenv("NAS_DOCTOR_NUT_HOST")
	upsName := os.Getenv("NAS_DOCTOR_UPS_NAME")

	// If no explicit UPS name, auto-detect from `upsc -l [host]`
	if upsName == "" {
		listArgs := []string{"-l"}
		if nutHost != "" {
			listArgs = append(listArgs, nutHost)
		}
		listOut, err := r.Output("upsc", listArgs...)
		if err != nil {
			// Some older NUT versions use -L (capital). Try fallback.
			listOut, err = r.Output("upsc", "-L")
			if err != nil {
				// Binary is present but we can't reach a NUT server. Surface a
				// diagnostic hint so the user sees *something* in the dashboard
				// rather than a silent "no UPS" — especially important on
				// container-networked setups where the daemon is on the host.
				return unreachableHint("nut", err), nil
			}
		}

		for _, line := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
			name := strings.TrimSpace(line)
			// upsc -L output can be "upsname: description" — extract just the name
			if idx := strings.Index(name, ":"); idx > 0 {
				name = strings.TrimSpace(name[:idx])
			}
			if name != "" {
				upsName = name
				break // use first UPS
			}
		}
	}
	if upsName == "" {
		return nil, nil
	}

	// Build the UPS identifier: "upsname" or "upsname@host" for remote
	upsID := upsName
	if nutHost != "" {
		upsID = upsName + "@" + nutHost
	}

	out, err := r.Output("upsc", upsID)
	if err != nil {
		return unreachableHint("nut", err), nil
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

func collectApcupsd() (*internal.UPSInfo, error) { return collectApcupsdWith(defaultRunner) }

func collectApcupsdWith(r UPSRunner) (*internal.UPSInfo, error) {
	if _, err := r.LookPath("apcaccess"); err != nil {
		return nil, err
	}

	// Support remote apcupsd daemon via NAS_DOCTOR_APCUPSD_HOST (e.g. "192.168.1.10:3551")
	args := []string{}
	if host := os.Getenv("NAS_DOCTOR_APCUPSD_HOST"); host != "" {
		args = append(args, "-h", host)
	}

	out, err := r.Output("apcaccess", args...)
	if err != nil {
		// Binary present but daemon unreachable — typical on Unraid when the
		// apcupsd plugin isn't running or the container isn't on host network.
		// Surface a diagnostic so the UPS section shows a helpful hint instead
		// of silently disappearing.
		return unreachableHint("apcupsd", err), nil
	}

	return parseApcaccess(string(out)), nil
}

// unreachableHint builds a UPSInfo that tells the user the client binary is
// present but the daemon can't be reached (wrong host/port, service down, or
// non-host-networked container). Available=true so the dashboard renders a row.
func unreachableHint(source string, cause error) *internal.UPSInfo {
	var hint string
	switch source {
	case "apcupsd":
		hint = "apcupsd client present but daemon unreachable — check the host apcupsd plugin is running and listening on 127.0.0.1:3551, or set NAS_DOCTOR_APCUPSD_HOST"
	case "nut":
		hint = "NUT client present but server unreachable — check upsd is running, or set NAS_DOCTOR_NUT_HOST for remote setups"
	default:
		hint = "UPS client present but daemon unreachable"
	}
	if cause != nil {
		hint += " (" + strings.TrimSpace(cause.Error()) + ")"
	}
	return &internal.UPSInfo{
		Available:   true,
		Source:      source,
		Status:      "unreachable",
		StatusHuman: hint,
	}
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
