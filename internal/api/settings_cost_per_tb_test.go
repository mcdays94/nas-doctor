package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newSettingsTestServer builds a minimal Server for exercising the settings
// handlers against an in-memory FakeStore.
func newSettingsTestServer() *Server {
	return &Server{
		store:     storage.NewFakeStore(),
		logger:    slog.Default(),
		version:   "test",
		startTime: time.Now(),
	}
}

// TestSettingsHTMLIncludesCostPerTBInput verifies the settings page template
// contains the cost-per-TB input widget with proper load/save wiring.
//
// Regression for #131 — backend Settings.CostPerTB was wired in commit 2a3eb3e
// but the UI input was never added, making the field unreachable from the app.
func TestSettingsHTMLIncludesCostPerTBInput(t *testing.T) {
	path := filepath.Join("templates", "settings.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings.html: %v", err)
	}
	content := string(data)

	checks := []struct {
		name   string
		substr string
	}{
		{"input element", `id="cost-per-tb"`},
		{"card anchor", `id="card-replacement-cost"`},
		{"sidebar nav link", `href="#card-replacement-cost"`},
		{"load from data", `data.cost_per_tb`},
		{"save payload key", `cost_per_tb:`},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.substr) {
				t.Errorf("settings.html missing %q — expected substring: %q", tc.name, tc.substr)
			}
		})
	}
}

// TestSettings_CostPerTB_RoundTrip verifies the cost_per_tb setting is
// persisted through a PUT/GET cycle against the real settings handlers.
func TestSettings_CostPerTB_RoundTrip(t *testing.T) {
	srv := newSettingsTestServer()

	// PUT /api/v1/settings with cost_per_tb=22.5 and minimum valid fields
	// (handleUpdateSettings requires scan_interval + theme).
	putBody := map[string]interface{}{
		"scan_interval": "30m",
		"theme":         "midnight",
		"cost_per_tb":   22.5,
	}
	buf, _ := json.Marshal(putBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleUpdateSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/settings returned %d: %s", rec.Code, rec.Body.String())
	}

	// GET /api/v1/settings — cost_per_tb should round-trip.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rec2 := httptest.NewRecorder()
	srv.handleGetSettings(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/settings returned %d", rec2.Code)
	}
	body, _ := io.ReadAll(rec2.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	got, ok := parsed["cost_per_tb"].(float64)
	if !ok {
		t.Fatalf("cost_per_tb missing or wrong type in response: %v", parsed["cost_per_tb"])
	}
	if got != 22.5 {
		t.Errorf("cost_per_tb round-trip: got %v, want 22.5", got)
	}
}
