package collector

import (
	"testing"
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
