package collector

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// TestCollectSMARTForced_OmitsStandbyFlag is the defining contract of
// the new force-wake path (issue #238). Every smartctl invocation made
// by CollectSMARTForced MUST NOT carry `-n standby` — the whole point
// is to wake the drive and refresh SMART data even if it's spun down.
//
// If a future refactor accidentally pipes the default SMARTConfig
// (WakeDrives=false) into the forced path, the max-age safety net
// becomes a no-op. This test pins that contract.
func TestCollectSMARTForced_OmitsStandbyFlag(t *testing.T) {
	var calls [][]string
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		return `{"json_format_version":[1,0,0],"model_name":"Forced 4TB","serial_number":"SN-F1","user_capacity":{"bytes":4000000000000}}`, nil
	})()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	results, err := CollectSMARTForced([]string{"/dev/sda"}, logger)
	if err != nil {
		t.Fatalf("CollectSMARTForced: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Device != "/dev/sda" {
		t.Errorf("result device = %q, want %q", results[0].Device, "/dev/sda")
	}
	if results[0].Serial != "SN-F1" {
		t.Errorf("result serial = %q, want SN-F1", results[0].Serial)
	}

	if len(calls) == 0 {
		t.Fatal("expected at least one smartctl call")
	}
	for i, call := range calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "-n standby") {
			t.Errorf("call %d carries `-n standby` despite force-wake path: %s", i, joined)
		}
		if !strings.Contains(joined, "/dev/sda") {
			t.Errorf("call %d missing target device /dev/sda: %s", i, joined)
		}
	}
}

// TestCollectSMARTForced_PartialFailureContinues verifies that when one
// device in the list fails SMART read, the remaining devices are still
// processed. Matches the graceful-degradation requirement in issue #238
// ("one-drive failure doesn't block others").
func TestCollectSMARTForced_PartialFailureContinues(t *testing.T) {
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "/dev/sdgood"):
			return `{"json_format_version":[1,0,0],"model_name":"Good 2TB","serial_number":"SN-GOOD","user_capacity":{"bytes":2000000000000}}`, nil
		case strings.Contains(argv, "/dev/sdbad"):
			// No parseable output at all — collectSMART's existing fall-through
			// treats this as a failed read (not standby, not unsupported).
			return "", errors.New("smartctl exploded")
		}
		return "", nil
	})()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	results, err := CollectSMARTForced([]string{"/dev/sdbad", "/dev/sdgood"}, logger)
	// We tolerate a non-nil err (so the caller can log it), but the good
	// device MUST be in the results regardless.
	_ = err

	foundGood := false
	for _, r := range results {
		if r.Device == "/dev/sdgood" {
			foundGood = true
		}
	}
	if !foundGood {
		t.Errorf("expected /dev/sdgood in results despite /dev/sdbad failure; got %+v", results)
	}
}

// TestCollectSMARTForced_EmptyListNoCalls — a CollectSMARTForced call
// with no devices should be a fast no-op; it must not invoke smartctl
// at all (important because the scheduler will call this on every scan
// cycle, and on cycles with no stale drives the list is empty).
func TestCollectSMARTForced_EmptyListNoCalls(t *testing.T) {
	var calls int
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls++
		return "", nil
	})()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	results, err := CollectSMARTForced(nil, logger)
	if err != nil {
		t.Errorf("empty list should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty list must produce empty results; got %d", len(results))
	}
	if calls != 0 {
		t.Errorf("empty list must make 0 smartctl calls; got %d", calls)
	}
}

// TestCollectSMARTForced_LogsPerDevice pins the INFO log emitted for
// each successful force-wake. The scheduler's StaleSMARTChecker emits
// the "forcing SMART wake on ..." line at the orchestration layer; the
// collector emits a lower-level "force-read SMART" per device so
// operators can distinguish force-reads from standby skips in logs.
func TestCollectSMARTForced_LogsPerDevice(t *testing.T) {
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		return `{"json_format_version":[1,0,0],"model_name":"X","serial_number":"X","user_capacity":{"bytes":1000000000000}}`, nil
	})()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_, _ = CollectSMARTForced([]string{"/dev/sda", "/dev/sdb"}, logger)

	var matches int
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if msg, _ := rec["msg"].(string); strings.Contains(msg, "force-read SMART") {
			matches++
		}
	}
	if matches != 2 {
		t.Errorf("expected 2 per-device force-read log lines, got %d; log:\n%s", matches, buf.String())
	}
}
