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
	// ── System ──
	cpuUsage   prometheus.Gauge
	memUsed    prometheus.Gauge
	memTotal   prometheus.Gauge
	memPercent prometheus.Gauge
	swapUsed   prometheus.Gauge
	swapTotal  prometheus.Gauge
	loadAvg1   prometheus.Gauge
	loadAvg5   prometheus.Gauge
	loadAvg15  prometheus.Gauge
	ioWait     prometheus.Gauge
	uptime     prometheus.Gauge
	cpuCores   prometheus.Gauge

	// ── Disk ──
	diskUsedBytes  *prometheus.GaugeVec
	diskTotalBytes *prometheus.GaugeVec
	diskUsedPct    *prometheus.GaugeVec

	// ── SMART ──
	smartHealthy      *prometheus.GaugeVec
	smartTemp         *prometheus.GaugeVec
	smartTempMax      *prometheus.GaugeVec
	smartReallocated  *prometheus.GaugeVec
	smartPending      *prometheus.GaugeVec
	smartOffline      *prometheus.GaugeVec
	smartUDMACRC      *prometheus.GaugeVec
	smartCmdTimeout   *prometheus.GaugeVec
	smartSpinRetry    *prometheus.GaugeVec
	smartPowerOnHours *prometheus.GaugeVec
	smartSizeBytes    *prometheus.GaugeVec

	// ── Docker ──
	containerCPU        *prometheus.GaugeVec
	containerMem        *prometheus.GaugeVec
	containerMemPct     *prometheus.GaugeVec
	containerNetIn      *prometheus.GaugeVec
	containerNetOut     *prometheus.GaugeVec
	containerBlockRead  *prometheus.GaugeVec
	containerBlockWrite *prometheus.GaugeVec
	containerState      *prometheus.GaugeVec
	containerTotal      prometheus.Gauge

	// ── Network ──
	netInterfaceUp  *prometheus.GaugeVec
	netInterfaceMTU *prometheus.GaugeVec

	// ── UPS ──
	upsBatteryPct  prometheus.Gauge
	upsBatteryV    prometheus.Gauge
	upsInputV      prometheus.Gauge
	upsOutputV     prometheus.Gauge
	upsLoadPct     prometheus.Gauge
	upsRuntimeMins prometheus.Gauge
	upsWattage     prometheus.Gauge
	upsTemperature prometheus.Gauge
	upsOnBattery   prometheus.Gauge
	upsLowBattery  prometheus.Gauge

	// ── ZFS ──
	zpoolState        *prometheus.GaugeVec
	zpoolUsedBytes    *prometheus.GaugeVec
	zpoolTotalBytes   *prometheus.GaugeVec
	zpoolUsedPct      *prometheus.GaugeVec
	zpoolFragPct      *prometheus.GaugeVec
	zpoolScanPct      *prometheus.GaugeVec
	zpoolScanErrors   *prometheus.GaugeVec
	zpoolReadErrors   *prometheus.GaugeVec
	zpoolWriteErrors  *prometheus.GaugeVec
	zpoolCksumErrors  *prometheus.GaugeVec
	zfsARCSize        prometheus.Gauge
	zfsARCMaxSize     prometheus.Gauge
	zfsARCHitRate     prometheus.Gauge
	zfsARCHits        prometheus.Gauge
	zfsARCMisses      prometheus.Gauge
	zfsL2Size         prometheus.Gauge
	zfsL2HitRate      prometheus.Gauge
	zdatasetUsed      *prometheus.GaugeVec
	zdatasetAvail     *prometheus.GaugeVec
	zdatasetCompRatio *prometheus.GaugeVec

	// ── Service Checks ──
	serviceUp       *prometheus.GaugeVec
	serviceLatency  *prometheus.GaugeVec
	serviceFailures *prometheus.GaugeVec

	// ── Parity ──
	paritySpeedMBs    prometheus.Gauge
	parityDurationSec prometheus.Gauge
	parityErrors      prometheus.Gauge
	parityRunning     prometheus.Gauge

	// ── Tunnels ──
	cfTunnelUp    *prometheus.GaugeVec
	cfTunnelConns *prometheus.GaugeVec
	tsNodeOnline  *prometheus.GaugeVec
	tsNodeTxBytes *prometheus.GaugeVec
	tsNodeRxBytes *prometheus.GaugeVec

	// ── Proxmox ──
	pveNodeCPU      *prometheus.GaugeVec
	pveNodeMemUsed  *prometheus.GaugeVec
	pveNodeMemTotal *prometheus.GaugeVec
	pveNodeOnline   *prometheus.GaugeVec
	pveGuestCPU     *prometheus.GaugeVec
	pveGuestMemUsed *prometheus.GaugeVec
	pveGuestMemMax  *prometheus.GaugeVec
	pveGuestRunning *prometheus.GaugeVec
	pveStorageUsed  *prometheus.GaugeVec
	pveStorageTotal *prometheus.GaugeVec

	// ── Kubernetes ──
	k8sNodeReady     *prometheus.GaugeVec
	k8sNodePods      *prometheus.GaugeVec
	k8sPodRunning    *prometheus.GaugeVec
	k8sPodRestarts   *prometheus.GaugeVec
	k8sDeployReady   *prometheus.GaugeVec
	k8sDeployDesired *prometheus.GaugeVec

	// ── GPU ──
	gpuUsagePct    *prometheus.GaugeVec
	gpuMemUsedMB   *prometheus.GaugeVec
	gpuMemTotalMB  *prometheus.GaugeVec
	gpuMemPct      *prometheus.GaugeVec
	gpuTemperature *prometheus.GaugeVec
	gpuPowerW      *prometheus.GaugeVec
	gpuPowerMaxW   *prometheus.GaugeVec
	gpuFanPct      *prometheus.GaugeVec
	gpuEncoderPct  *prometheus.GaugeVec
	gpuDecoderPct  *prometheus.GaugeVec

	// ── Findings ──
	findingsTotal    *prometheus.GaugeVec
	findingsCritical prometheus.Gauge
	findingsWarning  prometheus.Gauge

	// ── Collection ──
	collectionDuration prometheus.Gauge
	lastCollectionTime prometheus.Gauge

	// ── Update ──
	updateAvailable prometheus.Gauge

	mu       sync.Mutex
	registry *prometheus.Registry
}

func gauge(ns, sub, name, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Subsystem: sub, Name: name, Help: help})
}
func gaugeVec(ns, sub, name, help string, labels []string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Subsystem: sub, Name: name, Help: help}, labels)
}

const ns = "nasdoctor"

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics() *Metrics {
	m := &Metrics{registry: prometheus.NewRegistry()}

	// ── System ──
	m.cpuUsage = gauge(ns, "system", "cpu_usage_percent", "CPU usage percentage")
	m.memUsed = gauge(ns, "system", "memory_used_bytes", "Used memory in bytes")
	m.memTotal = gauge(ns, "system", "memory_total_bytes", "Total memory in bytes")
	m.memPercent = gauge(ns, "system", "memory_used_percent", "Memory usage percentage")
	m.swapUsed = gauge(ns, "system", "swap_used_bytes", "Used swap in bytes")
	m.swapTotal = gauge(ns, "system", "swap_total_bytes", "Total swap in bytes")
	m.loadAvg1 = gauge(ns, "system", "load_avg_1", "1-minute load average")
	m.loadAvg5 = gauge(ns, "system", "load_avg_5", "5-minute load average")
	m.loadAvg15 = gauge(ns, "system", "load_avg_15", "15-minute load average")
	m.ioWait = gauge(ns, "system", "io_wait_percent", "CPU I/O wait percentage")
	m.uptime = gauge(ns, "system", "uptime_seconds", "System uptime in seconds")
	m.cpuCores = gauge(ns, "system", "cpu_cores", "Number of CPU cores")

	// ── Disk ──
	diskLabels := []string{"device", "mountpoint", "label"}
	m.diskUsedBytes = gaugeVec(ns, "disk", "used_bytes", "Used disk space in bytes", diskLabels)
	m.diskTotalBytes = gaugeVec(ns, "disk", "total_bytes", "Total disk space in bytes", diskLabels)
	m.diskUsedPct = gaugeVec(ns, "disk", "used_percent", "Disk usage percentage", diskLabels)

	// ── SMART ──
	smartLabels := []string{"device", "model", "serial"}
	m.smartHealthy = gaugeVec(ns, "smart", "healthy", "SMART health (1=passed, 0=failed)", smartLabels)
	m.smartTemp = gaugeVec(ns, "smart", "temperature_celsius", "Drive temperature", smartLabels)
	m.smartTempMax = gaugeVec(ns, "smart", "temperature_max_celsius", "Drive max temperature", smartLabels)
	m.smartReallocated = gaugeVec(ns, "smart", "reallocated_sectors", "Reallocated sector count", smartLabels)
	m.smartPending = gaugeVec(ns, "smart", "pending_sectors", "Pending sector count", smartLabels)
	m.smartOffline = gaugeVec(ns, "smart", "offline_uncorrectable", "Offline uncorrectable count", smartLabels)
	m.smartUDMACRC = gaugeVec(ns, "smart", "udma_crc_errors", "UDMA CRC error count", smartLabels)
	m.smartCmdTimeout = gaugeVec(ns, "smart", "command_timeout", "Command timeout count", smartLabels)
	m.smartSpinRetry = gaugeVec(ns, "smart", "spin_retry_count", "Spin retry count", smartLabels)
	m.smartPowerOnHours = gaugeVec(ns, "smart", "power_on_hours", "Drive power-on hours", smartLabels)
	m.smartSizeBytes = gaugeVec(ns, "smart", "size_bytes", "Drive size in bytes", smartLabels)

	// ── Docker ──
	containerLabels := []string{"name", "image"}
	m.containerCPU = gaugeVec(ns, "docker", "container_cpu_percent", "Container CPU usage", containerLabels)
	m.containerMem = gaugeVec(ns, "docker", "container_memory_bytes", "Container memory usage in bytes", containerLabels)
	m.containerMemPct = gaugeVec(ns, "docker", "container_memory_percent", "Container memory usage percent", containerLabels)
	m.containerNetIn = gaugeVec(ns, "docker", "container_net_in_bytes", "Container cumulative network bytes received", containerLabels)
	m.containerNetOut = gaugeVec(ns, "docker", "container_net_out_bytes", "Container cumulative network bytes sent", containerLabels)
	m.containerBlockRead = gaugeVec(ns, "docker", "container_block_read_bytes", "Container cumulative block bytes read", containerLabels)
	m.containerBlockWrite = gaugeVec(ns, "docker", "container_block_write_bytes", "Container cumulative block bytes written", containerLabels)
	m.containerState = gaugeVec(ns, "docker", "container_running", "Container running state (1=running, 0=stopped)", containerLabels)
	m.containerTotal = gauge(ns, "docker", "container_count", "Total container count")

	// ── Network ──
	netLabels := []string{"interface"}
	m.netInterfaceUp = gaugeVec(ns, "network", "interface_up", "Interface up state (1=up, 0=down)", netLabels)
	m.netInterfaceMTU = gaugeVec(ns, "network", "interface_mtu", "Interface MTU", netLabels)

	// ── UPS ──
	m.upsBatteryPct = gauge(ns, "ups", "battery_percent", "UPS battery percentage")
	m.upsBatteryV = gauge(ns, "ups", "battery_voltage", "UPS battery voltage")
	m.upsInputV = gauge(ns, "ups", "input_voltage", "UPS input voltage")
	m.upsOutputV = gauge(ns, "ups", "output_voltage", "UPS output voltage")
	m.upsLoadPct = gauge(ns, "ups", "load_percent", "UPS load percentage")
	m.upsRuntimeMins = gauge(ns, "ups", "runtime_minutes", "UPS estimated runtime in minutes")
	m.upsWattage = gauge(ns, "ups", "wattage_watts", "UPS wattage draw")
	m.upsTemperature = gauge(ns, "ups", "temperature_celsius", "UPS internal temperature")
	m.upsOnBattery = gauge(ns, "ups", "on_battery", "UPS on battery (1=yes, 0=no)")
	m.upsLowBattery = gauge(ns, "ups", "low_battery", "UPS low battery (1=yes, 0=no)")

	// ── ZFS ──
	poolLabels := []string{"pool"}
	m.zpoolState = gaugeVec(ns, "zfs", "pool_healthy", "ZFS pool healthy (1=ONLINE, 0=other)", poolLabels)
	m.zpoolUsedBytes = gaugeVec(ns, "zfs", "pool_used_bytes", "ZFS pool used in bytes", poolLabels)
	m.zpoolTotalBytes = gaugeVec(ns, "zfs", "pool_total_bytes", "ZFS pool total in bytes", poolLabels)
	m.zpoolUsedPct = gaugeVec(ns, "zfs", "pool_used_percent", "ZFS pool usage percentage", poolLabels)
	m.zpoolFragPct = gaugeVec(ns, "zfs", "pool_fragmentation_percent", "ZFS pool fragmentation", poolLabels)
	m.zpoolScanPct = gaugeVec(ns, "zfs", "pool_scan_percent", "ZFS scrub/resilver progress", poolLabels)
	m.zpoolScanErrors = gaugeVec(ns, "zfs", "pool_scan_errors", "ZFS scan error count", poolLabels)
	m.zpoolReadErrors = gaugeVec(ns, "zfs", "pool_read_errors", "ZFS pool read errors", poolLabels)
	m.zpoolWriteErrors = gaugeVec(ns, "zfs", "pool_write_errors", "ZFS pool write errors", poolLabels)
	m.zpoolCksumErrors = gaugeVec(ns, "zfs", "pool_checksum_errors", "ZFS pool checksum errors", poolLabels)
	m.zfsARCSize = gauge(ns, "zfs", "arc_size_bytes", "ZFS ARC size in bytes")
	m.zfsARCMaxSize = gauge(ns, "zfs", "arc_max_size_bytes", "ZFS ARC max size in bytes")
	m.zfsARCHitRate = gauge(ns, "zfs", "arc_hit_rate_percent", "ZFS ARC hit rate")
	m.zfsARCHits = gauge(ns, "zfs", "arc_hits_total", "ZFS ARC hits total")
	m.zfsARCMisses = gauge(ns, "zfs", "arc_misses_total", "ZFS ARC misses total")
	m.zfsL2Size = gauge(ns, "zfs", "l2arc_size_bytes", "ZFS L2ARC size in bytes")
	m.zfsL2HitRate = gauge(ns, "zfs", "l2arc_hit_rate_percent", "ZFS L2ARC hit rate")
	dsLabels := []string{"dataset", "pool"}
	m.zdatasetUsed = gaugeVec(ns, "zfs", "dataset_used_bytes", "ZFS dataset used in bytes", dsLabels)
	m.zdatasetAvail = gaugeVec(ns, "zfs", "dataset_avail_bytes", "ZFS dataset available in bytes", dsLabels)
	m.zdatasetCompRatio = gaugeVec(ns, "zfs", "dataset_compression_ratio", "ZFS dataset compression ratio", dsLabels)

	// ── Service Checks ──
	svcLabels := []string{"name", "type", "target"}
	m.serviceUp = gaugeVec(ns, "service", "up", "Service check up (1=up, 0=down)", svcLabels)
	m.serviceLatency = gaugeVec(ns, "service", "response_ms", "Service check response latency in ms", svcLabels)
	m.serviceFailures = gaugeVec(ns, "service", "consecutive_failures", "Service check consecutive failures", svcLabels)

	// ── Parity ──
	m.paritySpeedMBs = gauge(ns, "parity", "speed_mb_per_sec", "Latest parity check speed in MB/s")
	m.parityDurationSec = gauge(ns, "parity", "duration_seconds", "Latest parity check duration in seconds")
	m.parityErrors = gauge(ns, "parity", "errors", "Latest parity check error count")
	m.parityRunning = gauge(ns, "parity", "running", "Parity check in progress (1=yes, 0=no)")

	// ── Tunnels ──
	m.cfTunnelUp = gaugeVec(ns, "tunnel", "cloudflared_up", "Cloudflared tunnel healthy (1=yes, 0=no)", []string{"name"})
	m.cfTunnelConns = gaugeVec(ns, "tunnel", "cloudflared_connections", "Cloudflared tunnel connections", []string{"name"})
	m.tsNodeOnline = gaugeVec(ns, "tunnel", "tailscale_node_online", "Tailscale node online (1=yes, 0=no)", []string{"name", "ip"})
	m.tsNodeTxBytes = gaugeVec(ns, "tunnel", "tailscale_node_tx_bytes", "Tailscale node TX bytes", []string{"name", "ip"})
	m.tsNodeRxBytes = gaugeVec(ns, "tunnel", "tailscale_node_rx_bytes", "Tailscale node RX bytes", []string{"name", "ip"})

	// ── Proxmox ──
	pveNodeLabels := []string{"node"}
	m.pveNodeCPU = gaugeVec(ns, "proxmox", "node_cpu_usage", "PVE node CPU usage (0-1)", pveNodeLabels)
	m.pveNodeMemUsed = gaugeVec(ns, "proxmox", "node_memory_used_bytes", "PVE node memory used", pveNodeLabels)
	m.pveNodeMemTotal = gaugeVec(ns, "proxmox", "node_memory_total_bytes", "PVE node memory total", pveNodeLabels)
	m.pveNodeOnline = gaugeVec(ns, "proxmox", "node_online", "PVE node online (1=yes)", pveNodeLabels)
	pveGuestLabels := []string{"vmid", "name", "type", "node"}
	m.pveGuestCPU = gaugeVec(ns, "proxmox", "guest_cpu_usage", "PVE guest CPU usage (0-1)", pveGuestLabels)
	m.pveGuestMemUsed = gaugeVec(ns, "proxmox", "guest_memory_used_bytes", "PVE guest memory used", pveGuestLabels)
	m.pveGuestMemMax = gaugeVec(ns, "proxmox", "guest_memory_max_bytes", "PVE guest memory max", pveGuestLabels)
	m.pveGuestRunning = gaugeVec(ns, "proxmox", "guest_running", "PVE guest running (1=yes)", pveGuestLabels)
	pveStorageLabels := []string{"storage", "node", "type"}
	m.pveStorageUsed = gaugeVec(ns, "proxmox", "storage_used_bytes", "PVE storage used", pveStorageLabels)
	m.pveStorageTotal = gaugeVec(ns, "proxmox", "storage_total_bytes", "PVE storage total", pveStorageLabels)

	// ── Kubernetes ──
	k8sNodeLabels := []string{"node"}
	m.k8sNodeReady = gaugeVec(ns, "k8s", "node_ready", "K8s node ready (1=yes)", k8sNodeLabels)
	m.k8sNodePods = gaugeVec(ns, "k8s", "node_pod_count", "K8s node pod count", k8sNodeLabels)
	k8sPodLabels := []string{"pod", "namespace"}
	m.k8sPodRunning = gaugeVec(ns, "k8s", "pod_running", "K8s pod running (1=yes)", k8sPodLabels)
	m.k8sPodRestarts = gaugeVec(ns, "k8s", "pod_restarts", "K8s pod restart count", k8sPodLabels)
	k8sDepLabels := []string{"deployment", "namespace"}
	m.k8sDeployReady = gaugeVec(ns, "k8s", "deployment_ready_replicas", "K8s deployment ready replicas", k8sDepLabels)
	m.k8sDeployDesired = gaugeVec(ns, "k8s", "deployment_desired_replicas", "K8s deployment desired replicas", k8sDepLabels)

	// ── GPU ──
	gpuLabels := []string{"index", "name", "vendor"}
	m.gpuUsagePct = gaugeVec(ns, "gpu", "usage_percent", "GPU core utilization", gpuLabels)
	m.gpuMemUsedMB = gaugeVec(ns, "gpu", "mem_used_mb", "GPU VRAM used MB", gpuLabels)
	m.gpuMemTotalMB = gaugeVec(ns, "gpu", "mem_total_mb", "GPU VRAM total MB", gpuLabels)
	m.gpuMemPct = gaugeVec(ns, "gpu", "mem_percent", "GPU VRAM utilization", gpuLabels)
	m.gpuTemperature = gaugeVec(ns, "gpu", "temperature_celsius", "GPU temperature", gpuLabels)
	m.gpuPowerW = gaugeVec(ns, "gpu", "power_watts", "GPU power draw", gpuLabels)
	m.gpuPowerMaxW = gaugeVec(ns, "gpu", "power_max_watts", "GPU power limit", gpuLabels)
	m.gpuFanPct = gaugeVec(ns, "gpu", "fan_percent", "GPU fan speed", gpuLabels)
	m.gpuEncoderPct = gaugeVec(ns, "gpu", "encoder_percent", "GPU video encoder utilization", gpuLabels)
	m.gpuDecoderPct = gaugeVec(ns, "gpu", "decoder_percent", "GPU video decoder utilization", gpuLabels)

	// ── Findings ──
	m.findingsTotal = gaugeVec(ns, "findings", "total", "Findings by severity", []string{"severity"})
	m.findingsCritical = gauge(ns, "findings", "critical_count", "Critical finding count")
	m.findingsWarning = gauge(ns, "findings", "warning_count", "Warning finding count")

	// ── Collection ──
	m.collectionDuration = gauge(ns, "", "collection_duration_seconds", "Last diagnostic collection duration")
	m.lastCollectionTime = gauge(ns, "", "last_collection_timestamp", "Last collection unix timestamp")

	// ── Update ──
	m.updateAvailable = gauge(ns, "update", "available", "Platform update available (1=yes, 0=no)")

	// Register all
	collectors := []prometheus.Collector{
		m.cpuUsage, m.memUsed, m.memTotal, m.memPercent, m.swapUsed, m.swapTotal,
		m.loadAvg1, m.loadAvg5, m.loadAvg15, m.ioWait, m.uptime, m.cpuCores,
		m.diskUsedBytes, m.diskTotalBytes, m.diskUsedPct,
		m.smartHealthy, m.smartTemp, m.smartTempMax, m.smartReallocated, m.smartPending,
		m.smartOffline, m.smartUDMACRC, m.smartCmdTimeout, m.smartSpinRetry,
		m.smartPowerOnHours, m.smartSizeBytes,
		m.containerCPU, m.containerMem, m.containerMemPct,
		m.containerNetIn, m.containerNetOut, m.containerBlockRead, m.containerBlockWrite,
		m.containerState, m.containerTotal,
		m.netInterfaceUp, m.netInterfaceMTU,
		m.upsBatteryPct, m.upsBatteryV, m.upsInputV, m.upsOutputV,
		m.upsLoadPct, m.upsRuntimeMins, m.upsWattage, m.upsTemperature,
		m.upsOnBattery, m.upsLowBattery,
		m.zpoolState, m.zpoolUsedBytes, m.zpoolTotalBytes, m.zpoolUsedPct,
		m.zpoolFragPct, m.zpoolScanPct, m.zpoolScanErrors,
		m.zpoolReadErrors, m.zpoolWriteErrors, m.zpoolCksumErrors,
		m.zfsARCSize, m.zfsARCMaxSize, m.zfsARCHitRate, m.zfsARCHits, m.zfsARCMisses,
		m.zfsL2Size, m.zfsL2HitRate,
		m.zdatasetUsed, m.zdatasetAvail, m.zdatasetCompRatio,
		m.serviceUp, m.serviceLatency, m.serviceFailures,
		m.paritySpeedMBs, m.parityDurationSec, m.parityErrors, m.parityRunning,
		m.cfTunnelUp, m.cfTunnelConns, m.tsNodeOnline, m.tsNodeTxBytes, m.tsNodeRxBytes,
		m.pveNodeCPU, m.pveNodeMemUsed, m.pveNodeMemTotal, m.pveNodeOnline,
		m.pveGuestCPU, m.pveGuestMemUsed, m.pveGuestMemMax, m.pveGuestRunning,
		m.pveStorageUsed, m.pveStorageTotal,
		m.k8sNodeReady, m.k8sNodePods, m.k8sPodRunning, m.k8sPodRestarts,
		m.k8sDeployReady, m.k8sDeployDesired,
		m.gpuUsagePct, m.gpuMemUsedMB, m.gpuMemTotalMB, m.gpuMemPct,
		m.gpuTemperature, m.gpuPowerW, m.gpuPowerMaxW, m.gpuFanPct,
		m.gpuEncoderPct, m.gpuDecoderPct,
		m.findingsTotal, m.findingsCritical, m.findingsWarning,
		m.collectionDuration, m.lastCollectionTime, m.updateAvailable,
	}
	for _, c := range collectors {
		m.registry.MustRegister(c)
	}
	return m
}

// Registry returns the prometheus registry for the HTTP handler.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// Update refreshes all Prometheus metrics from a snapshot.
func (m *Metrics) Update(snap *internal.Snapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// ── System ──
	m.cpuUsage.Set(snap.System.CPUUsage)
	m.memUsed.Set(float64(snap.System.MemUsedMB) * 1024 * 1024)
	m.memTotal.Set(float64(snap.System.MemTotalMB) * 1024 * 1024)
	m.memPercent.Set(snap.System.MemPercent)
	m.swapUsed.Set(float64(snap.System.SwapUsedMB) * 1024 * 1024)
	m.swapTotal.Set(float64(snap.System.SwapTotalMB) * 1024 * 1024)
	m.loadAvg1.Set(snap.System.LoadAvg1)
	m.loadAvg5.Set(snap.System.LoadAvg5)
	m.loadAvg15.Set(snap.System.LoadAvg15)
	m.ioWait.Set(snap.System.IOWait)
	m.uptime.Set(float64(snap.System.UptimeSecs))
	m.cpuCores.Set(float64(snap.System.CPUCores))

	// Reset label-based metrics to clear stale entries
	m.diskUsedBytes.Reset()
	m.diskTotalBytes.Reset()
	m.diskUsedPct.Reset()
	m.smartHealthy.Reset()
	m.smartTemp.Reset()
	m.smartTempMax.Reset()
	m.smartReallocated.Reset()
	m.smartPending.Reset()
	m.smartOffline.Reset()
	m.smartUDMACRC.Reset()
	m.smartCmdTimeout.Reset()
	m.smartSpinRetry.Reset()
	m.smartPowerOnHours.Reset()
	m.smartSizeBytes.Reset()
	m.containerCPU.Reset()
	m.containerMem.Reset()
	m.containerMemPct.Reset()
	m.containerNetIn.Reset()
	m.containerNetOut.Reset()
	m.containerBlockRead.Reset()
	m.containerBlockWrite.Reset()
	m.containerState.Reset()
	m.netInterfaceUp.Reset()
	m.netInterfaceMTU.Reset()
	m.zpoolState.Reset()
	m.zpoolUsedBytes.Reset()
	m.zpoolTotalBytes.Reset()
	m.zpoolUsedPct.Reset()
	m.zpoolFragPct.Reset()
	m.zpoolScanPct.Reset()
	m.zpoolScanErrors.Reset()
	m.zpoolReadErrors.Reset()
	m.zpoolWriteErrors.Reset()
	m.zpoolCksumErrors.Reset()
	m.zdatasetUsed.Reset()
	m.zdatasetAvail.Reset()
	m.zdatasetCompRatio.Reset()
	m.serviceUp.Reset()
	m.serviceLatency.Reset()
	m.serviceFailures.Reset()
	m.cfTunnelUp.Reset()
	m.cfTunnelConns.Reset()
	m.tsNodeOnline.Reset()
	m.tsNodeTxBytes.Reset()
	m.tsNodeRxBytes.Reset()

	// ── Disks ──
	for _, d := range snap.Disks {
		l := prometheus.Labels{"device": d.Device, "mountpoint": d.MountPoint, "label": d.Label}
		m.diskUsedBytes.With(l).Set(d.UsedGB * 1024 * 1024 * 1024)
		m.diskTotalBytes.With(l).Set(d.TotalGB * 1024 * 1024 * 1024)
		m.diskUsedPct.With(l).Set(d.UsedPct)
	}

	// ── SMART ──
	for _, d := range snap.SMART {
		l := prometheus.Labels{"device": d.Device, "model": sanitizeLabel(d.Model), "serial": d.Serial}
		m.smartHealthy.With(l).Set(boolToFloat(d.HealthPassed))
		m.smartTemp.With(l).Set(float64(d.Temperature))
		m.smartTempMax.With(l).Set(float64(d.TempMax))
		m.smartReallocated.With(l).Set(float64(d.Reallocated))
		m.smartPending.With(l).Set(float64(d.Pending))
		m.smartOffline.With(l).Set(float64(d.Offline))
		m.smartUDMACRC.With(l).Set(float64(d.UDMACRC))
		m.smartCmdTimeout.With(l).Set(float64(d.CommandTimeout))
		m.smartSpinRetry.With(l).Set(float64(d.SpinRetry))
		m.smartPowerOnHours.With(l).Set(float64(d.PowerOnHours))
		m.smartSizeBytes.With(l).Set(d.SizeGB * 1024 * 1024 * 1024)
	}

	// ── Docker ──
	m.containerTotal.Set(float64(len(snap.Docker.Containers)))
	for _, c := range snap.Docker.Containers {
		l := prometheus.Labels{"name": c.Name, "image": sanitizeLabel(c.Image)}
		running := c.State == "running"
		m.containerState.With(l).Set(boolToFloat(running))
		if running {
			m.containerCPU.With(l).Set(c.CPU)
			m.containerMem.With(l).Set(c.MemMB * 1024 * 1024)
			m.containerMemPct.With(l).Set(c.MemPct)
			m.containerNetIn.With(l).Set(c.NetIn)
			m.containerNetOut.With(l).Set(c.NetOut)
			m.containerBlockRead.With(l).Set(c.BlockRead)
			m.containerBlockWrite.With(l).Set(c.BlockWrite)
		}
	}

	// ── Network ──
	for _, iface := range snap.Network.Interfaces {
		l := prometheus.Labels{"interface": iface.Name}
		up := strings.EqualFold(iface.State, "up")
		m.netInterfaceUp.With(l).Set(boolToFloat(up))
		m.netInterfaceMTU.With(l).Set(float64(iface.MTU))
	}

	// ── UPS ──
	if snap.UPS != nil && snap.UPS.Available {
		m.upsBatteryPct.Set(snap.UPS.BatteryPct)
		m.upsBatteryV.Set(snap.UPS.BatteryV)
		m.upsInputV.Set(snap.UPS.InputV)
		m.upsOutputV.Set(snap.UPS.OutputV)
		m.upsLoadPct.Set(snap.UPS.LoadPct)
		m.upsRuntimeMins.Set(snap.UPS.RuntimeMins)
		m.upsWattage.Set(snap.UPS.WattageW)
		m.upsTemperature.Set(snap.UPS.Temperature)
		m.upsOnBattery.Set(boolToFloat(snap.UPS.OnBattery))
		m.upsLowBattery.Set(boolToFloat(snap.UPS.LowBattery))
	}

	// ── GPU ──
	if snap.GPU != nil && snap.GPU.Available {
		m.gpuUsagePct.Reset()
		m.gpuMemUsedMB.Reset()
		m.gpuMemTotalMB.Reset()
		m.gpuMemPct.Reset()
		m.gpuTemperature.Reset()
		m.gpuPowerW.Reset()
		m.gpuPowerMaxW.Reset()
		m.gpuFanPct.Reset()
		m.gpuEncoderPct.Reset()
		m.gpuDecoderPct.Reset()
		for _, g := range snap.GPU.GPUs {
			l := prometheus.Labels{"index": fmt.Sprintf("%d", g.Index), "name": g.Name, "vendor": g.Vendor}
			m.gpuUsagePct.With(l).Set(g.UsagePct)
			m.gpuMemUsedMB.With(l).Set(g.MemUsedMB)
			m.gpuMemTotalMB.With(l).Set(g.MemTotalMB)
			m.gpuMemPct.With(l).Set(g.MemPct)
			m.gpuTemperature.With(l).Set(float64(g.Temperature))
			m.gpuPowerW.With(l).Set(g.PowerW)
			m.gpuPowerMaxW.With(l).Set(g.PowerMaxW)
			m.gpuFanPct.With(l).Set(g.FanPct)
			m.gpuEncoderPct.With(l).Set(g.EncoderPct)
			m.gpuDecoderPct.With(l).Set(g.DecoderPct)
		}
	}

	// ── ZFS ──
	if snap.ZFS != nil && snap.ZFS.Available {
		for _, pool := range snap.ZFS.Pools {
			l := prometheus.Labels{"pool": pool.Name}
			m.zpoolState.With(l).Set(boolToFloat(strings.EqualFold(pool.State, "ONLINE")))
			m.zpoolUsedBytes.With(l).Set(pool.UsedGB * 1024 * 1024 * 1024)
			m.zpoolTotalBytes.With(l).Set(pool.TotalGB * 1024 * 1024 * 1024)
			m.zpoolUsedPct.With(l).Set(pool.UsedPct)
			m.zpoolFragPct.With(l).Set(float64(pool.Fragmentation))
			m.zpoolScanPct.With(l).Set(pool.ScanPct)
			m.zpoolScanErrors.With(l).Set(float64(pool.ScanErrors))
			m.zpoolReadErrors.With(l).Set(float64(pool.Errors.Read))
			m.zpoolWriteErrors.With(l).Set(float64(pool.Errors.Write))
			m.zpoolCksumErrors.With(l).Set(float64(pool.Errors.Checksum))
		}
		if snap.ZFS.ARC != nil {
			m.zfsARCSize.Set(snap.ZFS.ARC.SizeMB * 1024 * 1024)
			m.zfsARCMaxSize.Set(snap.ZFS.ARC.MaxSizeMB * 1024 * 1024)
			m.zfsARCHitRate.Set(snap.ZFS.ARC.HitRate)
			m.zfsARCHits.Set(float64(snap.ZFS.ARC.Hits))
			m.zfsARCMisses.Set(float64(snap.ZFS.ARC.Misses))
			m.zfsL2Size.Set(snap.ZFS.ARC.L2SizeMB * 1024 * 1024)
			m.zfsL2HitRate.Set(snap.ZFS.ARC.L2HitRate)
		}
		for _, ds := range snap.ZFS.Datasets {
			l := prometheus.Labels{"dataset": ds.Name, "pool": ds.Pool}
			m.zdatasetUsed.With(l).Set(ds.UsedGB * 1024 * 1024 * 1024)
			m.zdatasetAvail.With(l).Set(ds.AvailGB * 1024 * 1024 * 1024)
			m.zdatasetCompRatio.With(l).Set(ds.CompRatio)
		}
	}

	// ── Service Checks ──
	for _, sc := range snap.Services {
		l := prometheus.Labels{"name": sc.Name, "type": sc.Type, "target": sanitizeLabel(sc.Target)}
		m.serviceUp.With(l).Set(boolToFloat(sc.Status == "up"))
		m.serviceLatency.With(l).Set(float64(sc.ResponseMS))
		m.serviceFailures.With(l).Set(float64(sc.ConsecutiveFailures))
	}

	// ── Parity ──
	if snap.Parity != nil {
		m.parityRunning.Set(boolToFloat(strings.EqualFold(snap.Parity.Status, "running")))
		if len(snap.Parity.History) > 0 {
			last := snap.Parity.History[len(snap.Parity.History)-1]
			m.paritySpeedMBs.Set(last.SpeedMBs)
			m.parityDurationSec.Set(float64(last.Duration))
			m.parityErrors.Set(float64(last.Errors))
		}
	}

	// ── Tunnels ──
	if snap.Tunnels != nil {
		if snap.Tunnels.Cloudflared != nil {
			for _, t := range snap.Tunnels.Cloudflared.Tunnels {
				l := prometheus.Labels{"name": t.Name}
				m.cfTunnelUp.With(l).Set(boolToFloat(t.Status == "healthy"))
				m.cfTunnelConns.With(l).Set(float64(t.Connections))
			}
		}
		if snap.Tunnels.Tailscale != nil {
			all := make([]internal.TailscaleNode, 0)
			if snap.Tunnels.Tailscale.Self != nil {
				all = append(all, *snap.Tunnels.Tailscale.Self)
			}
			all = append(all, snap.Tunnels.Tailscale.Peers...)
			for _, nd := range all {
				l := prometheus.Labels{"name": nd.Name, "ip": nd.IP}
				m.tsNodeOnline.With(l).Set(boolToFloat(nd.Online))
				m.tsNodeTxBytes.With(l).Set(float64(nd.TxBytes))
				m.tsNodeRxBytes.With(l).Set(float64(nd.RxBytes))
			}
		}
	}

	// ── Proxmox ──
	m.pveNodeCPU.Reset()
	m.pveNodeMemUsed.Reset()
	m.pveNodeMemTotal.Reset()
	m.pveNodeOnline.Reset()
	m.pveGuestCPU.Reset()
	m.pveGuestMemUsed.Reset()
	m.pveGuestMemMax.Reset()
	m.pveGuestRunning.Reset()
	m.pveStorageUsed.Reset()
	m.pveStorageTotal.Reset()
	if snap.Proxmox != nil && snap.Proxmox.Connected {
		for _, n := range snap.Proxmox.Nodes {
			l := prometheus.Labels{"node": n.Name}
			m.pveNodeCPU.With(l).Set(n.CPUUsage)
			m.pveNodeMemUsed.With(l).Set(float64(n.MemUsed))
			m.pveNodeMemTotal.With(l).Set(float64(n.MemTotal))
			m.pveNodeOnline.With(l).Set(boolToFloat(n.Status == "online"))
		}
		for _, g := range snap.Proxmox.Guests {
			l := prometheus.Labels{"vmid": fmt.Sprintf("%d", g.VMID), "name": g.Name, "type": g.Type, "node": g.Node}
			m.pveGuestCPU.With(l).Set(g.CPUUsage)
			m.pveGuestMemUsed.With(l).Set(float64(g.MemUsed))
			m.pveGuestMemMax.With(l).Set(float64(g.MemMax))
			m.pveGuestRunning.With(l).Set(boolToFloat(g.Status == "running"))
		}
		for _, s := range snap.Proxmox.Storage {
			l := prometheus.Labels{"storage": s.Storage, "node": s.Node, "type": s.Type}
			m.pveStorageUsed.With(l).Set(float64(s.Used))
			m.pveStorageTotal.With(l).Set(float64(s.Total))
		}
	}

	// ── Kubernetes ──
	m.k8sNodeReady.Reset()
	m.k8sNodePods.Reset()
	m.k8sPodRunning.Reset()
	m.k8sPodRestarts.Reset()
	m.k8sDeployReady.Reset()
	m.k8sDeployDesired.Reset()
	if snap.Kubernetes != nil && snap.Kubernetes.Connected {
		for _, n := range snap.Kubernetes.Nodes {
			l := prometheus.Labels{"node": n.Name}
			m.k8sNodeReady.With(l).Set(boolToFloat(n.Status == "Ready"))
			m.k8sNodePods.With(l).Set(float64(n.PodCount))
		}
		for _, p := range snap.Kubernetes.Pods {
			l := prometheus.Labels{"pod": p.Name, "namespace": p.Namespace}
			m.k8sPodRunning.With(l).Set(boolToFloat(p.Status == "Running"))
			m.k8sPodRestarts.With(l).Set(float64(p.Restarts))
		}
		for _, d := range snap.Kubernetes.Deployments {
			l := prometheus.Labels{"deployment": d.Name, "namespace": d.Namespace}
			m.k8sDeployReady.With(l).Set(float64(d.ReadyReplicas))
			m.k8sDeployDesired.With(l).Set(float64(d.Replicas))
		}
	}

	// ── Findings ──
	critical, warnings, infos := countBySeverity(snap.Findings)
	m.findingsCritical.Set(float64(critical))
	m.findingsWarning.Set(float64(warnings))
	m.findingsTotal.With(prometheus.Labels{"severity": "critical"}).Set(float64(critical))
	m.findingsTotal.With(prometheus.Labels{"severity": "warning"}).Set(float64(warnings))
	m.findingsTotal.With(prometheus.Labels{"severity": "info"}).Set(float64(infos))

	// ── Update ──
	if snap.Update != nil {
		m.updateAvailable.Set(boolToFloat(snap.Update.UpdateAvailable))
	}

	// ── Collection ──
	m.collectionDuration.Set(snap.Duration)
	m.lastCollectionTime.Set(float64(snap.Timestamp.Unix()))
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func sanitizeLabel(s string) string {
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
