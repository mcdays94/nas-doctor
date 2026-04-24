package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// BorgExternalRepo is one explicitly user-configured external Borg repo.
// Matches the api.BorgExternalRepo shape but lives in the collector
// package so callers below api don't need to import the api layer.
// Issue #279.
type BorgExternalRepo struct {
	// Enabled gates the repo from polling — users can disable without
	// deleting to keep a repo's config around.
	Enabled bool
	// Label is a user display name; optional. Falls back to the
	// canonical repo path basename in the dashboard widget.
	Label string
	// RepoPath is the container-visible path to the repo (bind-mount
	// from host) or an ssh:// URL. Required when Enabled.
	RepoPath string
	// BinaryPath overrides the default `borg` PATH lookup. Empty =
	// bundled Alpine musl borg at /usr/bin/borg. Override must be
	// musl-compatible; see #279 architecture correction.
	BinaryPath string
	// PassphraseEnv names the env var NAS Doctor reads to discover the
	// repo's passphrase. Empty = default "BORG_PASSPHRASE". NAS Doctor
	// does NOT store the passphrase itself; users set it via Docker
	// env vars at container-start time.
	PassphraseEnv string
	// SSHKeyPath is an optional container-visible path to an SSH key
	// file used for ssh:// repos. NAS Doctor sets BORG_RSH to a `ssh
	// -i <path>` invocation when this is set.
	SSHKeyPath string
}

// CollectBackupsOptions carries non-defaults for the backup-collection
// tick. Passed to CollectBackups by the scheduler / server; the zero
// value represents "auto-detect only, no external repos" which
// preserves the pre-#279 behaviour exactly.
type CollectBackupsOptions struct {
	// Runner is the BorgRunner to use for any Borg probe (auto-detect
	// or external). Nil → uses NewExecBorgRunner().
	Runner BorgRunner
	// ExternalBorg is the user-configured list of external Borg repos.
	// Entries with Enabled=false are skipped; entries whose canonical
	// path collides with an auto-detected entry are deduped (auto-
	// detect kept as authoritative, but marked Configured).
	ExternalBorg []BorgExternalRepo
	// ReadEnv resolves a named env var — injected so tests don't have
	// to touch os.Getenv. Defaults to os.Getenv when nil.
	ReadEnv func(string) string
}

// CollectBackups detects and queries backup tools: Borg, Restic, PBS,
// Duplicati, Rclone. opts may be zero — all fields have sensible
// defaults that preserve pre-#279 behaviour. Public so the api /
// scheduler layers can drive external-repo polling; the lowercase
// variant remains for the internal Collect() path.
func CollectBackups(opts CollectBackupsOptions) *internal.BackupInfo {
	info := &internal.BackupInfo{Available: false}
	runner := opts.Runner
	if runner == nil {
		runner = NewExecBorgRunner()
	}
	readEnv := opts.ReadEnv
	if readEnv == nil {
		readEnv = os.Getenv
	}

	borgJobs := collectBorg(runner, readEnv)
	externalJobs := collectBorgExternal(runner, opts.ExternalBorg, readEnv)
	borgJobs = mergeBorgJobs(borgJobs, externalJobs)
	resticJobs := collectRestic()
	pbsJobs := collectPBS()
	duplicatiJobs := collectDuplicati()

	info.Jobs = append(info.Jobs, borgJobs...)
	info.Jobs = append(info.Jobs, resticJobs...)
	info.Jobs = append(info.Jobs, pbsJobs...)
	info.Jobs = append(info.Jobs, duplicatiJobs...)

	if len(info.Jobs) > 0 {
		info.Available = true
	}
	return info
}

// collectBackups is the zero-options shim used by the monolithic
// Collect() flow. Behaviourally unchanged from pre-#279.
func collectBackups() *internal.BackupInfo {
	return CollectBackups(CollectBackupsOptions{})
}

// logBackupResults emits structured per-repo lines for the backup
// subsystem. One INFO per healthy Configured=true repo and one ERROR
// per failed repo, with the specific reason category. Auto-detect
// entries are intentionally silent (noise reduction). Called from
// Collect() after each backup tick (issue #279 user story 15).
//
// externalConfigured indicates the user has at least one entry in
// settings; used to decide whether to emit a summary even when the
// result set is empty (0-of-N success).
func logBackupResults(logger *slog.Logger, info *internal.BackupInfo, externalConfigured bool) {
	if logger == nil || info == nil {
		return
	}
	if !externalConfigured {
		return
	}
	for _, j := range info.Jobs {
		if !j.Configured {
			continue
		}
		if j.Error != "" {
			logger.Error("borg external: repo probe failed",
				"repo", j.Repository,
				"label", j.Label,
				"reason", j.ErrorReason)
			continue
		}
		logger.Info("borg external: repo probed",
			"repo", j.Repository,
			"label", j.Label,
			"archives", j.SnapshotCount)
	}
}

// ---------- Borg ----------

// collectBorg auto-detects Borg repos by probing $BORG_REPO + a fixed
// set of common NAS-layout locations. Each detected repo is queried
// via the injected BorgRunner. Refactored from the pre-#279 direct
// exec path so the modern-Borg two-call fix (issue #279 scope
// addendum) applies uniformly.
func collectBorg(runner BorgRunner, readEnv func(string) string) []internal.BackupJob {
	// Fast-path exit: if borg isn't on PATH we have nothing to probe.
	// This preserves the pre-#279 behaviour (no noisy logs on
	// borg-less installs) and keeps the per-repo loop tight.
	if _, err := exec.LookPath("borg"); err != nil {
		return nil
	}

	repos := findBorgRepos(readEnv)
	var jobs []internal.BackupJob
	for _, repo := range repos {
		job := queryBorgRepoViaRunner(runner, repo, "", nil)
		if job != nil && job.Error == "" {
			// Auto-detect path intentionally drops error cards — a
			// speculative "common location" probe that fails is just
			// noise. Explicit configs (collectBorgExternal) render
			// errors.
			jobs = append(jobs, *job)
		}
	}
	return jobs
}

// collectBorgExternal polls the user-configured external Borg repos.
// Each enabled entry is queried via the injected BorgRunner; failures
// produce a BackupJob with Error + ErrorReason populated so the
// dashboard widget can render an error card with a specific reason.
func collectBorgExternal(runner BorgRunner, cfg []BorgExternalRepo, readEnv func(string) string) []internal.BackupJob {
	if len(cfg) == 0 {
		return nil
	}
	var out []internal.BackupJob
	for _, repo := range cfg {
		if !repo.Enabled {
			continue
		}
		repoPath := strings.TrimSpace(repo.RepoPath)
		if repoPath == "" {
			continue
		}
		binary := strings.TrimSpace(repo.BinaryPath)
		env := buildBorgEnv(repo, readEnv)
		job := queryBorgRepoViaRunner(runner, repoPath, binary, env)
		if job == nil {
			// Runner may return nil for a totally un-recoverable
			// state; synthesise an unknown-error card so the user
			// still sees something on the dashboard.
			job = &internal.BackupJob{
				Provider:    "borg",
				Name:        filepath.Base(repoPath),
				Repository:  repoPath,
				Status:      "failed",
				Error:       "backup probe returned nil",
				ErrorReason: BorgErrUnknown,
			}
		}
		job.Configured = true
		if repo.Label != "" {
			job.Label = repo.Label
			if job.Name == "" || job.Name == filepath.Base(repoPath) {
				job.Name = repo.Label
			}
		}
		out = append(out, *job)
	}
	return out
}

// BuildBorgEnvForTest exposes buildBorgEnv to the api package's Test
// endpoint so the same env-resolution logic drives both scheduled
// polling and the user-invoked Test button. Signature is the same as
// buildBorgEnv; the exported name is the only addition. Issue #279.
func BuildBorgEnvForTest(r BorgExternalRepo, readEnv func(string) string) map[string]string {
	return buildBorgEnv(r, readEnv)
}

// buildBorgEnv composes the env map passed to BorgRunner.Info for one
// configured repo. The passphrase env var name defaults to
// BORG_PASSPHRASE; a non-empty override is resolved from the NAS
// Doctor process env and forwarded under the canonical
// BORG_PASSPHRASE name the borg binary expects. If SSH key path is
// set, BORG_RSH is wired up so remote repos auth without a system-
// wide key.
//
// Returns nil when no env vars are needed — lets the runner skip the
// ambient-merge branch.
func buildBorgEnv(r BorgExternalRepo, readEnv func(string) string) map[string]string {
	env := map[string]string{}
	envName := strings.TrimSpace(r.PassphraseEnv)
	if envName == "" {
		envName = "BORG_PASSPHRASE"
	}
	if v := readEnv(envName); v != "" {
		env["BORG_PASSPHRASE"] = v
	}
	// BORG_RELOCATED_REPO_ACCESS_IS_OK suppresses an interactive prompt
	// when the repo is accessed from a different path than the one it
	// was last seen at (common on bind-mount reorganizations). Always
	// on for programmatic access.
	env["BORG_RELOCATED_REPO_ACCESS_IS_OK"] = "yes"
	env["BORG_UNKNOWN_UNENCRYPTED_REPO_ACCESS_IS_OK"] = "yes"
	if keyPath := strings.TrimSpace(r.SSHKeyPath); keyPath != "" {
		env["BORG_RSH"] = fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new", keyPath)
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

// mergeBorgJobs merges the auto-detect job list with the external-
// config job list, deduped by canonical repo path. Auto-detect is
// authoritative for repo metadata when both sides produced a job for
// the same path — but the Configured flag flips on (so the user
// still sees the "Configured" pill) and the Label, if supplied on
// the external entry, overrides the auto-detect display name.
func mergeBorgJobs(auto, external []internal.BackupJob) []internal.BackupJob {
	if len(external) == 0 {
		return auto
	}
	byPath := make(map[string]int, len(auto)+len(external))
	out := make([]internal.BackupJob, 0, len(auto)+len(external))
	for _, j := range auto {
		key := CanonicalRepoPath(j.Repository)
		out = append(out, j)
		byPath[key] = len(out) - 1
	}
	for _, j := range external {
		key := CanonicalRepoPath(j.Repository)
		if idx, ok := byPath[key]; ok {
			// Dedupe: mark auto-detect entry as configured, carry
			// label if provided. Preserves auto-detect's known-good
			// metadata.
			out[idx].Configured = true
			if j.Label != "" {
				out[idx].Label = j.Label
			}
			continue
		}
		out = append(out, j)
		byPath[key] = len(out) - 1
	}
	return out
}

// queryBorgRepoViaRunner runs the BorgRunner-backed two-call probe
// and composes a BackupJob. binary + env are passed through verbatim
// to the runner; empty binary means "use $PATH/borg". On runner
// error the returned job carries Error + ErrorReason so the dashboard
// can render an error card (external-config only; collectBorg drops
// these).
func queryBorgRepoViaRunner(runner BorgRunner, repoPath, binary string, env map[string]string) *internal.BackupJob {
	if strings.TrimSpace(repoPath) == "" {
		return nil
	}
	ctx := context.Background()
	info, err := runner.Info(ctx, repoPath, binary, env)
	if err != nil {
		var bre *BorgRunError
		reason := BorgErrUnknown
		errStr := err.Error()
		if errors.As(err, &bre) {
			reason = bre.Reason
			// Drop the underlying exec noise from the user-visible
			// Error string; keep the reason category as the primary
			// signal. The full err lands in logs via the caller.
			errStr = fmt.Sprintf("borg probe failed: %s", reason)
		}
		return &internal.BackupJob{
			Provider:    "borg",
			Name:        filepath.Base(repoPath),
			Repository:  repoPath,
			Status:      "failed",
			Error:       errStr,
			ErrorReason: reason,
		}
	}
	job := &internal.BackupJob{
		Provider:      "borg",
		Name:          filepath.Base(repoPath),
		Repository:    repoPath,
		SnapshotCount: info.ArchiveCount,
		Encrypted:     info.EncryptionMode != "" && info.EncryptionMode != "none",
	}
	if info.LatestArchive != nil {
		if !info.LatestArchive.Start.IsZero() {
			job.LastRun = info.LatestArchive.Start
			job.LastSuccess = info.LatestArchive.Start
		}
		if !info.LatestArchive.Start.IsZero() && !info.LatestArchive.End.IsZero() {
			job.Duration = info.LatestArchive.End.Sub(info.LatestArchive.Start).Seconds()
		}
		job.SizeBytes = info.LatestArchive.OriginalSize
		job.FilesCount = info.LatestArchive.NFiles
	} else if !info.LastModified.IsZero() {
		// Empty-repo / no-archive case: fall back to repository
		// last_modified so the widget has something to show.
		job.LastRun = info.LastModified
	}
	job.Status = backupStatus(job.LastSuccess)
	return job
}

func findBorgRepos(readEnv func(string) string) []string {
	var repos []string
	// Check BORG_REPO env
	if v := strings.TrimSpace(readEnv("BORG_REPO")); v != "" {
		repos = append(repos, v)
	}
	// Scan common locations
	for _, pattern := range []string{
		"/mnt/*/backups/borg",
		"/mnt/backup*/borg",
		"/backup/borg",
		"/volume*/backups/borg",
	} {
		matches, _ := filepath.Glob(pattern)
		repos = append(repos, matches...)
	}
	return repos
}

// ---------- Restic ----------

func collectRestic() []internal.BackupJob {
	if _, err := exec.LookPath("restic"); err != nil {
		return nil
	}

	// Check RESTIC_REPOSITORY env
	repo := ""
	if out, err := execCmd("sh", "-c", "echo $RESTIC_REPOSITORY"); err == nil {
		repo = strings.TrimSpace(out)
	}
	if repo == "" {
		return nil
	}

	// restic snapshots --json --latest 1
	out, err := execCmd("restic", "snapshots", "--json", "--latest", "1")
	if err != nil {
		return nil
	}

	var snapshots []struct {
		Time     string   `json:"time"`
		Hostname string   `json:"hostname"`
		Paths    []string `json:"paths"`
		ShortID  string   `json:"short_id"`
	}
	if err := json.Unmarshal([]byte(out), &snapshots); err != nil {
		return nil
	}

	job := &internal.BackupJob{
		Provider:   "restic",
		Name:       "restic",
		Repository: repo,
	}

	if len(snapshots) > 0 {
		latest := snapshots[0]
		if t, err := time.Parse(time.RFC3339Nano, latest.Time); err == nil {
			job.LastRun = t
			job.LastSuccess = t
		}
		job.Name = strings.Join(latest.Paths, ", ")
	}

	// Get stats
	if statsOut, err := execCmd("restic", "stats", "--json"); err == nil {
		var stats struct {
			TotalSize      int64 `json:"total_size"`
			TotalFileCount int   `json:"total_file_count"`
			SnapshotsCount int   `json:"snapshots_count"`
		}
		if json.Unmarshal([]byte(statsOut), &stats) == nil {
			job.SizeBytes = stats.TotalSize
			job.FilesCount = stats.TotalFileCount
			job.SnapshotCount = stats.SnapshotsCount
		}
	}

	job.Status = backupStatus(job.LastSuccess)
	return []internal.BackupJob{*job}
}

// ---------- Proxmox Backup Server (PBS) ----------

func collectPBS() []internal.BackupJob {
	if _, err := exec.LookPath("proxmox-backup-client"); err != nil {
		return nil
	}

	out, err := execCmd("proxmox-backup-client", "snapshot", "list", "--output-format", "json")
	if err != nil {
		return nil
	}

	var snapshots []struct {
		BackupType string `json:"backup-type"`
		BackupID   string `json:"backup-id"`
		BackupTime int64  `json:"backup-time"`
		Size       int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &snapshots); err != nil {
		return nil
	}

	if len(snapshots) == 0 {
		return nil
	}

	// Group by backup-id
	latest := snapshots[len(snapshots)-1]
	job := &internal.BackupJob{
		Provider:      "pbs",
		Name:          fmt.Sprintf("%s/%s", latest.BackupType, latest.BackupID),
		Repository:    "PBS",
		LastRun:       time.Unix(latest.BackupTime, 0),
		LastSuccess:   time.Unix(latest.BackupTime, 0),
		SizeBytes:     latest.Size,
		SnapshotCount: len(snapshots),
		Encrypted:     true,
	}
	job.Status = backupStatus(job.LastSuccess)
	return []internal.BackupJob{*job}
}

// ---------- Duplicati ----------

func collectDuplicati() []internal.BackupJob {
	if _, err := exec.LookPath("duplicati-cli"); err != nil {
		return nil
	}

	// Duplicati stores its DB in a known location
	// Try to query via the CLI
	out, err := execCmd("duplicati-cli", "list-broken-files", "--dbpath=/data/duplicati-config")
	if err != nil {
		// Try reading the server API instead
		out, err = execCmd("curl", "-sf", "http://localhost:8200/api/v1/backups")
		if err != nil {
			return nil
		}
	}
	_ = out // Parse Duplicati API response if available
	return nil
}

// ---------- Helpers ----------

func backupStatus(lastSuccess time.Time) string {
	if lastSuccess.IsZero() {
		return "failed"
	}
	age := time.Since(lastSuccess)
	switch {
	case age < 25*time.Hour:
		return "ok"
	case age < 49*time.Hour:
		return "warning"
	default:
		return "stale"
	}
}

// formatBytes formats bytes into human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
