package api

import (
	"strings"
	"testing"
)

// Issue #243: the Tunnels widget silently drops Tailscale when
// BackendState != Running. The collector already emits a helpful
// `hint` field for the Unreachable case (e.g. "bind-mount
// /var/run/tailscale from the host"), but the dashboard's
// `sections.tunnels` gates the entire Tailscale render on
// `tunnels.tailscale.self`. When the daemon is unreachable `self`
// is nil, so the hint never reaches the UI.
//
// The fix introduces three render branches for Tailscale:
//   - Happy  (installed && self)           → existing peer graph
//   - Hint   (installed && !self && hint)  → compact warning row with hint text verbatim
//   - Empty  (installed && !self && !hint) → minimal "installed, no peers reported" row
//
// These tests pin the invariants against DashboardJS so a future
// refactor cannot silently collapse the branches again.
//
// Implementation note: these are string-literal assertions against
// the embedded JS source (DashboardJS), matching the pattern in
// dashboard_speedtest_pending_test.go and the other dash_*_test.go
// files. The dashboard theme templates (midnight.html / clean.html)
// do NOT link /css/shared.css — any CSS needed for the new render
// paths must be via inline `style=""` attributes, matching the
// existing sections.tunnels pattern.

// tunnelsSectionBody extracts the sections.tunnels function body
// so assertions are scoped and don't accidentally match unrelated
// strings elsewhere in DashboardJS.
func tunnelsSectionBody(t *testing.T) string {
	t.Helper()
	js := DashboardJS
	start := strings.Index(js, "sections.tunnels = function")
	if start < 0 {
		t.Fatal("DashboardJS: sections.tunnels function not found")
	}
	rest := js[start:]
	// Next top-level `sections.` assignment is the bound.
	end := strings.Index(rest[1:], "sections.")
	if end < 0 {
		end = len(rest)
	} else {
		end++ // compensate for rest[1:] offset
	}
	return rest[:end]
}

// Step 1: tracer bullet. When backend_state is Unreachable (or any
// state other than Running) and a `hint` is set, render a warning
// row carrying the hint text verbatim. Without this, the user gets
// zero signal that Tailscale detection ran.
func TestDashboardJS_TunnelsSection_RendersUnreachableTailscale(t *testing.T) {
	body := tunnelsSectionBody(t)

	// Must read the hint field off the tailscale object.
	if !strings.Contains(body, "tunnels.tailscale.hint") &&
		!strings.Contains(body, ".hint") {
		t.Error("sections.tunnels does not reference tailscale.hint; the Unreachable hint path is not wired")
	}
	// Must acknowledge installed-without-self as a distinct render branch.
	// The happy-path gate was `tunnels.tailscale && tunnels.tailscale.self`;
	// the fix must also handle `tunnels.tailscale.installed` when !self.
	if !strings.Contains(body, "tunnels.tailscale.installed") &&
		!strings.Contains(body, ".tailscale.installed") {
		t.Error("sections.tunnels does not branch on tailscale.installed; hint/empty paths are unreachable")
	}
}

// Step 2: regression guard — the happy path (self populated) must
// keep rendering the peer graph exactly as before.
func TestDashboardJS_TunnelsSection_RendersHappyPathUnchanged(t *testing.T) {
	body := tunnelsSectionBody(t)

	// The self-gated render must still exist: peers loop composes
	// [self] + peers.
	if !strings.Contains(body, "[ts.self].concat(ts.peers") {
		t.Error("happy-path peer graph rendering missing: expected '[ts.self].concat(ts.peers ...' construction")
	}
	// The per-node color/dot machinery remains.
	if !strings.Contains(body, "nd.online") {
		t.Error("happy-path node rendering does not check nd.online; peer online/offline dots are missing")
	}
	if !strings.Contains(body, "nd.exit_node") {
		t.Error("happy-path node rendering does not check nd.exit_node; exit-node badge is missing")
	}
	// The "self" badge on the first node: rendered inline as `>self<`.
	if !strings.Contains(body, ">self<") {
		t.Error("happy-path first-node 'self' badge is missing from DashboardJS (expected literal >self<)")
	}
}

// Step 3: empty path — installed but no self and no hint.
// Minimal row so the user sees detection ran.
func TestDashboardJS_TunnelsSection_RendersEmptyTailscale(t *testing.T) {
	body := tunnelsSectionBody(t)

	// The empty-path copy must be present.
	if !strings.Contains(body, "no peers reported") {
		t.Error("sections.tunnels does not render the 'no peers reported' fallback; empty-installed state is silent")
	}
}

// Step 4: backend_state header surfacing. When the Tailscale
// daemon is in a non-Running state (Unreachable, NeedsLogin,
// Stopped, …), the header should show the state alongside the
// version/tailnet so the operator immediately sees that something
// is off.
func TestDashboardJS_TunnelsSection_TailscaleBackendStateInHeader(t *testing.T) {
	body := tunnelsSectionBody(t)

	if !strings.Contains(body, "backend_state") {
		t.Error("sections.tunnels does not reference backend_state; non-Running states are invisible in the header")
	}
	// We specifically do NOT want to show the state when it's
	// Running — that's just noise. The fix should gate the header
	// state chip on backend_state != 'Running'.
	if !strings.Contains(body, "'Running'") && !strings.Contains(body, "\"Running\"") {
		t.Error("sections.tunnels does not compare backend_state against 'Running'; the state chip will always render")
	}
}

// Regression guard: the hint text must be HTML-escaped before
// being interpolated into the template. Hints come from the
// collector (internal/collector/tunnels.go) but that code path
// has changed before (v0.9.2 introduced the plain-text parser
// fallback, which emits its own version-skew hint). Belt and
// braces — run user-controllable strings through esc().
func TestDashboardJS_TunnelsSection_HintIsEscaped(t *testing.T) {
	body := tunnelsSectionBody(t)
	// The esc() helper is defined at the top of sections.tunnels.
	// Require that ANY reference to `.hint` in this function body
	// is wrapped by esc(). A minimal sufficient check: the literal
	// `esc(` must occur somewhere near a `.hint` mention.
	if !strings.Contains(body, "esc(") {
		t.Error("sections.tunnels does not use esc(); hint rendering would be unescaped")
	}
	// Stricter structural assertion: the substring `esc(ts.hint`
	// or `esc(tunnels.tailscale.hint` (whichever the implementation
	// picks) must appear.
	if !strings.Contains(body, "esc(ts.hint") &&
		!strings.Contains(body, "esc(tunnels.tailscale.hint") {
		t.Error("sections.tunnels does not pass hint through esc(); XSS risk on collector-supplied hint text")
	}
}
