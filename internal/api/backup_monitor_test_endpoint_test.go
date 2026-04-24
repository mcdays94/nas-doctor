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

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// stubBorgRunner lets tests inject an expected result / error without
// any subprocess. Mirrors the collector-package fakeBorgRunner but
// lives in api-test scope.
type stubBorgRunner struct {
	info    collector.BorgInfoJSON
	err     error
	sawEnv  map[string]string
	sawPath string
}

func (s *stubBorgRunner) Info(ctx context.Context, repoPath, binaryPath string, env map[string]string) (collector.BorgInfoJSON, error) {
	s.sawPath = repoPath
	s.sawEnv = env
	return s.info, s.err
}

// TestHandleTestBorgMonitor_HappyPathReturnsArchiveCount pins the
// success response shape — UI inspects archive_count and repository
// to show the user confirmation inline.
func TestHandleTestBorgMonitor_HappyPathReturnsArchiveCount(t *testing.T) {
	srv := newTestServer(t)
	srv.borgRunner = &stubBorgRunner{
		info: collector.BorgInfoJSON{
			ArchiveCount:   12,
			RepoLocation:   "/mnt/backups/main",
			EncryptionMode: "repokey-blake2",
		},
	}
	body, _ := json.Marshal(BorgExternalRepo{RepoPath: "/mnt/backups/main", Enabled: true})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/borg/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestBorgMonitor(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var got borgTestResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK {
		t.Error("OK = false; want true")
	}
	if got.ArchiveCount != 12 {
		t.Errorf("ArchiveCount = %d; want 12", got.ArchiveCount)
	}
	if got.Repository != "/mnt/backups/main" {
		t.Errorf("Repository = %q; want /mnt/backups/main", got.Repository)
	}
	if got.Encryption != "repokey-blake2" {
		t.Errorf("Encryption = %q; want repokey-blake2", got.Encryption)
	}
}

// TestHandleTestBorgMonitor_FailureSurfacesStableReason pins each
// BorgErr* category in the response body — the UI keys off these
// strings for user-visible messaging.
func TestHandleTestBorgMonitor_FailureSurfacesStableReason(t *testing.T) {
	cases := []struct {
		name   string
		reason string
	}{
		{"binary not found", collector.BorgErrBinaryNotFound},
		{"repo inaccessible", collector.BorgErrRepoInaccessible},
		{"passphrase rejected", collector.BorgErrPassphraseRejected},
		{"ssh timeout", collector.BorgErrSSHTimeout},
		{"corrupt repo", collector.BorgErrCorruptRepo},
		{"unknown", collector.BorgErrUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			srv.borgRunner = &stubBorgRunner{err: &collector.BorgRunError{Reason: tc.reason, Err: errors.New("underlying")}}
			body, _ := json.Marshal(BorgExternalRepo{RepoPath: "/r", Enabled: true})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/borg/test", bytes.NewReader(body))
			w := httptest.NewRecorder()
			srv.handleTestBorgMonitor(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d", w.Code)
			}
			var got borgTestResult
			_ = json.Unmarshal(w.Body.Bytes(), &got)
			if got.OK {
				t.Error("OK = true on failure case")
			}
			if got.Reason != tc.reason {
				t.Errorf("Reason = %q; want %q", got.Reason, tc.reason)
			}
			if got.Message == "" {
				t.Error("Message empty; UI needs a human-readable tip")
			}
		})
	}
}

// TestHandleTestBorgMonitor_NeverEchoesPassphrase is a security-
// critical assertion — the Test endpoint resolves the passphrase from
// the server process env but MUST never include its value in the
// response body. User story from PRD #278: secrets only live in
// Docker env, never in responses or logs.
func TestHandleTestBorgMonitor_NeverEchoesPassphrase(t *testing.T) {
	const secret = "SUPER-SECRET-DO-NOT-LEAK"
	srv := newTestServer(t)
	srv.borgRunner = &stubBorgRunner{info: collector.BorgInfoJSON{ArchiveCount: 1}}
	srv.borgTestEnvLookup = func(name string) string {
		if name == "BORG_PASSPHRASE_MAIN" {
			return secret
		}
		return ""
	}
	body, _ := json.Marshal(BorgExternalRepo{
		RepoPath:      "/r",
		PassphraseEnv: "BORG_PASSPHRASE_MAIN",
		Enabled:       true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/borg/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestBorgMonitor(w, req)
	if strings.Contains(w.Body.String(), secret) {
		t.Fatalf("response body leaked passphrase value: %s", w.Body.String())
	}
	// Confirm the runner actually saw the resolved env — the
	// plumbing worked, the leak just didn't happen.
	stub := srv.borgRunner.(*stubBorgRunner)
	if stub.sawEnv["BORG_PASSPHRASE"] != secret {
		t.Errorf("runner env BORG_PASSPHRASE = %q; want secret forwarded via custom env var lookup", stub.sawEnv["BORG_PASSPHRASE"])
	}
}

// TestHandleTestBorgMonitor_RejectsEmptyRepoPath pins the 400 on
// invalid input — matches PUT validation surface.
func TestHandleTestBorgMonitor_RejectsEmptyRepoPath(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(BorgExternalRepo{RepoPath: "", Enabled: true})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/borg/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestBorgMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

// TestHandleTestBorgMonitor_RejectsInvalidPassphraseEnv pins that
// bogus env var names are rejected at the Test endpoint just like
// PUT — defence in depth against malformed client requests.
func TestHandleTestBorgMonitor_RejectsInvalidPassphraseEnv(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(BorgExternalRepo{
		RepoPath:      "/r",
		PassphraseEnv: "has-dash",
		Enabled:       true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-monitor/borg/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleTestBorgMonitor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for invalid passphrase_env", w.Code)
	}
}

// TestHumanReasonMessage_CoversAllBorgErrCategories is a locked-in
// coverage contract: every reason the runner can return must produce
// a non-empty human message.
func TestHumanReasonMessage_CoversAllBorgErrCategories(t *testing.T) {
	categories := []string{
		collector.BorgErrBinaryNotFound,
		collector.BorgErrRepoInaccessible,
		collector.BorgErrPassphraseRejected,
		collector.BorgErrSSHTimeout,
		collector.BorgErrCorruptRepo,
		collector.BorgErrUnknown,
	}
	for _, c := range categories {
		if humanReasonMessage(c) == "" {
			t.Errorf("humanReasonMessage(%q) = empty; every category needs a message", c)
		}
	}
}
