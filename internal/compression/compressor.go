package compression

import (
	"ckb/internal/output"
)

// Compressor applies budgets and limits to compress response data
type Compressor struct {
	budget *ResponseBudget
	limits *BackendLimits
}

// NewCompressor creates a new Compressor with the given budget and limits
func NewCompressor(budget *ResponseBudget, limits *BackendLimits) *Compressor {
	if budget == nil {
		budget = DefaultBudget()
	}
	if limits == nil {
		limits = DefaultLimits()
	}

	return &Compressor{
		budget: budget,
		limits: limits,
	}
}

// CompressModules truncates the modules list to fit within budget
// Returns the compressed modules and truncation info
func (c *Compressor) CompressModules(modules []output.Module) ([]output.Module, *TruncationInfo) {
	originalCount := len(modules)
	maxModules := c.budget.MaxModules

	if originalCount <= maxModules {
		return modules, nil
	}

	// Take the first MaxModules items (assumed to be sorted by priority)
	compressed := modules[:maxModules]

	return compressed, NewTruncationInfo(TruncMaxModules, originalCount, maxModules)
}

// CompressSymbols truncates the symbols list to fit within budget per module
// Returns the compressed symbols and truncation info
func (c *Compressor) CompressSymbols(symbols []output.Symbol) ([]output.Symbol, *TruncationInfo) {
	originalCount := len(symbols)
	maxSymbols := c.budget.MaxSymbolsPerModule

	if originalCount <= maxSymbols {
		return symbols, nil
	}

	// Take the first MaxSymbolsPerModule items (assumed to be sorted by confidence/priority)
	compressed := symbols[:maxSymbols]

	return compressed, NewTruncationInfo(TruncMaxSymbols, originalCount, maxSymbols)
}

// CompressImpactItems truncates the impact items list to fit within budget
// Returns the compressed items and truncation info
func (c *Compressor) CompressImpactItems(items []output.ImpactItem) ([]output.ImpactItem, *TruncationInfo) {
	originalCount := len(items)
	maxItems := c.budget.MaxImpactItems

	if originalCount <= maxItems {
		return items, nil
	}

	// Take the first MaxImpactItems (assumed to be sorted by confidence/priority)
	compressed := items[:maxItems]

	return compressed, NewTruncationInfo(TruncMaxItems, originalCount, maxItems)
}

// CompressReferences truncates the references list to fit within backend limits
// Returns the compressed references and truncation info
func (c *Compressor) CompressReferences(refs []output.Reference) ([]output.Reference, *TruncationInfo) {
	originalCount := len(refs)
	maxRefs := c.limits.MaxRefsPerQuery

	if originalCount <= maxRefs {
		return refs, nil
	}

	// Take the first MaxRefsPerQuery items
	compressed := refs[:maxRefs]

	return compressed, NewTruncationInfo(TruncMaxRefs, originalCount, maxRefs)
}

// GetBudget returns the current response budget
func (c *Compressor) GetBudget() *ResponseBudget {
	return c.budget
}

// GetLimits returns the current backend limits
func (c *Compressor) GetLimits() *BackendLimits {
	return c.limits
}
