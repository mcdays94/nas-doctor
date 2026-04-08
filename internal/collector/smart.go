package collector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectSMART() ([]internal.SMARTInfo, error) {
	devices := discoverDrives()
	if len(devices) == 0 {
		// Fallback: try smartctl --scan
		out, _ := execCmd("smartctl", "--scan")
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 1 && strings.HasPrefix(fields[0], "/dev/") {
				devices = append(devices, fields[0])
			}
		}
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no drives discovered")
	}

	var results []internal.SMARTInfo
	var lastErr error
	var skipped int
	for _, dev := range devices {
		info, err := readSMARTDevice(dev)
		if err != nil {
			lastErr = err
			skipped++
			continue
		}
		// Skip entries with no useful data (empty model = failed read)
		if info.Model == "" && info.Serial == "" {
			skipped++
			continue
		}
		results = append(results, info)
	}
	if len(results) == 0 && lastErr != nil {
		return nil, fmt.Errorf("all %d drives failed SMART read (%d skipped), last error: %w", len(devices), skipped, lastErr)
	}
	return results, nil
}

func discoverDrives() []string {
	var drives []string

	// Discover /dev/sd* drives (Linux SCSI/SATA)
	matches, _ := filepath.Glob("/dev/sd[a-z]")
	drives = append(drives, matches...)

	// Discover /dev/da* drives (FreeBSD SCSI/SAS — TrueNAS CORE)
	daMatches, _ := filepath.Glob("/dev/da[0-9]")
	drives = append(drives, daMatches...)
	daMatches2, _ := filepath.Glob("/dev/da[0-9][0-9]")
	drives = append(drives, daMatches2...)

	// Discover /dev/ada* drives (FreeBSD ATA — TrueNAS CORE)
	adaMatches, _ := filepath.Glob("/dev/ada[0-9]")
	drives = append(drives, adaMatches...)
	adaMatches2, _ := filepath.Glob("/dev/ada[0-9][0-9]")
	drives = append(drives, adaMatches2...)

	// Discover NVMe drives
	nvmeMatches, _ := filepath.Glob("/dev/nvme[0-9]n1")
	drives = append(drives, nvmeMatches...)

	return drives
}

// readSMARTDevice uses `smartctl --json` for reliable parsing.
// Note: smartctl returns non-zero exit codes even on success (bit-masked status).
// We check the output content instead of relying on the exit code.
func readSMARTDevice(device string) (internal.SMARTInfo, error) {
	info := internal.SMARTInfo{Device: device}

	// Try JSON output first (smartctl 7.0+)
	// Ignore exit code — smartctl uses bitmask exit codes even for successful reads
	out, _ := execCmd("smartctl", "--json=c", "-a", device)
	if strings.Contains(out, "json_format_version") {
		return parseSMARTJSON(device, out)
	}

	// Check for USB bridge / unsupported device
	if strings.Contains(out, "Unknown USB bridge") || strings.Contains(out, "Please specify device type") {
		return info, fmt.Errorf("unsupported device (USB bridge): %s", device)
	}

	// Fallback to text parsing (also ignore exit code)
	out, _ = execCmd("smartctl", "-a", device)
	if out == "" {
		return info, fmt.Errorf("smartctl returned no output for %s", device)
	}
	if strings.Contains(out, "Unknown USB bridge") || strings.Contains(out, "Please specify device type") {
		return info, fmt.Errorf("unsupported device (USB bridge): %s", device)
	}
	return parseSMARTText(device, out), nil
}

type smartctlJSON struct {
	ModelName    string `json:"model_name"`
	SerialNumber string `json:"serial_number"`
	FirmwareVer  string `json:"firmware_version"`
	UserCapacity struct {
		Bytes int64 `json:"bytes"`
	} `json:"user_capacity"`
	SmartStatus struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	ATASmartAttributes struct {
		Table []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Value int    `json:"value"`
			Worst int    `json:"worst"`
			Raw   struct {
				Value int64  `json:"value"`
				Str   string `json:"string"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	RotationRate int `json:"rotation_rate"`
	FormFactor   struct {
		Name string `json:"name"`
	} `json:"form_factor"`
}

func parseSMARTJSON(device, out string) (internal.SMARTInfo, error) {
	info := internal.SMARTInfo{Device: device}

	// smartctl JSON output may have trailing newlines or extra content after the JSON object.
	// Find the JSON boundaries.
	start := strings.Index(out, "{")
	if start < 0 {
		return info, fmt.Errorf("no JSON object found in smartctl output for %s", device)
	}
	// Find matching closing brace
	depth := 0
	end := -1
	for i := start; i < len(out); i++ {
		if out[i] == '{' {
			depth++
		} else if out[i] == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		return info, fmt.Errorf("incomplete JSON in smartctl output for %s", device)
	}
	jsonStr := out[start:end]

	var data smartctlJSON
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return info, fmt.Errorf("JSON parse error for %s: %w", device, err)
	}

	info.Model = data.ModelName
	info.Serial = data.SerialNumber
	info.Firmware = data.FirmwareVer
	// Default to true if smart_status is not present in JSON output (some platforms
	// like Synology DSM omit it). A drive without explicit failure is assumed healthy.
	info.HealthPassed = true
	if data.SmartStatus.Passed == false {
		// Only mark as failed if smartctl explicitly included smart_status in the JSON.
		// Check if the JSON actually contained the key by looking for it in the raw string.
		if strings.Contains(jsonStr, "\"smart_status\"") {
			info.HealthPassed = false
		}
	}
	info.Temperature = data.Temperature.Current
	info.PowerOnHours = data.PowerOnTime.Hours
	info.SizeGB = float64(data.UserCapacity.Bytes) / (1024 * 1024 * 1024)

	// Determine disk type
	if strings.Contains(device, "nvme") {
		info.DiskType = "nvme"
	} else if data.RotationRate == 0 {
		info.DiskType = "ssd"
	} else {
		info.DiskType = "hdd"
	}

	// Parse SMART attributes
	for _, attr := range data.ATASmartAttributes.Table {
		switch attr.ID {
		case 5:
			info.Reallocated = attr.Raw.Value
		case 187:
			// Reported Uncorrectable - use as fallback for reallocated
		case 188:
			info.CommandTimeout = attr.Raw.Value
		case 194:
			info.Temperature = int(attr.Raw.Value & 0xFF) // lower byte is current temp
		case 196:
			// Reallocation Event Count
		case 197:
			info.Pending = attr.Raw.Value
		case 198:
			info.Offline = attr.Raw.Value
		case 199:
			info.UDMACRC = attr.Raw.Value
		case 10:
			info.SpinRetry = attr.Raw.Value
		case 1:
			info.RawReadError = attr.Raw.Value
		case 7:
			info.SeekError = attr.Raw.Value
		}
	}

	// ATA port mapping
	info.ATAPort = resolveATAPort(device)
	info.ArraySlot = resolveArraySlot(device)

	return info, nil
}

func parseSMARTText(device, out string) internal.SMARTInfo {
	info := internal.SMARTInfo{Device: device}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)

		// Device info
		if strings.HasPrefix(line, "Device Model:") || strings.HasPrefix(line, "Model Number:") {
			info.Model = extractValue(line)
		}
		if strings.HasPrefix(line, "Serial Number:") {
			info.Serial = extractValue(line)
		}
		if strings.HasPrefix(line, "Firmware Version:") {
			info.Firmware = extractValue(line)
		}
		// Only match the overall-health self-assessment line, not arbitrary lines
		// that may contain "FAILED" in attribute thresholds or other contexts.
		// Typical format: "SMART overall-health self-assessment test result: PASSED"
		if strings.Contains(line, "self-assessment") || strings.Contains(line, "overall-health") || strings.Contains(line, "SMART Health Status") {
			if strings.Contains(line, "PASSED") || strings.Contains(line, "OK") {
				info.HealthPassed = true
			} else if strings.Contains(line, "FAILED") {
				info.HealthPassed = false
			}
		}

		// Parse attribute lines (ID# ATTRIBUTE_NAME FLAGS VALUE WORST THRESH TYPE UPDATED RAW)
		fields := strings.Fields(line)
		if len(fields) >= 10 {
			id, err := strconv.Atoi(fields[0])
			if err != nil {
				continue
			}
			rawVal, _ := strconv.ParseInt(fields[9], 10, 64)

			switch id {
			case 5:
				info.Reallocated = rawVal
			case 9:
				info.PowerOnHours = rawVal
			case 10:
				info.SpinRetry = rawVal
			case 188:
				info.CommandTimeout = rawVal
			case 194:
				info.Temperature = int(rawVal)
			case 197:
				info.Pending = rawVal
			case 198:
				info.Offline = rawVal
			case 199:
				info.UDMACRC = rawVal
			case 1:
				info.RawReadError = rawVal
			case 7:
				info.SeekError = rawVal
			}
		}
	}

	if strings.Contains(device, "nvme") {
		info.DiskType = "nvme"
	} else {
		info.DiskType = "hdd" // default; hard to detect SSD from text
	}

	info.ATAPort = resolveATAPort(device)
	info.ArraySlot = resolveArraySlot(device)

	return info
}

func extractValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// resolveATAPort maps /dev/sdX to its SATA/ATA port via sysfs.
func resolveATAPort(device string) string {
	devName := filepath.Base(device)
	link, err := os.Readlink(fmt.Sprintf("/sys/block/%s/device", devName))
	if err != nil {
		return ""
	}
	// Link looks like: ../../../0:0:0:0 or points through ataX
	parts := strings.Split(link, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, "ata") {
			return p
		}
	}
	return ""
}

// resolveArraySlot maps /dev/sdX to its Unraid array slot via mdstat or emhttp.
func resolveArraySlot(device string) string {
	// Check Unraid's emhttp disk assignments
	data, err := os.ReadFile("/var/local/emhttp/disks.ini")
	if err != nil {
		return ""
	}
	devName := filepath.Base(device)
	currentSlot := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSlot = strings.Trim(line, "[]")
		}
		if strings.Contains(line, "device=") && strings.Contains(line, devName) {
			return currentSlot
		}
	}
	return ""
}
