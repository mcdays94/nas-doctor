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

## Current Work-in-Progress

**Branch**: `dev/new-features` (at `55c8ed8`, based on `dev/predictive-intelligence`)

### Completed
1. **GPU Monitoring** — full stack implementation:
   - Collector: Nvidia (`nvidia-smi`), AMD (`rocm-smi` + sysfs), Intel (i915/xe sysfs)
   - Model: `GPUInfo`/`GPUDevice` structs, `CategoryGPU`, `Snapshot.GPU` field
   - Analyzer: temperature (>85/95°C), VRAM exhaustion (>95%), power limit rules
   - Storage: `gpu_history` table with per-GPU time-series metrics
   - API: `GET /api/v1/history/gpu?hours=N` endpoint (1/24/168)
   - Dashboard: GPU section in all 3 themes with area charts and 1H/1D/1W toggle buttons
   - Prometheus: 10 GPU gauges (usage, temp, VRAM, power, fan, encoder, decoder)
   - Settings: GPU section toggle in dashboard sections
   - Demo: RTX 4060 + Intel UHD 730 mock data with 48h hourly history

### Remaining Features (in order)
2. **Per-Container Resource Metrics** — CPU/mem/net per Docker container with history charts (like Beszel)
3. **Backup Monitoring** — detect PBS, Borg, Restic, Duplicati; track last successful backup
4. **Network Speed Test History** — periodic speedtest with graphs
5. **ZFS Scrub Scheduling** — trigger/schedule scrubs from settings UI
6. **Power Consumption Tracking** — IPMI/smart plugs, watts + monthly cost estimate

### Implementation Pattern (same for each feature)
Model (`models.go`) → Collector (`collector/<feature>.go`) → Wire (`collector.go`) → Analyzer rules → Demo data (`demo.go` + `main.go` history loop) → Prometheus gauges → Dashboard sections (3 themes + sectionMap) → Settings toggle (`api_extended.go` + `settings.html` secIds/payload) → Storage history table (if charts needed)

### Also on this branch (pre-existing)
- Nav bar standardization fix (commit `4a0b832` on `dev/predictive-intelligence`, carried forward)
