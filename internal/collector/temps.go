// CPU + mainboard temperature collection for the dashboard header
// stats row. Issue #269.
//
// Detection strategy (all paths read from sysClassBase, which defaults
// to /sys/class but is redirected to a tempdir in tests):
//
//   CPU temp:
//     1. {sysClassBase}/thermal/thermal_zone*/type == "x86_pkg_temp"
//        — canonical Intel package temp (preferred).
//     2. hwmon name == "coretemp" → temp{N}_input where label == "Package id 0",
//        else fall back to temp1_input.
//     3. hwmon name == "k10temp" → temp1_input (Tctl/Tdie on AMD).
//
//   Mobo temp:
//     1. hwmon name == "acpitz" → temp1_input.
//     2. {sysClassBase}/thermal/thermal_zone*/type == "acpitz" → temp.
//
// Anything outside (0, 120]°C is rejected as a disconnected/glitched
// reading. Returning (0, 0) is the documented "no sensor available"
// signal for callers — the dashboard header hides the gauge entirely
// in that case rather than rendering an "—" placeholder, per the
// acceptance criteria on issue #269.
//
// NOTE: we deliberately do NOT skip the acpitz thermal zone for the
// mobo temp reading the way readThermalZoneTemp() does for CPU temp.
// That helper avoided acpitz because the underlying sensor is often
// the ACPI critical trip-point (98°C) reported as "current temperature"
// — meaningless for a CPU reading. For mainboard temp the acpitz
// reading is the *intended* source: it's literally the system's ACPI
// thermal zone. Boards with broken ACPI tables will simply report
// implausible values that our 0<t<=120 filter rejects.
package collector

import (
	"path/filepath"
	"strings"
)

// collectCPUMoboTemps returns the package-level CPU temperature and
// mainboard temperature in degrees Celsius. Either or both can be 0
// to indicate "no sensor available" — callers must treat 0 as missing
// rather than as 0°C (no real CPU runs at 0°C anyway).
func collectCPUMoboTemps() (cpu int, mobo int) {
	cpu = readCPUTemp()
	mobo = readMoboTemp()
	return cpu, mobo
}

// readCPUTemp implements the three-tier CPU detection strategy
// described at the top of this file.
func readCPUTemp() int {
	// 1. x86_pkg_temp thermal zone — Intel canonical.
	zones, _ := filepath.Glob(filepath.Join(sysClassBase, "thermal", "thermal_zone*", "type"))
	for _, typePath := range zones {
		zoneType := strings.TrimSpace(readSysfs(typePath))
		if zoneType == "x86_pkg_temp" {
			if t := parseInt(readSysfs(filepath.Join(filepath.Dir(typePath), "temp"))) / 1000; isPlausibleTemp(t) {
				return t
			}
		}
	}

	// 2. coretemp hwmon — Intel via lm_sensors.
	if dir := findHwmonByName("coretemp"); dir != "" {
		if t := readPackageOrFirstTemp(dir); isPlausibleTemp(t) {
			return t
		}
	}

	// 3. k10temp hwmon — AMD.
	if dir := findHwmonByName("k10temp"); dir != "" {
		if t := parseInt(readSysfs(filepath.Join(dir, "temp1_input"))) / 1000; isPlausibleTemp(t) {
			return t
		}
	}

	return 0
}

// readMoboTemp implements the two-tier mainboard / system temp
// detection strategy.
func readMoboTemp() int {
	// 1. acpitz hwmon — most common path on x86 boards.
	if dir := findHwmonByName("acpitz"); dir != "" {
		if t := parseInt(readSysfs(filepath.Join(dir, "temp1_input"))) / 1000; isPlausibleTemp(t) {
			return t
		}
	}

	// 2. acpitz thermal zone — fallback on systems where the hwmon
	//    interface isn't registered but the thermal subsystem is.
	zones, _ := filepath.Glob(filepath.Join(sysClassBase, "thermal", "thermal_zone*", "type"))
	for _, typePath := range zones {
		zoneType := strings.TrimSpace(readSysfs(typePath))
		if zoneType == "acpitz" {
			if t := parseInt(readSysfs(filepath.Join(filepath.Dir(typePath), "temp"))) / 1000; isPlausibleTemp(t) {
				return t
			}
		}
	}

	return 0
}

// readPackageOrFirstTemp scans a coretemp hwmon directory for a temp*
// entry labelled "Package..." (Intel coretemp's package-level sensor)
// and falls back to temp1_input (the conventional package sensor when
// no labels are exposed).
func readPackageOrFirstTemp(dir string) int {
	tempInputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
	for _, tempPath := range tempInputs {
		base := strings.TrimSuffix(filepath.Base(tempPath), "_input")
		labelPath := filepath.Join(dir, base+"_label")
		label := strings.ToLower(readSysfs(labelPath))
		if strings.HasPrefix(label, "package") {
			return parseInt(readSysfs(tempPath)) / 1000
		}
	}
	return parseInt(readSysfs(filepath.Join(dir, "temp1_input"))) / 1000
}

// findHwmonByName scans every {sysClassBase}/hwmon/hwmon*/name entry
// and returns the absolute path to the hwmon directory whose name
// matches. Returns "" when no match is found. Used by readCPUTemp /
// readMoboTemp to locate the right hwmon among the typical 4-8
// devices a modern board exposes (nvme, coretemp, acpitz, nct6798, ...).
func findHwmonByName(name string) string {
	matches, _ := filepath.Glob(filepath.Join(sysClassBase, "hwmon", "hwmon*", "name"))
	for _, namePath := range matches {
		if strings.TrimSpace(readSysfs(namePath)) == name {
			return filepath.Dir(namePath)
		}
	}
	return ""
}

// isPlausibleTemp filters out 0°C (sensor disconnected / never
// initialised) and absurd values like 250°C (sensor glitch or wrong
// scale). The dashboard renders the gauge only when the reading is
// both non-zero and plausible — this guard centralises the "is this
// value safe to surface" check.
func isPlausibleTemp(t int) bool {
	return t > 0 && t <= 120
}
