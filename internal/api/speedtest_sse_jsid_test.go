// Issue #296 B2 regression guard at the HTTP boundary.
//
// UAT showed `GET /api/v1/speedtest/stream/{id}` returning 404
// within ~300ms of `POST /api/v1/speedtest/run` while
// /metrics.in_progress=1 simultaneously reported the test as in
// flight. Root cause: the int64 test_id (~1.78e18 from
// time.Now().UnixNano()) round-tripped through the dashboard's
// JSON.parse → toString chain, hit JS Number's 2^53 ceiling, and
// landed on a different int64. The URL-targeted ID then never
// matched m.active.id and GetLive returned 404.
//
// This test pins the contract end-to-end: a test_id returned by
// /api/v1/speedtest/run, when emitted as JSON and parsed by a
// JavaScript-faithful pipeline (float64 cast → toString), must
// resolve to the SAME id when used as the URL path segment for
// /api/v1/speedtest/stream/{id}.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/livetest"
)

// blockingRunnerSSE blocks Run until release is closed, then returns
// the configured result with no samples. Lets the test keep a test
// in flight long enough to attach the SSE stream and assert the
// start event arrives.
type blockingRunnerSSE struct {
	result  *internal.SpeedTestResult
	release chan struct{}
}

func (r *blockingRunnerSSE) Run(_ context.Context) (*internal.SpeedTestResult, <-chan collector.SpeedTestSample, error) {
	out := make(chan collector.SpeedTestSample)
	go func() {
		<-r.release
		close(out)
	}()
	return r.result, out, nil
}

// jsRoundtrip simulates JavaScript's JSON.parse → Number → toString
// pipeline that the dashboard executes inline (`'/api/v1/speedtest/stream/' + body.test_id`).
// JS Numbers are float64; any int64 above 2^53 - 1 loses precision.
func jsRoundtrip(id int64) string {
	asFloat := float64(id)
	asInt := int64(asFloat)
	return strconv.FormatInt(asInt, 10)
}

// TestSpeedtestSSE_TestIDIsJSSafe asserts that the test_id returned
// by POST /api/v1/speedtest/run survives a JS-faithful float64
// roundtrip without changing value. Pre-fix the registry's default
// idGen used UnixNano which produced ~1.78e18 — far above
// Number.MAX_SAFE_INTEGER (2^53 - 1 ~= 9.0e15) — and the JS roundtrip
// silently rounded to a neighbouring int64.
func TestSpeedtestSSE_TestIDIsJSSafe(t *testing.T) {
	t.Parallel()
	// Production wiring: NewManager with idGen=nil → default. We
	// must assert the DEFAULT behaviour, not a counterIDGen() stub
	// (which only produces 1, 2, 3...).
	runner := &blockingRunnerSSE{
		result:  &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo},
		release: make(chan struct{}),
	}
	mgr := livetest.NewManager(runner, quietSSELogger(), nil) // nil → default idGen
	srv := &Server{}
	srv.testLiveTestRegistry = mgr
	r := chi.NewRouter()
	r.Post("/api/v1/speedtest/run", srv.handleSpeedtestRun)
	httpsrv := httptest.NewServer(r)
	defer httpsrv.Close()

	resp, err := http.Post(httpsrv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		TestID int64 `json:"test_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TestID == 0 {
		t.Fatal("test_id = 0")
	}

	// JS-faithful roundtrip: serialise → parse-as-float64 → emit
	// as string. The result MUST match the original FormatInt.
	want := strconv.FormatInt(body.TestID, 10)
	got := jsRoundtrip(body.TestID)
	if want != got {
		t.Errorf("test_id=%d lost precision in JS roundtrip: %q → %q. "+
			"Issue #296 B2 — dashboard EventSource URL targets a "+
			"different int64 than the registry's m.active.id, "+
			"causing GET /stream/{id} → 404 even while "+
			"in_progress=1.",
			body.TestID, want, got)
	}

	close(runner.release)
}

// TestSpeedtestSSE_StreamReturns200WithStartEvent_DefaultIDGen drives
// the full POST /run + GET /stream/{id} HTTP path with a runner that
// blocks long enough for the GET to attach mid-flight. Pre-fix this
// returned 404 for production-shaped IDs because the dashboard's
// float-rounded URL targeted a different int64. With the JS-safe
// idGen the URL targets the SAME int64 and GetLive resolves cleanly.
//
// Specifically asserts the dashboard's exact serialisation pipeline:
//
//   1. JSON.parse the /run body → JS Number (float64)
//   2. concat into URL: '/api/v1/speedtest/stream/' + parsedID
//   3. GET that URL
//
// The GET MUST return 200 with the start event.
func TestSpeedtestSSE_StreamReturns200WithStartEvent_DefaultIDGen(t *testing.T) {
	t.Parallel()
	runner := &blockingRunnerSSE{
		result:  &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo},
		release: make(chan struct{}),
	}
	mgr := livetest.NewManager(runner, quietSSELogger(), nil) // production idGen
	srv := &Server{}
	srv.testLiveTestRegistry = mgr
	r := chi.NewRouter()
	r.Post("/api/v1/speedtest/run", srv.handleSpeedtestRun)
	r.Get("/api/v1/speedtest/stream/{test_id}", srv.handleSpeedtestStream)
	httpsrv := httptest.NewServer(r)
	defer httpsrv.Close()

	// Phase 1: POST /run, parse the body as a JS-faithful pipeline
	// would (float64 → toString).
	resp, err := http.Post(httpsrv.URL+"/api/v1/speedtest/run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		TestID int64 `json:"test_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TestID == 0 {
		t.Fatal("test_id = 0")
	}

	// Phase 2: build the URL the way the JS does — round-trip the
	// id through float64 first, then format it as decimal.
	jsURL := fmt.Sprintf("%s/api/v1/speedtest/stream/%s", httpsrv.URL, jsRoundtrip(body.TestID))

	// Phase 3: subscribe in a goroutine so the runner stays alive
	// long enough for the SSE handler to write the start frame.
	// Release the runner after the GET request issues so the
	// stream can deliver its terminal event and the goroutine
	// returns cleanly without a custom Reader-deadline dance.
	type result struct {
		status int
		body   string
		err    error
	}
	streamRes := make(chan result, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, jsURL, nil)
		streamResp, err := http.DefaultClient.Do(req)
		if err != nil {
			streamRes <- result{err: err}
			return
		}
		defer streamResp.Body.Close()
		if streamResp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(streamResp.Body)
			streamRes <- result{status: streamResp.StatusCode, body: string(b)}
			return
		}
		// Read until EOF (which happens after the runner releases
		// + the SSE handler writes the end event).
		b, _ := io.ReadAll(streamResp.Body)
		streamRes <- result{status: 200, body: string(b)}
	}()

	// Give the goroutine a moment to issue the GET, then release
	// the runner so the test can complete cleanly.
	time.Sleep(150 * time.Millisecond)
	close(runner.release)

	r2 := <-streamRes
	if r2.err != nil {
		t.Fatalf("GET /stream: %v", r2.err)
	}
	if r2.status != http.StatusOK {
		t.Fatalf("GET /stream %s → %d, want 200. Body: %s. "+
			"Issue #296 B2 — JS-faithful URL roundtrip lost test_id "+
			"precision; registry's GetLive resolves a different int64 "+
			"than what was returned from /run.",
			jsURL, r2.status, r2.body)
	}
	if !containsStartEvent([]byte(r2.body)) {
		t.Errorf("GET /stream returned 200 but no start event observed. Body: %q", r2.body)
	}

	wg.Wait()
}

// containsStartEvent does a substring match for the SSE start frame.
// Cheap; we only need to know "did anything land before timeout".
func containsStartEvent(buf []byte) bool {
	return len(buf) > 0 && (containsByteSeq(buf, []byte("event: start")) || containsByteSeq(buf, []byte("event:start")))
}
func containsByteSeq(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Compile-time guard.
var _ livetest.Runner = (*blockingRunnerSSE)(nil)
