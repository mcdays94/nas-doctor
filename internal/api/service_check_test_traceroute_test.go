package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// TestHandleTestServiceCheck_Traceroute_InjectsRunner — the /test endpoint
// must wire a traceroute runner when the type=traceroute so the Test
// button produces a real result (not a nil-runner "down"). Tests inject
// a stub via srv.tracerouteRunner to avoid needing mtr on the test host.
func TestHandleTestServiceCheck_Traceroute_InjectsRunner(t *testing.T) {
	srv := newSettingsTestServer()
	called := false
	srv.tracerouteRunner = func(target string, cycles int) (*collector.MTRResult, error) {
		called = true
		if target != "8.8.8.8" {
			t.Errorf("expected target 8.8.8.8, got %q", target)
		}
		// Test button path should use cycles >= 10 for a richer
		// sample than the scheduler's 5.
		if cycles < 10 {
			t.Errorf("expected cycles >= 10 for Test-button, got %d", cycles)
		}
		return &collector.MTRResult{
			Target: "8.8.8.8",
			Hops: []collector.MTRHop{
				{Count: 1, Host: "10.0.0.1", LossPct: 0, AvgMs: 0.5},
				{Count: 2, Host: "8.8.8.8", LossPct: 0, AvgMs: 12.3},
			},
			FinalRTTMs: 12.3,
		}, nil
	}

	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "trace-test",
		"type":   "traceroute",
		"target": "8.8.8.8",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected traceroute runner to be invoked")
	}
	r := decodeResult(t, rec)
	if r.Status != "up" {
		t.Fatalf("expected status up, got %q (error=%q)", r.Status, r.Error)
	}
	// Test button path must set SetCollectDetails so the UI can render
	// the hop-by-hop table.
	if r.Details == nil {
		t.Fatal("expected Details populated by Test endpoint")
	}
}

// TestHandleTestServiceCheck_Traceroute_ExposesHopsInDetails — the Test
// button response must include the hops slice so the JS renderer can
// draw the per-hop table.
func TestHandleTestServiceCheck_Traceroute_ExposesHopsInDetails(t *testing.T) {
	srv := newSettingsTestServer()
	srv.tracerouteRunner = func(_ string, _ int) (*collector.MTRResult, error) {
		return &collector.MTRResult{
			Target: "8.8.8.8",
			Hops: []collector.MTRHop{
				{Count: 1, Host: "10.0.0.1", LossPct: 0, AvgMs: 0.5},
				{Count: 2, Host: "8.8.8.8", LossPct: 0, AvgMs: 12.3},
			},
		}, nil
	}
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "trace-hops",
		"type":   "traceroute",
		"target": "8.8.8.8",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Decode generically so we can see what the JSON payload sends
	// over the wire — the JS renderer reads this shape.
	var generic map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &generic); err != nil {
		t.Fatalf("decode: %v", err)
	}
	details, ok := generic["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %T", generic["details"])
	}
	hopsRaw, ok := details["hops"]
	if !ok {
		t.Fatal("expected hops key in details")
	}
	hops, ok := hopsRaw.([]any)
	if !ok {
		t.Fatalf("expected hops to be an array over the wire, got %T", hopsRaw)
	}
	if len(hops) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(hops))
	}
	firstHop, ok := hops[0].(map[string]any)
	if !ok {
		t.Fatalf("expected hop[0] to be an object, got %T", hops[0])
	}
	if firstHop["host"] != "10.0.0.1" {
		t.Errorf("expected first hop host=10.0.0.1, got %v", firstHop["host"])
	}
}

// TestHandleTestServiceCheck_Traceroute_RunnerError — runner failure
// must produce a 200 response with status=down and a non-empty error.
// (Mirrors TestHandleTestServiceCheck_HTTP_Down.)
func TestHandleTestServiceCheck_Traceroute_RunnerError(t *testing.T) {
	srv := newSettingsTestServer()
	srv.tracerouteRunner = func(_ string, _ int) (*collector.MTRResult, error) {
		return nil, errExecFailed{}
	}
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "trace-err",
		"type":   "traceroute",
		"target": "8.8.8.8",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	r := decodeResult(t, rec)
	if r.Status != "down" {
		t.Fatalf("expected status down on runner error, got %q", r.Status)
	}
	if r.Error == "" {
		t.Fatal("expected non-empty error")
	}
}

type errExecFailed struct{}

func (errExecFailed) Error() string { return "mtr: exec failed (simulated)" }
