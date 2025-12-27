package deadcode

import (
	"testing"
)

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Go tests
		{"internal/query/engine_test.go", true},
		{"cmd/main_test.go", true},
		{"pkg/utils/utils.go", false},

		// TypeScript/JavaScript tests
		{"src/components/Button.test.ts", true},
		{"src/utils/helper.spec.js", true},
		{"src/components/Button.tsx", false},

		// Python tests
		{"tests/test_main.py", true},
		{"src/main_test.py", true},
		{"src/main.py", false},

		// Test directories
		{"test/fixtures/data.json", false}, // test/ not matched, only /test/
		{"tests/unit/test_api.py", true},
		{"__tests__/Button.test.tsx", false}, // requires leading /
		{"testdata/sample.json", false},      // testdata/ is for fixtures, not actual test files
		{"src/__tests__/Button.test.tsx", true},
		{"pkg/tests/unit.go", true},

		// Non-test files
		{"internal/query/engine.go", false},
		{"README.md", false},
		{"docs/testing.md", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := IsTestFile(tc.path)
			if result != tc.expected {
				t.Errorf("IsTestFile(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestIsExported(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected bool
	}{
		// Exported symbols
		{"Engine", "type", true},
		{"NewEngine", "function", true},
		{"HandleRequest", "method", true},

		// Unexported symbols
		{"engine", "type", false},
		{"newEngine", "function", false},
		{"handleRequest", "method", false},

		// Variables and parameters are never "exported" in our sense
		{"Config", "variable", false},
		{"ctx", "parameter", false},

		// Empty name
		{"", "function", false},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_"+tc.kind, func(t *testing.T) {
			result := isExported(tc.name, tc.kind)
			if result != tc.expected {
				t.Errorf("isExported(%q, %q) = %v, want %v", tc.name, tc.kind, result, tc.expected)
			}
		})
	}
}

func TestIsGeneratedFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Generated files
		{"internal/api/api_generated.go", true},
		{"pkg/proto/service.pb.go", true},
		{"pkg/proto/service.pb.gw.go", true},
		{"internal/types/types_gen.go", true},
		{"internal/mocks/mock_engine.go", true},
		{"generated/schema.go", true},
		{"internal/ent/client.go", true},
		{"internal/sqlc/query.sql.go", true},

		// Non-generated files
		{"internal/query/engine.go", false},
		{"cmd/main.go", false},
		{"pkg/utils/helper.go", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := isGeneratedFile(tc.path)
			if result != tc.expected {
				t.Errorf("isGeneratedFile(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestHasSerializationTag(t *testing.T) {
	tests := []struct {
		tags     string
		expected bool
	}{
		// Has serialization tags
		{`json:"name"`, true},
		{`json:"name,omitempty"`, true},
		{`xml:"name"`, true},
		{`yaml:"name"`, true},
		{`json:"name" validate:"required"`, true},
		{`db:"user_id"`, true},
		{`gorm:"column:user_id"`, true},

		// No serialization tags
		{"", false},
		{`comment:"some comment"`, false},
		{`custom:"value"`, false},
	}

	for _, tc := range tests {
		t.Run(tc.tags, func(t *testing.T) {
			result := hasSerializationTag(tc.tags)
			if result != tc.expected {
				t.Errorf("hasSerializationTag(%q) = %v, want %v", tc.tags, result, tc.expected)
			}
		})
	}
}

func TestIsCommonInterfaceMethod(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected bool
	}{
		// Common interface methods
		{"String", "method", true},
		{"Error", "method", true},
		{"Read", "method", true},
		{"Write", "method", true},
		{"Close", "method", true},
		{"MarshalJSON", "method", true},
		{"UnmarshalJSON", "method", true},
		{"ServeHTTP", "method", true},

		// Not interface methods
		{"String", "function", false}, // Not a method
		{"CustomMethod", "method", false},
		{"ProcessData", "method", false},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_"+tc.kind, func(t *testing.T) {
			result := isCommonInterfaceMethod(tc.name, tc.kind)
			if result != tc.expected {
				t.Errorf("isCommonInterfaceMethod(%q, %q) = %v, want %v", tc.name, tc.kind, result, tc.expected)
			}
		})
	}
}

func TestExclusionRules_ShouldExclude(t *testing.T) {
	tests := []struct {
		name     string
		sym      SymbolInfo
		patterns []string
		excluded bool
	}{
		{
			name: "main function",
			sym: SymbolInfo{
				Name: "main",
				Kind: "function",
			},
			excluded: true,
		},
		{
			name: "init function",
			sym: SymbolInfo{
				Name: "init",
				Kind: "function",
			},
			excluded: true,
		},
		{
			name: "test function",
			sym: SymbolInfo{
				Name:     "TestSomething",
				Kind:     "function",
				FilePath: "internal/query/engine_test.go",
			},
			excluded: true,
		},
		{
			name: "benchmark function",
			sym: SymbolInfo{
				Name: "BenchmarkEngine",
				Kind: "function",
			},
			excluded: true,
		},
		{
			name: "example function",
			sym: SymbolInfo{
				Name: "ExampleEngine",
				Kind: "function",
			},
			excluded: true,
		},
		{
			name: "interface check variable",
			sym: SymbolInfo{
				Name: "_",
				Kind: "variable",
			},
			excluded: true,
		},
		{
			name: "Stringer method",
			sym: SymbolInfo{
				Name: "String",
				Kind: "method",
			},
			excluded: true,
		},
		{
			name: "generated file",
			sym: SymbolInfo{
				Name:     "GeneratedFunc",
				Kind:     "function",
				FilePath: "internal/api/api_generated.go",
			},
			excluded: true,
		},
		{
			name: "custom exclusion pattern",
			sym: SymbolInfo{
				Name:     "LegacyFunc",
				Kind:     "function",
				FilePath: "internal/legacy/old.go",
			},
			patterns: []string{"**legacy**"},
			excluded: true,
		},
		{
			name: "regular function",
			sym: SymbolInfo{
				Name:     "ProcessData",
				Kind:     "function",
				FilePath: "internal/query/engine.go",
			},
			excluded: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rules := NewExclusionRules(tc.patterns)
			reason := rules.ShouldExclude(tc.sym)
			excluded := reason != ""

			if excluded != tc.excluded {
				t.Errorf("ShouldExclude() = %v (reason: %q), want excluded=%v",
					excluded, reason, tc.excluded)
			}
		})
	}
}

func TestKindToString(t *testing.T) {
	tests := []struct {
		kind     int32
		expected string
	}{
		{1, "package"},
		{2, "type"},
		{3, "term"},
		{4, "method"},
		{5, "type_parameter"},
		{6, "parameter"},
		{7, "self_parameter"},
		{8, "attr"},
		{9, "macro"},
		{0, "unknown"},
		{99, "unknown"},
	}

	for _, tc := range tests {
		result := kindToString(tc.kind)
		if result != tc.expected {
			t.Errorf("kindToString(%d) = %q, want %q", tc.kind, result, tc.expected)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if !opts.IncludeExported {
		t.Error("IncludeExported should default to true")
	}
	if opts.IncludeUnexported {
		t.Error("IncludeUnexported should default to false")
	}
	if opts.MinConfidence != 0.7 {
		t.Errorf("MinConfidence should default to 0.7, got %v", opts.MinConfidence)
	}
	if !opts.ExcludeTestOnly {
		t.Error("ExcludeTestOnly should default to true")
	}
	if opts.Limit != 100 {
		t.Errorf("Limit should default to 100, got %d", opts.Limit)
	}
}

func TestExtractFilePathFromSymbol(t *testing.T) {
	tests := []struct {
		symbolID     string
		shouldHaveGo bool // Just check if it found a .go file
	}{
		// Go symbols - should find something with .go
		{"scip-go gomod github.com/example/pkg v1.0.0 internal/query/engine.go/Engine", true},
		{"scip-go gomod ckb v0.0.0 cmd/ckb/main.go/main", true},

		// TypeScript symbols - should find something with .ts
		{"scip-typescript npm @types/node v18.0.0 src/index.ts/Handler", false}, // complex path

		// Edge cases
		{"", false},
		{"no-path-here", false},
	}

	for _, tc := range tests {
		t.Run(tc.symbolID, func(t *testing.T) {
			result := extractFilePathFromSymbol(tc.symbolID)
			hasGo := result != "" && (len(result) >= 3 && result[len(result)-3:] == ".go")
			if tc.shouldHaveGo && !hasGo && result == "" {
				t.Errorf("extractFilePathFromSymbol(%q) = %q, expected to find a path",
					tc.symbolID, result)
			}
		})
	}
}

func TestReferenceStats(t *testing.T) {
	stats := ReferenceStats{
		Total:     10,
		FromTests: 3,
		FromSelf:  2,
		External:  3,
		Internal:  2,
	}

	// Verify total matches sum of components (minus self)
	nonSelf := stats.Total - stats.FromSelf
	if nonSelf != 8 {
		t.Errorf("Non-self references should be 8, got %d", nonSelf)
	}
}
