package ownership

import (
	"regexp"
	"testing"
	"time"
)

func TestParseBlameOutput(t *testing.T) {
	// Sample git blame porcelain output
	output := []byte(`abc123def456789012345678901234567890abcd 1 1 1
author John Doe
author-mail <john@example.com>
author-time 1700000000
author-tz +0000
committer John Doe
committer-mail <john@example.com>
committer-time 1700000000
committer-tz +0000
summary Initial commit
filename test.go
	package main
def456abc789012345678901234567890abcd1234 2 2 1
author Jane Smith
author-mail <jane@example.com>
author-time 1700100000
author-tz +0000
committer Jane Smith
committer-mail <jane@example.com>
committer-time 1700100000
committer-tz +0000
summary Add feature
filename test.go
	func main() {}
`)

	entries, err := parseBlameOutput(output)
	if err != nil {
		t.Fatalf("Failed to parse blame output: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0].Author != "John Doe" {
		t.Errorf("Expected author 'John Doe', got '%s'", entries[0].Author)
	}
	if entries[0].AuthorMail != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got '%s'", entries[0].AuthorMail)
	}

	// Check second entry
	if entries[1].Author != "Jane Smith" {
		t.Errorf("Expected author 'Jane Smith', got '%s'", entries[1].Author)
	}
}

func TestComputeBlameOwnership(t *testing.T) {
	now := time.Now()
	recent := now.Add(-7 * 24 * time.Hour)    // 7 days ago
	old := now.Add(-180 * 24 * time.Hour)     // 180 days ago (2 half-lives)

	result := &BlameResult{
		FilePath: "test.go",
		Entries: []BlameEntry{
			// Recent contributor - 2 lines
			{CommitHash: "abc123", Author: "Recent Dev", AuthorMail: "recent@example.com", Timestamp: recent, LineNumber: 1},
			{CommitHash: "abc123", Author: "Recent Dev", AuthorMail: "recent@example.com", Timestamp: recent, LineNumber: 2},
			// Old contributor - 2 lines
			{CommitHash: "def456", Author: "Old Dev", AuthorMail: "old@example.com", Timestamp: old, LineNumber: 3},
			{CommitHash: "def456", Author: "Old Dev", AuthorMail: "old@example.com", Timestamp: old, LineNumber: 4},
		},
	}

	config := DefaultBlameConfig()
	ownership := ComputeBlameOwnership(result, config)

	if ownership.TotalLines != 4 {
		t.Errorf("Expected 4 total lines, got %d", ownership.TotalLines)
	}

	if len(ownership.Contributors) != 2 {
		t.Errorf("Expected 2 contributors, got %d", len(ownership.Contributors))
	}

	// Recent contributor should have higher percentage due to time decay
	if ownership.Contributors[0].Email != "recent@example.com" {
		t.Errorf("Expected recent contributor first, got %s", ownership.Contributors[0].Email)
	}

	// Recent contributor should have significantly higher weighted percentage
	// 2 recent lines vs 2 old lines (2 half-lives = 0.25 weight)
	// Recent weight ≈ 2 * 0.95 ≈ 1.9 (roughly, 7 days is small decay)
	// Old weight ≈ 2 * 0.25 = 0.5
	// So recent should be about 80% vs 20%
	if ownership.Contributors[0].Percentage < 0.6 {
		t.Errorf("Expected recent contributor to have >60%%, got %.2f%%", ownership.Contributors[0].Percentage*100)
	}
}

func TestComputeBlameOwnershipExcludesBots(t *testing.T) {
	now := time.Now()
	recent := now.Add(-7 * 24 * time.Hour)

	result := &BlameResult{
		FilePath: "test.go",
		Entries: []BlameEntry{
			{CommitHash: "abc123", Author: "dependabot[bot]", AuthorMail: "dependabot@github.com", Timestamp: recent, LineNumber: 1},
			{CommitHash: "def456", Author: "Human Dev", AuthorMail: "human@example.com", Timestamp: recent, LineNumber: 2},
			{CommitHash: "ghi789", Author: "renovate[bot]", AuthorMail: "renovate@github.com", Timestamp: recent, LineNumber: 3},
		},
	}

	config := DefaultBlameConfig()
	ownership := ComputeBlameOwnership(result, config)

	// Only human contributor should be counted
	if len(ownership.Contributors) != 1 {
		t.Errorf("Expected 1 contributor (excluding bots), got %d", len(ownership.Contributors))
	}

	if ownership.Contributors[0].Email != "human@example.com" {
		t.Errorf("Expected human contributor, got %s", ownership.Contributors[0].Email)
	}

	// Human should have 100% since bots are excluded
	if ownership.Contributors[0].Percentage != 1.0 {
		t.Errorf("Expected 100%% ownership, got %.2f%%", ownership.Contributors[0].Percentage*100)
	}
}

func TestBlameOwnershipToOwners(t *testing.T) {
	ownership := &BlameOwnership{
		FilePath:   "test.go",
		TotalLines: 100,
		Contributors: []AuthorContribution{
			{Author: "maintainer", Email: "maintainer@example.com", Percentage: 0.55},
			{Author: "reviewer", Email: "reviewer@example.com", Percentage: 0.25},
			{Author: "contributor", Email: "contributor@example.com", Percentage: 0.10},
		},
		Confidence: 0.79,
	}

	owners := BlameOwnershipToOwners(ownership)

	if len(owners) != 3 {
		t.Errorf("Expected 3 owners, got %d", len(owners))
	}

	// Check scopes
	if owners[0].Scope != "maintainer" {
		t.Errorf("Expected scope 'maintainer' for 55%%, got '%s'", owners[0].Scope)
	}
	if owners[1].Scope != "reviewer" {
		t.Errorf("Expected scope 'reviewer' for 25%%, got '%s'", owners[1].Scope)
	}
	if owners[2].Scope != "contributor" {
		t.Errorf("Expected scope 'contributor' for 10%%, got '%s'", owners[2].Scope)
	}

	// Check source and confidence
	for _, owner := range owners {
		if owner.Source != "git-blame" {
			t.Errorf("Expected source 'git-blame', got '%s'", owner.Source)
		}
		if owner.Confidence != 0.79 {
			t.Errorf("Expected confidence 0.79, got %f", owner.Confidence)
		}
	}
}

func TestIsBot(t *testing.T) {
	config := DefaultBlameConfig()
	var patterns []*regexp.Regexp
	for _, pattern := range config.BotPatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			patterns = append(patterns, re)
		}
	}

	tests := []struct {
		author   string
		email    string
		expected bool
	}{
		{"dependabot[bot]", "dependabot@github.com", true},
		{"renovate[bot]", "renovate@github.com", true},
		{"github-actions[bot]", "actions@github.com", true},
		{"Human Developer", "human@example.com", false},
		{"John Doe", "john@example.com", false},
	}

	for _, tt := range tests {
		result := isBot(tt.author, tt.email, patterns)
		if result != tt.expected {
			t.Errorf("isBot(%s, %s): expected %v, got %v", tt.author, tt.email, tt.expected, result)
		}
	}
}

func TestNormalizeAuthorKey(t *testing.T) {
	tests := []struct {
		author   string
		email    string
		expected string
	}{
		{"John Doe", "john@example.com", "john@example.com"},
		{"Jane Smith", "JANE@EXAMPLE.COM", "jane@example.com"},
		{"Anon", "noreply@github.com", "anon"}, // noreply emails fall back to name
		{"No Email", "", "no email"},
	}

	for _, tt := range tests {
		result := normalizeAuthorKey(tt.author, tt.email)
		if result != tt.expected {
			t.Errorf("normalizeAuthorKey(%s, %s): expected %s, got %s", tt.author, tt.email, tt.expected, result)
		}
	}
}

func TestDefaultBlameConfig(t *testing.T) {
	config := DefaultBlameConfig()

	if config.TimeDecayHalfLife != 90 {
		t.Errorf("Expected TimeDecayHalfLife 90, got %d", config.TimeDecayHalfLife)
	}

	if !config.ExcludeBots {
		t.Error("Expected ExcludeBots to be true")
	}

	if len(config.BotPatterns) == 0 {
		t.Error("Expected bot patterns to be populated")
	}

	if config.MinContribution != 0.05 {
		t.Errorf("Expected MinContribution 0.05, got %f", config.MinContribution)
	}
}
