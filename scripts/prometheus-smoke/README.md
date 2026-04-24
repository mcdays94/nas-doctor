# NAS Doctor — Prometheus smoke-test harness

A minimal, self-contained docker-compose stack that spins up **Prometheus +
Grafana** locally and scrapes a NAS Doctor `/metrics` endpoint. Intended for
end-to-end validation of the Prometheus exporter against UAT (or a local NAS
Doctor instance), per issue [#255](https://github.com/mcdays94/nas-doctor/issues/255)
Phase 1.

**This is a discovery harness, not production monitoring.** Any anomalies
found (missing `# HELP`/`# TYPE` lines, inconsistent labels, slow scrapes,
high-cardinality series) should be filed as separate issues.

## Quick start

```bash
cd scripts/prometheus-smoke
cp .env.example .env
# Edit .env and fill in ND_API_KEY, CF_ACCESS_CLIENT_ID, CF_ACCESS_CLIENT_SECRET
docker compose up -d

# Wait ~60 seconds for the first two scrape cycles, then:
open http://localhost:9090/targets        # Prometheus — NAS Doctor should be UP
open http://localhost:3000                 # Grafana — admin/admin, dashboard in "NAS Doctor" folder
```

Tear down: `docker compose down -v` (the `-v` also wipes Prometheus TSDB +
Grafana DB, which is what you want between runs).

## What runs

| Service | Purpose |
|---|---|
| `prometheus-config-init` | One-shot Alpine container. Runs `envsubst` on `prometheus.yml.tmpl` → `prometheus.yml` using values from `.env`. Runs before Prometheus starts; exits 0 on success. |
| `prometheus` | prom/prometheus v3.x. Scrapes the configured NAS Doctor target every 30s plus itself. UI on `:9090`. |
| `grafana` | grafana/grafana v11.x. Auto-provisions the Prometheus datasource and the overview dashboard on first boot. UI on `:3000`. Default creds `admin/admin`; anonymous Viewer access also enabled for quick poking. |

## Configuration (via `.env`)

| Var | Purpose | Example |
|---|---|---|
| `SCRAPE_TARGET` | NAS Doctor host:port (no scheme). Defaults to UAT. | `nasdoctoruat.mdias.info`, `192.168.1.10:8060` |
| `SCRAPE_SCHEME` | `http` or `https`. | `https` |
| `ND_API_KEY` | Bearer token from NAS Doctor Settings → API. | `nd-…` |
| `CF_ACCESS_CLIENT_ID` | CF Access service-token client ID. Leave blank for local instances not behind CF Access. | `xxx.access` |
| `CF_ACCESS_CLIENT_SECRET` | CF Access service-token client secret. Leave blank for local. | `…` |

Credentials are read from `.env` only. The rendered `prometheus.yml` ends up
inside a docker volume (`prom-config`), never on the host. **`.env` is
gitignored — never commit it.**

## Scraping a local NAS Doctor instance

```bash
# Example: a local dev instance running on the host on port 8060
cat > .env <<EOF
SCRAPE_TARGET=host.docker.internal:8060
SCRAPE_SCHEME=http
ND_API_KEY=nd-your-local-api-key
CF_ACCESS_CLIENT_ID=
CF_ACCESS_CLIENT_SECRET=
EOF
docker compose up -d
```

(Linux users who don't have `host.docker.internal` resolved by default may
need to add `extra_hosts: ["host.docker.internal:host-gateway"]` to the
prometheus service, or use the LAN IP of the docker host.)

## Verifying the scrape

After `docker compose up -d`, wait ~60s (two scrape intervals), then:

```bash
# Target status
curl -s http://localhost:9090/api/v1/targets \
  | jq '.data.activeTargets[] | {job: .labels.job, health, lastScrape, lastScrapeDuration}'

# Full list of discovered metric names
curl -s http://localhost:9090/api/v1/label/__name__/values \
  | jq -r '.data[]' | grep ^nasdoctor_ | head -40

# Specific metric — should return samples with instance label
curl -s 'http://localhost:9090/api/v1/query?query=nasdoctor_system_cpu_usage_percent' | jq

# Prometheus's own warnings from the scrape
docker compose logs prometheus 2>&1 | grep -iE 'warn|error'
```

## What to look for (Phase 1 checklist)

- [ ] `/targets` shows `nas-doctor` job `health: "up"`
- [ ] `lastScrapeDuration` under 500ms (NAS Doctor's budget)
- [ ] `curl .../label/__name__/values` returns ~110+ `nasdoctor_*` metric names
- [ ] `docker compose logs prometheus` contains no parse warnings
- [ ] Grafana dashboard "NAS Doctor — Overview" renders panels for every category
- [ ] Pick one sample metric (e.g., `nasdoctor_smart_temperature_celsius`) and
      confirm the `device` label is consistently present across scrapes

Any negative result → file a separate issue. Do **not** modify NAS Doctor itself
in this harness.

## Cleanup

```bash
docker compose down -v
rm -f .env   # if you're done
```
