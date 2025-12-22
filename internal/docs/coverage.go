package docs

// CoverageAnalyzer analyzes documentation coverage for symbols.
type CoverageAnalyzer struct {
	store       *Store
	symbolIndex SymbolIndex
}

// NewCoverageAnalyzer creates a new coverage analyzer.
func NewCoverageAnalyzer(store *Store, symbolIndex SymbolIndex) *CoverageAnalyzer {
	return &CoverageAnalyzer{
		store:       store,
		symbolIndex: symbolIndex,
	}
}

// SymbolLister is an interface for listing all symbols.
type SymbolLister interface {
	// ListAllSymbols returns all symbols in the index.
	ListAllSymbols() []Symbol

	// GetSymbolCentrality returns the centrality score for a symbol (0-1).
	// Higher scores mean more important symbols.
	GetSymbolCentrality(symbolID string) float64
}

// Analyze analyzes documentation coverage.
// If symbolLister is provided, it will compute actual coverage.
// Otherwise, it returns stats from indexed docs.
func (a *CoverageAnalyzer) Analyze(lister SymbolLister, exportedOnly bool, topN int) (*CoverageReport, error) {
	report := &CoverageReport{}

	if lister == nil {
		// Just return stats from indexed docs
		stats, err := a.store.GetStats()
		if err != nil {
			return nil, err
		}
		report.Documented = stats.Resolved
		report.TotalSymbols = stats.ReferencesFound
		if report.TotalSymbols > 0 {
			report.CoveragePercent = float64(report.Documented) / float64(report.TotalSymbols) * 100
		}
		return report, nil
	}

	// Get all symbols from the index
	symbols := lister.ListAllSymbols()
	report.TotalSymbols = len(symbols)

	// Build set of documented symbols
	documentedSymbols := make(map[string]bool)
	docs, err := a.store.GetAllDocuments()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		fullDoc, err := a.store.GetDocument(doc.Path)
		if err != nil || fullDoc == nil {
			continue
		}
		for _, ref := range fullDoc.References {
			if ref.SymbolID != nil && *ref.SymbolID != "" {
				documentedSymbols[*ref.SymbolID] = true
			}
		}
	}

	report.Documented = len(documentedSymbols)
	report.Undocumented = report.TotalSymbols - report.Documented

	if report.TotalSymbols > 0 {
		report.CoveragePercent = float64(report.Documented) / float64(report.TotalSymbols) * 100
	}

	// Find top undocumented symbols by centrality
	type symbolScore struct {
		sym        Symbol
		centrality float64
	}

	var undocumented []symbolScore
	for _, sym := range symbols {
		if !documentedSymbols[sym.ID] {
			centrality := lister.GetSymbolCentrality(sym.ID)
			undocumented = append(undocumented, symbolScore{sym, centrality})
		}
	}

	// Sort by centrality (descending)
	for i := 0; i < len(undocumented)-1; i++ {
		for j := i + 1; j < len(undocumented); j++ {
			if undocumented[j].centrality > undocumented[i].centrality {
				undocumented[i], undocumented[j] = undocumented[j], undocumented[i]
			}
		}
	}

	// Take top N
	if topN <= 0 {
		topN = 10
	}
	if topN > len(undocumented) {
		topN = len(undocumented)
	}

	for i := 0; i < topN; i++ {
		u := undocumented[i]
		report.TopUndocumented = append(report.TopUndocumented, UndocSymbol{
			SymbolID:   u.sym.ID,
			Name:       u.sym.DisplayName,
			Centrality: u.centrality,
		})
	}

	return report, nil
}
