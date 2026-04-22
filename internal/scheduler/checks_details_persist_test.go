package scheduler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestRunDueChecks_HTTP_PersistsStatusCode — issue #182. Once the scheduled
// path stops stripping Details, a 404 from the target server must round-
// trip into the persisted history row so the /service-checks log UI can
// render HTTP 404 alongside the status=down line.
//
// This is the sibling of TestRunCheck_Details_HTTP_404_StillRecordsCode,
// but asserted at the RunDueChecks level — i.e. end-to-end through the
// store, which is what the log UI actually reads.
func TestRunDueChecks_HTTP_PersistsStatusCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	sc, store := newTestChecker()
	sc.SetCollectDetails(true) // mirrors the scheduler-owned checker setup in #182

	checks := []internal.ServiceCheckConfig{{
		Name:    "scheduled-404",
		Type:    internal.ServiceCheckHTTP,
		Target:  ts.URL,
		Enabled: true,
	}}
	results := sc.RunDueChecks(checks, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "down" {
		t.Fatalf("expected down, got %q", results[0].Status)
	}

	entries, err := store.ListLatestServiceChecks(10)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 persisted entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Details == nil {
		t.Fatalf("expected Details persisted, got nil (scheduled path still stripping)")
	}
	// FakeStore preserves original int; the DB path would come back as
	// float64 via JSON. Accept either.
	got, ok := detailsInt(e.Details["status_code"])
	if !ok || got != 404 {
		t.Fatalf("expected status_code=404 in persisted Details, got %v (%T)", e.Details["status_code"], e.Details["status_code"])
	}
	if stage, _ := e.Details["failure_stage"].(string); stage != "http_status" {
		t.Fatalf("expected failure_stage=http_status, got %v", e.Details["failure_stage"])
	}
	if ct, _ := e.Details["content_type"].(string); ct == "" {
		t.Fatalf("expected content_type populated, got %v", e.Details["content_type"])
	}
}

func detailsInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	}
	return 0, false
}
