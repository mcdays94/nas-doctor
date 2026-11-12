package collector

import (
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/mnt/keys/borg_id_ed25519":          "/mnt/keys/borg_id_ed25519",
		"/mnt/my keys/id":                    "'/mnt/my keys/id'",
		"/dev/null -o ProxyCommand=touch /x": "'/dev/null -o ProxyCommand=touch /x'",
		"/k/it's":                            `'/k/it'\''s'`,
		"":                                   "''",
		"/k/$(id)":                           "'/k/$(id)'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestBuildBorgEnv_SSHKeyPathIsOneArgument: Borg splits BORG_RSH like a
// shell, so a key path with spaces must not turn into extra ssh options.
func TestBuildBorgEnv_SSHKeyPathIsOneArgument(t *testing.T) {
	env := buildBorgEnv(BorgExternalRepo{SSHKeyPath: "/dev/null -o ProxyCommand=touch /tmp/x"}, noopEnv)
	rsh := env["BORG_RSH"]
	if !strings.HasPrefix(rsh, "ssh -i '/dev/null -o ProxyCommand=touch /tmp/x' ") {
		t.Errorf("BORG_RSH = %q; the key path must be quoted as a single word", rsh)
	}
	// A plain path keeps the familiar unquoted form.
	plain := buildBorgEnv(BorgExternalRepo{SSHKeyPath: "/mnt/keys/id_ed25519"}, noopEnv)["BORG_RSH"]
	if plain != "ssh -i /mnt/keys/id_ed25519 -o StrictHostKeyChecking=accept-new" {
		t.Errorf("BORG_RSH = %q", plain)
	}
}

func TestBorgRunner_RefusesOptionLikeRepoPath(t *testing.T) {
	_, err := NewExecBorgRunner().Info(nil, "--rsh=touch /tmp/x", "", nil)
	if err == nil {
		t.Fatal("a repo path starting with '-' reached borg")
	}
}
