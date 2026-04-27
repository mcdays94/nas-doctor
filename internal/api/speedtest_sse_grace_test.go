package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
)

// Issue #294 R3 — UAT showed GET /api/v1/speedtest/stream/{id} returning
// 404 within ~100ms of POST /api/v1/speedtest/run on the live host.
// Root cause: a fast-failing runner (showwin's FetchUserInfo erroring
// before the SSE client could attach) cleared the registry slot
// before GetLive could resolve the test_id. Pre-fix, the user saw
// "test not found" instead of an error event in the stream.
//
// The grace window in livetest.Manager.GetLive (graceWindow) keeps
// just-completed tests discoverable for 5s so late SSE clients can
// still attach. This test pins that contract end-to-end.

// fastFailRunner returns an error from Run() immediately. Mirrors
// what showwin/speedtest-go does when FetchUserInfo or FetchServers
// hits a transient network failure on UAT.
type fastFailRunner struct {
	err error
}

func (r *fastFailRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	if r.err == nil {
		r.err = errors.New("FetchUserInfo: timeout")
	}
	return nil, nil, r.err
}

// TestSpeedtestSSE_FastFailingRunner_StreamReturns200WithErrorEvent
// drives the full HTTP path: POST /run with a runner that errors
// immediately, then GET /stream/{id} ~50ms later (after the runner
// has already cleared the registry slot but within the grace window).
// The stream must return 200 with a clean error+end event sequence,
// NOT 404. Pre-fix this returned 404. Issue #294 R3.
func TestSpeedtestSSE_FastFailingRunner_StreamReturns200WithErrorEvent(t *testing.T) {
	t.Parallel()
	runner := &fastFailRunner{err: errors.New("FetchUserInfo: timeout")}
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())

	srv := &Server{}
	srv.testLiveTestRegistry = mgr
	r := chi.NewRouter()
	r.Post("/api/v1/speedtest/run", srv.handleSpeedtestRun)
	r.Get("/api/v1/speedtest/stream/{test_id}", srv.handleSpeedtestStream)
	httpsrv := httptest.NewServer(r)
	defer httpsrv.Close()

	// Trigger the run.
	resp, err := http.Post(httpsrv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /run status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		TestID int64 `json:"test_id"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.TestID == 0 {
		t.Fatal("POST /run returned test_id=0")
	}

	// Wait long enough for the runner to fail + the registry slot
	// to be cleared. 50ms is overkill but deterministic. Pre-fix
	// the next GET /stream/{id} would 404. Post-fix the grace
	// window keeps it discoverable.
	time.Sleep(50 * time.Millisecond)

	streamResp, err := http.Get(fmt.Sprintf("%s/api/v1/speedtest/stream/%d", httpsrv.URL, body.TestID))
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /stream status = %d, want 200 (grace window must keep just-completed test discoverable for late SSE clients — issue #294 R3)", streamResp.StatusCode)
	}

	events, err := parseSSEStream(streamResp.Body)
	if err != nil {
		t.Fatalf("parseSSEStream: %v", err)
	}
	// We expect at least: start, error, end.
	var seenStart, seenError, seenEnd bool
	for _, e := range events {
		switch e.Event {
		case "start":
			seenStart = true
		case "error":
			seenError = true
		case "end":
			seenEnd = true
		}
	}
	if !seenStart {
		t.Error("late stream missing 'start' event")
	}
	if !seenError {
		t.Error("late stream missing 'error' event — UI cannot tell user why the test failed")
	}
	if !seenEnd {
		t.Error("late stream missing 'end' event — EventSource won't close cleanly")
	}
}

// Compile-time guard: fastFailRunner satisfies livetest.Runner.
var _ livetest.Runner = (*fastFailRunner)(nil)
