package collector

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #300 — Synology DSM was mis-detected as "alpine" inside
// nas_doctor containers because the existing detection paths require
// either /etc/synoinfo.conf bind mounts or a /proc/version banner
// that mentions "synology". Neither is present in the typical
// Container Manager deployment described in the Synology section of
// the README. As a result, snap.System.Platform reported "alpine",
// blocking every Synology-aware finding (including the storage-mount
// hint added alongside this fix).
//
// These tests pin the new third detection signal: Synology-specific
// kernel sysctl files exposed under /proc/sys/kernel/syno_* by every
// DSM kernel build. Linux exposes /proc fully into containers by
// default so these are visible without bind mounts.

// swapSynologyKernelMarkers replaces the package-level
// synologyKernelMarkers slice with `paths` for the duration of a
// test. Returns a restore function; callers should defer restore().
func swapSynologyKernelMarkers(paths []string) (restore func()) {
	orig := synologyKernelMarkers
	synologyKernelMarkers = paths
	return func() { synologyKernelMarkers = orig }
}

func TestHasSynologyKernelMarker_DetectsExistingFile(t *testing.T) {
	tmp := t.TempDir()
	// Create one of the marker files. The detection only requires
	// any single match; production has three but a future kernel
	// dropping two of them would still detect.
	marker := filepath.Join(tmp, "syno_CPU_info_clock")
	if err := os.WriteFile(marker, []byte("2400\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	defer swapSynologyKernelMarkers([]string{marker})()

	if !hasSynologyKernelMarker() {
		t.Errorf("expected hasSynologyKernelMarker()=true when marker file exists at %s", marker)
	}
}

func TestHasSynologyKernelMarker_DetectsAnyOfMultiple(t *testing.T) {
	tmp := t.TempDir()
	// Three candidate paths, only the LAST one exists. Mirrors the
	// production behaviour where the fallback list has multiple
	// candidates so a single kernel-rename doesn't break detection.
	exists := filepath.Join(tmp, "syno_ata_debug")
	if err := os.WriteFile(exists, []byte("0\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	defer swapSynologyKernelMarkers([]string{
		filepath.Join(tmp, "missing-1"),
		filepath.Join(tmp, "missing-2"),
		exists,
	})()

	if !hasSynologyKernelMarker() {
		t.Errorf("expected detection to succeed when ANY marker exists")
	}
}

func TestHasSynologyKernelMarker_FalseWhenNoneExist(t *testing.T) {
	tmp := t.TempDir()
	// All paths reference files that don't exist. Pin that we don't
	// false-positive on plain Linux / Alpine / Unraid hosts.
	defer swapSynologyKernelMarkers([]string{
		filepath.Join(tmp, "missing-1"),
		filepath.Join(tmp, "missing-2"),
		filepath.Join(tmp, "missing-3"),
	})()

	if hasSynologyKernelMarker() {
		t.Errorf("expected hasSynologyKernelMarker()=false when no markers exist")
	}
}

func TestHasSynologyKernelMarker_EmptyList(t *testing.T) {
	// Defensive: an empty marker list (e.g. someone wipes the
	// constants without realising) must return false rather than
	// panic or accidentally always-true.
	defer swapSynologyKernelMarkers([]string{})()

	if hasSynologyKernelMarker() {
		t.Errorf("expected hasSynologyKernelMarker()=false for empty marker list")
	}
}

// TestSynologyKernelMarkers_ContainsExpectedPaths is a structural
// guard: we want at least the three production paths in the default
// list so the detection has redundancy across DSM kernel versions.
// Tests should fail loudly if a refactor accidentally trims the
// list, since the issue-#300 detection silently degrades otherwise.
func TestSynologyKernelMarkers_ContainsExpectedPaths(t *testing.T) {
	want := map[string]bool{
		"/proc/sys/kernel/syno_CPU_info_clock": true,
		"/proc/sys/kernel/syno_CPU_info_core":  true,
		"/proc/sys/kernel/syno_ata_debug":      true,
	}
	got := make(map[string]bool, len(synologyKernelMarkers))
	for _, p := range synologyKernelMarkers {
		got[p] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("synologyKernelMarkers missing expected path %q (full list: %v)", w, synologyKernelMarkers)
		}
	}
}
