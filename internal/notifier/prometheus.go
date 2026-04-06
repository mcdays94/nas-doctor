package notifier

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics exposed by nas-doctor.
type Metrics struct {
	// System metrics
	cpuUsage  prometheus.Gauge
	memUsage  prometheus.Gauge
	memTotal  prometheus.Gauge
	loadAvg1  prometheus.Gauge
	loadAvg5  prometheus.Gauge
	loadAvg15 prometheus.Gauge
	ioWait    prometheus.Gauge
	uptime    prometheus.Gauge

	// Disk metrics
	diskUsedBytes  *prometheus.GaugeVec
	diskTotalBytes *prometheus.GaugeVec
	diskUsedPct    *prometheus.GaugeVec

	// SMART metrics
	smartHealthy      *prometheus.GaugeVec
	smartTemp         *prometheus.GaugeVec
	smartReallocated  *prometheus.GaugeVec
	smartPending      *prometheus.GaugeVec
	smartUDMACRC      *prometheus.GaugeVec
	smartPowerOnHours *prometheus.GaugeVec

	// Docker metrics
	containerCPU *prometheus.GaugeVec
	containerMem *prometheus.GaugeVec

	// Finding metrics
	findingsTotal    *prometheus.GaugeVec
	findingsCritical prometheus.Gauge
	findingsWarning  prometheus.Gauge

	// Parity metrics (Unraid)
	paritySpeedMBs    prometheus.Gauge
	parityDurationSec prometheus.Gauge

	// Collection metrics
	collectionDuration prometheus.Gauge
	lastCollectionTime prometheus.Gauge

	mu       sync.Mutex
	registry *prometheus.Registry
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
	}

	m.cpuUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "cpu_usage_percent",
		Help: "Current CPU usage percentage",
	})
	m.memUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "memory_used_bytes",
		Help: "Used memory in bytes",
	})
	m.memTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "memory_total_bytes",
		Help: "Total memory in bytes",
	})
	m.loadAvg1 = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "load_avg_1",
		Help: "1-minute load average",
	})
	m.loadAvg5 = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "load_avg_5",
		Help: "5-minute load average",
	})
	m.loadAvg15 = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "load_avg_15",
		Help: "15-minute load average",
	})
	m.ioWait = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "io_wait_percent",
		Help: "CPU I/O wait percentage",
	})
	m.uptime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "system", Name: "uptime_seconds",
		Help: "System uptime in seconds",
	})

	m.diskUsedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "disk", Name: "used_bytes",
		Help: "Used disk space in bytes",
	}, []string{"device", "mountpoint", "label"})
	m.diskTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "disk", Name: "total_bytes",
		Help: "Total disk space in bytes",
	}, []string{"device", "mountpoint", "label"})
	m.diskUsedPct = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "disk", Name: "used_percent",
		Help: "Disk usage percentage",
	}, []string{"device", "mountpoint", "label"})

	m.smartHealthy = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "smart", Name: "healthy",
		Help: "SMART health status (1=passed, 0=failed)",
	}, []string{"device", "model", "serial"})
	m.smartTemp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "smart", Name: "temperature_celsius",
		Help: "Drive temperature in Celsius",
	}, []string{"device", "model", "serial"})
	m.smartReallocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "smart", Name: "reallocated_sectors",
		Help: "SMART reallocated sector count",
	}, []string{"device", "model", "serial"})
	m.smartPending = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "smart", Name: "pending_sectors",
		Help: "SMART pending sector count",
	}, []string{"device", "model", "serial"})
	m.smartUDMACRC = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "smart", Name: "udma_crc_errors",
		Help: "UDMA CRC error count",
	}, []string{"device", "model", "serial"})
	m.smartPowerOnHours = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "smart", Name: "power_on_hours",
		Help: "Drive power-on hours",
	}, []string{"device", "model", "serial"})

	m.containerCPU = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "docker", Name: "container_cpu_percent",
		Help: "Container CPU usage percentage",
	}, []string{"name", "image"})
	m.containerMem = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "docker", Name: "container_memory_bytes",
		Help: "Container memory usage in bytes",
	}, []string{"name", "image"})

	m.findingsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "findings", Name: "total",
		Help: "Total number of findings by severity",
	}, []string{"severity"})
	m.findingsCritical = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "findings", Name: "critical_count",
		Help: "Number of critical findings",
	})
	m.findingsWarning = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "findings", Name: "warning_count",
		Help: "Number of warning findings",
	})

	m.paritySpeedMBs = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "parity", Name: "speed_mb_per_sec",
		Help: "Latest parity check speed in MB/s",
	})
	m.parityDurationSec = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Subsystem: "parity", Name: "duration_seconds",
		Help: "Latest parity check duration in seconds",
	})

	m.collectionDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Name: "collection_duration_seconds",
		Help: "Time taken for the last diagnostic collection",
	})
	m.lastCollectionTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nasdoctor", Name: "last_collection_timestamp",
		Help: "Unix timestamp of the last collection",
	})

	// Register all
	collectors := []prometheus.Collector{
		m.cpuUsage, m.memUsage, m.memTotal,
		m.loadAvg1, m.loadAvg5, m.loadAvg15,
		m.ioWait, m.uptime,
		m.diskUsedBytes, m.diskTotalBytes, m.diskUsedPct,
		m.smartHealthy, m.smartTemp, m.smartReallocated,
		m.smartPending, m.smartUDMACRC, m.smartPowerOnHours,
		m.containerCPU, m.containerMem,
		m.findingsTotal, m.findingsCritical, m.findingsWarning,
		m.paritySpeedMBs, m.parityDurationSec,
		m.collectionDuration, m.lastCollectionTime,
	}
	for _, c := range collectors {
		m.registry.MustRegister(c)
	}

	return m
}

// Registry returns the prometheus registry for the HTTP handler.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// Update refreshes all Prometheus metrics from a snapshot.
func (m *Metrics) Update(snap *internal.Snapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// System
	m.cpuUsage.Set(snap.System.CPUUsage)
	m.memUsage.Set(float64(snap.System.MemUsedMB) * 1024 * 1024)
	m.memTotal.Set(float64(snap.System.MemTotalMB) * 1024 * 1024)
	m.loadAvg1.Set(snap.System.LoadAvg1)
	m.loadAvg5.Set(snap.System.LoadAvg5)
	m.loadAvg15.Set(snap.System.LoadAvg15)
	m.ioWait.Set(snap.System.IOWait)
	m.uptime.Set(float64(snap.System.UptimeSecs))

	// Disks
	for _, d := range snap.Disks {
		labels := prometheus.Labels{
			"device":     d.Device,
			"mountpoint": d.MountPoint,
			"label":      d.Label,
		}
		m.diskUsedBytes.With(labels).Set(d.UsedGB * 1024 * 1024 * 1024)
		m.diskTotalBytes.With(labels).Set(d.TotalGB * 1024 * 1024 * 1024)
		m.diskUsedPct.With(labels).Set(d.UsedPct)
	}

	// SMART
	for _, d := range snap.SMART {
		labels := prometheus.Labels{
			"device": d.Device,
			"model":  sanitizeLabel(d.Model),
			"serial": d.Serial,
		}
		healthy := 1.0
		if !d.HealthPassed {
			healthy = 0
		}
		m.smartHealthy.With(labels).Set(healthy)
		m.smartTemp.With(labels).Set(float64(d.Temperature))
		m.smartReallocated.With(labels).Set(float64(d.Reallocated))
		m.smartPending.With(labels).Set(float64(d.Pending))
		m.smartUDMACRC.With(labels).Set(float64(d.UDMACRC))
		m.smartPowerOnHours.With(labels).Set(float64(d.PowerOnHours))
	}

	// Docker
	for _, c := range snap.Docker.Containers {
		if c.State != "running" {
			continue
		}
		labels := prometheus.Labels{
			"name":  c.Name,
			"image": sanitizeLabel(c.Image),
		}
		m.containerCPU.With(labels).Set(c.CPU)
		m.containerMem.With(labels).Set(c.MemMB * 1024 * 1024)
	}

	// Findings
	critical, warnings, infos := countBySeverity(snap.Findings)
	m.findingsCritical.Set(float64(critical))
	m.findingsWarning.Set(float64(warnings))
	m.findingsTotal.With(prometheus.Labels{"severity": "critical"}).Set(float64(critical))
	m.findingsTotal.With(prometheus.Labels{"severity": "warning"}).Set(float64(warnings))
	m.findingsTotal.With(prometheus.Labels{"severity": "info"}).Set(float64(infos))

	// Parity
	if snap.Parity != nil && len(snap.Parity.History) > 0 {
		last := snap.Parity.History[len(snap.Parity.History)-1]
		m.paritySpeedMBs.Set(last.SpeedMBs)
		m.parityDurationSec.Set(float64(last.Duration))
	}

	// Collection meta
	m.collectionDuration.Set(snap.Duration)
	m.lastCollectionTime.Set(float64(snap.Timestamp.Unix()))
}

func sanitizeLabel(s string) string {
	// Prometheus labels can't have certain chars; replace common ones
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "/", "_")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}

// FormatPrometheusTextExposition is a helper to manually format metrics if needed.
// In practice, use promhttp.HandlerFor(m.Registry(), ...) instead.
func FormatPrometheusTextExposition(snap *internal.Snapshot) string {
	var b strings.Builder

	// System
	fmt.Fprintf(&b, "# HELP nasdoctor_system_cpu_usage_percent CPU usage percentage\n")
	fmt.Fprintf(&b, "# TYPE nasdoctor_system_cpu_usage_percent gauge\n")
	fmt.Fprintf(&b, "nasdoctor_system_cpu_usage_percent %.2f\n", snap.System.CPUUsage)

	fmt.Fprintf(&b, "# HELP nasdoctor_system_memory_used_bytes Used memory in bytes\n")
	fmt.Fprintf(&b, "# TYPE nasdoctor_system_memory_used_bytes gauge\n")
	fmt.Fprintf(&b, "nasdoctor_system_memory_used_bytes %d\n", snap.System.MemUsedMB*1024*1024)

	fmt.Fprintf(&b, "# HELP nasdoctor_system_io_wait_percent IO wait percentage\n")
	fmt.Fprintf(&b, "# TYPE nasdoctor_system_io_wait_percent gauge\n")
	fmt.Fprintf(&b, "nasdoctor_system_io_wait_percent %.2f\n", snap.System.IOWait)

	return b.String()
}
