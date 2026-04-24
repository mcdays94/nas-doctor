package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// noopEnv is a readEnv stub that returns "" for every name.
func noopEnv(string) string { return "" }

// envFromMap returns a readEnv closure backed by the given map.
func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TestCollectBorgExternal_HealthyRepoPopulatesBackupJob locks the
// happy-path composition: a healthy external entry produces a
// BackupJob with Configured=true, SnapshotCount from list, and
// LatestArchive stats from info --last 1.
func TestCollectBorgExternal_HealthyRepoPopulatesBackupJob(t *testing.T) {
	r := newFakeBorgRunner()
	now := time.Now().Add(-1 * time.Hour)
	r.set("/mnt/backups/offsite", fakeBorgScenario{
		Info: BorgInfoJSON{
			ArchiveCount:   5,
			RepoLocation:   "/mnt/backups/offsite",
			EncryptionMode: "repokey-blake2",
			LatestArchive: &BorgArchive{
				Name:         "2026-04-24-daily",
				Start:        now,
				End:          now.Add(2 * time.Minute),
				NFiles:       1234,
				OriginalSize: 9_000_000_000,
			},
		},
	})
	cfg := []BorgExternalRepo{{
		Enabled:  true,
		Label:    "Offsite Backup",
		RepoPath: "/mnt/backups/offsite",
	}}
	jobs := collectBorgExternal(r, cfg, noopEnv)
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs; want 1", len(jobs))
	}
	j := jobs[0]
	if !j.Configured {
		t.Error("Configured = false; explicit config must flip this true")
	}
	if j.Label != "Offsite Backup" {
		t.Errorf("Label = %q; want Offsite Backup", j.Label)
	}
	if j.SnapshotCount != 5 {
		t.Errorf("SnapshotCount = %d; want 5", j.SnapshotCount)
	}
	if !j.Encrypted {
		t.Error("Encrypted = false; repokey-blake2 must count as encrypted")
	}
	if j.SizeBytes != 9_000_000_000 {
		t.Errorf("SizeBytes = %d; want 9000000000", j.SizeBytes)
	}
	if j.FilesCount != 1234 {
		t.Errorf("FilesCount = %d; want 1234", j.FilesCount)
	}
	if j.Status != "ok" {
		t.Errorf("Status = %q; want ok (1h-old archive)", j.Status)
	}
	if j.Error != "" {
		t.Errorf("Error = %q; want empty on healthy repo", j.Error)
	}
}

// TestCollectBorgExternal_DisabledRepoSkipped pins the Enabled=false
// short-circuit. Disabled entries should produce no BackupJob — the
// user can re-enable without losing the config.
func TestCollectBorgExternal_DisabledRepoSkipped(t *testing.T) {
	r := newFakeBorgRunner()
	r.set("/mnt/disabled", fakeBorgScenario{Info: BorgInfoJSON{ArchiveCount: 1}})
	cfg := []BorgExternalRepo{{Enabled: false, RepoPath: "/mnt/disabled"}}
	jobs := collectBorgExternal(r, cfg, noopEnv)
	if len(jobs) != 0 {
		t.Fatalf("disabled repo produced %d jobs; want 0", len(jobs))
	}
}

// TestCollectBorgExternal_ErrorProducesErrorCard pins that a runner
// failure surfaces as a BackupJob with Error + ErrorReason populated
// rather than being silently dropped.
func TestCollectBorgExternal_ErrorProducesErrorCard(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		reason string
	}{
		{"passphrase_rejected", &BorgRunError{Reason: BorgErrPassphraseRejected, Err: errors.New("exit 2")}, BorgErrPassphraseRejected},
		{"binary_not_found", &BorgRunError{Reason: BorgErrBinaryNotFound, Err: errors.New("exec")}, BorgErrBinaryNotFound},
		{"ssh_timeout", &BorgRunError{Reason: BorgErrSSHTimeout, Err: errors.New("deadline")}, BorgErrSSHTimeout},
		{"corrupt_repo", &BorgRunError{Reason: BorgErrCorruptRepo, Err: errors.New("IntegrityError")}, BorgErrCorruptRepo},
		{"repo_inaccessible", &BorgRunError{Reason: BorgErrRepoInaccessible, Err: errors.New("ENOENT")}, BorgErrRepoInaccessible},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newFakeBorgRunner()
			r.set("/mnt/bad", fakeBorgScenario{Err: tc.err})
			jobs := collectBorgExternal(r, []BorgExternalRepo{{Enabled: true, RepoPath: "/mnt/bad"}}, noopEnv)
			if len(jobs) != 1 {
				t.Fatalf("got %d jobs; want 1", len(jobs))
			}
			if jobs[0].Error == "" {
				t.Error("Error empty; want non-empty for failed probe")
			}
			if jobs[0].ErrorReason != tc.reason {
				t.Errorf("ErrorReason = %q; want %q", jobs[0].ErrorReason, tc.reason)
			}
			if !jobs[0].Configured {
				t.Error("Configured false; even error-state jobs from explicit config should flip it true")
			}
			if jobs[0].Status != "failed" {
				t.Errorf("Status = %q; want failed", jobs[0].Status)
			}
		})
	}
}

// TestCollectBorgExternal_MultipleRepos processes each entry
// independently: one healthy + one failing must both appear in the
// result set.
func TestCollectBorgExternal_MultipleRepos(t *testing.T) {
	r := newFakeBorgRunner()
	r.set("/mnt/good", fakeBorgScenario{Info: BorgInfoJSON{ArchiveCount: 2, LatestArchive: &BorgArchive{Start: time.Now()}}})
	r.set("/mnt/bad", fakeBorgScenario{Err: &BorgRunError{Reason: BorgErrSSHTimeout, Err: errors.New("deadline")}})
	cfg := []BorgExternalRepo{
		{Enabled: true, RepoPath: "/mnt/good"},
		{Enabled: true, RepoPath: "/mnt/bad"},
	}
	jobs := collectBorgExternal(r, cfg, noopEnv)
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs; want 2", len(jobs))
	}
	// Order matches config order for UI predictability.
	if jobs[0].Repository != "/mnt/good" {
		t.Errorf("jobs[0].Repository = %q; want /mnt/good", jobs[0].Repository)
	}
	if jobs[1].ErrorReason != BorgErrSSHTimeout {
		t.Errorf("jobs[1].ErrorReason = %q; want ssh_timeout", jobs[1].ErrorReason)
	}
}

// TestBuildBorgEnv_DefaultPassphraseEnvVarName pins that an empty
// PassphraseEnv defaults to BORG_PASSPHRASE when looking up the
// process env.
func TestBuildBorgEnv_DefaultPassphraseEnvVarName(t *testing.T) {
	read := envFromMap(map[string]string{"BORG_PASSPHRASE": "sekret"})
	env := buildBorgEnv(BorgExternalRepo{Enabled: true, RepoPath: "/a"}, read)
	if env["BORG_PASSPHRASE"] != "sekret" {
		t.Errorf("BORG_PASSPHRASE = %q; want sekret (from default lookup)", env["BORG_PASSPHRASE"])
	}
}

// TestBuildBorgEnv_CustomPassphraseEnvVarResolved covers the
// per-repo secret separation story from PRD #278: user points repo A
// at BORG_PASSPHRASE_MAIN and repo B at BORG_PASSPHRASE_OFFSITE, and
// NAS Doctor pipes each under the canonical BORG_PASSPHRASE name.
func TestBuildBorgEnv_CustomPassphraseEnvVarResolved(t *testing.T) {
	read := envFromMap(map[string]string{
		"BORG_PASSPHRASE_OFFSITE": "offsite-secret",
		"BORG_PASSPHRASE":         "wrong-default",
	})
	env := buildBorgEnv(BorgExternalRepo{PassphraseEnv: "BORG_PASSPHRASE_OFFSITE", RepoPath: "/a"}, read)
	if env["BORG_PASSPHRASE"] != "offsite-secret" {
		t.Errorf("BORG_PASSPHRASE = %q; want offsite-secret (resolved via custom env var name)", env["BORG_PASSPHRASE"])
	}
}

// TestBuildBorgEnv_DefaultPassphraseEnv_UnsetDoesNotSetBorgPassphrase
// pins the rc3 Finding B behavior for the DEFAULT-lookup path: when
// the user left PassphraseEnv blank (meaning "use whatever the
// process already has"), and BORG_PASSPHRASE is not in the readEnv
// lookup, buildBorgEnv must NOT set env["BORG_PASSPHRASE"]. The
// subprocess then inherits the container's BORG_PASSPHRASE via
// os.Environ() — which is the backwards-compat path from rc2.
func TestBuildBorgEnv_DefaultPassphraseEnv_UnsetDoesNotSetBorgPassphrase(t *testing.T) {
	env := buildBorgEnv(BorgExternalRepo{Enabled: true, RepoPath: "/a"}, noopEnv)
	if _, ok := env["BORG_PASSPHRASE"]; ok {
		t.Errorf("BORG_PASSPHRASE set to %q; default-lookup path must leave it unset so subprocess inherits process env",
			env["BORG_PASSPHRASE"])
	}
}

// TestBuildBorgEnv_ExplicitPassphraseEnv_MissingVarOverridesWithEmpty
// pins the rc3 Finding B fix for the EXPLICIT-override path: when
// the user specified PassphraseEnv="BORG_WRONG" and that var is
// unset, buildBorgEnv MUST set env["BORG_PASSPHRASE"]="" explicitly
// so the subprocess does NOT silently inherit the container's
// default BORG_PASSPHRASE. User's explicit override wins even when
// the lookup returns empty — they'll get a passphrase_rejected
// error from borg, which is the correct, honest outcome.
func TestBuildBorgEnv_ExplicitPassphraseEnv_MissingVarOverridesWithEmpty(t *testing.T) {
	// readEnv returns "" for BORG_DOES_NOT_EXIST but a real value
	// for BORG_PASSPHRASE (simulating the container default). The
	// explicit override must NOT fall back to the default.
	read := envFromMap(map[string]string{"BORG_PASSPHRASE": "container-default-should-not-leak"})
	env := buildBorgEnv(BorgExternalRepo{PassphraseEnv: "BORG_DOES_NOT_EXIST", RepoPath: "/a"}, read)
	v, ok := env["BORG_PASSPHRASE"]
	if !ok {
		t.Fatal("BORG_PASSPHRASE not set; explicit override must set the key so subprocess doesn't inherit default")
	}
	if v != "" {
		t.Errorf("BORG_PASSPHRASE = %q; want empty string (explicit override wins, even when lookup is empty)", v)
	}
}

// TestBuildBorgEnv_ExplicitPassphraseEnv_PresentVarResolved pins the
// happy path for explicit overrides: the named env var resolves to a
// real value and is piped under the canonical BORG_PASSPHRASE key.
// This overlaps with TestBuildBorgEnv_CustomPassphraseEnvVarResolved
// (which predates rc3); kept separate so the rc3 symmetry with the
// missing-var case is readable.
func TestBuildBorgEnv_ExplicitPassphraseEnv_PresentVarResolved(t *testing.T) {
	read := envFromMap(map[string]string{"BORG_X": "custom-val"})
	env := buildBorgEnv(BorgExternalRepo{PassphraseEnv: "BORG_X", RepoPath: "/a"}, read)
	if env["BORG_PASSPHRASE"] != "custom-val" {
		t.Errorf("BORG_PASSPHRASE = %q; want custom-val from explicit BORG_X lookup", env["BORG_PASSPHRASE"])
	}
}

// TestBuildBorgEnv_SSHKeyPathWiresBorgRSH pins that setting
// SSHKeyPath produces a BORG_RSH env var — upstream Borg reads
// BORG_RSH to decide the ssh invocation.
func TestBuildBorgEnv_SSHKeyPathWiresBorgRSH(t *testing.T) {
	env := buildBorgEnv(BorgExternalRepo{SSHKeyPath: "/mnt/keys/borg_id_ed25519"}, noopEnv)
	rsh, ok := env["BORG_RSH"]
	if !ok {
		t.Fatal("BORG_RSH not set when SSHKeyPath configured")
	}
	if rsh == "" || rsh == "ssh" {
		t.Errorf("BORG_RSH = %q; want ssh invocation with -i key path", rsh)
	}
}

// TestMergeBorgJobs_DedupesByCanonicalPath pins the merge behaviour:
// auto-detect and explicit-config entries that point at the same
// canonical path produce ONE entry, with auto-detect's metadata and
// Configured=true.
func TestMergeBorgJobs_DedupesByCanonicalPath(t *testing.T) {
	auto := []internal.BackupJob{
		{Provider: "borg", Name: "offsite", Repository: "/mnt/backups/offsite", SnapshotCount: 5},
	}
	external := []internal.BackupJob{
		// Same repo via an un-cleaned path — Clean+Abs normalizes it.
		{Provider: "borg", Name: "offsite-dup", Repository: "/mnt/backups/../backups/offsite", Configured: true, Label: "Offsite Backup"},
	}
	merged := mergeBorgJobs(auto, external)
	if len(merged) != 1 {
		t.Fatalf("got %d merged jobs; want 1 (dedupe)", len(merged))
	}
	if !merged[0].Configured {
		t.Error("merged Configured = false; want true after dedupe against explicit config")
	}
	if merged[0].Label != "Offsite Backup" {
		t.Errorf("merged Label = %q; want Offsite Backup (from external config)", merged[0].Label)
	}
	if merged[0].SnapshotCount != 5 {
		t.Errorf("merged SnapshotCount = %d; want 5 (auto-detect metadata preserved)", merged[0].SnapshotCount)
	}
}

// TestMergeBorgJobs_DistinctPathsYieldBothEntries sanity check — two
// different repos produce two entries.
func TestMergeBorgJobs_DistinctPathsYieldBothEntries(t *testing.T) {
	auto := []internal.BackupJob{{Repository: "/a/borg"}}
	external := []internal.BackupJob{{Repository: "/b/borg", Configured: true}}
	merged := mergeBorgJobs(auto, external)
	if len(merged) != 2 {
		t.Fatalf("got %d; want 2", len(merged))
	}
}

// TestMergeBorgJobs_EmptyExternalPassthrough is the no-op path
// preserving pre-#279 semantics.
func TestMergeBorgJobs_EmptyExternalPassthrough(t *testing.T) {
	auto := []internal.BackupJob{{Repository: "/a/borg", SnapshotCount: 3}}
	merged := mergeBorgJobs(auto, nil)
	if len(merged) != 1 || merged[0].SnapshotCount != 3 {
		t.Fatalf("passthrough modified auto jobs: %+v", merged)
	}
	if merged[0].Configured {
		t.Error("Configured flipped to true without external config — must stay false")
	}
}

// TestCollectBackups_WithExternalOnly composes the public surface:
// CollectBackups called with ExternalBorg entries and no auto-
// detection produces the expected job list. Verifies the options
// struct plumbing end-to-end.
func TestCollectBackups_WithExternalOnly(t *testing.T) {
	r := newFakeBorgRunner()
	r.set("/mnt/main", fakeBorgScenario{Info: BorgInfoJSON{ArchiveCount: 7, LatestArchive: &BorgArchive{Start: time.Now()}}})
	info := CollectBackups(CollectBackupsOptions{
		Runner: r,
		ExternalBorg: []BorgExternalRepo{
			{Enabled: true, Label: "Main", RepoPath: "/mnt/main"},
		},
		ReadEnv: noopEnv,
	})
	if info == nil || !info.Available {
		t.Fatal("Available = false; want true with external config populated")
	}
	var found bool
	for _, j := range info.Jobs {
		if j.Repository == "/mnt/main" {
			found = true
			if !j.Configured {
				t.Error("Configured = false; want true from external config")
			}
			if j.SnapshotCount != 7 {
				t.Errorf("SnapshotCount = %d; want 7", j.SnapshotCount)
			}
		}
	}
	if !found {
		t.Errorf("no job for /mnt/main in %+v", info.Jobs)
	}
}

// TestQueryBorgRepoViaRunner_ModernBorgLatentBugFix is the direct
// regression guard for the scope-addendum on issue #279: the
// pre-#279 auto-detect path called `borg info --json <repo>` only,
// which on modern Borg 1.4+ returns no archives array — silently
// producing a BackupJob with SnapshotCount=0 and all archive-level
// fields zero. The refactored path reads archive count from
// `borg list --json` and archive stats from `borg info --last 1
// --json`, so a modern-Borg healthy repo produces a COMPLETE
// BackupJob.
func TestQueryBorgRepoViaRunner_ModernBorgLatentBugFix(t *testing.T) {
	r := newFakeBorgRunner()
	r.set("/mnt/modern", fakeBorgScenario{
		Info: BorgInfoJSON{
			ArchiveCount: 3, // from `borg list --json`
			LatestArchive: &BorgArchive{
				Name:         "daily-latest",
				Start:        time.Now().Add(-2 * time.Hour),
				End:          time.Now().Add(-2*time.Hour + 5*time.Minute),
				NFiles:       42,
				OriginalSize: 1_000_000,
			},
			EncryptionMode: "none",
		},
	})
	job := queryBorgRepoViaRunner(r, "/mnt/modern", "", nil)
	if job == nil {
		t.Fatal("nil job")
	}
	// On the pre-#279 path ALL of these would be zero because
	// `borg info --json <repo>` doesn't emit archives. Assert each
	// separately so a regression names the field that slipped.
	if job.SnapshotCount != 3 {
		t.Errorf("SnapshotCount = %d; want 3 (latent-bug guard)", job.SnapshotCount)
	}
	if job.FilesCount != 42 {
		t.Errorf("FilesCount = %d; want 42 (latent-bug guard)", job.FilesCount)
	}
	if job.SizeBytes != 1_000_000 {
		t.Errorf("SizeBytes = %d; want 1000000 (latent-bug guard)", job.SizeBytes)
	}
	if job.LastSuccess.IsZero() {
		t.Error("LastSuccess zero; want populated from archive Start (latent-bug guard)")
	}
	if job.Duration == 0 {
		t.Error("Duration = 0; want populated from archive End-Start")
	}
}

// TestQueryBorgRepoViaRunner_HonoursBinaryPathArg pins that a
// non-empty BinaryPath is forwarded to the runner, letting users
// override the bundled borg with a mounted alt build.
func TestQueryBorgRepoViaRunner_HonoursBinaryPathArg(t *testing.T) {
	var gotBinary string
	r := &binaryCapturingRunner{out: BorgInfoJSON{ArchiveCount: 1, LatestArchive: &BorgArchive{Start: time.Now()}}, captured: &gotBinary}
	_ = queryBorgRepoViaRunner(r, "/mnt/x", "/custom/borg", nil)
	if gotBinary != "/custom/borg" {
		t.Errorf("runner saw binary %q; want /custom/borg", gotBinary)
	}
}

// binaryCapturingRunner is a narrower fake for the
// HonoursBinaryPathArg test above — standard fakeBorgRunner doesn't
// record the binary arg.
type binaryCapturingRunner struct {
	out      BorgInfoJSON
	captured *string
}

func (b *binaryCapturingRunner) Info(ctx context.Context, repoPath, binaryPath string, env map[string]string) (BorgInfoJSON, error) {
	*b.captured = binaryPath
	return b.out, nil
}
