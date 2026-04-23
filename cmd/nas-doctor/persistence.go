package main

import (
	"log/slog"
	"syscall"
)

// devIDFn resolves a filesystem path to a device identifier. In production
// this is a stat(2) call that returns Stat_t.Dev (the device the path lives
// on); tests inject a fake to exercise the comparison logic without real
// filesystem state. Returning an error lets the caller distinguish "path
// missing / not accessible" from "path exists on device X".
type devIDFn func(path string) (uint64, error)

// realDevID returns the device id that the given path currently lives on.
// On Linux this is populated by the kernel from the inode's superblock,
// so a bind-mounted /data (hosted by a real filesystem on the host) will
// have a different Dev than / (the container's overlay rootfs).
func realDevID(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

// checkDataPersistenceWith reports whether dataDir appears to be a
// persistent (bind-mounted) filesystem distinct from rootDir. If dataDir
// and rootDir share a device id they are both on the container's writable
// overlay layer — which means SQLite writes survive container restart but
// are silently wiped on every container recreation (docker-compose down,
// template re-apply, image bump). See issue #227.
//
// Returns:
//
//	persistent=true  when dataDir and rootDir are on different devices
//	persistent=false when they share a device (ephemeral overlay fs)
//	err != nil       when either stat failed; caller decides how to react
//
// The devIDFn seam exists for tests; production callers use realDevID.
func checkDataPersistenceWith(dataDir, rootDir string, fn devIDFn) (persistent bool, err error) {
	dataDev, err := fn(dataDir)
	if err != nil {
		return false, err
	}
	rootDev, err := fn(rootDir)
	if err != nil {
		return false, err
	}
	return dataDev != rootDev, nil
}

// warnIfDataEphemeralWith runs the persistence check and emits a loud
// WARN log if /data is on the same device as /. Returns the effective
// persistent flag the caller should propagate to the dashboard banner:
// stat errors are treated as "unknown, assume persistent" to avoid a
// flapping false-positive banner on transient filesystem hiccups. The
// log still captures the stat error so operators can investigate.
//
// Issue #227 — defense-in-depth. Most users run the container correctly
// with /mnt/user/appdata/nas-doctor bind-mounted to /data; this warning
// catches the silent-data-loss footgun when that mount is missing.
func warnIfDataEphemeralWith(logger *slog.Logger, dataDir, rootDir string, fn devIDFn) (persistent bool) {
	persistent, err := checkDataPersistenceWith(dataDir, rootDir, fn)
	if err != nil {
		// Non-authoritative: log INFO (not WARN — don't scream for a
		// benign transient error) and assume persistent so we don't
		// raise a false-positive dashboard banner.
		logger.Info("data persistence check skipped", "data_dir", dataDir, "root_dir", rootDir, "error", err)
		return true
	}
	if !persistent {
		logger.Warn(
			dataDir+" does not appear to be a persistent bind-mount — SQLite DB and config will be LOST on container recreation. "+
				"Map a host path (e.g. /mnt/user/appdata/nas-doctor) to "+dataDir+" in your container configuration. "+
				"See the README for bind-mount setup.",
			"data_dir", dataDir,
			"root_dir", rootDir,
			"issue", "#227",
		)
	}
	return persistent
}

// warnIfDataEphemeral is the production entry point wired from main. It
// runs the real stat-based device-id comparison against /data and /.
func warnIfDataEphemeral(logger *slog.Logger, dataDir string) (persistent bool) {
	return warnIfDataEphemeralWith(logger, dataDir, "/", realDevID)
}
