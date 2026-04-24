package collector

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// realisticListJSON is the unencrypted-3-archive payload captured
// from Borg 1.4.4 on 2026-04-24 during UAT. Timestamps use Borg's
// "no-timezone, 6-fractional" format ("2026-04-24T19:25:39.000000")
// which parseBorgTime must accept.
const realisticListJSON = `{
    "archives": [
        {"archive":"archive-1","barchive":"archive-1","id":"124b73d7ed77","name":"archive-1","start":"2026-04-24T19:24:58.000000","time":"2026-04-24T19:24:58.000000"},
        {"archive":"archive-2","barchive":"archive-2","id":"0a2189232ae6","name":"archive-2","start":"2026-04-24T19:25:22.000000","time":"2026-04-24T19:25:22.000000"},
        {"archive":"archive-3","barchive":"archive-3","id":"e1744f9cb5bc","name":"archive-3","start":"2026-04-24T19:25:39.000000","time":"2026-04-24T19:25:39.000000"}
    ],
    "encryption": {"mode": "none"},
    "repository": {"id":"efdf7da294","last_modified":"2026-04-24T19:25:39.000000","location":"/mnt/user/appdata/borg/repos/test-unencrypted"}
}`

// realisticInfoLast1JSON is the `borg info --last 1 --json` payload
// captured from the same 3-archive repo on 2026-04-24.
const realisticInfoLast1JSON = `{
    "archives": [
        {
            "chunker_params":["buzhash",19,23,21,4095],
            "command_line":["/mnt/user/appdata/borg/borg","create","borg::archive-3","/tmp/borg-test-content"],
            "comment":"",
            "cwd":"/root",
            "duration":0.00808,
            "end":"2026-04-24T19:25:39.000000",
            "hostname":"Tower",
            "id":"e1744f9cb5bc",
            "name":"archive-3",
            "start":"2026-04-24T19:25:39.000000",
            "stats":{"compressed_size":141,"deduplicated_size":87,"nfiles":3,"original_size":198},
            "username":"root"
        }
    ],
    "cache": {"path":"..."},
    "encryption": {"mode":"none"},
    "repository": {"id":"efdf7da294","last_modified":"2026-04-24T19:25:39.000000","location":"/mnt/user/appdata/borg/repos/test-unencrypted"}
}`

// emptyRepoInfoJSON represents an initialised but never-written repo —
// no archives, just repo metadata + encryption mode.
const emptyRepoInfoJSON = `{
    "cache": {"path":"..."},
    "encryption": {"mode": "repokey-blake2"},
    "repository": {"id":"aabbcc","last_modified":"2026-04-23T12:00:00.000000","location":"/mnt/user/appdata/borg/repos/brand-new"}
}`

const emptyRepoListJSON = `{
    "archives": [],
    "encryption": {"mode": "repokey-blake2"},
    "repository": {"id":"aabbcc","last_modified":"2026-04-23T12:00:00.000000","location":"/mnt/user/appdata/borg/repos/brand-new"}
}`

// fakeBorgRunner is an in-process BorgRunner implementation used by
// unit tests. Construct one with a map of (repoPath) → scenario, or
// preload per-scenario return values via the helper constructors.
type fakeBorgRunner struct {
	byRepo map[string]fakeBorgScenario
}

// fakeBorgScenario describes a canned outcome for one repoPath.
// Either Info is populated (success) or Err is populated (failure).
// calls tracks how many times the runner was invoked for the repo
// (useful for dedup + two-call assertions).
type fakeBorgScenario struct {
	Info  BorgInfoJSON
	Err   error
	calls int
}

func newFakeBorgRunner() *fakeBorgRunner {
	return &fakeBorgRunner{byRepo: make(map[string]fakeBorgScenario)}
}

func (r *fakeBorgRunner) set(repoPath string, s fakeBorgScenario) {
	r.byRepo[repoPath] = s
}

func (r *fakeBorgRunner) Info(ctx context.Context, repoPath, binaryPath string, env map[string]string) (BorgInfoJSON, error) {
	s, ok := r.byRepo[repoPath]
	if !ok {
		return BorgInfoJSON{}, &BorgRunError{Reason: BorgErrRepoInaccessible, Err: errors.New("fake: no scenario configured")}
	}
	s.calls++
	r.byRepo[repoPath] = s
	return s.Info, s.Err
}

// TestBorgRunner_Contract_HealthyRepoReturnsPopulatedInfoJSON ensures
// a successful fake response round-trips all the BorgInfoJSON fields
// the collector needs. Regression-guards the contract for the
// production runner: any field added here must be mirrored in the
// two-call composition.
func TestBorgRunner_Contract_HealthyRepoReturnsPopulatedInfoJSON(t *testing.T) {
	r := newFakeBorgRunner()
	r.set("/mnt/backups/main", fakeBorgScenario{
		Info: BorgInfoJSON{
			ArchiveCount:   3,
			RepoLocation:   "/mnt/backups/main",
			EncryptionMode: "none",
			LatestArchive: &BorgArchive{
				Name:         "archive-3",
				NFiles:       3,
				OriginalSize: 198,
			},
		},
	})
	got, err := r.Info(context.Background(), "/mnt/backups/main", "borg", nil)
	if err != nil {
		t.Fatalf("Info err = %v; want nil", err)
	}
	if got.ArchiveCount != 3 {
		t.Errorf("ArchiveCount = %d; want 3", got.ArchiveCount)
	}
	if got.LatestArchive == nil {
		t.Fatal("LatestArchive nil; want populated")
	}
	if got.LatestArchive.Name != "archive-3" {
		t.Errorf("LatestArchive.Name = %q; want archive-3", got.LatestArchive.Name)
	}
}

// TestBorgRunner_Contract_EachErrorReasonSurfaces verifies every
// documented BorgRunError.Reason category round-trips through the
// interface. Pinning each reason separately means the dashboard can
// trust the set of values it renders.
func TestBorgRunner_Contract_EachErrorReasonSurfaces(t *testing.T) {
	cases := []struct {
		name   string
		reason string
	}{
		{"binary not found", BorgErrBinaryNotFound},
		{"repo inaccessible", BorgErrRepoInaccessible},
		{"passphrase rejected", BorgErrPassphraseRejected},
		{"ssh timeout", BorgErrSSHTimeout},
		{"corrupt repo", BorgErrCorruptRepo},
		{"repo readonly", BorgErrRepoReadOnly},
		{"unknown", BorgErrUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newFakeBorgRunner()
			r.set("/repo", fakeBorgScenario{Err: &BorgRunError{Reason: tc.reason, Err: errors.New("underlying")}})
			_, err := r.Info(context.Background(), "/repo", "borg", nil)
			if err == nil {
				t.Fatal("want non-nil error")
			}
			var bre *BorgRunError
			if !errors.As(err, &bre) {
				t.Fatalf("error is not *BorgRunError: %v", err)
			}
			if bre.Reason != tc.reason {
				t.Errorf("Reason = %q; want %q", bre.Reason, tc.reason)
			}
		})
	}
}

// TestParseBorgTime_AcceptsBorgNativeFormat pins that the 6-fractional
// no-timezone shape emitted by Borg 1.4.x on UAT is parsed. Previously
// the latent-bug auto-detect path used the same layout in queryBorgRepo,
// so we re-exercise it here as part of the contract.
func TestParseBorgTime_AcceptsBorgNativeFormat(t *testing.T) {
	in := "2026-04-24T19:25:39.000000"
	got := parseBorgTime(in)
	if got.IsZero() {
		t.Fatalf("parseBorgTime(%q) = zero; want parsed time", in)
	}
	if got.Year() != 2026 || got.Month() != 4 || got.Day() != 24 || got.Hour() != 19 {
		t.Errorf("parseBorgTime(%q) = %v; unexpected components", in, got)
	}
}

// TestParseBorgTime_RejectsGarbage returns a zero time for non-
// parseable input instead of panicking.
func TestParseBorgTime_RejectsGarbage(t *testing.T) {
	if !parseBorgTime("not-a-timestamp").IsZero() {
		t.Error("parseBorgTime garbage; want zero time")
	}
	if !parseBorgTime("").IsZero() {
		t.Error("parseBorgTime empty; want zero time")
	}
}

// TestParseBorgInfoLast_ExtractsLatestArchiveStats parses the UAT-
// captured `borg info --last 1 --json` payload and checks all fields
// the collector consumes are correctly populated.
func TestParseBorgInfoLast_ExtractsLatestArchiveStats(t *testing.T) {
	meta, latest, err := parseBorgInfoLast(realisticInfoLast1JSON)
	if err != nil {
		t.Fatalf("parseBorgInfoLast err = %v", err)
	}
	if meta.Location != "/mnt/user/appdata/borg/repos/test-unencrypted" {
		t.Errorf("Location = %q; want repo path", meta.Location)
	}
	if meta.EncryptionMode != "none" {
		t.Errorf("EncryptionMode = %q; want none", meta.EncryptionMode)
	}
	if latest == nil {
		t.Fatal("latest nil; want populated archive")
	}
	if latest.Name != "archive-3" {
		t.Errorf("latest.Name = %q; want archive-3", latest.Name)
	}
	if latest.NFiles != 3 {
		t.Errorf("latest.NFiles = %d; want 3", latest.NFiles)
	}
	if latest.OriginalSize != 198 {
		t.Errorf("latest.OriginalSize = %d; want 198", latest.OriginalSize)
	}
	if latest.Start.IsZero() {
		t.Error("latest.Start is zero; want parsed timestamp")
	}
}

// TestParseBorgInfoMetadata_EmptyRepoStillYieldsMetadata pins that an
// initialised-but-empty repo still produces location + encryption
// mode; the caller can then skip the --last 1 --json step.
func TestParseBorgInfoMetadata_EmptyRepoStillYieldsMetadata(t *testing.T) {
	meta, err := parseBorgInfoMetadata(emptyRepoInfoJSON)
	if err != nil {
		t.Fatalf("parseBorgInfoMetadata err = %v", err)
	}
	if meta.EncryptionMode != "repokey-blake2" {
		t.Errorf("EncryptionMode = %q; want repokey-blake2", meta.EncryptionMode)
	}
	if meta.Location == "" {
		t.Error("Location empty; want repo path")
	}
}

// TestClassifyBorgError_PassphraseRejectedFromStderr pins the
// error-classification mapping for common borg stderr phrases.
func TestClassifyBorgError_PassphraseRejectedFromStderr(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		reason string
	}{
		{"passphrase supplied", "passphrase supplied in BORG_PASSPHRASE is incorrect", BorgErrPassphraseRejected},
		{"ssh connection refused", "ssh: connect to host: Connection refused", BorgErrSSHTimeout},
		{"repo does not exist", "Repository /mnt/nonexistent does not exist.", BorgErrRepoInaccessible},
		{"integrity error", "IntegrityError: Data integrity error", BorgErrCorruptRepo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyBorgError("borg", errors.New("exit 2"), tc.stderr)
			var bre *BorgRunError
			if !errors.As(got, &bre) {
				t.Fatalf("classifyBorgError not *BorgRunError: %v", got)
			}
			if bre.Reason != tc.reason {
				t.Errorf("Reason = %q; want %q (stderr=%q)", bre.Reason, tc.reason, tc.stderr)
			}
		})
	}
}

// TestClassifyBorgError_ContextDeadlineExceededMapsToSSHTimeout pins
// the specific mapping for context cancellation → ssh_timeout. We
// chose ssh_timeout over a new "timeout" reason because any timeout
// in practice is SSH (local repos don't hang).
func TestClassifyBorgError_ContextDeadlineExceededMapsToSSHTimeout(t *testing.T) {
	got := classifyBorgError("borg", context.DeadlineExceeded, "")
	var bre *BorgRunError
	if !errors.As(got, &bre) {
		t.Fatalf("classifyBorgError not *BorgRunError: %v", got)
	}
	if bre.Reason != BorgErrSSHTimeout {
		t.Errorf("Reason = %q; want ssh_timeout", bre.Reason)
	}
}

// TestCanonicalRepoPath_LocalPathsAreCleanedAndAbsed covers the
// dedupe-path input normalization.
func TestCanonicalRepoPath_LocalPathsAreCleanedAndAbsed(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/a/b/..//c", "/a/c"},
		{"/mnt/foo/./bar", "/mnt/foo/bar"},
	}
	for _, tc := range cases {
		if got := CanonicalRepoPath(tc.in); got != tc.want {
			t.Errorf("CanonicalRepoPath(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestCanonicalRepoPath_SSHRepoPreserved pins that ssh:// URLs are
// passed through verbatim — filepath.Clean would corrupt them.
func TestCanonicalRepoPath_SSHRepoPreserved(t *testing.T) {
	in := "ssh://user@host:22//var/backups/borg"
	if got := CanonicalRepoPath(in); got != in {
		t.Errorf("CanonicalRepoPath(%q) = %q; want passthrough", in, got)
	}
}

// TestFakeBorgRunner_UnknownRepoReturnsErrReason pins that the
// fake fails closed — tests that accidentally forget to seed a
// scenario get a deterministic error reason.
func TestFakeBorgRunner_UnknownRepoReturnsErrReason(t *testing.T) {
	r := newFakeBorgRunner()
	_, err := r.Info(context.Background(), "/not-configured", "borg", nil)
	var bre *BorgRunError
	if !errors.As(err, &bre) {
		t.Fatalf("error not *BorgRunError: %v", err)
	}
	if bre.Reason != BorgErrRepoInaccessible {
		t.Errorf("Reason = %q; want repo_inaccessible", bre.Reason)
	}
	if !strings.Contains(bre.Err.Error(), "no scenario") {
		t.Error("underlying err should describe missing fake scenario")
	}
}

// TestParseBorgInfoLast_ModernShape_FixesLatentBug guards the
// scope-addendum fix on issue #279: modern Borg 1.4+ dropped the
// archives array from `borg info --json <repo>`. The fix is to call
// `borg info --last 1 --json` separately; this test asserts we
// parse that payload correctly (independent from the list-count
// check covered above).
func TestParseBorgInfoLast_ModernShape_FixesLatentBug(t *testing.T) {
	_, latest, err := parseBorgInfoLast(realisticInfoLast1JSON)
	if err != nil {
		t.Fatalf("parseBorgInfoLast err = %v", err)
	}
	if latest == nil {
		t.Fatal("latest nil; modern-Borg path must still produce an archive")
	}
	// The legacy auto-detect path (queryBorgRepo pre-#279) silently
	// produced zero-valued stats on modern Borg because the archives
	// array was missing. New path must populate these:
	if latest.NFiles == 0 {
		t.Error("NFiles = 0; latent-bug regression guard — modern-Borg info --last 1 must populate nfiles")
	}
	if latest.OriginalSize == 0 {
		t.Error("OriginalSize = 0; latent-bug regression guard")
	}
}

// The empty-repo payload has archives = [] — callers that branch on
// count must not crash on a nil LatestArchive.
func TestParseBorgInfoLast_EmptyArchivesReturnsNilLatest(t *testing.T) {
	_, latest, err := parseBorgInfoLast(emptyRepoListJSON)
	if err != nil {
		t.Fatalf("parseBorgInfoLast err = %v", err)
	}
	if latest != nil {
		t.Errorf("latest = %+v; want nil for empty archives array", latest)
	}
}

// ---------- rc2: --bypass-lock argv + unconditional env vars (#279) ----------

// TestBorgArgs_ListInvocationIncludesBypassLock pins that the pure
// argv builder puts --bypass-lock on the `borg list --json` call.
// Without this, even read-only operations fail on a Read-Only-mounted
// repo because borg acquires a lock file in the repo dir — see issue
// #279 rc1 UAT finding 1.
func TestBorgArgs_ListInvocationIncludesBypassLock(t *testing.T) {
	args := buildBorgListArgs("/mnt/backups/offsite")
	if !containsArg(args, "list") {
		t.Fatalf("buildBorgListArgs = %v; want list subcommand", args)
	}
	if !containsArg(args, "--bypass-lock") {
		t.Errorf("buildBorgListArgs = %v; want --bypass-lock flag", args)
	}
	if !containsArg(args, "--json") {
		t.Errorf("buildBorgListArgs = %v; want --json flag", args)
	}
	if !containsArg(args, "/mnt/backups/offsite") {
		t.Errorf("buildBorgListArgs = %v; want repo path as last arg", args)
	}
}

// TestBorgArgs_InfoLast1InvocationIncludesBypassLock pins that
// --bypass-lock is also present on the `borg info --last 1 --json`
// call. Same rationale as the list call — lock acquisition fails
// on RO mounts.
func TestBorgArgs_InfoLast1InvocationIncludesBypassLock(t *testing.T) {
	args := buildBorgInfoLastArgs("/mnt/backups/offsite")
	if !containsArg(args, "info") {
		t.Fatalf("buildBorgInfoLastArgs = %v; want info subcommand", args)
	}
	if !containsArg(args, "--bypass-lock") {
		t.Errorf("buildBorgInfoLastArgs = %v; want --bypass-lock flag", args)
	}
	if !containsArg(args, "--last") {
		t.Errorf("buildBorgInfoLastArgs = %v; want --last flag", args)
	}
	if !containsArg(args, "--json") {
		t.Errorf("buildBorgInfoLastArgs = %v; want --json flag", args)
	}
}

// TestBorgArgs_InfoMetadataInvocationIncludesBypassLock pins that the
// empty-repo fallback `borg info --json` call also sets
// --bypass-lock. This is the path used when `list` returns zero
// archives and we still need repo metadata for the dashboard.
func TestBorgArgs_InfoMetadataInvocationIncludesBypassLock(t *testing.T) {
	args := buildBorgInfoMetadataArgs("/mnt/backups/offsite")
	if !containsArg(args, "info") {
		t.Fatalf("buildBorgInfoMetadataArgs = %v; want info subcommand", args)
	}
	if !containsArg(args, "--bypass-lock") {
		t.Errorf("buildBorgInfoMetadataArgs = %v; want --bypass-lock flag", args)
	}
	if !containsArg(args, "--json") {
		t.Errorf("buildBorgInfoMetadataArgs = %v; want --json flag", args)
	}
}

// TestBuildRunnerEnv_AlwaysIncludesNonInteractiveOverrides pins the
// rc2 Finding-2 guarantee: the two env vars that suppress borg's
// interactive prompts on unknown-unencrypted + relocated repos are
// set unconditionally by the runner, even when the caller passes a
// nil env map (the auto-detect path).
func TestBuildRunnerEnv_AlwaysIncludesNonInteractiveOverrides(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
	}{
		{"nil env", nil},
		{"empty env", map[string]string{}},
		{"caller-supplied env is preserved", map[string]string{"BORG_PASSPHRASE": "sekret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRunnerEnv(tc.in)
			assertEnvPairPresent(t, got, "BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK=yes")
			assertEnvPairPresent(t, got, "BORG_RELOCATED_REPO_ACCESS_IS_OK=yes")
			if tc.in["BORG_PASSPHRASE"] == "sekret" {
				assertEnvPairPresent(t, got, "BORG_PASSPHRASE=sekret")
			}
		})
	}
}

// TestBuildRunnerEnv_CallerSuppliedValueWinsOverDefault confirms that
// if the caller somehow sets BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK
// themselves (unusual, but legal), the caller's value wins. This is
// defensive: a hypothetical future caller-side override should not be
// silently clobbered by the runner defaults.
func TestBuildRunnerEnv_CallerSuppliedValueWinsOverDefault(t *testing.T) {
	got := buildRunnerEnv(map[string]string{
		"BORG_RELOCATED_REPO_ACCESS_IS_OK": "no",
	})
	assertEnvPairPresent(t, got, "BORG_RELOCATED_REPO_ACCESS_IS_OK=no")
	// Other default still kicks in when not overridden.
	assertEnvPairPresent(t, got, "BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK=yes")
}

// TestClassifyBorgError_ReadOnlyFilesystemMapsToRepoReadonly pins the
// Finding-2 defense-in-depth classifier. With --bypass-lock in place
// this path should never fire, but we guarantee a specific reason
// category for the edge case so users see a targeted message.
func TestClassifyBorgError_ReadOnlyFilesystemMapsToRepoReadonly(t *testing.T) {
	stderr := "Failed to create/acquire the lock /mnt/borg/lock.exclusive ([Errno 30] Read-only file system: '/mnt/borg/lock.exclusive.tmp')."
	got := classifyBorgError("borg", errors.New("exit 2"), stderr)
	var bre *BorgRunError
	if !errors.As(got, &bre) {
		t.Fatalf("classifyBorgError not *BorgRunError: %v", got)
	}
	if bre.Reason != BorgErrRepoReadOnly {
		t.Errorf("Reason = %q; want %q", bre.Reason, BorgErrRepoReadOnly)
	}
}

// ---------- rc3: stdout/stderr separation (#279) ----------

// TestExecBorgRunner_Run_SeparatesStdoutFromStderr guards the rc3
// Finding A fix: when borg writes an auto-accepted
// "unknown unencrypted repo" warning to stderr, the runner must not
// interleave it with the JSON on stdout. Otherwise json.Unmarshal
// fails on the warning prefix and the runner returns
// `parse list json: ...` as BorgErrUnknown on every unencrypted
// probe. rc2 shipped with cmd.CombinedOutput(); rc3 uses cmd.Output()
// + an explicit bytes.Buffer for stderr.
//
// The test drives /bin/sh directly to emit realistic stdout + stderr
// without needing a real borg binary.
func TestExecBorgRunner_Run_SeparatesStdoutFromStderr(t *testing.T) {
	r := &execBorgRunner{}
	script := `printf 'Warning: Attempting to access a previously unknown unencrypted repository!\n' 1>&2
printf 'Do you want to continue? [yN] yes (from BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK)\n' 1>&2
printf '%s' '{"archives":[]}'`
	out, err := r.run(context.Background(), "/bin/sh", []string{"-c", script}, nil)
	if err != nil {
		t.Fatalf("run err = %v; want nil (exit 0)", err)
	}
	// stdout only — stderr warning MUST NOT appear in out.
	if strings.Contains(out, "Warning:") {
		t.Errorf("run stdout contains stderr warning text; want stdout-only separation. Got: %q", out)
	}
	if strings.Contains(out, "unknown unencrypted") {
		t.Errorf("run stdout contaminated by stderr: %q", out)
	}
	if out != `{"archives":[]}` {
		t.Errorf("run out = %q; want clean JSON", out)
	}
}

// TestExecBorgRunner_Run_StderrAvailableOnNonZeroExit ensures that
// when the subprocess exits non-zero, the returned string carries the
// stderr content (so classifyBorgError can still match on phrases
// like "passphrase supplied"). With cmd.Output() + stderr buffer,
// this is the error path: stdout is returned from Output() (may be
// partial/empty), but we return stderr.String() so the caller's
// classifier sees what it expects.
func TestExecBorgRunner_Run_StderrAvailableOnNonZeroExit(t *testing.T) {
	r := &execBorgRunner{}
	script := `printf 'passphrase supplied in BORG_PASSPHRASE is incorrect\n' 1>&2
exit 2`
	out, err := r.run(context.Background(), "/bin/sh", []string{"-c", script}, nil)
	if err == nil {
		t.Fatal("run err = nil; want non-nil on exit 2")
	}
	if !strings.Contains(out, "passphrase supplied") {
		t.Errorf("run out (stderr on error) = %q; want to contain stderr text for classifier", out)
	}
}

// containsArg is true when needle appears anywhere in args.
func containsArg(args []string, needle string) bool {
	for _, a := range args {
		if a == needle {
			return true
		}
	}
	return false
}

// assertEnvPairPresent fails the test if needle is not exactly one of
// the KEY=VALUE entries in env.
func assertEnvPairPresent(t *testing.T, env []string, needle string) {
	t.Helper()
	for _, e := range env {
		if e == needle {
			return
		}
	}
	t.Errorf("env missing %q; full env = %v", needle, env)
}
