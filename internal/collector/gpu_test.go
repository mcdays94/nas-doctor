package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── readThermalZoneTemp ──
//
// These tests simulate /sys/class by creating a temp directory and pointing
// sysClassBase at it. The function under test reads from that simulated tree.

// withSysClassBase redirects sysClassBase to the given path for the duration
// of a test, and restores it afterwards.
func withSysClassBase(t *testing.T, path string) {
	t.Helper()
	orig := sysClassBase
	sysClassBase = path
	t.Cleanup(func() { sysClassBase = orig })
}

// writeFile is a tiny helper: makes parent dirs, writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// seedThermalZone creates /sys/class/thermal/thermal_zoneN/{type,temp} with
// the given type string and millidegree value.
func seedThermalZone(t *testing.T, base string, n int, zoneType string, milliDegrees int) {
	t.Helper()
	dir := filepath.Join(base, "thermal", "thermal_zone"+itoa(n))
	writeFile(t, filepath.Join(dir, "type"), zoneType+"\n")
	writeFile(t, filepath.Join(dir, "temp"), itoa(milliDegrees)+"\n")
}

// seedHwmon creates /sys/class/hwmon/hwmonN/name plus any temp*_input and
// temp*_label pairs supplied in entries.
func seedHwmon(t *testing.T, base string, n int, name string, entries map[string]struct {
	label    string
	millidec int
}) {
	t.Helper()
	dir := filepath.Join(base, "hwmon", "hwmon"+itoa(n))
	writeFile(t, filepath.Join(dir, "name"), name+"\n")
	for slot, e := range entries {
		writeFile(t, filepath.Join(dir, slot+"_input"), itoa(e.millidec)+"\n")
		if e.label != "" {
			writeFile(t, filepath.Join(dir, slot+"_label"), e.label+"\n")
		}
	}
}

func itoa(i int) string {
	// Inline minimal itoa to avoid importing strconv in the helper block.
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// TestReadThermalZoneTemp_PrefersX86PkgTemp is the happy path.
func TestReadThermalZoneTemp_PrefersX86PkgTemp(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedThermalZone(t, base, 0, "acpitz", 98000)       // must NOT be picked
	seedThermalZone(t, base, 1, "x86_pkg_temp", 62000) // must be picked

	got := readThermalZoneTemp()
	if got != 62 {
		t.Errorf("readThermalZoneTemp() = %d, want 62 (x86_pkg_temp preferred over acpitz)", got)
	}
}

// TestReadThermalZoneTemp_SkipsAcpitzEvenAsOnlySource is the #157 regression
// guard. Before the fix, the old secondary fallback picked the first thermal
// zone with 0<t<120, which on Unraid routinely lands on acpitz reporting 98°C
// (the ACPI critical trip point) instead of current temp.
func TestReadThermalZoneTemp_SkipsAcpitzEvenAsOnlySource(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedThermalZone(t, base, 0, "acpitz", 98000)
	seedThermalZone(t, base, 1, "acpitz", 45000)

	got := readThermalZoneTemp()
	if got != 0 {
		t.Errorf("readThermalZoneTemp() = %d, want 0 (acpitz must not be surfaced — it's the misleading 98°C source)", got)
	}
}

// TestReadThermalZoneTemp_FallsBackToCoretempHwmon covers the case where
// the kernel doesn't expose an x86_pkg_temp thermal zone but coretemp IS
// available via hwmon.
func TestReadThermalZoneTemp_FallsBackToCoretempHwmon(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	// No x86_pkg_temp, only acpitz (must be skipped)
	seedThermalZone(t, base, 0, "acpitz", 98000)

	// coretemp hwmon: Package id 0 labelled + per-core inputs
	seedHwmon(t, base, 1, "coretemp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"Package id 0", 58000},
		"temp2": {"Core 0", 57000},
		"temp3": {"Core 1", 59000},
	})

	got := readThermalZoneTemp()
	if got != 58 {
		t.Errorf("readThermalZoneTemp() = %d, want 58 (coretemp Package id 0)", got)
	}
}

// TestReadThermalZoneTemp_CoretempWithoutPackageLabel uses temp1_input as the
// conventional package sensor when no labels are defined.
func TestReadThermalZoneTemp_CoretempWithoutPackageLabel(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)

	seedHwmon(t, base, 0, "coretemp", map[string]struct {
		label    string
		millidec int
	}{
		"temp1": {"", 55000},
	})

	got := readThermalZoneTemp()
	if got != 55 {
		t.Errorf("readThermalZoneTemp() = %d, want 55 (temp1_input on coretemp without label)", got)
	}
}

// TestReadThermalZoneTemp_NothingUsable returns 0 so the caller can treat it
// as "no CPU temp available" rather than surfacing a misleading fallback.
func TestReadThermalZoneTemp_NothingUsable(t *testing.T) {
	base := t.TempDir()
	withSysClassBase(t, base)
	// No thermal zones, no hwmon.

	got := readThermalZoneTemp()
	if got != 0 {
		t.Errorf("readThermalZoneTemp() = %d, want 0 (no sources available)", got)
	}
}

// ── collectIntel regression guard ──
//
// We can't unit-test the full collectIntel flow without simulating
// /sys/class/drm/ too, but we CAN assert at the source level that the
// CPU-package-temp fallback was removed. This catches a future refactor
// that reintroduces the misleading "GPU temperature" approximation.

// TestCollectIntel_DoesNotFallBackToCPUPackageTemp verifies the source
// comment + absence of the old fallback call.
func TestCollectIntel_DoesNotFallBackToCPUPackageTemp(t *testing.T) {
	data, err := os.ReadFile("gpu.go")
	if err != nil {
		t.Fatalf("read gpu.go: %v", err)
	}
	src := string(data)
	// The old fallback invocation must not appear inside collectIntel.
	// We scope the search to the function body.
	startRe := "func collectIntel()"
	start := strings.Index(src, startRe)
	if start < 0 {
		t.Fatal("collectIntel function not found in gpu.go")
	}
	// The function body roughly spans ~1600 bytes — take a generous window.
	end := start + 4000
	if end > len(src) {
		end = len(src)
	}
	body := src[start:end]

	if strings.Contains(body, "gpu.Temperature = readThermalZoneTemp()") {
		t.Error("collectIntel still falls back to CPU package temp for GPU temperature — see issue #157 for why this is removed")
	}
	if !strings.Contains(body, "issue #157") {
		t.Error("collectIntel missing the comment explaining why the CPU fallback was removed (see issue #157)")
	}
}
