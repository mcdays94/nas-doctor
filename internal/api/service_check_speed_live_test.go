package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
)

// Issue #346: the speed check Test button renders a live chart and the final
// result inside the editor card. The `result` event only carries the check
// verdict and the three speeds, so the stream also sends `engine_result` with
// what the engine measured (server, ISP, jitter, engine name) for the card.
func TestServiceChecksTestStream_SpeedEngineResultEvent(t *testing.T) {
	deadline := assertCompletesWithin(t, 5*time.Second)
	defer deadline()

	srv := newSettingsTestServer()
	srv.streamingSpeedTestRunner = func(_ context.Context) (<-chan collector.SpeedTestSample, <-chan collector.StreamingSpeedFinal) {
		updates := make(chan collector.SpeedTestSample, 4)
		final := make(chan collector.StreamingSpeedFinal, 1)
		go func() {
			defer close(updates)
			defer close(final)
			now := time.Now()
			updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseDownload, At: now, Mbps: 900}
			updates <- collector.SpeedTestSample{Phase: collector.SpeedTestPhaseUpload, At: now.Add(time.Second), Mbps: 400}
			final <- collector.StreamingSpeedFinal{Result: &internal.SpeedTestResult{
				DownloadMbps: 905.5,
				UploadMbps:   410.2,
				LatencyMs:    8.1,
				JitterMs:     0.3,
				ServerName:   "Coimbra",
				ISP:          "NOS",
				ExternalIP:   "203.0.113.7",
				Engine:       internal.SpeedTestEngineSpeedTestGo,
			}}
		}()
		return updates, final
	}

	resp := postTestStream(t, srv, map[string]any{"name": "speed-live", "type": "speed", "target": "speedtest"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	events, err := parseTraceSSEStream(resp.Body)
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}

	engineIdx, resultIdx := -1, -1
	var engine map[string]any
	for i, e := range events {
		switch e.Event {
		case "engine_result":
			engineIdx = i
			if err := json.Unmarshal([]byte(e.Data), &engine); err != nil {
				t.Fatalf("engine_result data malformed: %v (data=%s)", err, e.Data)
			}
		case "result":
			resultIdx = i
		}
	}
	if engineIdx < 0 {
		t.Fatal("no engine_result event; the card can't show server, ISP or jitter")
	}
	if resultIdx >= 0 && engineIdx > resultIdx {
		t.Errorf("engine_result at %d comes after result at %d; want it first", engineIdx, resultIdx)
	}
	for key, want := range map[string]any{
		"server_name":   "Coimbra",
		"isp":           "NOS",
		"jitter_ms":     0.3,
		"engine":        internal.SpeedTestEngineSpeedTestGo,
		"download_mbps": 905.5,
		"upload_mbps":   410.2,
		"latency_ms":    8.1,
	} {
		if engine[key] != want {
			t.Errorf("engine_result[%q] = %v; want %v", key, engine[key], want)
		}
	}
	if _, leaked := engine["external_ip"]; leaked {
		t.Error("engine_result exposes external_ip; send only the fields the card shows")
	}
}

// The live card replaces the toast-per-event rendering that filled the page
// with a few hundred notifications per test.
func TestServiceChecksHTML_SpeedTestRendersInCard(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("templates", "settings.html"))
	if err != nil {
		t.Fatalf("read settings.html: %v", err)
	}
	page := string(raw)
	// The panel is the shared NasSpeedLive from charts.js, the same one
	// the dashboard card uses.
	for _, want := range []string{`id="sc-speed-live"`, `"engine_result"`, `NasSpeedLive.create({ id: "sc-speed-live"`} {
		if !strings.Contains(page, want) {
			t.Errorf("settings.html missing %s", want)
		}
	}
	// speedSampleTime places samples by their own timestamp (#348), so the
	// chart keeps its shape when samples arrive in a burst.
	for _, want := range []string{"function drawSpeedTest(", "function speedSampleTime", "speedSampleTime(d.ts)"} {
		if !strings.Contains(ChartJS, want) {
			t.Errorf("ChartJS missing %s", want)
		}
	}
	for _, fn := range []string{"function runSpeedTestStream", "function handleSpeedEvent"} {
		start := strings.Index(page, fn)
		if start < 0 {
			t.Fatalf("settings.html missing %s", fn)
		}
		body := page[start+len(fn):]
		if end := strings.Index(body, "\nfunction "); end >= 0 {
			body = body[:end]
		}
		if strings.Contains(body, "showToast(") {
			t.Errorf("%s still calls showToast; speed test progress and results belong in the card", fn)
		}
	}
}
