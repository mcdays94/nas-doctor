package collector

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectSystem(hp internal.HostPaths) (internal.SystemInfo, error) {
	info := internal.SystemInfo{}

	// Hostname
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
	}

	// Kernel
	if out, err := execCmd("uname", "-r"); err == nil {
		info.Kernel = strings.TrimSpace(out)
	}

	// CPU model
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.CPUModel = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	info.CPUCores = runtime.NumCPU()

	// Load average
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			info.LoadAvg1, _ = strconv.ParseFloat(fields[0], 64)
			info.LoadAvg5, _ = strconv.ParseFloat(fields[1], 64)
			info.LoadAvg15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// Memory from /proc/meminfo
	if f, err := os.Open("/proc/meminfo"); err == nil {
		defer f.Close()
		memMap := map[string]int64{}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(parts[1])
			valStr = strings.TrimSuffix(valStr, " kB")
			valStr = strings.TrimSpace(valStr)
			val, _ := strconv.ParseInt(valStr, 10, 64)
			memMap[key] = val
		}
		info.MemTotalMB = memMap["MemTotal"] / 1024
		memUsed := memMap["MemTotal"] - memMap["MemAvailable"]
		info.MemUsedMB = memUsed / 1024
		if memMap["MemTotal"] > 0 {
			info.MemPercent = float64(memUsed) / float64(memMap["MemTotal"]) * 100
		}
		info.SwapTotalMB = memMap["SwapTotal"] / 1024
		info.SwapUsedMB = (memMap["SwapTotal"] - memMap["SwapFree"]) / 1024
	}

	// Uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			upF, _ := strconv.ParseFloat(fields[0], 64)
			info.UptimeSecs = int64(upF)
		}
	}

	// CPU usage and I/O wait from /proc/stat (two samples, 500ms apart)
	info.CPUUsage, info.IOWait = readCPUStats()

	// Detect platform (uses cached singleton — safe to call multiple times)
	plat := DetectPlatform(hp)
	info.Platform = plat.Name
	info.PlatformVer = plat.Version
	info.OS = fmt.Sprintf("%s %s (kernel %s)", info.Platform, info.PlatformVer, info.Kernel)

	// Motherboard (dmidecode)
	if out, err := execCmd("dmidecode", "-t", "baseboard"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Manufacturer:") || strings.HasPrefix(line, "Product Name:") {
				if info.Motherboard != "" {
					info.Motherboard += " "
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.Motherboard += strings.TrimSpace(parts[1])
				}
			}
		}
	}

	// Top processes by CPU
	info.TopProcesses = collectTopProcesses(10)

	return info, nil
}

// readCPUStats samples /proc/stat multiple times over a few seconds to compute
// a smoothed CPU usage and I/O wait percentage. Multiple samples reduce the
// impact of short-lived spikes (including our own collection overhead).
func readCPUStats() (cpuUsage, ioWait float64) {
	type cpuSample struct {
		idle, iowait, total float64
	}

	parse := func() (cpuSample, bool) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return cpuSample{}, false
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) >= 6 {
					// fields: cpu user nice system idle iowait irq softirq steal ...
					var s cpuSample
					s.idle, _ = strconv.ParseFloat(fields[4], 64)
					s.iowait, _ = strconv.ParseFloat(fields[5], 64)
					for _, f := range fields[1:] {
						v, _ := strconv.ParseFloat(f, 64)
						s.total += v
					}
					return s, true
				}
			}
		}
		return cpuSample{}, false
	}

	// Take 4 samples over 3 seconds (1s apart) and average the 3 intervals.
	// This gives a representative ~3s window that smooths out short spikes.
	const numSamples = 4
	const sampleInterval = time.Second
	samples := make([]cpuSample, 0, numSamples)

	for i := 0; i < numSamples; i++ {
		s, ok := parse()
		if !ok {
			return 0, 0
		}
		samples = append(samples, s)
		if i < numSamples-1 {
			time.Sleep(sampleInterval)
		}
	}

	var totalCPU, totalIOWait float64
	intervals := 0
	for i := 1; i < len(samples); i++ {
		totalDelta := samples[i].total - samples[i-1].total
		if totalDelta <= 0 {
			continue
		}
		idleDelta := samples[i].idle - samples[i-1].idle
		iowaitDelta := samples[i].iowait - samples[i-1].iowait

		totalCPU += (1.0 - idleDelta/totalDelta) * 100
		totalIOWait += (iowaitDelta / totalDelta) * 100
		intervals++
	}

	if intervals == 0 {
		return 0, 0
	}
	return totalCPU / float64(intervals), totalIOWait / float64(intervals)
}

// detectPlatform and fetchTrueNASVersion have been moved to platform.go
// as the centralized Platform detection singleton.

func collectTopProcesses(n int) []internal.ProcessInfo {
	// Try GNU ps first (--sort flag), fall back to POSIX ps without sorting
	out, err := execCmd("ps", "aux", "--sort=-%cpu")
	if err != nil {
		out, err = execCmd("ps", "aux")
		if err != nil {
			return nil
		}
	}
	var procs []internal.ProcessInfo
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" { // skip header
			continue
		}
		if len(procs) >= n {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		cmd := strings.Join(fields[10:], " ")
		procs = append(procs, internal.ProcessInfo{
			PID:     pid,
			User:    fields[0],
			CPU:     cpu,
			Mem:     mem,
			Command: cmd,
		})
	}
	return procs
}

// extractContainerID reads /proc/<pid>/cgroup and extracts the Docker
// container ID. procRoot allows overriding the proc filesystem root for
// testing (default: "/proc").
func extractContainerID(pid int, procRoot string) string {
	path := fmt.Sprintf("%s/%d/cgroup", procRoot, pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return "" // file doesn't exist (macOS) or permission denied
	}
	return parseContainerIDFromCgroup(string(data))
}

// buildContainerIDMap creates a lookup map from Docker short container IDs
// to container names, using the ContainerInfo slice from collectDocker().
func buildContainerIDMap(containers []internal.ContainerInfo) map[string]string {
	m := make(map[string]string, len(containers))
	for _, c := range containers {
		if c.ID != "" {
			m[c.ID] = c.Name
		}
	}
	return m
}

// enrichProcessContainers populates ContainerID and ContainerName on each
// ProcessInfo by reading cgroup data and matching against the container map.
// The containerMap keys are short Docker IDs (typically 12 chars from docker ps);
// we match by checking if the full 64-char cgroup ID starts with a short ID.
// procRoot allows overriding the proc filesystem root for testing.
func enrichProcessContainers(procs []internal.ProcessInfo, containerMap map[string]string, procRoot string) {
	for i := range procs {
		fullID := extractContainerID(procs[i].PID, procRoot)
		if fullID == "" {
			continue
		}
		procs[i].ContainerID = fullID

		// Try to find the container name by prefix-matching short IDs
		for shortID, name := range containerMap {
			if strings.HasPrefix(fullID, shortID) {
				procs[i].ContainerName = name
				break
			}
		}
	}
}

// parseContainerIDFromCgroup extracts a Docker container ID (64-char hex)
// from the contents of a /proc/PID/cgroup file. Supports:
//   - cgroup v1 (Unraid): "12:devices:/docker/<64-hex>"
//   - cgroup v2 (TrueNAS SCALE): "0::/system.slice/docker-<64-hex>.scope"
//
// Returns empty string if no Docker container ID is found.
func parseContainerIDFromCgroup(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// cgroup v1: look for "/docker/<id>" anywhere in the line
		if idx := strings.Index(line, "/docker/"); idx != -1 {
			candidate := line[idx+len("/docker/"):]
			if id := extractHexID(candidate); id != "" {
				return id
			}
		}

		// cgroup v2: look for "docker-<id>.scope" anywhere in the line
		if idx := strings.Index(line, "docker-"); idx != -1 {
			candidate := line[idx+len("docker-"):]
			// Strip ".scope" suffix if present
			candidate = strings.TrimSuffix(candidate, ".scope")
			if id := extractHexID(candidate); id != "" {
				return id
			}
		}
	}
	return ""
}

// extractHexID returns the first 64 hex characters from s, or "" if
// s doesn't start with a valid 64-char hex string.
func extractHexID(s string) string {
	if len(s) < 64 {
		return ""
	}
	candidate := s[:64]
	for _, c := range candidate {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return ""
		}
	}
	return candidate
}

// execCmd shells out to a command and returns combined stdout+stderr plus
// any exec error. Defined as a package-level var (rather than a plain
// function) so tests can swap in a fake implementation — see
// smart_standby_test.go for the seam usage.
var execCmd = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
