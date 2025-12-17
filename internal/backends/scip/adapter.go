package scip

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"ckb/internal/backends"
	"ckb/internal/config"
	"ckb/internal/errors"
	"ckb/internal/logging"
	"ckb/internal/repostate"
)

// SCIPAdapter implements the Backend and SymbolBackend interfaces for SCIP
type SCIPAdapter struct {
	indexPath    string
	index        *SCIPIndex
	logger       *logging.Logger
	queryTimeout time.Duration
	repoRoot     string
	cfg          *config.Config

	// Mutex for thread-safe access to index
	mu sync.RWMutex

	// freshness tracks index freshness
	freshness *IndexFreshness
}

// NewSCIPAdapter creates a new SCIP adapter
func NewSCIPAdapter(cfg *config.Config, logger *logging.Logger) (*SCIPAdapter, error) {
	if !cfg.Backends.Scip.Enabled {
		return nil, errors.NewCkbError(
			errors.BackendUnavailable,
			"SCIP backend is disabled in configuration",
			nil,
			nil,
			nil,
		)
	}

	indexPath := GetIndexPath(cfg.RepoRoot, cfg.Backends.Scip.IndexPath)

	// Get default query timeout from config
	queryTimeout := time.Duration(cfg.QueryPolicy.TimeoutMs["scip"]) * time.Millisecond
	if queryTimeout == 0 {
		queryTimeout = 5 * time.Second
	}

	adapter := &SCIPAdapter{
		indexPath:    indexPath,
		logger:       logger,
		queryTimeout: queryTimeout,
		repoRoot:     cfg.RepoRoot,
		cfg:          cfg,
	}

	// Try to load the index immediately
	if err := adapter.LoadIndex(); err != nil {
		// Log warning but don't fail - adapter can still report unavailable
		logger.Warn("Failed to load SCIP index", map[string]interface{}{
			"error": err.Error(),
			"path":  indexPath,
		})
	}

	return adapter, nil
}

// ID returns the backend identifier
func (s *SCIPAdapter) ID() backends.BackendID {
	return backends.BackendSCIP
}

// IsAvailable checks if the SCIP backend is available
func (s *SCIPAdapter) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if index is loaded
	if s.index == nil {
		return false
	}

	// Check if index file still exists
	if _, err := os.Stat(s.indexPath); os.IsNotExist(err) {
		return false
	}

	return true
}

// Capabilities returns the capabilities supported by SCIP
func (s *SCIPAdapter) Capabilities() []string {
	return []string{
		"symbol-search",
		"find-references",
		"goto-definition",
		"find-implementations",
		"type-hierarchy",
	}
}

// Priority returns the priority of the SCIP backend (highest priority)
func (s *SCIPAdapter) Priority() int {
	return 1 // SCIP has highest priority
}

// LoadIndex loads or reloads the SCIP index
func (s *SCIPAdapter) LoadIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Loading SCIP index", map[string]interface{}{
		"path": s.indexPath,
	})

	index, err := LoadSCIPIndex(s.indexPath)
	if err != nil {
		return err
	}

	s.index = index

	s.logger.Info("SCIP index loaded successfully", map[string]interface{}{
		"documents": len(index.Documents),
		"symbols":   len(index.Symbols),
		"commit":    index.IndexedCommit,
	})

	// Compute freshness
	if repoState, err := repostate.ComputeRepoState(s.repoRoot); err == nil {
		s.freshness = ComputeIndexFreshness(index.IndexedCommit, repoState, s.repoRoot)
		if s.freshness.IsStale() {
			s.logger.Warn("SCIP index is stale", map[string]interface{}{
				"warning": s.freshness.Warning,
			})
		}
	}

	return nil
}

// GetSymbol retrieves detailed information about a specific symbol
func (s *SCIPAdapter) GetSymbol(ctx context.Context, id string) (*backends.SymbolResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.index == nil {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			"SCIP index not loaded",
			nil,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Apply timeout
	_, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	// Get symbol from index
	scipSym, err := s.index.GetSymbolByID(id)
	if err != nil {
		return nil, errors.NewCkbError(
			errors.SymbolNotFound,
			fmt.Sprintf("Symbol not found: %s", id),
			err,
			nil,
			nil,
		)
	}

	// Convert to SymbolResult
	result := s.convertToSymbolResult(scipSym)

	return result, nil
}

// SearchSymbols searches for symbols matching the query
func (s *SCIPAdapter) SearchSymbols(ctx context.Context, query string, opts backends.SearchOptions) (*backends.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.index == nil {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			"SCIP index not loaded",
			nil,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Apply timeout
	_, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	// Convert options
	scipOpts := SearchOptions{
		MaxResults:   opts.MaxResults,
		IncludeTests: opts.IncludeTests,
		Scope:        opts.Scope,
		Kind:         convertKindsToSCIP(opts.Kind),
	}

	// Search symbols
	scipSymbols, err := s.index.SearchSymbols(query, scipOpts)
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to search symbols",
			err,
			nil,
			nil,
		)
	}

	// Convert results
	symbols := make([]backends.SymbolResult, len(scipSymbols))
	for i, scipSym := range scipSymbols {
		symbols[i] = *s.convertToSymbolResult(scipSym)
	}

	// Compute completeness
	completeness := s.computeCompleteness()

	return &backends.SearchResult{
		Symbols:      symbols,
		Completeness: completeness,
		TotalMatches: len(symbols),
	}, nil
}

// FindReferences finds all references to a symbol
func (s *SCIPAdapter) FindReferences(ctx context.Context, symbolID string, opts backends.RefOptions) (*backends.ReferencesResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.index == nil {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			"SCIP index not loaded",
			nil,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Apply timeout
	_, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	// Convert options
	scipOpts := ReferenceOptions{
		MaxResults:        opts.MaxResults,
		IncludeDefinition: opts.IncludeDeclaration,
		IncludeTests:      opts.IncludeTests,
		IncludeContext:    true,
		Scope:             opts.Scope,
	}

	// Find references
	scipRefs, err := s.index.FindReferences(symbolID, scipOpts)
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to find references",
			err,
			nil,
			nil,
		)
	}

	// Convert results
	references := make([]backends.Reference, len(scipRefs))
	for i, scipRef := range scipRefs {
		references[i] = s.convertToReference(scipRef)
	}

	// Compute completeness
	completeness := s.computeCompleteness()

	return &backends.ReferencesResult{
		References:      references,
		Completeness:    completeness,
		TotalReferences: len(references),
	}, nil
}

// convertToSymbolResult converts a SCIPSymbol to a SymbolResult
func (s *SCIPAdapter) convertToSymbolResult(scipSym *SCIPSymbol) *backends.SymbolResult {
	var location backends.Location
	if scipSym.Location != nil {
		location = backends.Location{
			Path:      scipSym.Location.FileId,
			Line:      scipSym.Location.StartLine + 1, // Convert to 1-indexed
			Column:    scipSym.Location.StartColumn + 1,
			EndLine:   scipSym.Location.EndLine + 1,
			EndColumn: scipSym.Location.EndColumn + 1,
		}
	}

	// Compute visibility confidence
	visibilityConfidence := 0.9 // SCIP has good visibility inference
	if scipSym.Visibility == "" {
		visibilityConfidence = 0.5
	}

	return &backends.SymbolResult{
		StableID:             scipSym.StableId,
		Name:                 scipSym.Name,
		Kind:                 string(scipSym.Kind),
		Location:             location,
		SignatureNormalized:  scipSym.SignatureNormalized,
		SignatureFull:        "", // TODO: Extract full signature
		Visibility:           scipSym.Visibility,
		VisibilityConfidence: visibilityConfidence,
		ContainerName:        scipSym.ContainerName,
		ModuleID:             "", // TODO: Determine module ID
		Documentation:        scipSym.Documentation,
		Completeness:         s.computeCompleteness(),
	}
}

// convertToReference converts a SCIPReference to a Reference
func (s *SCIPAdapter) convertToReference(scipRef *SCIPReference) backends.Reference {
	var location backends.Location
	if scipRef.Location != nil {
		location = backends.Location{
			Path:      scipRef.Location.FileId,
			Line:      scipRef.Location.StartLine + 1, // Convert to 1-indexed
			Column:    scipRef.Location.StartColumn + 1,
			EndLine:   scipRef.Location.EndLine + 1,
			EndColumn: scipRef.Location.EndColumn + 1,
		}
	}

	return backends.Reference{
		Location: location,
		Kind:     string(scipRef.Kind),
		SymbolID: scipRef.SymbolId,
		Context:  scipRef.Context,
	}
}

// computeCompleteness computes the completeness of results based on index freshness
func (s *SCIPAdapter) computeCompleteness() backends.CompletenessInfo {
	if s.freshness == nil {
		return backends.NewCompletenessInfo(1.0, backends.FullBackend, "SCIP index loaded")
	}

	score := s.freshness.GetCompletenessScore()
	reason := backends.FullBackend
	details := "SCIP index available"

	if s.freshness.IsStale() {
		reason = backends.IndexStale
		details = s.freshness.Warning
	}

	return backends.NewCompletenessInfo(score, reason, details)
}

// convertKindsToSCIP converts backend kind strings to SCIP SymbolKind
func convertKindsToSCIP(kinds []string) []SymbolKind {
	scipKinds := make([]SymbolKind, len(kinds))
	for i, k := range kinds {
		scipKinds[i] = SymbolKind(k)
	}
	return scipKinds
}

// GetIndexInfo returns information about the loaded index
func (s *SCIPAdapter) GetIndexInfo() *IndexInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.index == nil {
		return &IndexInfo{
			Available: false,
			Path:      s.indexPath,
		}
	}

	return &IndexInfo{
		Available:     true,
		Path:          s.indexPath,
		DocumentCount: len(s.index.Documents),
		SymbolCount:   len(s.index.Symbols),
		IndexedCommit: s.index.IndexedCommit,
		LoadedAt:      s.index.LoadedAt,
		Freshness:     s.freshness,
	}
}

// IndexInfo contains information about the SCIP index
type IndexInfo struct {
	Available     bool
	Path          string
	DocumentCount int
	SymbolCount   int
	IndexedCommit string
	LoadedAt      time.Time
	Freshness     *IndexFreshness
}

// Reload reloads the SCIP index
func (s *SCIPAdapter) Reload() error {
	return s.LoadIndex()
}

// Close closes the adapter and releases resources
func (s *SCIPAdapter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.index = nil
	s.freshness = nil

	s.logger.Info("SCIP adapter closed", nil)
	return nil
}
