package api

// HTTP-handler tests for the cancel endpoint added in issue #304.
// Three status codes are pinned by the issue's acceptance criteria:
//
//   - 200 OK when an active in-flight test is cancelled.
//   - 404 Not Found when the test_id is unknown / forgotten.
//   - 409 Conflict when the test had already completed (still in
//     the registry's grace window) before Cancel arrived.
//
// We also pin 400 Bad Request for non-numeric test_id (defense
// against URL-builder bugs on the dashboard side) and the SSE
// `cancelled` event payload shape.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
)

// blockingRunner mirrors the registry-test-only ctxAwareFakeRunner —
// duplicated here to keep the api package's test self-contained
// (different package; can't reach into livetest_test).
type blockingRunner struct {
	started chan struct{}
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{})}
}

func (b *blockingRunner) Run(ctx context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	close(b.started)
	out := make(chan collector.SpeedTestSample, 4)
	go func() {
		defer close(out)
		out <- collector.SpeedTestSample{
			Phase: collector.SpeedTestPhaseDownload,
			Mbps:  100,
			At:    time.Now(),
		}
		<-ctx.Done()
	}()
	return &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}, out, nil
}

func newServerWithCancelRoute(t *testing.T, reg livetest.Registry) (*Server, http.Handler) {
	t.Helper()
	srv := &Server{}
	srv.testLiveTestRegistry = reg
	r := chi.NewRouter()
	r.Post("/api/v1/speedtest/run", srv.handleSpeedtestRun)
	r.Get("/api/v1/speedtest/stream/{test_id}", srv.handleSpeedtestStream)
	r.Post("/api/v1/speedtest/cancel/{test_id}", srv.handleSpeedtestCancel)
	return srv, r
}

func TestHandleSpeedtestCancel_200_OnActiveID(t *testing.T) {
	t.Parallel()
	runner := newBlockingRunner()
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithCancelRoute(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Kick off a test.
	resp, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		TestID int64 `json:"test_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /run body: %v", err)
	}
	<-runner.started

	// POST cancel — must return 200 + Cancelled:true.
	cancelResp, err := http.Post(
		fmt.Sprintf("%s/api/v1/speedtest/cancel/%d", srv.URL, body.TestID),
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /cancel: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(cancelResp.Body)
		t.Fatalf("cancel status = %d, want 200; body=%s", cancelResp.StatusCode, raw)
	}
	var cancelBody struct {
		TestID    int64 `json:"test_id"`
		Cancelled bool  `json:"cancelled"`
	}
	if err := json.NewDecoder(cancelResp.Body).Decode(&cancelBody); err != nil {
		t.Fatalf("decode /cancel body: %v", err)
	}
	if cancelBody.TestID != body.TestID {
		t.Errorf("cancel test_id = %d, want %d", cancelBody.TestID, body.TestID)
	}
	if !cancelBody.Cancelled {
		t.Errorf("cancel.cancelled = false, want true")
	}
}

func TestHandleSpeedtestCancel_404_OnUnknownID(t *testing.T) {
	t.Parallel()
	runner := newBlockingRunner()
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithCancelRoute(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// No test in flight. Cancel must return 404.
	resp, err := http.Post(srv.URL+"/api/v1/speedtest/cancel/999999", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /cancel: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 404; body=%s", resp.StatusCode, raw)
	}
}

func TestHandleSpeedtestCancel_409_OnAlreadyCompleted(t *testing.T) {
	t.Parallel()
	// Use a fast-completing runner so we land in the grace window
	// before Cancel arrives.
	runner := newFakeRunnerSSE(&internal.SpeedTestResult{
		Engine: internal.SpeedTestEngineSpeedTestGo,
	})
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithCancelRoute(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Start the test then immediately let it complete.
	resp, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	var body struct {
		TestID int64 `json:"test_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /run body: %v", err)
	}
	resp.Body.Close()

	// Let the runner finish naturally so the test is in the
	// grace window when we try to cancel.
	close(runner.done)

	// Wait for completion + the slot to transition into grace.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !mgr.InProgress() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if mgr.InProgress() {
		t.Fatal("test did not finish in 2s")
	}

	// Cancel — must return 409 (already completed, still in grace).
	cancelResp, err := http.Post(
		fmt.Sprintf("%s/api/v1/speedtest/cancel/%d", srv.URL, body.TestID),
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /cancel: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusConflict {
		raw, _ := io.ReadAll(cancelResp.Body)
		t.Fatalf("status = %d, want 409; body=%s", cancelResp.StatusCode, raw)
	}
}

func TestHandleSpeedtestCancel_400_OnInvalidID(t *testing.T) {
	t.Parallel()
	runner := newBlockingRunner()
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithCancelRoute(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/speedtest/cancel/not-a-number", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /cancel: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleSpeedtestCancel_StreamReceivesCancelledEvent(t *testing.T) {
	t.Parallel()
	// End-to-end: start a test, subscribe via SSE, cancel via the
	// HTTP endpoint, assert the stream emitted a `cancelled` event
	// followed by `end`. This is the contract the dashboard relies
	// on to flip the strip into the idle state cleanly.
	runner := newBlockingRunner()
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithCancelRoute(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	var body struct {
		TestID int64 `json:"test_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /run body: %v", err)
	}
	resp.Body.Close()
	<-runner.started

	// Open the SSE stream in a goroutine.
	streamURL := fmt.Sprintf("%s/api/v1/speedtest/stream/%d", srv.URL, body.TestID)
	streamResp, err := http.Get(streamURL)
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer streamResp.Body.Close()

	// Briefly let the stream emit start + sample events before we
	// cancel, so the test exercises the mid-flight cancel path.
	time.Sleep(50 * time.Millisecond)

	cancelResp, err := http.Post(
		fmt.Sprintf("%s/api/v1/speedtest/cancel/%d", srv.URL, body.TestID),
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /cancel: %v", err)
	}
	cancelResp.Body.Close()

	// Read the SSE body to EOF / `end` event.
	scanner := bufio.NewScanner(streamResp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	sawCancelled := false
	sawEnd := false
	deadline := time.Now().Add(3 * time.Second)
	for scanner.Scan() && time.Now().Before(deadline) {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: cancelled") {
			sawCancelled = true
		}
		if strings.HasPrefix(line, "event: end") {
			sawEnd = true
			break
		}
	}
	if !sawCancelled {
		t.Errorf("SSE stream did not emit `cancelled` event before end")
	}
	if !sawEnd {
		t.Errorf("SSE stream did not emit `end` event")
	}
}
