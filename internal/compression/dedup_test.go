package compression

import (
	"testing"

	"ckb/internal/output"
)

func TestDeduplicateReferences(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := DeduplicateReferences([]output.Reference{})
		if len(result) != 0 {
			t.Errorf("DeduplicateReferences(empty) = %d refs, want 0", len(result))
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		refs := []output.Reference{
			{FileId: "a.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15},
			{FileId: "b.go", StartLine: 20, StartColumn: 3, EndLine: 20, EndColumn: 10},
		}
		result := DeduplicateReferences(refs)
		if len(result) != 2 {
			t.Errorf("DeduplicateReferences(no dups) = %d refs, want 2", len(result))
		}
	})

	t.Run("removes duplicates", func(t *testing.T) {
		refs := []output.Reference{
			{FileId: "a.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15},
			{FileId: "a.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15}, // duplicate
			{FileId: "b.go", StartLine: 20, StartColumn: 3, EndLine: 20, EndColumn: 10},
			{FileId: "a.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15}, // another duplicate
		}
		result := DeduplicateReferences(refs)
		if len(result) != 2 {
			t.Errorf("DeduplicateReferences(with dups) = %d refs, want 2", len(result))
		}
	})

	t.Run("preserves order (first occurrence)", func(t *testing.T) {
		refs := []output.Reference{
			{FileId: "first.go", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 5},
			{FileId: "second.go", StartLine: 2, StartColumn: 2, EndLine: 2, EndColumn: 6},
			{FileId: "first.go", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 5}, // duplicate
		}
		result := DeduplicateReferences(refs)
		if result[0].FileId != "first.go" {
			t.Errorf("first ref = %q, want first.go", result[0].FileId)
		}
		if result[1].FileId != "second.go" {
			t.Errorf("second ref = %q, want second.go", result[1].FileId)
		}
	})

	t.Run("different positions are not duplicates", func(t *testing.T) {
		refs := []output.Reference{
			{FileId: "a.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15},
			{FileId: "a.go", StartLine: 10, StartColumn: 6, EndLine: 10, EndColumn: 16}, // different column
			{FileId: "a.go", StartLine: 11, StartColumn: 5, EndLine: 11, EndColumn: 15}, // different line
		}
		result := DeduplicateReferences(refs)
		if len(result) != 3 {
			t.Errorf("DeduplicateReferences(different positions) = %d refs, want 3", len(result))
		}
	})
}

func TestDeduplicateSymbols(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := DeduplicateSymbols([]output.Symbol{})
		if len(result) != 0 {
			t.Errorf("DeduplicateSymbols(empty) = %d symbols, want 0", len(result))
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		symbols := []output.Symbol{
			{StableId: "sym:a", Name: "FunctionA"},
			{StableId: "sym:b", Name: "FunctionB"},
		}
		result := DeduplicateSymbols(symbols)
		if len(result) != 2 {
			t.Errorf("DeduplicateSymbols(no dups) = %d symbols, want 2", len(result))
		}
	})

	t.Run("removes duplicates by stableId", func(t *testing.T) {
		symbols := []output.Symbol{
			{StableId: "sym:a", Name: "FunctionA"},
			{StableId: "sym:a", Name: "FunctionA-dup"}, // same stableId
			{StableId: "sym:b", Name: "FunctionB"},
		}
		result := DeduplicateSymbols(symbols)
		if len(result) != 2 {
			t.Errorf("DeduplicateSymbols(with dups) = %d symbols, want 2", len(result))
		}
	})

	t.Run("keeps first occurrence", func(t *testing.T) {
		symbols := []output.Symbol{
			{StableId: "sym:a", Name: "First"},
			{StableId: "sym:a", Name: "Second"},
		}
		result := DeduplicateSymbols(symbols)
		if len(result) != 1 {
			t.Fatalf("expected 1 symbol, got %d", len(result))
		}
		if result[0].Name != "First" {
			t.Errorf("kept symbol Name = %q, want First", result[0].Name)
		}
	})
}

func TestDeduplicateModules(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := DeduplicateModules([]output.Module{})
		if len(result) != 0 {
			t.Errorf("DeduplicateModules(empty) = %d modules, want 0", len(result))
		}
	})

	t.Run("removes duplicates by moduleId", func(t *testing.T) {
		modules := []output.Module{
			{ModuleId: "mod:a", Name: "module-a"},
			{ModuleId: "mod:a", Name: "module-a-dup"}, // same moduleId
			{ModuleId: "mod:b", Name: "module-b"},
		}
		result := DeduplicateModules(modules)
		if len(result) != 2 {
			t.Errorf("DeduplicateModules(with dups) = %d modules, want 2", len(result))
		}
	})
}

func TestDeduplicateImpactItems(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := DeduplicateImpactItems([]output.ImpactItem{})
		if len(result) != 0 {
			t.Errorf("DeduplicateImpactItems(empty) = %d items, want 0", len(result))
		}
	})

	t.Run("removes duplicates by stableId", func(t *testing.T) {
		items := []output.ImpactItem{
			{StableId: "sym:a", Name: "SymbolA"},
			{StableId: "sym:a", Name: "SymbolA-dup"}, // same stableId
			{StableId: "sym:b", Name: "SymbolB"},
		}
		result := DeduplicateImpactItems(items)
		if len(result) != 2 {
			t.Errorf("DeduplicateImpactItems(with dups) = %d items, want 2", len(result))
		}
	})

	t.Run("keeps first occurrence", func(t *testing.T) {
		items := []output.ImpactItem{
			{StableId: "sym:x", Name: "First", Confidence: 0.9},
			{StableId: "sym:x", Name: "Second", Confidence: 0.8},
		}
		result := DeduplicateImpactItems(items)
		if len(result) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result))
		}
		if result[0].Name != "First" {
			t.Errorf("kept item Name = %q, want First", result[0].Name)
		}
	})
}
