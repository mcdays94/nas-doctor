package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newTestServerForProcessHistory creates a minimal Server with a FakeStore for testing.
func newTestServerForProcessHistory(store storage.Store) *Server {
	return &Server{
		store:     store,
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
}

func TestHandleProcessHistory_ReturnsJSON(t *testing.T) {
	store := storage.NewFakeStore()
	// Seed some process data.
	err := store.SaveProcessStats([]internal.ProcessInfo{
		{PID: 1, User: "root", Command: "/usr/bin/python3 app.py", CPU: 25.0, Mem: 10.0},
		{PID: 2, User: "root", Command: "/usr/sbin/nginx", CPU: 5.0, Mem: 3.0},
	})
	if err != nil {
		t.Fatalf("SaveProcessStats: %v", err)
	}

	srv := newTestServerForProcessHistory(store)
	req := httptest.NewRequest("GET", "/api/v1/history/processes?hours=24", nil)
	w := httptest.NewRecorder()
	srv.handleProcessHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var points []storage.ProcessHistoryPoint
	if err := json.Unmarshal(w.Body.Bytes(), &points); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if len(points) != 2 {
		t.Errorf("expected 2 points, got %d", len(points))
	}

	// Verify names are parsed from commands.
	names := make(map[string]bool)
	for _, p := range points {
		names[p.Name] = true
	}
	if !names["python3"] {
		t.Error("expected 'python3' in results")
	}
	if !names["nginx"] {
		t.Error("expected 'nginx' in results")
	}
}

func TestHandleProcessHistory_DefaultHours(t *testing.T) {
	store := storage.NewFakeStore()
	srv := newTestServerForProcessHistory(store)

	// No hours param — should default to 24.
	req := httptest.NewRequest("GET", "/api/v1/history/processes", nil)
	w := httptest.NewRecorder()
	srv.handleProcessHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var points []storage.ProcessHistoryPoint
	if err := json.Unmarshal(w.Body.Bytes(), &points); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	// Empty store returns empty array, not null.
	if points == nil {
		t.Error("expected non-nil empty array, got null")
	}
}

func TestHandleProcessHistory_CapsAt720Hours(t *testing.T) {
	store := storage.NewFakeStore()
	srv := newTestServerForProcessHistory(store)

	// Requesting more than 720 hours should be capped.
	req := httptest.NewRequest("GET", "/api/v1/history/processes?hours=9999", nil)
	w := httptest.NewRecorder()
	srv.handleProcessHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleProcessHistory_InvalidHoursIgnored(t *testing.T) {
	store := storage.NewFakeStore()
	srv := newTestServerForProcessHistory(store)

	req := httptest.NewRequest("GET", "/api/v1/history/processes?hours=abc", nil)
	w := httptest.NewRecorder()
	srv.handleProcessHistory(w, req)

	// Should still succeed with default hours.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestProcessHistoryRouteRegistered(t *testing.T) {
	store := storage.NewFakeStore()
	srv := newTestServerForProcessHistory(store)
	handler := srv.Router()

	req := httptest.NewRequest("GET", "/api/v1/history/processes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Route should exist and return 200 (not 404/405).
	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("route /api/v1/history/processes not registered: got %d", w.Code)
	}
}

func TestStatsHTMLContainsProcessHistorySection(t *testing.T) {
	// Read the embedded stats template.
	tmpl := statsHTML()
	if tmpl == "" {
		t.Skip("could not read stats.html")
	}

	checks := []struct {
		name   string
		substr string
	}{
		{"section title", "Process CPU History"},
		{"API fetch", "/api/v1/history/processes"},
		{"chart canvas", "chart-process-cpu"},
		{"NasChart.line call", "NasChart.line"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tmpl, tc.substr) {
				t.Errorf("stats.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// statsHTML is a test helper that reads the stats.html template content.
func statsHTML() string {
	return statsPageHTML
}
