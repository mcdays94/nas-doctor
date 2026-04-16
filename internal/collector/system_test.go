package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

func TestParseContainerIDFromCgroup(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "cgroup v1 docker path",
			content: "12:devices:/docker/abc123def456789abc123def456789abc123def456789abc123def456789abcd\n",
			want:    "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
		},
		{
			name: "cgroup v1 multi-line with docker on one line",
			content: `11:cpuset:/docker/abc123def456789abc123def456789abc123def456789abc123def456789abcd
10:memory:/docker/abc123def456789abc123def456789abc123def456789abc123def456789abcd
9:devices:/docker/abc123def456789abc123def456789abc123def456789abc123def456789abcd
`,
			want: "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
		},
		{
			name:    "cgroup v2 systemd docker scope",
			content: "0::/system.slice/docker-abc123def456789abc123def456789abc123def456789abc123def456789abcd.scope\n",
			want:    "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
		},
		{
			name:    "non-container process",
			content: "0::/user.slice/user-1000.slice/session-1.scope\n",
			want:    "",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "kernel thread or init",
			content: "0::/init.scope\n",
			want:    "",
		},
		{
			name: "cgroup v1 with nested docker path",
			content: `12:blkio:/docker/fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210
11:memory:/docker/fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210
`,
			want: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
		},
		{
			name:    "containerd/cri-o format (not docker)",
			content: "0::/kubepods/burstable/pod12345/cri-containerd-abc123\n",
			want:    "",
		},
		{
			name:    "docker compose with buildkit",
			content: "0::/system.slice/docker-aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb.scope\n",
			want:    "aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseContainerIDFromCgroup(tt.content)
			if got != tt.want {
				t.Errorf("parseContainerIDFromCgroup() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractContainerID(t *testing.T) {
	// Create a fake /proc/<pid>/cgroup file in a temp directory
	tmpDir := t.TempDir()

	// Fake PID 42 with cgroup v1 docker content
	pid42Dir := filepath.Join(tmpDir, "42")
	if err := os.MkdirAll(pid42Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cgroupContent := "12:devices:/docker/abc123def456789abc123def456789abc123def456789abc123def456789abcd\n"
	if err := os.WriteFile(filepath.Join(pid42Dir, "cgroup"), []byte(cgroupContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fake PID 99 with non-container cgroup
	pid99Dir := filepath.Join(tmpDir, "99")
	if err := os.MkdirAll(pid99Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pid99Dir, "cgroup"), []byte("0::/init.scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		pid  int
		want string
	}{
		{
			name: "docker container process",
			pid:  42,
			want: "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
		},
		{
			name: "non-container process",
			pid:  99,
			want: "",
		},
		{
			name: "nonexistent PID (graceful)",
			pid:  99999,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContainerID(tt.pid, tmpDir)
			if got != tt.want {
				t.Errorf("extractContainerID(%d) = %q, want %q", tt.pid, got, tt.want)
			}
		})
	}
}

func TestBuildContainerIDMap(t *testing.T) {
	containers := []internal.ContainerInfo{
		{ID: "abc123def456", Name: "nginx"},
		{ID: "fedcba987654", Name: "postgres"},
		{ID: "", Name: "broken"}, // edge: empty ID
	}

	m := buildContainerIDMap(containers)

	if got := m["abc123def456"]; got != "nginx" {
		t.Errorf("map[abc123def456] = %q, want %q", got, "nginx")
	}
	if got := m["fedcba987654"]; got != "postgres" {
		t.Errorf("map[fedcba987654] = %q, want %q", got, "postgres")
	}
	if _, ok := m[""]; ok {
		t.Error("empty ID should not be in the map")
	}
}

func TestEnrichProcessContainers(t *testing.T) {
	// Create fake proc filesystem
	tmpDir := t.TempDir()

	fullID := "abc123def456789abc123def456789abc123def456789abc123def456789abcd"

	// PID 10: in a docker container
	pid10Dir := filepath.Join(tmpDir, "10")
	if err := os.MkdirAll(pid10Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pid10Dir, "cgroup"),
		[]byte("12:devices:/docker/"+fullID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// PID 20: host process
	pid20Dir := filepath.Join(tmpDir, "20")
	if err := os.MkdirAll(pid20Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pid20Dir, "cgroup"),
		[]byte("0::/init.scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	procs := []internal.ProcessInfo{
		{PID: 10, User: "root", CPU: 25.0, Mem: 10.0, Command: "nginx"},
		{PID: 20, User: "root", CPU: 5.0, Mem: 2.0, Command: "sshd"},
		{PID: 30, User: "root", CPU: 1.0, Mem: 0.5, Command: "kworker"}, // no cgroup file
	}

	// containerMap: short IDs map to names, but we also need to test prefix matching.
	// Docker ps returns short IDs (12 char), cgroup has full 64-char IDs.
	containerMap := map[string]string{
		"abc123def456": "my-nginx",
	}

	enrichProcessContainers(procs, containerMap, tmpDir)

	// PID 10 should be attributed to my-nginx
	if procs[0].ContainerID != fullID {
		t.Errorf("PID 10 ContainerID = %q, want %q", procs[0].ContainerID, fullID)
	}
	if procs[0].ContainerName != "my-nginx" {
		t.Errorf("PID 10 ContainerName = %q, want %q", procs[0].ContainerName, "my-nginx")
	}

	// PID 20 should have no container
	if procs[1].ContainerID != "" {
		t.Errorf("PID 20 ContainerID = %q, want empty", procs[1].ContainerID)
	}
	if procs[1].ContainerName != "" {
		t.Errorf("PID 20 ContainerName = %q, want empty", procs[1].ContainerName)
	}

	// PID 30 should have no container (missing cgroup file)
	if procs[2].ContainerID != "" {
		t.Errorf("PID 30 ContainerID = %q, want empty", procs[2].ContainerID)
	}
	if procs[2].ContainerName != "" {
		t.Errorf("PID 30 ContainerName = %q, want empty", procs[2].ContainerName)
	}
}
