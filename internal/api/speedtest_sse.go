// Package api — speedtest_sse.go implements the SSE (Server-Sent
// Events) endpoints for live speed-test progress streaming, per
// PRD #283 / issue #285 (slice 2 of the speed-test live-progress
// PRD).
//
// Two endpoints:
//
//   - POST /api/v1/speedtest/run — idempotent. Returns the test_id
//     of the in-flight test, kicking off a new one if none is
//     running. Multi-tab "Run now" + cron tick all converge on the
//     same in-flight test.
//
//   - GET /api/v1/speedtest/stream/{test_id} — text/event-stream.
//     Emits the documented event sequence (start, phase_change,
//     sample, result, end / error). Closes after end.
//
// SSE wire format is fixed by the PRD's "SSE event format" section:
// every event has an `event:` line, a `data:` line with JSON, and
// a blank line terminator. A trailing `event: end` closes the
// stream gracefully so the EventSource on the dashboard can call
// .close() without the browser auto-reconnecting.
//
// Auth: same-origin EventSource cannot send custom headers (browser
// limitation), so the api-key middleware exempts same-origin
// requests via the Referer check (existing behaviour). External
// API consumers can pass api_key= as a query param if they need
// to consume the stream from another origin.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal/livetest"
)

// speedtestRunResponse is the body of POST /api/v1/speedtest/run.
type speedtestRunResponse struct {
	TestID    int64     `json:"test_id"`
	StartedAt time.Time `json:"started_at"`
	Engine    string    `json:"engine,omitempty"`
}

// handleSpeedtestRun implements POST /api/v1/speedtest/run.
//
// Idempotent: a second concurrent call while a test is in flight
// returns the existing test_id, NOT a new one. This is what makes
// "click Run twice" / multi-tab transparent.
//
// Returns 503 if no LiveTestRegistry is wired (defensive — the
// scheduler should always have one in production).
func (s *Server) handleSpeedtestRun(w http.ResponseWriter, r *http.Request) {
	reg := s.liveTestRegistry()
	if reg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "live speed test registry not configured",
		})
		return
	}
	// Detach the runner from the request context. r.Context()
	// cancels as soon as the POST response is written, which would
	// kill the just-started runner before the first sample arrives.
	// The registry owns the test's lifetime; the runner needs a
	// context that outlives this handler call. Annotate with the
	// caller tag so livetest's structured logs differentiate
	// API-triggered tests from cron-triggered ones (issue #294
	// debugging hint).
	lt, err := reg.StartTest(livetest.WithCaller(context.Background(), "api"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, speedtestRunResponse{
		TestID:    lt.ID(),
		StartedAt: lt.StartedAt(),
		Engine:    lt.Engine(),
	})
}

// handleSpeedtestStream implements GET /api/v1/speedtest/stream/{test_id}.
//
// Wire format (PRD #283):
//
//	event: start
//	data: {"test_id":...,"started_at":"...","engine":"speedtest_go"}
//
//	event: phase_change
//	data: {"phase":"download","phase_index":1,"total_phases":3}
//
//	event: sample
//	data: {"phase":"download","sample_index":0,"ts":"...","mbps":723.4,"latency_ms":8.2}
//
//	event: result
//	data: {"download_mbps":920.5,...}
//
//	event: end
//	data: {"test_id":...,"duration_seconds":31.4}
//
// The handler tracks the current phase per-stream so it can derive
// phase_change events from sample-phase transitions (the runner
// emits per-sample data; the SSE wire derives the change events).
//
// Returns 404 if test_id doesn't match the in-flight test.
func (s *Server) handleSpeedtestStream(w http.ResponseWriter, r *http.Request) {
	reg := s.liveTestRegistry()
	if reg == nil {
		http.Error(w, "live speed test registry not configured", http.StatusServiceUnavailable)
		return
	}
	idStr := chi.URLParam(r, "test_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid test_id", http.StatusBadRequest)
		return
	}
	lt, ok := reg.GetLive(id)
	if !ok {
		http.Error(w, "test not found or already completed", http.StatusNotFound)
		return
	}

	// SSE headers. Cache-Control: no-cache, no-transform prevents
	// intermediaries from buffering OR transforming (e.g. Cloudflare
	// auto-gzip would buffer chunks until enough bytes accumulate).
	// X-Accel-Buffering: no instructs nginx-style proxies to flush
	// per chunk. Connection: keep-alive is implied by HTTP/1.1 but
	// stated explicitly for clarity. Content-Type carries an explicit
	// charset so any content-sniffing proxy doesn't second-guess the
	// MIME type and accidentally enable compression / transformation.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	// Disable per-connection write deadlines so a 30-60s test
	// doesn't get killed by the router-wide Timeout middleware.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe BEFORE writing any events so we're guaranteed to
	// see every sample emitted from this point forward + the
	// replay of any samples already buffered.
	sub := lt.Subscribe()

	// Emit start event.
	startData := map[string]any{
		"test_id":    lt.ID(),
		"started_at": lt.StartedAt().Format(time.RFC3339Nano),
		"engine":     lt.Engine(),
	}
	if !writeSSEEvent(w, flusher, "start", startData) {
		return
	}

	// Initial padding to defeat first-N-bytes proxy buffers. Some
	// intermediaries (Cloudflare, certain CDN tunnels, scanning
	// gateways) hold the first chunk(s) of a streaming response
	// until N bytes have accumulated — typically 2KB. Without
	// padding, the SSE consumer on the dashboard sees the `start`
	// event but no progressive sample events: each per-event chunk
	// is small (~150 bytes) so the proxy keeps buffering them
	// until the test completes. A 2KB comment block (which the
	// SSE spec ignores — comment lines start with ":") fills that
	// initial buffer immediately, so subsequent flushes get
	// delivered chunk-by-chunk. Issue surfaced in v0.9.11-rc3 UAT
	// against the live host through Cloudflare; chunked encoding
	// alone is not enough.
	if !writeSSEPadding(w, flusher) {
		return
	}

	currentPhase := ""
	phaseIndex := 0
	totalPhases := 3 // latency, download, upload — PRD-pinned ordering
	sampleIndex := 0

	// Heartbeat ticker — sends a `: keepalive` SSE comment every
	// second to force flushes through any chunked-encoding-aware
	// intermediary that might still coalesce small writes. The
	// browser ignores comment lines per the SSE spec, so this is
	// invisible to the dashboard. Without heartbeats, a test phase
	// that emits no samples for a few seconds (e.g. between latency
	// samples on a fast link) would let the proxy re-engage its
	// buffer. Stop on defer so the ticker doesn't outlive the
	// handler.
	heartbeat := time.NewTicker(1 * time.Second)
	defer heartbeat.Stop()

	// Pump samples until the channel closes (test ended OR slow-
	// client drop). Either way we then emit the terminal event.
	clientGone := r.Context().Done()
loop:
	for {
		select {
		case <-heartbeat.C:
			if !writeSSEHeartbeat(w, flusher) {
				return
			}
		case s, ok := <-sub:
			if !ok {
				break loop
			}
			// Derive a phase_change event when the phase
			// transitions. The runner doesn't emit explicit
			// phase events; the SSE wire does the derivation.
			if string(s.Phase) != currentPhase {
				phaseIndex++
				currentPhase = string(s.Phase)
				if !writeSSEEvent(w, flusher, "phase_change", map[string]any{
					"phase":        currentPhase,
					"phase_index":  phaseIndex,
					"total_phases": totalPhases,
				}) {
					return
				}
			}
			payload := map[string]any{
				"phase":        currentPhase,
				"sample_index": sampleIndex,
				"ts":           s.At.Format(time.RFC3339Nano),
				"mbps":         s.Mbps,
				"latency_ms":   s.LatencyMs,
			}
			sampleIndex++
			if !writeSSEEvent(w, flusher, "sample", payload) {
				return
			}
		case <-clientGone:
			// Browser closed the EventSource. Don't bother
			// writing terminal events — the connection is
			// already dead.
			return
		}
	}

	// Test ended. Emit result (or error) + end.
	if err := lt.Err(); err != nil {
		_ = writeSSEEvent(w, flusher, "error", map[string]any{
			"message": err.Error(),
		})
	} else if res := lt.Result(); res != nil {
		_ = writeSSEEvent(w, flusher, "result", res)
	}
	duration := time.Since(lt.StartedAt()).Seconds()
	_ = writeSSEEvent(w, flusher, "end", map[string]any{
		"test_id":          lt.ID(),
		"duration_seconds": duration,
	})
}

// writeSSEEvent writes a single SSE-formatted event. Returns false
// on write failure (caller should abort the stream).
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data any) bool {
	buf, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, buf); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// ssePaddingBytes is the SSE comment block written immediately
// after the start event to defeat first-N-bytes proxy buffering.
// 2048 bytes of dots is chosen empirically as enough to clear
// the buffer threshold of common intermediaries (Cloudflare's
// auto-buffering layer, scanning gateways, CDN tunnels) while
// staying small enough to not feel wasteful. The leading ":"
// makes this a valid SSE comment line per the spec — the
// browser's EventSource parser ignores it entirely.
var ssePaddingBytes = func() []byte {
	const padSize = 2048
	b := make([]byte, padSize+3)
	b[0] = ':'
	b[1] = ' '
	for i := 2; i < padSize+1; i++ {
		b[i] = '.'
	}
	b[padSize+1] = '\n'
	b[padSize+2] = '\n'
	return b
}()

// writeSSEPadding writes the initial padding comment block. Called
// once after the start event to release first-N-bytes proxy buffers
// so subsequent per-sample chunks stream rather than accumulate.
func writeSSEPadding(w http.ResponseWriter, flusher http.Flusher) bool {
	if _, err := w.Write(ssePaddingBytes); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// writeSSEHeartbeat writes a periodic SSE comment line. Called
// once per second from the handler's select loop to force
// chunked-encoding intermediaries to keep flushing buffered
// per-sample chunks rather than coalescing them. Browsers ignore
// comment lines per the SSE spec.
func writeSSEHeartbeat(w http.ResponseWriter, flusher http.Flusher) bool {
	if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// liveTestRegistry returns the registry wired on the scheduler, or
// nil if not configured. Indirected through a method so tests can
// inject a fake without reaching into scheduler internals.
//
// Tests inject via testLiveTestRegistry (Server field). Production
// reads from s.scheduler.LiveTestRegistry() which is wired by
// cmd/nas-doctor/main.go on startup.
func (s *Server) liveTestRegistry() livetest.Registry {
	if s.testLiveTestRegistry != nil {
		return s.testLiveTestRegistry
	}
	if s.scheduler == nil {
		return nil
	}
	return s.scheduler.LiveTestRegistry()
}
