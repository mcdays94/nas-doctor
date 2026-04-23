package collector

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

// Package-level indirections so tests can stub out exec / filesystem access.
// They default to the real implementations; tests swap them with t.Cleanup.
var (
	tailscaleLookPath   = exec.LookPath
	tailscaleRunCommand = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
	tailscaleSocketStat = os.Stat
	// tailscaleSocketPath is the expected tailscaled control socket. Can be
	// overridden with NAS_DOCTOR_TAILSCALE_SOCKET for non-default paths.
	tailscaleSocketPath = func() string {
		if p := os.Getenv("NAS_DOCTOR_TAILSCALE_SOCKET"); p != "" {
			return p
		}
		return "/var/run/tailscale/tailscaled.sock"
	}()
)

// tailscaleCustomContainerPatterns parses the opt-in env var
// NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES into a lowercase substring
// list used to widen container-name detection. Semantics:
//
//   - comma-separated
//   - case-insensitive (stored lowercase for fast matching)
//   - whitespace trimmed around each token
//   - empty tokens dropped
//   - returns nil (not an empty slice) when the env var is unset or
//     contains only separators/whitespace
//
// Parsed on every call because scheduler-driven collection is already
// cheap and this avoids a package-init-time env read that would miss
// late-set env vars in tests.
func tailscaleCustomContainerPatterns() []string {
	raw := os.Getenv("NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES")
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		tok := strings.ToLower(strings.TrimSpace(part))
		if tok == "" {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// collectTunnels detects cloudflared and tailscale tunnel services.
// It checks both host-installed binaries and Docker containers.
func collectTunnels(docker internal.DockerInfo) *internal.TunnelInfo {
	cf := collectCloudflared(docker)
	ts := collectTailscale(docker)
	if cf == nil && ts == nil {
		return nil
	}
	return &internal.TunnelInfo{
		Cloudflared: cf,
		Tailscale:   ts,
	}
}

// ── Cloudflared ──

func collectCloudflared(docker internal.DockerInfo) *internal.CloudflaredInfo {
	info := &internal.CloudflaredInfo{}

	// 1. Check host binary
	if path, err := exec.LookPath("cloudflared"); err == nil && path != "" {
		info.Installed = true
		if out, err := exec.Command("cloudflared", "--version").CombinedOutput(); err == nil {
			info.Version = parseCloudflaredVersion(string(out))
		}
		// Try to list tunnels via CLI (requires login)
		if out, err := exec.Command("cloudflared", "tunnel", "list", "--output", "json").CombinedOutput(); err == nil {
			info.Tunnels = parseCloudflaredTunnelList(out)
		}
	}

	// 2. Check Docker containers (image contains "cloudflare" or "cloudflared")
	for _, c := range docker.Containers {
		img := strings.ToLower(c.Image)
		name := strings.ToLower(c.Name)
		if !strings.Contains(img, "cloudflare") && !strings.Contains(name, "cloudflare") {
			continue
		}
		info.Installed = true
		if info.Version == "" {
			info.Version = "(docker: " + c.Image + ")"
		}
		status := "down"
		if strings.ToLower(c.State) == "running" {
			status = "healthy"
		}
		// Check if this container is already captured as a CLI tunnel
		alreadyCaptured := false
		for _, t := range info.Tunnels {
			if strings.EqualFold(t.Name, c.Name) {
				alreadyCaptured = true
				break
			}
		}
		if !alreadyCaptured {
			info.Tunnels = append(info.Tunnels, internal.CloudflaredTunnel{
				ID:          c.ID,
				Name:        c.Name,
				Status:      status,
				Connections: boolToInt(status == "healthy"),
			})
		}
	}

	if !info.Installed {
		return nil
	}
	return info
}

func parseCloudflaredVersion(raw string) string {
	// "cloudflared version 2024.6.1 (built 2024-06-15-...)"
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "version "); idx >= 0 {
		ver := raw[idx+8:]
		if sp := strings.IndexByte(ver, ' '); sp > 0 {
			return ver[:sp]
		}
		return strings.TrimSpace(ver)
	}
	return strings.Split(raw, "\n")[0]
}

func parseCloudflaredTunnelList(data []byte) []internal.CloudflaredTunnel {
	// cloudflared tunnel list --output json returns an array of objects
	var raw []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		CreatedAt   string `json:"created_at"`
		Connections []struct {
			OriginIP string `json:"origin_ip"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	tunnels := make([]internal.CloudflaredTunnel, 0, len(raw))
	for _, r := range raw {
		status := "inactive"
		if len(r.Connections) > 0 {
			status = "healthy"
		}
		originIP := ""
		if len(r.Connections) > 0 {
			originIP = r.Connections[0].OriginIP
		}
		tunnels = append(tunnels, internal.CloudflaredTunnel{
			ID:          r.ID,
			Name:        r.Name,
			Status:      status,
			CreatedAt:   r.CreatedAt,
			Connections: len(r.Connections),
			OriginIP:    originIP,
		})
	}
	return tunnels
}

// ── Tailscale ──

func collectTailscale(docker internal.DockerInfo) *internal.TailscaleInfo {
	info := &internal.TailscaleInfo{}

	// 1. Check host binary
	if path, err := tailscaleLookPath("tailscale"); err == nil && path != "" {
		info.Installed = true
		if out, err := tailscaleRunCommand("tailscale", "version"); err == nil {
			info.Version = strings.TrimSpace(strings.Split(string(out), "\n")[0])
		}
		// Prefer the richer JSON output, but fall back to parsing the plain
		// tabular `tailscale status` output when JSON is unavailable. The
		// common trigger: the bundled Alpine tailscale CLI is older than the
		// host tailscaled daemon, which makes `--json` return empty bytes
		// (no error, no JSON) — e.g. Alpine 3.21 ships v1.76.6 against a
		// host running v1.96+. The plain-text format is version-stable and
		// still gives us IPs, hostnames, online state, and OS.
		jsonPopulatedPeers := false
		if out, err := tailscaleRunCommand("tailscale", "status", "--json"); err == nil && len(strings.TrimSpace(string(out))) > 0 {
			parseTailscaleStatus(out, info)
			jsonPopulatedPeers = info.Self != nil || len(info.Peers) > 0
		}
		if !jsonPopulatedPeers {
			// Try plain-text format. No --json and no extra args.
			if out, err := tailscaleRunCommand("tailscale", "status"); err == nil && len(strings.TrimSpace(string(out))) > 0 {
				if parsePlainStatus(string(out), info) {
					if info.BackendState == "" {
						info.BackendState = "Running"
					}
					// If --json returned empty, note the version skew once so
					// the UI can nudge the user to upgrade when convenient.
					if info.Hint == "" {
						info.Hint = "Using plain-text `tailscale status` parser because `--json` returned no output — likely a version skew between the container's tailscale CLI and the host tailscaled. Peer details are limited; upgrade the bundled tailscale binary to match the host for richer data."
					}
				}
			} else {
				// Daemon unreachable — typical Unraid case: the host runs the
				// tailscale-nas-util plugin but /var/run/tailscale is not
				// bind-mounted into the container. Surface a hint so the UI can
				// guide the user instead of silently showing "not installed".
				info.BackendState = "Unreachable"
				if _, statErr := tailscaleSocketStat(tailscaleSocketPath); os.IsNotExist(statErr) {
					info.Hint = "tailscale binary found but daemon socket " + tailscaleSocketPath +
						" is not accessible. On Unraid, bind-mount /var/run/tailscale from the host " +
						"(see the NAS Doctor Unraid template)."
				} else {
					info.Hint = "tailscale binary found but `tailscale status` failed. " +
						"Verify the daemon is running and the socket at " + tailscaleSocketPath +
						" is reachable."
				}
			}
		}
	}

	// 2. Check Docker containers. Default heuristic: image or container
	// name contains "tailscale". Users running a sidecar with a
	// non-obvious name (ts-sidecar, mullvad-tailscale-alt, vpn) can
	// opt-in additional substring patterns via the env var
	// NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES (comma-separated,
	// case-insensitive, matches BOTH name and image). See
	// docs/tailscale-install-methods.md.
	customPatterns := tailscaleCustomContainerPatterns()
	for _, c := range docker.Containers {
		img := strings.ToLower(c.Image)
		name := strings.ToLower(c.Name)
		matched := strings.Contains(img, "tailscale") || strings.Contains(name, "tailscale")
		if !matched {
			for _, pat := range customPatterns {
				if strings.Contains(img, pat) || strings.Contains(name, pat) {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		info.Installed = true
		if info.Version == "" {
			info.Version = "(docker: " + c.Image + ")"
		}
		if info.BackendState == "" {
			if strings.ToLower(c.State) == "running" {
				info.BackendState = "Running"
			} else {
				info.BackendState = "Stopped"
			}
		}
		// If no peers from CLI, add self as a node from docker info
		if info.Self == nil {
			info.Self = &internal.TailscaleNode{
				Name:   c.Name,
				Online: strings.ToLower(c.State) == "running",
				OS:     "docker",
			}
		}
	}

	if !info.Installed {
		return nil
	}
	return info
}

func parseTailscaleStatus(data []byte, info *internal.TailscaleInfo) {
	var status struct {
		BackendState string `json:"BackendState"`
		Self         struct {
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			OS           string   `json:"OS"`
			Online       bool     `json:"Online"`
			Relay        string   `json:"Relay"`
			HostName     string   `json:"HostName"`
			Tags         []string `json:"Tags"`
			TxBytes      int64    `json:"TxBytes"`
			RxBytes      int64    `json:"RxBytes"`
		} `json:"Self"`
		Peer map[string]struct {
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			OS           string   `json:"OS"`
			Online       bool     `json:"Online"`
			ExitNode     bool     `json:"ExitNode"`
			Relay        string   `json:"Relay"`
			HostName     string   `json:"HostName"`
			Tags         []string `json:"Tags"`
			TxBytes      int64    `json:"TxBytes"`
			RxBytes      int64    `json:"RxBytes"`
			LastSeen     string   `json:"LastSeen"`
		} `json:"Peer"`
		MagicDNSSuffix string `json:"MagicDNSSuffix"`
		CurrentTailnet struct {
			Name string `json:"Name"`
		} `json:"CurrentTailnet"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return
	}

	info.BackendState = status.BackendState
	info.MagicDNS = status.MagicDNSSuffix != ""
	info.TailnetName = status.CurrentTailnet.Name

	ip := ""
	if len(status.Self.TailscaleIPs) > 0 {
		ip = status.Self.TailscaleIPs[0]
	}
	info.Self = &internal.TailscaleNode{
		Name:    status.Self.HostName,
		DNSName: status.Self.DNSName,
		IP:      ip,
		OS:      status.Self.OS,
		Online:  true,
		Relay:   status.Self.Relay,
		TxBytes: status.Self.TxBytes,
		RxBytes: status.Self.RxBytes,
		Tags:    status.Self.Tags,
	}

	for _, peer := range status.Peer {
		peerIP := ""
		if len(peer.TailscaleIPs) > 0 {
			peerIP = peer.TailscaleIPs[0]
		}
		info.Peers = append(info.Peers, internal.TailscaleNode{
			Name:     peer.HostName,
			DNSName:  peer.DNSName,
			IP:       peerIP,
			OS:       peer.OS,
			Online:   peer.Online,
			ExitNode: peer.ExitNode,
			Relay:    peer.Relay,
			TxBytes:  peer.TxBytes,
			RxBytes:  peer.RxBytes,
			LastSeen: peer.LastSeen,
			Tags:     peer.Tags,
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// parsePlainStatus parses the tabular output of `tailscale status` (no --json).
// Format (one device per line, whitespace-separated):
//
//	<TailscaleIP> <HostName> <Owner> <OS> <LastSeenOrDash>
//
// The FIRST row (where LastSeen is "-") is always Self. Subsequent rows are peers.
// A "-" in the LastSeen column means the device is currently online. Any other
// value (e.g. "2h22m", "Dec 12 2024") is the last-seen timestamp and means the
// peer is offline.
//
// Emitted fields are limited compared to parseTailscaleStatus: we don't get
// TxBytes, RxBytes, Tags, Relay info, DNS name, or MagicDNS. Callers should
// prefer `--json` output when it's available.
//
// Returns true if at least one row was parsed (Self was set).
func parsePlainStatus(output string, info *internal.TailscaleInfo) bool {
	first := true
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// Skip stderr warnings that may have been folded in (e.g. "Warning:
		// client version ...") — these don't have the "IP hostname owner OS"
		// shape.
		if strings.HasPrefix(strings.ToLower(line), "warning") ||
			strings.HasPrefix(strings.ToLower(line), "health") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip, hostname, _owner, opsys := fields[0], fields[1], fields[2], fields[3]
		_ = _owner // unused; keeps layout obvious
		// Sanity: the first field must look like an IP (contains a dot or colon).
		if !strings.ContainsAny(ip, ".:") {
			continue
		}
		online := true
		if len(fields) >= 5 && fields[4] != "-" {
			online = false
		}
		node := internal.TailscaleNode{
			Name:   hostname,
			IP:     ip,
			OS:     opsys,
			Online: online,
		}
		if first {
			self := node
			self.Online = true
			info.Self = &self
			first = false
			continue
		}
		info.Peers = append(info.Peers, node)
	}
	return info.Self != nil
}
