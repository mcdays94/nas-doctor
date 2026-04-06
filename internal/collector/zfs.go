// ZFS pool and dataset collection.
// Parses output of: zpool status, zpool list, zfs list, /proc/spl/kstat/zfs/arcstats
package collector

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// collectZFS gathers ZFS pool, dataset, and ARC information.
func collectZFS() (*internal.ZFSInfo, error) {
	// Check if ZFS is available
	if _, err := exec.LookPath("zpool"); err != nil {
		return &internal.ZFSInfo{Available: false}, nil
	}

	info := &internal.ZFSInfo{Available: true}

	// Collect pool info (zpool list + zpool status)
	pools, err := collectZPools()
	if err != nil {
		return info, fmt.Errorf("zpool collection: %w", err)
	}
	info.Pools = pools

	// Collect datasets
	datasets, err := collectZDatasets()
	if err != nil {
		// Non-fatal
		_ = err
	}
	info.Datasets = datasets

	// Collect ARC stats
	arc, err := collectARCStats()
	if err != nil {
		// Non-fatal — ARC stats may not be available
		_ = err
	}
	info.ARC = arc

	return info, nil
}

// collectZPools runs zpool list and zpool status to get pool info.
func collectZPools() ([]internal.ZPool, error) {
	// Get basic pool info from zpool list
	listOut, err := exec.Command("zpool", "list", "-Hp", "-o", "name,size,alloc,free,frag,cap,health").Output()
	if err != nil {
		return nil, fmt.Errorf("zpool list: %w", err)
	}
	pools := parseZPoolList(string(listOut))

	// Get detailed status from zpool status
	statusOut, err := exec.Command("zpool", "status", "-v").Output()
	if err != nil {
		return pools, nil // Return basic info even if status fails
	}
	enrichPoolsFromStatus(pools, string(statusOut))

	return pools, nil
}

// parseZPoolList parses `zpool list -Hp` output.
// Format: name size alloc free frag cap health
func parseZPoolList(output string) []internal.ZPool {
	var pools []internal.ZPool
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 7 {
			continue
		}
		size, _ := strconv.ParseFloat(fields[1], 64)
		alloc, _ := strconv.ParseFloat(fields[2], 64)
		free, _ := strconv.ParseFloat(fields[3], 64)
		frag, _ := strconv.ParseInt(fields[4], 10, 64)
		cap, _ := strconv.ParseInt(fields[5], 10, 64)

		pools = append(pools, internal.ZPool{
			Name:          fields[0],
			State:         fields[6],
			TotalGB:       size / (1024 * 1024 * 1024),
			UsedGB:        alloc / (1024 * 1024 * 1024),
			FreeGB:        free / (1024 * 1024 * 1024),
			UsedPct:       float64(cap),
			Fragmentation: int(frag),
		})
	}
	return pools
}

// enrichPoolsFromStatus parses `zpool status -v` output and enriches pool data.
func enrichPoolsFromStatus(pools []internal.ZPool, output string) {
	// Split into per-pool sections
	sections := splitZPoolStatus(output)
	for poolName, section := range sections {
		for i := range pools {
			if pools[i].Name == poolName {
				parsePoolStatus(&pools[i], section)
			}
		}
	}
}

// splitZPoolStatus splits zpool status output into per-pool sections.
func splitZPoolStatus(output string) map[string]string {
	result := make(map[string]string)
	var currentPool string
	var currentSection strings.Builder

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pool:") {
			// Save previous pool
			if currentPool != "" {
				result[currentPool] = currentSection.String()
			}
			currentPool = strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))
			currentSection.Reset()
		}
		currentSection.WriteString(line)
		currentSection.WriteString("\n")
	}
	if currentPool != "" {
		result[currentPool] = currentSection.String()
	}
	return result
}

// parsePoolStatus extracts scan info, vdevs, and errors from a single pool's status output.
func parsePoolStatus(pool *internal.ZPool, section string) {
	lines := strings.Split(section, "\n")
	var inConfig bool
	var statusLines []string
	var actionLines []string
	var scanLines []string
	var collectingStatus, collectingAction, collectingScan bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Multi-line field detection
		if strings.HasPrefix(trimmed, "state:") {
			pool.State = strings.TrimSpace(strings.TrimPrefix(trimmed, "state:"))
			collectingStatus = false
			collectingAction = false
			collectingScan = false
			continue
		}
		if strings.HasPrefix(trimmed, "status:") {
			statusLines = []string{strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))}
			collectingStatus = true
			collectingAction = false
			collectingScan = false
			continue
		}
		if strings.HasPrefix(trimmed, "action:") {
			actionLines = []string{strings.TrimSpace(strings.TrimPrefix(trimmed, "action:"))}
			collectingAction = true
			collectingStatus = false
			collectingScan = false
			continue
		}
		if strings.HasPrefix(trimmed, "scan:") {
			scanLines = []string{strings.TrimSpace(strings.TrimPrefix(trimmed, "scan:"))}
			collectingScan = true
			collectingStatus = false
			collectingAction = false
			continue
		}
		if strings.HasPrefix(trimmed, "config:") {
			inConfig = true
			collectingStatus = false
			collectingAction = false
			collectingScan = false
			continue
		}
		if strings.HasPrefix(trimmed, "errors:") {
			inConfig = false
			collectingStatus = false
			collectingAction = false
			collectingScan = false
			errMsg := strings.TrimSpace(strings.TrimPrefix(trimmed, "errors:"))
			pool.Errors.Data = errMsg
			continue
		}

		// Continue multi-line fields
		if collectingStatus && !strings.HasPrefix(trimmed, "action:") && !strings.HasPrefix(trimmed, "scan:") && !strings.HasPrefix(trimmed, "config:") && trimmed != "" {
			statusLines = append(statusLines, trimmed)
			continue
		}
		if collectingAction && trimmed != "" && !strings.HasPrefix(trimmed, "scan:") && !strings.HasPrefix(trimmed, "config:") {
			actionLines = append(actionLines, trimmed)
			continue
		}
		if collectingScan && trimmed != "" && !strings.HasPrefix(trimmed, "config:") && !strings.HasPrefix(trimmed, "action:") {
			scanLines = append(scanLines, trimmed)
			continue
		}

		// Parse vdev config
		if inConfig && trimmed != "" && !strings.HasPrefix(trimmed, "NAME") {
			// Will be parsed as vdev tree
		}
	}

	pool.Status = strings.Join(statusLines, " ")
	pool.Action = strings.Join(actionLines, " ")

	// Parse scan info
	scanText := strings.Join(scanLines, " ")
	parseScanInfo(pool, scanText)

	// Parse vdev tree from config section
	pool.VDevs = parseVDevTree(section)
}

// parseScanInfo extracts scrub/resilver status from scan text.
func parseScanInfo(pool *internal.ZPool, scanText string) {
	if scanText == "" || strings.Contains(scanText, "none requested") {
		pool.ScanType = "none"
		pool.ScanStatus = "No scrubs performed"
		return
	}

	pool.ScanStatus = scanText

	if strings.Contains(scanText, "resilver") {
		pool.ScanType = "resilver"
	} else if strings.Contains(scanText, "scrub") {
		pool.ScanType = "scrub"
	}

	// Check if in progress
	if strings.Contains(scanText, "in progress") {
		// Try to extract percentage: "12.34% done"
		for _, word := range strings.Fields(scanText) {
			if strings.HasSuffix(word, "%") {
				if pct, err := strconv.ParseFloat(strings.TrimSuffix(word, "%"), 64); err == nil {
					pool.ScanPct = pct
				}
			}
		}
	}

	// Extract error count: "with 0 errors" or "with 5 errors"
	if idx := strings.Index(scanText, "with "); idx >= 0 {
		parts := strings.Fields(scanText[idx:])
		if len(parts) >= 3 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				pool.ScanErrors = n
			}
		}
	}

	// Extract date — look for patterns like "Sun Apr  6 12:00:00 2026"
	// The date is typically after "on" at the end
	if idx := strings.LastIndex(scanText, " on "); idx >= 0 {
		pool.ScanDate = strings.TrimSpace(scanText[idx+4:])
	}
}

// parseVDevTree extracts the vdev tree from the config section of zpool status.
func parseVDevTree(section string) []internal.ZVDev {
	lines := strings.Split(section, "\n")
	var configLines []string
	inConfig := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "config:") {
			inConfig = true
			continue
		}
		if inConfig && strings.HasPrefix(trimmed, "errors:") {
			break
		}
		if inConfig && trimmed != "" && !strings.HasPrefix(trimmed, "NAME") {
			configLines = append(configLines, line)
		}
	}

	if len(configLines) == 0 {
		return nil
	}

	// Measure indent levels relative to the pool root (first config line).
	// zpool status uses tab-based indentation:
	//   pool_name        (indent level 0 — root)
	//     mirror-0       (indent level 1 — vdev)
	//       /dev/sda     (indent level 2 — disk)
	// We use the first line's indent as the base, then detect levels by
	// counting how much deeper each subsequent line is indented.

	getIndent := func(line string) int {
		n := 0
		for _, c := range line {
			if c == '\t' {
				n += 4 // treat tab as 4 spaces
			} else if c == ' ' {
				n++
			} else {
				break
			}
		}
		return n
	}

	rootIndent := getIndent(configLines[0])

	var vdevs []internal.ZVDev
	var currentVDev *internal.ZVDev

	for i, line := range configLines {
		if i == 0 {
			continue // skip pool root line
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		indent := getIndent(line)
		depth := indent - rootIndent // relative depth

		name := fields[0]
		state := fields[1]
		readErr, _ := strconv.ParseInt(fields[2], 10, 64)
		writeErr, _ := strconv.ParseInt(fields[3], 10, 64)
		cksumErr, _ := strconv.ParseInt(fields[4], 10, 64)

		vdev := internal.ZVDev{
			Name:     name,
			State:    state,
			ReadErr:  readErr,
			WriteErr: writeErr,
			CksumErr: cksumErr,
			Type:     classifyVDev(name),
		}

		// depth <= 3 = top-level vdev, depth > 3 = child disk within a vdev
		if depth <= 3 {
			if currentVDev != nil {
				vdevs = append(vdevs, *currentVDev)
			}
			currentVDev = &vdev
		} else {
			vdev.Type = "disk"
			if currentVDev != nil {
				currentVDev.Children = append(currentVDev.Children, vdev)
			} else {
				// No parent vdev — this is a top-level single disk
				vdevs = append(vdevs, vdev)
			}
		}
	}
	if currentVDev != nil {
		vdevs = append(vdevs, *currentVDev)
	}

	return vdevs
}

// classifyVDev determines the vdev type from its name.
func classifyVDev(name string) string {
	switch {
	case strings.HasPrefix(name, "mirror"):
		return "mirror"
	case strings.HasPrefix(name, "raidz1"):
		return "raidz1"
	case strings.HasPrefix(name, "raidz2"):
		return "raidz2"
	case strings.HasPrefix(name, "raidz3"):
		return "raidz3"
	case name == "logs" || strings.HasPrefix(name, "log"):
		return "log"
	case name == "cache":
		return "cache"
	case name == "spares":
		return "spare"
	case name == "special":
		return "special"
	case strings.HasPrefix(name, "/dev/") || strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "da"):
		return "disk"
	default:
		return "disk"
	}
}

// collectZDatasets runs `zfs list` to get dataset info.
func collectZDatasets() ([]internal.ZDataset, error) {
	out, err := exec.Command("zfs", "list", "-Hp", "-o", "name,used,avail,refer,mountpoint,compression,compressratio,type").Output()
	if err != nil {
		return nil, err
	}
	return parseZFSList(string(out)), nil
}

// parseZFSList parses `zfs list -Hp` output.
func parseZFSList(output string) []internal.ZDataset {
	var datasets []internal.ZDataset
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}
		used, _ := strconv.ParseFloat(fields[1], 64)
		avail, _ := strconv.ParseFloat(fields[2], 64)
		refer, _ := strconv.ParseFloat(fields[3], 64)
		compRatio, _ := strconv.ParseFloat(strings.TrimSuffix(fields[6], "x"), 64)

		// Extract pool name from dataset path
		pool := fields[0]
		if idx := strings.Index(pool, "/"); idx > 0 {
			pool = pool[:idx]
		}

		ds := internal.ZDataset{
			Name:        fields[0],
			Pool:        pool,
			UsedGB:      used / (1024 * 1024 * 1024),
			AvailGB:     avail / (1024 * 1024 * 1024),
			ReferGB:     refer / (1024 * 1024 * 1024),
			MountPoint:  fields[4],
			Compression: fields[5],
			CompRatio:   compRatio,
			Type:        fields[7],
		}
		datasets = append(datasets, ds)
	}
	return datasets
}

// collectARCStats reads ZFS ARC stats from /proc/spl/kstat/zfs/arcstats.
func collectARCStats() (*internal.ZFSARCStats, error) {
	f, err := os.Open("/proc/spl/kstat/zfs/arcstats")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseARCStats(f)
}

// parseARCStats parses /proc/spl/kstat/zfs/arcstats content.
func parseARCStats(f *os.File) (*internal.ZFSARCStats, error) {
	vals := make(map[string]int64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			if v, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				vals[fields[0]] = v
			}
		}
	}

	arc := &internal.ZFSARCStats{
		SizeMB:    float64(vals["size"]) / (1024 * 1024),
		MaxSizeMB: float64(vals["c_max"]) / (1024 * 1024),
		Hits:      vals["hits"],
		Misses:    vals["misses"],
	}

	total := arc.Hits + arc.Misses
	if total > 0 {
		arc.HitRate = float64(arc.Hits) / float64(total) * 100
		arc.MissRate = float64(arc.Misses) / float64(total) * 100
	}

	arc.L2SizeMB = float64(vals["l2_size"]) / (1024 * 1024)
	l2Hits := vals["l2_hits"]
	l2Misses := vals["l2_misses"]
	l2Total := l2Hits + l2Misses
	if l2Total > 0 {
		arc.L2HitRate = float64(l2Hits) / float64(l2Total) * 100
	}

	return arc, nil
}
