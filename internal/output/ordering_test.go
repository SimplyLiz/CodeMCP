package output

import (
	"reflect"
	"testing"
)

func TestSortModules(t *testing.T) {
	tests := []struct {
		name     string
		input    []Module
		expected []Module
	}{
		{
			name: "sort by impact count descending",
			input: []Module{
				{ModuleId: "mod1", ImpactCount: 5, SymbolCount: 10},
				{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 5},
				{ModuleId: "mod3", ImpactCount: 8, SymbolCount: 7},
			},
			expected: []Module{
				{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 5},
				{ModuleId: "mod3", ImpactCount: 8, SymbolCount: 7},
				{ModuleId: "mod1", ImpactCount: 5, SymbolCount: 10},
			},
		},
		{
			name: "sort by symbol count when impact count is equal",
			input: []Module{
				{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 5},
				{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 15},
				{ModuleId: "mod3", ImpactCount: 10, SymbolCount: 10},
			},
			expected: []Module{
				{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 15},
				{ModuleId: "mod3", ImpactCount: 10, SymbolCount: 10},
				{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 5},
			},
		},
		{
			name: "sort by moduleId when impact and symbol count are equal",
			input: []Module{
				{ModuleId: "mod3", ImpactCount: 10, SymbolCount: 10},
				{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 10},
				{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 10},
			},
			expected: []Module{
				{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 10},
				{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 10},
				{ModuleId: "mod3", ImpactCount: 10, SymbolCount: 10},
			},
		},
		{
			name:     "empty slice",
			input:    []Module{},
			expected: []Module{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortModules(tt.input)
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("SortModules() = %v, want %v", tt.input, tt.expected)
			}
		})
	}
}

func TestSortSymbols(t *testing.T) {
	tests := []struct {
		name     string
		input    []Symbol
		expected []Symbol
	}{
		{
			name: "sort by confidence descending",
			input: []Symbol{
				{StableId: "sym1", Confidence: 0.5, RefCount: 10},
				{StableId: "sym2", Confidence: 0.9, RefCount: 5},
				{StableId: "sym3", Confidence: 0.7, RefCount: 7},
			},
			expected: []Symbol{
				{StableId: "sym2", Confidence: 0.9, RefCount: 5},
				{StableId: "sym3", Confidence: 0.7, RefCount: 7},
				{StableId: "sym1", Confidence: 0.5, RefCount: 10},
			},
		},
		{
			name: "sort by refCount when confidence is equal",
			input: []Symbol{
				{StableId: "sym1", Confidence: 0.9, RefCount: 5},
				{StableId: "sym2", Confidence: 0.9, RefCount: 15},
				{StableId: "sym3", Confidence: 0.9, RefCount: 10},
			},
			expected: []Symbol{
				{StableId: "sym2", Confidence: 0.9, RefCount: 15},
				{StableId: "sym3", Confidence: 0.9, RefCount: 10},
				{StableId: "sym1", Confidence: 0.9, RefCount: 5},
			},
		},
		{
			name: "sort by stableId when confidence and refCount are equal",
			input: []Symbol{
				{StableId: "sym3", Confidence: 0.9, RefCount: 10},
				{StableId: "sym1", Confidence: 0.9, RefCount: 10},
				{StableId: "sym2", Confidence: 0.9, RefCount: 10},
			},
			expected: []Symbol{
				{StableId: "sym1", Confidence: 0.9, RefCount: 10},
				{StableId: "sym2", Confidence: 0.9, RefCount: 10},
				{StableId: "sym3", Confidence: 0.9, RefCount: 10},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortSymbols(tt.input)
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("SortSymbols() = %v, want %v", tt.input, tt.expected)
			}
		})
	}
}

func TestSortReferences(t *testing.T) {
	tests := []struct {
		name     string
		input    []Reference
		expected []Reference
	}{
		{
			name: "sort by fileId ascending",
			input: []Reference{
				{FileId: "file2", StartLine: 10, StartColumn: 5},
				{FileId: "file1", StartLine: 20, StartColumn: 3},
				{FileId: "file3", StartLine: 15, StartColumn: 8},
			},
			expected: []Reference{
				{FileId: "file1", StartLine: 20, StartColumn: 3},
				{FileId: "file2", StartLine: 10, StartColumn: 5},
				{FileId: "file3", StartLine: 15, StartColumn: 8},
			},
		},
		{
			name: "sort by startLine when fileId is equal",
			input: []Reference{
				{FileId: "file1", StartLine: 20, StartColumn: 5},
				{FileId: "file1", StartLine: 10, StartColumn: 3},
				{FileId: "file1", StartLine: 15, StartColumn: 8},
			},
			expected: []Reference{
				{FileId: "file1", StartLine: 10, StartColumn: 3},
				{FileId: "file1", StartLine: 15, StartColumn: 8},
				{FileId: "file1", StartLine: 20, StartColumn: 5},
			},
		},
		{
			name: "sort by startColumn when fileId and startLine are equal",
			input: []Reference{
				{FileId: "file1", StartLine: 10, StartColumn: 8},
				{FileId: "file1", StartLine: 10, StartColumn: 3},
				{FileId: "file1", StartLine: 10, StartColumn: 5},
			},
			expected: []Reference{
				{FileId: "file1", StartLine: 10, StartColumn: 3},
				{FileId: "file1", StartLine: 10, StartColumn: 5},
				{FileId: "file1", StartLine: 10, StartColumn: 8},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortReferences(tt.input)
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("SortReferences() = %v, want %v", tt.input, tt.expected)
			}
		})
	}
}

func TestSortImpactItems(t *testing.T) {
	tests := []struct {
		name     string
		input    []ImpactItem
		expected []ImpactItem
	}{
		{
			name: "sort by kind priority",
			input: []ImpactItem{
				{StableId: "item1", Kind: "test-dependency", Confidence: 0.9},
				{StableId: "item2", Kind: "direct-caller", Confidence: 0.8},
				{StableId: "item3", Kind: "type-dependency", Confidence: 0.7},
			},
			expected: []ImpactItem{
				{StableId: "item2", Kind: "direct-caller", Confidence: 0.8},
				{StableId: "item3", Kind: "type-dependency", Confidence: 0.7},
				{StableId: "item1", Kind: "test-dependency", Confidence: 0.9},
			},
		},
		{
			name: "sort by confidence when kind is equal",
			input: []ImpactItem{
				{StableId: "item1", Kind: "direct-caller", Confidence: 0.7},
				{StableId: "item2", Kind: "direct-caller", Confidence: 0.9},
				{StableId: "item3", Kind: "direct-caller", Confidence: 0.8},
			},
			expected: []ImpactItem{
				{StableId: "item2", Kind: "direct-caller", Confidence: 0.9},
				{StableId: "item3", Kind: "direct-caller", Confidence: 0.8},
				{StableId: "item1", Kind: "direct-caller", Confidence: 0.7},
			},
		},
		{
			name: "sort by stableId when kind and confidence are equal",
			input: []ImpactItem{
				{StableId: "item3", Kind: "direct-caller", Confidence: 0.9},
				{StableId: "item1", Kind: "direct-caller", Confidence: 0.9},
				{StableId: "item2", Kind: "direct-caller", Confidence: 0.9},
			},
			expected: []ImpactItem{
				{StableId: "item1", Kind: "direct-caller", Confidence: 0.9},
				{StableId: "item2", Kind: "direct-caller", Confidence: 0.9},
				{StableId: "item3", Kind: "direct-caller", Confidence: 0.9},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortImpactItems(tt.input)
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("SortImpactItems() = %v, want %v", tt.input, tt.expected)
			}
		})
	}
}

func TestSortDrilldowns(t *testing.T) {
	tests := []struct {
		name     string
		input    []Drilldown
		expected []Drilldown
	}{
		{
			name: "sort by relevanceScore descending",
			input: []Drilldown{
				{Label: "drill1", Query: "q1", RelevanceScore: 0.5},
				{Label: "drill2", Query: "q2", RelevanceScore: 0.9},
				{Label: "drill3", Query: "q3", RelevanceScore: 0.7},
			},
			expected: []Drilldown{
				{Label: "drill2", Query: "q2", RelevanceScore: 0.9},
				{Label: "drill3", Query: "q3", RelevanceScore: 0.7},
				{Label: "drill1", Query: "q1", RelevanceScore: 0.5},
			},
		},
		{
			name: "sort by label when relevanceScore is equal",
			input: []Drilldown{
				{Label: "drill3", Query: "q3", RelevanceScore: 0.9},
				{Label: "drill1", Query: "q1", RelevanceScore: 0.9},
				{Label: "drill2", Query: "q2", RelevanceScore: 0.9},
			},
			expected: []Drilldown{
				{Label: "drill1", Query: "q1", RelevanceScore: 0.9},
				{Label: "drill2", Query: "q2", RelevanceScore: 0.9},
				{Label: "drill3", Query: "q3", RelevanceScore: 0.9},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortDrilldowns(tt.input)
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("SortDrilldowns() = %v, want %v", tt.input, tt.expected)
			}
		})
	}
}

func TestSortWarnings(t *testing.T) {
	tests := []struct {
		name     string
		input    []Warning
		expected []Warning
	}{
		{
			name: "sort by severity descending (by priority)",
			input: []Warning{
				{Severity: "info", Text: "text1"},
				{Severity: "error", Text: "text2"},
				{Severity: "warning", Text: "text3"},
			},
			expected: []Warning{
				{Severity: "error", Text: "text2"},
				{Severity: "warning", Text: "text3"},
				{Severity: "info", Text: "text1"},
			},
		},
		{
			name: "sort by text when severity is equal",
			input: []Warning{
				{Severity: "error", Text: "text3"},
				{Severity: "error", Text: "text1"},
				{Severity: "error", Text: "text2"},
			},
			expected: []Warning{
				{Severity: "error", Text: "text1"},
				{Severity: "error", Text: "text2"},
				{Severity: "error", Text: "text3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortWarnings(tt.input)
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("SortWarnings() = %v, want %v", tt.input, tt.expected)
			}
		})
	}
}

func TestSortingStability(t *testing.T) {
	// Test that sorting is stable - equal elements maintain their relative order
	t.Run("modules with equal impact and symbol counts", func(t *testing.T) {
		modules := []Module{
			{ModuleId: "mod1", Name: "first", ImpactCount: 10, SymbolCount: 10},
			{ModuleId: "mod2", Name: "second", ImpactCount: 10, SymbolCount: 10},
			{ModuleId: "mod3", Name: "third", ImpactCount: 10, SymbolCount: 10},
		}

		// Sort twice and ensure consistent results
		SortModules(modules)
		first := make([]Module, len(modules))
		copy(first, modules)

		SortModules(modules)
		second := make([]Module, len(modules))
		copy(second, modules)

		if !reflect.DeepEqual(first, second) {
			t.Errorf("Sorting is not stable: first=%v, second=%v", first, second)
		}
	})
}

func TestDeterministicSorting(t *testing.T) {
	// Test that sorting produces identical results across multiple runs
	modules := []Module{
		{ModuleId: "mod5", ImpactCount: 3, SymbolCount: 8},
		{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 5},
		{ModuleId: "mod3", ImpactCount: 7, SymbolCount: 12},
		{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 15},
		{ModuleId: "mod4", ImpactCount: 7, SymbolCount: 9},
	}

	// Run sorting 10 times
	var results [][]Module
	for i := 0; i < 10; i++ {
		test := make([]Module, len(modules))
		copy(test, modules)
		SortModules(test)
		results = append(results, test)
	}

	// All results should be identical
	for i := 1; i < len(results); i++ {
		if !reflect.DeepEqual(results[0], results[i]) {
			t.Errorf("Sorting is not deterministic: run 0=%v, run %d=%v", results[0], i, results[i])
		}
	}
}
