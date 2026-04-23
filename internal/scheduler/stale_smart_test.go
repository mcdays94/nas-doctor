package scheduler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// mockLastCollectedStore is a minimal stub that satisfies the
// last-collected lookup surface StaleSMARTChecker needs, without
// pulling in the full storage.HistoryStore machinery.
type mockLastCollectedStore struct {
	data map[string]time.Time // device -> last read
	// forceErr, when non-nil, is returned instead of a lookup.
	forceErr error
	calls    []string // devices queried in order
}

func newMockStore() *mockLastCollectedStore {
	return &mockLastCollectedStore{data: map[string]time.Time{}}
}

func (m *mockLastCollectedStore) GetLastSMARTCollectedAt(device string) (time.Time, bool, error) {
	m.calls = append(m.calls, device)
	if m.forceErr != nil {
		return time.Time{}, false, m.forceErr
	}
	ts, ok := m.data[device]
	return ts, ok, nil
}

// newTestStaleChecker constructs a StaleSMARTChecker with a mock store
// and a buffer-backed logger so tests can assert on log output.
func newTestStaleChecker(maxAgeDays int) (*StaleSMARTChecker, *mockLastCollectedStore, *bytes.Buffer) {
	store := newMockStore()
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	chk := NewStaleSMARTChecker(store, maxAgeDays, logger)
	return chk, store, buf
}

// TestStaleSMARTChecker_Check_PastThresholdReturnsDevice: a drive
// that's been in standby and whose last successful SMART read is
// 8 days old with MaxAgeDays=7 must be flagged for force-wake.
func TestStaleSMARTChecker_Check_PastThresholdReturnsDevice(t *testing.T) {
	chk, store, _ := newTestStaleChecker(7)
	now := time.Now().UTC()
	store.data["/dev/sda"] = now.Add(-8 * 24 * time.Hour)

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}
	got := chk.Check(snap)
	if len(got) != 1 || got[0] != "/dev/sda" {
		t.Errorf("expected [/dev/sda], got %v", got)
	}
}

// TestStaleSMARTChecker_Check_WithinThresholdSkipped: a drive whose
// last SMART read is 5 days old with MaxAgeDays=7 must NOT be
// flagged; the existing passive collection path already covers it.
func TestStaleSMARTChecker_Check_WithinThresholdSkipped(t *testing.T) {
	chk, store, _ := newTestStaleChecker(7)
	now := time.Now().UTC()
	store.data["/dev/sda"] = now.Add(-5 * 24 * time.Hour)

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}
	got := chk.Check(snap)
	if len(got) != 0 {
		t.Errorf("expected no force-wake (5d < 7d), got %v", got)
	}
}

// TestStaleSMARTChecker_Check_NoHistorySkipped: a drive in standby
// with no smart_history rows is a new drive — never force-wake. This
// is the user-story-7 contract.
func TestStaleSMARTChecker_Check_NoHistorySkipped(t *testing.T) {
	chk, _, _ := newTestStaleChecker(7)

	snap := &internal.Snapshot{
		Timestamp:           time.Now().UTC(),
		SMARTStandbyDevices: []string{"/dev/brand-new"},
	}
	got := chk.Check(snap)
	if len(got) != 0 {
		t.Errorf("expected no force-wake for new drive, got %v", got)
	}
}

// TestStaleSMARTChecker_Check_EmptyStandbyListSkipsStoreQueries: when
// the snapshot reports no drives in standby, the checker must not
// query the store at all (fast path).
func TestStaleSMARTChecker_Check_EmptyStandbyListSkipsStoreQueries(t *testing.T) {
	chk, store, _ := newTestStaleChecker(7)

	snap := &internal.Snapshot{
		Timestamp:           time.Now().UTC(),
		SMARTStandbyDevices: nil,
	}
	got := chk.Check(snap)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if len(store.calls) != 0 {
		t.Errorf("expected 0 store queries, got %d (calls=%v)", len(store.calls), store.calls)
	}
}

// TestStaleSMARTChecker_Check_MaxAgeZeroDisablesFeature: setting the
// threshold to 0 means "never force wake" (PRD #236 user story 5 —
// preserve exact v0.9.5 behaviour). No store queries, no devices
// returned — regardless of how old the histories are.
func TestStaleSMARTChecker_Check_MaxAgeZeroDisablesFeature(t *testing.T) {
	chk, store, _ := newTestStaleChecker(0)
	now := time.Now().UTC()
	store.data["/dev/sda"] = now.Add(-365 * 24 * time.Hour) // a year ago — would be way stale

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}
	got := chk.Check(snap)
	if len(got) != 0 {
		t.Errorf("MaxAgeDays=0 should disable force-wake, got %v", got)
	}
	if len(store.calls) != 0 {
		t.Errorf("MaxAgeDays=0 must not issue store queries, got %d", len(store.calls))
	}
}

// TestStaleSMARTChecker_Check_NilSnapshotSafe: a nil snapshot must not
// panic (defensive, in case a collection failure upstream passes nil).
func TestStaleSMARTChecker_Check_NilSnapshotSafe(t *testing.T) {
	chk, _, _ := newTestStaleChecker(7)
	got := chk.Check(nil)
	if len(got) != 0 {
		t.Errorf("nil snapshot must return empty, got %v", got)
	}
}

// TestStaleSMARTChecker_Check_StoreErrorLoggedContinues: if the store
// lookup fails for one device, the checker logs a WARN and continues
// processing the rest of the standby list. The failed device is not
// flagged for force-wake (we don't force-read without knowing when it
// was last read).
func TestStaleSMARTChecker_Check_StoreErrorLoggedContinues(t *testing.T) {
	store := newMockStore()
	store.data["/dev/sdb"] = time.Now().UTC().Add(-10 * 24 * time.Hour)
	// /dev/sda would fail — but mockLastCollectedStore forceErr is
	// global. Simulate per-device error by wrapping.
	var perDeviceErr = func(dev string) bool { return dev == "/dev/sda" }
	wrapper := &wrappedStore{inner: store, failIf: perDeviceErr}
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	chk := NewStaleSMARTChecker(wrapper, 7, logger)

	snap := &internal.Snapshot{
		Timestamp:           time.Now().UTC(),
		SMARTStandbyDevices: []string{"/dev/sda", "/dev/sdb"},
	}
	got := chk.Check(snap)
	if len(got) != 1 || got[0] != "/dev/sdb" {
		t.Errorf("expected [/dev/sdb], got %v", got)
	}

	// Assert a warn log was emitted for /dev/sda.
	foundWarn := false
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["level"] == "WARN" && strings.Contains(rec["msg"].(string), "stale-SMART lookup failed") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Errorf("expected WARN log for /dev/sda store failure; got: %s", buf.String())
	}
}

// wrappedStore injects a per-device error into the mock store.
type wrappedStore struct {
	inner  *mockLastCollectedStore
	failIf func(string) bool
}

func (w *wrappedStore) GetLastSMARTCollectedAt(device string) (time.Time, bool, error) {
	if w.failIf != nil && w.failIf(device) {
		return time.Time{}, false, errFake
	}
	return w.inner.GetLastSMARTCollectedAt(device)
}

var errFake = &fakeErr{"simulated DB failure"}

type fakeErr struct{ s string }

func (e *fakeErr) Error() string { return e.s }
