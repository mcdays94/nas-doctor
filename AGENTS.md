# NAS Doctor — Agent Context

## Release Process

**CRITICAL: Always tag and release after merging to main.**

After every merge to `main` that includes code changes (not just docs):

1. Determine the version bump:
   - Patch (`v0.7.x`) for bug fixes
   - Minor (`v0.x.0`) for new features
   - Major (`vX.0.0`) for breaking changes
2. Tag: `git tag v<version> && git push origin v<version>`
3. Create release: `gh release create v<version> --title "v<version> — <summary>" --notes "<notes>"`
4. Update latest tag: `git tag -f latest && git push -f origin latest`

The Docker CI workflow on `.github/workflows/docker.yml` publishes multi-arch (amd64+arm64) images to GHCR on every push to `main` with `latest` tag, and on version tags with semver tags.

**Never push to main without tagging a version afterward.**

**Use dev branches for testing — never push untested code to prod tags.**

## Versioning

- Current: v0.7.0
- Main branch is protected — all changes go through PRs
- Docker images: `ghcr.io/mcdays94/nas-doctor:{latest,version,major.minor}`
- Multi-arch: linux/amd64, linux/arm64 (Raspberry Pi, Apple Silicon)
- RC tags (`v0.x.0-rc1`) for pre-release testing

## Architecture

- Go backend, single binary, embedded HTML templates
- Multi-stage Dockerfile with Go cross-compilation (no pre-compiled binaries)
- 3 dashboard themes: midnight (default), clean, ember — each is a self-contained HTML file
- Subpages: alerts, settings, stats, fleet, disk_detail, service_checks, parity
- All subpages share `/css/shared.css` design system
- SQLite database at `/data/nas-doctor.db`
- Charts: custom vanilla JS library at `/js/charts.js` (no dependencies)
- API key authentication: all `/api/v1/*` except `/health` protected when key is set
- `/api/v1/health` is always public (Docker HEALTHCHECK, K8s probes, load balancers)

## Platform Support

- **Tested**: Unraid, Synology DSM (community), Proxmox (VM), Kubernetes (k3s)
- **Untested**: TrueNAS SCALE, QNAP, generic Linux
- The app must be platform-aware: detect the OS and adapt behavior
- Synology: `/volume<#>` for data, `/dev/mapper/cachedev_*` devices
- Unraid: `/mnt/disk<#>`, `/mnt/cache`, md arrays
- Proxmox: PVE REST API integration (nodes, VMs, LXCs, storage, HA, tasks)
- Kubernetes: K8s API integration (nodes, pods, deployments, services, PVCs, events)

## Key Files

- `internal/collector/platform.go` — centralized platform detection singleton
- `internal/collector/` — data collection (SMART, disk, docker, network, UPS, system, parity, tunnels, proxmox, kubernetes)
- `internal/analyzer/` — diagnostic rules engine, Backblaze thresholds, Proxmox rules, K8s rules
- `internal/api/` — HTTP handlers, embedded templates, API key middleware
- `internal/api/styles.go` — shared CSS design system
- `internal/api/templates/` — all HTML templates (10 pages)
- `internal/scheduler/` — scan scheduling, notification rules, service checks (independent 30s loop)
- `internal/notifier/` — webhook delivery (Discord, Slack, Gotify, Ntfy, generic) + Prometheus exporter (90+ metrics)
- `internal/fleet/` — multi-server fleet polling with custom headers
- `internal/logfwd/` — log forwarding (Loki, HTTP JSON, syslog)
- `internal/storage/` — SQLite database layer
- `internal/demo/` — mock data (drives, Docker, ZFS, UPS, tunnels, Proxmox, K8s)

## Integrations

- **Proxmox VE**: REST API collector, settings UI with test connection + node auto-detect + alias
- **Kubernetes**: API collector (in-cluster + external), nodes/pods/deployments/services/PVCs/events
- **Cloudflared**: Tunnel detection (host + Docker), status, connections, routes
- **Tailscale**: Peer graph, online status, TX/RX, exit nodes
- **Fleet**: Multi-instance monitoring with custom auth headers, NAS Doctor signature validation
- **Prometheus**: 90+ gauges covering all subsystems
- **Log Forwarding**: Loki push, HTTP JSON, syslog (RFC 5424)

## Important Patterns

- **Never use `lsof -ti:PORT | xargs kill`** — it kills the user's browser. Use `pkill -f "nas-doctor"` instead
- **Fleet servers persist via settings DB** — `buildSettingsPayload()` must use live `fleetServers` variable, not stale `base.fleet`
- **Section toggles**: Must be in `sectionMap` in all 3 themes AND in `secIds` in settings.html
- **Auto-enable sections**: When an integration is enabled (Proxmox, K8s), auto-set the section toggle to true
- **Settings load on startup**: Proxmox + K8s configs must be applied to collector at startup from persisted settings
- **Orphaned checks**: Match by target URL too, not just name (fleet auto-created checks have different names)

## App Store Submissions

- **Unraid CA**: Asana form submitted, docker-templates repo at mcdays94/docker-templates
- **TrueNAS**: PR #4804 at truenas/apps
- **Synology**: No app catalog (Docker Compose in README)

## Roadmap — Planned Features

### v0.8.0 — Predictive Intelligence (branch: `dev/predictive-intelligence`)

**1. Drive Replacement Planner** (new subpage: `/replacement-planner`)
- Predict failure windows per drive using Backblaze annualized failure data + current SMART health score
- Sort drives by urgency: "replace soon" → "monitor" → "healthy"
- Show estimated remaining life (already computed in stats.html, move to backend)
- Estimated replacement cost: user enters $/TB in settings, calculate per drive
- Fleet-aware: show replacement needs across all fleet instances
- Data source: existing SMART data (power_on_hours, reallocated_sectors, pending_sectors, temperature_c, health_passed)
- UI: table with drive name, health score, estimated remaining life, risk tier, cost estimate
- Implementation:
  - `internal/analyzer/replacement.go` — replacement urgency scoring, cost estimation
  - `internal/api/templates/replacement.html` — new subpage
  - `internal/api/replacement_page.go` — handler
  - Settings: add `replacement_cost_per_tb` field (float, default 0 = hidden)

**2. Storage Capacity Forecasting** (on existing stats page + dashboard)
- Track disk usage over time (new sparkline: usage_percent per mount point per snapshot)
- Linear regression on usage history to project days until 90%/95%/100%
- Show forecast on stats page as a "Capacity Forecast" section
- Alert rule: "Volume X will be full in N days" (configurable threshold)
- Data changes needed:
  - `internal/storage/db.go` — new table `disk_usage_history(id, timestamp, mount_point, label, used_percent, used_gb, total_gb)`
  - `internal/scheduler/` — on each scan, persist disk usage snapshot to history table
  - `internal/storage/db.go` — `GetDiskUsageHistory(mountPoint, limit)` query
  - `internal/api/` — new endpoint `GET /api/v1/disk-usage-history`
- Regression: simple least-squares in Go, no dependencies needed
- UI: chart per volume showing usage trend + projected line to 100%

**3. Anomaly Detection / Trend Alerting** (enhancement to existing alerting)
- Compute rolling 7-day baseline for CPU, memory, I/O wait, load average
- Alert when current value deviates significantly from baseline (configurable: 2x, 3x std dev)
- Uses existing system sparkline history data (already 30+ days stored)
- Implementation:
  - `internal/analyzer/anomaly.go` — baseline computation, deviation detection
  - New finding severity: "anomaly" (rendered same as "warning" but different category)
  - Settings: enable/disable anomaly detection, sensitivity (low/medium/high)

**4. Power & Cost Monitoring** (enhancement to UPS section + fleet)
- Read wattage from existing UPS data (`wattage_watts`, `load_percent`, `nominal_watts`)
- User enters electricity cost in settings: `electricity_cost_kwh` (float, e.g., 0.12)
- Calculate: monthly cost = watts × 24 × 30.44 / 1000 × $/kWh
- Show in dashboard UPS section: "Estimated monthly cost: $XX"
- Show in fleet view: per-server power cost column
- No external API needed — user enters their rate manually

**5. Docker Container Resource Tracking** (enhancement to Docker section)
- Collect per-container CPU%, memory usage, network I/O, restart count
- Uses Docker API stats endpoint (already have Docker client in collector)
- Alert on: crash loops (restart count > N in M minutes), excessive CPU/memory
- Show in dashboard: mini resource bars per container, sortable

**6. Incident Timeline** (new subpage: `/incidents`)
- Correlate findings by timestamp proximity across subsystems
- Group events within a 5-minute window into an "incident"
- Show timeline: temp spike → I/O wait → container crash → parity fail
- Data source: existing findings with `detected_at` timestamps
- Pure frontend rendering from existing findings data — no new storage needed

### Later Features (post v0.8.0)

**Public Status Page** (optional, default off)
- Shareable `/status/public` page, no auth required
- User selects which service checks are public in settings
- Shows uptime bars (24h, 7d, 30d) per check
- Clean minimal design, works as standalone page
- Setting: `status_page_enabled` (bool, default false), `status_page_checks` (list of check IDs)

**Backup Monitoring**
- File-age monitoring: user configures backup paths + max age thresholds
- Check most recent modification time in configured directories
- Alert when newest file is older than threshold ("Backup overdue")
- Works with any backup tool (Hyper Backup, Borg, Restic, rsync, rclone)
- Settings: list of `{name, path, max_age_hours}`

**Mobile PWA**
- Add `manifest.json` for PWA install-to-homescreen
- Optimize dashboard layout for narrow viewports
- Pull-to-refresh, large touch targets

**Plugin / Extension System**
- Drop-in shell/Go scripts in `/data/plugins/` directory
- NAS Doctor runs them on each scan, ingests JSON output
- Standard schema: `{metrics: [...], findings: [...]}`
