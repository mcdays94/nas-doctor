package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// spyStore wraps a FakeStore and records calls made to disk-history methods.
// This lets the API handler test verify that `?hours=24` routes to the
// time-windowed query with the right window, while no-param requests fall
// back to the legacy row-count query (preserving backward compatibility
// for any external caller of /api/v1/disks/{serial}).
type spyStore struct {
	*storage.FakeStore

	lastLegacyLimit    int
	legacyCalls        int
	lastWindow         time.Duration
	rangeCalls         int
	historyInRangeResp []storage.DiskHistoryPoint
	historyResp        []storage.DiskHistoryPoint
}

func (s *spyStore) GetDiskHistory(serial string, limit int) ([]storage.DiskHistoryPoint, error) {
	s.legacyCalls++
	s.lastLegacyLimit = limit
	return s.historyResp, nil
}

func (s *spyStore) GetDiskHistoryInRange(serial string, window time.Duration) ([]storage.DiskHistoryPoint, error) {
	s.rangeCalls++
	s.lastWindow = window
	return s.historyInRangeResp, nil
}

// newTestServerForDiskHistory builds a chi router identical in shape to the
// real one for the /api/v1/disks/{serial} route so chi.URLParam(r, "serial")
// resolves correctly.
func newTestServerForDiskHistory(store storage.Store) (*Server, http.Handler) {
	srv := &Server{
		store:     store,
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
	r := chi.NewRouter()
	r.Get("/api/v1/disks/{serial}", srv.handleGetDisk)
	return srv, r
}

// TestHandleGetDisk_HoursParamUsesTimeWindow asserts that when ?hours=N is
// supplied, the handler calls GetDiskHistoryInRange with window = N hours
// (NOT the legacy row-count GetDiskHistory).
//
// Issue #166.
func TestHandleGetDisk_HoursParamUsesTimeWindow(t *testing.T) {
	rangePoints := []storage.DiskHistoryPoint{
		{Timestamp: time.Now().Add(-2 * time.Hour), Temperature: 31},
		{Timestamp: time.Now().Add(-1 * time.Hour), Temperature: 32},
	}
	cases := []struct {
		label string
		hours int
	}{
		{"1D", 24},
		{"1W", 168},
		{"1M", 720},
		{"1Y", 8760},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			spy := &spyStore{FakeStore: storage.NewFakeStore(), historyInRangeResp: rangePoints}
			_, handler := newTestServerForDiskHistory(spy)

			req := httptest.NewRequest("GET", "/api/v1/disks/SN1?hours="+itoa(tc.hours), nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if spy.rangeCalls != 1 {
				t.Errorf("expected GetDiskHistoryInRange called once, got %d", spy.rangeCalls)
			}
			if spy.legacyCalls != 0 {
				t.Errorf("expected legacy GetDiskHistory NOT called when ?hours= supplied, got %d calls", spy.legacyCalls)
			}
			wantWindow := time.Duration(tc.hours) * time.Hour
			if spy.lastWindow != wantWindow {
				t.Errorf("window: expected %v, got %v", wantWindow, spy.lastWindow)
			}
		})
	}
}

// TestHandleGetDisk_HoursParamFallsBackToLatestSamplesWhenWindowTooSparse pins
// the /disk/<serial> behaviour after the v0.10.2-rc1 UAT finding: the Stats
// page showed useful temperature history for a drive via /api/v1/sparklines,
// but /disk/<serial> defaulted to the 1D window and showed "Not enough history
// data" because only one SMART row existed in the last 24 hours. If the chosen
// range has fewer than two points, the API falls back to the legacy latest-N
// history query so the disk detail page can render the same useful chart shape
// as /stats instead of a false empty-state.
func TestHandleGetDisk_HoursParamFallsBackToLatestSamplesWhenWindowTooSparse(t *testing.T) {
	latestSamples := []storage.DiskHistoryPoint{
		{Timestamp: time.Now().Add(-72 * time.Hour), Temperature: 34},
		{Timestamp: time.Now().Add(-48 * time.Hour), Temperature: 33},
		{Timestamp: time.Now().Add(-1 * time.Hour), Temperature: 30},
	}
	spy := &spyStore{
		FakeStore:          storage.NewFakeStore(),
		historyInRangeResp: []storage.DiskHistoryPoint{{Timestamp: time.Now().Add(-1 * time.Hour), Temperature: 30}},
		historyResp:        latestSamples,
	}
	_, handler := newTestServerForDiskHistory(spy)

	req := httptest.NewRequest("GET", "/api/v1/disks/5PK9RPTB?hours=24", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if spy.rangeCalls != 1 {
		t.Errorf("expected GetDiskHistoryInRange called once, got %d", spy.rangeCalls)
	}
	if spy.legacyCalls != 1 {
		t.Errorf("expected fallback GetDiskHistory called once when range has <2 points, got %d", spy.legacyCalls)
	}
	if spy.lastLegacyLimit != 500 {
		t.Errorf("expected fallback latest-samples limit 500, got %d", spy.lastLegacyLimit)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	history, ok := resp["history"].([]any)
	if !ok {
		t.Fatalf("response history missing/wrong type: %v", resp["history"])
	}
	if len(history) != len(latestSamples) {
		t.Fatalf("history len = %d; want fallback latest samples len %d", len(history), len(latestSamples))
	}
	last := history[len(history)-1].(map[string]any)
	if got := int(last["temperature"].(float64)); got != 30 {
		t.Errorf("last fallback temperature = %d; want 30", got)
	}
}

// TestHandleGetDisk_NoHoursParamPreservesLegacyBehavior ensures the
// no-query-param request path still returns 200 and produces the same
// response shape external callers already rely on.
func TestHandleGetDisk_NoHoursParamPreservesLegacyBehavior(t *testing.T) {
	spy := &spyStore{FakeStore: storage.NewFakeStore()}
	_, handler := newTestServerForDiskHistory(spy)

	req := httptest.NewRequest("GET", "/api/v1/disks/SN1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if spy.legacyCalls != 1 {
		t.Errorf("expected legacy GetDiskHistory called once for no-param request, got %d", spy.legacyCalls)
	}
	if spy.rangeCalls != 0 {
		t.Errorf("expected GetDiskHistoryInRange NOT called without ?hours=, got %d calls", spy.rangeCalls)
	}
	if spy.lastLegacyLimit != 500 {
		t.Errorf("expected legacy limit 500, got %d", spy.lastLegacyLimit)
	}

	// Response shape sanity check: top-level keys present.
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["history"]; !ok {
		t.Error("response missing 'history' field")
	}
}

// TestHandleGetDisk_InvalidHoursFallsBackToLegacy — a malformed ?hours=
// should NOT break the endpoint; it falls back to legacy behavior.
func TestHandleGetDisk_InvalidHoursFallsBackToLegacy(t *testing.T) {
	spy := &spyStore{FakeStore: storage.NewFakeStore()}
	_, handler := newTestServerForDiskHistory(spy)

	req := httptest.NewRequest("GET", "/api/v1/disks/SN1?hours=notanumber", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if spy.legacyCalls != 1 || spy.rangeCalls != 0 {
		t.Errorf("malformed ?hours= should fall back to legacy; got legacyCalls=%d rangeCalls=%d", spy.legacyCalls, spy.rangeCalls)
	}
}

// TestHandleGetDisk_CapsHours — runaway ?hours=99999999 requests should be
// capped so the query doesn't lock up the DB or return absurd time windows.
func TestHandleGetDisk_CapsHours(t *testing.T) {
	spy := &spyStore{FakeStore: storage.NewFakeStore()}
	_, handler := newTestServerForDiskHistory(spy)

	req := httptest.NewRequest("GET", "/api/v1/disks/SN1?hours=99999999", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if spy.rangeCalls != 1 {
		t.Fatalf("expected 1 call to in-range query, got %d", spy.rangeCalls)
	}
	// Cap at 1 year (8760h).
	maxWindow := 8760 * time.Hour
	if spy.lastWindow > maxWindow {
		t.Errorf("expected window capped at %v, got %v", maxWindow, spy.lastWindow)
	}
}

// itoa is a tiny helper to avoid importing strconv just for tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
