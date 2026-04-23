package collector

import (
	"errors"
	"os"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #243 (acceptance criterion #2): the Tailscale Docker-container
// detection heuristic previously matched only on the literal substring
// "tailscale" in image or container name. Users running Tailscale via
// a sidecar named `ts-sidecar` or `mullvad-tailscale-alt` with a
// non-obvious image were silently missed.
//
// Fix: opt-in env var NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES accepts a
// comma-separated list of case-insensitive substring patterns that
// should ALSO mark a container as Tailscale. When unset, behaviour is
// unchanged (substring match on "tailscale" in image or name) so
// upgrading users see no regression.
//
// Semantics documented alongside the feature:
//   - comma-separated
//   - case-insensitive
//   - substring match against BOTH container name and image
//   - whitespace around each token is trimmed
//   - empty tokens are ignored
//   - OR-combined with the default "tailscale" substring rule

func TestCollectTailscale_CustomContainerName_Matches(t *testing.T) {
	// No host binary — force the Docker fallback path.
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unused") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)
	t.Setenv("NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES", "ts-sidecar")

	docker := internal.DockerInfo{
		Containers: []internal.ContainerInfo{
			// Image/name contain nothing like "tailscale" — only the
			// env-var pattern can rescue detection.
			{ID: "abc", Name: "ts-sidecar", Image: "alpine:latest", State: "running"},
		},
	}
	got := collectTailscale(docker)
	if got == nil {
		t.Fatal("expected non-nil info: custom container-name env should have matched 'ts-sidecar'")
	}
	if !got.Installed {
		t.Error("expected Installed=true via custom-name env match")
	}
	if got.Self == nil || got.Self.Name != "ts-sidecar" {
		t.Errorf("expected Self.Name=ts-sidecar, got %+v", got.Self)
	}
}

func TestCollectTailscale_CustomContainerName_MultipleTokensCommaSeparated(t *testing.T) {
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unused") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)
	// Comma-separated, with extra whitespace and a stray empty token —
	// all of which should be tolerated.
	t.Setenv("NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES", " vpn-foo , , mullvad-ts ")

	docker := internal.DockerInfo{
		Containers: []internal.ContainerInfo{
			{ID: "1", Name: "mullvad-ts", Image: "alpine:latest", State: "running"},
		},
	}
	got := collectTailscale(docker)
	if got == nil {
		t.Fatal("expected non-nil info on 'mullvad-ts' match")
	}
	if got.Self == nil || got.Self.Name != "mullvad-ts" {
		t.Errorf("expected Self.Name=mullvad-ts, got %+v", got.Self)
	}
}

func TestCollectTailscale_CustomContainerName_CaseInsensitive(t *testing.T) {
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unused") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)
	t.Setenv("NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES", "TS-SIDECAR")

	docker := internal.DockerInfo{
		Containers: []internal.ContainerInfo{
			{ID: "1", Name: "ts-sidecar", Image: "alpine:latest", State: "running"},
		},
	}
	got := collectTailscale(docker)
	if got == nil || got.Self == nil {
		t.Fatal("expected case-insensitive match")
	}
}

// Regression guard: with the env var unset, the default substring
// heuristic ("tailscale" in name or image) must still work exactly as
// before — upgrading users without the opt-in must see no change.
func TestCollectTailscale_DefaultBehaviourUnchangedWhenEnvUnset(t *testing.T) {
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unused") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)
	// Ensure env is unset for the test even if it leaked in from the
	// parent shell.
	t.Setenv("NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES", "")

	docker := internal.DockerInfo{
		Containers: []internal.ContainerInfo{
			{ID: "1", Name: "tailscale", Image: "tailscale/tailscale:latest", State: "running"},
		},
	}
	got := collectTailscale(docker)
	if got == nil || got.Self == nil || got.Self.Name != "tailscale" {
		t.Fatalf("default substring match regressed: got %+v", got)
	}
}

// Regression guard: custom names must NOT match an unrelated
// container. If the env var reads "ts-sidecar" and the user happens to
// run a container called "postgres", no match — substring is still
// specific, not a catch-all.
func TestCollectTailscale_CustomContainerName_NonMatchingContainerIgnored(t *testing.T) {
	withTailscaleStubs(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { return nil, errors.New("unused") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		"/var/run/tailscale/tailscaled.sock",
	)
	t.Setenv("NAS_DOCTOR_TAILSCALE_CONTAINER_NAMES", "ts-sidecar")

	docker := internal.DockerInfo{
		Containers: []internal.ContainerInfo{
			{ID: "1", Name: "postgres", Image: "postgres:16", State: "running"},
		},
	}
	got := collectTailscale(docker)
	if got != nil {
		t.Errorf("expected nil (no detection) for unrelated container, got %+v", got)
	}
}
