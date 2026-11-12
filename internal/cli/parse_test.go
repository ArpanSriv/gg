package cli

import "testing"

func TestParsePRRef(t *testing.T) {
	tests := []struct {
		in          string
		owner, name string
		num         int
		wantErr     bool
	}{
		{"42", "", "", 42, false},
		{"#42", "", "", 42, false},
		{"cli/cli#123", "cli", "cli", 123, false},
		{"https://github.com/cli/cli/pull/123", "cli", "cli", 123, false},
		{"https://github.com/cli/cli/pull/123/files", "cli", "cli", 123, false},
		{"https://github.com/cli/cli/pull/123#discussion_r1", "cli", "cli", 123, false},
		{"https://example.com/cli/cli/pull/123", "", "", 0, true},
		{"cli/cli", "", "", 0, true},
		{"abc", "", "", 0, true},
	}
	for _, tt := range tests {
		owner, name, num, err := parsePRRef(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parsePRRef(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && (owner != tt.owner || name != tt.name || num != tt.num) {
			t.Errorf("parsePRRef(%q) = %q, %q, %d; want %q, %q, %d", tt.in, owner, name, num, tt.owner, tt.name, tt.num)
		}
	}
}

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		in          string
		owner, name string
		ok          bool
	}{
		{"https://github.com/cli/cli", "cli", "cli", true},
		{"https://github.com/cli/cli.git", "cli", "cli", true},
		{"https://github.com/cli/cli/", "cli", "cli", true},
		{"git@github.com:cli/cli.git", "cli", "cli", true},
		{"ssh://git@github.com/cli/cli.git", "cli", "cli", true},
		{"github.com/cli/go-gh", "cli", "go-gh", true},
		{"cli/cli", "", "", false},
		{"https://gitlab.com/cli/cli", "", "", false},
		{"https://github.com/cli/cli/pull/1", "", "", false},
	}
	for _, tt := range tests {
		owner, name, ok := parseRepoURL(tt.in)
		if ok != tt.ok || owner != tt.owner || name != tt.name {
			t.Errorf("parseRepoURL(%q) = %q, %q, %v; want %q, %q, %v", tt.in, owner, name, ok, tt.owner, tt.name, tt.ok)
		}
	}
}
