package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BorgRunner is the deep-module interface that separates Borg subprocess
// mechanics from the rest of the backup collector. Production code uses
// execBorgRunner (exec.CommandContext). Tests inject a fakeBorgRunner to
// exercise every error branch without shelling out. See issue #279 and
// PRD #278.
//
// Info runs `borg list --json <repoPath>` and `borg info --last 1 --json
// <repoPath>` and composes the two outputs into a single structured
// return. The two-call dance is required since modern Borg 1.4+ dropped
// the archives array from `borg info --json <repo>` — see the scope
// addendum on issue #279 for UAT context.
//
// env carries supplemental environment variables (BORG_PASSPHRASE,
// BORG_RSH for SSH keys, BORG_RELOCATED_REPO_ACCESS_IS_OK, etc.). The
// production runner merges env onto the ambient process env; the test
// runner ignores it. Callers must never log the env map — it carries
// secrets.
//
// binaryPath defaults to "borg" and should resolve via $PATH to the
// Alpine-bundled /usr/bin/borg inside the nas-doctor image. Users can
// override at config time (e.g. to a mounted custom build), but the
// common case requires no override.
type BorgRunner interface {
	Info(ctx context.Context, repoPath, binaryPath string, env map[string]string) (BorgInfoJSON, error)
}

// BorgInfoJSON composes the two Borg JSON payloads into a single
// result. ArchiveCount comes from the full `borg list --json` payload;
// LatestArchive (if non-nil) comes from `borg info --last 1 --json`
// which provides richer per-archive statistics than the list view.
// RepoLocation / LastModified / EncryptionMode come from the `info`
// payload's repository + encryption blocks.
type BorgInfoJSON struct {
	ArchiveCount   int
	LatestArchive  *BorgArchive
	RepoLocation   string
	LastModified   time.Time
	EncryptionMode string
}

// BorgArchive mirrors the per-archive fields we consume from
// `borg info --last 1 --json`. Subset of the upstream schema — we
// deliberately pick only what the dashboard + BackupJob renderer
// needs.
type BorgArchive struct {
	Name         string
	Start        time.Time
	End          time.Time
	NFiles       int
	OriginalSize int64
}

// Borg exit codes / error reasons we care about. Used to produce a
// short, category-style ErrorReason on BackupJob so the dashboard
// widget can render specific messages without leaking subprocess
// stderr text. See issue #279 acceptance criteria.
const (
	BorgErrBinaryNotFound     = "binary_not_found"
	BorgErrRepoInaccessible   = "repo_inaccessible"
	BorgErrPassphraseRejected = "passphrase_rejected"
	BorgErrSSHTimeout         = "ssh_timeout"
	BorgErrCorruptRepo        = "corrupt_repo"
	BorgErrRepoReadOnly       = "repo_readonly"
	BorgErrUnknown            = "unknown"
)

// BorgRunError wraps a short category string and the underlying error.
// Callers compare e.Reason to the BorgErr* constants to build
// user-visible messaging; the full err is retained for log output.
type BorgRunError struct {
	Reason string
	Err    error
}

func (e *BorgRunError) Error() string {
	if e.Err == nil {
		return e.Reason
	}
	return e.Reason + ": " + e.Err.Error()
}

func (e *BorgRunError) Unwrap() error { return e.Err }

// Default timeouts. Local repos typically respond in under a second;
// remote SSH repos might take longer. We bias toward the low end so
// an unresponsive repo doesn't hold up a scan tick — the user can
// see the timeout reason on the dashboard and investigate.
const (
	borgDefaultTimeout = 30 * time.Second
	borgMaxTimeout     = 120 * time.Second
)

// execBorgRunner is the production BorgRunner. It shells out to `borg`
// via exec.CommandContext with a bounded timeout; the env map is
// merged onto the ambient process env. See BorgRunner interface doc
// for semantics.
type execBorgRunner struct{}

// NewExecBorgRunner returns the production BorgRunner. Callers usually
// inject this at server wiring time (cmd/nas-doctor/main.go) so tests
// can substitute a fake.
func NewExecBorgRunner() BorgRunner { return &execBorgRunner{} }

// Info runs the two-call sequence and composes the result. The first
// call (`borg list --json`) produces the archive count; the second
// (`borg info --last 1 --json`) produces the detailed last-archive
// stats plus repo/encryption metadata. Either call can fail; we map
// stderr / exit-code patterns to BorgRunError.Reason and return.
func (r *execBorgRunner) Info(ctx context.Context, repoPath, binaryPath string, env map[string]string) (BorgInfoJSON, error) {
	binary := binaryPath
	if strings.TrimSpace(binary) == "" {
		binary = "borg"
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// borg list --bypass-lock --json <repo>. --bypass-lock is required
	// to support Read-Only-mounted repos, since borg's default lock
	// file creation would otherwise fail with ENOFILE (issue #279 rc2
	// Finding 1). The theoretical race with a concurrent host-side
	// `borg create` is benign for a read-only monitoring tool.
	listOut, listErr := r.run(ctx, binary, buildBorgListArgs(repoPath), env)
	if listErr != nil {
		return BorgInfoJSON{}, classifyBorgError(binary, listErr, listOut)
	}
	var listPayload struct {
		Archives []struct {
			Name string `json:"name"`
		} `json:"archives"`
	}
	if err := json.Unmarshal([]byte(listOut), &listPayload); err != nil {
		return BorgInfoJSON{}, &BorgRunError{Reason: BorgErrUnknown, Err: fmt.Errorf("parse list json: %w", err)}
	}

	result := BorgInfoJSON{ArchiveCount: len(listPayload.Archives)}

	// borg info --last 1 --json — only meaningful when at least one
	// archive exists. For empty repos we skip the second call and
	// return the structured list payload.
	if result.ArchiveCount == 0 {
		// Still need repo metadata (location + encryption mode) for
		// the dashboard card. Fall back to `borg info --json` which
		// returns repository metadata even with no archives. Same
		// --bypass-lock treatment as the list + info-last calls.
		infoOut, infoErr := r.run(ctx, binary, buildBorgInfoMetadataArgs(repoPath), env)
		if infoErr == nil {
			if meta, err := parseBorgInfoMetadata(infoOut); err == nil {
				result.RepoLocation = meta.Location
				result.LastModified = meta.LastModified
				result.EncryptionMode = meta.EncryptionMode
			}
		}
		return result, nil
	}

	infoOut, infoErr := r.run(ctx, binary, buildBorgInfoLastArgs(repoPath), env)
	if infoErr != nil {
		return BorgInfoJSON{}, classifyBorgError(binary, infoErr, infoOut)
	}

	meta, latest, err := parseBorgInfoLast(infoOut)
	if err != nil {
		return BorgInfoJSON{}, &BorgRunError{Reason: BorgErrUnknown, Err: fmt.Errorf("parse info json: %w", err)}
	}
	result.RepoLocation = meta.Location
	result.LastModified = meta.LastModified
	result.EncryptionMode = meta.EncryptionMode
	result.LatestArchive = latest
	return result, nil
}

// run executes a single borg subcommand with the merged env and a
// bounded timeout. The context.DeadlineExceeded case is classified as
// ssh_timeout because any remote repo is the most likely cause of a
// timeout in practice; purely-local unresponsive repos are rare.
func (r *execBorgRunner) run(parent context.Context, binary string, args []string, env map[string]string) (string, error) {
	timeout := borgDefaultTimeout
	// Widen the deadline when an SSH repo is in play; harmless for
	// local repos and avoids the SSH handshake eating the full budget
	// before borg can read the manifest.
	for _, a := range args {
		if strings.HasPrefix(a, "ssh://") {
			timeout = borgMaxTimeout
			break
		}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// Separate stdout from stderr (issue #279 rc3 Finding A). Borg emits
	// the auto-accepted "unknown unencrypted repo" / "relocated repo"
	// warnings to stderr even on success, so combining streams
	// contaminates the JSON payload on stdout and breaks
	// json.Unmarshal. On success we return stdout (clean JSON); on
	// error we return stderr (what classifyBorgError matches against).
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), buildRunnerEnv(env)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		return stderr.String(), err
	}
	return string(stdout), nil
}

// buildBorgListArgs returns the argv for `borg list --bypass-lock --json
// <repoPath>`. Extracted as a pure function so tests can pin the flag
// set (issue #279 rc2 Finding 1).
func buildBorgListArgs(repoPath string) []string {
	return []string{"list", "--bypass-lock", "--json", repoPath}
}

// buildBorgInfoLastArgs returns the argv for `borg info --last 1
// --bypass-lock --json <repoPath>`. Used to fetch the latest archive's
// detailed stats when list has reported a non-zero count.
func buildBorgInfoLastArgs(repoPath string) []string {
	return []string{"info", "--last", "1", "--bypass-lock", "--json", repoPath}
}

// buildBorgInfoMetadataArgs returns the argv for the empty-repo
// fallback path: `borg info --bypass-lock --json <repoPath>` gives
// repository + encryption metadata even when no archives exist.
func buildBorgInfoMetadataArgs(repoPath string) []string {
	return []string{"info", "--bypass-lock", "--json", repoPath}
}

// buildRunnerEnv composes the final subprocess env for a Borg
// invocation. Two env vars are ALWAYS set regardless of whether the
// caller supplied an env map (issue #279 rc2 Finding 2):
//
//   - BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK=yes suppresses the
//     interactive fingerprint-first-access prompt on unencrypted repos
//   - BORG_RELOCATED_REPO_ACCESS_IS_OK=yes suppresses the prompt on
//     repos accessed via a new bind-mount path
//
// Both prompts otherwise hang a non-TTY subprocess. Caller-supplied
// values for these same keys WIN — the runner defaults only fill in
// when the caller hasn't set a value. Keys with empty values are
// still emitted (borg treats empty BORG_PASSPHRASE as "no passphrase
// provided" — distinct from unset).
func buildRunnerEnv(env map[string]string) []string {
	defaults := map[string]string{
		"BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK": "yes",
		"BORG_RELOCATED_REPO_ACCESS_IS_OK":           "yes",
	}
	merged := make(map[string]string, len(defaults)+len(env))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range env {
		merged[k] = v
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, k+"="+v)
	}
	return out
}

// classifyBorgError maps an exec error + stderr output to a
// BorgRunError with a stable category. Category strings are stable —
// the dashboard widget keys off them for user-visible messaging.
func classifyBorgError(binary string, runErr error, out string) error {
	low := strings.ToLower(out)
	// exec.LookPath-style "not found" maps to binary_not_found. The
	// Alpine-musl vs glibc-borg ABI mismatch also surfaces this way
	// (see issue #279 architecture correction) — same bucket is fine
	// since the user-facing fix is identical (bundle in image or
	// provide a musl-compatible override).
	var pathErr *exec.Error
	if errors.As(runErr, &pathErr) {
		return &BorgRunError{Reason: BorgErrBinaryNotFound, Err: runErr}
	}
	if strings.Contains(low, "exec format error") || strings.Contains(low, "not found") {
		if _, err := exec.LookPath(binary); err != nil {
			return &BorgRunError{Reason: BorgErrBinaryNotFound, Err: runErr}
		}
	}
	if errors.Is(runErr, context.DeadlineExceeded) {
		return &BorgRunError{Reason: BorgErrSSHTimeout, Err: runErr}
	}
	switch {
	// Defense-in-depth: with --bypass-lock this path should never fire,
	// but older borg versions (pre-1.2) lack the flag. A specific
	// category lets the UI surface a targeted fix hint (issue #279 rc2
	// Finding 2 classifier addition).
	case strings.Contains(low, "read-only file system"):
		return &BorgRunError{Reason: BorgErrRepoReadOnly, Err: runErr}
	case strings.Contains(low, "passphrase supplied"),
		strings.Contains(low, "passphrase is incorrect"),
		strings.Contains(low, "passphrase required"),
		strings.Contains(low, "does not match"):
		return &BorgRunError{Reason: BorgErrPassphraseRejected, Err: runErr}
	case strings.Contains(low, "connection timed out"),
		strings.Contains(low, "connection refused"),
		strings.Contains(low, "host key verification failed"),
		strings.Contains(low, "permission denied (publickey"):
		return &BorgRunError{Reason: BorgErrSSHTimeout, Err: runErr}
	case strings.Contains(low, "integrityerror"),
		strings.Contains(low, "repository manifest is corrupted"),
		strings.Contains(low, "corruption"):
		return &BorgRunError{Reason: BorgErrCorruptRepo, Err: runErr}
	case strings.Contains(low, "does not exist"),
		strings.Contains(low, "no such file or directory"),
		strings.Contains(low, "not a valid repository"),
		strings.Contains(low, "is not a valid repository"):
		return &BorgRunError{Reason: BorgErrRepoInaccessible, Err: runErr}
	}
	return &BorgRunError{Reason: BorgErrUnknown, Err: fmt.Errorf("%w: %s", runErr, trimForLog(out))}
}

// trimForLog truncates a multiline stderr blob to something reasonable
// for structured logs; avoids dumping 200-line ssh banners.
func trimForLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// borgRepoMeta holds the repository + encryption metadata extracted
// from either `borg info --json` or `borg info --last 1 --json`.
type borgRepoMeta struct {
	Location       string
	LastModified   time.Time
	EncryptionMode string
}

// parseBorgInfoMetadata decodes the metadata-only subset of a `borg
// info --json <repo>` payload — used for empty-repo lookups where
// there's no latest archive to extract stats from.
func parseBorgInfoMetadata(raw string) (borgRepoMeta, error) {
	var payload struct {
		Repository struct {
			Location     string `json:"location"`
			LastModified string `json:"last_modified"`
		} `json:"repository"`
		Encryption struct {
			Mode string `json:"mode"`
		} `json:"encryption"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return borgRepoMeta{}, err
	}
	return borgRepoMeta{
		Location:       payload.Repository.Location,
		LastModified:   parseBorgTime(payload.Repository.LastModified),
		EncryptionMode: payload.Encryption.Mode,
	}, nil
}

// parseBorgInfoLast decodes both the repo metadata and the single
// latest-archive entry from a `borg info --last 1 --json` payload.
func parseBorgInfoLast(raw string) (borgRepoMeta, *BorgArchive, error) {
	var payload struct {
		Repository struct {
			Location     string `json:"location"`
			LastModified string `json:"last_modified"`
		} `json:"repository"`
		Encryption struct {
			Mode string `json:"mode"`
		} `json:"encryption"`
		Archives []struct {
			Name  string `json:"name"`
			Start string `json:"start"`
			End   string `json:"end"`
			Stats struct {
				NFiles       int   `json:"nfiles"`
				OriginalSize int64 `json:"original_size"`
			} `json:"stats"`
		} `json:"archives"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return borgRepoMeta{}, nil, err
	}
	meta := borgRepoMeta{
		Location:       payload.Repository.Location,
		LastModified:   parseBorgTime(payload.Repository.LastModified),
		EncryptionMode: payload.Encryption.Mode,
	}
	if len(payload.Archives) == 0 {
		return meta, nil, nil
	}
	a := payload.Archives[0]
	return meta, &BorgArchive{
		Name:         a.Name,
		Start:        parseBorgTime(a.Start),
		End:          parseBorgTime(a.End),
		NFiles:       a.Stats.NFiles,
		OriginalSize: a.Stats.OriginalSize,
	}, nil
}

// parseBorgTime parses Borg's JSON timestamp format. UAT captured on
// 2026-04-24 (Borg 1.4.4): "2026-04-24T19:25:39.000000" — no
// timezone suffix, 6 fractional digits. We try that layout first, then
// fall back to RFC3339 variants for older / remote versions.
func parseBorgTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05.000000Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// CanonicalRepoPath normalizes a Borg repository path for deduping
// purposes. Remote (ssh://) repos are returned as-is; local paths are
// Clean + Abs-resolved. Returns the input unchanged if Abs() fails so
// callers can still dedupe on the literal input. Used by
// collectBackups to merge auto-detected and explicitly-configured
// repos without double-rendering the same repo.
func CanonicalRepoPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.Contains(p, "://") {
		return p
	}
	clean := filepath.Clean(p)
	if abs, err := filepath.Abs(clean); err == nil {
		return abs
	}
	return clean
}
