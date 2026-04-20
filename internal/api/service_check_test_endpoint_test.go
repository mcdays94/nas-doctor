package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/scheduler"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// postServiceCheckTest issues a POST request against handleTestServiceCheck
// and returns the recorder for inspection.
func postServiceCheckTest(t *testing.T, srv *Server, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf []byte
	var err error
	switch v := body.(type) {
	case string:
		buf = []byte(v)
	case []byte:
		buf = v
	default:
		buf, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/service-checks/test", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleTestServiceCheck(rec, req)
	return rec
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder) internal.ServiceCheckResult {
	t.Helper()
	var r internal.ServiceCheckResult
	body, _ := io.ReadAll(rec.Body)
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("decode result: %v (body=%s)", err, string(body))
	}
	return r
}

// TestHandleTestServiceCheck_HTTP_Up — a reachable HTTP target reports up with
// a positive response time.
func TestHandleTestServiceCheck_HTTP_Up(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "web",
		"type":   "http",
		"target": ts.URL,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	r := decodeResult(t, rec)
	if r.Status != "up" {
		t.Fatalf("expected status up, got %q (error=%q)", r.Status, r.Error)
	}
	if r.ResponseMS < 0 {
		t.Fatalf("expected non-negative response time, got %d", r.ResponseMS)
	}
}

// TestHandleTestServiceCheck_HTTP_Down — an unreachable target reports down
// with a non-empty error.
func TestHandleTestServiceCheck_HTTP_Down(t *testing.T) {
	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":        "bad",
		"type":        "http",
		"target":      "http://192.0.2.1:1", // RFC 5737 TEST-NET, unreachable
		"timeout_sec": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (check ran, down), got %d: %s", rec.Code, rec.Body.String())
	}
	r := decodeResult(t, rec)
	if r.Status != "down" {
		t.Fatalf("expected status down, got %q", r.Status)
	}
	if r.Error == "" {
		t.Fatal("expected non-empty error on unreachable target")
	}
}

// TestHandleTestServiceCheck_TCP_Up — an open TCP port reports up.
func TestHandleTestServiceCheck_TCP_Up(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "tcp-svc",
		"type":   "tcp",
		"target": "127.0.0.1:" + port,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	r := decodeResult(t, rec)
	if r.Status != "up" {
		t.Fatalf("expected status up, got %q (error=%q)", r.Status, r.Error)
	}
}

// TestHandleTestServiceCheck_TCP_Down — a closed TCP port reports down.
func TestHandleTestServiceCheck_TCP_Down(t *testing.T) {
	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":        "tcp-closed",
		"type":        "tcp",
		"target":      "127.0.0.1:1",
		"timeout_sec": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	r := decodeResult(t, rec)
	if r.Status != "down" {
		t.Fatalf("expected status down, got %q", r.Status)
	}
}

// TestHandleTestServiceCheck_InvalidType — unknown types are rejected 400.
func TestHandleTestServiceCheck_InvalidType(t *testing.T) {
	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "weird",
		"type":   "gibberish",
		"target": "example.com",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid service check type") {
		t.Fatalf("expected invalid-type error, got: %s", rec.Body.String())
	}
}

// TestHandleTestServiceCheck_MissingTarget — empty target rejected 400.
func TestHandleTestServiceCheck_MissingTarget(t *testing.T) {
	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "no-target",
		"type":   "http",
		"target": "",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "target") {
		t.Fatalf("expected target error, got: %s", rec.Body.String())
	}
}

// TestHandleTestServiceCheck_MissingName — empty name rejected 400.
func TestHandleTestServiceCheck_MissingName(t *testing.T) {
	srv := newSettingsTestServer()
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "",
		"type":   "http",
		"target": "http://example.com",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "name") {
		t.Fatalf("expected name error, got: %s", rec.Body.String())
	}
}

// TestHandleTestServiceCheck_DoesNotPersist — calling /test must not write a
// row into the service-check history store.
func TestHandleTestServiceCheck_DoesNotPersist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	srv := newSettingsTestServer()
	store := srv.store.(*storage.FakeStore)

	// Seed a known baseline so we can count precisely.
	baseline, _ := store.ListLatestServiceChecks(1000)
	beforeCount := len(baseline)

	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":   "ephemeral",
		"type":   "http",
		"target": ts.URL,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("test call failed: %d %s", rec.Code, rec.Body.String())
	}

	after, _ := store.ListLatestServiceChecks(1000)
	if len(after) != beforeCount {
		t.Fatalf("service-check store changed: before=%d after=%d — /test must not persist", beforeCount, len(after))
	}
}

// TestHandleTestServiceCheck_DoesNotMutateSettings — a /test call must not
// mutate the saved settings (service_checks slice).
func TestHandleTestServiceCheck_DoesNotMutateSettings(t *testing.T) {
	srv := newSettingsTestServer()
	// Seed a saved service check named "web" pointing at a known target.
	saved := Settings{
		ScanInterval: "30m",
		Theme:        "midnight",
	}
	saved.ServiceChecks.Checks = []internal.ServiceCheckConfig{{
		Name:    "web",
		Type:    "http",
		Target:  "http://saved-target.example.com",
		Enabled: true,
	}}
	data, _ := json.Marshal(saved)
	if err := srv.store.SetConfig(settingsConfigKey, string(data)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	// POST a /test with the SAME name but a different target.
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":        "web",
		"type":        "http",
		"target":      "http://different-target.example.com",
		"timeout_sec": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("test call failed: %d %s", rec.Code, rec.Body.String())
	}

	// Reload settings from store — target for "web" must be unchanged.
	raw, err := srv.store.GetConfig(settingsConfigKey)
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	var reloaded Settings
	if err := json.Unmarshal([]byte(raw), &reloaded); err != nil {
		t.Fatalf("parse reloaded settings: %v", err)
	}
	if len(reloaded.ServiceChecks.Checks) != 1 {
		t.Fatalf("expected 1 saved check, got %d", len(reloaded.ServiceChecks.Checks))
	}
	if reloaded.ServiceChecks.Checks[0].Target != "http://saved-target.example.com" {
		t.Fatalf("saved service check target was mutated: got %q", reloaded.ServiceChecks.Checks[0].Target)
	}
}

// TestHandleTestServiceCheck_DoesNotTouchConsecutiveFailures — the ConsecutiveFailures
// tracked in store state must not be bumped by a failing /test call.
func TestHandleTestServiceCheck_DoesNotTouchConsecutiveFailures(t *testing.T) {
	srv := newSettingsTestServer()
	store := srv.store.(*storage.FakeStore)

	// Seed a failing history entry with ConsecutiveFailures=3.
	cfg := internal.ServiceCheckConfig{
		Name: "web", Type: "http", Target: "http://saved.example.com",
	}
	key := scheduler.CheckKey(cfg)
	_ = store.SaveServiceCheckResults([]internal.ServiceCheckResult{{
		Key:                 key,
		Name:                "web",
		Type:                "http",
		Target:              "http://saved.example.com",
		Status:              "down",
		ConsecutiveFailures: 3,
		CheckedAt:           "2026-01-01T00:00:00Z",
	}})

	stateBefore, ok, err := store.GetLatestServiceCheckState(key)
	if err != nil || !ok || stateBefore.ConsecutiveFailures != 3 {
		t.Fatalf("seed failed: ok=%v err=%v state=%+v", ok, err, stateBefore)
	}

	// POST a failing /test for the same-keyed config.
	rec := postServiceCheckTest(t, srv, map[string]any{
		"name":        "web",
		"type":        "http",
		"target":      "http://saved.example.com",
		"timeout_sec": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("test call failed: %d %s", rec.Code, rec.Body.String())
	}

	stateAfter, ok, err := store.GetLatestServiceCheckState(key)
	if err != nil || !ok {
		t.Fatalf("read state after: ok=%v err=%v", ok, err)
	}
	if stateAfter.ConsecutiveFailures != 3 {
		t.Fatalf("ConsecutiveFailures changed: before=3 after=%d — /test must not bump it",
			stateAfter.ConsecutiveFailures)
	}
}

// TestRegisterExtendedRoutes_ExposesServiceCheckTest — the new route is registered
// and responds on POST through the router.
func TestRegisterExtendedRoutes_ExposesServiceCheckTest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	srv := newSettingsTestServer()
	handler := srv.Router()

	body, _ := json.Marshal(map[string]any{
		"name": "routed", "type": "http", "target": ts.URL,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/service-checks/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("expected route to be registered, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from router, got %d: %s", rec.Code, rec.Body.String())
	}
}
