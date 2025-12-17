package query

import (
	"testing"

	"ckb/internal/backends/git"
)

func TestComputeJustifyVerdict(t *testing.T) {
	t.Run("keeps symbol with active callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Usage: &ExplainUsage{CallerCount: 5},
		}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "keep" {
			t.Errorf("expected verdict 'keep', got %q", verdict)
		}
		if confidence != 0.9 {
			t.Errorf("expected confidence 0.9, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("investigates public API with no callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Usage: &ExplainUsage{CallerCount: 0},
			Flags: &ExplainSymbolFlags{IsPublicApi: true},
		}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "investigate" {
			t.Errorf("expected verdict 'investigate', got %q", verdict)
		}
		if confidence != 0.6 {
			t.Errorf("expected confidence 0.6, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("removes private symbol with no callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Usage: &ExplainUsage{CallerCount: 0},
			Flags: &ExplainSymbolFlags{IsPublicApi: false},
		}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "remove-candidate" {
			t.Errorf("expected verdict 'remove-candidate', got %q", verdict)
		}
		if confidence != 0.7 {
			t.Errorf("expected confidence 0.7, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("removes when no usage info available", func(t *testing.T) {
		facts := ExplainSymbolFacts{}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "remove-candidate" {
			t.Errorf("expected verdict 'remove-candidate', got %q", verdict)
		}
		if confidence != 0.7 {
			t.Errorf("expected confidence 0.7, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})
}

func TestClassifyCommitFrequency(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "unknown"},
		{1, "stable"},
		{10, "stable"},
		{11, "moderate"},
		{50, "moderate"},
		{51, "volatile"},
		{100, "volatile"},
	}

	for _, tc := range tests {
		result := classifyCommitFrequency(tc.count)
		if result != tc.expected {
			t.Errorf("classifyCommitFrequency(%d) = %q, expected %q", tc.count, result, tc.expected)
		}
	}
}

func TestTopLevelModule(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"internal/query/engine.go", "internal"},
		{"./internal/query/engine.go", "internal"},
		{"cmd/ckb/main.go", "cmd"},
		{"main.go", "main.go"},
		{"", ""},
	}

	for _, tc := range tests {
		result := topLevelModule(tc.path)
		if result != tc.expected {
			t.Errorf("topLevelModule(%q) = %q, expected %q", tc.path, result, tc.expected)
		}
	}
}

func TestBuildExplainSummary(t *testing.T) {
	t.Run("builds complete summary", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Symbol: &SymbolInfo{
				Name:     "MyFunction",
				Kind:     "function",
				ModuleId: "internal/query",
			},
			Usage: &ExplainUsage{
				CallerCount:    5,
				ReferenceCount: 10,
				ModuleCount:    3,
			},
			History: &ExplainHistory{
				CommitCount:    15,
				LastModifiedAt: "2024-01-15",
			},
		}

		summary := buildExplainSummary(facts)

		if summary.Identity == "" {
			t.Error("expected non-empty identity")
		}
		if summary.Usage == "" {
			t.Error("expected non-empty usage")
		}
		if summary.History == "" {
			t.Error("expected non-empty history")
		}
		if summary.Tldr == "" {
			t.Error("expected non-empty tldr")
		}
	})

	t.Run("handles empty facts", func(t *testing.T) {
		facts := ExplainSymbolFacts{}
		summary := buildExplainSummary(facts)

		if summary.Tldr != "" {
			t.Errorf("expected empty tldr for empty facts, got %q", summary.Tldr)
		}
	})
}

// =============================================================================
// Phase 2: summarizeDiff Tests
// =============================================================================

func TestClassifyFileRiskLevel(t *testing.T) {
	tests := []struct {
		name     string
		stat     git.DiffStats
		role     string
		expected string
	}{
		{
			name:     "deleted file is high risk",
			stat:     git.DiffStats{FilePath: "foo.go", IsDeleted: true},
			role:     "core",
			expected: "high",
		},
		{
			name:     "new file is low risk",
			stat:     git.DiffStats{FilePath: "foo.go", IsNew: true},
			role:     "core",
			expected: "low",
		},
		{
			name:     "core file with large changes is high risk",
			stat:     git.DiffStats{FilePath: "foo.go", Additions: 80, Deletions: 30},
			role:     "core",
			expected: "high",
		},
		{
			name:     "core file with medium changes is medium risk",
			stat:     git.DiffStats{FilePath: "foo.go", Additions: 20, Deletions: 15},
			role:     "core",
			expected: "medium",
		},
		{
			name:     "config change is medium risk",
			stat:     git.DiffStats{FilePath: "config.json", Additions: 5, Deletions: 2},
			role:     "config",
			expected: "medium",
		},
		{
			name:     "test change is low risk",
			stat:     git.DiffStats{FilePath: "foo_test.go", Additions: 50, Deletions: 20},
			role:     "test",
			expected: "low",
		},
		{
			name:     "very large change is high risk",
			stat:     git.DiffStats{FilePath: "foo.go", Additions: 150, Deletions: 100},
			role:     "unknown",
			expected: "high",
		},
		{
			name:     "small change is low risk",
			stat:     git.DiffStats{FilePath: "foo.go", Additions: 10, Deletions: 5},
			role:     "unknown",
			expected: "low",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyFileRiskLevel(tc.stat, tc.role)
			if result != tc.expected {
				t.Errorf("classifyFileRiskLevel(%+v, %q) = %q, expected %q",
					tc.stat, tc.role, result, tc.expected)
			}
		})
	}
}

func TestSuggestTestPath(t *testing.T) {
	tests := []struct {
		filePath string
		language string
		expected string
	}{
		{"internal/query/engine.go", "go", "internal/query/engine_test.go"},
		{"src/utils.ts", "typescript", "src/utils.test.ts"},
		{"src/utils.js", "javascript", "src/utils.test.js"},
		{"lib/helper.py", "python", "lib/test_helper.py"},
		{"foo.rs", "rust", ""},
	}

	for _, tc := range tests {
		t.Run(tc.filePath, func(t *testing.T) {
			result := suggestTestPath(tc.filePath, tc.language)
			if result != tc.expected {
				t.Errorf("suggestTestPath(%q, %q) = %q, expected %q",
					tc.filePath, tc.language, result, tc.expected)
			}
		})
	}
}

func TestBuildDiffSummary(t *testing.T) {
	t.Run("builds summary with changes", func(t *testing.T) {
		files := []DiffFileChange{
			{FilePath: "a.go", ChangeType: "added"},
			{FilePath: "b.go", ChangeType: "modified"},
			{FilePath: "c.go", ChangeType: "modified"},
			{FilePath: "d.go", ChangeType: "deleted"},
		}
		commits := []DiffCommitInfo{
			{Hash: "abc123"},
			{Hash: "def456"},
		}

		summary := buildDiffSummary(files, nil, nil, commits)

		if summary.OneLiner == "" {
			t.Error("expected non-empty one-liner")
		}
		if len(summary.KeyChanges) == 0 {
			t.Error("expected key changes")
		}
	})

	t.Run("handles empty files", func(t *testing.T) {
		summary := buildDiffSummary(nil, nil, nil, nil)
		if summary.OneLiner != "0 files changed" {
			t.Errorf("expected '0 files changed', got %q", summary.OneLiner)
		}
	})
}

func TestComputeDiffConfidence(t *testing.T) {
	t.Run("git and scip available", func(t *testing.T) {
		basis := []ConfidenceBasisItem{
			{Backend: "git", Status: "available"},
			{Backend: "scip", Status: "available"},
		}
		conf := computeDiffConfidence(basis, nil)
		if conf != 0.89 {
			t.Errorf("expected 0.89, got %f", conf)
		}
	})

	t.Run("git only", func(t *testing.T) {
		basis := []ConfidenceBasisItem{
			{Backend: "git", Status: "available"},
			{Backend: "scip", Status: "missing"},
		}
		conf := computeDiffConfidence(basis, nil)
		if conf != 0.69 {
			t.Errorf("expected 0.69, got %f", conf)
		}
	})

	t.Run("git unavailable", func(t *testing.T) {
		basis := []ConfidenceBasisItem{
			{Backend: "git", Status: "missing"},
		}
		conf := computeDiffConfidence(basis, nil)
		if conf != 0.39 {
			t.Errorf("expected 0.39, got %f", conf)
		}
	})

	t.Run("with limitations", func(t *testing.T) {
		basis := []ConfidenceBasisItem{
			{Backend: "git", Status: "available"},
			{Backend: "scip", Status: "available"},
		}
		limitations := []string{"truncated"}
		conf := computeDiffConfidence(basis, limitations)
		if conf != 0.79 {
			t.Errorf("expected 0.79, got %f", conf)
		}
	})
}

// =============================================================================
// Phase 3: getHotspots Tests
// =============================================================================

func TestClassifyRecency(t *testing.T) {
	tests := []struct {
		name         string
		lastModified string
		expected     string
	}{
		{"empty timestamp", "", "stale"},
		{"invalid timestamp", "not-a-date", "stale"},
		// Note: actual date tests would need time mocking
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyRecency(tc.lastModified)
			if result != tc.expected {
				t.Errorf("classifyRecency(%q) = %q, expected %q",
					tc.lastModified, result, tc.expected)
			}
		})
	}
}

func TestClassifyHotspotRisk(t *testing.T) {
	tests := []struct {
		name     string
		churn    git.ChurnMetrics
		role     string
		expected string
	}{
		{
			name:     "high churn core file",
			churn:    git.ChurnMetrics{ChangeCount: 25, AuthorCount: 2},
			role:     "core",
			expected: "high",
		},
		{
			name:     "many authors",
			churn:    git.ChurnMetrics{ChangeCount: 5, AuthorCount: 6},
			role:     "unknown",
			expected: "high",
		},
		{
			name:     "very high churn",
			churn:    git.ChurnMetrics{ChangeCount: 35, AuthorCount: 2},
			role:     "unknown",
			expected: "high",
		},
		{
			name:     "moderate churn",
			churn:    git.ChurnMetrics{ChangeCount: 15, AuthorCount: 2},
			role:     "unknown",
			expected: "medium",
		},
		{
			name:     "low churn",
			churn:    git.ChurnMetrics{ChangeCount: 5, AuthorCount: 1},
			role:     "unknown",
			expected: "low",
		},
		{
			name:     "test file with high churn",
			churn:    git.ChurnMetrics{ChangeCount: 20, AuthorCount: 2},
			role:     "test",
			expected: "medium",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyHotspotRisk(tc.churn, tc.role)
			if result != tc.expected {
				t.Errorf("classifyHotspotRisk(%+v, %q) = %q, expected %q",
					tc.churn, tc.role, result, tc.expected)
			}
		})
	}
}

// =============================================================================
// Phase 4: explainPath Tests
// =============================================================================

func TestClassifyPathRole(t *testing.T) {
	tests := []struct {
		path         string
		expectedRole string
	}{
		// Test files (note: /test/ and /tests/ and /__tests__/ need slashes)
		{"internal/query/engine_test.go", "test-only"},
		{"src/utils.test.ts", "test-only"},
		{"app/test/helper.go", "test-only"},
		{"app/__tests__/app.js", "test-only"},

		// Config files (any path containing "config", ".json", etc.)
		{"config.json", "config"},
		{"settings.yaml", "config"},
		{".eslintrc.js", "config"},
		{"tsconfig.json", "config"},

		// Documentation
		{"README.md", "unknown"},
		{"app/docs/guide.txt", "unknown"},

		// Vendor (needs /vendor/ or /node_modules/ with slashes)
		{"app/vendor/github.com/foo/bar.go", "unknown"},
		{"app/node_modules/lodash/index.js", "unknown"},

		// Glue/integration (handler, middleware, routes, etc.)
		{"internal/api/handler.go", "glue"},
		{"src/middleware/auth.ts", "glue"},
		{"routes/api.go", "glue"},

		// Legacy
		{"legacy/old_code.go", "legacy"},
		{"deprecated/helper.js", "legacy"},

		// Entry points (note: /cmd/ needs slash, or main.go suffix, or index.ts/js)
		{"app/cmd/server/main.go", "core"},
		{"src/index.ts", "core"},

		// Core (needs /internal/, /src/, /lib/, /pkg/, /core/, /domain/, /services/)
		{"app/internal/query/engine.go", "core"},
		{"app/src/services/user.ts", "core"},
		{"app/lib/utils.py", "core"},

		// Unknown
		{"random.go", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			role, _, basis := classifyPathRole(tc.path)
			if role != tc.expectedRole {
				t.Errorf("classifyPathRole(%q) = %q, expected %q", tc.path, role, tc.expectedRole)
			}
			if len(basis) == 0 {
				t.Error("expected non-empty classification basis")
			}
		})
	}
}

func TestComputePathConfidence(t *testing.T) {
	tests := []struct {
		name     string
		basis    []ClassificationBasis
		expected float64
	}{
		{
			name:     "empty basis",
			basis:    nil,
			expected: 0.5,
		},
		{
			name: "high confidence naming",
			basis: []ClassificationBasis{
				{Type: "naming", Signal: "test pattern", Confidence: 0.95},
			},
			expected: 0.79, // Capped at 0.79
		},
		{
			name: "low confidence",
			basis: []ClassificationBasis{
				{Type: "naming", Signal: "no match", Confidence: 0.5},
			},
			expected: 0.5,
		},
		{
			name: "multiple basis items",
			basis: []ClassificationBasis{
				{Type: "naming", Confidence: 0.6},
				{Type: "location", Confidence: 0.75},
			},
			expected: 0.75,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := computePathConfidence(tc.basis)
			if result != tc.expected {
				t.Errorf("computePathConfidence(%+v) = %f, expected %f",
					tc.basis, result, tc.expected)
			}
		})
	}
}

// =============================================================================
// Phase 4: listKeyConcepts Tests
// =============================================================================

func TestExtractConcept(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		// Skip common names
		{"main", ""},
		{"init", ""},
		{"get", ""},
		{"test", ""},

		// Extract from simple names
		{"User", "User"},
		{"Cache", "Cache"},

		// Extract from compound names (skip suffixes)
		{"UserService", "User"},
		{"CacheManager", "Cache"},
		{"AuthHandler", "Auth"},
		{"ConfigProvider", "Config"},

		// Short words are skipped
		{"Do", ""},
		{"AB", ""},

		// All suffixes - use first
		{"ServiceHandler", "Service"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractConcept(tc.name)
			if result != tc.expected {
				t.Errorf("extractConcept(%q) = %q, expected %q", tc.name, result, tc.expected)
			}
		})
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"user", []string{"user"}},
		{"User", []string{"User"}},
		{"UserService", []string{"User", "Service"}},
		{"HTTPHandler", []string{"H", "T", "T", "P", "Handler"}},
		{"getUser", []string{"get", "User"}},
		{"XMLParser", []string{"X", "M", "L", "Parser"}},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := splitCamelCase(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("splitCamelCase(%q) = %v, expected %v", tc.input, result, tc.expected)
				return
			}
			for i := range result {
				if result[i] != tc.expected[i] {
					t.Errorf("splitCamelCase(%q)[%d] = %q, expected %q",
						tc.input, i, result[i], tc.expected[i])
				}
			}
		})
	}
}

func TestCategorizeConceptV52(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		// Technical
		{"Cache", "technical"},
		{"Queue", "technical"},
		{"Database", "technical"},
		{"HTTP", "technical"},

		// Pattern
		{"Factory", "pattern"},
		{"Builder", "pattern"},
		{"Adapter", "pattern"},
		{"Observer", "pattern"},

		// Domain (default)
		{"User", "domain"},
		{"Order", "domain"},
		{"Payment", "domain"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := categorizeConceptV52(tc.name)
			if result != tc.expected {
				t.Errorf("categorizeConceptV52(%q) = %q, expected %q", tc.name, result, tc.expected)
			}
		})
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "A"},
		{"hello", "Hello"},
		{"HELLO", "HELLO"},
		{"hELLO", "HELLO"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := titleCase(tc.input)
			if result != tc.expected {
				t.Errorf("titleCase(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

// =============================================================================
// Phase 4: recentlyRelevant Tests
// =============================================================================

func TestComputeRecencyScore(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		expected  float64
	}{
		{"empty timestamp", "", 0},
		{"invalid timestamp", "not-a-date", 0},
		// Note: actual date tests would need time mocking or very old dates
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := computeRecencyScore(tc.timestamp)
			if result != tc.expected {
				t.Errorf("computeRecencyScore(%q) = %f, expected %f",
					tc.timestamp, result, tc.expected)
			}
		})
	}
}

// =============================================================================
// File Role Classification Tests (shared helper)
// =============================================================================

func TestClassifyFileRole(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		// Test files (note: function checks for /test/ and /tests/ with slashes)
		{"foo_test.go", "test"},
		{"foo.test.ts", "test"},
		{"app/test/helper.go", "test"},
		{"app/tests/unit.py", "test"},

		// Config files
		{"config.json", "config"},
		{"settings.yaml", "config"},

		// Docs
		{"README.md", "unknown"},
		{"app/docs/guide.md", "unknown"},

		// Vendor (needs slash prefix: /vendor/, /node_modules/)
		{"app/vendor/lib.go", "unknown"},
		{"app/node_modules/pkg/index.js", "unknown"},

		// Entry points
		{"app/cmd/main.go", "entrypoint"},
		{"main.go", "entrypoint"},
		{"src/index.ts", "entrypoint"},

		// Core (needs slash prefix: /internal/, /pkg/, /lib/, /src/)
		{"app/internal/query/engine.go", "core"},
		{"app/pkg/utils/helper.go", "core"},
		{"app/lib/auth.py", "core"},
		{"app/src/services/user.ts", "core"},

		// Unknown
		{"random.txt", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := classifyFileRole(tc.path)
			if result != tc.expected {
				t.Errorf("classifyFileRole(%q) = %q, expected %q", tc.path, result, tc.expected)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"file.go", "go"},
		{"file.ts", "typescript"},
		{"file.tsx", "typescript"},
		{"file.js", "javascript"},
		{"file.jsx", "javascript"},
		{"file.py", "python"},
		{"file.rs", "rust"},
		{"file.java", "java"},
		{"file.rb", "ruby"},
		{"file.swift", "swift"},
		{"file.kt", "kotlin"},
		{"file.c", "c"},
		{"file.cpp", "cpp"},
		{"file.cs", "csharp"},
		{"file.php", "php"},
		{"file.txt", ""},
		{"file", ""},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := detectLanguage(tc.path)
			if result != tc.expected {
				t.Errorf("detectLanguage(%q) = %q, expected %q", tc.path, result, tc.expected)
			}
		})
	}
}
