package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
func TestServiceChecksTestStream_TraceHappyPath(t *testing.T) {
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

// TestServiceChecksTestStream_NonTraceReturns400 — type=http (and
// other non-trace types) must be rejected with 400 + a hint pointing
// at the synchronous endpoint.
func TestServiceChecksTestStream_NonTraceReturns400(t *testing.T) {
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
