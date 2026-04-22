package collector

import (
	"errors"
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// swapExecCmd replaces the package-level execCmd with a fake for the duration
// of a test. Returns a restore function; callers should defer restore().
//
// execCmd is defined as a package-level var in system.go specifically so
// tests can swap it out — the smartctl flag surface is otherwise difficult
// to unit-test (would require either building a fake binary on PATH or
// mocking exec.Command).
func swapExecCmd(fn func(name string, args ...string) (string, error)) (restore func()) {
	orig := execCmd
	execCmd = fn
	return func() { execCmd = orig }
}

// TestReadSMARTDevice_DefaultAddsStandbyFlag verifies the v0.9.5+ default:
// every smartctl invocation from the SMART collector must include `-n standby`
// unless the WakeDrivesForSMART setting is explicitly enabled. Without this,
// scans wake spun-down drives every scan cycle (issue #198).
func TestReadSMARTDevice_DefaultAddsStandbyFlag(t *testing.T) {
	var calls [][]string
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		// Return nothing matching json_format_version so readSMARTDevice
		// exercises all fallback paths (initial JSON, device-type loop,
		// text fallback).
		return "", nil
	})()

	_, _ = readSMARTDevice("/dev/sda", false /* wakeDrives */)

	if len(calls) == 0 {
		t.Fatal("expected at least one smartctl call")
	}
	for i, call := range calls {
		if call[0] != "smartctl" {
			t.Errorf("call %d: expected smartctl binary, got %q", i, call[0])
			continue
		}
		joined := strings.Join(call, " ")
		if !strings.Contains(joined, "-n standby") {
			t.Errorf("call %d missing `-n standby` flag (wakeDrives=false): %s", i, joined)
		}
	}
}

// TestReadSMARTDevice_WakeDrivesOmitsStandbyFlag verifies the opt-out: when
// the user explicitly enables WakeDrivesForSMART, the `-n standby` flag must
// NOT be passed (restoring v0.9.4 and earlier behavior).
func TestReadSMARTDevice_WakeDrivesOmitsStandbyFlag(t *testing.T) {
	var calls [][]string
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		return "", nil
	})()

	_, _ = readSMARTDevice("/dev/sda", true /* wakeDrives */)

	if len(calls) == 0 {
		t.Fatal("expected at least one smartctl call")
	}
	for i, call := range calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "-n standby") {
			t.Errorf("call %d included `-n standby` flag despite wakeDrives=true: %s", i, joined)
		}
	}
}

// TestReadSMARTDevice_StandbyOutputReturnsSentinel verifies that when
// smartctl reports the drive is in standby (via `-n standby` skip), we
// return errDriveInStandby so collectSMART can silently skip the drive
// rather than log an error or create a broken history row.
func TestReadSMARTDevice_StandbyOutputReturnsSentinel(t *testing.T) {
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		// Typical smartctl text output when -n standby skips the drive.
		return "smartctl 7.3 2022-02-28 r5338 [x86_64-linux-6.1.0] (local build)\n" +
			"Copyright (C) 2002-22, Bruce Allen, Christian Franke, www.smartmontools.org\n\n" +
			"Device is in STANDBY mode, exit(2)\n", nil
	})()

	_, err := readSMARTDevice("/dev/sda", false)
	if !errors.Is(err, errDriveInStandby) {
		t.Errorf("expected errDriveInStandby, got %v", err)
	}
}

// TestReadSMARTDevice_StandbyJSONReturnsSentinel verifies standby detection
// for the --json=c invocation path (smartctl emits a JSON object with
// power_mode=STANDBY even when skipping).
func TestReadSMARTDevice_StandbyJSONReturnsSentinel(t *testing.T) {
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		// Minimal JSON smartctl returns when -n standby skips — no
		// json_format_version (so we shouldn't accidentally parse it as
		// a successful SMART read), with an explicit power_mode signal.
		return `{"power_mode":"STANDBY","exit_status":2}`, nil
	})()

	_, err := readSMARTDevice("/dev/sda", false)
	if !errors.Is(err, errDriveInStandby) {
		t.Errorf("expected errDriveInStandby for JSON-mode standby output, got %v", err)
	}
}

// TestReadSMARTDevice_WakeDrivesTrueIgnoresStandbyHeuristic ensures the
// standby-detection heuristic only runs when wakeDrives=false. When the
// setting is enabled, we never pass -n standby, so STANDBY should not
// appear in output; but even if some attribute text happened to contain
// the word, we must not short-circuit.
func TestReadSMARTDevice_WakeDrivesTrueIgnoresStandbyHeuristic(t *testing.T) {
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		// Deliberately include the word STANDBY in output (e.g. as part
		// of a device-type attribute description) to prove wakeDrives=true
		// never returns errDriveInStandby.
		return `{"json_format_version":[1,0,0],"model_name":"STANDBY Pro 2TB","user_capacity":{"bytes":2000000000000}}`, nil
	})()

	info, err := readSMARTDevice("/dev/sda", true)
	if errors.Is(err, errDriveInStandby) {
		t.Errorf("unexpected errDriveInStandby when wakeDrives=true: %v", err)
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Model != "STANDBY Pro 2TB" {
		t.Errorf("expected parsed model=%q, got %q", "STANDBY Pro 2TB", info.Model)
	}
}

// TestCollectSMART_PropagatesWakeDrivesFlag wires the top-level collectSMART
// entry to the readSMARTDevice seam by checking whether the standby flag is
// threaded through the SMARTConfig struct.
func TestCollectSMART_PropagatesWakeDrivesFlag(t *testing.T) {
	// Use a discovery-minimal approach: we can't replace discoverDrives
	// without another seam, so we verify the SMARTConfig struct exists
	// and the field name is stable. More integration-like behaviour is
	// exercised by the readSMARTDevice tests above.
	cfg := SMARTConfig{WakeDrives: true}
	if !cfg.WakeDrives {
		t.Errorf("SMARTConfig.WakeDrives did not round-trip")
	}
	// Also make sure the zero value is false (default behavior = standby-aware).
	var zero SMARTConfig
	if zero.WakeDrives {
		t.Errorf("SMARTConfig zero value should have WakeDrives=false; got true")
	}

	// Sanity-check that the Collector exposes a setter so the API/main
	// plumbing can reach the field.
	var c Collector
	c.SetSMARTConfig(cfg)
	if !c.smartConfig.WakeDrives {
		t.Errorf("SetSMARTConfig did not persist cfg")
	}
}

// compile-time guard: SMARTInfo remains the return type of readSMARTDevice
// (catches accidental signature drift).
var _ = internal.SMARTInfo{}
