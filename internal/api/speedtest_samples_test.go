package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/livetest"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

func intToString(n int64) string { return strconv.FormatInt(n, 10) }

// newSamplesServer wires a Server backed by a FakeStore, with optional
// LiveTestRegistry injected via the test seam. Routes: only the
// samples endpoint, mounted on a fresh chi router so chi.URLParam
// resolves correctly.
func newSamplesServer(t *testing.T, reg livetest.Registry) (*Server, *storage.FakeStore, http.Handler) {
	t.Helper()
	store := storage.NewFakeStore()
	srv := &Server{store: store}
	if reg != nil {
		srv.testLiveTestRegistry = reg
	}
	r := chi.NewRouter()
	r.Get("/api/v1/speedtest/samples/{test_id}", srv.handleSpeedtestSamples)
	return srv, store, r
}

// TestSpeedtestSamples_HappyPath_ReturnsSamples asserts that a
// completed test with persisted per-sample telemetry is returned as a
// JSON array under .samples, with the count populated and ordering
// preserved (sample_index ascending).
func TestSpeedtestSamples_HappyPath_ReturnsSamples(t *testing.T) {
	t.Parallel()
	_, store, handler := newSamplesServer(t, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	id, err := store.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 920, UploadMbps: 88, LatencyMs: 8, Timestamp: time.Now(),
		Engine: internal.SpeedTestEngineSpeedTestGo,
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.InsertSpeedTestSamples(id, []storage.SpeedTestSample{
		{SampleIndex: 0, Phase: "latency", Timestamp: now, LatencyMs: 8.2},
		{SampleIndex: 1, Phase: "download", Timestamp: now.Add(time.Second), Mbps: 723.4},
		{SampleIndex: 2, Phase: "upload", Timestamp: now.Add(2 * time.Second), Mbps: 88.3},
	}); err != nil {
		t.Fatalf("InsertSpeedTestSamples: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/samples/" + intToString(id))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (want 200), body = %s", resp.StatusCode, body)
	}
	var got struct {
		TestID  int64                     `json:"test_id"`
		Samples []storage.SpeedTestSample `json:"samples"`
		Count   int                       `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TestID != id {
		t.Errorf("test_id = %d, want %d", got.TestID, id)
	}
	if got.Count != 3 || len(got.Samples) != 3 {
		t.Fatalf("count/len = %d/%d, want 3/3", got.Count, len(got.Samples))
	}
	for i, s := range got.Samples {
		if s.SampleIndex != i {
			t.Errorf("samples[%d].SampleIndex = %d, want %d (order broken)", i, s.SampleIndex, i)
		}
	}
	if got.Samples[0].Phase != "latency" || got.Samples[1].Phase != "download" || got.Samples[2].Phase != "upload" {
		t.Errorf("phase ordering broken: %+v", got.Samples)
	}
}

// TestSpeedtestSamples_LegacyRow_ReturnsEmptyArray asserts that a
// completed test with no persisted samples (e.g. pre-#286 row) returns
// 200 with an empty samples array — NOT 404. The dashboard's
// expanded-log mini-chart renders the "no per-sample data available"
// empty-state hint when count=0.
func TestSpeedtestSamples_LegacyRow_ReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	_, store, handler := newSamplesServer(t, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	id, err := store.SaveSpeedTestReturningID("snap-1", &internal.SpeedTestResult{
		DownloadMbps: 100, Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("SaveSpeedTestReturningID: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/samples/" + intToString(id))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (want 200 for legacy row), body = %s", resp.StatusCode, body)
	}
	var got struct {
		Samples []storage.SpeedTestSample `json:"samples"`
		Count   int                       `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count != 0 {
		t.Errorf("count = %d, want 0", got.Count)
	}
	if got.Samples == nil {
		t.Errorf("samples must be a JSON array (possibly empty), got nil — frontend .length checks would fail")
	}
}

// TestSpeedtestSamples_InFlight_Returns404WithStreamHint asserts that
// requesting samples for an in-flight test (registry's GetLive
// recognises the ID) yields a 404 with a `hint` field pointing at
// the stream endpoint. PRD #283 strict separation: live = stream,
// completed = samples.
func TestSpeedtestSamples_InFlight_Returns404WithStreamHint(t *testing.T) {
	t.Parallel()
	// Spin up a runner that never closes its samples channel so the
	// registered test stays in flight for the duration of the assertion.
	runner := newStuckRunner()
	defer close(runner.unblock)
	mgr := livetest.NewManager(runner, quietSSELogger(), counterIDGen())
	lt, err := mgr.StartTest(t.Context())
	if err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	_, _, handler := newSamplesServer(t, mgr)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/samples/" + intToString(lt.ID()))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (want 404), body = %s", resp.StatusCode, body)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hint"] == "" {
		t.Errorf("expected hint pointing at /stream/{id}, got body %v", body)
	}
}

// TestSpeedtestSamples_UnknownTestID_Returns404 asserts that a test_id
// with no matching speedtest_history row returns 404. This is what
// happens after the retention loop prunes an old test or if the
// caller hits a typoed ID.
func TestSpeedtestSamples_UnknownTestID_Returns404(t *testing.T) {
	t.Parallel()
	_, _, handler := newSamplesServer(t, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/samples/999999")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (want 404), body = %s", resp.StatusCode, body)
	}
}

// TestSpeedtestSamples_InvalidID_Returns400 asserts that a non-numeric
// test_id parameter yields 400, NOT a 500.
func TestSpeedtestSamples_InvalidID_Returns400(t *testing.T) {
	t.Parallel()
	_, _, handler := newSamplesServer(t, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/speedtest/samples/not-a-number")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (want 400), body = %s", resp.StatusCode, body)
	}
}

// TestSpeedtestSamples_APIKeyMiddleware asserts the samples endpoint
// is protected by the API-key middleware in production wiring. We
// build the production NewRouter and confirm a request without an
// API key (when one is set) is rejected. Mirrors the existing
// API-key middleware tests for /run + /stream.
func TestSpeedtestSamples_APIKeyMiddleware_RejectsMissingKey(t *testing.T) {
	t.Parallel()
	store := storage.NewFakeStore()
	// Persist a settings blob containing api_key so getSettings()
	// reads it back through the canonical config-key path. Setting
	// the api_key directly in the config table is not enough — the
	// middleware reads s.getSettings().APIKey which deserialises
	// from settings_v3.
	settingsJSON := `{"settings_version":3,"api_key":"nd-test-secret"}`
	_ = store.SetConfig("settings", settingsJSON)
	srv := &Server{store: store}
	r := chi.NewRouter()
	r.With(srv.apiKeyMiddleware).Get("/api/v1/speedtest/samples/{test_id}", srv.handleSpeedtestSamples)
	httpsrv := httptest.NewServer(r)
	defer httpsrv.Close()

	// No Authorization header, no api_key query param, no Referer →
	// the middleware should reject.
	req, _ := http.NewRequest(http.MethodGet, httpsrv.URL+"/api/v1/speedtest/samples/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d (want 401), body = %s", resp.StatusCode, body)
	}
}

// stuckRunner is a livetest.Runner that opens a samples channel and
// blocks until unblock is closed. Used to keep a test "in flight"
// for the lifetime of an assertion.
type stuckRunner struct {
	unblock chan struct{}
}

func newStuckRunner() *stuckRunner {
	return &stuckRunner{unblock: make(chan struct{})}
}

func (r *stuckRunner) Run(_ context.Context) (*internal.SpeedTestResult, <-chan livetest.Sample, error) {
	out := make(chan livetest.Sample)
	go func() {
		<-r.unblock
		close(out)
	}()
	return &internal.SpeedTestResult{Engine: internal.SpeedTestEngineSpeedTestGo}, out, nil
}
