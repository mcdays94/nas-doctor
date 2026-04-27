package collector

import (
	"testing"
)

// Issue #269 — CPU + mainboard temperature collector for the dashboard
// header. These tests cover the hwmon-name detection paths used in
// collectCPUMoboTemps. They reuse the seedHwmon / seedThermalZone /
// withSysClassBase helpers already defined in gpu_test.go.

// TestCollectCPUMoboTemps_Coretemp covers the canonical Intel path:
// hwmon name == "coretemp", with a temp1 entry labelled "Package id 0".
// We expect milli-Celsius -> Celsius conversion (58000 -> 58).
func TestCollectCPUMoboTemps_CoretempPackageLabel(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 1, "coretemp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"Package id 0", 58000},
		"temp2": {"Core 0", 57000},
		"temp3": {"Core 1", 59000},
	})

	cpu, mobo := collectCPUMoboTemps()
	if cpu != 58 {
		t.Errorf("collectCPUMoboTemps() cpu = %d, want 58 (coretemp Package id 0)", cpu)
	}
	if mobo != 0 {
		t.Errorf("collectCPUMoboTemps() mobo = %d, want 0 (no acpitz seeded)", mobo)
	}
}

// TestCollectCPUMoboTemps_CoretempWithoutPackageLabel falls back to
// temp1_input when no Package label is defined.
func TestCollectCPUMoboTemps_CoretempWithoutPackageLabel(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 0, "coretemp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"", 55000},
	})

	cpu, _ := collectCPUMoboTemps()
	if cpu != 55 {
		t.Errorf("collectCPUMoboTemps() cpu = %d, want 55 (temp1_input on coretemp without label)", cpu)
	}
}

// TestCollectCPUMoboTemps_K10Temp covers AMD CPUs: hwmon name == "k10temp",
// temp1_input is Tctl/Tdie.
func TestCollectCPUMoboTemps_K10Temp(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 0, "k10temp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"Tctl", 62000},
	})

	cpu, _ := collectCPUMoboTemps()
	if cpu != 62 {
		t.Errorf("collectCPUMoboTemps() cpu = %d, want 62 (k10temp Tctl)", cpu)
	}
}

// TestCollectCPUMoboTemps_ThermalZoneFallback simulates the case where
// the kernel exposes x86_pkg_temp via /sys/class/thermal but coretemp
// is not present (some lightweight kernels).
func TestCollectCPUMoboTemps_ThermalZoneFallback(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedThermalZone(t, base, 0, "acpitz", 41000)       // mobo
	seedThermalZone(t, base, 1, "x86_pkg_temp", 64000) // CPU

	cpu, mobo := collectCPUMoboTemps()
	if cpu != 64 {
		t.Errorf("collectCPUMoboTemps() cpu = %d, want 64 (x86_pkg_temp thermal zone)", cpu)
	}
	if mobo != 41 {
		t.Errorf("collectCPUMoboTemps() mobo = %d, want 41 (acpitz thermal zone)", mobo)
	}
}

// TestCollectCPUMoboTemps_AcpitzHwmon — mainboard temp comes from acpitz
// hwmon when no thermal_zone exposes it.
func TestCollectCPUMoboTemps_AcpitzHwmon(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 0, "coretemp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"Package id 0", 60000},
	})
	seedHwmon(t, base, 1, "acpitz", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"", 42000},
	})

	cpu, mobo := collectCPUMoboTemps()
	if cpu != 60 {
		t.Errorf("collectCPUMoboTemps() cpu = %d, want 60 (coretemp Package id 0)", cpu)
	}
	if mobo != 42 {
		t.Errorf("collectCPUMoboTemps() mobo = %d, want 42 (acpitz hwmon)", mobo)
	}
}

// TestCollectCPUMoboTemps_NothingAvailable — Synology, K8s pods, and
// other environments without /sys/class/hwmon must return (0, 0). The
// dashboard header renders no gauge in that case (graceful fallback,
// per acceptance criteria item 3).
func TestCollectCPUMoboTemps_NothingAvailable(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)
	// Empty tree.

	cpu, mobo := collectCPUMoboTemps()
	if cpu != 0 || mobo != 0 {
		t.Errorf("collectCPUMoboTemps() = (%d, %d), want (0, 0) on empty /sys", cpu, mobo)
	}
}

// TestCollectCPUMoboTemps_ImplausibleReadingsFiltered — sensors
// occasionally report nonsense (e.g. 0, negative, or 250°C from a
// disconnected probe). Anything outside (0, 120] must be treated as
// "not available" rather than surfaced. Same defence applied to
// readThermalZoneTemp; mirror it for the new collector to avoid
// shipping 250°C readings to the header.
func TestCollectCPUMoboTemps_ImplausibleReadingsFiltered(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 0, "coretemp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"Package id 0", 250000}, // 250°C — impossible
	})
	seedHwmon(t, base, 1, "acpitz", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"", 0}, // 0°C — sensor disconnected
	})

	cpu, mobo := collectCPUMoboTemps()
	if cpu != 0 {
		t.Errorf("collectCPUMoboTemps() cpu = %d, want 0 (250°C must be rejected)", cpu)
	}
	if mobo != 0 {
		t.Errorf("collectCPUMoboTemps() mobo = %d, want 0 (0°C must be rejected)", mobo)
	}
}

// TestFindHwmonByName verifies the helper that scans every
// /sys/class/hwmon/hwmon*/name entry and returns the directory whose
// name matches. Multiple hwmon devices are common (coretemp +
// nvme + acpitz + nct6798 ...), so the helper must scan them all,
// not just hwmon0.
func TestFindHwmonByName_FindsByName(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 0, "nvme", map[string]struct {
		label    string
		millidec int
	}{"temp1": {"", 35000}})
	seedHwmon(t, base, 1, "coretemp", map[string]struct {
		label    string
		millidec int
	}{"temp1": {"Package id 0", 58000}})
	seedHwmon(t, base, 2, "acpitz", map[string]struct {
		label    string
		millidec int
	}{"temp1": {"", 42000}})

	got := findHwmonByName("coretemp")
	if got == "" {
		t.Fatal("findHwmonByName('coretemp') returned empty; expected hwmon1 directory")
	}
	// The returned path should END with /hwmon1 — not hwmon0 (nvme) or
	// hwmon2 (acpitz).
	if want := "/hwmon1"; !endsWith(got, want) {
		t.Errorf("findHwmonByName('coretemp') = %q, expected suffix %q", got, want)
	}

	if got := findHwmonByName("acpitz"); !endsWith(got, "/hwmon2") {
		t.Errorf("findHwmonByName('acpitz') = %q, expected suffix /hwmon2", got)
	}

	if got := findHwmonByName("nonexistent"); got != "" {
		t.Errorf("findHwmonByName('nonexistent') = %q, want empty string", got)
	}
}

// endsWith is a tiny helper to keep the assertion readable; we don't
// want to depend on strings just for this.
func endsWith(s, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
