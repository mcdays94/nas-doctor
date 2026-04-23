package scheduler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// newApplyChecker builds a StaleSMARTChecker wired to a JSON-buffer
// logger so tests can assert on the canonical INFO log format.
func newApplyChecker(maxAgeDays int) (*StaleSMARTChecker, *mockLastCollectedStore, *bytes.Buffer) {
	return newTestStaleChecker(maxAgeDays)
}

// TestStaleSMARTChecker_Apply_EmptyListSkipsCallback: with no stale
// devices, the forced-collector callback must not fire, and the
// snapshot must be untouched.
func TestStaleSMARTChecker_Apply_EmptyListSkipsCallback(t *testing.T) {
	chk, _, _ := newApplyChecker(7)
	snap := &internal.Snapshot{SMARTStandbyDevices: []string{"/dev/sda"}}

	var called bool
	chk.Apply(snap, nil, func(devices []string) ([]internal.SMARTInfo, error) {
		called = true
		return nil, nil
	})
	if called {
		t.Errorf("callback must not fire for empty stale list")
	}
}

// TestStaleSMARTChecker_Apply_MergesResultsIntoSnapshot: when the
// callback returns SMARTInfo for a previously-standby device, the
// snapshot gains that entry and the device is removed from the
// standby list.
func TestStaleSMARTChecker_Apply_MergesResultsIntoSnapshot(t *testing.T) {
	chk, store, _ := newApplyChecker(7)
	now := time.Now().UTC()
	// Seed so lastAt is 10 days old -> past threshold.
	store.data["/dev/sda"] = now.Add(-10 * 24 * time.Hour)

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}
	stale := chk.Check(snap)
	if len(stale) != 1 {
		t.Fatalf("precondition: expected 1 stale, got %v", stale)
	}

	fresh := internal.SMARTInfo{Device: "/dev/sda", Serial: "SN-FRESH", Model: "Freshly Woke 8TB"}
	chk.Apply(snap, stale, func(devices []string) ([]internal.SMARTInfo, error) {
		if len(devices) != 1 || devices[0] != "/dev/sda" {
			t.Errorf("callback got %v, want [/dev/sda]", devices)
		}
		return []internal.SMARTInfo{fresh}, nil
	})

	if len(snap.SMART) != 1 || snap.SMART[0].Serial != "SN-FRESH" {
		t.Errorf("expected snap.SMART to contain fresh /dev/sda entry, got %+v", snap.SMART)
	}
	// Device must be removed from the standby list — it is no longer
	// in standby after a force-read.
	for _, dev := range snap.SMARTStandbyDevices {
		if dev == "/dev/sda" {
			t.Errorf("expected /dev/sda removed from standby list after force-wake; got %v", snap.SMARTStandbyDevices)
		}
	}
}

// TestStaleSMARTChecker_Apply_EmitsCanonicalInfoLog pins the log
// format specified in issue #238:
//
//	forcing SMART wake on <device>: last read <duration> ago exceeds max_age_days=<N>
func TestStaleSMARTChecker_Apply_EmitsCanonicalInfoLog(t *testing.T) {
	chk, store, buf := newApplyChecker(7)
	now := time.Now().UTC()
	store.data["/dev/sda"] = now.Add(-10 * 24 * time.Hour)

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}
	stale := chk.Check(snap)
	chk.Apply(snap, stale, func(devices []string) ([]internal.SMARTInfo, error) {
		return []internal.SMARTInfo{{Device: "/dev/sda", Model: "X", Serial: "SN"}}, nil
	})

	// Scan the log lines for the canonical INFO entry.
	var foundMsg string
	re := regexp.MustCompile(`forcing SMART wake on \S+: last read .+ ago exceeds max_age_days=7`)
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["level"] != "INFO" {
			continue
		}
		msg, _ := rec["msg"].(string)
		if re.MatchString(msg) {
			foundMsg = msg
			break
		}
	}
	if foundMsg == "" {
		t.Errorf("expected INFO log matching canonical format; got:\n%s", buf.String())
	}
}

// TestStaleSMARTChecker_Apply_PartialFailureContinues: when the
// forced collector returns partial results (one entry + one error),
// the successful entry is merged and the error is logged at ERROR.
// The failed device remains in the standby list (it's still
// considered asleep because the force-read didn't produce data).
func TestStaleSMARTChecker_Apply_PartialFailureContinues(t *testing.T) {
	chk, store, buf := newApplyChecker(7)
	now := time.Now().UTC()
	store.data["/dev/sda"] = now.Add(-10 * 24 * time.Hour)
	store.data["/dev/sdb"] = now.Add(-10 * 24 * time.Hour)

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda", "/dev/sdb"},
	}
	stale := chk.Check(snap)
	if len(stale) != 2 {
		t.Fatalf("precondition: expected 2 stale, got %v", stale)
	}

	chk.Apply(snap, stale, func(devices []string) ([]internal.SMARTInfo, error) {
		return []internal.SMARTInfo{
			{Device: "/dev/sdb", Serial: "SN-B", Model: "OK"},
		}, errors.New("simulated force-read failure on /dev/sda")
	})

	// /dev/sdb's fresh entry merged.
	foundSDB := false
	for _, s := range snap.SMART {
		if s.Device == "/dev/sdb" {
			foundSDB = true
		}
	}
	if !foundSDB {
		t.Errorf("expected /dev/sdb merged into snap.SMART; got %+v", snap.SMART)
	}

	// /dev/sda must remain on the standby list (never force-read successfully).
	foundSDA := false
	for _, d := range snap.SMARTStandbyDevices {
		if d == "/dev/sda" {
			foundSDA = true
		}
	}
	if !foundSDA {
		t.Errorf("expected /dev/sda still in standby list, got %v", snap.SMARTStandbyDevices)
	}

	// Assert an ERROR log was emitted for the callback failure.
	foundErrLog := false
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec["level"] == "ERROR" {
			foundErrLog = true
		}
	}
	if !foundErrLog {
		t.Errorf("expected ERROR log for force-read failure; got:\n%s", buf.String())
	}
}

// TestStaleSMARTChecker_Apply_NilCallbackSafe ensures a nil callback
// (e.g. scheduler mis-wire) doesn't panic.
func TestStaleSMARTChecker_Apply_NilCallbackSafe(t *testing.T) {
	chk, store, _ := newApplyChecker(7)
	now := time.Now().UTC()
	store.data["/dev/sda"] = now.Add(-10 * 24 * time.Hour)

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}
	stale := chk.Check(snap)
	chk.Apply(snap, stale, nil) // must not panic
	// Nothing merged.
	if len(snap.SMART) != 0 {
		t.Errorf("nil callback should not mutate snapshot, got %+v", snap.SMART)
	}
}

// Silence unused-import hint when the test file above doesn't use fmt.
var _ = fmt.Sprintf

// Reusing slog so the compiler keeps the import honest across files.
var _ = slog.LevelInfo
