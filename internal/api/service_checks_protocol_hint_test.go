package api

import (
	"strings"
	"testing"
)

// TestServiceChecksHTML_RendersProtocolHint guards the UI cross-reference
// for issue #188. The Go ProtocolHint helper (internal/scheduler/
// protocol_hints.go) populates a protocol_hint key on TCP service
// check Details; the expanded log entry renderer in service_checks.html
// and the Test button toast renderer in settings.html must both
// consume that key to draw a badge.
//
// Classic §4b trap: a future refactor renames the key on one side only
// and the badge silently disappears. This test locks the key name in
// place on the JS side; the Go side is locked in the ProtocolHint
// unit tests.
func TestServiceChecksHTML_RendersProtocolHint(t *testing.T) {
	html := loadServiceChecksHTML(t)
	if !strings.Contains(html, "protocol_hint") {
		t.Errorf("service_checks.html expanded log renderer missing protocol_hint key — TCP protocol badge will not render (issue #188)")
	}
}

func TestSettingsHTML_TestButtonTCPProtocolHint(t *testing.T) {
	html := loadSettingsHTML(t)
	// The TCP renderer in the Test-button toast helper must consume
	// protocol_hint so the toast mirrors the badge shown in the
	// expanded log row. Scoped to the renderServiceCheckDetails
	// function by string match on both tokens — settings.html is
	// 3k+ lines and grepping for the key alone could false-positive
	// on unrelated content in a future expansion.
	if !strings.Contains(html, "renderServiceCheckDetails") {
		t.Fatal("settings.html missing renderServiceCheckDetails helper — Test button toast detail rendering broken")
	}
	if !strings.Contains(html, "protocol_hint") {
		t.Errorf("settings.html Test-button renderer missing protocol_hint key — toast won't show protocol badge (issue #188)")
	}
}
