package collector

import (
	"errors"
	"testing"
)

// ── NUT (upsc) parser ──

const sampleNUT = `battery.charge: 100
battery.charge.low: 10
battery.charge.warning: 20
battery.runtime: 2700
battery.voltage: 13.8
battery.voltage.nominal: 12.0
device.model: Back-UPS ES 700G
device.type: ups
input.sensitivity: medium
input.transfer.high: 139
input.transfer.low: 88
input.transfer.reason: input voltage out of range
input.voltage: 121.0
output.voltage: 121.0
ups.load: 32
ups.model: Back-UPS ES 700G
ups.realpower.nominal: 450
ups.status: OL
ups.temperature: 28.5
`

func TestParseNUT(t *testing.T) {
	info := parseNUT("myups", sampleNUT)
	if !info.Available {
		t.Fatal("expected Available=true")
	}
	if info.Source != "nut" {
		t.Errorf("source: got %q, want nut", info.Source)
	}
	if info.Name != "myups" {
		t.Errorf("name: got %q, want myups", info.Name)
	}
	if info.Model != "Back-UPS ES 700G" {
		t.Errorf("model: got %q", info.Model)
	}
	if info.Status != "OL" {
		t.Errorf("status: got %q, want OL", info.Status)
	}
	if info.StatusHuman != "Online" {
		t.Errorf("status human: got %q, want Online", info.StatusHuman)
	}
	if info.OnBattery {
		t.Error("should not be on battery")
	}
	if info.BatteryPct != 100 {
		t.Errorf("battery: got %.0f, want 100", info.BatteryPct)
	}
	if info.RuntimeMins != 45 { // 2700s / 60
		t.Errorf("runtime: got %.0f, want 45", info.RuntimeMins)
	}
	if info.LoadPct != 32 {
		t.Errorf("load: got %.0f, want 32", info.LoadPct)
	}
	if info.InputV != 121 {
		t.Errorf("input voltage: got %.0f, want 121", info.InputV)
	}
	if info.NominalW != 450 {
		t.Errorf("nominal watts: got %.0f, want 450", info.NominalW)
	}
	if info.WattageW < 143 || info.WattageW > 145 {
		t.Errorf("wattage: got %.0f, want ~144", info.WattageW)
	}
	if info.Temperature != 28.5 {
		t.Errorf("temp: got %.1f, want 28.5", info.Temperature)
	}
	if info.LastTransfer != "input voltage out of range" {
		t.Errorf("last transfer: got %q", info.LastTransfer)
	}
}

func TestParseNUTOnBattery(t *testing.T) {
	out := `ups.status: OB DISCHRG
battery.charge: 72
battery.runtime: 900
ups.load: 45
ups.model: CyberPower CP1500
`
	info := parseNUT("ups1", out)
	if !info.OnBattery {
		t.Error("expected OnBattery=true")
	}
	if info.StatusHuman != "On Battery, Discharging" {
		t.Errorf("status human: got %q", info.StatusHuman)
	}
	if info.BatteryPct != 72 {
		t.Errorf("battery: got %.0f, want 72", info.BatteryPct)
	}
}

func TestParseNUTLowBattery(t *testing.T) {
	out := `ups.status: OB LB
battery.charge: 8
battery.runtime: 120
`
	info := parseNUT("ups1", out)
	if !info.LowBattery {
		t.Error("expected LowBattery=true")
	}
	if !info.OnBattery {
		t.Error("expected OnBattery=true")
	}
}

// ── apcupsd (apcaccess) parser ──

const sampleApcaccess = `APC      : 001,034,0839
DATE     : 2026-04-06 12:00:00 +0100
HOSTNAME : tower
VERSION  : 3.14.14
UPSNAME  : ServerUPS
CABLE    : USB Cable
DRIVER   : USB UPS Driver
UPSMODE  : Stand Alone
STARTTIME: 2026-03-01 10:00:00 +0100
MODEL    : Back-UPS XS 1400U
STATUS   : ONLINE
LINEV    : 122.0 Volts
LOADPCT  : 28.5 Percent
BCHARGE  : 100.0 Percent
TIMELEFT : 52.3 Minutes
MBATTCHG : 5 Percent
MINTIMEL : 3 Minutes
MAXTIME  : 0 Seconds
OUTPUTV  : 122.0 Volts
BATTV    : 27.2 Volts
NOMPOWER : 865 Watts
ITEMP    : 31.2 C
LASTXFER : Low line voltage
`

func TestParseApcaccess(t *testing.T) {
	info := parseApcaccess(sampleApcaccess)
	if !info.Available {
		t.Fatal("expected Available=true")
	}
	if info.Source != "apcupsd" {
		t.Errorf("source: got %q, want apcupsd", info.Source)
	}
	if info.Name != "ServerUPS" {
		t.Errorf("name: got %q, want ServerUPS", info.Name)
	}
	if info.Model != "Back-UPS XS 1400U" {
		t.Errorf("model: got %q", info.Model)
	}
	if info.Status != "ONLINE" {
		t.Errorf("status: got %q, want ONLINE", info.Status)
	}
	if info.StatusHuman != "Online" {
		t.Errorf("status human: got %q, want Online", info.StatusHuman)
	}
	if info.OnBattery {
		t.Error("should not be on battery")
	}
	if info.BatteryPct != 100 {
		t.Errorf("battery: got %.0f, want 100", info.BatteryPct)
	}
	if info.RuntimeMins != 52.3 {
		t.Errorf("runtime: got %.1f, want 52.3", info.RuntimeMins)
	}
	if info.LoadPct != 28.5 {
		t.Errorf("load: got %.1f, want 28.5", info.LoadPct)
	}
	if info.InputV != 122 {
		t.Errorf("input: got %.0f, want 122", info.InputV)
	}
	if info.NominalW != 865 {
		t.Errorf("nominal: got %.0f, want 865", info.NominalW)
	}
	if info.WattageW < 246 || info.WattageW > 247 {
		t.Errorf("wattage: got %.0f, want ~246", info.WattageW)
	}
	if info.Temperature != 31.2 {
		t.Errorf("temp: got %.1f, want 31.2", info.Temperature)
	}
	if info.LastTransfer != "Low line voltage" {
		t.Errorf("last transfer: got %q", info.LastTransfer)
	}
}

func TestParseApcaccessOnBatt(t *testing.T) {
	out := `STATUS   : ONBATT
BCHARGE  : 64.0 Percent
TIMELEFT : 18.2 Minutes
LOADPCT  : 55.0 Percent
MODEL    : Smart-UPS 1500
`
	info := parseApcaccess(out)
	if !info.OnBattery {
		t.Error("expected OnBattery=true")
	}
	if info.StatusHuman != "On Battery" {
		t.Errorf("status human: got %q", info.StatusHuman)
	}
}

// ── Detection: binary missing / daemon unreachable ──

// fakeRunner is an injectable UPSRunner for tests — no real exec/LookPath required.
type fakeRunner struct {
	lookPath func(string) (string, error)
	output   func(name string, args ...string) ([]byte, error)
}

func (f fakeRunner) LookPath(name string) (string, error) { return f.lookPath(name) }
func (f fakeRunner) Output(name string, args ...string) ([]byte, error) {
	return f.output(name, args...)
}

func TestCollectApcupsd_BinaryMissing_ReturnsNil(t *testing.T) {
	r := fakeRunner{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		output: func(string, ...string) ([]byte, error) {
			t.Fatal("output should not be called when binary missing")
			return nil, nil
		},
	}
	info, err := collectApcupsdWith(r)
	if err == nil {
		t.Error("expected error when apcaccess missing")
	}
	if info != nil {
		t.Errorf("expected nil UPSInfo, got %+v", info)
	}
}

func TestCollectNUT_BinaryMissing_ReturnsNil(t *testing.T) {
	r := fakeRunner{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		output: func(string, ...string) ([]byte, error) {
			t.Fatal("output should not be called when binary missing")
			return nil, nil
		},
	}
	info, err := collectNUTWith(r)
	if err == nil {
		t.Error("expected error when upsc missing")
	}
	if info != nil {
		t.Errorf("expected nil UPSInfo, got %+v", info)
	}
}

func TestCollectApcupsd_BinaryPresent_DaemonUnreachable_ReturnsHint(t *testing.T) {
	// Simulate the Unraid failure mode: image has apcaccess, but the host daemon
	// is unreachable on 127.0.0.1:3551 (e.g. apcupsd plugin not running or
	// container not using host networking).
	r := fakeRunner{
		lookPath: func(name string) (string, error) { return "/usr/sbin/" + name, nil },
		output: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("Error contacting apcupsd @ 127.0.0.1:3551: Connection refused")
		},
	}
	info, err := collectApcupsdWith(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected UPSInfo with diagnostic hint, got nil")
	}
	if !info.Available {
		t.Error("expected Available=true so users see the diagnostic in the dashboard")
	}
	if info.Source != "apcupsd" {
		t.Errorf("source: got %q, want apcupsd", info.Source)
	}
	if info.Status != "unreachable" {
		t.Errorf("status: got %q, want unreachable", info.Status)
	}
	if info.StatusHuman == "" {
		t.Error("expected a human-readable hint explaining the unreachable daemon")
	}
}

func TestCollectNUT_BinaryPresent_DaemonUnreachable_ReturnsHint(t *testing.T) {
	r := fakeRunner{
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		output: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("Error: Connection failure: Connection refused")
		},
	}
	info, err := collectNUTWith(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected UPSInfo with diagnostic hint, got nil")
	}
	if !info.Available {
		t.Error("expected Available=true so users see the diagnostic in the dashboard")
	}
	if info.Source != "nut" {
		t.Errorf("source: got %q, want nut", info.Source)
	}
	if info.Status != "unreachable" {
		t.Errorf("status: got %q, want unreachable", info.Status)
	}
}

// ── NUT status mapping ──

func TestNutStatusToHuman(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"OL", "Online"},
		{"OB", "On Battery"},
		{"OB LB", "On Battery, Low Battery"},
		{"OL CHRG", "Online, Charging"},
		{"OL TRIM", "Online, Trimming"},
		{"OB DISCHRG", "On Battery, Discharging"},
		{"FSD", "Forced Shutdown"},
		{"", "Unknown"},
	}
	for _, tt := range tests {
		got := nutStatusToHuman(tt.input)
		if got != tt.want {
			t.Errorf("nutStatusToHuman(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}
