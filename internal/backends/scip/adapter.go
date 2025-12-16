package scip

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ckb/internal/backends"
	"ckb/internal/config"
	"ckb/internal/errors"
	"ckb/internal/logging"
	"ckb/internal/repostate"
)

// repoIndex wraps a loaded SCIP index with repo metadata for cross-repo lookups.
type repoIndex struct {
	name      string
	repoRoot  string
	indexPath string
	index     *SCIPIndex
	freshness *IndexFreshness
}

// SCIPAdapter implements the Backend and SymbolBackend interfaces for SCIP
// and now supports multiple repository indexes.
type SCIPAdapter struct {
	logger       *logging.Logger
	queryTimeout time.Duration
	cfg          *config.Config

	// Mutex for thread-safe access to indexes
	mu sync.RWMutex

	indexes     map[string]*repoIndex
	defaultRepo string
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

	// Get default query timeout from config
	queryTimeout := time.Duration(cfg.QueryPolicy.TimeoutMs["scip"]) * time.Millisecond
	if queryTimeout == 0 {
		queryTimeout = 5 * time.Second
	}

	indexes := buildRepoIndexes(cfg)
	adapter := &SCIPAdapter{
		logger:       logger,
		queryTimeout: queryTimeout,
		cfg:          cfg,
		indexes:      indexes,
		defaultRepo:  selectDefaultRepo(indexes),
	}

	// Try to load the indexes immediately
	if err := adapter.LoadIndex(); err != nil {
		// Log warning but don't fail - adapter can still report unavailable
		logger.Warn("Failed to load SCIP indexes", map[string]interface{}{
			"error": err.Error(),
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

	for _, repo := range s.indexes {
		if repo.index == nil {
			continue
		}
		if _, err := os.Stat(repo.indexPath); os.IsNotExist(err) {
			continue
		}
		return true
	}

	return false
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

	var lastErr error

	for _, repo := range s.indexes {
		s.logger.Info("Loading SCIP index", map[string]interface{}{
			"path": repo.indexPath,
			"repo": repo.name,
		})

		index, err := LoadSCIPIndex(repo.indexPath)
		if err != nil {
			lastErr = err
			s.logger.Warn("Failed to load SCIP index", map[string]interface{}{
				"path":  repo.indexPath,
				"repo":  repo.name,
				"error": err.Error(),
			})
			repo.index = nil
			repo.freshness = nil
			continue
		}

		repo.index = index

		s.logger.Info("SCIP index loaded successfully", map[string]interface{}{
			"documents": len(index.Documents),
			"symbols":   len(index.Symbols),
			"commit":    index.IndexedCommit,
			"repo":      repo.name,
		})

		// Compute freshness per repo
		if repoState, err := repostate.ComputeRepoState(repo.repoRoot); err == nil {
			repo.freshness = ComputeIndexFreshness(index.IndexedCommit, repoState, repo.repoRoot)
			if repo.freshness.IsStale() {
				s.logger.Warn("SCIP index is stale", map[string]interface{}{
					"warning": repo.freshness.Warning,
					"repo":    repo.name,
				})
			}
		}
	}

	return lastErr
}

// GetSymbol retrieves detailed information about a specific symbol
func (s *SCIPAdapter) GetSymbol(ctx context.Context, id string) (*backends.SymbolResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repoName, symbolID := splitSymbolID(id, s.defaultRepo)
	repo, ok := s.indexes[repoName]
	if !ok || repo.index == nil {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			"SCIP index not loaded",
			nil,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	// Get symbol from index
	scipSym, err := repo.index.GetSymbolByID(symbolID)
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
	result := s.convertToSymbolResult(repoName, scipSym)

	return result, nil
}

// SearchSymbols searches for symbols matching the query
func (s *SCIPAdapter) SearchSymbols(ctx context.Context, query string, opts backends.SearchOptions) (*backends.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.indexes) == 0 {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			"SCIP index not loaded",
			nil,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var symbols []backends.SymbolResult
	totalMatches := 0

	for _, repo := range s.indexes {
		if repo.index == nil {
			continue
		}

		scopedPaths, allowed := filterScopeForRepo(opts.Scope, repo.name)
		if !allowed {
			continue
		}

		scipOpts := SearchOptions{
			MaxResults:   opts.MaxResults,
			IncludeTests: opts.IncludeTests,
			Scope:        scopedPaths,
			Kind:         convertKindsToSCIP(opts.Kind),
		}

		scipSymbols, err := repo.index.SearchSymbols(query, scipOpts)
		if err != nil {
			return nil, errors.NewCkbError(
				errors.InternalError,
				fmt.Sprintf("Failed to search symbols for repo %s", repo.name),
				err,
				nil,
				nil,
			)
		}

		for _, scipSym := range scipSymbols {
			symbols = append(symbols, *s.convertToSymbolResult(repo.name, scipSym))
			if opts.MaxResults > 0 && len(symbols) >= opts.MaxResults {
				completeness := s.computeAggregateCompleteness()
				return &backends.SearchResult{
					Symbols:      symbols,
					Completeness: completeness,
					TotalMatches: len(symbols),
				}, nil
			}
		}

		totalMatches += len(scipSymbols)
	}

	// Compute completeness
	completeness := s.computeAggregateCompleteness()

	return &backends.SearchResult{
		Symbols:      symbols,
		Completeness: completeness,
		TotalMatches: totalMatches,
	}, nil
}

// FindReferences finds all references to a symbol
func (s *SCIPAdapter) FindReferences(ctx context.Context, symbolID string, opts backends.RefOptions) (*backends.ReferencesResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repoName, scopedSymbolID := splitSymbolID(symbolID, s.defaultRepo)
	repo, ok := s.indexes[repoName]
	if !ok || repo.index == nil {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			"SCIP index not loaded",
			nil,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
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
	scipRefs, err := repo.index.FindReferences(scopedSymbolID, scipOpts)
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
		references[i] = s.convertToReference(repo.name, scipRef)
	}

	// Compute completeness
	completeness := s.computeCompleteness(repoName)

	return &backends.ReferencesResult{
		References:      references,
		Completeness:    completeness,
		TotalReferences: len(references),
	}, nil
}

func buildRepoIndexes(cfg *config.Config) map[string]*repoIndex {
	indexes := make(map[string]*repoIndex)

	if len(cfg.Backends.Scip.Indexes) > 0 {
		for _, idx := range cfg.Backends.Scip.Indexes {
			repoRoot := idx.RepoRoot
			if repoRoot == "" {
				repoRoot = cfg.RepoRoot
			}

			indexPath := idx.IndexPath
			if indexPath == "" {
				indexPath = cfg.Backends.Scip.IndexPath
			}

			name := idx.Name
			if name == "" {
				name = filepath.Base(repoRoot)
				if name == "." || name == "" {
					name = "default"
				}
			}

			indexes[name] = &repoIndex{
				name:      name,
				repoRoot:  repoRoot,
				indexPath: GetIndexPath(repoRoot, indexPath),
			}
		}
	}

	if len(indexes) == 0 {
		defaultName := filepath.Base(cfg.RepoRoot)
		if defaultName == "." || defaultName == "" {
			defaultName = "default"
		}

		indexes[defaultName] = &repoIndex{
			name:      defaultName,
			repoRoot:  cfg.RepoRoot,
			indexPath: GetIndexPath(cfg.RepoRoot, cfg.Backends.Scip.IndexPath),
		}
	}

	return indexes
}

func selectDefaultRepo(indexes map[string]*repoIndex) string {
	if _, ok := indexes["default"]; ok {
		return "default"
	}

	if len(indexes) == 1 {
		for name := range indexes {
			return name
		}
	}

	names := make([]string, 0, len(indexes))
	for name := range indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 0 {
		return names[0]
	}

	return ""
}

func splitSymbolID(symbolID, defaultRepo string) (string, string) {
	parts := strings.SplitN(symbolID, "::", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return defaultRepo, symbolID
}

func qualifySymbolID(repoName, symbolID string) string {
	if repoName == "" {
		return symbolID
	}
	return fmt.Sprintf("%s::%s", repoName, symbolID)
}

func qualifyPath(repoName, path string) string {
	if repoName == "" {
		return path
	}
	return fmt.Sprintf("%s:%s", repoName, path)
}

func filterScopeForRepo(scopes []string, repoName string) ([]string, bool) {
	filtered := make([]string, 0, len(scopes))
	hasRepoScoped := false
	allowed := false

	for _, scope := range scopes {
		parts := strings.SplitN(scope, ":", 2)
		if len(parts) == 2 && parts[0] != "" {
			hasRepoScoped = true
			if parts[0] == repoName {
				filtered = append(filtered, parts[1])
				allowed = true
			}
			continue
		}

		filtered = append(filtered, scope)
		// If no repo-specific scope is provided, all repos are allowed.
	}

	if hasRepoScoped {
		return filtered, allowed
	}

	return filtered, true
}

func (s *SCIPAdapter) computeAggregateCompleteness() backends.CompletenessInfo {
	bestScore := 0.0
	bestReason := backends.NoBackendAvailable
	bestDetails := "No SCIP indexes loaded"
	hasIndex := false

	for name, repo := range s.indexes {
		if repo.index == nil {
			continue
		}
		hasIndex = true
		info := s.computeCompleteness(name)
		if bestScore == 0.0 || info.Score < bestScore {
			bestScore = info.Score
			bestReason = info.Reason
			bestDetails = info.Details
		}
	}

	if !hasIndex {
		return backends.NewCompletenessInfo(0.0, bestReason, bestDetails)
	}

	return backends.NewCompletenessInfo(bestScore, bestReason, bestDetails)
}

// convertToSymbolResult converts a SCIPSymbol to a SymbolResult
func (s *SCIPAdapter) convertToSymbolResult(repoName string, scipSym *SCIPSymbol) *backends.SymbolResult {
	var location backends.Location
	if scipSym.Location != nil {
		location = backends.Location{
			Path:      qualifyPath(repoName, scipSym.Location.FileId),
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
		StableID:             qualifySymbolID(repoName, scipSym.StableId),
		Name:                 scipSym.Name,
		Kind:                 string(scipSym.Kind),
		Location:             location,
		SignatureNormalized:  scipSym.SignatureNormalized,
		SignatureFull:        "", // TODO: Extract full signature
		Visibility:           scipSym.Visibility,
		VisibilityConfidence: visibilityConfidence,
		ContainerName:        scipSym.ContainerName,
		ModuleID:             repoName,
		Documentation:        scipSym.Documentation,
		Completeness:         s.computeCompleteness(repoName),
	}
}

// convertToReference converts a SCIPReference to a Reference
func (s *SCIPAdapter) convertToReference(repoName string, scipRef *SCIPReference) backends.Reference {
	var location backends.Location
	if scipRef.Location != nil {
		location = backends.Location{
			Path:      qualifyPath(repoName, scipRef.Location.FileId),
			Line:      scipRef.Location.StartLine + 1, // Convert to 1-indexed
			Column:    scipRef.Location.StartColumn + 1,
			EndLine:   scipRef.Location.EndLine + 1,
			EndColumn: scipRef.Location.EndColumn + 1,
		}
	}

	return backends.Reference{
		Location: location,
		Kind:     string(scipRef.Kind),
		SymbolID: qualifySymbolID(repoName, scipRef.SymbolId),
		Context:  scipRef.Context,
	}
}

// computeCompleteness computes the completeness of results based on index freshness
func (s *SCIPAdapter) computeCompleteness(repoName string) backends.CompletenessInfo {
	repo, ok := s.indexes[repoName]
	if !ok || repo == nil {
		return backends.NewCompletenessInfo(0.0, backends.NoBackendAvailable, "SCIP index not loaded")
	}

	if repo.freshness == nil {
		return backends.NewCompletenessInfo(1.0, backends.FullBackend, "SCIP index loaded")
	}

	score := repo.freshness.GetCompletenessScore()
	reason := backends.FullBackend
	details := "SCIP index available"

	if repo.freshness.IsStale() {
		reason = backends.IndexStale
		details = repo.freshness.Warning
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

	info := &IndexInfo{Repositories: make([]RepoIndexInfo, 0, len(s.indexes))}

	if len(s.indexes) == 0 {
		return info
	}

	for _, repo := range s.indexes {
		repoInfo := RepoIndexInfo{
			Name:      repo.name,
			Path:      repo.indexPath,
			Freshness: repo.freshness,
		}

		if repo.index != nil {
			info.Available = true
			repoInfo.DocumentCount = len(repo.index.Documents)
			repoInfo.SymbolCount = len(repo.index.Symbols)
			repoInfo.IndexedCommit = repo.index.IndexedCommit
			repoInfo.LoadedAt = repo.index.LoadedAt

			info.DocumentCount += repoInfo.DocumentCount
			info.SymbolCount += repoInfo.SymbolCount
		}

		info.Repositories = append(info.Repositories, repoInfo)
	}

	if len(info.Repositories) == 1 {
		repo := info.Repositories[0]
		info.Path = repo.Path
		info.IndexedCommit = repo.IndexedCommit
		info.LoadedAt = repo.LoadedAt
		info.Freshness = repo.Freshness
	} else if info.Available {
		info.Path = "multiple"
	}

	return info
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

	Repositories []RepoIndexInfo
}

// RepoIndexInfo describes a single repository index instance.
type RepoIndexInfo struct {
	Name          string
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

	for _, repo := range s.indexes {
		repo.index = nil
		repo.freshness = nil
	}

	s.logger.Info("SCIP adapter closed", nil)
	return nil
}
