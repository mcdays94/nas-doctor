package collector

import (
	"os"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectDisks() ([]internal.DiskInfo, error) {
	out, err := execCmd("df", "-h", "--output=source,fstype,size,used,avail,pcent,target")
	if err != nil {
		// Fallback to simpler df
		out, err = execCmd("df", "-h")
		if err != nil {
			return nil, err
		}
		return parseDFSimple(out), nil
	}
	return parseDFOutput(out), nil
}

func parseDFOutput(out string) []internal.DiskInfo {
	var disks []internal.DiskInfo
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		device := fields[0]
		// Skip virtual filesystems
		if isVirtualFS(device) {
			continue
		}
		mount := fields[6]
		if isVirtualMount(mount) {
			continue
		}

		total := parseSize(fields[2])
		used := parseSize(fields[3])
		free := parseSize(fields[4])
		pctStr := strings.TrimSuffix(fields[5], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		disks = append(disks, internal.DiskInfo{
			Device:     device,
			MountPoint: mount,
			Label:      guessLabel(mount, device),
			FSType:     fields[1],
			TotalGB:    total,
			UsedGB:     used,
			FreeGB:     free,
			UsedPct:    pct,
		})
	}
	return disks
}

func parseDFSimple(out string) []internal.DiskInfo {
	var disks []internal.DiskInfo
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		device := fields[0]
		if isVirtualFS(device) {
			continue
		}
		mount := fields[5]
		if isVirtualMount(mount) {
			continue
		}

		total := parseSize(fields[1])
		used := parseSize(fields[2])
		free := parseSize(fields[3])
		pctStr := strings.TrimSuffix(fields[4], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		disks = append(disks, internal.DiskInfo{
			Device:     device,
			MountPoint: mount,
			Label:      guessLabel(mount, device),
			TotalGB:    total,
			UsedGB:     used,
			FreeGB:     free,
			UsedPct:    pct,
		})
	}
	return disks
}

func isVirtualFS(device string) bool {
	prefixes := []string{"tmpfs", "devtmpfs", "shm", "overlay", "none", "sysfs", "proc", "cgroup", "udev"}
	for _, p := range prefixes {
		if strings.HasPrefix(device, p) {
			return true
		}
	}
	return false
}

func isVirtualMount(mount string) bool {
	prefixes := []string{"/sys", "/proc", "/dev/", "/run", "/snap"}
	for _, p := range prefixes {
		if strings.HasPrefix(mount, p) {
			return true
		}
	}
	return false
}

// guessLabel tries to derive a friendly label from mount and device.
func guessLabel(mount, device string) string {
	// Unraid patterns
	if strings.HasPrefix(mount, "/mnt/disk") {
		return "Disk " + strings.TrimPrefix(mount, "/mnt/disk")
	}
	if strings.HasPrefix(mount, "/mnt/cache") {
		return "Cache"
	}
	if mount == "/mnt/user" || mount == "/mnt/user0" {
		return "User Share"
	}
	// Read label from filesystem if available
	if strings.HasPrefix(device, "/dev/") {
		devName := strings.TrimPrefix(device, "/dev/")
		labelPath := "/sys/block/" + devName + "/device/../label"
		if data, err := os.ReadFile(labelPath); err == nil {
			l := strings.TrimSpace(string(data))
			if l != "" {
				return l
			}
		}
	}
	if mount == "/" {
		return "Root"
	}
	return mount
}

// parseSize converts human-readable sizes like "1.5T", "500G", "100M" to GB.
func parseSize(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	multiplier := 1.0
	if strings.HasSuffix(s, "T") || strings.HasSuffix(s, "Ti") {
		multiplier = 1024
		s = strings.TrimRight(s, "Ti")
	} else if strings.HasSuffix(s, "G") || strings.HasSuffix(s, "Gi") {
		multiplier = 1
		s = strings.TrimRight(s, "Gi")
	} else if strings.HasSuffix(s, "M") || strings.HasSuffix(s, "Mi") {
		multiplier = 1.0 / 1024
		s = strings.TrimRight(s, "Mi")
	} else if strings.HasSuffix(s, "K") || strings.HasSuffix(s, "Ki") {
		multiplier = 1.0 / (1024 * 1024)
		s = strings.TrimRight(s, "Ki")
	} else if strings.HasSuffix(s, "P") || strings.HasSuffix(s, "Pi") {
		multiplier = 1024 * 1024
		s = strings.TrimRight(s, "Pi")
	}

	val, _ := strconv.ParseFloat(s, 64)
	return val * multiplier
}
