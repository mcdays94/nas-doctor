package storage

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

func TestFakeStore_SaveAndGetSnapshot(t *testing.T) {
	store := NewFakeStore()

	snap := &internal.Snapshot{
		ID:        "snap-001",
		Timestamp: time.Now(),
		Duration:  1.5,
		Findings: []internal.Finding{
			{Severity: "critical", Title: "disk failure"},
			{Severity: "warning", Title: "high temp"},
			{Severity: "info", Title: "update available"},
		},
	}

	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	got, err := store.GetLatestSnapshot()
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if got == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if got.ID != "snap-001" {
		t.Errorf("expected ID snap-001, got %s", got.ID)
	}
}

func TestFakeStore_GetSnapshotByID(t *testing.T) {
	store := NewFakeStore()

	snap1 := &internal.Snapshot{ID: "snap-001", Timestamp: time.Now().Add(-time.Hour)}
	snap2 := &internal.Snapshot{ID: "snap-002", Timestamp: time.Now()}
	store.SaveSnapshot(snap1)
	store.SaveSnapshot(snap2)

	got, err := store.GetSnapshot("snap-001")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got == nil || got.ID != "snap-001" {
		t.Errorf("expected snap-001, got %v", got)
	}

	got, err = store.GetSnapshot("nonexistent")
	if err != nil {
		t.Fatalf("GetSnapshot nonexistent: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent snapshot, got %v", got)
	}
}

func TestFakeStore_ListSnapshots(t *testing.T) {
	store := NewFakeStore()

	for i := 0; i < 5; i++ {
		store.SaveSnapshot(&internal.Snapshot{
			ID:        "snap-" + string(rune('A'+i)),
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	summaries, err := store.ListSnapshots(3)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("expected 3 summaries, got %d", len(summaries))
	}
	// Should be newest first.
	if len(summaries) >= 2 && summaries[0].Timestamp.Before(summaries[1].Timestamp) {
		t.Error("expected newest-first order")
	}
}

func TestFakeStore_SaveAndListServiceChecks(t *testing.T) {
	store := NewFakeStore()

	now := time.Now().UTC()
	results := []internal.ServiceCheckResult{
		{
			Key:       "key-1",
			Name:      "Web Server",
			Type:      "http",
			Target:    "http://localhost",
			Status:    "up",
			CheckedAt: now.Format(time.RFC3339),
		},
		{
			Key:       "key-2",
			Name:      "DNS",
			Type:      "dns",
			Target:    "8.8.8.8",
			Status:    "down",
			Error:     "timeout",
			CheckedAt: now.Add(-time.Minute).Format(time.RFC3339),
		},
	}

	if err := store.SaveServiceCheckResults(results); err != nil {
		t.Fatalf("SaveServiceCheckResults: %v", err)
	}

	entries, err := store.ListLatestServiceChecks(10)
	if err != nil {
		t.Fatalf("ListLatestServiceChecks: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify the "up" result is there.
	found := false
	for _, e := range entries {
		if e.Key == "key-1" && e.Status == "up" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find key-1 with status up")
	}
}

func TestFakeStore_GetLatestServiceCheckState(t *testing.T) {
	store := NewFakeStore()

	now := time.Now().UTC()
	store.SaveServiceCheckResults([]internal.ServiceCheckResult{
		{Key: "key-1", Status: "up", ConsecutiveFailures: 0, CheckedAt: now.Add(-time.Minute).Format(time.RFC3339)},
		{Key: "key-1", Status: "down", ConsecutiveFailures: 1, CheckedAt: now.Format(time.RFC3339)},
	})

	state, found, err := store.GetLatestServiceCheckState("key-1")
	if err != nil {
		t.Fatalf("GetLatestServiceCheckState: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if state.Status != "down" {
		t.Errorf("expected status=down, got %s", state.Status)
	}
	if state.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", state.ConsecutiveFailures)
	}

	_, found, _ = store.GetLatestServiceCheckState("nonexistent")
	if found {
		t.Error("expected found=false for nonexistent key")
	}
}

func TestFakeStore_ConfigGetSet(t *testing.T) {
	store := NewFakeStore()

	// Get missing key returns error.
	_, err := store.GetConfig("missing")
	if err == nil {
		t.Error("expected error for missing config key")
	}

	// Set and get.
	if err := store.SetConfig("theme", "midnight"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	val, err := store.GetConfig("theme")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "midnight" {
		t.Errorf("expected midnight, got %s", val)
	}

	// Overwrite.
	store.SetConfig("theme", "clean")
	val, _ = store.GetConfig("theme")
	if val != "clean" {
		t.Errorf("expected clean after overwrite, got %s", val)
	}
}

func TestFakeStore_NotificationLog(t *testing.T) {
	store := NewFakeStore()

	if err := store.SaveNotificationLog("webhook1", "discord", "sent", 3, ""); err != nil {
		t.Fatalf("SaveNotificationLog: %v", err)
	}
	if err := store.SaveNotificationLog("webhook1", "discord", "failed", 2, "timeout"); err != nil {
		t.Fatalf("SaveNotificationLog: %v", err)
	}

	entries, err := store.GetNotificationLog(10)
	if err != nil {
		t.Fatalf("GetNotificationLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestFakeStore_CanSendNotification(t *testing.T) {
	store := NewFakeStore()
	now := time.Now()
	cooldown := 10 * time.Minute

	// First send: should be allowed.
	allowed, err := store.CanSendNotification("fp-1", "route-1", cooldown, now)
	if err != nil {
		t.Fatalf("CanSendNotification: %v", err)
	}
	if !allowed {
		t.Error("expected allowed=true for first send")
	}

	// Record the send.
	store.SaveNotificationState("fp-1", "route-1", "sent", now)

	// Immediately after: should be blocked.
	allowed, _ = store.CanSendNotification("fp-1", "route-1", cooldown, now.Add(time.Minute))
	if allowed {
		t.Error("expected allowed=false within cooldown")
	}

	// After cooldown: should be allowed.
	allowed, _ = store.CanSendNotification("fp-1", "route-1", cooldown, now.Add(11*time.Minute))
	if !allowed {
		t.Error("expected allowed=true after cooldown expires")
	}
}

func TestFakeStore_DeleteServiceCheckByKey(t *testing.T) {
	store := NewFakeStore()

	now := time.Now().UTC()
	store.SaveServiceCheckResults([]internal.ServiceCheckResult{
		{Key: "keep", Name: "Keep", CheckedAt: now.Format(time.RFC3339)},
		{Key: "delete-me", Name: "Delete", CheckedAt: now.Format(time.RFC3339)},
		{Key: "delete-me", Name: "Delete", CheckedAt: now.Add(time.Minute).Format(time.RFC3339)},
	})

	deleted, err := store.DeleteServiceCheckByKey("delete-me")
	if err != nil {
		t.Fatalf("DeleteServiceCheckByKey: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	entries, _ := store.ListLatestServiceChecks(10)
	if len(entries) != 1 {
		t.Errorf("expected 1 remaining entry, got %d", len(entries))
	}
}

func TestFakeStore_LifecycleMethods(t *testing.T) {
	store := NewFakeStore()

	// These should not panic and return zero values.
	if _, err := store.PruneSnapshots(24*time.Hour, 5); err != nil {
		t.Errorf("PruneSnapshots: %v", err)
	}
	if _, err := store.PruneNotificationLog(24 * time.Hour); err != nil {
		t.Errorf("PruneNotificationLog: %v", err)
	}
	if _, err := store.PruneAlerts(24 * time.Hour); err != nil {
		t.Errorf("PruneAlerts: %v", err)
	}
	if _, err := store.PruneOrphanedFindings(); err != nil {
		t.Errorf("PruneOrphanedFindings: %v", err)
	}
	if err := store.Vacuum(); err != nil {
		t.Errorf("Vacuum: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if dir := store.DataDir(); dir == "" {
		t.Error("expected non-empty DataDir")
	}

	stats, err := store.GetDBStats()
	if err != nil {
		t.Errorf("GetDBStats: %v", err)
	}
	if stats == nil {
		t.Error("expected non-nil DBStats")
	}
}
