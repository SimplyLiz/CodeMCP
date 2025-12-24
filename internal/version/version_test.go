package version

import (
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	// Save original values
	origVersion := Version
	origCommit := Commit
	defer func() {
		Version = origVersion
		Commit = origCommit
	}()

	tests := []struct {
		name        string
		version     string
		commit      string
		wantContain string
		wantExact   string
	}{
		{
			name:      "unknown commit",
			version:   "1.0.0",
			commit:    "unknown",
			wantExact: "1.0.0",
		},
		{
			name:      "short commit",
			version:   "1.0.0",
			commit:    "abc",
			wantExact: "1.0.0",
		},
		{
			name:        "full commit hash",
			version:     "1.0.0",
			commit:      "abc1234567890",
			wantContain: "abc1234",
		},
		{
			name:        "exactly 7 char commit",
			version:     "2.0.0",
			commit:      "1234567",
			wantExact:   "2.0.0",
		},
		{
			name:        "8 char commit",
			version:     "2.0.0",
			commit:      "12345678",
			wantContain: "1234567",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			Commit = tt.commit

			got := Info()

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("Info() = %q, want %q", got, tt.wantExact)
			}
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("Info() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

func TestFull(t *testing.T) {
	// Save original values
	origVersion := Version
	origCommit := Commit
	origBuildDate := BuildDate
	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildDate = origBuildDate
	}()

	Version = "1.2.3"
	Commit = "abcdef123456"
	BuildDate = "2024-01-15"

	got := Full()

	expectedParts := []string{
		"CKB version 1.2.3",
		"Commit: abcdef123456",
		"Built: 2024-01-15",
	}

	for _, part := range expectedParts {
		if !strings.Contains(got, part) {
			t.Errorf("Full() = %q, want to contain %q", got, part)
		}
	}
}

func TestDefaultValues(t *testing.T) {
	// Ensure version is set to a reasonable value
	if Version == "" {
		t.Error("Version should not be empty")
	}

	// Version should match semantic versioning pattern
	parts := strings.Split(Version, ".")
	if len(parts) < 2 {
		t.Errorf("Version %q doesn't appear to be semver", Version)
	}
}
