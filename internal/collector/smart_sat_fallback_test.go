package collector

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// Issue #298 — on a Synology DS918+ (DSM 7, smartctl 6.5) two SATA
// bays presented WD Red WD30EFRX drives via a path that smartctl
// auto-detected as SCSI rather than SATA. Bare `smartctl -a /dev/sdc`
// returned a SCSI-mode information section ending with:
//
//   SMART support is:     Unavailable - device lacks SMART capability.
//
// The text was nowhere in the SAT-fallback trigger list, so the retry
// loop never fired, parseSMARTText found no `Device Model:` line
// (output uses `Vendor:` / `Product:` for SCSI), returned an empty
// SMARTInfo, and collectSMART then mis-categorised the drives as
// `failed`. Adding `-d sat` made smartctl return full ATA SMART data
// — these tests pin that retry behaviour for both transports.
//
// The fixture strings below are verbatim captures from the reporter's
// DS918+ — they're deliberately not minimised so future grep-based
// debugging against real smartctl output stays straightforward.

// scsiAutoDetectOutput is what smartctl 6.5 emits when its auto-detect
// picks SCSI for a SATA drive on the Apollo Lake on-SoC AHCI port.
// `Vendor` / `Product` instead of `Device Model`, `LU is fully
// provisioned` (a SCSI INQUIRY field), and the giveaway final line.
const scsiAutoDetectOutput = `smartctl 6.5 (build date Sep 26 2022) [x86_64-linux-4.4.302+] (local build)
Copyright (C) 2002-16, Bruce Allen, Christian Franke, www.smartmontools.org

=== START OF INFORMATION SECTION ===
Vendor:               WDC
Product:              WD30EFRX-68EUZN0
Revision:             0A82
User Capacity:        3,000,592,982,016 bytes [3.00 TB]
Logical block size:   512 bytes
Physical block size:  4096 bytes
LU is fully provisioned
Rotation Rate:        5400 rpm
Logical Unit id:      0x50014ee2665ddf18
Serial number:        WD-WCC4N3FV0HNV
Device type:          disk
Local Time is:        Mon Apr 27 15:23:48 2026 GMT
SMART support is:     Unavailable - device lacks SMART capability.

=== START OF READ SMART DATA SECTION ===
Current Drive Temperature:     0 C
Drive Trip Temperature:        0 C

Error Counter logging not supported


[GLTSD (Global Logging Target Save Disable) set. Enable Save with '-S on']
Device does not support Self Test logging
`

// satRetryTextOutput is what `smartctl -d sat -a /dev/sdc` returns for
// the same drive — the SAT translation reaches the underlying ATA
// controller and we get a full information section with attributes.
// Trimmed to the lines parseSMARTText actually consumes, plus a
// couple of the standard attribute rows so power-on hours and
// temperature land correctly.
const satRetryTextOutput = `smartctl 6.5 (build date Sep 26 2022) [x86_64-linux-4.4.302+] (local build)
Copyright (C) 2002-16, Bruce Allen, Christian Franke, www.smartmontools.org

=== START OF INFORMATION SECTION ===
Model Family:     Western Digital Red (CMR)
Device Model:     WDC WD30EFRX-68EUZN0
Serial Number:    WD-WCC4N3FV0HNV
LU WWN Device Id: 5 0014ee 2665ddf18
Firmware Version: 82.00A82
User Capacity:    3,000,592,982,016 bytes [3.00 TB]
Sector Sizes:     512 bytes logical, 4096 bytes physical
Rotation Rate:    5400 rpm
Device is:        In smartctl database [for details use: -P show]
ATA Version is:   ACS-2 (minor revision not indicated)
SATA Version is:  SATA 3.0, 6.0 Gb/s (current: 6.0 Gb/s)
SMART support is: Available - device has SMART capability.
SMART support is: Enabled

=== START OF READ SMART DATA SECTION ===
SMART overall-health self-assessment test result: PASSED

ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
  5 Reallocated_Sector_Ct   0x0033   200   200   140    Pre-fail  Always       -       0
  9 Power_On_Hours          0x0032   018   018   000    Old_age   Always       -       59942
194 Temperature_Celsius     0x0022   116   108   000    Old_age   Always       -       34
197 Current_Pending_Sector  0x0032   200   200   000    Old_age   Always       -       0
199 UDMA_CRC_Error_Count    0x0032   200   200   000    Old_age   Always       -       0
`

// TestNeedsSATFallback_PinsTriggerStrings is a focused unit test on
// the helper that gates both retry loops. Pinning each trigger
// string here means a future contributor can't accidentally drop one
// of them while refactoring without a test failure pointing at
// exactly which case regressed.
func TestNeedsSATFallback_PinsTriggerStrings(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		// --- triggers ---
		{"empty output", "", true},
		{"USB bridge classic", "/dev/sda: Unknown USB bridge [0x0951:0x1666]", true},
		{"please specify device type", "Please specify device type with the -d option.", true},
		{"INQUIRY failed", "scsiModePageOffset: response length too short, resp_len=0 offset=4 bd_len=0\nINQUIRY failed", true},
		{"unable to detect", "smartctl was unable to detect device type, please specify with -d", true},
		{"DS918+ SCSI auto-detect (issue #298)", scsiAutoDetectOutput, true},
		{"lacks SMART capability bare phrase", "...\nSMART support is:     Unavailable - device lacks SMART capability.\n", true},
		// smartctl 7.x JSON equivalent — the bundled smartctl
		// in the nas-doctor Docker image is 7.4, which emits this
		// key instead of the text-mode phrase. Captured verbatim
		// from the issue-#298 reporter's container.
		{"smartctl 7.x JSON (production container)", `{"json_format_version":[1,0],"smartctl":{"version":[7,4]},"device":{"type":"scsi"},"smart_support":{"available":false}}`, true},

		// --- non-triggers ---
		// "Unavailable" on its own (without the trailing
		// "device lacks SMART capability") is intentionally NOT a
		// trigger — smartctl's whitespace varies between versions
		// and "Unavailable" alone is too generic for a substring
		// match. The DS918+ case always emits the trailing phrase
		// on the same line, which is what we key on.
		{"\"Unavailable\" alone is not enough", "SMART support is:     Unavailable - cause unknown\n", false},
		{"healthy ATA drive", "Device Model:     ST20000NM002H-3KV133\nSMART support is: Available - device has SMART capability.\nSMART support is: Enabled", false},
		// smart_support:{"available":true} must NOT trigger —
		// healthy NVMes and SATA drives always emit this and the
		// retry would be a wasted call.
		{"smart_support available:true (healthy)", `{"smart_support":{"available":true,"enabled":true},"model_name":"WD Blue SN5000 2TB"}`, false},
		{"healthy NVMe", `{"json_format_version":[1,0,0],"model_name":"Samsung 980 Pro","smart_status":{"passed":true}}`, false},
		{"standby skip (handled separately)", "Device is in STANDBY mode, exit(2)", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := needsSATFallback(tc.out)
			if got != tc.want {
				t.Errorf("needsSATFallback(%q) = %v, want %v", abbrev(tc.out), got, tc.want)
			}
		})
	}
}

// TestReadSMARTDevice_TextModeSATRetry_RescuesDS918Drive is the
// primary issue-#298 regression guard. Simulates the smartctl 6.5
// flow on the reporter's DS918+:
//
//  1. `smartctl --json=c -a /dev/sdc` — returns the "UNRECOGNIZED
//     OPTION" smartctl-help output (no JSON support pre-7.x).
//  2. `smartctl -a /dev/sdc` — returns the SCSI-mode "lacks SMART
//     capability" output.
//  3. `smartctl -a -d sat /dev/sdc` — returns full ATA SMART data.
//
// Pre-fix: step 2 was the last call. parseSMARTText returned an
// empty SMARTInfo, collectSMART hit the empty-model guard and
// counted the drive as `failed`.
//
// Post-fix: step 3 fires from the new text-mode SAT-retry loop,
// returns full data, parseSMARTText extracts Model + Serial +
// PowerOnHours, and the drive shows up as a normal active read.
func TestReadSMARTDevice_TextModeSATRetry_RescuesDS918Drive(t *testing.T) {
	var calls [][]string
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "--json=c"):
			// smartctl 6.x doesn't know --json; emit the help banner
			// it actually emits in that case so the JSON path takes
			// the empty-output branch (which itself trips
			// needsSATFallback). The retry loop will keep trying
			// JSON variants and getting the same response.
			return "Copyright (C) 2002-16, Bruce Allen, Christian Franke, www.smartmontools.org\n\n=======> UNRECOGNIZED OPTION: json=c\n\nUse smartctl -h to get a usage summary\n", nil
		case strings.Contains(argv, "-d sat"):
			return satRetryTextOutput, nil
		case strings.Contains(argv, "-d auto"), strings.Contains(argv, "-d scsi"):
			// Mirror real-world: `-d auto` on this drive falls back
			// to SCSI mode the same way the bare invocation does.
			// The retry loop tries `sat` first so this branch is
			// only here to model the full smartctl behaviour and
			// prove we don't accidentally rely on `auto` succeeding.
			return scsiAutoDetectOutput, nil
		default:
			// Bare `smartctl -a /dev/sdc` — the failing call.
			return scsiAutoDetectOutput, nil
		}
	})()

	info, err := readSMARTDevice("/dev/sdc", false /* wakeDrives */)
	if err != nil {
		t.Fatalf("expected successful read after SAT retry, got error: %v", err)
	}
	if errors.Is(err, errDriveInStandby) {
		t.Fatalf("got standby sentinel; the test fixture has no STANDBY hint")
	}
	if info.Model == "" {
		t.Errorf("info.Model is empty; SAT retry didn't run or didn't parse. Calls:\n  %s", strings.Join(joinedArgs(calls), "\n  "))
	}
	if info.Serial == "" {
		t.Errorf("info.Serial is empty; SAT retry parsed Model but not Serial")
	}
	// Sanity-check a few fields parseSMARTText fills in from
	// satRetryTextOutput so a future refactor that drops the
	// attribute table parsing gets caught here too.
	if info.PowerOnHours != 59942 {
		t.Errorf("info.PowerOnHours = %d, want 59942 (from attribute 9)", info.PowerOnHours)
	}
	if info.Temperature != 34 {
		t.Errorf("info.Temperature = %d, want 34 (from attribute 194)", info.Temperature)
	}

	// The retry loop must have actually issued a `-d sat` call.
	// Without this the test could pass for the wrong reasons (e.g.
	// a future change makes parseSMARTText robust to SCSI-mode
	// output, masking the SAT-fallback regression).
	sawSATCall := false
	for _, c := range calls {
		joined := strings.Join(c, " ")
		// Look specifically for the text-mode SAT call (no --json=c).
		if strings.Contains(joined, "-d sat") && !strings.Contains(joined, "--json=c") {
			sawSATCall = true
			break
		}
	}
	if !sawSATCall {
		t.Errorf("readSMARTDevice never issued a text-mode `-d sat` call; got:\n  %s", strings.Join(joinedArgs(calls), "\n  "))
	}
}

// TestReadSMARTDevice_JSONModeSATRetry_DSMSCSIWrappedInJSON covers the
// smartctl 7.x flavour of the same bug. The bundled smartctl in the
// nas-doctor Docker image is 7.4, so this is the path that fires in
// production on the issue-#298 reporter's deployment (the host
// smartctl is 6.5 but is irrelevant — the container does the
// querying). For this transport, `--json=c` always returns a JSON
// envelope including json_format_version, even when SMART is
// unavailable. The DS918+ SCSI auto-detect case shows up as
// "smart_support":{"available":false}" at the top level (NOT in the
// messages array — that's the smartctl 6.x text-mode mechanism).
//
// JSON envelope captured verbatim from the reporter's running
// container's `smartctl --json=c -a /dev/sdc` (truncated to the
// fields parseSMARTJSON / needsSATFallback actually consume).
//
// Pre-fix: line 265's `if strings.Contains(out, "json_format_version")`
// fired first, parseSMARTJSON parsed the envelope but model_name was
// absent (the SCSI envelope uses scsi_model_name) so info.Model
// stayed empty. info.Serial picked up the top-level serial_number
// field. collectSMART's empty-model guard requires BOTH to be empty
// to skip, so the drive was kept in results with mostly-empty
// fields — exactly what the reporter's UI shows ("NO DATA" badge
// next to a drive that does have a serial in the API).
//
// Post-fix: needsSATFallback matches the
// "smart_support":{"available":false}" substring, the JSON early-return
// is bypassed, and the JSON retry loop's `-d sat` call returns full
// ATA SMART data with a populated model_name.
func TestReadSMARTDevice_JSONModeSATRetry_DSMSCSIWrappedInJSON(t *testing.T) {
	// Compact, single-line JSON. smartctl is invoked with --json=c
	// (the "c" is for compact) so production output never has
	// whitespace inside `{"available":false}` — the needsSATFallback
	// substring match relies on that. The fixture mirrors the real
	// shape rather than being pretty-printed for human readers.
	scsiJSONEnvelope := `{"json_format_version":[1,0],"smartctl":{"version":[7,4],"argv":["smartctl","--json=c","-a","/dev/sdc"],"exit_status":4},"device":{"name":"/dev/sdc","info_name":"/dev/sdc","type":"scsi","protocol":"SCSI"},"scsi_vendor":"WDC","scsi_product":"WD30EFRX-68EUZN0","scsi_model_name":"WDC WD30EFRX-68EUZN0","serial_number":"WD-WCC4N3FV0HNV","smart_support":{"available":false}}`

	// Compact JSON for the same reason as scsiJSONEnvelope. This
	// fixture is what `smartctl --json=c -a -d sat /dev/sdc` returns
	// in the same container — full ATA SMART data with the proper
	// model_name / serial_number / power_on_time fields populated.
	satJSONResponse := `{"json_format_version":[1,0],"smartctl":{"version":[7,4],"exit_status":0},"model_name":"WDC WD30EFRX-68EUZN0","serial_number":"WD-WCC4N3FV0HNV","firmware_version":"82.00A82","user_capacity":{"bytes":3000592982016},"smart_status":{"passed":true},"temperature":{"current":34},"power_on_time":{"hours":59942},"rotation_rate":5400,"ata_smart_attributes":{"table":[{"id":5,"name":"Reallocated_Sector_Ct","raw":{"value":0}},{"id":194,"name":"Temperature_Celsius","raw":{"value":34}},{"id":197,"name":"Current_Pending_Sector","raw":{"value":0}}]}}`

	var calls [][]string
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "--json=c") && strings.Contains(argv, "-d sat"):
			return satJSONResponse, nil
		case strings.Contains(argv, "--json=c"):
			// Both the initial probe AND `-d auto` / `-d scsi`
			// retries return the SCSI-wrapped JSON. Only `-d sat`
			// gets through.
			return scsiJSONEnvelope, nil
		default:
			// Text-mode fallback shouldn't fire on this path; if it
			// does the assertions below catch it.
			return scsiAutoDetectOutput, nil
		}
	})()

	info, err := readSMARTDevice("/dev/sdc", false /* wakeDrives */)
	if err != nil {
		t.Fatalf("expected successful read after JSON SAT retry, got error: %v", err)
	}
	if info.Model != "WDC WD30EFRX-68EUZN0" {
		t.Errorf("info.Model = %q, want %q (JSON SAT retry didn't run or didn't parse)", info.Model, "WDC WD30EFRX-68EUZN0")
	}
	if info.Serial != "WD-WCC4N3FV0HNV" {
		t.Errorf("info.Serial = %q, want %q", info.Serial, "WD-WCC4N3FV0HNV")
	}
	if info.PowerOnHours != 59942 {
		t.Errorf("info.PowerOnHours = %d, want 59942", info.PowerOnHours)
	}

	// The JSON retry loop must have issued a `--json=c -d sat` call.
	sawJSONSATCall := false
	for _, c := range calls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "--json=c") && strings.Contains(joined, "-d sat") {
			sawJSONSATCall = true
			break
		}
	}
	if !sawJSONSATCall {
		t.Errorf("readSMARTDevice never issued a `--json=c -d sat` call; got:\n  %s", strings.Join(joinedArgs(calls), "\n  "))
	}
}

// TestReadSMARTDevice_HealthyDriveStillTakesEarlyJSONReturn guards
// against the obvious foot-gun in the fix: now that the early-return
// at line 265 has an additional `&& !needsSATFallback(out)` clause,
// a healthy drive's JSON output must not coincidentally trigger
// needsSATFallback and force an unnecessary retry round-trip. The
// trigger substrings are specific to smartctl diagnostic messages,
// but a drive whose vendor/model legitimately contained one of those
// strings would be a regression.
func TestReadSMARTDevice_HealthyDriveStillTakesEarlyJSONReturn(t *testing.T) {
	var calls [][]string
	healthyJSON := `{"json_format_version":[1,0,0],"model_name":"Samsung 980 Pro","serial_number":"S6BX","user_capacity":{"bytes":2000000000000},"smart_status":{"passed":true},"temperature":{"current":42},"power_on_time":{"hours":1234}}`

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		return healthyJSON, nil
	})()

	info, err := readSMARTDevice("/dev/nvme0n1", false)
	if err != nil {
		t.Fatalf("unexpected error on healthy drive: %v", err)
	}
	if info.Model != "Samsung 980 Pro" {
		t.Errorf("info.Model = %q, want Samsung 980 Pro", info.Model)
	}
	// Exactly one call: the initial --json=c probe. No retry round
	// trips, no text-mode fallback.
	if len(calls) != 1 {
		t.Errorf("expected exactly 1 smartctl call on healthy drive (early-return into parseSMARTJSON), got %d:\n  %s",
			len(calls), strings.Join(joinedArgs(calls), "\n  "))
	}
}

// TestCollectSMART_DS918DriveCountedAsActiveAfterFix is the
// integration-level shape of the fix: feed the same SCSI-auto-detect
// + SAT-retry pattern through collectSMART and assert the drive
// lands in `active`, not `failed`. Mirrors the
// TestCollectSMART_USBBridge_CountedAsUnsupportedNotFailed test from
// #206 but for the issue-#298 path.
func TestCollectSMART_DS918DriveCountedAsActiveAfterFix(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*; cannot run deterministic fake-execCmd test")
	}

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--scan" {
			return "/dev/fake-ds918-bay -d sat # /dev/fake-ds918-bay, SAT\n", nil
		}
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "--json=c"):
			return "Copyright (C) 2002-16\n=======> UNRECOGNIZED OPTION: json=c\n", nil
		case strings.Contains(argv, "-d sat"):
			return satRetryTextOutput, nil
		default:
			return scsiAutoDetectOutput, nil
		}
	})()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	results, _, _ := collectSMART(SMARTConfig{WakeDrives: false}, logger)

	if len(results) != 1 {
		t.Fatalf("expected 1 active drive after SAT retry, got %d. Logs:\n%s", len(results), buf.String())
	}
	if results[0].Model == "" {
		t.Errorf("active drive has empty Model — SAT retry parsed nothing")
	}

	summary := scanSMARTSummary(t, buf.String())
	assertCounter(t, summary, "total", 1)
	assertCounter(t, summary, "active", 1)
	assertCounter(t, summary, "failed", 0)
	assertCounter(t, summary, "unsupported", 0)
	assertCounter(t, summary, "standby", 0)
}

// abbrev keeps test failure messages readable when fixture strings
// run to dozens of lines.
func abbrev(s string) string {
	const max = 80
	s = strings.ReplaceAll(s, "\n", `\n`)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// joinedArgs flattens a slice of [name, ...args] into a slice of
// space-joined strings, suitable for one-per-line failure messages.
func joinedArgs(calls [][]string) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}
