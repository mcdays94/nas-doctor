# NAS Doctor — Agent Context

## Release Process

**CRITICAL: Always tag and release after merging to main.**

After every merge to `main` that includes code changes (not just docs):

1. Determine the version bump:
   - Patch (`v0.8.x`) for bug fixes
   - Minor (`v0.x.0`) for new features
   - Major (`vX.0.0`) for breaking changes
2. Tag: `git tag v<version> && git push origin v<version>`
3. Create release: `gh release create v<version> --title "v<version> — <summary>" --notes "<notes>"`
4. Update latest tag: `git tag -f latest && git push -f origin latest`

The Docker CI workflow on `.github/workflows/docker.yml` publishes multi-arch (amd64+arm64) images to GHCR on every push to `main` with `latest` tag, and on version tags with semver tags.

**Never push to main without tagging a version afterward.**

**Use dev branches for testing — never push untested code to prod tags.**

## Versioning

- Current: v0.8.0
- Main branch is protected — all changes go through PRs
- Docker images: `ghcr.io/mcdays94/nas-doctor:{latest,version,major.minor}`
- Multi-arch: linux/amd64, linux/arm64 (Raspberry Pi, Apple Silicon)
- RC tags (`v0.x.0-rc1`) for pre-release testing

## Architecture

- Go backend, single binary, embedded HTML templates
- Multi-stage Dockerfile with Go cross-compilation (no pre-compiled binaries)
- 3 dashboard themes: midnight (default), clean, ember — each is a self-contained HTML file
- Subpages: alerts, settings, stats, fleet, disk_detail, service_checks, parity, replacement-planner
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
- `internal/collector/` — data collection (SMART, disk, docker, network, UPS, system, parity, tunnels, proxmox, kubernetes, gpu, backup, speedtest)
- `internal/analyzer/` — diagnostic rules engine, Backblaze thresholds, Proxmox rules, K8s rules, backup staleness
- `internal/api/` — HTTP handlers, embedded templates, API key middleware
- `internal/api/styles.go` — shared CSS design system
- `internal/api/templates/` — all HTML templates (11 pages)
- `internal/scheduler/` — scan scheduling, notification rules, service checks (independent 30s loop), speed test (independent 4h loop)
- `internal/notifier/` — webhook delivery (Discord, Slack, Gotify, Ntfy, generic) + Prometheus exporter (90+ metrics)
- `internal/fleet/` — multi-server fleet polling with custom headers
- `internal/logfwd/` — log forwarding (Loki, HTTP JSON, syslog)
- `internal/storage/` — SQLite database layer
- `internal/demo/` — mock data (drives, Docker, ZFS, UPS, tunnels, Proxmox, K8s, GPU, backup, speedtest)

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
- **Service check types**: http, tcp, dns, ping, smb, nfs, speed — must be in BOTH `isSupportedServiceCheckType()` in scheduler AND the API handler validation switch in `handleUpdateSettings()`
- **Speed checks**: Use `status: "degraded"` (not just up/down) — CSS `.status-dot.degraded` and `.pill-speed` must exist

## App Store Submissions

- **Unraid CA**: Asana form submitted, docker-templates repo at mcdays94/docker-templates
- **TrueNAS**: PR #4804 at truenas/apps — open, 0 reviews (as of Apr 2026)
- **Synology**: No app catalog (Docker Compose in README)

## v0.8.0 Features (released)

1. **GPU Monitoring** — Nvidia/AMD/Intel with usage/temp/VRAM/power charts
2. **Per-Container Resource Metrics** — CPU/mem/net/block I/O per container with merged view
3. **Backup Monitoring** — Borg, Restic, PBS, Duplicati detection + stale/failed alerts
4. **Network Speed Test** — Periodic speedtest with download/upload/latency charts (4h schedule)
5. **Speed Service Check** — New check type: contracted speed vs actual with margin %, three-state (up/degraded/down)
6. **Chart Range Persistence** — 1H/1D/1W synced across all chart sections, saved to config
7. **Scroll Fade Edges** — Gradient overlays on horizontal scroll containers
8. **Section Resize** — Drag handle, heights persist to config, bottom fade on overflow
9. **Notification Rule UX** — Live preview, contextual hints, context-aware conditions per check type
10. **Test Notifications** — Context-specific test on both webhooks and notification rules

## Remaining Features (planned)

- **Export Reports** — Rework the diagnostic report to include all v0.8.0 features (GPU, backup, speed test, container metrics). Currently removed from UI.
- **ZFS Scrub Scheduling** — trigger/schedule scrubs from settings UI
- **Power Consumption Tracking** — IPMI/smart plugs, watts + monthly cost estimate

## Implementation Pattern (same for each feature)

Model (`models.go`) → Collector (`collector/<feature>.go`) → Wire (`collector.go`) → Analyzer rules → Demo data (`demo.go` + `main.go` history loop) → Prometheus gauges → Dashboard sections (3 themes + sectionMap) → Settings toggle (`api_extended.go` + `settings.html` secIds/payload) → Storage history table (if charts needed)

## Live Demo

- **URL**: https://nasdoctordemo.mdias.info
- **Architecture**: Cloudflare Worker (KV-backed) + feeder cron Worker. See `demo-worker/README.md`.
- **Source**: `demo-worker/` (demo Worker) + `demo-worker/feeder/` (data generator)
- **Deploy**: `cd demo-worker && npx wrangler deploy` (or via GH Action on release)
- **GH Action**: `.github/workflows/demo-deploy.yml` — builds Go binary, runs demo, captures pages, seeds KV, deploys both Workers
- **Platform Switcher**: `?platform=unraid|synology|truenas|proxmox|kubernetes` (cookie persisted)
- **Security**: All POST/PUT/DELETE blocked with 403; chart-range/section-heights return graceful no-ops; settings page has disabled inputs
- **Data**: Feeder cron (every 5 min) reads seed data from KV, applies time-based jitter + platform transformation, writes back. All data originated from real Go binary.
- **Auto-update**: GH Action triggers on release publish + push to main when demo-worker/ or templates change
