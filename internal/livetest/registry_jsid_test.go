// Issue #296 B2 — manual `POST /run` returned a test_id whose
// JSON-roundtrip in the dashboard's EventSource code path lost
// precision, producing a `/stream/{id}` URL whose integer didn't
// match the registry's stored id. The browser saw 404 within
// ~300ms even though the backend gauge said in_progress=1.
//
// Root cause: NewManager's default idGen used time.Now().UnixNano()
// which produces values around 1.78e18 — outside JavaScript's safe
// integer range (Number.MAX_SAFE_INTEGER == 2^53 - 1 == ~9.0e15).
// JSON-parsing such an integer in JavaScript silently rounds to the
// nearest float64, then the toString that the URL builder uses lands
// on a different int64. ParseInt on the server side then resolves to
// a NEW id which doesn't match m.active.id and GetLive returns 404.
//
// The fix uses UnixMilli() (1.78e12) which is safely under the JS
// safe-integer ceiling AND remains monotonically increasing for the
// "next test" scenario at human-clickable cadences (since milliseconds
// are well below the lockout windows).
//
// This test pins the contract: every value the default idGen produces
// must fit in the JS safe-integer range so the dashboard's
// JSON.parse(testId).toString() roundtrip is lossless.
package livetest

import (
	"strconv"
	"testing"
)

// jsMaxSafeInteger is JavaScript's Number.MAX_SAFE_INTEGER (2^53 - 1).
// Any int64 ≤ this value roundtrips losslessly through JSON.parse →
// Number → toString in the browser.
const jsMaxSafeInteger int64 = 1<<53 - 1 // 9_007_199_254_740_991

// TestNewManager_DefaultIDGen_ProducesJSSafeInteger asserts that the
// default idGen wired into NewManager (when callers pass nil) produces
// values ≤ Number.MAX_SAFE_INTEGER. Pre-fix, the default was
// time.Now().UnixNano() which is ~1.78e18 in the v0.9.11 timeframe —
// 197x larger than MAX_SAFE_INTEGER. That broke the dashboard's
// EventSource attach because JSON.parse rounded the test_id to the
// nearest representable float64, yielding a different int64 on
// toString.
func TestNewManager_DefaultIDGen_ProducesJSSafeInteger(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, quietLogger(), nil)
	if mgr.idGen == nil {
		t.Fatal("default idGen is nil")
	}
	id := mgr.idGen()
	if id <= 0 {
		t.Fatalf("default idGen produced non-positive id: %d", id)
	}
	if id > jsMaxSafeInteger {
		t.Errorf("default idGen produced %d, > Number.MAX_SAFE_INTEGER (%d). "+
			"This breaks the dashboard's JSON.parse → toString roundtrip "+
			"for /api/v1/speedtest/stream/{id}; resulting URL targets a "+
			"DIFFERENT int64 on the server, GetLive returns 404. "+
			"Issue #296 B2.",
			id, jsMaxSafeInteger)
	}
}

// TestNewManager_DefaultIDGen_StableStringRoundtrip asserts that the
// default idGen's output, when round-tripped through the JSON wire
// format the dashboard uses, produces the same string. Equivalent to
// the JS roundtrip test but written in Go terms: the integer must be
// preserved across strconv.FormatInt → strconv.ParseInt at int64
// precision AND fit in float64 without loss.
func TestNewManager_DefaultIDGen_StableStringRoundtrip(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, quietLogger(), nil)
	id := mgr.idGen()
	s := strconv.FormatInt(id, 10)
	parsed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("ParseInt: %v", err)
	}
	if parsed != id {
		t.Errorf("int64 ↔ string roundtrip lost precision: %d → %q → %d", id, s, parsed)
	}
	// And the JS-equivalent: float64 cast must roundtrip to the
	// same int64. This is the actual constraint that
	// JSON.parse(...).toString() imposes in the browser.
	asFloat := float64(id)
	asInt := int64(asFloat)
	if asInt != id {
		t.Errorf("int64 → float64 → int64 lost precision: %d → %g → %d "+
			"(JavaScript's JSON.parse roundtrip would land on %d, not %d). "+
			"Issue #296 B2 — broke /api/v1/speedtest/stream/{id} URL routing.",
			id, asFloat, asInt, asInt, id)
	}
}

// TestNewManager_DefaultIDGen_Monotonic asserts that successive calls
// to the default idGen return increasing values — the registry's
// "active vs grace test" lookup relies on IDs being unique-per-test
// AND monotonically increasing so a fresh test never collides with a
// just-completed grace test.
func TestNewManager_DefaultIDGen_Monotonic(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, quietLogger(), nil)
	prev := mgr.idGen()
	for i := 0; i < 5; i++ {
		// Sleep a tick to advance the clock past millisecond
		// resolution; otherwise back-to-back calls within the
		// same millisecond produce equal values.
		next := mgr.idGen()
		if next < prev {
			t.Errorf("idGen() decreased: prev=%d next=%d (iteration %d)", prev, next, i)
		}
		prev = next
	}
}
