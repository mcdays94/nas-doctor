package storage

import (
	"testing"
	"time"
)

// TestSpeedTestAttempt_RoundTrip verifies that a saved LastSpeedTestAttempt
// can be read back with identical status + timestamp + error_msg.
// Covers the single-row upsert semantics: repeated Save calls should
// overwrite, not append (the column surface is a single current value).
func TestSpeedTestAttempt_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// No attempt yet → Get returns (nil, nil).
	got, err := db.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt on empty DB: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on empty DB, got %+v", got)
	}

	now := time.Now().UTC().Truncate(time.Second)
	att := LastSpeedTestAttempt{
		Timestamp: now,
		Status:    "success",
		ErrorMsg:  "",
	}
	if err := db.SaveSpeedTestAttempt(att); err != nil {
		t.Fatalf("SaveSpeedTestAttempt: %v", err)
	}

	got, err = db.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("GetLastSpeedTestAttempt after save: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil after save")
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if !got.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, now)
	}

	// Second save overwrites the first (single-row table).
	later := now.Add(5 * time.Minute)
	att2 := LastSpeedTestAttempt{
		Timestamp: later,
		Status:    "failed",
		ErrorMsg:  "ookla binary not found",
	}
	if err := db.SaveSpeedTestAttempt(att2); err != nil {
		t.Fatalf("second SaveSpeedTestAttempt: %v", err)
	}
	got, err = db.GetLastSpeedTestAttempt()
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("expected overwrite status=failed, got %q", got.Status)
	}
	if got.ErrorMsg != "ookla binary not found" {
		t.Errorf("ErrorMsg = %q, want ookla binary not found", got.ErrorMsg)
	}
	if !got.Timestamp.Equal(later) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, later)
	}
}
