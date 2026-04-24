package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDockerfile_BundlesBorgbackup pins that the Alpine base image
// includes the borgbackup package. Issue #279 architecture correction
// (#278 comment 2026-04-24): mounting a host glibc binary into the
// Alpine container silently fails (ABI mismatch); bundling the musl-
// compiled package is the architectural fix.
//
// The regression guard reads the Dockerfile directly — a rename like
// "borgbackup" → "borg" (which doesn't exist on Alpine) would pass
// local build on the existing cache but break a fresh build.
func TestDockerfile_BundlesBorgbackup(t *testing.T) {
	// Walk up from working dir to find Dockerfile (go test runs from
	// the package dir, so we need to pop back to repo root).
	dir, _ := os.Getwd()
	var path string
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "Dockerfile")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
			break
		}
		dir = filepath.Dir(dir)
	}
	if path == "" {
		t.Fatal("Dockerfile not found in parent tree")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "borgbackup") {
		t.Error("Dockerfile missing borgbackup package — issue #279 requires the Alpine package bundled in-image")
	}
	// Also assert the package is on an apk line (not accidentally
	// removed from apk but left in a comment). The simplest test:
	// the literal line containing `apk add --no-cache` should
	// include borgbackup.
	var found bool
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "apk add --no-cache") && strings.Contains(line, "borgbackup") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Dockerfile has borgbackup mentioned but not on an apk install line")
	}
}
