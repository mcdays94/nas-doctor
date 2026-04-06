package collector

import "testing"

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest    string
		installed string
		want      bool
	}{
		{"7.2.0", "7.1.4", true},
		{"7.1.5", "7.1.4", true},
		{"7.1.4", "7.1.4", false},
		{"7.1.3", "7.1.4", false},
		{"8.0.0", "7.99.99", true},
		{"6.12.11", "6.12.10", true},
		{"6.12.10", "6.12.10", false},
		{"6.13.0", "6.12.10", true},
		{"24.10.1", "24.10.0", true},
		{"24.10.0", "24.10.0", false},
		{"24.11.0", "24.10.2", true},
		{"1.0", "1.0.0", false}, // equal when shorter
		{"1.0.1", "1.0", true},  // patch bump counts
		{"2.0", "1.99", true},
	}
	for _, tt := range tests {
		got := isNewerVersion(tt.latest, tt.installed)
		if got != tt.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.installed, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"7.1.4", []int{7, 1, 4}},
		{"6.12.10-Unraid", []int{6, 12, 10}},
		{"24.10.0.1", []int{24, 10, 0, 1}},
		{"v3.2.1", []int{3, 2, 1}}, // after normalizeVersion strips v
		{"1.0", []int{1, 0}},
		{"", nil},
	}
	for _, tt := range tests {
		input := normalizeVersion(tt.input)
		got := parseVersion(input)
		if len(got) != len(tt.want) {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v7.1.4", "7.1.4"},
		{"V3.0.0", "3.0.0"},
		{"  7.1.4  ", "7.1.4"},
		{"7.1.4", "7.1.4"},
	}
	for _, tt := range tests {
		got := normalizeVersion(tt.input)
		if got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
