package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// borgTestResult is the response body for POST /api/v1/backup-monitor/borg/test.
// Never echoes the passphrase or any secret — user stories 9-11 from PRD #278 +
// issue #279 acceptance criteria.
type borgTestResult struct {
	OK           bool   `json:"ok"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	ArchiveCount int    `json:"archive_count,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Encryption   string `json:"encryption,omitempty"`
}

// handleTestBorgMonitor exec's a borg probe against the single entry
// in the request body and returns a structured pass/fail result.
// POST /api/v1/backup-monitor/borg/test — called by the Settings UI
// Test button (issue #279 user story 10).
//
// Security:
//   - The request body never contains a passphrase — the UI sends the
//     env var NAME only; the handler resolves the actual value from
//     its own process env (os.Getenv).
//   - The response never echoes any passphrase value, only a pass/fail
//     flag and a category string (binary_not_found, etc.).
//   - Log output from this handler avoids dumping the env map for the
//     same reason (see collector.BorgRunner doc).
func (s *Server) handleTestBorgMonitor(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()

	var req BorgExternalRepo
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	req.RepoPath = strings.TrimSpace(req.RepoPath)
	req.BinaryPath = strings.TrimSpace(req.BinaryPath)
	req.PassphraseEnv = strings.TrimSpace(req.PassphraseEnv)
	req.SSHKeyPath = strings.TrimSpace(req.SSHKeyPath)

	if req.RepoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repo_path is required"})
		return
	}
	if req.PassphraseEnv != "" && !envVarPattern.MatchString(req.PassphraseEnv) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid passphrase_env"})
		return
	}

	runner := s.borgRunner
	if runner == nil {
		runner = collector.NewExecBorgRunner()
	}

	readEnv := s.borgTestEnvLookup
	if readEnv == nil {
		readEnv = os.Getenv
	}
	env := collector.BuildBorgEnvForTest(collector.BorgExternalRepo{
		Enabled:       true,
		Label:         req.Label,
		RepoPath:      req.RepoPath,
		BinaryPath:    req.BinaryPath,
		PassphraseEnv: req.PassphraseEnv,
		SSHKeyPath:    req.SSHKeyPath,
	}, readEnv)

	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	info, runErr := runner.Info(ctx, req.RepoPath, req.BinaryPath, env)
	if runErr != nil {
		reason := collector.BorgErrUnknown
		if bre, ok := runErr.(*collector.BorgRunError); ok {
			reason = bre.Reason
		}
		// Short human-friendly message derived from the reason —
		// never includes the raw stderr blob (may contain file
		// paths / hostnames that are fine to show the user but we
		// keep the message terse for the inline UI).
		writeJSON(w, http.StatusOK, borgTestResult{
			OK:      false,
			Reason:  reason,
			Message: humanReasonMessage(reason),
		})
		s.logger.Error("borg external test failed",
			"repo", req.RepoPath,
			"reason", reason)
		return
	}

	s.logger.Info("borg external test succeeded",
		"repo", info.RepoLocation,
		"archives", info.ArchiveCount)
	writeJSON(w, http.StatusOK, borgTestResult{
		OK:           true,
		ArchiveCount: info.ArchiveCount,
		Repository:   info.RepoLocation,
		Encryption:   info.EncryptionMode,
	})
}

// humanReasonMessage maps a stable BorgErr* category to a UI-facing
// one-liner. The mapping is stable — UI assertions pin these strings.
func humanReasonMessage(reason string) string {
	switch reason {
	case collector.BorgErrBinaryNotFound:
		return "borg binary not found — check binary_path or rebuild the image"
	case collector.BorgErrRepoInaccessible:
		return "repository path is not accessible from inside the container"
	case collector.BorgErrPassphraseRejected:
		return "passphrase was rejected — check the env var named in passphrase_env"
	case collector.BorgErrSSHTimeout:
		return "SSH connection failed or timed out — check ssh_key_path and the remote host"
	case collector.BorgErrCorruptRepo:
		return "repository integrity check failed — run `borg check` on the host"
	default:
		return "unknown error — see container logs for the full borg output"
	}
}
