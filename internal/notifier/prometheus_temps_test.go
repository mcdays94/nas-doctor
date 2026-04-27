package notifier

import (
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Issue #269 — CPU + mainboard temperature gauges. These are plain
// gauges (not GaugeVec) under the system subsystem, mirroring the
// pattern used for cpu_usage_percent / io_wait_percent. They MUST be
// registered and updated on Snapshot.System.{CPUTempC, MoboTempC}.

// TestPrometheus_SystemTempGauges_Registered asserts the metric
// names appear in the registry's exposition output, regardless of
// value. Catches a future refactor that drops the gauge from the
// collector list (the v0.9.5-era class of bug).
func TestPrometheus_SystemTempGauges_Registered(t *testing.T) {
	m := NewMetrics()
	out, err := gatherText(m.Registry())
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, name := range []string{
		"nasdoctor_system_cpu_temp_celsius",
		"nasdoctor_system_mobo_temp_celsius",
	} {
		if !strings.Contains(out, name) {
			t.Errorf("metric %s missing from /metrics exposition; expected registration in NewMetrics()", name)
		}
	}
}

// TestPrometheus_SystemTempGauges_UpdatedFromSnapshot pins the
// snapshot.System.{CPUTempC,MoboTempC} → gauge plumbing. Without
// this Update wiring the metric exists but always reads 0.
func TestPrometheus_SystemTempGauges_UpdatedFromSnapshot(t *testing.T) {
	m := NewMetrics()
	snap := &internal.Snapshot{
		System: internal.SystemInfo{
			CPUTempC:  58,
			MoboTempC: 42,
		},
	}
	m.Update(snap)

	if got := gaugeValue(t, m.cpuTempC); got != 58 {
		t.Errorf("nasdoctor_system_cpu_temp_celsius = %v, want 58 after Update", got)
	}
	if got := gaugeValue(t, m.moboTempC); got != 42 {
		t.Errorf("nasdoctor_system_mobo_temp_celsius = %v, want 42 after Update", got)
	}
}

// TestPrometheus_SystemTempGauges_ZeroOnGracefulFallback covers the
// platforms where /sys/class/hwmon doesn't expose a CPU/mobo sensor
// (Synology, K8s pods). The collector returns (0, 0); the gauge must
// reflect that as 0, not omit the metric. Prometheus consumers
// filter with `> 0` to drop platforms without sensors.
func TestPrometheus_SystemTempGauges_ZeroOnGracefulFallback(t *testing.T) {
	m := NewMetrics()
	snap := &internal.Snapshot{
		System: internal.SystemInfo{CPUTempC: 0, MoboTempC: 0},
	}
	m.Update(snap)

	if got := gaugeValue(t, m.cpuTempC); got != 0 {
		t.Errorf("nasdoctor_system_cpu_temp_celsius = %v, want 0 on graceful fallback", got)
	}
	if got := gaugeValue(t, m.moboTempC); got != 0 {
		t.Errorf("nasdoctor_system_mobo_temp_celsius = %v, want 0 on graceful fallback", got)
	}
}

// gaugeValue extracts the current numeric value of a prometheus.Gauge
// without depending on the testutil package (which would pull godebug
// into go.mod). Mirrors testutil.ToFloat64 for a single Gauge.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var dtoMetric dto.Metric
	if err := g.Write(&dtoMetric); err != nil {
		t.Fatalf("gauge Write: %v", err)
	}
	if dtoMetric.Gauge == nil {
		t.Fatal("gauge has no Gauge dto field")
	}
	return dtoMetric.Gauge.GetValue()
}

// gatherText renders a registry's metrics in text-exposition format.
func gatherText(reg *prometheus.Registry) (string, error) {
	mfs, err := reg.Gather()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, mf := range mfs {
		b.WriteString(mf.GetName())
		b.WriteString("\n")
	}
	return b.String(), nil
}
