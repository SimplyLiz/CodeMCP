package secrets

import (
	"testing"
)

func TestValidateGitRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		// Valid refs
		{"empty string", "", false},
		{"sha1 hash", "abc123def456789012345678901234567890abcd", false},
		{"short hash", "abc123d", false},
		{"branch name", "main", false},
		{"feature branch", "feature/my-branch", false},
		{"HEAD", "HEAD", false},
		{"HEAD~1", "HEAD~1", false},
		{"HEAD^2", "HEAD^2", false},
		{"tag with dot", "v1.2.3", false},
		{"tag with underscore", "release_1.0", false},
		{"reflog syntax", "HEAD@{1}", false},

		// Invalid refs - potential injection
		{"starts with dash", "-malicious", true},
		{"contains space", "abc 123", true},
		{"contains semicolon", "abc;rm -rf", true},
		{"contains backtick", "abc`whoami`", true},
		{"contains dollar", "abc$HOME", true},
		{"contains single quote", "abc'def", true},
		{"contains double quote", `abc"def`, true},
		{"contains pipe", "abc|cat", true},
		{"contains ampersand", "abc&&cat", true},
		{"contains newline", "abc\ndef", true},
		{"contains null", "abc\x00def", true},
		{"too long", string(make([]byte, 300)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGitRef(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}
