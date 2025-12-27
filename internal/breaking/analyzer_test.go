package breaking

import (
	"testing"
)

func TestDefaultCompareOptions(t *testing.T) {
	opts := DefaultCompareOptions()

	if opts.BaseRef != "HEAD~1" {
		t.Errorf("BaseRef should default to HEAD~1, got %s", opts.BaseRef)
	}
	if opts.TargetRef != "HEAD" {
		t.Errorf("TargetRef should default to HEAD, got %s", opts.TargetRef)
	}
	if opts.IncludeMinor != false {
		t.Error("IncludeMinor should default to false")
	}
	if opts.IgnorePrivate != true {
		t.Error("IgnorePrivate should default to true")
	}
}

func TestCompareResult_HasBreakingChanges(t *testing.T) {
	tests := []struct {
		name     string
		summary  *Summary
		expected bool
	}{
		{
			name:     "nil summary",
			summary:  nil,
			expected: false,
		},
		{
			name: "no breaking changes",
			summary: &Summary{
				TotalChanges:    5,
				BreakingChanges: 0,
				Additions:       5,
			},
			expected: false,
		},
		{
			name: "has breaking changes",
			summary: &Summary{
				TotalChanges:    3,
				BreakingChanges: 2,
				Warnings:        1,
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &CompareResult{Summary: tc.summary}
			if result.HasBreakingChanges() != tc.expected {
				t.Errorf("HasBreakingChanges() = %v, want %v", result.HasBreakingChanges(), tc.expected)
			}
		})
	}
}

func TestCompareSymbolSets(t *testing.T) {
	analyzer := &Analyzer{}

	baseSymbols := []APISymbol{
		{Name: "OldFunc", Kind: "function", Package: "pkg", Exported: true, Signature: "func()"},
		{Name: "StableFunc", Kind: "function", Package: "pkg", Exported: true, Signature: "func(x int)"},
		{Name: "ChangedFunc", Kind: "function", Package: "pkg", Exported: true, Signature: "func(a string)"},
	}

	targetSymbols := []APISymbol{
		{Name: "StableFunc", Kind: "function", Package: "pkg", Exported: true, Signature: "func(x int)"},
		{Name: "ChangedFunc", Kind: "function", Package: "pkg", Exported: true, Signature: "func(a, b string)"}, // signature changed
		{Name: "NewFunc", Kind: "function", Package: "pkg", Exported: true, Signature: "func()"},                // added
	}

	result := analyzer.CompareSymbolSets(baseSymbols, targetSymbols)

	if result.TotalBaseSymbols != 3 {
		t.Errorf("TotalBaseSymbols = %d, want 3", result.TotalBaseSymbols)
	}
	if result.TotalTargetSymbols != 3 {
		t.Errorf("TotalTargetSymbols = %d, want 3", result.TotalTargetSymbols)
	}

	// Should have: 1 removed (OldFunc), 1 signature change (ChangedFunc), 1 added (NewFunc)
	if len(result.Changes) != 3 {
		t.Errorf("Expected 3 changes, got %d", len(result.Changes))
	}

	// Check summary
	if result.Summary == nil {
		t.Fatal("Summary should not be nil")
	}
	if result.Summary.BreakingChanges != 2 { // removed + signature change
		t.Errorf("BreakingChanges = %d, want 2", result.Summary.BreakingChanges)
	}
	if result.Summary.Additions != 1 {
		t.Errorf("Additions = %d, want 1", result.Summary.Additions)
	}
}

func TestSeverityOrder(t *testing.T) {
	if severityOrder(SeverityBreaking) >= severityOrder(SeverityWarning) {
		t.Error("Breaking should have lower order than Warning")
	}
	if severityOrder(SeverityWarning) >= severityOrder(SeverityNonBreaking) {
		t.Error("Warning should have lower order than NonBreaking")
	}
}

func TestExtractNameFromSymbol(t *testing.T) {
	tests := []struct {
		symbolID string
		expected string
	}{
		{"scip-go gomod pkg v1.0.0 internal/query/engine.go/Engine", "Engine"},
		{"scip-go gomod ckb v0.0.0 cmd/ckb/main.go/main", "main"},
		{"", ""},
		{"no/slashes", "slashes"}, // Takes last component after slash
	}

	for _, tc := range tests {
		result := extractNameFromSymbol(tc.symbolID)
		if result != tc.expected {
			t.Errorf("extractNameFromSymbol(%q) = %q, want %q", tc.symbolID, result, tc.expected)
		}
	}
}

func TestIsExported(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Engine", true},
		{"NewEngine", true},
		{"engine", false},
		{"newEngine", false},
		{"", false},
	}

	for _, tc := range tests {
		result := isExported(tc.name)
		if result != tc.expected {
			t.Errorf("isExported(%q) = %v, want %v", tc.name, result, tc.expected)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"internal/query/engine_test.go", true},
		{"internal/query/engine.go", false},
		{"src/app.test.ts", true},
		{"src/app.ts", false},
		{"src/tests/unit/test_main.py", true}, // needs /tests/ not just tests/
		{"src/main.py", false},
	}

	for _, tc := range tests {
		result := isTestFile(tc.path)
		if result != tc.expected {
			t.Errorf("isTestFile(%q) = %v, want %v", tc.path, result, tc.expected)
		}
	}
}

func TestComputeSemverAdvice(t *testing.T) {
	analyzer := &Analyzer{}

	tests := []struct {
		name     string
		summary  *Summary
		expected string
	}{
		{
			name:     "breaking changes = major",
			summary:  &Summary{BreakingChanges: 1},
			expected: "major",
		},
		{
			name:     "additions only = minor",
			summary:  &Summary{Additions: 5},
			expected: "minor",
		},
		{
			name:     "no changes = patch",
			summary:  &Summary{},
			expected: "patch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := analyzer.computeSemverAdvice(tc.summary)
			if result != tc.expected {
				t.Errorf("computeSemverAdvice() = %q, want %q", result, tc.expected)
			}
		})
	}
}
