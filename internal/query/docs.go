package query

import (
	"context"
	"fmt"

	"ckb/internal/backends"
	"ckb/internal/docs"
)

// IndexDocs scans and indexes documentation for symbol references.
func (e *Engine) IndexDocs(force bool) (*docs.IndexStats, error) {
	store := docs.NewStore(e.db)

	// Create a symbol index adapter
	symbolIndex := &scipSymbolIndex{engine: e}

	// First, rebuild the suffix index if SCIP is available
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		if err := e.rebuildSuffixIndex(store); err != nil {
			e.logger.Warn("Failed to rebuild suffix index", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	// Create indexer and run
	config := docs.DefaultIndexerConfig()
	indexer := docs.NewIndexer(e.repoRoot, symbolIndex, store, config)

	return indexer.IndexAll(force)
}

// GetDocsForSymbol finds all documents that reference a symbol.
func (e *Engine) GetDocsForSymbol(symbol string, limit int) ([]docs.DocReference, error) {
	store := docs.NewStore(e.db)
	ctx := context.Background()

	// Try to find the symbol ID
	symbolID := symbol
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		// Try to resolve the symbol name to an ID via search
		opts := backends.SearchOptions{MaxResults: 1}
		result, err := e.scipAdapter.SearchSymbols(ctx, symbol, opts)
		if err == nil && result != nil && len(result.Symbols) > 0 {
			symbolID = result.Symbols[0].StableID
		}
	}

	return store.GetDocsForSymbol(symbolID, limit)
}

// GetDocumentInfo retrieves information about an indexed document.
func (e *Engine) GetDocumentInfo(path string) (*docs.Document, error) {
	store := docs.NewStore(e.db)
	return store.GetDocument(path)
}

// GetDocsForModule finds all documents linked to a module.
func (e *Engine) GetDocsForModule(moduleID string) ([]docs.Document, error) {
	store := docs.NewStore(e.db)
	return store.GetDocsForModule(moduleID)
}

// CheckDocStaleness checks a single document for stale references.
func (e *Engine) CheckDocStaleness(path string) (*docs.StalenessReport, error) {
	store := docs.NewStore(e.db)
	symbolIndex := &scipSymbolIndex{engine: e}

	doc, err := store.GetDocument(path)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", path)
	}

	// v1.1: Use checker with identity support for rename detection
	checker := docs.NewStalenessCheckerWithIdentity(symbolIndex, store, e.db, e.logger)
	report := checker.CheckDocument(*doc)
	return &report, nil
}

// CheckAllDocsStaleness checks all indexed documents for stale references.
func (e *Engine) CheckAllDocsStaleness() ([]docs.StalenessReport, error) {
	store := docs.NewStore(e.db)
	symbolIndex := &scipSymbolIndex{engine: e}

	// v1.1: Use checker with identity support for rename detection
	checker := docs.NewStalenessCheckerWithIdentity(symbolIndex, store, e.db, e.logger)
	return checker.CheckAllDocuments()
}

// GetDocCoverage returns documentation coverage statistics.
func (e *Engine) GetDocCoverage(exportedOnly bool, topN int) (*docs.CoverageReport, error) {
	store := docs.NewStore(e.db)
	symbolIndex := &scipSymbolIndex{engine: e}

	analyzer := docs.NewCoverageAnalyzer(store, symbolIndex)

	// For now, pass nil lister (basic stats only)
	// Full coverage analysis requires symbol listing which is heavy
	return analyzer.Analyze(nil, exportedOnly, topN)
}

// rebuildSuffixIndex rebuilds the suffix index from SCIP symbols.
func (e *Engine) rebuildSuffixIndex(store *docs.Store) error {
	if e.scipAdapter == nil || !e.scipAdapter.IsAvailable() {
		return nil
	}

	// Get all symbols from SCIP
	scipSymbols := e.scipAdapter.AllSymbols()
	if scipSymbols == nil {
		return nil
	}

	// Convert to docs.Symbol format
	symbols := make([]docs.Symbol, 0, len(scipSymbols))
	for _, sym := range scipSymbols {
		canonical := docs.ParseCanonicalName(sym.Symbol)
		display := sym.DisplayName
		if display == "" {
			display = docs.ExtractDisplayName(sym.Symbol)
		}

		symbols = append(symbols, docs.Symbol{
			ID:            sym.Symbol,
			CanonicalName: canonical,
			DisplayName:   display,
		})
	}

	// Build suffix index
	suffixIndex := docs.NewSuffixIndex(store)
	version := e.getSymbolIndexVersion()
	return suffixIndex.Build(symbols, version)
}

// getSymbolIndexVersion returns a version string for the current symbol index.
func (e *Engine) getSymbolIndexVersion() string {
	// Use HEAD commit as version
	if e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		state, err := e.gitAdapter.GetRepoState()
		if err == nil {
			return state.HeadCommit
		}
	}
	return "unknown"
}

// scipSymbolIndex adapts SCIP backend to docs.SymbolIndex interface.
type scipSymbolIndex struct {
	engine *Engine
}

func (s *scipSymbolIndex) ExactMatch(canonicalName string) (string, bool) {
	if s.engine.scipAdapter == nil || !s.engine.scipAdapter.IsAvailable() {
		return "", false
	}

	ctx := context.Background()

	// Search for exact match
	// The canonical name might be in format "internal/pkg.Type.Method"
	opts := backends.SearchOptions{MaxResults: 10}
	result, err := s.engine.scipAdapter.SearchSymbols(ctx, canonicalName, opts)
	if err != nil || result == nil || len(result.Symbols) == 0 {
		return "", false
	}

	// Look for exact match by checking the parsed canonical name
	for _, sym := range result.Symbols {
		symCanonical := docs.ParseCanonicalName(sym.StableID)
		if symCanonical == canonicalName {
			return sym.StableID, true
		}
	}

	return "", false
}

func (s *scipSymbolIndex) GetDisplayName(symbolID string) string {
	if s.engine.scipAdapter == nil || !s.engine.scipAdapter.IsAvailable() {
		return docs.ExtractDisplayName(symbolID)
	}

	ctx := context.Background()
	sym, err := s.engine.scipAdapter.GetSymbol(ctx, symbolID)
	if err != nil || sym == nil {
		return docs.ExtractDisplayName(symbolID)
	}

	if sym.Name != "" {
		return sym.Name
	}
	return docs.ExtractDisplayName(symbolID)
}

func (s *scipSymbolIndex) Exists(symbolID string) bool {
	if s.engine.scipAdapter == nil || !s.engine.scipAdapter.IsAvailable() {
		return false
	}

	ctx := context.Background()
	sym, err := s.engine.scipAdapter.GetSymbol(ctx, symbolID)
	return err == nil && sym != nil
}

func (s *scipSymbolIndex) IsLanguageIndexed(hint string) bool {
	// For now, assume Go is always indexed if SCIP is available
	if s.engine.scipAdapter == nil || !s.engine.scipAdapter.IsAvailable() {
		return false
	}
	return true
}
