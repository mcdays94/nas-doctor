package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// duplicacyTestResult is the response body for
// POST /api/v1/backup-monitor/duplicacy/test. Carries the runner's
// classifier outcome plus enough summary stats to render the inline
// pill + caption in the Settings Test button. Stable JSON shape —
// the UI keys off these field names. PRD #310 / issue #313.
type duplicacyTestResult struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
	// Message is a short, human-readable hint mapped from Reason.
	// Stable phrasing — pinned by tests so future renames don't
	// silently break the inline UI.
	Message string `json:"message,omitempty"`

	// SnapshotCount is the total number of distinct snapshot
	// revision files discovered across all snapshot IDs in the
	// repo. Zero on most non-OK reasons.
	SnapshotCount int `json:"snapshot_count,omitempty"`

	// LatestBackupAt is the RFC3339 timestamp of the newest
	// snapshot. Empty when no snapshots were found.
	LatestBackupAt string `json:"latest_backup_at,omitempty"`

	// LatestBackupSizeBytes is the file_size field from the newest
	// snapshot. Zero when no snapshots were found.
	LatestBackupSizeBytes int64 `json:"latest_backup_size_bytes,omitempty"`

	// LatestBackupFiles is the number_of_files field from the
	// newest snapshot.
	LatestBackupFiles int64 `json:"latest_backup_files,omitempty"`

	// LatestSnapshotID is the snapshot id (e.g. "documents") that
	// produced the newest snapshot.
	LatestSnapshotID string `json:"latest_snapshot_id,omitempty"`

	// LatestSnapshotRevision is the integer revision of the newest
	// snapshot.
	LatestSnapshotRevision int `json:"latest_snapshot_revision,omitempty"`

	// SnapshotIDs is the sorted list of distinct snapshot ids in
	// the repo.
	SnapshotIDs []string `json:"snapshot_ids,omitempty"`

	// CurrentlyRunning is the orthogonal aux flag — true when a
	// lock or incomplete-snapshot marker is detected on disk. The
	// UI surfaces this as a small "running" badge alongside the
	// reason pill, regardless of Reason.
	CurrentlyRunning bool `json:"currently_running,omitempty"`
}

// handleTestDuplicacyMonitor runs the V1a DuplicacyRunner against the
// tentative entry in the request body and returns the outcome as JSON.
//
// POST /api/v1/backup-monitor/duplicacy/test — called by the Settings
// UI Test button (issue #313 acceptance criterion 6: live form values,
// not yet persisted).
//
// Response policy:
//   - 200 on any classifier outcome — caller inspects the `reason`
//     field. The runner is total: every input maps to one of the
//     eight DuplicacyReason codes plus the orthogonal currently_running
//     flag. PRD #310 §4.
//   - 400 on malformed JSON body or invalid Kind. Defensive — the
//     UI form-validates before posting, but a direct API caller could
//     still send garbage.
//   - 401 enforced upstream by apiKeyMiddleware (route registration).
//
// The handler does NOT persist the entry, mutate settings, write to
// the DB, or update Prometheus gauges. It is purely a single-entry
// probe — the on-demand sibling to the scheduled Collect cycle.
// PRD #310 §5.
func (s *Server) handleTestDuplicacyMonitor(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()

	var req DuplicacyEntry
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Path = strings.TrimSpace(req.Path)
	req.StorageID = strings.TrimSpace(req.StorageID)

	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	switch req.Kind {
	case collector.DuplicacyKindCLIRepo, collector.DuplicacyKindWebCache:
		// ok
	case "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind is required (cli-repo | web-cache)"})
		return
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid kind " + req.Kind + " (must be cli-repo or web-cache)",
		})
		return
	}
	if req.Kind == collector.DuplicacyKindWebCache && req.StorageID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "storage_id is required when kind=web-cache",
		})
		return
	}

	runner := s.duplicacyRunner
	if runner == nil {
		runner = collector.NewDiskDuplicacyRunner()
	}

	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	state, runErr := runner.Read(ctx, collector.DuplicacyEntry{
		Enabled:    true, // Test endpoint always runs the read regardless of stored Enabled
		Label:      req.Label,
		Kind:       req.Kind,
		Path:       req.Path,
		StorageID:  req.StorageID,
		StaleAfter: req.StaleAfter,
	})
	// V1a runner is total — runErr is reserved for genuinely unexpected
	// faults; the contract is that it never returns non-nil today.
	// Defensive: if it ever does, surface as a 200 with reason=unknown
	// + the error message, mirroring the borg path's "200 with reason"
	// principle so the UI's inspection logic stays uniform.
	if runErr != nil {
		writeJSON(w, http.StatusOK, duplicacyTestResult{
			OK:      false,
			Reason:  "unknown",
			Message: "internal runner error: " + runErr.Error(),
		})
		s.logger.Error("duplicacy external test runner error",
			"path", req.Path,
			"kind", req.Kind,
			"err", runErr)
		return
	}

	res := duplicacyTestResult{
		OK:                     state.ReasonCode == collector.DuplicacyReasonOK,
		Reason:                 string(state.ReasonCode),
		Message:                humanDuplicacyReasonMessage(state.ReasonCode),
		SnapshotCount:          state.SnapshotCount,
		LatestBackupSizeBytes:  state.LatestBackupSizeBytes,
		LatestBackupFiles:      state.LatestBackupFiles,
		LatestSnapshotID:       state.LatestSnapshotID,
		LatestSnapshotRevision: state.LatestSnapshotRevision,
		SnapshotIDs:            state.SnapshotIDs,
		CurrentlyRunning:       state.CurrentlyRunning,
	}
	if !state.LatestBackupAt.IsZero() {
		res.LatestBackupAt = state.LatestBackupAt.UTC().Format(time.RFC3339)
	}

	s.logger.Info("duplicacy external test",
		"label", req.Label,
		"kind", req.Kind,
		"reason", res.Reason,
		"snapshot_count", res.SnapshotCount,
		"currently_running", res.CurrentlyRunning)

	writeJSON(w, http.StatusOK, res)
}

// humanDuplicacyReasonMessage maps a DuplicacyReason to a stable,
// UI-facing one-liner. Pinned by tests so renames here surface as
// failing assertions rather than silent UI drift.
func humanDuplicacyReasonMessage(reason collector.DuplicacyReason) string {
	switch reason {
	case collector.DuplicacyReasonOK:
		return "repository healthy and snapshots are recent"
	case collector.DuplicacyReasonPathNotFound:
		return "path not found — check the bind mount or correct the typo"
	case collector.DuplicacyReasonPathUnreadable:
		return "path exists but is not readable from inside the container — check permissions"
	case collector.DuplicacyReasonNotARepo:
		return "path is not a Duplicacy repository — expected a .duplicacy/ subdirectory"
	case collector.DuplicacyReasonStorageIDNotFound:
		return "storage_id subdirectory not found under path — check the storage_id field"
	case collector.DuplicacyReasonNoSnapshotsYet:
		return "no snapshots yet — repo is configured but no backup has run"
	case collector.DuplicacyReasonStale:
		return "newest snapshot is older than the stale-after threshold"
	case collector.DuplicacyReasonCorruptSnapshot:
		return "at least one snapshot file failed to parse — check the repo on disk"
	default:
		return "unknown error — see container logs for the runner output"
	}
}
