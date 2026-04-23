package collector

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestCollectSMART_EmitsSummaryLogFormat is a grep-based cross-reference
// that pins the summary log line's field names. v0.9.5 (#203) introduced
// total/active/standby/failed/duration; v0.9.7 (#206) added `unsupported`
// to separate SMART-incapable devices (USB bridges, boot flashes) from
// real collection failures. Operators' log pipelines depend on this
// format not drifting — if someone renames or drops a field, this test
// fails loudly.
func TestCollectSMART_EmitsSummaryLogFormat(t *testing.T) {
	data, err := os.ReadFile("smart.go")
	if err != nil {
		t.Fatalf("read smart.go: %v", err)
	}
	src := string(data)

	if !strings.Contains(src, `"SMART collection complete"`) {
		t.Errorf("smart.go missing summary log message %q — if you renamed it, update the log pipeline docs too", "SMART collection complete")
	}
	for _, field := range []string{`"total"`, `"active"`, `"standby"`, `"unsupported"`, `"failed"`, `"duration"`} {
		if !strings.Contains(src, field) {
			t.Errorf("smart.go summary log missing required field %s", field)
		}
	}
}

// TestCollectSMART_SummaryLogCounters exercises collectSMART against a fake
// execCmd that returns a mix of active + standby + failed drives, and
// asserts the resulting INFO summary has the expected counter math.
//
// We rely on the smartctl --scan fallback to inject a controlled device
// list: discoverDrives() on a dev box either returns real /dev/sd* paths
// or nothing, but when it returns nothing collectSMART falls back to
// `smartctl --scan`, which our fake execCmd fully controls.
func TestCollectSMART_SummaryLogCounters(t *testing.T) {
	// If discoverDrives() finds real drives on the test host, we can't
	// deterministically control the counter math. Detect and skip.
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*; cannot run deterministic fake-execCmd test")
	}

	// Craft smartctl responses per device. --scan enumerates 3 fake devs.
	// /dev/fake0 → active (full JSON read)
	// /dev/fake1 → standby
	// /dev/fake2 → empty output across all fallbacks → counted as failed
	fakeActiveJSON := `{"json_format_version":[1,0,0],"model_name":"FakeDrive 1TB","serial_number":"SN-ACTIVE","user_capacity":{"bytes":1000000000000},"temperature":{"current":30},"power_on_time":{"hours":100}}`
	fakeStandbyOut := "smartctl 7.3 2022-02-28\n\nDevice is in STANDBY mode, exit(2)\n"

	defer swapExecCmd(func(name string, args ...string) (string, error) {
		// --scan enumerates devices
		if len(args) == 1 && args[0] == "--scan" {
			return "/dev/fake0 -d sat # /dev/fake0, SAT\n" +
				"/dev/fake1 -d sat # /dev/fake1, SAT\n" +
				"/dev/fake2 -d sat # /dev/fake2, SAT\n", nil
		}
		// Route per-device smartctl calls based on which /dev/fakeN
		// appears in the argv (it's always the trailing positional arg
		// or penultimate for `-d TYPE DEV` forms).
		argv := strings.Join(args, " ")
		switch {
		case strings.Contains(argv, "/dev/fake0"):
			return fakeActiveJSON, nil
		case strings.Contains(argv, "/dev/fake1"):
			return fakeStandbyOut, nil
		case strings.Contains(argv, "/dev/fake2"):
			return "", nil // no output → falls through → counted as failed
		}
		return "", nil
	})()

	// Capture log output as structured JSON so we can parse the summary
	// line's fields.
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	_, _ = collectSMART(SMARTConfig{WakeDrives: false}, logger)

	// Scan log lines for the summary.
	var summary map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if msg, _ := rec["msg"].(string); msg == "SMART collection complete" {
			summary = rec
			break
		}
	}
	if summary == nil {
		t.Fatalf("no 'SMART collection complete' log line emitted; got:\n%s", buf.String())
	}

	// JSON numbers decode as float64.
	gotInt := func(field string) int {
		v, ok := summary[field]
		if !ok {
			t.Fatalf("summary log missing %q field; got: %v", field, summary)
		}
		f, ok := v.(float64)
		if !ok {
			t.Fatalf("summary field %q is not a number: %v (%T)", field, v, v)
		}
		return int(f)
	}

	if got := gotInt("total"); got != 3 {
		t.Errorf("total = %d, want 3", got)
	}
	if got := gotInt("active"); got != 1 {
		t.Errorf("active = %d, want 1", got)
	}
	if got := gotInt("standby"); got != 1 {
		t.Errorf("standby = %d, want 1", got)
	}
	if got := gotInt("failed"); got != 1 {
		t.Errorf("failed = %d, want 1", got)
	}
	if _, ok := summary["duration"].(string); !ok {
		t.Errorf("duration field missing or not a string: %v", summary["duration"])
	}
}

// TestCollectSMART_SummaryLogEmittedOnNoDrives verifies the no-drive edge
// case still emits a summary (with all-zero counters) before the error
// return. Operators should always see one summary per cycle.
func TestCollectSMART_SummaryLogEmittedOnNoDrives(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*")
	}
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		// --scan returns nothing → collectSMART returns the "no drives
		// discovered" error, but must log first.
		return "", nil
	})()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	_, err := collectSMART(SMARTConfig{}, logger)
	if err == nil {
		t.Fatal("expected 'no drives discovered' error")
	}
	if !strings.Contains(buf.String(), `"SMART collection complete"`) {
		t.Errorf("expected summary log even on no-drive edge case; got:\n%s", buf.String())
	}
}

// TestCollectSMART_NilLoggerTolerated guards the nil-logger defensive
// check on the summary-emit path.
func TestCollectSMART_NilLoggerTolerated(t *testing.T) {
	if len(discoverDrives()) > 0 {
		t.Skip("host has real drives discoverable via /dev/sd*")
	}
	defer swapExecCmd(func(name string, args ...string) (string, error) {
		return "", nil
	})()

	// Should not panic despite logger=nil.
	_, _ = collectSMART(SMARTConfig{}, nil)
}
