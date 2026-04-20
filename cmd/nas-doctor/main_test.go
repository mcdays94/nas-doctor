package main

import "testing"

// TestNormalizeListenAddr verifies the listen address normalization accepts
// both bare port numbers (e.g. "8067") and full address forms (e.g. ":8067",
// "0.0.0.0:8067"). This exists to reduce footguns for Unraid template users
// who type a bare port number into the NAS_DOCTOR_LISTEN variable.
func TestNormalizeListenAddr(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare port gets colon prefix", "8067", ":8067"},
		{"colon-prefixed port unchanged", ":8067", ":8067"},
		{"host:port unchanged", "0.0.0.0:8067", "0.0.0.0:8067"},
		{"default port unchanged", ":8060", ":8060"},
		{"ipv6-like host:port unchanged", "[::1]:8060", "[::1]:8060"},
		{"empty string unchanged", "", ""},
		{"whitespace is trimmed then normalized", "  8067  ", ":8067"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeListenAddr(c.in)
			if got != c.want {
				t.Errorf("normalizeListenAddr(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
