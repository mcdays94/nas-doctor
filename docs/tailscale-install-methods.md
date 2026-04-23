# Tailscale detection: install methods, coverage, and bind-mounts

NAS Doctor detects Tailscale in two ways: the host-installed `tailscale`
CLI talking to a running `tailscaled` daemon, and known-named Docker
containers. The quality of the peer graph you see on the dashboard
depends entirely on which install method is in play and whether the
control socket at `/var/run/tailscale/tailscaled.sock` is reachable
from inside the NAS Doctor container.

This doc is the canonical reference when the dashboard surfaces an
`Unreachable` or `Stopped` state chip, or when Tailscale detection
disappears unexpectedly. The short version: **mount
`/var/run/tailscale` read-only from the host into the NAS Doctor
container, or use `network_mode: host`**.

## Coverage matrix

| Install method                                                      | Host-binary path                                       | Docker-container path                          | Peer graph fidelity                     | Bind-mount needed                 | Works today?                       |
| ------------------------------------------------------------------- | ------------------------------------------------------ | ---------------------------------------------- | --------------------------------------- | --------------------------------- | ---------------------------------- |
| **Unraid plugin** (`tailscale-nas-util`)                            | bundled CLI + host socket                              | n/a                                            | Full (JSON) when socket is mounted      | `/var/run/tailscale`              | Yes, when socket is mounted        |
| **Docker container, `network_mode: host`**                          | yes, via mounted host socket                           | yes, container-only (no peer data) as fallback | Full (JSON) via mount                   | `/var/run/tailscale`              | Yes, when socket is mounted        |
| **Docker container, own network namespace**                         | socket not accessible                                  | yes, container-only detection                  | None (container presence only)          | n/a (socket not shareable easily) | Partial — no peer graph            |
| **TrueNAS / Synology / Proxmox / plain-Linux host install**         | yes, when socket mount is in the NAS Doctor container  | n/a                                            | Full (JSON) when socket is mounted      | `/var/run/tailscale`              | Yes, when socket is mounted        |
| **Kubernetes, sidecar pattern sharing `/var/run/tailscale` via `emptyDir`** | yes, if `emptyDir` is configured and the volume is mounted into NAS Doctor's pod | no Docker socket in k8s                        | Full in sidecar when emptyDir is shared | `emptyDir` shared between pods    | Yes, advanced — see example below  |
| **Kubernetes, Tailscale in a separate pod**                         | no (socket isolated per pod)                           | no                                             | None                                    | n/a                               | Not supported                      |

## Why the socket matters

`tailscaled` exposes its control API over a Unix domain socket. The
`tailscale status --json` command — which NAS Doctor uses to build
the peer graph — is a thin client on top of that socket. Without the
socket mount, the CLI runs but returns

```
failed to connect to local tailscaled
```

NAS Doctor catches this and sets `BackendState=Unreachable` with a
hint field that names the expected socket path (overridable with
`NAS_DOCTOR_TAILSCALE_SOCKET`). The dashboard renders the hint
verbatim so you know what to fix.

## Version skew (CLI vs daemon)

When the container's `tailscale` CLI is older than the host's
`tailscaled` (common on Alpine: the packaged CLI lags the host by a
minor version or two), `tailscale status --json` will sometimes
return zero bytes with no error — no data, no diagnostic. NAS Doctor
falls back to parsing the tabular `tailscale status` output, which
is version-stable but gives a **reduced field set**:

- Captured by the fallback: IP, hostname, owner, OS, online/offline
  derived from the `LastSeen` column
- Lost in the fallback: TX/RX bytes, relay region, DNS name, MagicDNS
  suffix, exit-node flag, tags

When the fallback fires, the `Hint` field explains the version skew
and recommends upgrading the bundled `tailscale` binary to match the
host.

## Opt-in: custom-named Tailscale containers

By default, Docker-container detection matches the substring
`tailscale` in the container name or image. If you run Tailscale as a
sidecar under a different name — `ts-sidecar`, `mullvad-tailscale-alt`,
`vpn` — set:

```bash
NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES=ts-sidecar,mullvad-ts,vpn
```

Semantics:

- comma-separated
- case-insensitive
- substring match against **both** container name and image
- whitespace around each token is trimmed, empty tokens are dropped
- OR-combined with the default `tailscale` substring rule — so
  turning this on cannot break existing `tailscale/tailscale` detection

## Bind-mount recipes

### Docker Compose

```yaml
services:
  nas-doctor:
    # ...
    volumes:
      - /var/run/tailscale:/var/run/tailscale:ro
```

### Unraid

Fill in the path mapping labelled **Tailscale Socket** in the
NAS Doctor CA template. Host path: `/var/run/tailscale`. Container
path: `/var/run/tailscale`. Mode: `RO`.

In v0.9.8+ this field is promoted to always-visible so new installs
don't miss it. Earlier versions hid it behind `Display="advanced"`;
if you install an older template and don't see the field, toggle
"Advanced View" in the Docker UI.

### Kubernetes — Tailscale sidecar sharing the socket via `emptyDir`

Deploy `tailscaled` as a sidecar in the same pod and share
`/var/run/tailscale` via an `emptyDir` volume mounted into both
containers:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nas-doctor
spec:
  volumes:
    - name: tailscale-socket
      emptyDir: {}
    # ... your other volumes ...
  containers:
    - name: tailscaled
      image: tailscale/tailscale:stable
      env:
        - name: TS_KUBE_SECRET
          value: tailscale-state
        - name: TS_USERSPACE
          value: "false"
      securityContext:
        capabilities:
          add: ["NET_ADMIN"]
      volumeMounts:
        - name: tailscale-socket
          mountPath: /var/run/tailscale
    - name: nas-doctor
      image: ghcr.io/mcdays94/nas-doctor:latest
      ports:
        - containerPort: 8060
      volumeMounts:
        - name: tailscale-socket
          mountPath: /var/run/tailscale
          readOnly: true
```

Notes:

- `emptyDir: {}` means the volume is pod-lifetime; the socket only
  exists while both containers are running
- Read-only mount on the NAS Doctor side is sufficient — the CLI
  only needs read access to query status
- If your cluster's `tailscale/tailscale` image wraps `tailscaled` in
  `userspace-networking` mode, `/var/run/tailscale/tailscaled.sock`
  is still created at the standard path, so the bind-mount works
  the same way

### Tailscale in a separate pod

Not supported. Each pod has its own filesystem namespace; the socket
in the Tailscale pod is not addressable from the NAS Doctor pod
without using a shared `hostPath` (which defeats the isolation k8s
gives you) or the Kubernetes API (which NAS Doctor doesn't use for
Tailscale). Run the sidecar pattern above instead.

## Unraid quick path

1. Install the `tailscale-nas-util` plugin from Community Applications
2. Log the plugin in to your tailnet
3. Install NAS Doctor from CA — the default template mounts
   `/var/run/tailscale` for you
4. Open the NAS Doctor dashboard; the Tunnels section shows your
   tailnet with self + peers and state dots

If step 4 shows an `Unreachable` chip instead of peer data, the
bind-mount is missing. Edit the container, confirm the Tailscale
Socket path is set to `/var/run/tailscale`, and apply.

## Related

- Issue [#243](https://github.com/mcdays94/nas-doctor/issues/243) —
  the tracker that produced this document; also expanded the collector
  heuristic and reworked the dashboard render paths
- Issue [#244](https://github.com/mcdays94/nas-doctor/issues/244) —
  broader README claims-audit tracker
- Collector code: `internal/collector/tunnels.go`
- Dashboard rendering: `internal/api/dashboard.go` (`sections.tunnels`)
