package collector

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// defaultExecTimeout bounds every external command the collectors run during a
// scan. Before this, the collector exec calls had no deadline, so a single
// hung binary — a dead disk under smartctl, a stale NFS/SMB mount under df, a
// suspended pool under zpool, a wedged dockerd — blocked RunOnce forever.
// Because RunOnce is serialized behind a `running` flag, every later tick was
// then skipped and collection stopped for good while the HTTP server stayed
// healthy: the invisible "collection silently stopped" failure mode. A
// generous 60s ceiling lets legitimately slow commands finish (a spinning-up
// SATA drive, a large df) while guaranteeing a hung one is killed so the scan
// can proceed.
const defaultExecTimeout = 60 * time.Second

// execCmdContext runs name+args with a hard deadline and returns their combined
// stdout+stderr, mirroring (*exec.Cmd).CombinedOutput. On timeout the whole
// process group is SIGKILLed (not just the direct child) via the shared
// setProcessGroup/killProcessGroup helpers, so a shell that spawned a
// still-running helper cannot keep the output pipe open and block Wait. A 5s
// WaitDelay is a final backstop against a descendant that ignores SIGKILL.
func execCmdContext(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	setProcessGroup(cmd)
	cmd.Cancel = func() error { killProcessGroup(cmd); return nil }
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("%s timed out after %s", name, timeout)
	}
	return string(out), err
}

// execOutputContext is execCmdContext for callers that need stdout only
// (mirroring (*exec.Cmd).Output), used where interleaved stderr warnings would
// corrupt a stdout parser — e.g. zpool/zfs emit "pool is degraded"-style
// notices to stderr that must not land in the parsed table.
func execOutputContext(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	setProcessGroup(cmd)
	cmd.Cancel = func() error { killProcessGroup(cmd); return nil }
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	return out, err
}
