package collector

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// SMARTConfig controls SMART-collector behaviour that may need to change
// based on user preference. See issue #198 for the v0.9.5 default shift.
type SMARTConfig struct {
	// WakeDrives, when true, instructs smartctl to read SMART attributes
	// even on spun-down drives — the v0.9.4 and earlier behaviour. When
	// false (the new default), smartctl is invoked with `-n standby`,
	// which causes it to skip (exit 2) any drive currently in standby.
	// Users who prefer every-cycle SMART reads can opt back in via the
	// Settings → Advanced UI.
	WakeDrives bool
}

// errDriveInStandby is returned by readSMARTDevice when smartctl reported
// that the target drive is spun down and therefore no SMART data was read.
// The SMART collector treats this as "skip silently" rather than an error
// that should create a history row or surface in logs.
var errDriveInStandby = errors.New("drive in standby; skipped SMART read")

func collectSMART(cfg SMARTConfig, logger *slog.Logger) ([]internal.SMARTInfo, error) {
	devices := discoverDrives()
	if len(devices) == 0 {
		// Fallback: try smartctl --scan. (The --scan subcommand does not
		// itself wake drives; it just enumerates what's attached, so no
		// standby flag needed here.)
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
	var skipped, standby int
	for _, dev := range devices {
		info, err := readSMARTDevice(dev, cfg.WakeDrives)
		if err != nil {
			if errors.Is(err, errDriveInStandby) {
				// Expected when `-n standby` is in effect and the drive
				// is spun down. Not an error; no history row created.
				// Emit an INFO log so operators can see per-cycle which
				// drives were skipped for standby (issue #202).
				if logger != nil {
					logger.Info("skipped SMART read: drive in standby", "device", dev)
				}
				standby++
				continue
			}
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
	// If every discovered drive is in standby and nothing else failed,
	// that's a legitimate outcome (all disks asleep); return no error and
	// an empty slice so the caller can persist an empty SMART snapshot
	// rather than treating it as a collection failure.
	if len(results) == 0 && lastErr != nil {
		return nil, fmt.Errorf("all %d drives failed SMART read (%d skipped, %d standby), last error: %w", len(devices), skipped, standby, lastErr)
	}
	return results, nil
}

func discoverDrives() []string {
	var drives []string

	// Discover /dev/sd* drives (Linux SCSI/SATA)
	matches, _ := filepath.Glob("/dev/sd[a-z]")
	drives = append(drives, matches...)
	// Multi-letter (sdaa, sdab, etc.)
	matches2, _ := filepath.Glob("/dev/sd[a-z][a-z]")
	drives = append(drives, matches2...)

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

	// Synology: /dev/sata* devices (maps to internal bays)
	sataMatches, _ := filepath.Glob("/dev/sata[0-9]")
	drives = append(drives, sataMatches...)
	sataMatches2, _ := filepath.Glob("/dev/sata[0-9][0-9]")
	drives = append(drives, sataMatches2...)

	// Discover NVMe drives
	nvmeMatches, _ := filepath.Glob("/dev/nvme[0-9]n1")
	drives = append(drives, nvmeMatches...)
	nvmeMatches2, _ := filepath.Glob("/dev/nvme[0-9][0-9]n1")
	drives = append(drives, nvmeMatches2...)

	return drives
}

// readSMARTDevice uses `smartctl --json` for reliable parsing.
// Note: smartctl returns non-zero exit codes even on success (bit-masked status).
// We check the output content instead of relying on the exit code.
//
// When wakeDrives is false (the v0.9.5+ default), each smartctl invocation
// is prefixed with `-n standby` so spun-down drives are not woken by the
// scan cycle. If smartctl reports the drive is in standby, this function
// returns errDriveInStandby, which the caller (collectSMART) treats as a
// silent skip rather than a collection failure.
func readSMARTDevice(device string, wakeDrives bool) (internal.SMARTInfo, error) {
	info := internal.SMARTInfo{Device: device}

	// smartctlArgs builds the argument slice for a smartctl call,
	// prefixing `-n standby` when the user has not opted into waking
	// spun-down drives.
	smartctlArgs := func(extra ...string) []string {
		if wakeDrives {
			return extra
		}
		// Prepend -n standby. Order matters less than presence, but we
		// keep it at the front so it's visible to anyone grepping the
		// argv of a running smartctl.
		return append([]string{"-n", "standby"}, extra...)
	}

	// Try JSON output first (smartctl 7.0+)
	// Ignore exit code — smartctl uses bitmask exit codes even for successful reads
	out, _ := execCmd("smartctl", smartctlArgs("--json=c", "-a", device)...)
	if !wakeDrives && looksLikeStandbyOutput(out) {
		return info, errDriveInStandby
	}
	if strings.Contains(out, "json_format_version") {
		return parseSMARTJSON(device, out)
	}

	// Fallback: try with SCSI-to-ATA translation (needed for some Synology/QNAP bays)
	if strings.Contains(out, "Unknown USB bridge") || strings.Contains(out, "Please specify device type") ||
		strings.Contains(out, "INQUIRY failed") || strings.Contains(out, "unable to detect device") ||
		out == "" {
		for _, devType := range []string{"sat", "auto", "scsi"} {
			out2, _ := execCmd("smartctl", smartctlArgs("--json=c", "-a", "-d", devType, device)...)
			if !wakeDrives && looksLikeStandbyOutput(out2) {
				return info, errDriveInStandby
			}
			if strings.Contains(out2, "json_format_version") {
				return parseSMARTJSON(device, out2)
			}
		}
	}

	// Check for USB bridge / unsupported device
	if strings.Contains(out, "Unknown USB bridge") || strings.Contains(out, "Please specify device type") {
		return info, fmt.Errorf("unsupported device (USB bridge): %s", device)
	}

	// Fallback to text parsing (also ignore exit code)
	out, _ = execCmd("smartctl", smartctlArgs("-a", device)...)
	if !wakeDrives && looksLikeStandbyOutput(out) {
		return info, errDriveInStandby
	}
	if out == "" {
		return info, fmt.Errorf("smartctl returned no output for %s", device)
	}
	if strings.Contains(out, "Unknown USB bridge") || strings.Contains(out, "Please specify device type") {
		return info, fmt.Errorf("unsupported device (USB bridge): %s", device)
	}
	return parseSMARTText(device, out), nil
}

// looksLikeStandbyOutput returns true when smartctl's output indicates the
// target drive is spun down and was therefore skipped under `-n standby`.
// Covers both the text-mode banner ("Device is in STANDBY mode, exit(2)")
// and the --json=c response where power_mode carries STANDBY without the
// json_format_version header that accompanies a full SMART read.
func looksLikeStandbyOutput(out string) bool {
	if out == "" {
		return false
	}
	// Text-mode marker — most common on Unraid / typical Linux installs.
	if strings.Contains(out, "STANDBY mode") || strings.Contains(out, "in standby mode") {
		return true
	}
	// JSON-mode marker: smartctl emits a small envelope with power_mode
	// set to STANDBY and no attribute table. Be conservative and require
	// the absence of json_format_version (which only appears in a full
	// read) so we don't mis-classify a model name containing "STANDBY".
	if strings.Contains(out, `"power_mode"`) &&
		strings.Contains(strings.ToUpper(out), "STANDBY") &&
		!strings.Contains(out, "json_format_version") {
		return true
	}
	return false
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
	// Mark data as available if we got meaningful attributes (a powered drive
	// always has temperature > 0 or power-on hours > 0)
	info.DataAvailable = info.Temperature > 0 || info.PowerOnHours > 0 ||
		strings.Contains(jsonStr, "\"ata_smart_attributes\"") ||
		strings.Contains(jsonStr, "\"nvme_smart_health_information_log\"")

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

// resolveArraySlot maps /dev/sdX to its Unraid array slot via emhttp.
// Only reads Unraid-specific files when running on Unraid.
func resolveArraySlot(device string) string {
	if !GetPlatform().IsUnraid() {
		return ""
	}
	devName := filepath.Base(device) // "sdb"

	// Primary: Unraid's emhttp disk assignments (host file)
	for _, path := range []string{"/var/local/emhttp/disks.ini", "/host/var/local/emhttp/disks.ini"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		currentSlot := ""
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				currentSlot = strings.Trim(line, "[]\"'")
			}
			if strings.Contains(line, "device=") && strings.Contains(line, devName) {
				return currentSlot
			}
		}
	}

	// Fallback: resolve via /sys/block/md*/slaves/ (works inside Docker with /sys mounted)
	mdMap := buildMDToPhysicalMap()
	if mdNum, ok := mdMap[devName]; ok {
		return "disk" + mdNum
	}
	// Check if this is the cache drive (mounted at /mnt/cache on a partition of this device)
	if out, err := execCmd("findmnt", "-n", "-o", "SOURCE", "/mnt/cache"); err == nil {
		cacheDev := strings.TrimRight(strings.TrimSpace(out), "0123456789") // "/dev/sdc1" -> "/dev/sdc"
		cacheDev = filepath.Base(cacheDev)                                  // "sdc"
		if cacheDev == devName {
			return "cache"
		}
	}

	return ""
}
