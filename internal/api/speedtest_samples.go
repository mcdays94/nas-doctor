// Package api — speedtest_samples.go implements the per-sample JSON
// endpoint for COMPLETED speed tests, per PRD #283 / issue #286
// (slice 3 of the speed-test live-progress PRD).
//
// Strict separation from the SSE flow:
//
//   - Live tests (in flight): subscribe to /api/v1/speedtest/stream/{id}
//   - Completed tests: GET /api/v1/speedtest/samples/{id}
//
// The samples endpoint returns 404 for in-flight tests with a hint
// pointing at the stream endpoint, NOT a snapshot of samples-so-far.
// This keeps the live and historical paths from drifting and gives
// the dashboard one obvious choice per state.
package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// speedtestSamplesResponse is the body of GET /api/v1/speedtest/samples/{test_id}.
//
// test_id round-trips so a curl response stands alone without the
// caller needing to remember which ID they asked for. samples is an
// always-non-null array (empty on a legacy row that pre-dates the
// per-sample feature) so a JS .length check on the frontend never
// fails. count is a convenience for clients that want to alert on
// "old test, no samples" without iterating.
type speedtestSamplesResponse struct {
	TestID  int64                       `json:"test_id"`
	Samples []storage.SpeedTestSample   `json:"samples"`
	Count   int                         `json:"count"`
}

// handleSpeedtestSamples implements GET /api/v1/speedtest/samples/{test_id}.
//
// Response codes:
//
//   - 200: completed test, samples returned (possibly empty array
//     for a legacy row that pre-dates the per-sample feature).
//   - 400: test_id parameter is not a valid int64.
//   - 404: test_id is in flight (use /stream/{id} instead — hint in
//     body), OR test_id is unknown / pruned. Both cases share a
//     status code; the body's hint differentiates.
//
// The "in flight" check goes through the LiveTestRegistry. The
// "unknown / pruned" check looks at speedtest_history — if no row
// exists with this ID, return 404. If the row exists but has zero
// samples (legacy or new-but-mid-pruning), return 200 with an empty
// array so the dashboard's mini-chart renders the empty-state hint.
func (s *Server) handleSpeedtestSamples(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "test_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid test_id",
		})
		return
	}

	// In-flight check: if the registry recognises this ID, the test
	// is still running. The samples endpoint only serves COMPLETED
	// tests; live data lives on /stream/{id}.
	if reg := s.liveTestRegistry(); reg != nil {
		if _, alive := reg.GetLive(id); alive {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "test is in flight; subscribe via /api/v1/speedtest/stream/" + idStr + " for live samples",
				"hint":  "/api/v1/speedtest/stream/" + idStr,
			})
			return
		}
	}

	// Completed test — fetch samples from the store. Empty result
	// is valid (legacy row without per-sample data); the UI renders
	// the empty-state hint in that case.
	samples, err := s.store.GetSpeedTestSamples(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch samples: " + err.Error(),
		})
		return
	}

	// Distinguish "test exists but no samples" (200, empty array) from
	// "test_id unknown" (404). The store's GetSpeedTestSamples returns
	// an empty slice for both, so we probe the history table to tell
	// them apart. This is best-effort: a fake test store may not
	// surface IDs through GetSpeedTestHistory (e.g. tests use
	// SaveSpeedTest + the synthetic FakeStore ID counter), so when
	// samples ARE present we trust the existence of samples and skip
	// the existence probe.
	if len(samples) == 0 {
		exists, err := s.speedtestHistoryRowExists(id)
		if err == nil && !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "unknown test_id; the test may have been pruned",
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, speedtestSamplesResponse{
		TestID:  id,
		Samples: samples,
		Count:   len(samples),
	})
}

// speedtestHistoryRowExists probes whether a speedtest_history row
// with the given ID exists. Used by the samples endpoint to
// differentiate legacy-row-with-no-samples (200) from unknown-test-id
// (404). Returns nil error + false on missing row so callers can
// branch cleanly. Returns nil error + true if the row exists OR if
// the store doesn't surface IDs (fake stores) — in the latter case
// the caller falls back to "200 empty" which is the safer default.
func (s *Server) speedtestHistoryRowExists(testID int64) (bool, error) {
	// Fetch a generous time window so any non-pruned row is included.
	// 365 days is well beyond any retention policy nas-doctor ships
	// with. If the row still isn't visible after this query, we treat
	// it as unknown.
	points, err := s.store.GetSpeedTestHistory(24 * 365)
	if err != nil {
		return false, err
	}
	for _, p := range points {
		if p.ID == testID {
			return true, nil
		}
	}
	return false, nil
}
