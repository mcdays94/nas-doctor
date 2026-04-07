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

// readCPUStats reads /proc/stat twice with a short delay to compute
// instantaneous CPU usage and I/O wait percentages.
func readCPUStats() (cpuUsage, ioWait float64) {
	parse := func() (idle, iowait, total float64, ok bool) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0, 0, false
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) >= 6 {
					// fields: cpu user nice system idle iowait irq softirq steal ...
					idle, _ = strconv.ParseFloat(fields[4], 64)
					iowait, _ = strconv.ParseFloat(fields[5], 64)
					for _, f := range fields[1:] {
						v, _ := strconv.ParseFloat(f, 64)
						total += v
					}
					return idle, iowait, total, true
				}
			}
		}
		return 0, 0, 0, false
	}

	idle1, iowait1, total1, ok1 := parse()
	time.Sleep(500 * time.Millisecond)
	idle2, iowait2, total2, ok2 := parse()

	if !ok1 || !ok2 {
		return 0, 0
	}

	totalDelta := total2 - total1
	if totalDelta <= 0 {
		return 0, 0
	}

	idleDelta := idle2 - idle1
	iowaitDelta := iowait2 - iowait1

	cpuUsage = (1.0 - idleDelta/totalDelta) * 100
	ioWait = (iowaitDelta / totalDelta) * 100
	return cpuUsage, ioWait
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
	// Note: /proc/version contains the KERNEL version (e.g. "6.12.24-Unraid"),
	// NOT the Unraid OS version (e.g. "7.1.4"). We only use it to confirm
	// the platform is Unraid, not to extract the version number.
	if platform == "" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			if strings.Contains(string(data), "-Unraid") {
				platform = "unraid"
				// version stays empty — can't determine OS version from kernel
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
