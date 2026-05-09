// service_check_test_stream.go — SSE streaming variant of the
// service-checks Test button.
//
// Originally introduced for type=traceroute (issue #224, PR #317);
// extended in issue #318 to also support type=speed so the
// settings-page Test button for speed-type checks gets the same
// live-progress UX the dashboard's Speed Test card has had since
// v0.9.11. Other types continue to use the synchronous /test
// endpoint and are rejected here with a 400 hint.
//
// Wire format mirrors the v0.9.11 speedtest SSE contract verbatim
// where shared:
//
// Traceroute:
//
//	event: start
//	data: {"target":"8.8.8.8","cycles":10,"started_at":"..."}
//
//	event: hop
//	data: {"cycle":1,"total_cycles":10,"hops":[...]}    // cumulative
//
//	event: result
//	data: <ServiceCheckResult — same as POST /test sync endpoint>
//
//	event: end
//	data: {"duration_seconds":12.4}
//
// Speed:
//
//	event: start
//	data: {"target":"speedtest","started_at":"..."}
//
//	event: phase_change
//	data: {"phase":"download","phase_index":2,"total_phases":3}
//
//	event: sample
//	data: {"phase":"download","sample_index":7,"ts":"...","mbps":723.4,"latency_ms":0}
//
//	event: result
//	data: <ServiceCheckResult — same as POST /test sync endpoint>
//
//	event: error
//	data: {"message":"engine offline: ..."}             // only on engine error
//
//	event: end
//	data: {"duration_seconds":31.4}
//
// In both cases the terminal `result` event's payload is byte-
// identical to the synchronous /test endpoint's response so the JS
// renderer can reuse renderServiceCheckDetails verbatim. The
// per-type live events (`hop` for traceroute, `phase_change` +
// `sample` for speed) are designed so the JS dispatcher can branch
// purely on event name without knowing the check type.
//
// Process-group SIGKILL on context cancellation (v0.9.14 #304
// pattern) is delegated to the underlying runner. When the
// EventSource closes client-side, ctx fires; the runner sees
// ctx.Done(), terminates the subprocess (mtr / speedtest), returns,
// and the writer goroutine here exits.

package api

import (
	"context"
	"encoding/json"
	"errors"
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

// streamingSpeedTotalPhases pins the SSE wire's total_phases value
// for type=speed runs. Mirrors the dashboard SSE handler's PRD-
// pinned ordering: latency → download → upload.
const streamingSpeedTotalPhases = 3

// handleTestServiceCheckStream is the SSE variant of
// handleTestServiceCheck. POST a ServiceCheckConfig with a
// supported type, receive a text/event-stream response.
//
// Supported types:
//   - traceroute — per-cycle hop events
//   - speed      — phase_change + per-sample events
//
// All other types return 400 with a hint pointing at the synchronous
// /api/v1/service-checks/test endpoint. The settings.html JS only
// opens this stream for traceroute / speed configs, so the 400 is
// defensive — exercised by direct curl / external tooling that
// POSTs the wrong shape.
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

	switch cfg.Type {
	case internal.ServiceCheckTraceroute:
		s.streamTraceServiceCheck(w, r, cfg)
	case internal.ServiceCheckSpeed:
		s.streamSpeedServiceCheck(w, r, cfg)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("/test-stream supports type=traceroute or type=speed (got %q); use POST /api/v1/service-checks/test for synchronous types", cfg.Type),
		})
	}
}

// writeStreamHeaders applies the SSE defensive header set used by
// every streaming endpoint in this package — explicit charset, no-
// cache+no-transform Cache-Control, X-Accel-Buffering hint for
// nginx-style proxies, explicit keep-alive — and disables the per-
// connection write deadline so a 30-60s test isn't killed by the
// router-wide WriteTimeout. Returns the http.Flusher (always
// non-nil; true if streaming is supported, false otherwise — caller
// must abort).
func writeStreamHeaders(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return nil, false
	}
	return flusher, true
}

// streamTraceServiceCheck handles type=traceroute requests on the
// /test-stream endpoint. Runs the streaming MTR runner (production
// or injected fake), emits per-cycle cumulative hop events, and
// terminates with a canonical ServiceCheckResult built via
// scheduler.RunCheck so thresholds + Details map are byte-identical
// to the sync endpoint's response.
func (s *Server) streamTraceServiceCheck(w http.ResponseWriter, r *http.Request, cfg internal.ServiceCheckConfig) {
	flusher, ok := writeStreamHeaders(w)
	if !ok {
		return
	}

	runner := s.streamingTracerouteRunner
	if runner == nil {
		runner = collector.RunStreamingMTR
	}

	target := cfg.Target
	startedAt := time.Now()

	if !writeSSEEvent(w, flusher, "start", map[string]any{
		"target":     target,
		"cycles":     streamingTracerouteCycles,
		"started_at": startedAt.Format(time.RFC3339Nano),
	}) {
		return
	}
	if !writeSSEPadding(w, flusher) {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	updates, final := runner(ctx, target, streamingTracerouteCycles)

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
			// Drain buffered updates after seeing final — Go's
			// select can schedule `final` before pending
			// `updates` when both are ready. CRITICAL: guard
			// against updates==nil to avoid the v0.9.15-rc1
			// nil-channel range deadlock (legal channel-close
			// ordering hits the bad path on CI Linux).
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
			return
		}
	}

	// Build the canonical ServiceCheckResult so the terminal
	// `result` event matches the sync endpoint's shape.
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

// streamSpeedServiceCheck handles type=speed requests on the
// /test-stream endpoint. Runs the streaming speed-test runner
// (production composite of speedtest-go primary + Ookla CLI
// fallback, or an injected fake), forwards per-phase samples as
// SSE `sample` events with derived `phase_change` events on
// transitions, and terminates with a canonical ServiceCheckResult
// built via scheduler.RunCheck so thresholds (ContractedDownMbps
// / ContractedUpMbps / MarginPct + three-state up/degraded/down
// logic) are applied identically to the sync endpoint.
//
// Engine errors (no speedtest tool available, network-bound
// failures, etc.) are surfaced as a separate `error` SSE event
// before the terminal `result` event, mirroring the dashboard
// speedtest_sse.go contract. The result event still fires (with
// status=down, error message in Error field) so the JS renderer
// can produce a coherent toast even on the failure path.
//
// Issue #318.
func (s *Server) streamSpeedServiceCheck(w http.ResponseWriter, r *http.Request, cfg internal.ServiceCheckConfig) {
	flusher, ok := writeStreamHeaders(w)
	if !ok {
		return
	}

	runner := s.streamingSpeedTestRunner
	if runner == nil {
		runner = collector.RunStreamingSpeedTest
	}

	startedAt := time.Now()
	// target is decorative for speed checks — most configs ship
	// with cfg.Target="speedtest" or empty. Echo whatever was
	// configured so the start event is honest about what the
	// user submitted.
	target := cfg.Target
	if target == "" {
		target = "speedtest"
	}

	if !writeSSEEvent(w, flusher, "start", map[string]any{
		"target":     target,
		"started_at": startedAt.Format(time.RFC3339Nano),
	}) {
		return
	}
	if !writeSSEPadding(w, flusher) {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	updates, final := runner(ctx)

	heartbeat := time.NewTicker(1 * time.Second)
	defer heartbeat.Stop()

	clientGone := r.Context().Done()

	currentPhase := ""
	phaseIndex := 0
	sampleIndex := 0

	emitSample := func(s collector.SpeedTestSample) bool {
		// Derive a phase_change event when the phase transitions —
		// the runner doesn't emit explicit phase events, the SSE
		// wire does the derivation. Mirrors speedtest_sse.go.
		if string(s.Phase) != currentPhase {
			phaseIndex++
			currentPhase = string(s.Phase)
			if !writeSSEEvent(w, flusher, "phase_change", map[string]any{
				"phase":        currentPhase,
				"phase_index":  phaseIndex,
				"total_phases": streamingSpeedTotalPhases,
			}) {
				return false
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
		return writeSSEEvent(w, flusher, "sample", payload)
	}

	var resultPtr *internal.SpeedTestResult
	var runErr error

loop:
	for {
		select {
		case <-heartbeat.C:
			if !writeSSEHeartbeat(w, flusher) {
				return
			}
		case sample, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			if !emitSample(sample) {
				return
			}
		case fin, ok := <-final:
			if !ok {
				break loop
			}
			resultPtr = fin.Result
			runErr = fin.RunErr
			// Drain buffered updates after seeing final — same
			// ordering guard as the trace branch. The nil-
			// channel guard is critical: a legal channel-close
			// ordering can leave updates set to nil after the
			// outer select observed close BEFORE final, and
			// ranging over a nil channel deadlocks. v0.9.15-
			// rc1 burn lesson.
			if updates != nil {
				for sample := range updates {
					if !emitSample(sample) {
						return
					}
				}
			}
			break loop
		case <-clientGone:
			return
		}
	}

	// Engine error: emit a dedicated `error` event before the
	// terminal `result` so JS consumers that want to render an
	// engine-specific failure message can do so without parsing
	// ServiceCheckResult.Error. Mirrors speedtest_sse.go.
	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		_ = writeSSEEvent(w, flusher, "error", map[string]any{
			"message": runErr.Error(),
		})
	}

	// Build the canonical ServiceCheckResult so the terminal
	// `result` event matches the sync endpoint's shape exactly.
	// We re-use scheduler.RunCheck via runSpeedCheckViaRunner —
	// SetSpeedTestRunner injects a stub that returns the streamed
	// result. RunCheck applies ContractedDownMbps / ContractedUp
	// Mbps thresholds + MarginPct + three-state status logic, and
	// stamps DownloadOK / UploadOK / Details exactly as the sync
	// endpoint does.
	checker := scheduler.NewServiceChecker(s.store, s.logger)
	checker.SetCollectDetails(true)
	checker.SetSpeedTestRunner(func() *internal.SpeedTestResult {
		// runSpeedCheckViaRunner treats nil as "no speedtest tool
		// available". That's the correct semantic when the engine
		// errored — preserves the sync-endpoint behaviour.
		return resultPtr
	})
	scResult := checker.RunCheck(cfg, time.Now().UTC())

	_ = writeSSEEvent(w, flusher, "result", scResult)
	_ = writeSSEEvent(w, flusher, "end", map[string]any{
		"duration_seconds": time.Since(startedAt).Seconds(),
	})
}
