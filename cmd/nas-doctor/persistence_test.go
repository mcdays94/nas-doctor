package main

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// TestCheckDataPersistence_SameDevice_ReportsEphemeral verifies the statfs-style
// device-ID comparison catches the classic "user forgot the bind-mount" footgun:
// when /data and / share a device ID they are on the same filesystem (the
// container's overlay writable layer) and the SQLite DB will be silently wiped
// on every container recreation. Issue #227.
func TestCheckDataPersistence_SameDevice_ReportsEphemeral(t *testing.T) {
	devIDs := map[string]uint64{
		"/data": 42,
		"/":     42,
	}
	persistent, err := checkDataPersistenceWith("/data", "/", fakeDevIDFn(devIDs))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if persistent {
		t.Error("expected /data to be reported ephemeral when it shares a device with / (overlay fs)")
	}
}

// TestCheckDataPersistence_DifferentDevice_ReportsPersistent verifies the
// happy path — when /data resolves to a different device than /, it is a
// real bind-mount and data survives container recreation.
func TestCheckDataPersistence_DifferentDevice_ReportsPersistent(t *testing.T) {
	devIDs := map[string]uint64{
		"/data": 42,
		"/":     1,
	}
	persistent, err := checkDataPersistenceWith("/data", "/", fakeDevIDFn(devIDs))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !persistent {
		t.Error("expected /data to be reported persistent when its device differs from /")
	}
}

// TestCheckDataPersistence_StatError_ReturnsError verifies that if stat fails
// (path missing, permission denied) we return the error rather than silently
// guessing. Callers log a warning and treat the check as non-authoritative.
func TestCheckDataPersistence_StatError_ReturnsError(t *testing.T) {
	fn := func(path string) (uint64, error) {
		return 0, errors.New("no such file")
	}
	_, err := checkDataPersistenceWith("/data", "/", fn)
	if err == nil {
		t.Fatal("expected error when stat fails, got nil")
	}
}

// TestWarnIfDataEphemeral_LogsWARN verifies that when the persistence check
// reports /data is ephemeral, we emit a WARN-level log line containing a
// user-actionable pointer to the bind-mount configuration. This is the
// user-visible signal documented in issue #227.
func TestWarnIfDataEphemeral_LogsWARN(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	devIDs := map[string]uint64{
		"/data": 42,
		"/":     42,
	}
	persistent := warnIfDataEphemeralWith(logger, "/data", "/", fakeDevIDFn(devIDs))
	if persistent {
		t.Error("expected persistent=false when /data shares device with /")
	}
	out := buf.String()
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("expected WARN level log entry, got: %s", out)
	}
	if !strings.Contains(out, "/data") {
		t.Errorf("expected log to mention /data, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "bind-mount") && !strings.Contains(strings.ToLower(out), "persistent") {
		t.Errorf("expected log to guide user toward bind-mount / persistent storage fix, got: %s", out)
	}
}

// TestWarnIfDataEphemeral_Persistent_NoWARN verifies the happy path stays
// silent — no WARN line when /data is correctly bind-mounted, so we don't
// add noise to the production logs of properly-configured users.
func TestWarnIfDataEphemeral_Persistent_NoWARN(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	devIDs := map[string]uint64{
		"/data": 42,
		"/":     1,
	}
	persistent := warnIfDataEphemeralWith(logger, "/data", "/", fakeDevIDFn(devIDs))
	if !persistent {
		t.Error("expected persistent=true when /data differs from /")
	}
	if strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("did not expect WARN log when /data is persistent, got: %s", buf.String())
	}
}

// TestWarnIfDataEphemeral_StatError_DoesNotCrash verifies that if we can't
// determine the device (e.g. /data hasn't been created yet) we neither
// crash nor return a false persistent=true. We log but treat unknown as
// "give the user the benefit of the doubt and report persistent" so that
// the dashboard banner doesn't flap on transient stat errors. The log
// captures it for ops visibility.
func TestWarnIfDataEphemeral_StatError_DoesNotCrash(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	fn := func(path string) (uint64, error) {
		return 0, errors.New("stat failed")
	}
	persistent := warnIfDataEphemeralWith(logger, "/data", "/", fn)
	if !persistent {
		t.Error("on stat error we default to persistent=true to avoid false-positive banners")
	}
	// We still want visibility — INFO or DEBUG log is fine, but don't scream WARN
	// for a benign transient error. The check simply couldn't run.
	if strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("should not log WARN for a stat error, got: %s", buf.String())
	}
}

func fakeDevIDFn(m map[string]uint64) func(string) (uint64, error) {
	return func(path string) (uint64, error) {
		id, ok := m[path]
		if !ok {
			return 0, errors.New("no fake entry for " + path)
		}
		return id, nil
	}
}
