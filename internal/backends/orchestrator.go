package backends

import (
	"context"
	"fmt"
	"sync"
	"time"

	"ckb/internal/errors"
	"ckb/internal/logging"
)

// Orchestrator coordinates queries across multiple backends
type Orchestrator struct {
	backends map[BackendID]Backend
	policy   *QueryPolicy
	limiter  *RateLimiter
	ladder   *BackendLadder
	logger   *logging.Logger

	// Merge strategies
	preferFirstMerger *PreferFirstMerger
	unionMerger       *UnionMerger

	mu sync.RWMutex
}

// NewOrchestrator creates a new backend orchestrator
func NewOrchestrator(policy *QueryPolicy, logger *logging.Logger) *Orchestrator {
	return &Orchestrator{
		backends:          make(map[BackendID]Backend),
		policy:            policy,
		limiter:           NewRateLimiter(policy),
		ladder:            NewBackendLadder(policy, logger),
		logger:            logger,
		preferFirstMerger: NewPreferFirstMerger(policy, logger),
		unionMerger:       NewUnionMerger(policy, logger),
	}
}

// RegisterBackend registers a backend with the orchestrator
func (o *Orchestrator) RegisterBackend(backend Backend) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.backends[backend.ID()] = backend
	o.logger.Info("Registered backend", map[string]interface{}{
		"backend":      backend.ID(),
		"capabilities": backend.Capabilities(),
		"available":    backend.IsAvailable(),
	})
}

// UnregisterBackend removes a backend from the orchestrator
func (o *Orchestrator) UnregisterBackend(backendID BackendID) {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.backends, backendID)
	o.logger.Info("Unregistered backend", map[string]interface{}{
		"backend": backendID,
	})
}

// GetAvailableBackends returns a list of currently available backends
func (o *Orchestrator) GetAvailableBackends() []BackendID {
	o.mu.RLock()
	defer o.mu.RUnlock()

	available := []BackendID{}
	for id, backend := range o.backends {
		if backend.IsAvailable() {
			available = append(available, id)
		}
	}

	return available
}

// Query executes a query against appropriate backends and merges results
func (o *Orchestrator) Query(ctx context.Context, req QueryRequest) (*QueryResult, error) {
	startTime := time.Now()

	o.logger.Debug("Starting query", map[string]interface{}{
		"queryType": req.Type,
		"mergeMode": o.policy.MergeMode,
	})

	// Get available backends
	o.mu.RLock()
	availableBackends := make(map[BackendID]Backend)
	for id, backend := range o.backends {
		if backend.IsAvailable() {
			availableBackends[id] = backend
		}
	}
	o.mu.RUnlock()

	if len(availableBackends) == 0 {
		return nil, errors.NewCkbError(
			errors.BackendUnavailable,
			"No backends available",
			nil,
			errors.GetSuggestedFixes(errors.BackendUnavailable),
			nil,
		)
	}

	// Select backends to query based on merge mode
	selectedBackends := o.ladder.SelectBackends(ctx, availableBackends, req)
	if len(selectedBackends) == 0 {
		return nil, errors.NewCkbError(
			errors.BackendUnavailable,
			"No suitable backends available for query",
			nil,
			nil,
			nil,
		)
	}

	o.logger.Debug("Selected backends", map[string]interface{}{
		"backends": selectedBackends,
		"count":    len(selectedBackends),
	})

	// Query selected backends in parallel
	backendResults, err := o.queryBackends(ctx, req, selectedBackends, availableBackends)
	if err != nil {
		return nil, err
	}

	// Check if all backends failed
	allFailed := true
	for _, result := range backendResults {
		if result.Error == nil {
			allFailed = false
			break
		}
	}

	if allFailed {
		return nil, errors.NewCkbError(
			errors.BackendUnavailable,
			"All backends failed to respond",
			nil,
			nil,
			nil,
		)
	}

	// Merge results based on merge mode
	mergedResult, err := o.mergeResults(ctx, req, backendResults)
	if err != nil {
		return nil, err
	}

	// Build final result
	result := &QueryResult{
		Data:            mergedResult.Data,
		Completeness:    mergedResult.Completeness,
		Contributions:   o.buildContributions(backendResults),
		Provenance:      mergedResult.Provenance,
		TotalDurationMs: time.Since(startTime).Milliseconds(),
	}

	o.logger.Info("Query completed", map[string]interface{}{
		"durationMs":        result.TotalDurationMs,
		"completenessScore": result.Completeness.Score,
		"primaryBackend":    result.Provenance.PrimaryBackend,
	})

	return result, nil
}

// queryBackends queries multiple backends in parallel
func (o *Orchestrator) queryBackends(
	ctx context.Context,
	req QueryRequest,
	backendIDs []BackendID,
	availableBackends map[BackendID]Backend,
) ([]BackendResult, error) {
	results := make([]BackendResult, len(backendIDs))
	var wg sync.WaitGroup

	for i, backendID := range backendIDs {
		wg.Add(1)
		go func(idx int, id BackendID) {
			defer wg.Done()

			backend, ok := availableBackends[id]
			if !ok {
				results[idx] = BackendResult{
					BackendID: id,
					Error:     fmt.Errorf("backend not available"),
				}
				return
			}

			// Create timeout context
			timeout := time.Duration(o.policy.GetTimeout(id)) * time.Millisecond
			queryCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Acquire rate limit permit
			if err := o.limiter.Acquire(queryCtx, id); err != nil {
				results[idx] = BackendResult{
					BackendID: id,
					Error:     err,
				}
				return
			}
			defer o.limiter.Release(id)

			// Execute query with coalescing
			startTime := time.Now()
			data, err := o.limiter.CoalesceOrExecute(queryCtx, req, id, func(ctx context.Context, r QueryRequest) (interface{}, error) {
				return o.executeBackendQuery(ctx, backend, r)
			})

			results[idx] = BackendResult{
				BackendID:  id,
				Data:       data,
				Error:      err,
				DurationMs: time.Since(startTime).Milliseconds(),
			}

			if err != nil {
				o.logger.Warn("Backend query failed", map[string]interface{}{
					"backend": id,
					"error":   err.Error(),
				})
			}
		}(i, backendID)
	}

	wg.Wait()
	return results, nil
}

// executeBackendQuery executes a query on a specific backend
func (o *Orchestrator) executeBackendQuery(
	ctx context.Context,
	backend Backend,
	req QueryRequest,
) (interface{}, error) {
	symbolBackend, ok := backend.(SymbolBackend)
	if !ok {
		return nil, fmt.Errorf("backend does not implement SymbolBackend")
	}

	switch req.Type {
	case QueryTypeSymbol:
		return symbolBackend.GetSymbol(ctx, req.SymbolID)

	case QueryTypeSearch:
		opts := SearchOptions{}
		if req.SearchOpts != nil {
			opts = *req.SearchOpts
		}
		return symbolBackend.SearchSymbols(ctx, req.Query, opts)

	case QueryTypeReferences:
		opts := RefOptions{}
		if req.RefOpts != nil {
			opts = *req.RefOpts
		}
		return symbolBackend.FindReferences(ctx, req.SymbolID, opts)

	default:
		return nil, fmt.Errorf("unsupported query type: %s", req.Type)
	}
}

// mergeResults merges backend results based on merge mode
func (o *Orchestrator) mergeResults(
	ctx context.Context,
	req QueryRequest,
	backendResults []BackendResult,
) (*QueryResult, error) {
	var data interface{}
	var provenance Provenance
	var completeness CompletenessInfo

	switch o.policy.MergeMode {
	case MergeModePreferFirst:
		switch req.Type {
		case QueryTypeSymbol:
			merged, prov := o.preferFirstMerger.MergeSymbolResults(backendResults)
			if merged != nil {
				completeness = merged.Completeness
			}
			data = merged
			provenance = prov

		case QueryTypeSearch:
			merged, prov := o.preferFirstMerger.MergeSearchResults(backendResults)
			if merged != nil {
				completeness = merged.Completeness
			}
			data = merged
			provenance = prov

		case QueryTypeReferences:
			merged, prov := o.preferFirstMerger.MergeReferencesResults(backendResults)
			if merged != nil {
				completeness = merged.Completeness
			}
			data = merged
			provenance = prov
		}

	case MergeModeUnion:
		switch req.Type {
		case QueryTypeSymbol:
			merged, prov := o.unionMerger.MergeSymbolResults(backendResults)
			if merged != nil {
				completeness = merged.Completeness
			}
			data = merged
			provenance = prov

		case QueryTypeSearch:
			merged, prov := o.unionMerger.MergeSearchResults(backendResults)
			if merged != nil {
				completeness = merged.Completeness
			}
			data = merged
			provenance = prov

		case QueryTypeReferences:
			merged, prov := o.unionMerger.MergeReferencesResults(backendResults)
			if merged != nil {
				completeness = merged.Completeness
			}
			data = merged
			provenance = prov
		}
	}

	if data == nil {
		return nil, fmt.Errorf("merge failed to produce result")
	}

	return &QueryResult{
		Data:         data,
		Completeness: completeness,
		Provenance:   provenance,
	}, nil
}

// buildContributions creates contribution records from backend results
func (o *Orchestrator) buildContributions(results []BackendResult) []BackendContribution {
	contributions := make([]BackendContribution, len(results))

	for i, result := range results {
		contrib := BackendContribution{
			BackendID:  result.BackendID,
			DurationMs: result.DurationMs,
			WasUsed:    result.Error == nil,
		}

		if result.Error != nil {
			contrib.Error = result.Error.Error()
		} else {
			// Count items based on result type
			switch data := result.Data.(type) {
			case *SymbolResult:
				contrib.ItemCount = 1
			case *SearchResult:
				contrib.ItemCount = len(data.Symbols)
			case *ReferencesResult:
				contrib.ItemCount = len(data.References)
			}
		}

		contributions[i] = contrib
	}

	return contributions
}

// GetBackend retrieves a backend by ID
func (o *Orchestrator) GetBackend(backendID BackendID) (Backend, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	backend, ok := o.backends[backendID]
	return backend, ok
}

// GetSymbolBackend retrieves a SymbolBackend by ID
func (o *Orchestrator) GetSymbolBackend(backendID BackendID) (SymbolBackend, bool) {
	backend, ok := o.GetBackend(backendID)
	if !ok {
		return nil, false
	}
	symbolBackend, ok := backend.(SymbolBackend)
	return symbolBackend, ok
}

// Shutdown stops all backends and releases resources
func (o *Orchestrator) Shutdown() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	var lastErr error
	for id, backend := range o.backends {
		if closer, ok := backend.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				o.logger.Warn("Failed to close backend", map[string]interface{}{
					"backend": id,
					"error":   err.Error(),
				})
				lastErr = err
			}
		}
	}

	o.backends = make(map[BackendID]Backend)
	return lastErr
}

// IsHealthy checks if all registered backends are healthy
func (o *Orchestrator) IsHealthy() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, backend := range o.backends {
		if healthChecker, ok := backend.(interface{ IsHealthy() bool }); ok {
			if !healthChecker.IsHealthy() {
				return false
			}
		}
	}
	return true
}
