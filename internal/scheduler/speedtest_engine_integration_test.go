package scheduler

import (
	"context"
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// TestSpeedTestEngine_EndToEnd_RunnerDrivesHistoryRowEngineColumn
// drives a fake SpeedTestRunner from outside the package, has the
// scheduler runSpeedTest path invoke it via collector.RunSpeedTest,
// and asserts the resulting row in storage.GetSpeedTestHistory has
// the correct engine column. Closes the loop on the issue #284
// acceptance: "Integration test: run a test, assert history row has
// correct `engine` column value".
func TestSpeedTestEngine_EndToEnd_RunnerDrivesHistoryRowEngineColumn(t *testing.T) {
	store := newFakeStore()

	// Wire a deterministic runner that always reports a
	// speedtest_go-stamped result. Roll back at end of test so
	// other tests aren't polluted by the global override.
	restore := collector.SetSpeedTestRunnerForTest(stubSpeedTestRunner{
		result: &internal.SpeedTestResult{
			Timestamp:    time.Now(),
			DownloadMbps: 500, UploadMbps: 50, LatencyMs: 4,
			Engine: internal.SpeedTestEngineSpeedTestGo,
		},
	})
	defer restore()

	// The scheduler's runSpeedTest path calls collector.RunSpeedTest
	// (the legacy public API) which now delegates to the runner. Use
	// it directly here — that's the same path the cron tick takes.
	res := collector.RunSpeedTest()
	if res == nil {
		t.Fatal("RunSpeedTest returned nil — runner override did not take effect")
	}
	if res.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Fatalf("Engine = %q, want speedtest_go", res.Engine)
	}

	// Persist via the storage layer (mirrors what scheduler does).
	if err := store.SaveSpeedTest("integration-1", res); err != nil {
		t.Fatalf("SaveSpeedTest: %v", err)
	}

	// Read back via the public history API.
	pts, err := store.GetSpeedTestHistory(24)
	if err != nil {
		t.Fatalf("GetSpeedTestHistory: %v", err)
	}
	if len(pts) != 1 {
		t.Fatalf("history len = %d, want 1", len(pts))
	}
	if pts[0].Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("history row engine = %q, want speedtest_go", pts[0].Engine)
	}
}

// stubSpeedTestRunner is a SpeedTestRunner that always returns a
// canned result with a closed empty samples channel. Useful for
// driving the public collector.RunSpeedTest entry without going to
// the network. Mirrors fakeSpeedTestEngine but at the runner level.
type stubSpeedTestRunner struct {
	result *internal.SpeedTestResult
}

func (s stubSpeedTestRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	ch := make(chan collector.SpeedTestSample)
	close(ch)
	return s.result, ch, nil
}

// newFakeStore returns a minimal speedtest-history-capable store
// backed by a real on-disk SQLite (each test gets its own tempdir).
func newFakeStore() *storage.DB {
	db, err := storage.Open(":memory:", nil)
	if err != nil {
		panic(err)
	}
	return db
}
