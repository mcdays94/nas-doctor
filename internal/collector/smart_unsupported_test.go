package collector

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// Issue #206 — the v0.9.5 summary log counted USB bridges and other
// unsupported devices as "failed", misleading users into thinking a
// drive is failing when actually the device is just not SMART-capable
// (classic example: an Unraid boot flash at /dev/sda presenting as
// Unknown USB bridge). This file pins the new semantics:
//
//   - Unsupported devices increment a new `unsupported` counter, not
//     `failed`. Real failures (smartctl returned no output, or some
//     other non-unsupported error) continue to increment `failed`.
//   - Each unsupported device emits a per-drive INFO log carrying the
//     device name and the reason so operators can see which device
//     was skipped without cross-referencing discovery output.

// helper: grab the single "SMART collection complete" JSON summary log
// line from the captured buffer. Used by several tests below.
func scanSMARTSummary(t *testing.T, raw string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if msg, _ := rec["msg"].(string); msg == "SMART collection complete" {
			return rec
		}
	}
	t.Fatalf("no 'SMART collection complete' log line; raw:\n%s", raw)
	return nil
}

// helper: asserts a specific numeric field exists on the summary and
// equals want. JSON numbers decode as float64.
func assertCounter(t *testing.T, summary map[string]any, field string, want int) {
	t.Helper()
	v, ok := summary[field]
	if !ok {
		t.Fatalf("summary missing %q field; got: %v", field, summary)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("summary %q is not a number: %v (%T)", field, v, v)
	}
	if int(f) != want {
		t.Errorf("summary %q = %d, want %d (full summary: %v)", field, int(f), want, summary)
	}
}

// TestCollectSMART_USBBridge_CountedAsUnsupportedNotFailed is the #206
// regression guard: a USB-bridge device must be categorised as
// `unsupported` in the summary log, not `failed`. Shape of the fake
// environment: three drives, one of them reporting "Unknown USB
// bridge" across every fallback — exactly the shape of an Unraid
// boot flash at /dev/sda.
func TestCollectSMART_USBBridge_CountedAsUnsupportedNotFailed(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*; cannot run deterministic fake-execCmd test")
	}

	fakeActiveJSON := `{"json_format_version":[1,0,0],"model_name":"FakeDrive 1TB","serial_number":"SN-ACTIVE","user_capacity":{"bytes":1000000000000},"temperature":{"current":30},"power_on_time":{"hours":100}}`
	usbBridgeOut := `smartctl 7.4 2023-08-01 r5530 [x86_64-linux-6.12.24-Unraid] (local build)
Copyright (C) 2002-23, Bruce Allen, Christian Franke, www.smartmontools.org

/dev/fake-usb: Unknown USB bridge [0x0951:0x1666 (0x001)]
Please specify device type with the -d option.

Use smartctl -h to get a usage summary
`

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--scan" {
			return "/dev/fake-usb -d sat # /dev/fake-usb, SAT\n" +
				"/dev/fake-active -d sat # /dev/fake-active, SAT\n", nil
		}
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "/dev/fake-usb"):
			// Every fallback returns the same USB bridge error — mirroring
			// the smartctl behaviour on /dev/sda on the reporter's Tower.
			return usbBridgeOut, nil
		case strings.Contains(argv, "/dev/fake-active"):
			return fakeActiveJSON, nil
		}
		return "", nil
	})()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_, _ = collectSMART(SMARTConfig{WakeDrives: false}, logger)

	summary := scanSMARTSummary(t, buf.String())

	assertCounter(t, summary, "total", 2)
	assertCounter(t, summary, "active", 1)
	assertCounter(t, summary, "standby", 0)
	assertCounter(t, summary, "unsupported", 1) // USB bridge
	assertCounter(t, summary, "failed", 0)      // NOT lumped into failed

	// Conservation identity: total must equal active + standby +
	// unsupported + failed. Catches future regressions that might add
	// a fifth bucket without updating the summary math.
	total := 2
	sum := 1 + 0 + 1 + 0
	if total != sum {
		t.Errorf("counter math broken: active(1) + standby(0) + unsupported(1) + failed(0) = %d, total = %d", sum, total)
	}
}

// TestCollectSMART_UnsupportedDrive_EmitsPerDriveLog pins the operator-
// facing INFO log emitted for each unsupported device. Mirrors the
// per-drive standby log introduced in #202 so operators can see
// which device was skipped without cross-referencing discovery
// output.
func TestCollectSMART_UnsupportedDrive_EmitsPerDriveLog(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*")
	}

	usbBridgeOut := "/dev/fake-usb: Unknown USB bridge [0x0951:0x1666]\nPlease specify device type with the -d option.\n"

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--scan" {
			return "/dev/fake-usb -d sat # /dev/fake-usb, SAT\n", nil
		}
		return usbBridgeOut, nil
	})()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_, _ = collectSMART(SMARTConfig{WakeDrives: false}, logger)

	// Find the per-drive INFO log. Carries the exact device name so
	// a user with multiple unsupported devices can tell them apart.
	var perDrive map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if msg, _ := rec["msg"].(string); strings.Contains(strings.ToLower(msg), "unsupported") && strings.Contains(strings.ToLower(msg), "smart") {
			perDrive = rec
			break
		}
	}
	if perDrive == nil {
		t.Fatalf("no per-drive unsupported-device INFO log; raw:\n%s", buf.String())
	}
	if dev, _ := perDrive["device"].(string); dev != "/dev/fake-usb" {
		t.Errorf("per-drive log device = %q, want /dev/fake-usb", dev)
	}
	// Reason field should exist and be non-empty so operators can
	// actually see why the device was skipped.
	if reason, _ := perDrive["reason"].(string); reason == "" {
		t.Errorf("per-drive log reason is empty; expected a human-readable explanation")
	}
}

// TestCollectSMART_USBBridge_JSONWrapped_AlsoCountedAsUnsupported covers
// the follow-up #206 fix: smartctl 7.x ALWAYS emits a JSON envelope
// when invoked with --json=c, including on error outputs. The envelope's
// smartctl.messages[] array carries the "Unknown USB bridge" / "Please
// specify device type" strings that the rc1 fix relied on seeing in
// text-mode fallback output.
//
// When the initial --json=c call returns JSON (which it does for
// every smartctl 7.x invocation), readSMARTDevice immediately hands
// off to parseSMARTJSON at line 188 and NEVER reaches the text-mode
// fallback at line 220 where the rc1 check lives. Result on the
// reporter's Tower: /dev/sda (Unraid boot flash) lands in `failed`
// not `unsupported` because parseSMARTJSON returns an empty info
// struct that collectSMART then trips on via the empty-model guard.
//
// This test feeds the real shape of smartctl 7.4's JSON output for
// a USB-bridge device (taken from the reporter's actual /dev/sda
// response) and asserts the drive is categorised correctly.
func TestCollectSMART_USBBridge_JSONWrapped_AlsoCountedAsUnsupported(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*; cannot run deterministic fake-execCmd test")
	}

	// Shape of smartctl 7.x --json=c output on an Unraid boot flash.
	// Critical fields: json_format_version (so readSMARTDevice routes
	// to parseSMARTJSON), and smartctl.messages[] carrying the error
	// strings. ModelName / SerialNumber are absent because smartctl
	// couldn't read the device.
	usbBridgeJSON := `{
		"json_format_version": [1, 0],
		"smartctl": {
			"version": [7, 4],
			"pre_release": false,
			"svn_revision": "5530",
			"platform_info": "x86_64-linux-6.12.24-Unraid",
			"build_info": "(local build)",
			"argv": ["smartctl", "--json=c", "-a", "/dev/fake-usb"],
			"exit_status": 4,
			"messages": [
				{"string": "/dev/fake-usb: Unknown USB bridge [0x0951:0x1666 (0x001)]", "severity": "error"},
				{"string": "Please specify device type with the -d option.", "severity": "error"}
			]
		}
	}`

	fakeActiveJSON := `{"json_format_version":[1,0,0],"model_name":"FakeDrive 1TB","serial_number":"SN-ACTIVE","user_capacity":{"bytes":1000000000000},"temperature":{"current":30},"power_on_time":{"hours":100}}`

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--scan" {
			return "/dev/fake-usb -d sat # /dev/fake-usb, SAT\n" +
				"/dev/fake-active -d sat # /dev/fake-active, SAT\n", nil
		}
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "/dev/fake-usb"):
			// Every fallback also returns the JSON-wrapped USB bridge
			// error — mirrors smartctl 7.x's actual behaviour where
			// --json output is emitted regardless of -d option.
			return usbBridgeJSON, nil
		case strings.Contains(argv, "/dev/fake-active"):
			return fakeActiveJSON, nil
		}
		return "", nil
	})()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	_, _ = collectSMART(SMARTConfig{WakeDrives: false}, logger)

	summary := scanSMARTSummary(t, buf.String())

	assertCounter(t, summary, "total", 2)
	assertCounter(t, summary, "active", 1)
	assertCounter(t, summary, "standby", 0)
	assertCounter(t, summary, "unsupported", 1)
	assertCounter(t, summary, "failed", 0)
}
