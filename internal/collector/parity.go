package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectParity(hp internal.HostPaths) (*internal.ParityInfo, error) {
	info := &internal.ParityInfo{}

	// Current parity status from var.ini
	// mdResyncPos > 0 means a sync is in progress
	// mdResyncAction describes the type (check, recon, etc) but persists after completion
	varIniPath := "/var/local/emhttp/var.ini"
	if data, err := os.ReadFile(varIniPath); err == nil {
		var resyncPos string
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "mdResyncPos=") {
				resyncPos = strings.Trim(strings.TrimPrefix(line, "mdResyncPos="), "\"")
			}
		}
		if resyncPos != "" && resyncPos != "0" {
			info.Status = "running"
		} else {
			info.Status = "idle"
		}
	}

	// Historical parity checks
	parityLogPaths := []string{
		filepath.Join(hp.Boot, "config/parity-checks.log"),
		"/boot/config/parity-checks.log",
	}

	for _, path := range parityLogPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		info.History = parseParityLog(string(data))
		break
	}

	if info.Status == "" {
		info.Status = "idle"
	}

	return info, nil
}

// parseParityLog parses Unraid parity-checks.log format:
// date|duration_seconds|speed|exit_code|errors|action|size
func parseParityLog(content string) []internal.ParityCheck {
	var checks []internal.ParityCheck
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 5 {
			continue
		}

		check := internal.ParityCheck{
			Date: strings.TrimSpace(fields[0]),
		}

		check.Duration, _ = strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)

		// Speed can be in different formats
		speedStr := strings.TrimSpace(fields[2])
		speedStr = strings.ReplaceAll(speedStr, ",", "")
		if strings.Contains(speedStr, "MB/s") {
			speedStr = strings.TrimSuffix(speedStr, " MB/s")
			check.SpeedMBs, _ = strconv.ParseFloat(speedStr, 64)
		} else {
			// Might be bytes/sec
			speedBytes, _ := strconv.ParseFloat(speedStr, 64)
			if speedBytes > 1000000 {
				check.SpeedMBs = speedBytes / (1024 * 1024)
			} else {
				check.SpeedMBs = speedBytes // assume already MB/s
			}
		}

		check.ExitCode, _ = strconv.Atoi(strings.TrimSpace(fields[3]))
		check.Errors, _ = strconv.Atoi(strings.TrimSpace(fields[4]))

		if len(fields) >= 6 {
			check.Action = strings.TrimSpace(fields[5])
		}
		if len(fields) >= 7 {
			sizeStr := strings.TrimSpace(fields[6])
			sizeBytes, _ := strconv.ParseFloat(sizeStr, 64)
			if sizeBytes > 0 {
				check.SizeGB = sizeBytes / (1024 * 1024 * 1024)
			}
		}

		checks = append(checks, check)
	}
	return checks
}
