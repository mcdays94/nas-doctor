package scheduler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestStaleSMARTChecker_Integration_EndToEnd pins the end-to-end
// contract from issue #238: given a real *storage.DB seeded with
// smart_history rows at known ages, the Check+Apply flow must
//
//  1. identify exactly the drive(s) past the MaxAgeDays threshold
//  2. invoke the forced-collector callback for those drive(s)
//  3. merge the fresh results into the snapshot
//  4. emit the canonical INFO log
//
// This is the "integration test with seeded DB" acceptance criterion
// from the issue.
func TestStaleSMARTChecker_Integration_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "integration.db")
	silentLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := storage.Open(dbPath, silentLogger)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	defer db.Close()

	// Seed a snapshot row + three smart_history rows of varying ages.
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.SaveSnapshot(&internal.Snapshot{
		ID:        "seed-snap",
		Timestamp: now.Add(-30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Directly seed smart_history with known-age rows.
	type seed struct {
		device string
		age    time.Duration
	}
	for _, s := range []seed{
		{"/dev/sda", 10 * 24 * time.Hour}, // 10d — stale at 7d threshold
		{"/dev/sdb", 3 * 24 * time.Hour},  // 3d — fresh
		// /dev/sdc: intentionally has NO history row → new drive
	} {
		if err := seedSmartHistoryRow(db, "seed-snap", s.device, now.Add(-s.age)); err != nil {
			t.Fatalf("seed %s: %v", s.device, err)
		}
	}

	// Build the snapshot the collector would have produced — with all
	// three drives reported as standby (the `-n standby` skip path).
	// /dev/sda and /dev/sdc are past-threshold candidates on paper;
	// only /dev/sda should actually be flagged because /dev/sdc has
	// no history and new drives are never force-woken (user story 7).
	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda", "/dev/sdb", "/dev/sdc"},
	}

	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	chk := NewStaleSMARTChecker(db, 7 /* maxAgeDays */, logger)

	stale := chk.Check(snap)
	if len(stale) != 1 || stale[0] != "/dev/sda" {
		t.Fatalf("expected Check to flag only /dev/sda, got %v", stale)
	}

	// Capture the callback argument list to confirm it's invoked with
	// the exact stale list.
	var gotDevices []string
	forced := func(devices []string) ([]internal.SMARTInfo, error) {
		gotDevices = append([]string{}, devices...)
		return []internal.SMARTInfo{
			{Device: "/dev/sda", Model: "Reawakened 8TB", Serial: "SN-SDA"},
		}, nil
	}
	chk.Apply(snap, stale, forced)

	if len(gotDevices) != 1 || gotDevices[0] != "/dev/sda" {
		t.Errorf("callback devices = %v, want [/dev/sda]", gotDevices)
	}
	// Snapshot merge: /dev/sda appears with fresh serial, removed from standby list.
	foundFresh := false
	for _, s := range snap.SMART {
		if s.Device == "/dev/sda" && s.Serial == "SN-SDA" {
			foundFresh = true
		}
	}
	if !foundFresh {
		t.Errorf("expected fresh /dev/sda entry in snap.SMART; got %+v", snap.SMART)
	}
	stillStandby := false
	for _, d := range snap.SMARTStandbyDevices {
		if d == "/dev/sda" {
			stillStandby = true
		}
	}
	if stillStandby {
		t.Errorf("/dev/sda should be removed from standby list after force-wake; got %v", snap.SMARTStandbyDevices)
	}

	// Assert the canonical INFO log fired for /dev/sda with a
	// duration describing roughly 10 days.
	foundLog := false
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		msg, _ := rec["msg"].(string)
		if strings.HasPrefix(msg, "forcing SMART wake on /dev/sda") &&
			strings.Contains(msg, "max_age_days=7") {
			foundLog = true
		}
	}
	if !foundLog {
		t.Errorf("expected canonical INFO log for /dev/sda; got:\n%s", buf.String())
	}
}

// TestStaleSMARTChecker_Integration_MaxAgeZeroNoForceWake is the
// regression guard for user story 5 (MaxAgeDays=0 preserves v0.9.5
// behaviour). Even with a drive 30 days stale, no force-wake happens.
func TestStaleSMARTChecker_Integration_MaxAgeZeroNoForceWake(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "disabled.db")
	silentLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := storage.Open(dbPath, silentLogger)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	if err := db.SaveSnapshot(&internal.Snapshot{ID: "s-off", Timestamp: now.Add(-60 * 24 * time.Hour)}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := seedSmartHistoryRow(db, "s-off", "/dev/sda", now.Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	snap := &internal.Snapshot{
		Timestamp:           now,
		SMARTStandbyDevices: []string{"/dev/sda"},
	}

	chk := NewStaleSMARTChecker(db, 0 /* disabled */, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	stale := chk.Check(snap)
	if len(stale) != 0 {
		t.Fatalf("MaxAgeDays=0 must disable force-wake; got stale=%v", stale)
	}

	var callbackFired bool
	chk.Apply(snap, stale, func(devices []string) ([]internal.SMARTInfo, error) {
		callbackFired = true
		return nil, nil
	})
	if callbackFired {
		t.Errorf("MaxAgeDays=0: force-wake callback must not fire")
	}
	// Snapshot shape preserved.
	if len(snap.SMART) != 0 {
		t.Errorf("snap.SMART should not be mutated; got %+v", snap.SMART)
	}
	if len(snap.SMARTStandbyDevices) != 1 || snap.SMARTStandbyDevices[0] != "/dev/sda" {
		t.Errorf("standby list should be preserved, got %v", snap.SMARTStandbyDevices)
	}
}

// seedSmartHistoryRow is a small test helper that inserts a
// smart_history row directly. Uses db.Exec on the (unexported) *sql.DB
// via a cast — that's why this helper lives in the scheduler package
// and calls db methods that are exported for this purpose. If the
// storage package stops exporting a raw Exec path we can switch to
// constructing full Snapshots and calling SaveSnapshot.
func seedSmartHistoryRow(db *storage.DB, snapshotID, device string, ts time.Time) error {
	// Construct a real Snapshot and call SaveSnapshot — the only
	// exported write path in storage.DB. Each call generates one row
	// in smart_history with the specified timestamp (snapshot's
	// Timestamp field is used for the row).
	snap := &internal.Snapshot{
		ID:        snapshotID + "-" + device,
		Timestamp: ts,
		SMART: []internal.SMARTInfo{
			{
				Device:       device,
				Serial:       "SERIAL-" + device,
				Model:        "MODEL",
				Temperature:  30,
				HealthPassed: true,
				PowerOnHours: 1000,
			},
		},
	}
	return db.SaveSnapshot(snap)
}
