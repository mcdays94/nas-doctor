package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// driveEventsTestServer wires RegisterExtendedRoutes onto a chi router so
// {slot_key} and {id} URL params are extracted the same way they are in
// production. Returns the router + underlying FakeStore so tests can seed
// or inspect events directly.
func driveEventsTestServer(t *testing.T) (http.Handler, *storage.FakeStore) {
	t.Helper()
	fs := storage.NewFakeStore()
	srv := newSettingsTestServer()
	srv.store = fs
	r := chi.NewRouter()
	srv.RegisterExtendedRoutes(r)
	return r, fs
}

// TestHandleDriveEvents_CRUD exercises the four endpoints end-to-end:
//
//	GET    /api/v1/drives/{slot_key}/events           → list
//	POST   /api/v1/drives/{slot_key}/events           → create (manual note)
//	PUT    /api/v1/drives/{slot_key}/events/{id}      → update note
//	DELETE /api/v1/drives/{slot_key}/events/{id}      → delete note
//
// Plus the 403 contract for auto events (Update/Delete forbidden).
func TestHandleDriveEvents_CRUD(t *testing.T) {
	handler, fs := driveEventsTestServer(t)

	// Pre-seed an auto (replacement) event that should NOT be
	// mutable via PUT/DELETE.
	autoID, err := fs.SaveDriveEvent(storage.DriveEvent{
		SlotKey:   "disk1",
		Platform:  "unraid",
		EventType: "replacement",
		EventTime: time.Now().UTC(),
		Content:   `{"old_serial":"A","new_serial":"B"}`,
		IsAuto:    true,
	})
	if err != nil {
		t.Fatalf("seed auto event: %v", err)
	}

	// --- POST: create manual note ---
	body := strings.NewReader(`{"content":"SATA cable replaced"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drives/disk1/events", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID      int64  `json:"id"`
		SlotKey string `json:"slot_key"`
		IsAuto  bool   `json:"is_auto"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode POST response: %v (body=%s)", err, rec.Body.String())
	}
	if created.ID <= 0 {
		t.Errorf("POST id = %d, want >0", created.ID)
	}
	if created.IsAuto {
		t.Errorf("POST created event is_auto=true, want false")
	}
	if created.SlotKey != "disk1" {
		t.Errorf("POST slot_key = %q, want disk1", created.SlotKey)
	}
	if created.Content != "SATA cable replaced" {
		t.Errorf("POST content = %q, want SATA cable replaced", created.Content)
	}
	manualID := created.ID

	// --- POST: reject empty content ---
	req = httptest.NewRequest(http.MethodPost, "/api/v1/drives/disk1/events", strings.NewReader(`{"content":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST empty content expected 400, got %d", rec.Code)
	}

	// --- GET: list returns both events, newest first ---
	req = httptest.NewRequest(http.MethodGet, "/api/v1/drives/disk1/events", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET list expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("GET expected 2 events, got %d", len(list))
	}

	// --- PUT: update manual note ---
	putBody := strings.NewReader(`{"content":"SATA cable replaced and tested"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/drives/disk1/events/"+itoaInt64(manualID), putBody)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT manual expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ev, err := fs.GetDriveEvent(manualID)
	if err != nil || ev == nil {
		t.Fatalf("GetDriveEvent: %v, ev=%v", err, ev)
	}
	if ev.Content != "SATA cable replaced and tested" {
		t.Errorf("after PUT, content = %q", ev.Content)
	}
	if ev.UpdatedAt == nil {
		t.Errorf("after PUT, updated_at is nil")
	}

	// --- PUT: auto event → 403 ---
	req = httptest.NewRequest(http.MethodPut, "/api/v1/drives/disk1/events/"+itoaInt64(autoID),
		strings.NewReader(`{"content":"cannot change"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("PUT auto expected 403, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// --- DELETE: auto event → 403 ---
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/drives/disk1/events/"+itoaInt64(autoID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("DELETE auto expected 403, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// --- DELETE: manual event → 204 ---
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/drives/disk1/events/"+itoaInt64(manualID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("DELETE manual expected 204, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	// Confirm gone from store.
	if ev, _ := fs.GetDriveEvent(manualID); ev != nil {
		t.Errorf("manual event still present after DELETE")
	}

	// --- DELETE: nonexistent → 404 ---
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/drives/disk1/events/99999", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE missing expected 404, got %d", rec.Code)
	}
}

// TestHandleDriveEvents_URLEncodedSlotKey verifies slot keys containing
// a space (like "Disk 1") round-trip correctly through URL path encoding.
func TestHandleDriveEvents_URLEncodedSlotKey(t *testing.T) {
	handler, fs := driveEventsTestServer(t)
	// Slot key "Disk 1" → "/api/v1/drives/Disk%201/events"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drives/Disk%201/events",
		strings.NewReader(`{"content":"note with spaces"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	events, err := fs.ListDriveEvents("Disk 1")
	if err != nil {
		t.Fatalf("ListDriveEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for Disk 1, got %d", len(events))
	}
	if events[0].SlotKey != "Disk 1" {
		t.Errorf("slot_key stored as %q, want %q", events[0].SlotKey, "Disk 1")
	}
}

// TestHandleDriveEvents_POST_ValidatesEventTime ensures a bad event_time
// timestamp is rejected with 400, and a valid RFC3339 value is accepted.
func TestHandleDriveEvents_POST_ValidatesEventTime(t *testing.T) {
	handler, _ := driveEventsTestServer(t)

	// Invalid event_time.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drives/disk1/events",
		strings.NewReader(`{"content":"x","event_time":"not-a-date"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad event_time, got %d", rec.Code)
	}

	// Valid event_time.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/drives/disk1/events",
		strings.NewReader(`{"content":"x","event_time":"2026-04-15T10:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 for valid event_time, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func itoaInt64(i int64) string {
	return strconv.FormatInt(i, 10)
}
