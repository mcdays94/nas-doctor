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

// samplingRunner emits a deterministic per-sample stream during a
// fake test run, mirroring what showwin/speedtest-go would produce in
// production. The samples close before Run returns so the registry
// completes deterministically. PRD #283 slice 3 / issue #286.
type samplingRunner struct {
	result  *internal.SpeedTestResult
	samples []collector.SpeedTestSample
}

func (r *samplingRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	out := make(chan collector.SpeedTestSample, len(r.samples))
	for _, s := range r.samples {
		out <- s
	}
	close(out)
	return r.result, out, nil
}

// TestRunSpeedTest_PersistsSamplesToStore drives the full scheduler →
// registry → store path and asserts the buffered sample set lands in
// speedtest_samples linked to the parent speedtest_history row. End-
// to-end integration test for slice 3.
func TestRunSpeedTest_PersistsSamplesToStore(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()

	s := New(nil, store, nil, nil, logger, time.Hour)

	now := time.Now().UTC()
	runner := &samplingRunner{
		result: &internal.SpeedTestResult{
			DownloadMbps: 920,
			UploadMbps:   88,
			LatencyMs:    8,
			Timestamp:    now,
			Engine:       internal.SpeedTestEngineSpeedTestGo,
		},
		samples: []collector.SpeedTestSample{
			{Phase: collector.SpeedTestPhaseLatency, At: now, LatencyMs: 8.2},
			{Phase: collector.SpeedTestPhaseLatency, At: now.Add(time.Second), LatencyMs: 9.1},
			{Phase: collector.SpeedTestPhaseDownload, At: now.Add(2 * time.Second), Mbps: 421},
			{Phase: collector.SpeedTestPhaseDownload, At: now.Add(3 * time.Second), Mbps: 723},
			{Phase: collector.SpeedTestPhaseUpload, At: now.Add(4 * time.Second), Mbps: 88},
		},
	}
	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	s.runSpeedTest()

	// History row must have been written via SaveSpeedTestReturningID.
	id, ok, err := store.GetLatestSpeedTestHistoryID()
	if err != nil {
		t.Fatalf("GetLatestSpeedTestHistoryID: %v", err)
	}
	if !ok || id == 0 {
		t.Fatalf("expected a history row to be persisted, got ok=%v id=%d", ok, id)
	}

	// Samples must be linked to that history ID, in emission order.
	got, err := store.GetSpeedTestSamples(id)
	if err != nil {
		t.Fatalf("GetSpeedTestSamples: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len(samples) = %d, want 5", len(got))
	}
	for i, s := range got {
		if s.SampleIndex != i {
			t.Errorf("samples[%d].SampleIndex = %d, want %d", i, s.SampleIndex, i)
		}
	}
	if got[0].Phase != "latency" || got[2].Phase != "download" || got[4].Phase != "upload" {
		t.Errorf("phase ordering broken: %+v", got)
	}
}

// TestRunSpeedTest_NoSamples_HistoryStillWritten asserts that a
// completed test with zero per-sample telemetry (e.g. legacy Ookla CLI
// fallback path that emits no per-sample data) still produces a
// history row + flips LastAttempt to success. The optional sample-
// sidecar is decoupled from history persistence — a missing sample
// stream is NOT a failure.
func TestRunSpeedTest_NoSamples_HistoryStillWritten(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()
	s := New(nil, store, nil, nil, logger, time.Hour)

	runner := &samplingRunner{
		result: &internal.SpeedTestResult{
			DownloadMbps: 100, UploadMbps: 50, LatencyMs: 12,
			Timestamp: time.Now(), Engine: internal.SpeedTestEngineOoklaCLI,
		},
		samples: nil,
	}
	mgr := livetest.NewManager(runner, logger, nil)
	s.SetLiveTestRegistry(mgr)

	s.runSpeedTest()

	id, ok, _ := store.GetLatestSpeedTestHistoryID()
	if !ok || id == 0 {
		t.Fatalf("expected history row, got ok=%v id=%d", ok, id)
	}
	got, _ := store.GetSpeedTestSamples(id)
	if len(got) != 0 {
		t.Errorf("expected zero samples for ookla CLI fallback, got %d", len(got))
	}
	att, _ := store.GetLastSpeedTestAttempt()
	if att == nil || att.Status != "success" {
		t.Errorf("LastAttempt status = %v, want success", att)
	}
}

// TestSpeedCheck_StampsSpeedTestHistoryID asserts that the type=speed
// scheduled service-check dispatch (issue #210 reads-from-history
// path) populates ServiceCheckResult.SpeedTestHistoryID with the ID
// of the history row it consulted. This is the linkage the
// /service-checks expanded-log mini-chart uses to fetch
// /api/v1/speedtest/samples/{id}. PRD #283 slice 3 / issue #286.
func TestSpeedCheck_StampsSpeedTestHistoryID(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	logger := quietLogger()

	// Seed a successful attempt + history row that the read-from-
	// history dispatch will consult.
	id, err := store.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 100, UploadMbps: 50, LatencyMs: 5,
		Timestamp: time.Now(), Engine: internal.SpeedTestEngineSpeedTestGo,
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}
	_ = store.SaveSpeedTestAttempt(storage.LastSpeedTestAttempt{
		Timestamp: time.Now(), Status: "success",
	})

	sc := NewServiceChecker(store, logger)
	check := internal.ServiceCheckConfig{
		Name:   "Internet Speed",
		Type:   "speed",
		Target: "speedtest",
	}
	result := internal.ServiceCheckResult{Key: "internet-speed", Name: check.Name, Type: check.Type, Target: check.Target}
	sc.runSpeedCheck(check, &result, time.Now())

	if result.SpeedTestHistoryID != id {
		t.Errorf("result.SpeedTestHistoryID = %d, want %d (linked to seed history row)", result.SpeedTestHistoryID, id)
	}
}
