package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// TestHandleLatestSnapshot_ExposesSubsystemLastRan confirms that when
// a snapshot carries the SubsystemLastRan map (populated by the
// scheduler's RunOnce after each dispatcher tick), the field is
// serialised as `subsystem_last_ran` on /api/v1/snapshot/latest.
//
// This is the API-data surface for issue #260 user story 17. UI
// consumers (future dashboard "last scanned 4m ago" indicators) will
// read this JSON field directly. Dashboard UI is out of scope for
// this slice — the contract under test is the JSON shape only.
func TestHandleLatestSnapshot_ExposesSubsystemLastRan(t *testing.T) {
	srv := newSettingsTestServer()

	// Seed a snapshot with SubsystemLastRan populated.
	now := time.Now().UTC().Truncate(time.Second)
	snap := &internal.Snapshot{
		ID:        "test-lastran-snap",
		Timestamp: now,
		SubsystemLastRan: map[string]string{
			"smart":  now.Add(-1 * time.Hour).Format(time.RFC3339),
			"docker": now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/latest returned %d: %s", rec.Code, rec.Body.String())
	}

	// Parse as generic map so we can assert on JSON key presence
	// (the internal.Snapshot Go struct uses omitempty, so absence
	// would look like an empty map after unmarshal).
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse snapshot JSON: %v", err)
	}
	raw, ok := got["subsystem_last_ran"]
	if !ok {
		t.Fatalf("subsystem_last_ran key missing from snapshot JSON; got %v", got)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("subsystem_last_ran is not an object; got %T", raw)
	}
	if len(m) != 2 {
		t.Errorf("subsystem_last_ran size = %d, want 2; got %+v", len(m), m)
	}
	if _, ok := m["smart"]; !ok {
		t.Errorf("smart missing from subsystem_last_ran; got %+v", m)
	}
	if _, ok := m["docker"]; !ok {
		t.Errorf("docker missing from subsystem_last_ran; got %+v", m)
	}

	// Verify the values are parseable RFC3339 timestamps.
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			t.Errorf("subsystem_last_ran[%q] is not a string; got %T", k, v)
			continue
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			t.Errorf("subsystem_last_ran[%q]=%q not valid RFC3339: %v", k, s, err)
		}
	}
}

// TestHandleLatestSnapshot_OmitsSubsystemLastRan_WhenEmpty ensures
// the field does not appear on the JSON when the map is nil/empty
// (omitempty tag). Demo-mode snapshots built synthetically by the
// feeder don't populate this map — without omitempty, they'd render
// a misleading "subsystem_last_ran: {}" entry that UI would have to
// filter out.
func TestHandleLatestSnapshot_OmitsSubsystemLastRan_WhenEmpty(t *testing.T) {
	srv := newSettingsTestServer()
	snap := &internal.Snapshot{
		ID:        "test-noplastran",
		Timestamp: time.Now().UTC(),
	}
	if err := srv.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/latest", nil)
	rec := httptest.NewRecorder()
	srv.handleLatestSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/latest returned %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse snapshot JSON: %v", err)
	}
	if _, present := got["subsystem_last_ran"]; present {
		t.Errorf("subsystem_last_ran should be absent when map is nil; got %+v", got["subsystem_last_ran"])
	}
}
