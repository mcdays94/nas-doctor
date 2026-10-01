package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// postTestStream issues a POST to /api/v1/service-checks/test-stream
// against the live router (so the chi route definition is exercised
// alongside the handler). Returns the full response so the caller
// can inspect headers + parse the SSE body.
func postTestStream(t *testing.T, srv *Server, body any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	resp, err := http.Post(ts.URL+"/api/v1/service-checks/test-stream", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

// parseTraceSSEStream parses the test-stream SSE body into a slice
// of events. Stops at end-of-stream or `event: end`.
func parseTraceSSEStream(body io.Reader) ([]sseEvent, error) {
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

// TestServiceChecksTestStream_TraceHappyPath — type=trace request
// produces start + per-cycle hop + result + end events with parseable
// JSON payloads.
//
// Regression guard: the test asserts the WHOLE flow completes in
// well under 5s. A nil-channel range bug caused
// v0.9.15-rc1 CI to hang for the full 10-minute test timeout while
// the local dev box passed in <100ms — Go's select is pseudo-random
// and the local box happened to schedule `final` before all
// `updates` had drained, hitting a safe code path that masked the
// bug. The deadline is the discipline that keeps a recurrence loud
// and fast instead of silent and slow.
func TestServiceChecksTestStream_TraceHappyPath(t *testing.T) {
	deadline := assertCompletesWithin(t, 5*time.Second)
	defer deadline()

	srv := newSettingsTestServer()

	// Inject a fake streaming runner that emits 3 cumulative hop
	// updates then finalises with a 3-hop result.
	srv.streamingTracerouteRunner = func(ctx context.Context, target string, cycles int) (<-chan collector.StreamingMTRUpdate, <-chan collector.StreamingTraceFinal) {
		updates := make(chan collector.StreamingMTRUpdate, 4)
		final := make(chan collector.StreamingTraceFinal, 1)
		go func() {
			defer close(updates)
			defer close(final)
			hop1 := collector.MTRHop{Count: 1, Host: "10.0.0.1", AvgMs: 0.5}
			hop2 := collector.MTRHop{Count: 2, Host: "203.0.113.1", AvgMs: 2.1}
			hop3 := collector.MTRHop{Count: 3, Host: "8.8.8.8", AvgMs: 12.3}
			updates <- collector.StreamingMTRUpdate{Cycle: 1, TotalCycle: 3, Hops: []collector.MTRHop{hop1}}
			updates <- collector.StreamingMTRUpdate{Cycle: 2, TotalCycle: 3, Hops: []collector.MTRHop{hop1, hop2}}
			updates <- collector.StreamingMTRUpdate{Cycle: 3, TotalCycle: 3, Hops: []collector.MTRHop{hop1, hop2, hop3}}
			final <- collector.StreamingTraceFinal{
				Result: &collector.MTRResult{
					Target:     target,
					Hops:       []collector.MTRHop{hop1, hop2, hop3},
					FinalRTTMs: 12.3,
				},
			}
		}()
		return updates, final
	}

	resp := postTestStream(t, srv, map[string]any{
		"name":   "trace-stream",
		"type":   "traceroute",
		"target": "8.8.8.8",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream prefix", got)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "no-cache") || !strings.Contains(got, "no-transform") {
		t.Fatalf("Cache-Control = %q, want no-cache + no-transform", got)
	}

	events, err := parseTraceSSEStream(resp.Body)
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}

	// Skim the event sequence. Expected order:
	//   start, hop, hop, hop, result, end
	// We don't pin exact event count past the minimum because
	// heartbeat comments + race-induced re-orderings of the
	// runner-vs-handler goroutines could reasonably produce
	// extra harmless artefacts.
	gotEvents := make([]string, 0, len(events))
	for _, e := range events {
		gotEvents = append(gotEvents, e.Event)
	}
	t.Logf("event sequence: %v", gotEvents)

	if len(gotEvents) < 6 {
		t.Fatalf("expected >= 6 events, got %d: %v", len(gotEvents), gotEvents)
	}
	if gotEvents[0] != "start" {
		t.Errorf("event[0] = %q, want start", gotEvents[0])
	}

	hopCount := 0
	resultIdx := -1
	endIdx := -1
	for i, ev := range gotEvents {
		switch ev {
		case "hop":
			hopCount++
		case "result":
			resultIdx = i
		case "end":
			endIdx = i
		}
	}
	if hopCount != 3 {
		t.Errorf("expected 3 hop events, got %d", hopCount)
	}
	if resultIdx == -1 || endIdx == -1 || resultIdx >= endIdx {
		t.Errorf("expected result before end, got result=%d end=%d", resultIdx, endIdx)
	}

	// Spot-check payload shapes.
	for _, e := range events {
		switch e.Event {
		case "start":
			var d struct {
				Target string `json:"target"`
				Cycles int    `json:"cycles"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("start data malformed: %v (data=%s)", err, e.Data)
			}
			if d.Target != "8.8.8.8" {
				t.Errorf("start.target = %q, want 8.8.8.8", d.Target)
			}
			if d.Cycles <= 0 {
				t.Errorf("start.cycles = %d, want positive", d.Cycles)
			}
		case "hop":
			var d struct {
				Cycle       int                `json:"cycle"`
				TotalCycles int                `json:"total_cycles"`
				Hops        []collector.MTRHop `json:"hops"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("hop data malformed: %v (data=%s)", err, e.Data)
			}
			if d.Cycle <= 0 || d.TotalCycles <= 0 {
				t.Errorf("hop fields invalid: cycle=%d total=%d", d.Cycle, d.TotalCycles)
			}
		case "result":
			// Spot-check that the result is the canonical
			// ServiceCheckResult shape — same fields the sync
			// /test endpoint produces. We don't pin status here
			// because RunCheck applies thresholds; just assert
			// type + structure.
			var d map[string]any
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("result data malformed: %v (data=%s)", err, e.Data)
			}
			if _, ok := d["status"]; !ok {
				t.Errorf("result missing status field; data=%s", e.Data)
			}
		case "end":
			var d struct {
				DurationSeconds float64 `json:"duration_seconds"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("end data malformed: %v", err)
			}
		}
	}
}

// TestServiceChecksTestStream_NonTraceNonSpeedReturns400 — type=http
// (and other types that are neither traceroute nor speed) must be
// rejected with 400 + a hint pointing at the synchronous endpoint.
//
// The error message MUST now mention BOTH supported streaming
// types (traceroute, speed) so direct curl / external tooling
// callers see the full set of streamable types in the rejection
// reason. Issue #318 — replaces the old _NonTraceReturns400 guard.
func TestServiceChecksTestStream_NonTraceNonSpeedReturns400(t *testing.T) {
	srv := newSettingsTestServer()
	resp := postTestStream(t, srv, map[string]any{
		"name":   "http-bad",
		"type":   "http",
		"target": "http://example.com",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "/api/v1/service-checks/test") {
		t.Errorf("expected hint pointing at /api/v1/service-checks/test, got: %s", body)
	}
	if !strings.Contains(string(body), "traceroute") {
		t.Errorf("expected hint mentioning traceroute, got: %s", body)
	}
	if !strings.Contains(string(body), "speed") {
		t.Errorf("expected hint mentioning speed (issue #318 — both supported types must be enumerated), got: %s", body)
	}
}

// TestServiceChecksTestStream_CtxCancelKillsRunner — closing the
// EventSource client-side must promptly cancel the runner ctx.
// We assert by injecting a runner that blocks on ctx.Done() and
// verifying that the function returns within a small budget after
// the client closes the connection.
func TestServiceChecksTestStream_CtxCancelKillsRunner(t *testing.T) {
	srv := newSettingsTestServer()

	runnerExited := make(chan struct{})
	srv.streamingTracerouteRunner = func(ctx context.Context, _ string, _ int) (<-chan collector.StreamingMTRUpdate, <-chan collector.StreamingTraceFinal) {
		updates := make(chan collector.StreamingMTRUpdate)
		final := make(chan collector.StreamingTraceFinal, 1)
		go func() {
			defer close(updates)
			defer close(final)
			defer close(runnerExited)
			<-ctx.Done()
			final <- collector.StreamingTraceFinal{RunErr: ctx.Err()}
		}()
		return updates, final
	}

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"name":   "trace-cancel",
		"type":   "traceroute",
		"target": "8.8.8.8",
	})

	// Use a request with cancellable context so we can simulate
	// EventSource.close() at the wire level.
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/service-checks/test-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	// Read just enough to confirm the start event arrived (proving
	// the handler entered the loop and the runner is in flight),
	// then cancel.
	br := bufio.NewReader(resp.Body)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Read a few lines until we see start, then return.
		for i := 0; i < 50; i++ {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if strings.Contains(line, "event: start") {
				return
			}
		}
	}()
	wg.Wait()

	cancel()
	resp.Body.Close()

	select {
	case <-runnerExited:
		// Expected — ctx cancellation propagated, runner exited.
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not exit within 2s of client cancel — ctx cancellation not propagating")
	}
}

// TestServiceChecksTestStream_UpdatesCloseBeforeFinal — pins the
// exact channel-ordering edge case that caused the v0.9.15-rc1 CI
// hang. Drives the runner to close `updates` BEFORE emitting
// `final` so the handler's outer select observes the
// `case u, ok := <-updates: if !ok { updates = nil }` branch
// FIRST, and only then sees `final`. Without the nil-channel
// guard, the post-final drain `for u := range updates` ranges
// over a nil channel and blocks forever.
//
// Local dev boxes hit this rarely because Go's select is
// pseudo-random and a small runner queue can complete in any
// order. CI's slower scheduler made the bad ordering reliably
// reproducible. This test pins the bad ordering deterministically
// by using a stepped-channel runner.
func TestServiceChecksTestStream_UpdatesCloseBeforeFinal(t *testing.T) {
	deadline := assertCompletesWithin(t, 5*time.Second)
	defer deadline()

	srv := newSettingsTestServer()
	srv.streamingTracerouteRunner = func(_ context.Context, target string, _ int) (<-chan collector.StreamingMTRUpdate, <-chan collector.StreamingTraceFinal) {
		updates := make(chan collector.StreamingMTRUpdate, 1)
		final := make(chan collector.StreamingTraceFinal)
		// Push exactly one update, close updates immediately, THEN
		// emit final on a separate goroutine after a short pause.
		// This sequence forces the handler to:
		//   1. select pulls the buffered update.
		//   2. select pulls the close signal → updates set to nil.
		//   3. select pulls final → enters drain branch.
		// Without the nil-channel guard step 3 ranges over nil
		// and blocks forever.
		updates <- collector.StreamingMTRUpdate{
			Cycle:      1,
			TotalCycle: 1,
			Hops:       []collector.MTRHop{{Count: 1, Host: "10.0.0.1", AvgMs: 0.5}},
		}
		close(updates)
		go func() {
			// Tiny pause so the handler definitely processes the
			// updates close before final arrives — guarantees the
			// nil-assignment branch fires.
			time.Sleep(50 * time.Millisecond)
			final <- collector.StreamingTraceFinal{
				Result: &collector.MTRResult{
					Target: target,
					Hops:   []collector.MTRHop{{Count: 1, Host: "10.0.0.1", AvgMs: 0.5}},
				},
			}
			close(final)
		}()
		return updates, final
	}

	resp := postTestStream(t, srv, map[string]any{
		"name":   "trace-ordering",
		"type":   "traceroute",
		"target": "8.8.8.8",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	events, err := parseTraceSSEStream(resp.Body)
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}
	gotEnd := false
	for _, e := range events {
		if e.Event == "end" {
			gotEnd = true
			break
		}
	}
	if !gotEnd {
		t.Fatalf("expected terminal end event; sequence: %v", eventNames(events))
	}
}

// assertCompletesWithin returns a stop function that, when invoked,
// cancels the deadline timer. If the deadline expires before stop
// is called the goroutine kills the process with a stack dump so
// CI shows where the test was hung instead of timing out 10
// minutes later. Mirrors the discipline added to the speedtest SSE
// tests after the v0.9.11-rc UAT cycles.
func assertCompletesWithin(t *testing.T, d time.Duration) func() {
	t.Helper()
	timer := time.AfterFunc(d, func() {
		// Print every goroutine's stack so the cause of the hang
		// is obvious in the failure log, then fail the test.
		// We can't call t.Fatal from a non-test goroutine; the
		// best alternative is panic which fails the test cleanly.
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		panic(fmt.Sprintf("test exceeded %s deadline; goroutine dump:\n%s", d, buf[:n]))
	})
	return func() { timer.Stop() }
}

// eventNames is a small helper for diagnostic output in failure
// messages — turns a slice of sseEvent into a flat slice of event
// names so the failure log shows the wire-level sequence.
func eventNames(events []sseEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Event
	}
	return out
}

// TestServiceChecksHTML_TestStreamWiring — settings.html JS must
// invoke /api/v1/service-checks/test-stream for type=trace Test
// buttons. Other types use the existing sync /test endpoint.
func TestServiceChecksHTML_TestStreamWiring(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("templates", "settings.html"))
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	data := string(raw)
	if !strings.Contains(data, "/api/v1/service-checks/test-stream") {
		t.Error("settings.html missing fetch to /api/v1/service-checks/test-stream — Test button traceroute SSE wiring not present")
	}
	// Sanity: the trace branch must reference SSE event names. We
	// use fetch + ReadableStream (POST body required, EventSource
	// is GET-only) so the consumer must hand-parse the wire format.
	if !strings.Contains(data, `"hop"`) && !strings.Contains(data, "event: hop") {
		t.Error("settings.html missing reference to SSE 'hop' event in trace Test button branch")
	}
}

// TestServiceChecksHTML_TestStreamSpeedWiring — settings.html JS
// must also branch type=speed Test buttons through the streaming
// endpoint, with the speed-specific SSE event names ("sample" /
// "phase_change") referenced in the dispatcher. Issue #318.
func TestServiceChecksHTML_TestStreamSpeedWiring(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("templates", "settings.html"))
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	data := string(raw)
	// The speed branch must dispatch via the same /test-stream URL
	// the trace branch uses (one shared endpoint, switch on type
	// server-side — see handleTestServiceCheckStream).
	if !strings.Contains(data, "/api/v1/service-checks/test-stream") {
		t.Fatal("settings.html missing /test-stream URL — Test button SSE wiring not present at all")
	}
	// Speed-branch trigger: the JS must check sc.type === "speed"
	// and route to a new SSE handler. The exact identifier is
	// runSpeedTestStream (mirrors the existing runTraceTestStream
	// for traceroute).
	if !strings.Contains(data, "runSpeedTestStream") {
		t.Error("settings.html missing runSpeedTestStream — type=speed Test button branch not wired to SSE")
	}
	if !strings.Contains(data, `sc.type === "speed"`) {
		t.Error("settings.html missing `sc.type === \"speed\"` branch — speed Test button still uses sync /test endpoint")
	}
	// Speed-specific SSE events the dispatcher must handle.
	if !strings.Contains(data, `"sample"`) {
		t.Error("settings.html missing reference to SSE 'sample' event for speed Test button branch")
	}
	if !strings.Contains(data, `"phase_change"`) {
		t.Error("settings.html missing reference to SSE 'phase_change' event for speed Test button branch")
	}
}

// TestServiceChecksTestStream_SpeedHappyPath — type=speed request
// produces start + per-phase phase_change + sample × N + result +
// end events with parseable JSON payloads. Phase derivation must
// produce one phase_change per phase transition, NOT per sample.
//
// 5s deadline standard is the v0.9.15-rc1 burn discipline: any test
// exercising goroutine ordering or channel-close timing must use
// the helper so a deadlock fails loud-and-fast instead of waiting
// out the default 10-min go test timeout.
func TestServiceChecksTestStream_SpeedHappyPath(t *testing.T) {
	deadline := assertCompletesWithin(t, 5*time.Second)
	defer deadline()

	srv := newSettingsTestServer()

	// Inject a fake streaming runner that emits 4 samples across 3
	// phases (latency, download×2, upload) then finalises with a
	// canonical SpeedTestResult.
	srv.streamingSpeedTestRunner = func(_ context.Context) (<-chan collector.SpeedTestSample, <-chan collector.StreamingSpeedFinal) {
		updates := make(chan collector.SpeedTestSample, 8)
		final := make(chan collector.StreamingSpeedFinal, 1)
		go func() {
			defer close(updates)
			defer close(final)
			now := time.Now()
			updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseLatency, At: now, LatencyMs: 5.0}
			updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseDownload, At: now.Add(time.Second), Mbps: 100.0}
			updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseDownload, At: now.Add(2 * time.Second), Mbps: 200.0}
			updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseUpload, At: now.Add(3 * time.Second), Mbps: 50.0}
			final <- collector.StreamingSpeedFinal{
				Result: &internal.SpeedTestResult{
					DownloadMbps: 200.0,
					UploadMbps:   50.0,
					LatencyMs:    5.0,
					Engine:       internal.SpeedTestEngineSpeedTestGo,
				},
			}
		}()
		return updates, final
	}

	resp := postTestStream(t, srv, map[string]any{
		"name":   "speed-stream",
		"type":   "speed",
		"target": "speedtest",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream prefix", got)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "no-cache") || !strings.Contains(got, "no-transform") {
		t.Fatalf("Cache-Control = %q, want no-cache + no-transform", got)
	}

	events, err := parseTraceSSEStream(resp.Body)
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}

	gotEvents := make([]string, 0, len(events))
	for _, e := range events {
		gotEvents = append(gotEvents, e.Event)
	}
	t.Logf("event sequence: %v", gotEvents)

	if gotEvents[0] != "start" {
		t.Errorf("event[0] = %q, want start", gotEvents[0])
	}

	phaseCount := 0
	sampleCount := 0
	resultIdx := -1
	endIdx := -1
	for i, ev := range gotEvents {
		switch ev {
		case "phase_change":
			phaseCount++
		case "sample":
			sampleCount++
		case "result":
			resultIdx = i
		case "end":
			endIdx = i
		}
	}
	if phaseCount != 3 {
		t.Errorf("expected 3 phase_change events (one per phase transition), got %d", phaseCount)
	}
	if sampleCount != 4 {
		t.Errorf("expected 4 sample events, got %d", sampleCount)
	}
	if resultIdx == -1 || endIdx == -1 || resultIdx >= endIdx {
		t.Errorf("expected result before end, got result=%d end=%d", resultIdx, endIdx)
	}

	// Per-event payload spot-checks.
	for _, e := range events {
		switch e.Event {
		case "phase_change":
			var d struct {
				Phase       string `json:"phase"`
				PhaseIndex  int    `json:"phase_index"`
				TotalPhases int    `json:"total_phases"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("phase_change data malformed: %v (data=%s)", err, e.Data)
			}
			if d.Phase == "" || d.PhaseIndex <= 0 || d.TotalPhases != 3 {
				t.Errorf("phase_change fields invalid: %+v", d)
			}
		case "sample":
			var d struct {
				Phase       string  `json:"phase"`
				SampleIndex int     `json:"sample_index"`
				Mbps        float64 `json:"mbps"`
				LatencyMs   float64 `json:"latency_ms"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("sample data malformed: %v (data=%s)", err, e.Data)
			}
			if d.Phase == "" {
				t.Errorf("sample missing phase: data=%s", e.Data)
			}
		case "result":
			// Canonical ServiceCheckResult shape — same fields the sync
			// /test endpoint produces. The speed runner with no
			// contracted thresholds should land status=up.
			var d map[string]any
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("result data malformed: %v (data=%s)", err, e.Data)
			}
			if _, ok := d["status"]; !ok {
				t.Errorf("result missing status field; data=%s", e.Data)
			}
			// download_mbps and upload_mbps must be propagated from
			// the streamed engine result so the JS toast can show
			// real numbers.
			if dl, ok := d["download_mbps"].(float64); !ok || dl <= 0 {
				t.Errorf("result.download_mbps missing or zero: %v", d["download_mbps"])
			}
			if ul, ok := d["upload_mbps"].(float64); !ok || ul <= 0 {
				t.Errorf("result.upload_mbps missing or zero: %v", d["upload_mbps"])
			}
		case "end":
			var d struct {
				DurationSeconds float64 `json:"duration_seconds"`
			}
			if err := json.Unmarshal([]byte(e.Data), &d); err != nil {
				t.Errorf("end data malformed: %v", err)
			}
		}
	}
}

// TestServiceChecksTestStream_SpeedCtxCancelKillsRunner — closing the
// EventSource client-side must promptly cancel the runner ctx for
// type=speed checks too. We assert by injecting a runner that blocks
// on ctx.Done() and verifying the function returns within a small
// budget after the client closes the connection.
func TestServiceChecksTestStream_SpeedCtxCancelKillsRunner(t *testing.T) {
	deadline := assertCompletesWithin(t, 8*time.Second)
	defer deadline()

	srv := newSettingsTestServer()

	runnerExited := make(chan struct{})
	srv.streamingSpeedTestRunner = func(ctx context.Context) (<-chan collector.SpeedTestSample, <-chan collector.StreamingSpeedFinal) {
		updates := make(chan collector.SpeedTestSample)
		final := make(chan collector.StreamingSpeedFinal, 1)
		go func() {
			defer close(updates)
			defer close(final)
			defer close(runnerExited)
			<-ctx.Done()
			final <- collector.StreamingSpeedFinal{RunErr: ctx.Err()}
		}()
		return updates, final
	}

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"name":   "speed-cancel",
		"type":   "speed",
		"target": "speedtest",
	})

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/service-checks/test-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	br := bufio.NewReader(resp.Body)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if strings.Contains(line, "event: start") {
				return
			}
		}
	}()
	wg.Wait()

	cancel()
	resp.Body.Close()

	select {
	case <-runnerExited:
	case <-time.After(2 * time.Second):
		t.Fatal("speed runner did not exit within 2s of client cancel — ctx cancellation not propagating")
	}
}

// TestServiceChecksTestStream_SpeedUpdatesCloseBeforeFinal pins the
// nil-channel range edge case for the speed branch. Same shape as
// the trace counterpart: forces the runner to close `updates`
// BEFORE emitting `final` so the handler's outer select observes
// the close-of-updates first, then final. Without the nil guard,
// the post-final drain ranges over nil and deadlocks.
func TestServiceChecksTestStream_SpeedUpdatesCloseBeforeFinal(t *testing.T) {
	deadline := assertCompletesWithin(t, 5*time.Second)
	defer deadline()

	srv := newSettingsTestServer()
	srv.streamingSpeedTestRunner = func(_ context.Context) (<-chan collector.SpeedTestSample, <-chan collector.StreamingSpeedFinal) {
		updates := make(chan collector.SpeedTestSample, 1)
		final := make(chan collector.StreamingSpeedFinal)
		updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseDownload, Mbps: 50.0}
		close(updates)
		go func() {
			time.Sleep(50 * time.Millisecond)
			final <- collector.StreamingSpeedFinal{Result: &internal.SpeedTestResult{
				DownloadMbps: 50.0,
				UploadMbps:   10.0,
				LatencyMs:    5.0,
			}}
			close(final)
		}()
		return updates, final
	}

	resp := postTestStream(t, srv, map[string]any{
		"name":   "speed-ordering",
		"type":   "speed",
		"target": "speedtest",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	events, err := parseTraceSSEStream(resp.Body)
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}
	gotEnd := false
	for _, e := range events {
		if e.Event == "end" {
			gotEnd = true
			break
		}
	}
	if !gotEnd {
		t.Fatalf("expected terminal end event; sequence: %v", eventNames(events))
	}
}

// TestServiceChecksTestStream_SpeedEngineErrorEmitsErrorEvent — when
// the runner fails before any samples (e.g. no speedtest tool
// available), the SSE stream must still emit a coherent event
// sequence: start → error → result (status=down) → end. The dedicated
// `error` event is what gives JS consumers a hook to render an
// engine-specific failure message before the canonical result event
// fires.
func TestServiceChecksTestStream_SpeedEngineErrorEmitsErrorEvent(t *testing.T) {
	deadline := assertCompletesWithin(t, 5*time.Second)
	defer deadline()

	srv := newSettingsTestServer()
	srv.streamingSpeedTestRunner = func(_ context.Context) (<-chan collector.SpeedTestSample, <-chan collector.StreamingSpeedFinal) {
		updates := make(chan collector.SpeedTestSample)
		final := make(chan collector.StreamingSpeedFinal, 1)
		go func() {
			defer close(updates)
			defer close(final)
			final <- collector.StreamingSpeedFinal{
				RunErr: errSpeedEngineUnavailable,
			}
		}()
		return updates, final
	}

	resp := postTestStream(t, srv, map[string]any{
		"name":   "speed-engine-fail",
		"type":   "speed",
		"target": "speedtest",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	events, err := parseTraceSSEStream(resp.Body)
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}

	gotError := false
	gotResult := false
	gotEnd := false
	for _, e := range events {
		switch e.Event {
		case "error":
			gotError = true
			if !strings.Contains(e.Data, "speedtest engine unavailable") {
				t.Errorf("error event message did not surface engine error: %s", e.Data)
			}
		case "result":
			gotResult = true
			// Result must reflect the failure — status != "up" and
			// an error message present.
			var d map[string]any
			_ = json.Unmarshal([]byte(e.Data), &d)
			if d["status"] == "up" {
				t.Errorf("result.status = up despite engine error; data=%s", e.Data)
			}
		case "end":
			gotEnd = true
		}
	}
	if !gotError {
		t.Errorf("expected error event after engine failure; sequence: %v", eventNames(events))
	}
	if !gotResult {
		t.Errorf("expected result event after engine error (not a stream abort); sequence: %v", eventNames(events))
	}
	if !gotEnd {
		t.Errorf("expected terminal end event; sequence: %v", eventNames(events))
	}
}

// errSpeedEngineUnavailable is a static error fixture used by the
// engine-error test above so the assertion can grep for a stable
// substring without scraping a wrapped errors.Is chain.
var errSpeedEngineUnavailable = errSpeedTestSentinel("speedtest engine unavailable: both engines offline")

type errSpeedTestSentinel string

func (e errSpeedTestSentinel) Error() string { return string(e) }
