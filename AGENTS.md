# NAS Doctor — Agent Context

## Release Process

**CRITICAL: Always tag and release after merging to main.**

After every merge to `main` that includes code changes (not just docs):

1. Determine the version bump:
   - Patch (`v0.4.x`) for bug fixes
   - Minor (`v0.x.0`) for new features
   - Major (`vX.0.0`) for breaking changes
2. Tag: `git tag v<version> && git push origin v<version>`
3. Create release: `gh release create v<version> --title "v<version> — <summary>" --notes "<notes>"`

The Docker CI workflow on `.github/workflows/docker.yml` publishes to GHCR on every push to `main` with `latest` tag, and on version tags with semver tags (`0.4.0`, `0.4`).

**Never push to main without tagging a version afterward.**

## Versioning

- Current: semver (`v0.2.0`, `v0.3.0`, `v0.3.1`, `v0.4.0`, `v0.4.1`)
- Main branch is protected — all changes go through PRs
- Docker images: `ghcr.io/mcdays94/nas-doctor:{latest,version,major.minor}`

## Architecture

- Go backend, single binary, embedded HTML templates
- 3 dashboard themes: midnight (default), clean, ember — each is a self-contained HTML file
- Subpages (alerts, settings, stats, fleet, disk_detail) share `/css/shared.css` design system
- SQLite database at `/data/nas-doctor.db`
- Charts: custom vanilla JS library at `/js/charts.js` (no dependencies)

## Platform Support

- **Tested**: Unraid, Synology DSM
- **Untested**: TrueNAS SCALE, QNAP, Proxmox, generic Linux
- The app must be platform-aware: detect the OS and adapt behavior (disk paths, SMART parsing, network interfaces, volume mounts)
- Synology uses `/volume<#>` for data, `/dev/mapper/cachedev_*` devices
- Unraid uses `/mnt/disk<#>`, `/mnt/cache`, md arrays

## Key Files

- `internal/collector/platform.go` — **centralized platform detection singleton** (Unraid, Synology, TrueNAS, QNAP, Proxmox, Linux). Detected once, cached for process lifetime. All collectors use `GetPlatform().IsUnraid()` etc.
- `internal/collector/` — data collection (SMART, disk, docker, network, UPS, system, parity)
- `internal/analyzer/` — diagnostic rules engine, Backblaze thresholds
- `internal/api/` — HTTP handlers, embedded templates, chart library
- `internal/api/styles.go` — shared CSS design system
- `internal/api/templates/` — all HTML templates
- `internal/scheduler/` — scan scheduling, alert lifecycle, notification policies
- `internal/notifier/` — webhook delivery (Discord, Slack, Gotify, Ntfy, generic)
