package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
)

// fakeRunnerSSE drives an SSE-test livetest.Manager. The test pushes
// samples into the .samples channel; Run forwards them to the
// returned channel until .done is closed.
type fakeRunnerSSE struct {
	samples chan collector.SpeedTestSample
	done    chan struct{}
	result  *internal.SpeedTestResult
	err     error
}

func newFakeRunnerSSE(result *internal.SpeedTestResult) *fakeRunnerSSE {
	return &fakeRunnerSSE{
		samples: make(chan collector.SpeedTestSample, 256),
		done:    make(chan struct{}),
		result:  result,
	}
}

func (f *fakeRunnerSSE) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	out := make(chan collector.SpeedTestSample, 256)
	go func() {
		defer close(out)
		for {
			select {
			case s := <-f.samples:
				out <- s
			case <-f.done:
				for {
					select {
					case s := <-f.samples:
						out <- s
					default:
						return
					}
				}
			}
		}
	}()
	return f.result, out, nil
}

// Counter ID gen for deterministic test_id values.
func counterIDGen() func() int64 {
	var n int64
	return func() int64 { return atomic.AddInt64(&n, 1) }
}

// quietLogger discards log output.
func quietSSELogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubRegistryHolder is a minimal Server stand-in for SSE testing.
// We don't need a full scheduler — just a Server that returns our
// registry from liveTestRegistry(). The simplest path is to use a
// real Server with a *Scheduler that has the registry wired.
//
// But constructing a Scheduler in api tests pulls in the whole
// scheduler package + storage. Cheaper: monkey-patch with a minimal
// router that calls our handlers directly, with the Server itself
// holding the registry. We do this by wiring the Server's scheduler
// field to a stub that returns the registry; that requires an
// exported setter.
//
// Cleanest solution: introduce a small Server-level setter for tests
// only.

// sseEvent is one parsed SSE event from the wire.
type sseEvent struct {
	Event string
	Data  string
}

// parseSSEStream parses an SSE response body into a slice of events.
// Stops at EOF or when an `event: end` is encountered + its data
// emitted (the PRD-pinned terminal event). Strict adherence to the
// "event:\ndata:\n\n" three-line frame.
func parseSSEStream(body io.Reader) ([]sseEvent, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var events []sseEvent
	var cur sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		case line == "":
			// Frame terminator. Flush current event.
			if cur.Event != "" {
				events = append(events, cur)
				if cur.Event == "end" {
					return events, nil
				}
			}
			cur = sseEvent{}
		}
	}
	return events, scanner.Err()
}

// newServerWithRegistry builds a bare Server with no scheduler but
// with a SchedulerProvider stub returning our registry. We can't
// do this without a setter — see helper below.
func newServerWithRegistry(t *testing.T, reg livetest.Registry) (*Server, http.Handler) {
	t.Helper()
	// Construct a minimal Server. We don't have a scheduler; the
	// liveTestRegistry() method will read s.scheduler.LiveTestRegistry()
	// which is nil. So override via a test seam.
	srv := &Server{}
	// Inject the registry directly via the test-only seam:
	srv.testLiveTestRegistry = reg

	r := chi.NewRouter()
	r.Post("/api/v1/speedtest/run", srv.handleSpeedtestRun)
	r.Get("/api/v1/speedtest/stream/{test_id}", srv.handleSpeedtestStream)
	return srv, r
}

func TestSpeedtestSSE_Run_ReturnsTestID(t *testing.T) {
	t.Parallel()
	runner := newFakeRunnerSSE(&internal.SpeedTestResult{
		Engine: internal.SpeedTestEngineSpeedTestGo,
	})
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithRegistry(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		TestID    int64     `json:"test_id"`
		StartedAt time.Time `json:"started_at"`
		Engine    string    `json:"engine"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TestID == 0 {
		t.Errorf("test_id = 0, want non-zero")
	}
	if body.StartedAt.IsZero() {
		t.Errorf("started_at is zero time")
	}

	// Idempotency: a second POST while the test is in flight
	// should return the SAME test_id.
	resp2, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run #2: %v", err)
	}
	defer resp2.Body.Close()
	var body2 struct {
		TestID int64 `json:"test_id"`
	}
	json.NewDecoder(resp2.Body).Decode(&body2)
	if body2.TestID != body.TestID {
		t.Errorf("idempotent /run returned different test_id: %d vs %d", body.TestID, body2.TestID)
	}

	close(runner.done)
}

func TestSpeedtestSSE_Stream_FullEventSequence(t *testing.T) {
	t.Parallel()
	result := &internal.SpeedTestResult{
		DownloadMbps: 920.5,
		UploadMbps:   88.3,
		LatencyMs:    7.8,
		Engine:       internal.SpeedTestEngineSpeedTestGo,
	}
	runner := newFakeRunnerSSE(result)
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithRegistry(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Start the test.
	resp, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	var runBody struct {
		TestID int64 `json:"test_id"`
	}
	json.NewDecoder(resp.Body).Decode(&runBody)

	// Open the stream concurrently.
	streamURL := fmt.Sprintf("%s/api/v1/speedtest/stream/%d", srv.URL, runBody.TestID)

	var wg sync.WaitGroup
	wg.Add(1)
	var events []sseEvent
	var streamErr error
	go func() {
		defer wg.Done()
		resp, err := http.Get(streamURL)
		if err != nil {
			streamErr = err
			return
		}
		defer resp.Body.Close()
		if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
			streamErr = fmt.Errorf("Content-Type = %q, want text/event-stream", got)
			return
		}
		events, streamErr = parseSSEStream(resp.Body)
	}()

	// Push samples through 3 phases: latency → download → upload.
	// Need a tiny pause to ensure the subscriber attached before
	// emit. Sleep is the simplest reliable signal here since the
	// stream goroutine subscribes asynchronously inside the handler.
	time.Sleep(50 * time.Millisecond)
	runner.samples <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseLatency, At: time.Now(), LatencyMs: 8.2}
	runner.samples <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseDownload, At: time.Now(), Mbps: 100}
	runner.samples <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseDownload, At: time.Now(), Mbps: 500}
	runner.samples <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseUpload, At: time.Now(), Mbps: 50}
	close(runner.done)

	wg.Wait()
	if streamErr != nil {
		t.Fatalf("stream: %v", streamErr)
	}

	// Build the expected sequence of event types.
	expected := []string{"start", "phase_change", "sample", "phase_change", "sample", "sample", "phase_change", "sample", "result", "end"}
	got := make([]string, len(events))
	for i, e := range events {
		got[i] = e.Event
	}
	if len(got) != len(expected) {
		t.Logf("got events: %v", got)
		t.Fatalf("event count = %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("event[%d] = %q, want %q", i, got[i], expected[i])
		}
	}

	// Spot-check the data shapes.
	for _, e := range events {
		switch e.Event {
		case "start":
			var d struct {
				TestID int64  `json:"test_id"`
				Engine string `json:"engine"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("start data malformed: %v", err)
			}
			if d.TestID != runBody.TestID {
				t.Errorf("start test_id = %d, want %d", d.TestID, runBody.TestID)
			}
		case "result":
			var r internal.SpeedTestResult
			if err := json.Unmarshal([]byte(e.Data), &r); err != nil {
				t.Errorf("result data malformed: %v", err)
			}
			if r.DownloadMbps != 920.5 {
				t.Errorf("result download = %v, want 920.5", r.DownloadMbps)
			}
		case "end":
			var d struct {
				TestID          int64   `json:"test_id"`
				DurationSeconds float64 `json:"duration_seconds"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("end data malformed: %v", err)
			}
		case "phase_change":
			var d struct {
				Phase       string `json:"phase"`
				PhaseIndex  int    `json:"phase_index"`
				TotalPhases int    `json:"total_phases"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("phase_change data malformed: %v", err)
			}
			if d.TotalPhases != 3 {
				t.Errorf("total_phases = %d, want 3", d.TotalPhases)
			}
		case "sample":
			var d struct {
				Phase       string  `json:"phase"`
				SampleIndex int     `json:"sample_index"`
				Mbps        float64 `json:"mbps"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("sample data malformed: %v", err)
			}
		}
	}
}

func TestSpeedtestSSE_Stream_404UnknownTestID(t *testing.T) {
	t.Parallel()
	runner := newFakeRunnerSSE(&internal.SpeedTestResult{Engine: "speedtest_go"})
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithRegistry(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/stream/99999999")
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSpeedtestSSE_Stream_400InvalidTestID(t *testing.T) {
	t.Parallel()
	mgr := livetest.NewManager(newFakeRunnerSSE(&internal.SpeedTestResult{}), quietSSELogger(), counterIDGen())
	_, handler := newServerWithRegistry(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/stream/notanumber")
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSpeedtestSSE_Run_503WhenNoRegistry(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	r := chi.NewRouter()
	r.Post("/api/v1/speedtest/run", srv.handleSpeedtestRun)
	httpsrv := httptest.NewServer(r)
	defer httpsrv.Close()

	resp, err := http.Post(httpsrv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestSpeedtestSSE_MultipleSubscribers_AllReceiveSameStream(t *testing.T) {
	// Multi-tab: open two streams against the same in-flight test;
	// both must receive the same event sequence (replay + live).
	t.Parallel()
	runner := newFakeRunnerSSE(&internal.SpeedTestResult{
		DownloadMbps: 100, Engine: internal.SpeedTestEngineSpeedTestGo,
	})
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	_, handler := newServerWithRegistry(t, mgr)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		TestID int64 `json:"test_id"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	streamURL := fmt.Sprintf("%s/api/v1/speedtest/stream/%d", srv.URL, body.TestID)

	const numSubs = 3
	var wg sync.WaitGroup
	results := make([][]sseEvent, numSubs)
	for i := 0; i < numSubs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := http.Get(streamURL)
			if err != nil {
				return
			}
			defer r.Body.Close()
			results[idx], _ = parseSSEStream(r.Body)
		}(i)
	}

	time.Sleep(100 * time.Millisecond) // let subscribers connect
	for i := 0; i < 5; i++ {
		runner.samples <- collector.SpeedTestSample{
			Phase: collector.SpeedTestPhaseDownload,
			At:    time.Now(),
			Mbps:  float64(i * 100),
		}
	}
	close(runner.done)
	wg.Wait()

	// Every subscriber must have seen at least start + result + end.
	for i, evs := range results {
		var seenStart, seenResult, seenEnd bool
		for _, e := range evs {
			if e.Event == "start" {
				seenStart = true
			}
			if e.Event == "result" {
				seenResult = true
			}
			if e.Event == "end" {
				seenEnd = true
			}
		}
		if !seenStart || !seenResult || !seenEnd {
			t.Errorf("subscriber %d missed events (start=%v result=%v end=%v)", i, seenStart, seenResult, seenEnd)
		}
	}
}
