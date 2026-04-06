<p align="center">
  <img src="icons/icon3.png" alt="NAS Doctor" width="128" height="128">
</p>

<h1 align="center">NAS Doctor</h1>

<p align="center">
  <strong>Local NAS diagnostic and monitoring tool.</strong><br>
  Run it as a Docker container on your Unraid, TrueNAS, Synology, or any Linux NAS.<br>
  Beautiful dashboards, Prometheus metrics, webhook alerts — no cloud account required.
</p>

> **Early Access** — NAS Doctor is under active development. Expect bugs, missing features, and breaking changes. [Report issues here.](https://github.com/mcdays94/nas-doctor/issues)

---

![NAS Doctor Dashboard](screenshots/midnight-top.jpg)

NAS Doctor runs periodic health checks on your server — analyzing SMART data, disk usage, Docker containers, kernel logs, temperatures, ZFS pools, UPS power, and Unraid parity — then surfaces findings with clear severity ratings, root-cause correlation, and actionable recommendations backed by Backblaze failure rate data.

Born from an [OpenCode diagnostic skill](https://github.com/mcdays94/opencode-server-diagnostic-skill) that generates professional PDF server reports, NAS Doctor packages the same intelligence into a self-hosted app anyone can install.

---

## What It Does

### Diagnostics
- **SMART Health**: Per-drive health, temperature, reallocated sectors, pending sectors, UDMA CRC errors, power-on hours, ATA port mapping, with **Backblaze failure-rate thresholds** (Q4-2025 data, 337k+ drives)
- **Historical Sparklines**: CPU, memory, I/O wait, and per-drive temperature trends inline on the dashboard
- **Disk Space**: Usage per mount point with color-coded thresholds
- **System**: CPU, memory, load average, I/O wait, uptime, platform detection
- **Docker**: Container listing with status and uptime
- **ZFS Pool Health**: Pool state, vdev tree, scrub/resilver status, ARC hit rate, fragmentation, dataset listing with compression ratios
- **UPS / Power**: Battery level, load, runtime, wattage via NUT or apcupsd — with critical alerts for on-battery and low-battery events
- **Network**: Interface speed negotiation, state, MTU
- **Logs**: Filtered dmesg and syslog errors (ATA errors, I/O errors, medium errors)
- **Parity** (Unraid): Historical parity check speed trend analysis, error tracking
- **OS Update Check**: Compares installed version against latest GitHub release for Unraid and TrueNAS

### Analysis Engine

20+ diagnostic rules with automatic cross-correlation:

- UDMA CRC errors + slow parity → **Root cause: SATA cable failure**
- High temperatures + slow parity → **Thermal throttling**
- No SSD cache + high I/O wait + Docker containers → **I/O starvation**
- Pending sectors + reallocated sectors → **Failing drive media**
- Reallocated sectors at Backblaze 12.0x failure rate → **Replace immediately**
- ZFS pool DEGRADED with REMOVED vdev → **No redundancy, replace disk**
- UPS on battery with low runtime → **Initiate graceful shutdown**
- OS significantly out of date → **Security vulnerability risk**
- And more...

### Export Reports

Click **Export Report** in the dashboard to generate a professional, print-ready diagnostic report styled after the CF Workers Design System. Open in your browser and Print → Save as PDF.

<p>
  <img src="screenshots/report-cover.jpg" alt="Report Cover Page" width="420">
</p>

### Multi-Server Fleet Monitoring

Monitor all your NAS Doctor instances from one dashboard. Go to `/fleet` to see an aggregated view of all servers with health status, finding counts, and direct links.

### Integrations

| Integration | How |
|---|---|
| **Prometheus** | Scrape `/metrics` — 30+ gauges for system, disk, SMART, Docker, findings |
| **Grafana** | Connect via Prometheus data source |
| **Discord** | Webhook with rich embeds, severity colors, finding details |
| **Slack** | Webhook with blocks, severity counts, top findings |
| **Gotify** | Native push notifications with priority mapping |
| **Ntfy** | Push notifications with priority and tags |
| **Generic HTTP** | JSON payload with HMAC-SHA256 signing for custom integrations |

---

## Quick Start

### Docker Compose (recommended)

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true
    network_mode: host
    volumes:
      - nas-doctor-data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /boot:/host/boot:ro
      - /var/log:/host/log:ro
      - /mnt:/host/mnt:ro
      - /etc/unraid-version:/etc/unraid-version:ro
    environment:
      - TZ=Europe/Lisbon
      - NAS_DOCTOR_INTERVAL=6h
    ports:
      - "8080:8060"
    restart: unless-stopped

volumes:
  nas-doctor-data:
```

```bash
docker compose up -d
```

Then open `http://your-nas:8060`.

### Docker Run

```bash
docker run -d \
  --name nas-doctor \
  --privileged \
  --network host \
  -v nas-doctor-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /boot:/host/boot:ro \
  -v /var/log:/host/log:ro \
  -v /mnt:/host/mnt:ro \
  -v /etc/unraid-version:/etc/unraid-version:ro \
  -e TZ=Europe/Lisbon \
  -p 8080:8060 \
  --restart unless-stopped \
  ghcr.io/mcdays94/nas-doctor:latest
```

### Build from Source

```bash
git clone https://github.com/mcdays94/nas-doctor.git
cd nas-doctor
go build -o nas-doctor ./cmd/nas-doctor
./nas-doctor -listen :8060 -data ./data -interval 6h
```

---

## Themes

NAS Doctor ships with 3 dashboard themes. Switch between them from the nav bar.

| Theme | Description |
|---|---|
| **Midnight** (default) | Ultra-dark precision dashboard |
| **Clean** | White, minimal gallery space |
| **Ember** | macOS-native depth, serif typography, micro-animations |

<p>
  <img src="screenshots/midnight-top.jpg" alt="Midnight" width="380">
  <img src="screenshots/clean-top.jpg" alt="Clean" width="380">
</p>
<p>
  <img src="screenshots/ember-top.jpg" alt="Ember" width="380">
</p>

---

## Settings

All configurable from the web UI at `/settings`:

- **Scan Interval**: Preset frequencies or custom (days/hours/minutes/seconds with cron preview)
- **Theme**: Midnight, Clean, or Ember — settings page inherits the active theme
- **Webhooks**: Add/remove/test Discord, Slack, Gotify, Ntfy, or generic HTTP webhooks
- **Data Lifecycle**: Snapshot retention days, max DB size cap, notification log retention
- **Automatic Backup**: Scheduled DB backups with configurable location and retention
- **Dashboard Sections**: Toggle visibility of individual sections (SMART, Docker, ZFS, UPS, etc.)
- **Fleet Monitoring**: Add/remove remote NAS Doctor instances for multi-server monitoring
- **Log Forwarding**: Forward scan results to external logging endpoints (coming soon)

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `NAS_DOCTOR_LISTEN` | `:8060` | HTTP listen address |
| `NAS_DOCTOR_DATA` | `/data` | SQLite database directory |
| `NAS_DOCTOR_INTERVAL` | `6h` | Diagnostic scan interval |
| `TZ` | `UTC` | Timezone |

---

## API Reference

| Endpoint | Method | Description |
|---|---|---|
| `/api/v1/health` | GET | Healthcheck (status, version, uptime) |
| `/api/v1/status` | GET | Server status summary with section visibility |
| `/api/v1/snapshot/latest` | GET | Full latest diagnostic snapshot |
| `/api/v1/snapshot/{id}` | GET | Specific snapshot by ID |
| `/api/v1/snapshots` | GET | List recent snapshots |
| `/api/v1/scan` | POST | Trigger immediate diagnostic scan |
| `/api/v1/report` | GET | Generate print-ready HTML diagnostic report |
| `/api/v1/settings` | GET/PUT | Read/write application settings |
| `/api/v1/sparklines` | GET | Condensed system + SMART history for charts |
| `/api/v1/db/stats` | GET | Database size and row counts |
| `/api/v1/backup` | GET/POST | List or trigger database backup |
| `/api/v1/fleet` | GET | Aggregated status of all remote servers |
| `/api/v1/fleet/servers` | GET/PUT | Manage remote server list |
| `/metrics` | GET | Prometheus metrics endpoint |

---

## Prometheus Metrics

All metrics prefixed with `nasdoctor_`. Full list:

<details>
<summary>Expand metric list</summary>

```
# System
nasdoctor_system_cpu_usage_percent
nasdoctor_system_memory_used_bytes / _total_bytes
nasdoctor_system_load_avg_1 / _5 / _15
nasdoctor_system_io_wait_percent
nasdoctor_system_uptime_seconds

# Disks (labels: device, mountpoint, label)
nasdoctor_disk_used_bytes / _total_bytes / _used_percent

# SMART (labels: device, model, serial)
nasdoctor_smart_healthy  (1=passed, 0=failed)
nasdoctor_smart_temperature_celsius
nasdoctor_smart_reallocated_sectors / _pending_sectors
nasdoctor_smart_udma_crc_errors / _power_on_hours

# Docker (labels: name, image)
nasdoctor_docker_container_cpu_percent / _memory_bytes

# Findings
nasdoctor_findings_critical_count / _warning_count
nasdoctor_findings_total{severity="critical|warning|info"}

# Parity (Unraid)
nasdoctor_parity_speed_mb_per_sec / _duration_seconds

# Collection
nasdoctor_collection_duration_seconds
nasdoctor_last_collection_timestamp
```

</details>

---

## Supported Platforms

| Platform | Status | Notes |
|---|---|---|
| **Unraid** | ✅ Tested | Parity analysis, array status, disk labels, OS update check |
| **TrueNAS SCALE** | ⚠️ Untested | ZFS pool health support built-in, but not yet validated on real hardware |
| **Synology DSM** | ⚠️ Untested | Should work via Docker / Container Manager |
| **QNAP QTS** | ⚠️ Untested | Should work via Container Station |
| **Proxmox** | ⚠️ Untested | ZFS pool health support built-in |
| **Generic Linux** | ⚠️ Untested | Any distro with Docker |

> NAS Doctor has only been tested on **Unraid** so far. Other platforms should work but may have issues with disk detection, SMART access, or platform-specific features. [Report issues here.](https://github.com/mcdays94/nas-doctor/issues)

---

## Resource Usage

NAS Doctor is designed to be invisible on your system:

| Resource | During scan (~15s every 6h) | Between scans |
|---|---|---|
| **CPU** | <2% | ~0% |
| **Memory** | ~30-50 MB | ~30-50 MB |
| **Disk I/O** | Read-only: `/proc`, `smartctl`, `dmesg` | Zero |
| **Network** | OS update check (1 req/day) | Serves UI only when accessed |

---

## Demo Mode

Preview all themes with realistic mock data (no NAS needed):

```bash
go build -o nas-doctor ./cmd/nas-doctor
./nas-doctor -demo -listen :8060
```

Demo includes: 7 SMART drives (with Backblaze-informed findings), 14 Docker containers, 2 ZFS pools (one DEGRADED), UPS monitoring, OS update notification, 30 days of historical sparkline data.

---

## Roadmap

- [x] Config UI in dashboard (manage webhooks, scan interval, theme, icon selection)
- [x] Unraid Community Apps template
- [x] PDF report export (CF Workers Design System styled, print-ready)
- [x] Custom scan frequency with full granularity + cron preview
- [x] Dynamic scheduler interval updates (no restart required)
- [x] Settings page inherits active theme
- [x] Historical trend sparklines (CPU, memory, I/O, SMART temperature)
- [x] Backblaze failure-rate thresholds (Q4-2025, 337k+ drives)
- [x] Data lifecycle management (retention policies, DB size cap, auto-pruning)
- [x] Automatic backup with configurable schedule and location
- [x] ZFS pool health detection (vdev tree, scrub/resilver, ARC stats)
- [x] UPS / Power monitoring (NUT + apcupsd)
- [x] NAS OS update check (Unraid, TrueNAS via GitHub API)
- [x] Multi-server fleet monitoring (hub/spoke)
- [x] Dashboard section visibility toggles
- [x] Click-to-expand findings with evidence detail (all themes)
- [ ] GitHub Actions CI for multi-arch Docker builds (amd64/arm64)
- [ ] Log Forwarding (export to external endpoints)
- [ ] Optional cloud AI integration for deep root-cause analysis

---

## License

MIT

---

<p align="center">
  If NAS Doctor helps you sleep better knowing your server is healthy:<br><br>
  <a href="https://buymeacoffee.com/miguelcaetanodias"><img src="https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-yellow.svg?style=flat-square&logo=buy-me-a-coffee" alt="Buy Me A Coffee"></a>
</p>
