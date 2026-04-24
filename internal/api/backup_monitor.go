package api

import (
	"fmt"
	"os"
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
		r.Label = strings.TrimSpace(r.Label)
		r.RepoPath = strings.TrimSpace(r.RepoPath)
		r.BinaryPath = strings.TrimSpace(r.BinaryPath)
		r.PassphraseEnv = strings.TrimSpace(r.PassphraseEnv)
		r.SSHKeyPath = strings.TrimSpace(r.SSHKeyPath)

		// Per-row label for error messages: prefer user-supplied
		// label when set, else "entry N" 1-indexed.
		label := r.Label
		if label == "" {
			label = fmt.Sprintf("entry %d", i+1)
		}

		if r.Enabled && r.RepoPath == "" {
			return fmt.Errorf("backup_monitor.borg[%s]: repo_path is required when enabled", label)
		}
		// Non-empty BinaryPath must stat() regardless of Enabled —
		// catches typos the user might have meant to test immediately
		// via the Test button.
		if r.BinaryPath != "" {
			if _, err := os.Stat(r.BinaryPath); err != nil {
				return fmt.Errorf("backup_monitor.borg[%s]: binary_path %q not accessible: %v", label, r.BinaryPath, err)
			}
		}
		if r.PassphraseEnv != "" {
			if !envVarPattern.MatchString(r.PassphraseEnv) {
				return fmt.Errorf("backup_monitor.borg[%s]: passphrase_env %q must match ^[A-Z_][A-Z0-9_]*$", label, r.PassphraseEnv)
			}
		}
		if r.SSHKeyPath != "" {
			if _, err := os.Stat(r.SSHKeyPath); err != nil {
				return fmt.Errorf("backup_monitor.borg[%s]: ssh_key_path %q not accessible: %v", label, r.SSHKeyPath, err)
			}
		}
	}
	return nil
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
