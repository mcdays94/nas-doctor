# NAS Doctor — Live Demo Infrastructure

**Live at: [nasdoctordemo.mdias.info](https://nasdoctordemo.mdias.info)**

The live demo serves the real NAS Doctor dashboard with automatically generated data that refreshes every 5 minutes. A platform switcher toolbar at the top lets visitors see how the dashboard looks on Unraid, Synology, Proxmox VE, and Kubernetes.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      GitHub Action                           │
│                (on release / push to main)                    │
│                                                               │
│  1. Build NAS Doctor Go binary                               │
│  2. Run in --demo mode, capture all pages + API responses    │
│  3. Seed Cloudflare KV with real data (api:* + seed:* keys)  │
│  4. Deploy demo Worker (serves pages, reads from KV)         │
│  5. Deploy feeder Worker (cron every 5 min)                  │
└──────────────────────────┬────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                     Cloudflare KV                             │
│                    "DEMO_DATA" namespace                      │
│                                                               │
│  seed:unraid:status     = { ... }   ← original Go binary     │
│  seed:unraid:snapshot   = { ... }      output, never mutated │
│  seed:unraid:sparklines = { ... }                             │
│  ...                                                          │
│                                                               │
│  api:unraid:status      = { ... }   ← refreshed by feeder    │
│  api:unraid:snapshot    = { ... }      every 5 minutes       │
│  api:unraid:sparklines  = { ... }                             │
│  ...                                                          │
└──────────┬────────────────────────────┬───────────────────────┘
           │                            │
           ▼                            ▼
┌────────────────────────┐   ┌────────────────────────────────┐
│     Demo Worker        │   │     Feeder Worker              │
│  nas-doctor-demo       │   │  nas-doctor-demo-feeder        │
│                        │   │                                │
│  • Reads from KV       │   │  • Cron: */5 * * * *           │
│  • Serves captured     │   │  • Reads seed:* keys from KV   │
│    HTML pages          │   │  • Applies time-based jitter   │
│  • Injects platform    │   │    to system metrics, SMART    │
│    switcher banner     │   │    temps, container stats      │
│  • Blocks all writes   │   │  • Shifts timestamps to now    │
│    (POST/PUT/DELETE)   │   │  • Day/night activity pattern  │
│  • Greys out settings  │   │  • Writes refreshed data to    │
│                        │   │    api:* keys in KV            │
│  ZERO hardcoded        │   │                                │
│  mock data in code     │   │  Data format = exact Go app    │
│                        │   │  output (never reinterpreted)  │
└────────────────────────┘   └────────────────────────────────┘
```

### Key design property

The demo Worker contains **zero hardcoded mock data**. Every API response it serves originated from the real NAS Doctor Go binary. The feeder Worker keeps data fresh by applying time-based noise to the seed data, but never changes the structure. This means:

- API response format is always correct (no TypeScript reimplementation drift)
- New features added to the Go app automatically appear in the demo after a release
- The dashboard JS receives data in the exact same format as a real deployment

## Components

### Demo Worker (`demo-worker/`)

- **URL**: [nasdoctordemo.mdias.info](https://nasdoctordemo.mdias.info)
- **Config**: `wrangler.toml`
- **Source**: `src/index.ts` (routing + banner injection + KV reads)
- **Static assets**: `captured/` (HTML pages in `_pages/`, plus `charts.js`, `shared.css`)
- **Platform switcher**: `src/html/banner.ts` (injected after `<body>` tag)
- **Platform detection**: `src/data/platforms.ts` (from `?platform=` query param or cookie)

### Feeder Worker (`demo-worker/feeder/`)

- **URL**: `nas-doctor-demo-feeder.mdias-info.workers.dev`
- **Config**: `feeder/wrangler.toml`
- **Source**: `feeder/src/index.ts`
- **Cron**: every 5 minutes (`*/5 * * * *`)
- **Manual trigger**: `GET /trigger` on the feeder's workers.dev URL

### KV Namespace

- **Name**: `DEMO_DATA`
- **ID**: `8a68c1992ad443de8f415f8dc6158428`
- **Key format**: `{type}:{platform}:{endpoint}`
  - `seed:unraid:snapshot` — original data from Go binary (immutable)
  - `api:unraid:snapshot` — live data served to visitors (refreshed by feeder)

### GitHub Action (`.github/workflows/demo-deploy.yml`)

Triggers on:
- Release published (new version tag)
- Push to `main` (when `demo-worker/`, templates, or the workflow itself change)
- Manual dispatch

Steps:
1. Builds the Go binary
2. Runs in `--demo` mode
3. Captures all HTML pages + API responses
4. Seeds KV with captured data (both `api:*` and `seed:*` keys)
5. Deploys demo Worker
6. Deploys feeder Worker

## Security

| Method | Behavior |
|--------|----------|
| `GET` | Allowed — reads from KV |
| `POST` | Blocked with 403 ("read-only demo") |
| `PUT` | Blocked, except `chart-range` and `section-heights` return graceful no-ops |
| `DELETE` | Blocked with 403 |

The settings page has all sensitive inputs (API key, webhooks, fleet, Proxmox/K8s credentials) greyed out with a "Read-Only" notice banner.

## Local development

```bash
# Start the demo Worker locally
cd demo-worker
npm install
npx wrangler dev --port 8787

# In another terminal, trigger the feeder manually
curl http://localhost:8787/trigger  # (if feeder is running locally)
```

## Deployment

```bash
# Deploy demo Worker
cd demo-worker
npx wrangler deploy

# Deploy feeder Worker
cd demo-worker/feeder
npm install
npx wrangler deploy
```

## Platform profiles

The platform switcher changes the `?platform=` query parameter (persisted via cookie). The demo Worker uses this to read platform-specific data from KV. Currently, all platforms fall back to the `unraid` seed data since the Go binary only generates one demo profile. Platform-specific seed data can be added by running the binary with different platform flags in the future.

| Platform | Features shown |
|----------|---------------|
| Unraid | Parity checks, disk array, cache pool, GPU, tunnels, Tailscale, UPS |
| Synology | Volume-based storage, SSD cache, package containers |
| Proxmox VE | ZFS pools, VM/LXC guests, HA groups, cluster nodes, GPU passthrough |
| Kubernetes | K3s nodes, pods, deployments, services, PVCs, Longhorn storage |
