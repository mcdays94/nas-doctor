package collector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// ── Tailscale status --json parser ──

func TestParseTailscaleStatusJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tailscale_status.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	info := &internal.TailscaleInfo{}
	parseTailscaleStatus(data, info)

	if info.BackendState != "Running" {
		t.Errorf("backend state: got %q, want Running", info.BackendState)
	}
	if info.TailnetName != "example@github" {
		t.Errorf("tailnet name: got %q, want example@github", info.TailnetName)
	}
	if !info.MagicDNS {
		t.Error("expected MagicDNS=true (MagicDNSSuffix present)")
	}
	if info.Self == nil {
		t.Fatal("expected Self to be populated")
	}
	if info.Self.Name != "tower" {
		t.Errorf("self name: got %q, want tower", info.Self.Name)
	}
	if info.Self.IP != "100.64.1.2" {
		t.Errorf("self ip: got %q, want 100.64.1.2", info.Self.IP)
	}
	if info.Self.OS != "linux" {
		t.Errorf("self os: got %q, want linux", info.Self.OS)
	}
	if info.Self.Relay != "lhr" {
		t.Errorf("self relay: got %q, want lhr", info.Self.Relay)
	}
	if info.Self.TxBytes != 7654321 || info.Self.RxBytes != 1234567 {
		t.Errorf("self bytes: tx=%d rx=%d", info.Self.TxBytes, info.Self.RxBytes)
	}

	if len(info.Peers) != 3 {
		t.Fatalf("peers: got %d, want 3", len(info.Peers))
	}

	// Locate peers by name (map iteration order is not stable)
	var laptop, phone, exit *internal.TailscaleNode
	for i := range info.Peers {
		switch info.Peers[i].Name {
		case "laptop":
			laptop = &info.Peers[i]
		case "phone":
			phone = &info.Peers[i]
		case "exit-node":
			exit = &info.Peers[i]
		}
	}
	if laptop == nil || phone == nil || exit == nil {
		t.Fatalf("missing peers: laptop=%v phone=%v exit=%v", laptop, phone, exit)
	}
	if !laptop.Online {
		t.Error("laptop should be online")
	}
	if phone.Online {
		t.Error("phone should be offline")
	}
	if !exit.ExitNode {
		t.Error("exit-node should be ExitNode=true")
	}
	if exit.Relay != "fra" {
		t.Errorf("exit-node relay: got %q, want fra", exit.Relay)
	}
}

// ── collectTailscale orchestration (binary + socket + docker) ──

// withTailscaleStubs swaps the package-level runner vars for the duration of
// a test and restores them on cleanup.
func withTailscaleStubs(
	t *testing.T,
	lookPath func(string) (string, error),
	run func(name string, args ...string) ([]byte, error),
	socketStat func(string) (os.FileInfo, error),
	socketPath string,
) {
	t.Helper()
	origLook := tailscaleLookPath
	origRun := tailscaleRunCommand
	origStat := tailscaleSocketStat
	origPath := tailscaleSocketPath

	tailscaleLookPath = lookPath
	tailscaleRunCommand = run
	tailscaleSocketStat = socketStat
	tailscaleSocketPath = socketPath

	t.Cleanup(func() {
		tailscaleLookPath = origLook
		tailscaleRunCommand = origRun
		tailscaleSocketStat = origStat
		tailscaleSocketPath = origPath
	})
}

func TestCollectTailscale_NoBinary_NoDocker_ReturnsNil(t *testing.T) {
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unreachable") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)

	got := collectTailscale(internal.DockerInfo{})
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestCollectTailscale_SocketHintWhenDaemonUnreachable(t *testing.T) {
	// Binary exists, but `tailscale status` fails and the socket is missing —
	// classic Unraid-plugin-without-mount scenario.
	withTailscaleStubs(t,
		func(bin string) (string, error) {
			if bin == "tailscale" {
				return "/usr/local/bin/tailscale", nil
			}
			return "", errors.New("not found")
		},
		func(name string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "version" {
				// Version probe succeeds even when daemon is unreachable.
				return []byte("1.74.1\n  tailscale commit: abc\n"), nil
			}
			// status --json fails because socket is not mounted
			return nil, errors.New("failed to connect to local tailscaled")
		},
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)

	got := collectTailscale(internal.DockerInfo{})
	if got == nil {
		t.Fatal("expected non-nil TailscaleInfo when binary exists")
	}
	if !got.Installed {
		t.Error("expected Installed=true")
	}
	if got.BackendState != "Unreachable" {
		t.Errorf("backend state: got %q, want Unreachable", got.BackendState)
	}
	if got.Hint == "" {
		t.Error("expected a non-empty Hint explaining the socket mount")
	}
	// Should mention the socket path so the user knows what to mount
	if !containsCI(got.Hint, "/var/run/tailscale") {
		t.Errorf("hint should reference /var/run/tailscale, got %q", got.Hint)
	}
}

func TestCollectTailscale_DockerFallbackStillWorks(t *testing.T) {
	// No host binary, but a Tailscale Docker container is running.
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unused") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)

	docker := internal.DockerInfo{
		Containers: []internal.ContainerInfo{
			{ID: "abc", Name: "tailscale", Image: "tailscale/tailscale:latest", State: "running"},
		},
	}
	got := collectTailscale(docker)
	if got == nil {
		t.Fatal("expected non-nil info from docker fallback")
	}
	if !got.Installed {
		t.Error("expected Installed=true via docker fallback")
	}
	if got.Self == nil || got.Self.Name != "tailscale" {
		t.Errorf("expected Self from docker container, got %+v", got.Self)
	}
}

// containsCI is a tiny case-insensitive substring helper for readable tests.
func containsCI(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	h := []byte(haystack)
	n := []byte(needle)
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			a, b := h[i+j], n[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
