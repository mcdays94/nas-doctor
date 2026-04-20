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
		// Get status JSON
		if out, err := tailscaleRunCommand("tailscale", "status", "--json"); err == nil {
			parseTailscaleStatus(out, info)
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

	// 2. Check Docker containers (image contains "tailscale")
	for _, c := range docker.Containers {
		img := strings.ToLower(c.Image)
		name := strings.ToLower(c.Name)
		if !strings.Contains(img, "tailscale") && !strings.Contains(name, "tailscale") {
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
