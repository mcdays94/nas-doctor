package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// stubDuplicacyRunner lets tests inject a canned DuplicacyState (and
// optional error) without touching the real filesystem. Mirrors the
// stubBorgRunner shape used by the Borg endpoint tests.
type stubDuplicacyRunner struct {
	state    collector.DuplicacyState
	err      error
	sawEntry collector.DuplicacyEntry
	sawCalls int
}

func (s *stubDuplicacyRunner) Read(ctx context.Context, entry collector.DuplicacyEntry) (collector.DuplicacyState, error) {
	s.sawEntry = entry
	s.sawCalls++
	return s.state, s.err
}

// TestHandleTestDuplicacyMonitor_HappyPath_OK pins the success
// response shape — the Settings UI reads `ok`, `reason`, and the
// summary stats (snapshot_count, latest_backup_at, sizes) to render
// the inline pill + caption.
func TestHandleTestDuplicacyMonitor_HappyPath_OK(t *testing.T) {
	srv := newTestServer(t)
	when := time.Date(2026, 5, 1, 3, 30, 0, 0, time.UTC)
	srv.duplicacyRunner = &stubDuplicacyRunner{
		state: collector.DuplicacyState{
			ReasonCode:             collector.DuplicacyReasonOK,
			SnapshotCount:          42,
			LatestBackupAt:         when,
			LatestBackupSizeBytes:  1234567890,
			LatestBackupFiles:      9876,
			LatestSnapshotID:       "documents",
			LatestSnapshotRevision: 87,
			SnapshotIDs:            []string{"documents", "media"},
			CurrentlyRunning:       false,
		},
	}
	body, _ := json.Marshal(DuplicacyEntry{
		Enabled: true,
		Label:   "Documents",
		Kind:    "cli-repo",
		Path:    "/mnt/duplicacy/documents",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", w.Code, w.Body.String())
	}
	var got duplicacyTestResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK {
		t.Error("OK = false; want true for reason=ok")
	}
	if got.Reason != "ok" {
		t.Errorf("Reason = %q; want ok", got.Reason)
	}
	if got.SnapshotCount != 42 {
		t.Errorf("SnapshotCount = %d; want 42", got.SnapshotCount)
	}
	if got.LatestBackupAt != "2026-05-01T03:30:00Z" {
		t.Errorf("LatestBackupAt = %q; want 2026-05-01T03:30:00Z", got.LatestBackupAt)
	}
	if got.LatestSnapshotID != "documents" {
		t.Errorf("LatestSnapshotID = %q; want documents", got.LatestSnapshotID)
	}
	if got.LatestSnapshotRevision != 87 {
		t.Errorf("LatestSnapshotRevision = %d; want 87", got.LatestSnapshotRevision)
	}
	if got.Message == "" {
		t.Error("Message empty; UI needs a hint string for every reason")
	}
	stub := srv.duplicacyRunner.(*stubDuplicacyRunner)
	if stub.sawCalls != 1 {
		t.Errorf("runner calls = %d; want 1", stub.sawCalls)
	}
	if stub.sawEntry.Path != "/mnt/duplicacy/documents" {
		t.Errorf("runner saw Path = %q; want forwarded as-is", stub.sawEntry.Path)
	}
}

// TestHandleTestDuplicacyMonitor_AllReasonsRoundTrip pins every
// closed-set reason value through the handler. The UI's switch
// statement keys off these strings; renames here would silently
// break the inline rendering.
func TestHandleTestDuplicacyMonitor_AllReasonsRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		reason collector.DuplicacyReason
		wantOK bool
		extra  func(*collector.DuplicacyState)
		hasMsg bool
	}{
		{"ok", collector.DuplicacyReasonOK, true, nil, true},
		{"path_not_found", collector.DuplicacyReasonPathNotFound, false, nil, true},
		{"path_unreadable", collector.DuplicacyReasonPathUnreadable, false, nil, true},
		{"not_a_duplicacy_repo", collector.DuplicacyReasonNotARepo, false, nil, true},
		{"storage_id_not_found", collector.DuplicacyReasonStorageIDNotFound, false, nil, true},
		{"no_snapshots_yet", collector.DuplicacyReasonNoSnapshotsYet, false, nil, true},
		{"stale", collector.DuplicacyReasonStale, false, nil, true},
		{"corrupt_snapshot", collector.DuplicacyReasonCorruptSnapshot, false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			state := collector.DuplicacyState{ReasonCode: tc.reason}
			if tc.extra != nil {
				tc.extra(&state)
			}
			srv.duplicacyRunner = &stubDuplicacyRunner{state: state}
			body, _ := json.Marshal(DuplicacyEntry{
				Kind: "cli-repo",
				Path: "/x",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
			w := httptest.NewRecorder()
			srv.handleTestDuplicacyMonitor(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d; want 200 (handler is total — every reason is a 200)", w.Code)
			}
			var got duplicacyTestResult
			_ = json.Unmarshal(w.Body.Bytes(), &got)
			if got.OK != tc.wantOK {
				t.Errorf("OK = %v; want %v", got.OK, tc.wantOK)
			}
			if got.Reason != string(tc.reason) {
				t.Errorf("Reason = %q; want %q", got.Reason, string(tc.reason))
			}
			if tc.hasMsg && got.Message == "" {
				t.Error("Message empty; UI needs a non-empty hint for every reason")
			}
		})
	}
}

// TestHandleTestDuplicacyMonitor_CurrentlyRunningSurfacedAsAuxFlag
// pins the orthogonal aux flag — UI renders a "running" badge
// alongside the reason pill regardless of reason. PRD #310
// user story 13.
func TestHandleTestDuplicacyMonitor_CurrentlyRunningSurfacedAsAuxFlag(t *testing.T) {
	srv := newTestServer(t)
	srv.duplicacyRunner = &stubDuplicacyRunner{
		state: collector.DuplicacyState{
			ReasonCode:       collector.DuplicacyReasonOK,
			CurrentlyRunning: true,
		},
	}
	body, _ := json.Marshal(DuplicacyEntry{Kind: "cli-repo", Path: "/x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got duplicacyTestResult
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.CurrentlyRunning {
		t.Error("CurrentlyRunning = false; want true")
	}
}

// TestHandleTestDuplicacyMonitor_RejectsEmptyPath pins the 400 on
// missing path — UI form-validates but defense in depth at the API
// boundary.
func TestHandleTestDuplicacyMonitor_RejectsEmptyPath(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(DuplicacyEntry{Kind: "cli-repo", Path: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for empty path", w.Code)
	}
}

// TestHandleTestDuplicacyMonitor_RejectsMissingKind pins 400 when
// the Kind field is absent — the runner switch defaults to
// not-a-repo, but we want a clearer 400 at the API boundary.
func TestHandleTestDuplicacyMonitor_RejectsMissingKind(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(DuplicacyEntry{Path: "/x"}) // Kind = ""
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for missing kind", w.Code)
	}
}

// TestHandleTestDuplicacyMonitor_RejectsInvalidKind pins 400 for
// unknown kind values — defensive against direct API misuse.
func TestHandleTestDuplicacyMonitor_RejectsInvalidKind(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(DuplicacyEntry{Kind: "bogus", Path: "/x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for bogus kind", w.Code)
	}
}

// TestHandleTestDuplicacyMonitor_WebCacheRequiresStorageID pins
// that web-cache entries with empty StorageID are rejected — user
// story 7 (storage_id_not_found is for typo'd values; missing
// entirely is a config-validation failure).
func TestHandleTestDuplicacyMonitor_WebCacheRequiresStorageID(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(DuplicacyEntry{
		Kind: "web-cache",
		Path: "/cache",
		// StorageID intentionally empty
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for web-cache without storage_id", w.Code)
	}
	if !strings.Contains(w.Body.String(), "storage_id") {
		t.Errorf("error body should mention storage_id; got %s", w.Body.String())
	}
}

// TestHandleTestDuplicacyMonitor_RejectsMalformedJSON pins 400 on
// garbage body — basic API hygiene.
func TestHandleTestDuplicacyMonitor_RejectsMalformedJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test",
		strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for malformed json", w.Code)
	}
}

// TestHandleTestDuplicacyMonitor_RunnerErrorSurfacesAsUnknownReason
// pins the defensive code path: if the runner ever returns a non-nil
// error (the V1a contract says it never does today, but the
// signature accommodates V1c streaming extensions), the handler
// returns 200 with reason=unknown rather than a 500. Keeps the UI's
// inspection logic uniform across borg + duplicacy.
func TestHandleTestDuplicacyMonitor_RunnerErrorSurfacesAsUnknownReason(t *testing.T) {
	srv := newTestServer(t)
	srv.duplicacyRunner = &stubDuplicacyRunner{
		err: errors.New("synthetic runner failure"),
	}
	body, _ := json.Marshal(DuplicacyEntry{Kind: "cli-repo", Path: "/x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 even on runner error", w.Code)
	}
	var got duplicacyTestResult
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.OK {
		t.Error("OK = true on runner error")
	}
	if got.Reason != "unknown" {
		t.Errorf("Reason = %q; want unknown", got.Reason)
	}
}

// TestHandleTestDuplicacyMonitor_ForwardsStaleAfter pins that the
// per-entry StaleAfter is forwarded to the runner verbatim — the
// runner applies its own zero→default substitution at read time.
// The handler must NOT translate zero before calling.
func TestHandleTestDuplicacyMonitor_ForwardsStaleAfter(t *testing.T) {
	srv := newTestServer(t)
	stub := &stubDuplicacyRunner{state: collector.DuplicacyState{ReasonCode: collector.DuplicacyReasonOK}}
	srv.duplicacyRunner = stub
	body, _ := json.Marshal(DuplicacyEntry{
		Kind:       "cli-repo",
		Path:       "/x",
		StaleAfter: 7,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if stub.sawEntry.StaleAfter != 7 {
		t.Errorf("runner saw StaleAfter = %d; want 7 (handler must forward verbatim, not substitute default)", stub.sawEntry.StaleAfter)
	}
}

// TestHumanDuplicacyReasonMessage_CoversAllReasons is the locked-in
// coverage contract: every DuplicacyReason value the runner can
// return must produce a non-empty human message.
func TestHumanDuplicacyReasonMessage_CoversAllReasons(t *testing.T) {
	all := []collector.DuplicacyReason{
		collector.DuplicacyReasonOK,
		collector.DuplicacyReasonPathNotFound,
		collector.DuplicacyReasonPathUnreadable,
		collector.DuplicacyReasonNotARepo,
		collector.DuplicacyReasonStorageIDNotFound,
		collector.DuplicacyReasonNoSnapshotsYet,
		collector.DuplicacyReasonStale,
		collector.DuplicacyReasonCorruptSnapshot,
	}
	for _, r := range all {
		if humanDuplicacyReasonMessage(r) == "" {
			t.Errorf("humanDuplicacyReasonMessage(%q) = empty; every reason needs a message", r)
		}
	}
	// Unknown sentinel — also non-empty so the V1a runner's
	// defensive "unexpected" path also produces a UI hint.
	if humanDuplicacyReasonMessage(collector.DuplicacyReason("synthetic-future-value")) == "" {
		t.Error("humanDuplicacyReasonMessage default branch empty; UI needs a fallback")
	}
}

// TestHandleTestDuplicacyMonitor_DoesNotMutateSettings pins the
// "single-entry probe" contract: the test endpoint must NOT persist
// the entry, mutate settings, or write to the DB. PRD #310 §5.
func TestHandleTestDuplicacyMonitor_DoesNotMutateSettings(t *testing.T) {
	srv := newTestServer(t)
	srv.duplicacyRunner = &stubDuplicacyRunner{
		state: collector.DuplicacyState{ReasonCode: collector.DuplicacyReasonOK},
	}

	// Confirm settings are at default (no Duplicacy entries) before.
	beforeRaw, _ := srv.store.GetConfig(settingsConfigKey)
	body, _ := json.Marshal(DuplicacyEntry{
		Enabled: true, Kind: "cli-repo", Path: "/probe-only",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/duplicacy/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestDuplicacyMonitor(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	afterRaw, _ := srv.store.GetConfig(settingsConfigKey)
	if beforeRaw != afterRaw {
		t.Errorf("settings blob mutated by Test endpoint; before=%q after=%q", beforeRaw, afterRaw)
	}
}
