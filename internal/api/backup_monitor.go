package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mcdays94/nas-doctor/internal/collector"
)

// envVarPattern matches a POSIX-ish env var name. Ruling out
// punctuation ensures the name we look up and the name we forward to
// borg's BORG_PASSPHRASE env can't accidentally collide with shell
// metachars or injection vectors if the value is ever templated into
// a subprocess spec.
var envVarPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// validateBackupMonitorBorg runs field-level checks on the user-
// configured external Borg array. Invariants enforced:
//
//   - Enabled entries must have a non-empty RepoPath
//   - BinaryPath, if non-empty, must stat() (catches typos before
//     the next scan tick; issue #279 user story 9)
//   - PassphraseEnv, if non-empty, must match envVarPattern
//   - SSHKeyPath, if non-empty, must stat() (catches missing key)
//
// Returns a user-friendly error with an "entry N" prefix so the
// Settings UI can point the user at the offending row. The error
// message shape is stable — the UI Test button reads the reason
// phrase directly.
func validateBackupMonitorBorg(repos []BorgExternalRepo) error {
	for i, r := range repos {
		// Per-row label for error messages: prefer user-supplied
		// label when set, else "entry N" 1-indexed.
		label := strings.TrimSpace(r.Label)
		if label == "" {
			label = fmt.Sprintf("entry %d", i+1)
		}
		if err := validateBorgRepo(r); err != nil {
			return fmt.Errorf("backup_monitor.borg[%s]: %v", label, err)
		}
	}
	return nil
}

// validateBorgRepo checks one external-Borg entry. It is shared by the
// settings save and the Test endpoint: both end in exec'ing BinaryPath
// with RepoPath on its command line and SSHKeyPath in BORG_RSH, so both
// must refuse the same inputs. The Test endpoint used to skip these checks
// entirely and would run whatever binary_path named.
func validateBorgRepo(r BorgExternalRepo) error {
	repoPath := strings.TrimSpace(r.RepoPath)
	binaryPath := strings.TrimSpace(r.BinaryPath)
	passEnv := strings.TrimSpace(r.PassphraseEnv)
	sshKeyPath := strings.TrimSpace(r.SSHKeyPath)

	if r.Enabled && repoPath == "" {
		return fmt.Errorf("repo_path is required when enabled")
	}
	if strings.HasPrefix(repoPath, "-") {
		// Would be read by borg as an option, not a repository.
		return fmt.Errorf("repo_path %q must not start with '-'", repoPath)
	}
	if hasControlChars(repoPath) {
		return fmt.Errorf("repo_path contains control characters")
	}
	// Non-empty BinaryPath is checked regardless of Enabled — catches
	// typos the user might have meant to test immediately via the Test
	// button. It must be a real borg binary: an absolute path to an
	// executable regular file whose name starts with "borg".
	if binaryPath != "" {
		if !filepath.IsAbs(binaryPath) {
			return fmt.Errorf("binary_path %q must be an absolute path", binaryPath)
		}
		if !strings.HasPrefix(filepath.Base(binaryPath), "borg") {
			return fmt.Errorf("binary_path %q must point at a borg binary (file name starting with \"borg\")", binaryPath)
		}
		fi, err := os.Stat(binaryPath)
		if err != nil {
			return fmt.Errorf("binary_path %q not accessible: %v", binaryPath, err)
		}
		if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("binary_path %q is not an executable file", binaryPath)
		}
	}
	if passEnv != "" && !envVarPattern.MatchString(passEnv) {
		return fmt.Errorf("passphrase_env %q must match ^[A-Z_][A-Z0-9_]*$", passEnv)
	}
	if sshKeyPath != "" {
		if !filepath.IsAbs(sshKeyPath) {
			return fmt.Errorf("ssh_key_path %q must be an absolute path", sshKeyPath)
		}
		if hasControlChars(sshKeyPath) {
			return fmt.Errorf("ssh_key_path contains control characters")
		}
		fi, err := os.Stat(sshKeyPath)
		if err != nil {
			return fmt.Errorf("ssh_key_path %q not accessible: %v", sshKeyPath, err)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("ssh_key_path %q is not a file", sshKeyPath)
		}
	}
	return nil
}

func hasControlChars(s string) bool {
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

// apiBorgReposToCollector converts the API-layer representation into
// the collector-layer BorgExternalRepo list. Kept as a pure function
// so tests can exercise the mapping without touching the full PUT
// handler. Issue #279.
func apiBorgReposToCollector(repos []BorgExternalRepo) []collector.BorgExternalRepo {
	if len(repos) == 0 {
		return nil
	}
	out := make([]collector.BorgExternalRepo, 0, len(repos))
	for _, r := range repos {
		out = append(out, collector.BorgExternalRepo{
			Enabled:       r.Enabled,
			Label:         strings.TrimSpace(r.Label),
			RepoPath:      strings.TrimSpace(r.RepoPath),
			BinaryPath:    strings.TrimSpace(r.BinaryPath),
			PassphraseEnv: strings.TrimSpace(r.PassphraseEnv),
			SSHKeyPath:    strings.TrimSpace(r.SSHKeyPath),
		})
	}
	return out
}

// apiDuplicacyEntriesToCollector converts the API-layer DuplicacyEntry
// list into the collector-layer list. Mirrors apiBorgReposToCollector
// in shape so the conversion stays trivial as more provider types
// are added. Issue #314.
func apiDuplicacyEntriesToCollector(entries []DuplicacyEntry) []collector.DuplicacyEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]collector.DuplicacyEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, collector.DuplicacyEntry{
			Enabled:    e.Enabled,
			Label:      strings.TrimSpace(e.Label),
			Kind:       strings.TrimSpace(e.Kind),
			Path:       strings.TrimSpace(e.Path),
			StorageID:  strings.TrimSpace(e.StorageID),
			StaleAfter: e.StaleAfter,
		})
	}
	return out
}
