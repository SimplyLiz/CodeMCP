package compression

import (
	"fmt"

	"ckb/internal/output"
)

// DeduplicateReferences removes duplicate references by location
// Two references are considered duplicates if they point to the same file and location
func DeduplicateReferences(refs []output.Reference) []output.Reference {
	if len(refs) == 0 {
		return refs
	}

	seen := make(map[string]bool)
	result := make([]output.Reference, 0, len(refs))

	for _, ref := range refs {
		// Create a unique key based on file and location
		key := fmt.Sprintf("%s:%d:%d:%d:%d",
			ref.FileId,
			ref.StartLine,
			ref.StartColumn,
			ref.EndLine,
			ref.EndColumn,
		)

		if !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}

	return result
}

// DeduplicateSymbols removes duplicate symbols by stableId
// The first occurrence of each symbol is kept
func DeduplicateSymbols(symbols []output.Symbol) []output.Symbol {
	if len(symbols) == 0 {
		return symbols
	}

	seen := make(map[string]bool)
	result := make([]output.Symbol, 0, len(symbols))

	for _, symbol := range symbols {
		if !seen[symbol.StableId] {
			seen[symbol.StableId] = true
			result = append(result, symbol)
		}
	}

	return result
}

// DeduplicateModules removes duplicate modules by moduleId
// The first occurrence of each module is kept
func DeduplicateModules(modules []output.Module) []output.Module {
	if len(modules) == 0 {
		return modules
	}

	seen := make(map[string]bool)
	result := make([]output.Module, 0, len(modules))

	for _, module := range modules {
		if !seen[module.ModuleId] {
			seen[module.ModuleId] = true
			result = append(result, module)
		}
	}

	return result
}

// DeduplicateImpactItems removes duplicate impact items by stableId
// The first occurrence of each item is kept
func DeduplicateImpactItems(items []output.ImpactItem) []output.ImpactItem {
	if len(items) == 0 {
		return items
	}

	seen := make(map[string]bool)
	result := make([]output.ImpactItem, 0, len(items))

	for _, item := range items {
		if !seen[item.StableId] {
			seen[item.StableId] = true
			result = append(result, item)
		}
	}

	return result
}
