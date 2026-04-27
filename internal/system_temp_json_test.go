package internal

import (
	"encoding/json"
	"strings"
	"testing"
)

// Issue #269 — JSON serialisation contract for the new SystemInfo
// temperature fields. The dashboard header and demo feeder both rely
// on the omitempty semantic: a zero value must not appear in the
// marshalled output, so the dashboard's `if (sys.cpu_temp_c)` guard
// can hide the gauge entirely on platforms without thermal sensors.

func TestSystemInfo_TempFields_OmittedWhenZero(t *testing.T) {
	s := SystemInfo{Hostname: "test"} // CPUTempC and MoboTempC default to 0
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "cpu_temp_c") {
		t.Errorf("expected cpu_temp_c to be omitted when zero, got: %s", out)
	}
	if strings.Contains(out, "mobo_temp_c") {
		t.Errorf("expected mobo_temp_c to be omitted when zero, got: %s", out)
	}
}

func TestSystemInfo_TempFields_PresentWhenNonZero(t *testing.T) {
	s := SystemInfo{CPUTempC: 58, MoboTempC: 42}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"cpu_temp_c":58`) {
		t.Errorf("expected cpu_temp_c=58 in JSON, got: %s", out)
	}
	if !strings.Contains(out, `"mobo_temp_c":42`) {
		t.Errorf("expected mobo_temp_c=42 in JSON, got: %s", out)
	}
}

// Round-trip check: a marshal → unmarshal cycle must preserve the
// integer values (no float rounding, no string conversion). Catches
// a future "let's switch to float64 for finer resolution" refactor
// that would silently break the dashboard's classForTemp guard.
func TestSystemInfo_TempFields_RoundTrip(t *testing.T) {
	in := SystemInfo{CPUTempC: 73, MoboTempC: 35}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out SystemInfo
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.CPUTempC != 73 || out.MoboTempC != 35 {
		t.Errorf("round-trip lost values: in=%+v out=%+v", in, out)
	}
}
