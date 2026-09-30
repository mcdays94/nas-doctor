package collector

import (
	"strings"
	"testing"
	"time"
)

// TestExecCmdContext_HappyPath confirms the timeout wrapper is transparent for
// well-behaved commands: output is returned and no error is reported.
func TestExecCmdContext_HappyPath(t *testing.T) {
	out, err := execCmdContext(10*time.Second, "sh", "-c", "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q; want %q", strings.TrimSpace(out), "hello")
	}
}

// TestExecCmdContext_NonZeroExitIsNotATimeout confirms a command that fails on
// its own (non-zero exit) returns an error but is NOT mislabelled as a timeout.
func TestExecCmdContext_NonZeroExitIsNotATimeout(t *testing.T) {
	_, err := execCmdContext(10*time.Second, "sh", "-c", "exit 3")
	if err == nil {
		t.Fatal("expected a non-nil error for a non-zero exit")
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Errorf("non-zero exit was mislabelled as a timeout: %v", err)
	}
}

// TestExecCmdContext_KillsHungProcessGroup is the core regression guard for the
// "collection silently stopped" wedge: a hung command must be killed at the
// deadline, and its whole process group with it.
//
// The command backgrounds a `sleep` child. If only the direct child (sh) were
// killed, the orphaned sleep would keep the output pipe open and CombinedOutput
// would block until the 5s WaitDelay backstop — so an elapsed time well under
// that proves the process-group kill (not just WaitDelay) did the work.
func TestExecCmdContext_KillsHungProcessGroup(t *testing.T) {
	start := time.Now()
	_, err := execCmdContext(300*time.Millisecond, "sh", "-c", "sleep 30 & sleep 30")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error for a hung command")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v; want a timeout error", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("command took %v; expected it to be killed near the 300ms deadline — "+
			"a duration near the 5s WaitDelay means the process group was NOT killed", elapsed)
	}
}
