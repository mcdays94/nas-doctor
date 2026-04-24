package collector

import (
	"log/slog"
	"os"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

// silentLogger returns a slog.Logger that writes errors-only so tests
// don't spew on stderr. The subsystem collectors log at Info/Warn on
// their normal paths; we're asserting on return-value contracts here,
// not log output.
func silentCollectorLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestCollector_CollectProxmox_NotEnabled_ReturnsNil: when the
// Proxmox config has Enabled=false (the default for installs that
// haven't configured PVE integration), CollectProxmox returns
// (nil, nil) — no error, no result — so the scheduler can cheaply
// call this on its configured cadence even when there's no cluster
// to talk to.
func TestCollector_CollectProxmox_NotEnabled_ReturnsNil(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	c.SetProxmoxConfig(ProxmoxConfig{Enabled: false})

	info, err := c.CollectProxmox()
	if err != nil {
		t.Errorf("CollectProxmox should not error when disabled; got %v", err)
	}
	if info != nil {
		t.Errorf("CollectProxmox should return nil when disabled; got %+v", info)
	}
}

// TestCollector_CollectKubernetes_NotEnabled_ReturnsNil: symmetric
// with TestCollector_CollectProxmox_NotEnabled_ReturnsNil.
func TestCollector_CollectKubernetes_NotEnabled_ReturnsNil(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	c.SetKubeConfig(KubeConfig{Enabled: false})

	info, err := c.CollectKubernetes()
	if err != nil {
		t.Errorf("CollectKubernetes should not error when disabled; got %v", err)
	}
	if info != nil {
		t.Errorf("CollectKubernetes should return nil when disabled; got %+v", info)
	}
}

// TestCollector_CollectGPU_ReturnsNonNil: CollectGPU always returns
// a non-nil *GPUInfo (whose Available flag is false on hardware
// without a GPU). This is the contract the dispatcher relies on to
// decide whether to merge into the snapshot.
func TestCollector_CollectGPU_ReturnsNonNil(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	info := c.CollectGPU()
	if info == nil {
		t.Fatalf("CollectGPU must return a non-nil *GPUInfo even on hardware without a GPU")
	}
	// On CI / test hosts without nvidia-smi / intel_gpu_top / amdgpu,
	// info.Available will be false. We don't assert on that — just
	// that the method is cleanly callable.
}

// TestCollector_CollectZFS_ReturnsNoPanic: CollectZFS is callable
// on hosts without ZFS. Either returns (nil, nil) or a non-nil info
// whose Available flag accurately reports state.
func TestCollector_CollectZFS_ReturnsNoPanic(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	info, _ := c.CollectZFS()
	// On a host without ZFS, info may be a non-nil value with
	// Available=false, or nil. Either is valid. What we're asserting
	// is that the method doesn't panic when ZFS binaries are absent.
	if info != nil && info.Available && len(info.Pools) == 0 {
		t.Errorf("CollectZFS reported Available=true but no pools; contract violation: %+v", info)
	}
}

// TestCollector_CollectDocker_ReturnsCallable: CollectDocker is the
// same code path as CollectDockerStats — a callable method even
// when Docker is absent.
func TestCollector_CollectDocker_ReturnsCallable(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	info, _ := c.CollectDocker()
	// On a host without Docker, info.Available is false. Containers
	// slice may be nil or empty. No panic is the contract we care
	// about for this wrapper.
	if info.Available && len(info.Containers) == 0 {
		// Not a hard failure — some hosts report Available with an
		// empty list legitimately (Docker up, no containers). Just
		// verify the shape is legal.
		t.Logf("CollectDocker returned Available=true with no containers")
	}
}

// TestCollector_CollectSMART_ReturnsNoPanic_EmptyPlatform: CollectSMART
// with an empty platform is callable. It may return (nil, nil, error)
// when no drives are discovered — that's fine, we're asserting
// non-panic behaviour of the wrapper itself.
func TestCollector_CollectSMART_ReturnsNoPanic_EmptyPlatform(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	// Intentionally no SMARTConfig — we want the default (no wake).
	// Just verify the method is callable.
	_, _, _ = c.CollectSMART("")
	// Also try with platform="unraid" to exercise the ArraySlot
	// enrichment branch without asserting on its output (which
	// depends on /sys/block/md*/slaves/).
	_, _, _ = c.CollectSMART("unraid")
}

// TestCollector_CollectProxmox_EnabledButUnreachable_ReturnsErrorInfo:
// when Proxmox is enabled but the URL is bogus, the wrapper returns
// a non-nil ProxmoxInfo with Error set and Connected=false. The
// scheduler uses this shape to report connection failures without
// crashing.
func TestCollector_CollectProxmox_EnabledButUnreachable_ReturnsErrorInfo(t *testing.T) {
	c := New(internal.HostPaths{}, silentCollectorLogger())
	c.SetProxmoxConfig(ProxmoxConfig{
		Enabled: true,
		URL:     "https://127.0.0.1:1",
		TokenID: "fake",
		Secret:  "fake",
		Alias:   "test-pve",
	})

	info, err := c.CollectProxmox()
	if err != nil {
		t.Errorf("CollectProxmox should not return a Go error for unreachable endpoints; got %v", err)
	}
	if info == nil {
		t.Fatalf("CollectProxmox should return non-nil info when Enabled=true, even on failure")
	}
	if info.Alias != "test-pve" {
		t.Errorf("Alias not propagated; got %q, want test-pve", info.Alias)
	}
}
