// Process-group helpers for the speedtest subprocess kill chain.
// Linux + Darwin both support Setpgid + negative-pid kill — the
// project ships only on those two platforms (Docker = Alpine Linux,
// development = macOS). A Windows stub is omitted deliberately.

//go:build linux || darwin

package collector

import (
	"os/exec"
	"syscall"
)

// setProcessGroup configures cmd to start in its own process group
// (Setpgid) so we can later signal the entire group with negative-pid
// kill — propagating to every descendant the subprocess spawns.
// Without this, sending SIGKILL to /bin/sh leaves an inherited-pipe-
// holding `sleep` (or speedtest binary) running and blocks Wait()
// until that orphan exits naturally. Issue #304 CI failure root cause.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup sends SIGKILL to the process group rooted at the
// given cmd. Idempotent: a no-op if the process never started or
// has already been reaped. The negative-pid argument to syscall.Kill
// is the canonical "signal the whole group" pattern documented in
// `man 2 kill`. Issue #304.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// Process already dead, or PID-namespace edge case in
		// rootless containers — fall back to a single-process kill
		// so we still try to abort whatever is left.
		_ = cmd.Process.Kill()
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
