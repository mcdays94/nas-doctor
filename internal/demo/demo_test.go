package demo

import "testing"

// A drive without DataAvailable is one smartctl couldn't read: the
// dashboard shows "NO DATA" and the analyzer adds a "SMART data
// unavailable" warning. Demo drives always have attributes.
func TestDemoSMART_DrivesHaveData(t *testing.T) {
	drives := demoSMART()
	if len(drives) == 0 {
		t.Fatal("demoSMART returned no drives")
	}
	for _, d := range drives {
		if !d.DataAvailable {
			t.Errorf("%s: DataAvailable = false; it renders as NO DATA", d.Device)
		}
	}
}

// The dashboard header shows CPU and mainboard temperatures when the
// host exposes them (#269); the demo host does.
func TestDemoSystem_HasTemperatures(t *testing.T) {
	sys := demoSystem()
	if sys.CPUTempC <= 0 || sys.MoboTempC <= 0 {
		t.Errorf("CPUTempC = %d, MoboTempC = %d; want both set", sys.CPUTempC, sys.MoboTempC)
	}
}
