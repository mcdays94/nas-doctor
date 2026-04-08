<p align="center">
  <img src="icons/icon3.png" alt="NAS Doctor" width="128" height="128">
</p>

<h1 align="center">NAS Doctor</h1>

<p align="center">
  <strong>Local NAS diagnostic and monitoring tool.</strong><br>
  Run it as a Docker container on your Unraid, TrueNAS, Synology, or any Linux NAS.<br>
  Beautiful dashboards, Prometheus metrics, webhook alerts — no cloud account required.
</p>

> **Alpha** — NAS Doctor is in alpha. Features may be incomplete, bugs are expected, and breaking changes can occur between releases. Only tested on Unraid. [Report issues here.](https://github.com/mcdays94/nas-doctor/issues)

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
- **UPS / Power**: Battery level, load, runtime, wattage via NUT or apcupsd (local or remote) — with critical alerts for on-battery and low-battery events
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

Click **Export Report** on the dashboard to generate a professional, print-ready diagnostic report. Open in your browser and Print -> Save as PDF.

<p>
  <img src="screenshots/report-cover.jpg" alt="Report Cover Page" width="420">
</p>

### Alerts & Incident Management

Dedicated `/alerts` page with:
- **Active Alerts** — acknowledge, snooze, unsnooze with full lifecycle timeline per alert
- **Incident Timeline & Correlation** — correlate alerts against CPU, memory, I/O wait, and disk temperature over selectable windows (24h/7d/30d)
- **Predictive Trend Intelligence** — worsening-pattern detection for SMART counters with urgency scoring, confidence levels, and parity risk markers
- **Notification History** — webhook delivery log with status, error details, and auto-refresh
- **Draggable cards** — reorder, collapse, and toggle card visibility with layout persistence

### Service Checks

Built-in uptime monitoring for your infrastructure:
- **HTTP/HTTPS** checks with status code and response time
- **TCP** port checks
- **DNS** resolution checks
- **SMB/NFS** share availability checks
- Configurable intervals, timeouts, and expected responses
- Historical results with latency tracking

### Notification Policies

Fine-grained alert routing configured from the Settings UI:
- **Notification Policies** — route alerts to specific webhooks by severity, category, and hostname with per-policy cooldowns
- **Quiet Hours** — suppress notifications during a daily time window (alerts still recorded)
- **Maintenance Windows** — scheduled suppression periods per hostname
- **Default Cooldown** — global deduplication window for repeated alerts
- **Webhook Custom Headers** — add custom HTTP headers to any webhook

### Multi-Server Fleet Monitoring

Monitor all your NAS Doctor instances from one dashboard. Go to `/fleet` to see an aggregated view of all servers with health status, finding counts, and direct links. Supports optional API key authentication per server.

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
    privileged: true          # Required for SMART access
    network_mode: host
    volumes:
      - nas-doctor-data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/log:/host/log:ro
      # Mount your storage volumes (platform-specific):
      - /mnt:/host/mnt:ro              # Unraid, TrueNAS
      # - /volume1:/host/volume1:ro    # Synology (add each volume)
      # - /volume2:/host/volume2:ro    # Synology
      # Unraid-specific (optional, omit on other platforms):
      - /boot:/host/boot:ro
      - /etc/unraid-version:/etc/unraid-version:ro
    environment:
      - TZ=Europe/Lisbon
      - NAS_DOCTOR_INTERVAL=6h
    restart: unless-stopped

volumes:
  nas-doctor-data:
```

```bash
docker compose up -d
```

Then open `http://your-nas:8060`. See platform-specific sections below for Unraid, Synology, and TrueNAS configurations.

### Unraid — Docker UI Setup

1. Go to **Docker** tab → scroll down → **Add Container**
2. Fill in the fields:

| Field | Value |
|---|---|
| **Name** | `nas-doctor` |
| **Repository** | `ghcr.io/mcdays94/nas-doctor:latest` |
| **Icon URL** | `https://raw.githubusercontent.com/mcdays94/nas-doctor/main/icons/icon3.png` |
| **WebUI** | `http://[IP]:[PORT:8060]/` |
| **Network Type** | `Host` |
| **Privileged** | `On` (**required** — SMART access needs raw device access) |

3. Add these **path mappings** (click "Add another Path, Port, Variable..." for each):

| Name | Container Path | Host Path | Mode | Why |
|---|---|---|---|---|
| Data | `/data` | `/mnt/user/appdata/nas-doctor` | RW | Database, config, backups |
| Docker Socket | `/var/run/docker.sock` | `/var/run/docker.sock` | RO | Container monitoring |
| Boot Config | `/host/boot` | `/boot` | RO | Parity logs, Unraid ident |
| System Logs | `/host/log` | `/var/log` | RO | dmesg, syslog analysis |
| Host Mounts | `/host/mnt` | `/mnt` | RO | Per-disk space monitoring |
| Unraid Version | `/etc/unraid-version` | `/etc/unraid-version` | RO | OS update detection |

4. Add this **variable**:

| Key | Value |
|---|---|
| `TZ` | Your timezone (e.g. `Europe/Lisbon`, `America/New_York`) |

5. Click **Apply**

Then open `http://your-unraid-ip:8060`.

> **Important**: Privileged mode and the Host Mounts volume (`/mnt:/host/mnt:ro`) are required. Without privileged, SMART data won't work. Without `/mnt`, per-disk space won't show.

### Synology DSM — Container Manager

Deploy via **Container Manager** (or Docker via SSH).

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true
    network_mode: host
    volumes:
      - /volume1/docker/nas-doctor:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/log:/host/log:ro
      - /volume1:/host/volume1:ro
      - /volume2:/host/volume2:ro          # add more volumes as needed
    environment:
      - TZ=Europe/Lisbon
      - NAS_DOCTOR_INTERVAL=6h
    restart: unless-stopped
```

Then open `http://your-synology-ip:8060`.

> **Synology notes**:
> - **Privileged mode is required** for SMART access (`smartctl` needs raw device access)
> - Mount each `/volume<#>` you want monitored — Synology uses `/volume1`, `/volume2`, etc. instead of `/mnt`
> - There is no `/boot` or `/etc/unraid-version` on Synology — omit those mounts
> - Parity analysis is Unraid-specific and will be skipped automatically

### TrueNAS SCALE

Deploy via **Apps** or via SSH with Docker Compose.

```yaml
services:
  nas-doctor:
    image: ghcr.io/mcdays94/nas-doctor:latest
    container_name: nas-doctor
    privileged: true
    network_mode: host
    volumes:
      - /mnt/pool/appdata/nas-doctor:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/log:/host/log:ro
      - /mnt:/host/mnt:ro
    environment:
      - TZ=America/New_York
      - NAS_DOCTOR_INTERVAL=6h
    restart: unless-stopped
```

Then open `http://your-truenas-ip:8060`.

> **TrueNAS notes**:
> - **Privileged mode is required** for SMART access
> - ZFS pool health, scrub status, ARC hit rate, and dataset listing work automatically
> - Mount `/mnt` to see all pool/dataset storage usage
> - Parity analysis is Unraid-specific and will be skipped automatically
> - UPS monitoring works if NUT is configured (TrueNAS has built-in NUT support)

### Build from Source

```bash
git clone https://github.com/mcdays94/nas-doctor.git
cd nas-doctor
go build -o nas-doctor ./cmd/nas-doctor
./nas-doctor -listen :8060 -data ./data -interval 6h
```

---

## Themes

NAS Doctor ships with 3 dashboard themes. Switch between them from Settings.

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

### More Pages

<p>
  <img src="screenshots/alerts-page.jpg" alt="Alerts Page" width="380">
  <img src="screenshots/settings-page.jpg" alt="Settings Page" width="380">
</p>
<p>
  <img src="screenshots/stats-page.jpg" alt="Stats Page" width="380">
  <img src="screenshots/fleet-page.jpg" alt="Fleet Page" width="380">
</p>

---

## Settings

All configurable from the web UI at `/settings`, organized with a sticky section nav:

- **General**: Scan interval (preset or custom with cron preview), theme selection, app icon
- **Webhooks**: Add/remove/test Discord, Slack, Gotify, Ntfy, or generic HTTP webhooks with optional custom headers and HMAC signing
- **Notification Behavior**: Default cooldown, quiet hours (timezone-aware), maintenance windows, notification policies with per-webhook routing rules
- **Service Checks**: HTTP, TCP, DNS, SMB/NFS uptime monitoring with configurable intervals
- **Fleet**: Add/remove remote NAS Doctor instances with optional API key auth
- **Dashboard Sections**: Toggle visibility of individual sections (SMART, Docker, ZFS, UPS, Parity, Network, etc.)
- **Data & Retention**: Snapshot retention days, max DB size cap, notification log retention
- **Backup**: Scheduled DB backups with configurable location, interval, and retention count
- **Log Forwarding**: Forward scan results to external logging endpoints (coming soon)

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `NAS_DOCTOR_LISTEN` | `:8060` | HTTP listen address |
| `NAS_DOCTOR_DATA` | `/data` | SQLite database directory |
| `NAS_DOCTOR_INTERVAL` | `6h` | Diagnostic scan interval |
| `NAS_DOCTOR_UPS_NAME` | (auto-detect) | NUT UPS name (skip auto-detect from `upsc -l`) |
| `NAS_DOCTOR_NUT_HOST` | (local) | Remote NUT server host (queries `upsname@host`) |
| `NAS_DOCTOR_APCUPSD_HOST` | (local) | Remote apcupsd daemon `host:port` |
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
| `/api/v1/settings/test-webhook` | POST | Send test notification to a webhook |
| `/api/v1/sparklines` | GET | Condensed system + SMART history for charts |
| `/api/v1/history/system` | GET | System metrics history (CPU, memory, I/O) |
| `/api/v1/disks` | GET | List all drives with SMART data |
| `/api/v1/disks/{serial}` | GET | Per-drive detail with full SMART history |
| `/api/v1/alerts` | GET | List alerts (filterable by status) |
| `/api/v1/alerts/{id}` | GET | Get single alert detail |
| `/api/v1/alerts/{id}/events` | GET | Alert lifecycle timeline events |
| `/api/v1/alerts/{id}/ack` | POST | Acknowledge an alert |
| `/api/v1/alerts/{id}/unack` | POST | Unacknowledge an alert |
| `/api/v1/alerts/{id}/snooze` | POST | Snooze an alert (with `until` timestamp) |
| `/api/v1/alerts/{id}/unsnooze` | POST | Unsnooze an alert |
| `/api/v1/incidents/timeline` | GET | Incident timeline with system metrics overlay |
| `/api/v1/incidents/correlation` | GET | Alert correlation (before/during/after metrics) |
| `/api/v1/smart/trends` | GET | SMART degradation trends with risk scoring |
| `/api/v1/notifications/log` | GET | Webhook delivery history |
| `/api/v1/service-checks` | GET | Latest service check results |
| `/api/v1/service-checks/history` | GET | Service check result history |
| `/api/v1/service-checks/run` | POST | Trigger service checks immediately |
| `/api/v1/findings/dismiss` | POST | Dismiss a finding from the dashboard |
| `/api/v1/findings/restore` | POST | Restore a dismissed finding |
| `/api/v1/db/stats` | GET | Database size and row counts |
| `/api/v1/backup` | GET/POST | List or trigger database backup |
| `/api/v1/fleet` | GET | Aggregated status of all remote servers |
| `/api/v1/fleet/servers` | GET/PUT | Manage remote server list |
| `/api/v1/fleet/test` | POST | Test connectivity to a remote server |
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
| **Synology DSM** | ✅ Tested | `/volume<#>` detection, `/dev/mapper/cachedev_*` support, SMART health parsing |
| **TrueNAS SCALE** | ⚠️ Untested | ZFS pool health support built-in, but not yet validated on real hardware |
| **QNAP QTS** | ⚠️ Untested | Should work via Container Station |
| **Proxmox** | ⚠️ Untested | ZFS pool health support built-in |
| **Generic Linux** | ⚠️ Untested | Any distro with Docker |

> Tested on **Unraid** and **Synology DSM**. Other platforms should work but may have issues with disk detection, SMART access, or platform-specific features. [Report issues here.](https://github.com/mcdays94/nas-doctor/issues)

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

## License

MIT

---

<p align="center">
  If NAS Doctor helps you sleep better knowing your server is healthy:<br><br>
  <a href="https://buymeacoffee.com/miguelcaetanodias"><img src="https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-yellow.svg?style=flat-square&logo=buy-me-a-coffee" alt="Buy Me A Coffee"></a>
</p>
