package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectDisks() ([]internal.DiskInfo, error) {
	out, err := execCmd("df", "-h", "--output=source,fstype,size,used,avail,pcent,target")
	if err != nil {
		out, err = execCmd("df", "-h")
		if err != nil {
			return nil, err
		}
		return dedupSameFilesystemMounts(parseDFSimple(out)), nil
	}
	disks := parseDFOutput(out)

	// If running inside Docker, merge host mount disks with df disks.
	// Host mounts override df entries for the same real mount point,
	// but df entries for volumes not bind-mounted (e.g. /volume2 on
	// Synology when only /volume1 is mapped) are preserved.
	if hostDisks := collectHostMountDisks(); len(hostDisks) > 0 {
		merged := make(map[string]internal.DiskInfo)
		// Start with df-detected disks (keyed by real mount point)
		for _, d := range disks {
			merged[d.MountPoint] = d
		}
		// Host-mount disks override and add new entries
		for _, d := range hostDisks {
			merged[d.MountPoint] = d
		}
		disks = make([]internal.DiskInfo, 0, len(merged))
		for _, d := range merged {
			disks = append(disks, d)
		}
	}

	return dedupSameFilesystemMounts(disks), nil
}

// dedupSameFilesystemMounts collapses entries that report the same
// physical filesystem mounted at multiple paths. Issue #300: on
// Synology DSM, the system root partition (/dev/md0) is visible at
// `/`, `/boot`, AND `/mnt`. When the typical Container Manager
// deployment bind-mounts both `/boot:/host/boot:ro` and
// `/mnt:/host/mnt:ro`, df reports both with identical Size/Used/%
// numbers, and the dashboard shows the same partition twice — eating
// vertical space and obscuring real user storage.
//
// Two entries are considered duplicates when (Device, TotalGB, UsedGB)
// match exactly. df reports the same numbers for any mount of the
// same filesystem, so this is a safe equality. The mount that "wins"
// is chosen by preferUserStoragePath: real user-storage paths
// (/volume*, /mnt/disk*, /mnt/cache, /mnt/user) beat system paths
// (/boot, /, /log) which beat anything else; ties are broken by
// shortest mount path. Non-duplicate entries pass through untouched
// and the relative ordering of survivors is preserved (no map
// iteration in the result path) so the dashboard stays
// deterministic across scans.
func dedupSameFilesystemMounts(disks []internal.DiskInfo) []internal.DiskInfo {
	if len(disks) <= 1 {
		return disks
	}
	// indexByKey records, for each (Device, TotalGB, UsedGB) tuple,
	// the index in result of the currently-winning entry. We replace
	// the entry in place when a better-named mount comes along, so
	// the original ordering is preserved as long as the first
	// occurrence of a tuple keeps its slot.
	indexByKey := make(map[string]int, len(disks))
	result := make([]internal.DiskInfo, 0, len(disks))
	for _, d := range disks {
		key := dedupKey(d)
		if idx, seen := indexByKey[key]; seen {
			if preferUserStoragePath(d, result[idx]) {
				result[idx] = d
			}
			continue
		}
		indexByKey[key] = len(result)
		result = append(result, d)
	}
	return result
}

// dedupKey is the fingerprint used to match "same filesystem mounted
// twice". Includes Device + TotalGB + UsedGB; matches the granularity
// df itself reports so we never collapse two genuinely-different
// filesystems that happen to share a device name (rare but possible
// for loop devices).
func dedupKey(d internal.DiskInfo) string {
	// Use 3 decimal places — the parseSize helper rounds to that
	// granularity already, so trailing-digit jitter from re-running df
	// can't artificially break dedup.
	return fmt.Sprintf("%s|%.3f|%.3f", d.Device, d.TotalGB, d.UsedGB)
}

// preferUserStoragePath returns true when `a` is a more useful mount
// path to display than `b` for the same filesystem. The hierarchy is:
//
//  1. real user-storage paths (/volume*, /mnt/disk*, /mnt/cache,
//     /mnt/user) beat anything else — these are what users want to
//     see in the storage section.
//  2. otherwise, shorter path wins — `/boot` is more obvious than
//     `/mnt` on a Synology system-root partition where neither is
//     "real" storage.
//  3. lexicographic tie-break for determinism.
func preferUserStoragePath(a, b internal.DiskInfo) bool {
	aReal := isUserStorageMount(a.MountPoint)
	bReal := isUserStorageMount(b.MountPoint)
	if aReal != bReal {
		return aReal
	}
	if len(a.MountPoint) != len(b.MountPoint) {
		return len(a.MountPoint) < len(b.MountPoint)
	}
	return a.MountPoint < b.MountPoint
}

// isUserStorageMount reports whether a mount path is a "real" user
// storage location worth highlighting in the dashboard. The patterns
// match the same prefixes collectHostMountDisks uses to decide what
// host bind mounts to surface, kept in sync deliberately.
func isUserStorageMount(mount string) bool {
	return strings.HasPrefix(mount, "/volume") ||
		strings.HasPrefix(mount, "/mnt/disk") ||
		strings.HasPrefix(mount, "/mnt/cache") ||
		strings.HasPrefix(mount, "/mnt/user")
}

// collectHostMountDisks reads disk info from the host's /mnt via the bind mount at /host/mnt.
// On Unraid, disks are at /mnt/disk1, /mnt/disk2, etc.
// On TrueNAS, ZFS datasets are at /mnt/poolname/dataset (e.g. /mnt/apps, /mnt/media).
func collectHostMountDisks() []internal.DiskInfo {
	// Check if /host/mnt exists (bind-mounted from host)
	out, err := execCmd("df", "-h", "--output=source,fstype,size,used,avail,pcent,target")
	if err != nil {
		return nil
	}

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
		mount := fields[6]

		// Include host-mounted paths — platform-aware prefixes:
		//   Unraid:   /host/mnt/disk1, /host/mnt/cache, /host/mnt/user
		//   TrueNAS:  /host/mnt/apps, /host/mnt/media, /host/mnt/poolname/dataset
		//   Synology: /host/volume1, /host/volume2
		isHostMount := strings.HasPrefix(mount, "/host/mnt/") ||
			strings.HasPrefix(mount, "/host/volume")
		if !isHostMount {
			continue
		}

		device := fields[0]
		fstype := fields[1]

		// Skip ZFS boot-pool system datasets (TrueNAS internal — not user data)
		if strings.HasPrefix(device, "boot-pool/") {
			continue
		}

		total := parseSize(fields[2])
		used := parseSize(fields[3])
		free := parseSize(fields[4])
		pctStr := strings.TrimSuffix(fields[5], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		// Clean up mount path: /host/mnt/disk1 → /mnt/disk1, /host/volume1 → /volume1
		displayMount := mount
		if strings.HasPrefix(mount, "/host/") {
			displayMount = "/" + strings.TrimPrefix(mount, "/host/")
		}

		disks = append(disks, internal.DiskInfo{
			Device:     device,
			MountPoint: displayMount,
			Label:      guessLabel(displayMount, device),
			FSType:     fstype,
			TotalGB:    total,
			UsedGB:     used,
			FreeGB:     free,
			UsedPct:    pct,
		})
	}
	return disks
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
		if isVirtualFS(device) {
			continue
		}
		mount := fields[6]
		if isVirtualMount(mount) {
			continue
		}
		if isContainerRootOrBind(device, mount) {
			continue
		}

		total := parseSize(fields[2])
		used := parseSize(fields[3])
		free := parseSize(fields[4])
		pctStr := strings.TrimSuffix(fields[5], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		// Relabel host bind mounts for clarity
		displayMount := mount
		if strings.HasPrefix(mount, "/host/") {
			displayMount = "/" + strings.TrimPrefix(mount, "/host/")
		}

		disks = append(disks, internal.DiskInfo{
			Device:     device,
			MountPoint: displayMount,
			Label:      guessLabel(displayMount, device),
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
		if isContainerRootOrBind(device, mount) {
			continue
		}

		total := parseSize(fields[1])
		used := parseSize(fields[2])
		free := parseSize(fields[3])
		pctStr := strings.TrimSuffix(fields[4], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		displayMount := mount
		if strings.HasPrefix(mount, "/host/") {
			displayMount = "/" + strings.TrimPrefix(mount, "/host/")
		}

		disks = append(disks, internal.DiskInfo{
			Device:     device,
			MountPoint: displayMount,
			Label:      guessLabel(displayMount, device),
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
	virtual := []string{"/sys", "/proc", "/dev/", "/run", "/snap"}
	for _, p := range virtual {
		if strings.HasPrefix(mount, p) {
			return true
		}
	}
	// Docker container bind mounts to filter out
	containerMounts := []string{"/etc/resolv.conf", "/etc/hostname", "/etc/hosts", "/etc/unraid-version"}
	for _, cm := range containerMounts {
		if mount == cm {
			return true
		}
	}
	// Unraid: /mnt/user0 is a non-cached view of the same array as /mnt/user — skip it
	if mount == "/mnt/user0" || strings.HasPrefix(mount, "/host/mnt/user0") {
		return true
	}
	return false
}

// isContainerRootOrBind detects mounts that are Docker overlay or bind mounts
// that don't represent real host disks.
func isContainerRootOrBind(device, mount string) bool {
	// Loop devices are container overlays (e.g. /dev/loop2 mounted at /)
	if strings.HasPrefix(device, "/dev/loop") {
		return true
	}
	// "shfs" is Unraid's share filesystem — this is good, keep it
	if device == "shfs" {
		return false
	}
	// Mounts at /host/* are bind mounts from the host — show them but relabel
	// Mounts at /data are the container's data volume
	if mount == "/data" {
		return true
	}
	// Synology: filter duplicate @appdata/ContainerManager sub-mounts (same device as /volume*)
	if strings.Contains(mount, "@appdata") || strings.Contains(mount, "@docker") {
		return true
	}
	// Filter /tmp mounts inside container
	if strings.HasPrefix(mount, "/tmp") {
		return true
	}
	return false
}

// buildMDToPhysicalMap reads /sys/block/md*/slaves/ to build a mapping
// from physical device (e.g. "sdb") to md device number (e.g. "1" for md1).
// This allows correlating SMART devices (/dev/sdb) to Unraid disk mounts (/mnt/disk1).
func buildMDToPhysicalMap() map[string]string {
	result := make(map[string]string) // "sdb" -> "1" (md number)
	mdDirs, _ := filepath.Glob("/sys/block/md*/slaves")
	for _, dir := range mdDirs {
		// Extract md number: /sys/block/md1/slaves -> "1"
		mdName := filepath.Base(filepath.Dir(dir)) // "md1"
		mdNum := strings.TrimPrefix(mdName, "md")

		// List slaves (physical devices)
		slaves, _ := os.ReadDir(dir)
		for _, slave := range slaves {
			name := slave.Name() // e.g. "sdb" or "sdb1"
			// Strip partition number
			devName := strings.TrimRight(name, "0123456789")
			result[devName] = mdNum
		}
	}
	return result
}

// guessLabel tries to derive a friendly label from mount and device.
func guessLabel(mount, device string) string {
	// Unraid patterns — include device for cross-reference
	if strings.HasPrefix(mount, "/mnt/disk") {
		num := strings.TrimPrefix(mount, "/mnt/disk")
		return "Disk " + num + " (" + device + ")"
	}
	if strings.HasPrefix(mount, "/mnt/cache") {
		return "Cache (" + device + ")"
	}
	if mount == "/mnt/user" {
		return "Array (User Share)"
	}
	if mount == "/mnt/user0" {
		return "Array (User Share, no cache)"
	}
	// ZFS dataset patterns (TrueNAS, Proxmox, generic ZFS)
	// Device looks like "pool/dataset" or "tank/media/movies"
	if strings.Contains(device, "/") && !strings.HasPrefix(device, "/dev/") {
		// Use the last path component as the label, capitalized
		parts := strings.Split(device, "/")
		label := parts[len(parts)-1]
		if label != "" {
			return strings.ToUpper(label[:1]) + label[1:]
		}
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
	// Synology DSM patterns — /volume1, /volume2
	if strings.HasPrefix(mount, "/volume") {
		num := strings.TrimPrefix(mount, "/volume")
		if num != "" {
			return "Volume " + num + " (" + device + ")"
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
