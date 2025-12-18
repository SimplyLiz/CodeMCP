package ownership

import (
	"regexp"
	"testing"
)

// =============================================================================
// v6.0 Ownership Benchmarks
// =============================================================================
// getOwnership: Cheap, P95 < 300ms
// =============================================================================

func BenchmarkMatchPattern(b *testing.B) {
	patterns := []string{
		"*.go",
		"internal/**/*.go",
		"/docs/*",
		"**/test/**",
	}
	paths := []string{
		"internal/query/engine.go",
		"docs/README.md",
		"test/fixtures/data.json",
		"cmd/main.go",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchPattern(patterns[i%len(patterns)], paths[i%len(paths)])
	}
}

func BenchmarkGetOwnersForPath(b *testing.B) {
	// Create a CodeownersFile structure with multiple rules
	rules := make([]CodeownersRule, 20)
	for i := 0; i < 20; i++ {
		rules[i] = CodeownersRule{
			Pattern:    "internal/**/*.go",
			Owners:     []string{"@team/backend"},
			LineNumber: i + 1,
			IsNegation: false,
		}
	}
	codeowners := &CodeownersFile{Rules: rules, Path: "CODEOWNERS"}

	paths := []string{
		"internal/query/engine.go",
		"internal/api/handler.go",
		"internal/storage/db.go",
		"cmd/main.go",
		"docs/README.md",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		codeowners.GetOwnersForPath(paths[i%len(paths)])
	}
}

func BenchmarkCodeownersToOwners(b *testing.B) {
	owners := []string{
		"@team/backend",
		"@alice",
		"bob@example.com",
		"@team/frontend",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CodeownersToOwners(owners)
	}
}

// BenchmarkOwnershipResolutionPipeline simulates full ownership resolution
func BenchmarkOwnershipResolutionPipeline(b *testing.B) {
	// Create realistic CODEOWNERS with 50 rules
	rules := make([]CodeownersRule, 50)
	patterns := []string{
		"*.go", "*.ts", "*.py", "*.rs", "*.java",
		"internal/**", "cmd/**", "pkg/**", "api/**", "config/**",
	}
	ownerLists := [][]string{
		{"@team/backend"},
		{"@team/frontend"},
		{"@team/platform"},
		{"@alice", "@bob"},
		{"@team/security"},
	}

	for i := 0; i < 50; i++ {
		rules[i] = CodeownersRule{
			Pattern:    patterns[i%len(patterns)],
			Owners:     ownerLists[i%len(ownerLists)],
			LineNumber: i + 1,
			IsNegation: false,
		}
	}
	codeowners := &CodeownersFile{Rules: rules, Path: "CODEOWNERS"}

	// Simulate resolving ownership for 100 files
	paths := make([]string, 100)
	for i := 0; i < 100; i++ {
		paths[i] = "internal/module/file" + string(rune('0'+i%10)) + ".go"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			owners := codeowners.GetOwnersForPath(path)
			if len(owners) > 0 {
				CodeownersToOwners(owners)
			}
		}
	}
}

func BenchmarkNormalizeAuthorKey(b *testing.B) {
	authors := []struct {
		name  string
		email string
	}{
		{"Alice Smith", "alice@example.com"},
		{"Bob Developer", "bob@example.com"},
		{"Charlie Brown", "charlie@example.com"},
		{"dependabot[bot]", "dependabot@github.com"},
		{"Jane Doe", "jane@example.com"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := authors[i%len(authors)]
		normalizeAuthorKey(a.name, a.email)
	}
}

func BenchmarkIsBot(b *testing.B) {
	authors := []struct {
		name  string
		email string
	}{
		{"dependabot[bot]", "dependabot@github.com"},
		{"github-actions", "actions@github.com"},
		{"renovate", "renovate@example.com"},
		{"Alice Smith", "alice@example.com"},
		{"Bob Developer", "bob@example.com"},
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)bot`),
		regexp.MustCompile(`(?i)dependabot`),
		regexp.MustCompile(`(?i)github-actions`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := authors[i%len(authors)]
		isBot(a.name, a.email, patterns)
	}
}

func BenchmarkBlameOwnershipToOwners(b *testing.B) {
	ownership := &BlameOwnership{
		FilePath:   "internal/query/engine.go",
		TotalLines: 500,
		Contributors: []AuthorContribution{
			{Author: "Alice Smith", Email: "alice@example.com", LineCount: 200, Percentage: 40.0, WeightedLines: 180.0},
			{Author: "Bob Developer", Email: "bob@example.com", LineCount: 150, Percentage: 30.0, WeightedLines: 135.0},
			{Author: "Charlie Brown", Email: "charlie@example.com", LineCount: 100, Percentage: 20.0, WeightedLines: 90.0},
			{Author: "Jane Doe", Email: "jane@example.com", LineCount: 50, Percentage: 10.0, WeightedLines: 45.0},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlameOwnershipToOwners(ownership)
	}
}
