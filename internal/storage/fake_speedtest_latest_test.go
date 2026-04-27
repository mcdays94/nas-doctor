package storage

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Issue #290: FakeStore must implement the same GetLatestSpeedTestResult
// contract as *DB so unit tests in scheduler/api don't need a real
// SQLite to exercise the snapshot-hydration path.

func TestFakeStore_GetLatestSpeedTestResult_Empty(t *testing.T) {
	store := NewFakeStore()

	got, ok, err := store.GetLatestSpeedTestResult()
	if err != nil {
		t.Fatalf("err on empty store: %v", err)
	}
	if ok || got != nil {
		t.Errorf("ok=%v got=%+v on empty store, want false/nil", ok, got)
	}
}

func TestFakeStore_GetLatestSpeedTestResult_ReturnsNewest(t *testing.T) {
	store := NewFakeStore()

	older := time.Now().Add(-3 * time.Hour).UTC()
	newer := time.Now().Add(-1 * time.Hour).UTC()

	if err := store.SaveSpeedTest("a", &internal.SpeedTestResult{
		Timestamp: older, DownloadMbps: 50, UploadMbps: 5, LatencyMs: 20,
		Engine: internal.SpeedTestEngineOoklaCLI,
	}); err != nil {
		t.Fatalf("save older: %v", err)
	}
	if err := store.SaveSpeedTest("b", &internal.SpeedTestResult{
		Timestamp: newer, DownloadMbps: 300, UploadMbps: 30, LatencyMs: 8,
		Engine: internal.SpeedTestEngineSpeedTestGo,
	}); err != nil {
		t.Fatalf("save newer: %v", err)
	}

	got, ok, err := store.GetLatestSpeedTestResult()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok || got == nil {
		t.Fatal("ok=false / got=nil with rows present")
	}
	if got.DownloadMbps != 300 {
		t.Errorf("DownloadMbps = %v, want 300 (newer row)", got.DownloadMbps)
	}
	if got.Engine != internal.SpeedTestEngineSpeedTestGo {
		t.Errorf("Engine = %q, want %q", got.Engine, internal.SpeedTestEngineSpeedTestGo)
	}
}
