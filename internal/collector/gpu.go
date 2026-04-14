// GPU monitoring — Nvidia (nvidia-smi), AMD (rocm-smi / sysfs), Intel (sysfs).
//
// Each vendor probe is tried in order and all detected GPUs are aggregated.
// If no GPU tooling is found, Available=false is returned (not an error).
package collector

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// collectGPU probes for Nvidia, AMD, and Intel GPUs. Returns Available=false
// when no GPU is detected (this is normal for headless NAS boxes).
func collectGPU() *internal.GPUInfo {
	info := &internal.GPUInfo{}

	// Nvidia — nvidia-smi is the de-facto standard
	if gpus := collectNvidia(); len(gpus) > 0 {
		info.GPUs = append(info.GPUs, gpus...)
	}

	// AMD — rocm-smi first, fall back to sysfs
	if gpus := collectAMD(); len(gpus) > 0 {
		info.GPUs = append(info.GPUs, gpus...)
	}

	// Intel — sysfs DRM + i915/xe driver
	if gpus := collectIntel(); len(gpus) > 0 {
		info.GPUs = append(info.GPUs, gpus...)
	}

	info.Available = len(info.GPUs) > 0
	return info
}

// ── Nvidia ──────────────────────────────────────────────────────────

func collectNvidia() []internal.GPUDevice {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}

	// CSV query — one row per GPU, all key metrics
	fields := strings.Join([]string{
		"index", "name", "driver_version",
		"utilization.gpu", "memory.used", "memory.total",
		"temperature.gpu", "fan.speed",
		"power.draw", "power.limit",
		"clocks.current.graphics", "clocks.current.memory",
		"pci.bus_id",
		"utilization.encoder", "utilization.decoder",
	}, ",")

	out, err := exec.Command("nvidia-smi",
		"--query-gpu="+fields,
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return nil
	}

	var gpus []internal.GPUDevice
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ", ")
		if len(parts) < 13 {
			continue
		}
		idx, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		memUsed := parseFloat(parts[4])
		memTotal := parseFloat(parts[5])
		memPct := 0.0
		if memTotal > 0 {
			memPct = (memUsed / memTotal) * 100
		}

		gpu := internal.GPUDevice{
			Index:       idx,
			Name:        strings.TrimSpace(parts[1]),
			Vendor:      "nvidia",
			Driver:      strings.TrimSpace(parts[2]),
			UsagePct:    parseFloat(parts[3]),
			MemUsedMB:   memUsed,
			MemTotalMB:  memTotal,
			MemPct:      memPct,
			Temperature: parseInt(parts[6]),
			FanPct:      parseFloat(parts[7]),
			PowerW:      parseFloat(parts[8]),
			PowerMaxW:   parseFloat(parts[9]),
			ClockMHz:    parseInt(parts[10]),
			MemClockMHz: parseInt(parts[11]),
			PCIeBus:     strings.TrimSpace(parts[12]),
		}
		if len(parts) > 13 {
			gpu.EncoderPct = parseFloat(parts[13])
		}
		if len(parts) > 14 {
			gpu.DecoderPct = parseFloat(parts[14])
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}

// ── AMD ─────────────────────────────────────────────────────────────

func collectAMD() []internal.GPUDevice {
	// Try rocm-smi first (ROCm stack installed)
	if gpus := collectAMDRocm(); len(gpus) > 0 {
		return gpus
	}
	// Fall back to sysfs AMDGPU driver
	return collectAMDSysfs()
}

func collectAMDRocm() []internal.GPUDevice {
	if _, err := exec.LookPath("rocm-smi"); err != nil {
		return nil
	}
	out, err := exec.Command("rocm-smi", "--showid", "--showtemp", "--showuse",
		"--showmeminfo", "vram", "--showpower", "--showclocks", "--csv").Output()
	if err != nil {
		return nil
	}
	// rocm-smi CSV: header line then one row per GPU
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil
	}
	// Parse header to find column indices
	header := strings.Split(lines[0], ",")
	colIdx := map[string]int{}
	for i, h := range header {
		colIdx[strings.TrimSpace(strings.ToLower(h))] = i
	}

	var gpus []internal.GPUDevice
	for i, line := range lines[1:] {
		cols := strings.Split(line, ",")
		gpu := internal.GPUDevice{
			Index:  i,
			Vendor: "amd",
		}
		if ci, ok := colIdx["card series"]; ok && ci < len(cols) {
			gpu.Name = strings.TrimSpace(cols[ci])
		}
		if ci, ok := colIdx["temperature (sensor edge) (c)"]; ok && ci < len(cols) {
			gpu.Temperature = parseInt(cols[ci])
		}
		if ci, ok := colIdx["gpu use (%)"]; ok && ci < len(cols) {
			gpu.UsagePct = parseFloat(cols[ci])
		}
		if ci, ok := colIdx["average graphics package power (w)"]; ok && ci < len(cols) {
			gpu.PowerW = parseFloat(cols[ci])
		}
		if ci, ok := colIdx["vram total memory (b)"]; ok && ci < len(cols) {
			gpu.MemTotalMB = parseFloat(cols[ci]) / 1048576
		}
		if ci, ok := colIdx["vram total used memory (b)"]; ok && ci < len(cols) {
			gpu.MemUsedMB = parseFloat(cols[ci]) / 1048576
		}
		if gpu.MemTotalMB > 0 {
			gpu.MemPct = (gpu.MemUsedMB / gpu.MemTotalMB) * 100
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}

func collectAMDSysfs() []internal.GPUDevice {
	// Look for /sys/class/drm/card*/device/vendor == 0x1002 (AMD)
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]*/device/vendor")
	var gpus []internal.GPUDevice
	for _, vendorPath := range cards {
		vendor := readSysfs(vendorPath)
		if vendor != "0x1002" {
			continue
		}
		base := filepath.Dir(vendorPath) // .../device/
		hwmonBase := findHwmon(filepath.Join(base, "hwmon"))

		gpu := internal.GPUDevice{
			Index:  len(gpus),
			Vendor: "amd",
			Name:   readSysfs(filepath.Join(base, "product_name")),
		}
		if gpu.Name == "" {
			gpu.Name = "AMD GPU"
		}
		if hwmonBase != "" {
			gpu.Temperature = parseInt(readSysfs(filepath.Join(hwmonBase, "temp1_input"))) / 1000
			gpu.PowerW = parseFloat(readSysfs(filepath.Join(hwmonBase, "power1_average"))) / 1000000
			gpu.PowerMaxW = parseFloat(readSysfs(filepath.Join(hwmonBase, "power1_cap"))) / 1000000
		}
		// GPU busy percent
		busyPct := readSysfs(filepath.Join(base, "gpu_busy_percent"))
		if busyPct != "" {
			gpu.UsagePct = parseFloat(busyPct)
		}
		// VRAM from mem_info_vram_* files
		vramTotal := readSysfs(filepath.Join(base, "mem_info_vram_total"))
		vramUsed := readSysfs(filepath.Join(base, "mem_info_vram_used"))
		if vramTotal != "" {
			gpu.MemTotalMB = parseFloat(vramTotal) / 1048576
		}
		if vramUsed != "" {
			gpu.MemUsedMB = parseFloat(vramUsed) / 1048576
		}
		if gpu.MemTotalMB > 0 {
			gpu.MemPct = (gpu.MemUsedMB / gpu.MemTotalMB) * 100
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}

// ── Intel ───────────────────────────────────────────────────────────

func collectIntel() []internal.GPUDevice {
	// Look for /sys/class/drm/card*/device/vendor == 0x8086 (Intel)
	// Filter: only if i915 or xe driver is bound (avoids matching Intel LAN/audio)
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]*/device/vendor")
	var gpus []internal.GPUDevice
	for _, vendorPath := range cards {
		vendor := readSysfs(vendorPath)
		if vendor != "0x8086" {
			continue
		}
		base := filepath.Dir(vendorPath)
		// Check driver symlink — must be i915 or xe (GPU drivers)
		driverLink, err := os.Readlink(filepath.Join(base, "driver"))
		if err != nil {
			continue
		}
		driverName := filepath.Base(driverLink)
		if driverName != "i915" && driverName != "xe" {
			continue
		}

		hwmonBase := findHwmon(filepath.Join(base, "hwmon"))
		gpu := internal.GPUDevice{
			Index:  len(gpus),
			Vendor: "intel",
			Driver: driverName,
			Name:   readSysfs(filepath.Join(base, "product_name")),
		}
		if gpu.Name == "" {
			// Try lspci-style identification via class
			gpu.Name = "Intel GPU"
		}
		if hwmonBase != "" {
			// Temperature: some Intel GPUs expose temp via hwmon
			tempStr := readSysfs(filepath.Join(hwmonBase, "temp1_input"))
			if tempStr != "" {
				gpu.Temperature = parseInt(tempStr) / 1000
			}
			// Power: Intel Xe/Arc expose energy counters, some expose power1_average
			pwrStr := readSysfs(filepath.Join(hwmonBase, "power1_average"))
			if pwrStr != "" {
				gpu.PowerW = parseFloat(pwrStr) / 1000000
			}
		}

		// Temperature fallback: iGPU shares die with CPU — use thermal_zone or coretemp
		if gpu.Temperature == 0 {
			gpu.Temperature = readThermalZoneTemp()
		}

		// GPU usage: estimate from frequency ratio (actual / max)
		cardDir := filepath.Dir(base) // e.g., /sys/class/drm/card0
		actFreq := readSysfs(filepath.Join(cardDir, "gt_act_freq_mhz"))
		maxFreq := readSysfs(filepath.Join(cardDir, "gt_max_freq_mhz"))
		if actFreq == "" {
			actFreq = readSysfs(filepath.Join(cardDir, "gt_cur_freq_mhz"))
		}
		if actFreq != "" && maxFreq != "" {
			act := parseFloat(actFreq)
			max := parseFloat(maxFreq)
			if max > 0 {
				gpu.UsagePct = (act / max) * 100
				if gpu.UsagePct > 100 {
					gpu.UsagePct = 100
				}
			}
			gpu.ClockMHz = int(act)
		}

		// Try intel_gpu_top for more accurate usage (if installed)
		if out, err := execCmd("intel_gpu_top", "-J", "-s", "100", "-l", "1"); err == nil {
			gpu.UsagePct = parseIntelGPUTop(out, &gpu)
		}

		// VRAM / local memory — Intel Arc discrete GPUs
		vramTotal := readSysfs(filepath.Join(base, "mem_info_vram_total"))
		vramUsed := readSysfs(filepath.Join(base, "mem_info_vram_used"))
		if vramTotal != "" {
			gpu.MemTotalMB = parseFloat(vramTotal) / 1048576
		}
		if vramUsed != "" {
			gpu.MemUsedMB = parseFloat(vramUsed) / 1048576
		}
		if gpu.MemTotalMB > 0 {
			gpu.MemPct = (gpu.MemUsedMB / gpu.MemTotalMB) * 100
		}

		// Don't show GPU section with all zeros — skip if no useful data
		if gpu.UsagePct == 0 && gpu.Temperature == 0 && gpu.MemTotalMB == 0 && gpu.PowerW == 0 {
			continue
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}

// ── Helpers ─────────────────────────────────────────────────────────

func readSysfs(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func findHwmon(hwmonDir string) string {
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "hwmon") {
			return filepath.Join(hwmonDir, e.Name())
		}
	}
	return ""
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "[N/A]" || s == "N/A" || s == "[Not Supported]" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "[N/A]" || s == "N/A" || s == "[Not Supported]" {
		return 0
	}
	// Handle float strings like "42.0"
	if strings.Contains(s, ".") {
		v, _ := strconv.ParseFloat(s, 64)
		return int(v)
	}
	v, _ := strconv.Atoi(s)
	return v
}

// readThermalZoneTemp reads the CPU/package temperature from thermal zones.
// Intel iGPUs share the die with the CPU, so the CPU package temp is the
// closest approximation of GPU temperature.
func readThermalZoneTemp() int {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/type")
	for _, typePath := range zones {
		zoneType := readSysfs(typePath)
		// Prefer x86_pkg_temp (package), then coretemp, then any zone
		if strings.Contains(zoneType, "x86_pkg") || strings.Contains(zoneType, "pkg") {
			tempStr := readSysfs(filepath.Join(filepath.Dir(typePath), "temp"))
			if tempStr != "" {
				return parseInt(tempStr) / 1000
			}
		}
	}
	// Fallback: try first thermal zone with a non-zero temperature
	zones2, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, tempPath := range zones2 {
		t := parseInt(readSysfs(tempPath)) / 1000
		if t > 0 && t < 120 {
			return t
		}
	}
	return 0
}

// parseIntelGPUTop parses JSON output from `intel_gpu_top -J -s 100 -l 1`.
// Returns the render busy percentage, and populates encoder/decoder if available.
func parseIntelGPUTop(output string, gpu *internal.GPUDevice) float64 {
	// intel_gpu_top JSON wraps data in {"period": {...}, "engines": {...}, "frequency": {...}, ...}
	// Look for "Render/3D" or "render" engine busy percentage
	type engineData struct {
		Busy float64 `json:"busy"`
	}
	type gpuTopEntry struct {
		Frequency struct {
			Actual float64 `json:"actual"`
			Max    float64 `json:"requested"`
		} `json:"frequency"`
		Engines map[string]engineData `json:"engines"`
		Power   struct {
			GPU float64 `json:"GPU"`
			Pkg float64 `json:"Package"`
		} `json:"power"`
	}

	// The output may have multiple lines or be wrapped — try to find the JSON object
	var entry gpuTopEntry
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		// intel_gpu_top sometimes wraps in a JSON array
		var entries []gpuTopEntry
		if err := json.Unmarshal([]byte(output), &entries); err == nil && len(entries) > 0 {
			entry = entries[len(entries)-1]
		} else {
			return 0
		}
	}

	// Sum render + video engine usage
	var renderBusy, videoBusy float64
	for name, eng := range entry.Engines {
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "render") || strings.Contains(nameLower, "3d") {
			renderBusy = eng.Busy
		}
		if strings.Contains(nameLower, "video") || strings.Contains(nameLower, "vecs") {
			videoBusy = eng.Busy
		}
	}

	if renderBusy > 0 {
		gpu.EncoderPct = videoBusy
	}
	if entry.Frequency.Actual > 0 {
		gpu.ClockMHz = int(entry.Frequency.Actual)
	}
	if entry.Power.GPU > 0 {
		gpu.PowerW = entry.Power.GPU
	}

	return renderBusy
}
