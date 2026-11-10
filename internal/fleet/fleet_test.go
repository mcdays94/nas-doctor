package fleet

import "testing"

// TestNormalizeHealth: a peer's health string is rendered by the fleet
// page, so only the known values pass through.
func TestNormalizeHealth(t *testing.T) {
	cases := map[string]string{
		"healthy":                      "healthy",
		"Warning":                      "warning",
		" critical ":                   "critical",
		"":                             "unknown",
		"<img src=x onerror=alert(1)>": "unknown",
		"ok":                           "unknown",
	}
	for in, want := range cases {
		if got := normalizeHealth(in); got != want {
			t.Errorf("normalizeHealth(%q) = %q, want %q", in, got, want)
		}
	}
}
