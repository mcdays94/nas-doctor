package collector

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

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

	// I/O wait from /proc/stat (simple snapshot)
	info.IOWait = readIOWait()

	// Detect platform
	info.Platform, info.PlatformVer = detectPlatform(hp)
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

func readIOWait() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) >= 6 {
				// fields: cpu user nice system idle iowait ...
				idle, _ := strconv.ParseFloat(fields[4], 64)
				iowait, _ := strconv.ParseFloat(fields[5], 64)
				total := 0.0
				for _, f := range fields[1:] {
					v, _ := strconv.ParseFloat(f, 64)
					total += v
				}
				if total > 0 {
					_ = idle // used in total
					return iowait / total * 100
				}
			}
		}
	}
	return 0
}

func detectPlatform(hp internal.HostPaths) (platform, version string) {
	// Check Unraid — multiple detection methods
	unraidIdent := hp.Boot + "/config/ident.cfg"
	if _, err := os.Stat(unraidIdent); err == nil {
		platform = "unraid"
	}
	// Try /etc/unraid-version (host or bind-mounted)
	for _, path := range []string{"/etc/unraid-version", "/host/etc/unraid-version"} {
		if data, err := os.ReadFile(path); err == nil {
			platform = "unraid"
			raw := strings.TrimSpace(string(data))
			if strings.Contains(raw, "=") {
				parts := strings.SplitN(raw, "=", 2)
				raw = parts[len(parts)-1]
			}
			version = strings.Trim(raw, "\"'")
			break
		}
	}
	// Fallback: extract version from kernel string (e.g. "6.6.78-Unraid" or "6.12.10-Unraid")
	if platform == "unraid" && version == "" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			vs := string(data)
			// Look for pattern like "X.Y.Z-Unraid"
			if idx := strings.Index(vs, "-Unraid"); idx > 0 {
				// Walk backwards to find the version start
				start := idx
				for start > 0 && (vs[start-1] == '.' || (vs[start-1] >= '0' && vs[start-1] <= '9')) {
					start--
				}
				if start < idx {
					version = vs[start:idx]
				}
			}
		}
	}
	if platform != "" {
		return
	}

	// Check /etc/os-release for others
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "ID=") {
				platform = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			}
			if strings.HasPrefix(line, "VERSION_ID=") {
				version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
	}

	if platform == "" {
		platform = "linux"
	}
	return
}

func collectTopProcesses(n int) []internal.ProcessInfo {
	out, err := execCmd("ps", "aux", "--sort=-%cpu")
	if err != nil {
		return nil
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

func execCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
