// Issue #296 B1 — UAT showed manual `POST /api/v1/speedtest/run`
// producing a speedtest_history row with download/upload/latency
// populated, but the linked speedtest_samples table was empty (zero
// rows). Pre-rc2 the same path produced 256 samples for a typical
// run. The rc2 lifecycle restructure (stampTerminal +
// closeSubscribersAndDone + completion-handler hook) was suspected,
// but the real-world bug surface only manifests on the showwin
// engine — local repro with the synthetic samplingRunner DOES
// persist samples correctly.
//
// This test pins the registry → completion-handler → bulk-insert
// contract for a runner that emits a non-empty per-sample stream
// via the API path (no scheduler.runSpeedTest blocker — the API
// path returns immediately and lets the registry observers do the
// persistence). Pre-existing
// TestLiveTestRegistry_APIPath_PersistsHistoryAndSamples covered the
// no-samples case (samples: nil); this test extends coverage to the
// non-nil case so a future regression where samples are read BEFORE
// the runner fully drains would fail loudly.
//
// Companion runner-boundary structured logging in
// internal/collector/speedtest_go_lib.go gives next UAT enough
// information to discriminate "showwin emitted N samples and the
// registry dropped them" from "showwin emitted 0 samples".
package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestLiveTestRegistry_APIPath_PersistsNonEmptySampleSet is the
// regression guard for issue #296 B1. Drives the same registry
// wiring as the API path (StartTest, no block-on-Done) but with a
// runner that emits a non-empty per-sample stream. Asserts the bulk-
// insert lands every emitted sample with correct sample_index +
// phase ordering.
//
// Pre-fix (i.e. if a future refactor reads SnapshotSamples() before
// the registry's drain loop completes), this test fails with
// len(persisted) < len(emitted).
func TestLiveTestRegistry_APIPath_PersistsNonEmptySampleSet(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)

	now := time.Now().UTC()
	emitted := []collector.SpeedTestSample{
		{Phase: collector.SpeedTestPhaseLatency, At: now, LatencyMs: 7.2},
		{Phase: collector.SpeedTestPhaseLatency, At: now.Add(100 * time.Millisecond), LatencyMs: 6.8},
		{Phase: collector.SpeedTestPhaseLatency, At: now.Add(200 * time.Millisecond), LatencyMs: 7.0},
		{Phase: collector.SpeedTestPhaseDownload, At: now.Add(time.Second), Mbps: 320},
		{Phase: collector.SpeedTestPhaseDownload, At: now.Add(2 * time.Second), Mbps: 720},
		{Phase: collector.SpeedTestPhaseDownload, At: now.Add(3 * time.Second), Mbps: 920},
		{Phase: collector.SpeedTestPhaseUpload, At: now.Add(4 * time.Second), Mbps: 50},
		{Phase: collector.SpeedTestPhaseUpload, At: now.Add(5 * time.Second), Mbps: 88},
	}
	runner := &samplingRunner{
		result: &internal.SpeedTestResult{
			DownloadMbps: 920, UploadMbps: 88, LatencyMs: 7,
			Timestamp: now,
			Engine:    internal.SpeedTestEngineSpeedTestGo,
		},
		samples: emitted,
	}

	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	// Simulate the API path: caller starts the test and exits
	// (does NOT block on Done — that's the cron path's discipline).
	lt, err := mgr.StartTest(context.Background())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	// Wait for the registry's completion handler to fire (which
	// runs the bulk-insert) before asserting on the store. The
	// handler runs INSIDE the registry's driveTest defer chain
	// BEFORE Done unblocks — so once Done is closed, persistence
	// is already complete.
	<-lt.Done()

	id, ok, err := store.GetLatestSpeedTestHistoryID()
	if err != nil {
		t.Fatalf("GetLatestSpeedTestHistoryID: %v", err)
	}
	if !ok || id == 0 {
		t.Fatalf("expected history row to be persisted via API path, got ok=%v id=%d", ok, id)
	}

	persisted, err := store.GetSpeedTestSamples(id)
	if err != nil {
		t.Fatalf("GetSpeedTestSamples: %v", err)
	}
	if len(persisted) != len(emitted) {
		t.Fatalf("API path persisted %d samples, runner emitted %d. "+
			"Issue #296 B1 — the registry's completion handler must "+
			"observe the FULL sample buffer before reading "+
			"SnapshotSamples().",
			len(persisted), len(emitted))
	}
	for i, s := range persisted {
		if s.SampleIndex != i {
			t.Errorf("persisted[%d].SampleIndex = %d, want %d (samples must be inserted in emission order)",
				i, s.SampleIndex, i)
		}
		if s.Phase != string(emitted[i].Phase) {
			t.Errorf("persisted[%d].Phase = %q, want %q", i, s.Phase, emitted[i].Phase)
		}
	}
}
