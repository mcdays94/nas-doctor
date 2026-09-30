package collector

import "testing"

// TestDecodeCommandTimeout covers the Seagate packed-encoding decode for SMART
// attribute 188 (issue #330).
func TestDecodeCommandTimeout(t *testing.T) {
	cases := []struct {
		name string
		raw  int64
		want int64
	}{
		{"reporter's value: 0x0001_0001_0001 = one timeout", 4295032833, 1},
		{"plain small count passes through", 42, 42},
		{"zero passes through", 0, 0},
		{"16-bit boundary passes through", 0xFFFF, 0xFFFF},
		{"count in the low word", 0x0000_0000_0005, 5},
		{"count in the high word is not lost", 0x0007_0000_0000, 7},
		{"largest sub-counter wins", 0x0002_0009_0003, 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decodeCommandTimeout(c.raw); got != c.want {
				t.Errorf("decodeCommandTimeout(%d) = %d; want %d", c.raw, got, c.want)
			}
		})
	}
}

// TestParseSMARTJSON_SeagateCommandTimeout is the end-to-end guard: a Seagate
// Exos reporting the packed raw value must surface 1 command timeout, not
// 4,295,032,833 — which the analyzer would otherwise escalate to a false
// "Severe — drive or controller failing" CRITICAL (issue #330).
func TestParseSMARTJSON_SeagateCommandTimeout(t *testing.T) {
	const jsonOut = `{
		"smartctl": {"exit_status": 0, "messages": []},
		"model_name": "ST20000NM007D-3DJ103",
		"serial_number": "ZVT0ABCD",
		"smart_status": {"passed": true},
		"temperature": {"current": 34},
		"power_on_time": {"hours": 12000},
		"ata_smart_attributes": {"table": [
			{"id": 188, "name": "Command_Timeout", "value": 100, "worst": 100, "raw": {"value": 4295032833, "string": "1 1 1"}}
		]}
	}`
	info, err := parseSMARTJSON("/dev/sdc", jsonOut)
	if err != nil {
		t.Fatalf("parseSMARTJSON: %v", err)
	}
	if info.CommandTimeout != 1 {
		t.Errorf("CommandTimeout = %d; want 1 (packed Seagate value must be decoded, not surfaced raw)", info.CommandTimeout)
	}
}

// TestParseSMARTText_SeagateCommandTimeout covers the same decode on the
// text-parser fallback path (smartctl < 7 / JSON-retry failure).
func TestParseSMARTText_SeagateCommandTimeout(t *testing.T) {
	// ID# ATTRIBUTE_NAME FLAG VALUE WORST THRESH TYPE UPDATED WHEN_FAILED RAW
	line := "188 Command_Timeout 0x0032 100 100 000 Old_age Always - 4295032833"
	info := parseSMARTText("/dev/sdc", line)
	if info.CommandTimeout != 1 {
		t.Errorf("CommandTimeout = %d; want 1", info.CommandTimeout)
	}
}
