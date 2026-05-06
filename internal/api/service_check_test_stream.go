// service_check_test_stream.go — SSE streaming variant of the
// service-checks Test button (issue #224).
//
// Scope decision (deliberately narrow): this slice ONLY supports
// type=traceroute. The dashboard's Speed Test card already has a
// dedicated SSE live-progress strip (v0.9.11), and adding a second
// streaming surface for the type=speed Test button is out of scope
// for #224's first PR. Non-trace requests are rejected with 400 and
// a hint pointing at the existing synchronous /service-checks/test
// endpoint, so the JS Test-button wiring can fall back cleanly.
//
// Wire format mirrors the v0.9.11 speedtest SSE contract:
//
//	event: start
//	data: {"target":"8.8.8.8","cycles":10,"started_at":"..."}
//
//	event: hop
//	data: {"cycle":1,"total_cycles":10,"hops":[{"count":1,"host":"...","Loss%":0,"Avg":...}, ...]}
//
//	event: result
//	data: <ServiceCheckResult shape — same as POST /test sync endpoint>
//
//	event: end
//	data: {"duration_seconds":12.4}
//
// The terminal `result` event's payload is byte-identical to the
// synchronous /test endpoint's response so the JS renderer can reuse
// renderServiceCheckDetails verbatim. The `hop` event's `hops` array
// is cumulative — every event carries the full hop table so far,
// not a delta — to keep the renderer code path identical for live
// vs final state.
//
// Process-group SIGKILL on context cancellation (v0.9.14 #304
// pattern) is delegated to RunStreamingMTR. When the EventSource
// closes client-side, ctx fires; the streaming runner's goroutine
// sees ctx.Done(), returns, and the writer goroutine here exits.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
)

// streamingTracerouteCycles is the cycle count used by the streaming
// Test endpoint. Matches the synchronous Test button (10 — richer
// sample than the scheduler's 5) so users get a comparable picture
// of network health from both surfaces.
const streamingTracerouteCycles = 10

// handleTestServiceCheckStream is the SSE variant of
// handleTestServiceCheck. POST a ServiceCheckConfig with type=trace,
// receive a text/event-stream response with one `start` event,
// per-cycle `hop` events with cumulative hop tables, one terminal
// `result` event whose body matches the sync endpoint's response,
// and a final `end` event.
//
// POST /api/v1/service-checks/test-stream
//
// Non-trace types: 400 with a hint pointing at the sync endpoint.
// Issue #224 narrows this slice to traceroute; speed-type was
// considered but its streaming surface is already covered by the
// dashboard's /api/v1/speedtest/run + /stream pair. A future PR can
// add type=speed support here if the settings-page Test button
// wants live progress too.
func (s *Server) handleTestServiceCheckStream(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer r.Body.Close()

	var cfg internal.ServiceCheckConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if err := normalizeServiceCheckConfig(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Type != internal.ServiceCheckTraceroute {
		// Non-trace types: redirect via a 400 with a hint. The
		// settings.html JS only opens this stream for type=trace
		// configs, so this path is defensive — exercised by direct
		// curl / external tooling that POSTs the wrong shape.
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("/test-stream only supports type=traceroute (got %q); use POST /api/v1/service-checks/test for synchronous types", cfg.Type),
		})
		return
	}

	// SSE headers — same defensive set as speedtest_sse.go. Explicit
	// charset prevents content-sniffing proxies from second-guessing
	// the MIME and re-engaging compression. no-transform discourages
	// CDN auto-gzip from buffering chunks. X-Accel-Buffering: no is
	// a hint to nginx-style intermediaries to flush per chunk.
	// Connection: keep-alive is implied by HTTP/1.1 but stated
	// explicitly so reviewers reading the headers understand intent.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	// Disable per-connection write deadlines — a 10-cycle traceroute
	// can take 30-60s on slow paths, which would otherwise trip the
	// http.Server WriteTimeout=30s baseline set in main.go.
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Resolve the streaming runner — production wiring uses
	// collector.RunStreamingMTR; tests inject a fake via
	// s.streamingTracerouteRunner so they don't need an actual mtr
	// binary. Mirrors the existing tracerouteRunner seam used by
	// the sync /test endpoint.
	runner := s.streamingTracerouteRunner
	if runner == nil {
		runner = collector.RunStreamingMTR
	}

	target := cfg.Target
	startedAt := time.Now()

	// Emit the start event before kicking off the runner so the
	// dashboard sees something immediately (defeats the perceived
	// "dead spinner" problem #224 was filed to address).
	if !writeSSEEvent(w, flusher, "start", map[string]any{
		"target":     target,
		"cycles":     streamingTracerouteCycles,
		"started_at": startedAt.Format(time.RFC3339Nano),
	}) {
		return
	}

	// Initial padding to defeat first-N-bytes proxy buffers — same
	// rationale as speedtest_sse.go (Cloudflare et al. hold the
	// first chunk until ~2KB has accumulated; without this the
	// per-cycle hop events stack up invisibly until the test ends).
	if !writeSSEPadding(w, flusher) {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	updates, final := runner(ctx, target, streamingTracerouteCycles)

	// Heartbeat so chunk-coalescing intermediaries keep flushing
	// between cycles. Same pattern as speedtest_sse.go.
	heartbeat := time.NewTicker(1 * time.Second)
	defer heartbeat.Stop()

	clientGone := r.Context().Done()

	var lastResult *collector.MTRResult
	var runErr error

loop:
	for {
		select {
		case <-heartbeat.C:
			if !writeSSEHeartbeat(w, flusher) {
				return
			}
		case u, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			if !writeSSEEvent(w, flusher, "hop", map[string]any{
				"cycle":        u.Cycle,
				"total_cycles": u.TotalCycle,
				"hops":         u.Hops,
			}) {
				return
			}
		case fin, ok := <-final:
			if !ok {
				break loop
			}
			lastResult = fin.Result
			runErr = fin.RunErr
			// Drain any buffered updates that the runner pushed
			// before its terminal value — Go's select can
			// schedule `final` before pending `updates` when
			// both are ready, and we don't want the SSE
			// consumer to miss a hop event the runner emitted.
			//
			// CRITICAL: guard against `updates == nil`. When the
			// updates channel closes BEFORE final arrives in the
			// outer select (a legal ordering when the runner
			// closes both channels back-to-back via defers and
			// CI scheduling drains updates first), the
			// `case u, ok := <-updates: if !ok { updates = nil }`
			// branch above sets the local var to nil. Ranging
			// over a nil channel blocks forever — the original
			// handler bug that caused the v0.9.15-rc1 CI hang.
			if updates != nil {
				for u := range updates {
					if !writeSSEEvent(w, flusher, "hop", map[string]any{
						"cycle":        u.Cycle,
						"total_cycles": u.TotalCycle,
						"hops":         u.Hops,
					}) {
						return
					}
				}
			}
			break loop
		case <-clientGone:
			// Browser closed the EventSource — ctx cancellation
			// will propagate to the runner via the ctx wired into
			// the runner above; bail without writing terminal
			// events (the connection is already dead).
			return
		}
	}

	// Build the canonical ServiceCheckResult so the SSE wire's
	// terminal `result` event matches the sync endpoint's response
	// shape. We re-use scheduler's RunCheck path with an injected
	// runner that returns the streamed result — this guarantees
	// thresholds, MaxLossPct policy, status determination, and
	// the Details map are computed exactly as on the sync path.
	checker := scheduler.NewServiceChecker(s.store, s.logger)
	checker.SetCollectDetails(true)
	checker.SetTraceRunner(func(_ string, _ int) (*collector.MTRResult, error) {
		return lastResult, runErr
	})
	scResult := checker.RunCheck(cfg, time.Now().UTC())

	_ = writeSSEEvent(w, flusher, "result", scResult)
	_ = writeSSEEvent(w, flusher, "end", map[string]any{
		"duration_seconds": time.Since(startedAt).Seconds(),
	})
}
