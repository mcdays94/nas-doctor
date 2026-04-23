package collector

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestCollectSMART_ReturnsStandbyDeviceList pins the signal that the
// scheduler's StaleSMARTChecker (issue #238) depends on: when
// `-n standby` skips a drive, its device path must be surfaced to the
// caller so the scheduler can evaluate its age against MaxAgeDays.
//
// Prior to issue #238 the collector silently dropped standby drives
// and the caller had no way to learn of them. The new contract is
// that collectSMART's third return value is the list of device paths
// that were skipped for standby this cycle.
func TestCollectSMART_ReturnsStandbyDeviceList(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*")
	}

	// Three fake devs: active, standby, active.
	activeJSON := `{"json_format_version":[1,0,0],"model_name":"Active","serial_number":"SN-ACTIVE","user_capacity":{"bytes":1000000000000}}`
	standbyOut := "smartctl 7.3\n\nDevice is in STANDBY mode, exit(2)\n"

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--scan" {
			return "/dev/fakeA -d sat\n/dev/fakeStandby -d sat\n/dev/fakeB -d sat\n", nil
		}
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "/dev/fakeStandby"):
			return standbyOut, nil
		default:
			return activeJSON, nil
		}
	})()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	results, standby, err := collectSMART(SMARTConfig{WakeDrives: false}, logger)
	if err != nil {
		t.Fatalf("collectSMART: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 active results, got %d", len(results))
	}
	if len(standby) != 1 || standby[0] != "/dev/fakeStandby" {
		t.Errorf("expected standby=[/dev/fakeStandby], got %v", standby)
	}
}
