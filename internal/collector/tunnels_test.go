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

// ── Tailscale plain-text (`tailscale status`) parser ──

// TestParsePlainStatus_RealWorldOutput reproduces the v0.9.2-rc2 hardware finding:
// when the container's tailscale CLI is older than the host tailscaled daemon,
// `tailscale status --json` silently returns empty bytes while `tailscale status`
// (tabular) still works. The collector must fall back to parsing the tabular form.
func TestParsePlainStatus_RealWorldOutput(t *testing.T) {
	// Fixture matches what the user captured on Unraid with tailscale 1.76.6 client
	// + tailscaled 1.96.2 server. First row = self (LastSeen "-" = online).
	// Subsequent rows = peers.
	output := `100.70.89.101   tower                amtccdias@   linux   -
100.85.250.94   iphone181            amtccdias@   iOS     -
100.92.71.34    old-laptop           amtccdias@   linux   2h22m`

	info := &internal.TailscaleInfo{}
	ok := parsePlainStatus(output, info)
	if !ok {
		t.Fatal("parsePlainStatus returned false; expected true for 3-row input")
	}
	if info.Self == nil {
		t.Fatal("Self was nil; expected populated from first row")
	}
	if info.Self.Name != "tower" || info.Self.IP != "100.70.89.101" || info.Self.OS != "linux" {
		t.Errorf("Self: got %+v", info.Self)
	}
	if !info.Self.Online {
		t.Error("Self.Online should be true (LastSeen=\"-\" = currently online)")
	}
	if len(info.Peers) != 2 {
		t.Fatalf("Peers: got %d, want 2", len(info.Peers))
	}
	var iphone, laptop *internal.TailscaleNode
	for i, p := range info.Peers {
		switch p.Name {
		case "iphone181":
			iphone = &info.Peers[i]
		case "old-laptop":
			laptop = &info.Peers[i]
		}
	}
	if iphone == nil {
		t.Fatal("missing peer iphone181")
	}
	if iphone.IP != "100.85.250.94" || iphone.OS != "iOS" || !iphone.Online {
		t.Errorf("iphone181: got %+v", iphone)
	}
	if laptop == nil {
		t.Fatal("missing peer old-laptop")
	}
	if laptop.Online {
		t.Error("old-laptop.Online should be false (LastSeen=\"2h22m\" means offline)")
	}
}

// TestParsePlainStatus_SkipsWarningLines guards against stderr bleeding into stdout
// (e.g. `Warning: client version ...` that older tailscale CLIs sometimes print
// alongside `status` output).
func TestParsePlainStatus_SkipsWarningLines(t *testing.T) {
	output := `Warning: client version "1.76.6-AlpineLinux" != tailscaled server version "1.96.2"
100.70.89.101   tower                amtccdias@   linux   -`

	info := &internal.TailscaleInfo{}
	ok := parsePlainStatus(output, info)
	if !ok {
		t.Fatal("expected Self to be populated despite warning line")
	}
	if info.Self == nil || info.Self.Name != "tower" {
		t.Errorf("Self: got %+v", info.Self)
	}
}

// TestParsePlainStatus_EmptyOrMalformed returns false without populating Self.
func TestParsePlainStatus_EmptyOrMalformed(t *testing.T) {
	cases := []string{"", "    \n   ", "only one field", "not-an-ip hostname owner os -"}
	for _, input := range cases {
		info := &internal.TailscaleInfo{}
		if parsePlainStatus(input, info) {
			t.Errorf("parsePlainStatus(%q) = true; want false", input)
		}
		if info.Self != nil {
			t.Errorf("Self populated for malformed input %q", input)
		}
	}
}

// TestCollectTailscale_FallsBackToPlainWhenJSONEmpty is the end-to-end test for
// the Alpine-client vs newer-server skew fix. `tailscale status --json` returns
// empty (no error, no bytes); the orchestration must try `tailscale status`
// (plain) and surface the peers from there.
func TestCollectTailscale_FallsBackToPlainWhenJSONEmpty(t *testing.T) {
	withTailscaleStubs(t,
		func(bin string) (string, error) {
			if bin == "tailscale" {
				return "/usr/bin/tailscale", nil
			}
			return "", errors.New("not found")
		},
		func(name string, args ...string) ([]byte, error) {
			if name != "tailscale" {
				return nil, errors.New("unexpected command")
			}
			switch {
			case len(args) == 1 && args[0] == "version":
				return []byte("1.76.6\n  tailscale commit: AlpineLinux\n"), nil
			case len(args) == 2 && args[0] == "status" && args[1] == "--json":
				// Empty output — the exact symptom observed in rc2.
				return []byte(""), nil
			case len(args) == 1 && args[0] == "status":
				return []byte("100.70.89.101   tower                amtccdias@   linux   -\n100.85.250.94   iphone181            amtccdias@   iOS     -\n"), nil
			}
			return nil, errors.New("unexpected args")
		},
		func(string) (os.FileInfo, error) { return nil, nil },
		"/var/run/tailscale/tailscaled.sock",
	)

	got := collectTailscale(internal.DockerInfo{})
	if got == nil {
		t.Fatal("expected non-nil TailscaleInfo")
	}
	if got.Self == nil || got.Self.Name != "tower" {
		t.Errorf("Self: got %+v; want tower", got.Self)
	}
	if len(got.Peers) != 1 || got.Peers[0].Name != "iphone181" {
		t.Errorf("Peers: got %+v", got.Peers)
	}
	if got.BackendState != "Running" {
		t.Errorf("BackendState: got %q, want Running", got.BackendState)
	}
	if got.Hint == "" {
		t.Error("expected a diagnostic hint explaining the --json fallback")
	}
}
