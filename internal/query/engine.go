// Package query provides the central query engine that coordinates all CKB operations.
// It connects backends, compression, caching, and response formatting.
package query

import (
	"context"
	"sync"
	"time"

	"ckb/internal/backends"
	"ckb/internal/backends/git"
	"ckb/internal/backends/lsp"
	"ckb/internal/backends/scip"
	"ckb/internal/compression"
	"ckb/internal/config"
	"ckb/internal/errors"
	"ckb/internal/identity"
	"ckb/internal/logging"
	"ckb/internal/output"
	"ckb/internal/storage"
)

// Engine is the central query coordinator for CKB.
type Engine struct {
	db         *storage.DB
	logger     *logging.Logger
	config     *config.Config
	compressor *compression.Compressor
	resolver   *identity.IdentityResolver
	repoRoot   string

	// Backend references
	orchestrator  *backends.Orchestrator
	scipAdapter   *scip.SCIPAdapter
	gitAdapter    *git.GitAdapter
	lspSupervisor *lsp.LspSupervisor

	// Cached repo state
	repoStateMu     sync.RWMutex
	cachedState     *RepoState
	stateComputedAt time.Time
}

// RepoState represents the current state of the repository.
type RepoState struct {
	RepoStateId         string `json:"repoStateId"`
	HeadCommit          string `json:"headCommit"`
	StagedDiffHash      string `json:"stagedDiffHash,omitempty"`
	WorkingTreeDiffHash string `json:"workingTreeDiffHash,omitempty"`
	UntrackedListHash   string `json:"untrackedListHash,omitempty"`
	Dirty               bool   `json:"dirty"`
	ComputedAt          string `json:"computedAt"`
}

// NewEngine creates a new query engine.
func NewEngine(repoRoot string, db *storage.DB, logger *logging.Logger, cfg *config.Config) (*Engine, error) {
	// Create compressor
	budget := compression.NewBudgetFromConfig(cfg)
	limits := compression.NewLimitsFromConfig(cfg)
	compressor := compression.NewCompressor(budget, limits)

	// Create identity resolver
	resolver := identity.NewIdentityResolver(db, logger)

	// Create orchestrator
	policy := backends.LoadQueryPolicy(cfg)
	orchestrator := backends.NewOrchestrator(policy, logger)

	engine := &Engine{
		db:           db,
		logger:       logger,
		config:       cfg,
		compressor:   compressor,
		resolver:     resolver,
		repoRoot:     repoRoot,
		orchestrator: orchestrator,
	}

	// Initialize backends
	if err := engine.initializeBackends(cfg); err != nil {
		logger.Warn("Some backends failed to initialize", map[string]interface{}{
			"error": err.Error(),
		})
		// Don't fail - some backends are optional
	}

	return engine, nil
}

// initializeBackends initializes all configured backends.
func (e *Engine) initializeBackends(cfg *config.Config) error {
	var lastErr error

	// Initialize Git backend (always available)
	gitAdapter, err := git.NewGitAdapter(cfg, e.logger)
	if err != nil {
		e.logger.Warn("Failed to initialize Git backend", map[string]interface{}{
			"error": err.Error(),
		})
		lastErr = err
	} else {
		e.gitAdapter = gitAdapter
	}

	// Initialize SCIP backend if enabled
	if cfg.Backends.Scip.Enabled {
		scipAdapter, err := scip.NewSCIPAdapter(cfg, e.logger)
		if err != nil {
			e.logger.Warn("Failed to initialize SCIP backend", map[string]interface{}{
				"error": err.Error(),
			})
			lastErr = err
		} else {
			e.scipAdapter = scipAdapter
			e.orchestrator.RegisterBackend(scipAdapter)
		}
	}

	// Initialize LSP supervisor if enabled
	if cfg.Backends.Lsp.Enabled {
		e.lspSupervisor = lsp.NewLspSupervisor(cfg, e.logger)
	}

	return lastErr
}

// GetRepoState returns the current repository state.
func (e *Engine) GetRepoState(ctx context.Context, mode string) (*RepoState, error) {
	e.repoStateMu.RLock()
	// Use cached state if fresh enough (< 5 seconds)
	if e.cachedState != nil && time.Since(e.stateComputedAt) < 5*time.Second {
		state := e.cachedState
		e.repoStateMu.RUnlock()
		return state, nil
	}
	e.repoStateMu.RUnlock()

	// Compute fresh state
	state, err := e.computeRepoState(ctx)
	if err != nil {
		return nil, err
	}

	e.repoStateMu.Lock()
	e.cachedState = state
	e.stateComputedAt = time.Now()
	e.repoStateMu.Unlock()

	return state, nil
}

// computeRepoState computes the current repository state from git.
func (e *Engine) computeRepoState(ctx context.Context) (*RepoState, error) {
	if e.gitAdapter == nil || !e.gitAdapter.IsAvailable() {
		return &RepoState{
			RepoStateId: "unknown",
			Dirty:       true,
			ComputedAt:  time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	state, err := e.gitAdapter.GetRepoState()
	if err != nil {
		e.logger.Warn("failed to get repo state from git", map[string]interface{}{
			"error": err.Error(),
		})
		//nolint:nilerr // return fallback state on git errors
		return &RepoState{
			RepoStateId: "unknown",
			Dirty:       true,
			ComputedAt:  time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	return &RepoState{
		RepoStateId:         state.RepoStateID,
		HeadCommit:          state.HeadCommit,
		StagedDiffHash:      state.StagedDiffHash,
		WorkingTreeDiffHash: state.WorkingTreeDiffHash,
		UntrackedListHash:   state.UntrackedListHash,
		Dirty:               state.Dirty,
		ComputedAt:          time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Provenance contains metadata about how a response was generated.
type Provenance struct {
	RepoStateId     string                `json:"repoStateId"`
	RepoStateDirty  bool                  `json:"repoStateDirty"`
	RepoStateMode   string                `json:"repoStateMode"`
	Backends        []BackendContribution `json:"backends"`
	Completeness    CompletenessInfo      `json:"completeness"`
	CachedAt        string                `json:"cachedAt,omitempty"`
	QueryDurationMs int64                 `json:"queryDurationMs"`
	Warnings        []string              `json:"warnings,omitempty"`
	Timeouts        []string              `json:"timeouts,omitempty"`
	Truncations     []string              `json:"truncations,omitempty"`
}

// BackendContribution describes a backend's contribution to a response.
type BackendContribution struct {
	BackendId    string  `json:"backendId"`
	Available    bool    `json:"available"`
	Used         bool    `json:"used"`
	ResultCount  int     `json:"resultCount,omitempty"`
	DurationMs   int64   `json:"durationMs,omitempty"`
	Completeness float64 `json:"completeness,omitempty"`
}

// CompletenessInfo describes the completeness of results.
type CompletenessInfo struct {
	Score   float64 `json:"score"`
	Reason  string  `json:"reason"`
	Details string  `json:"details,omitempty"`
}

// buildProvenance creates provenance metadata for a response.
func (e *Engine) buildProvenance(
	repoState *RepoState,
	mode string,
	startTime time.Time,
	contributions []BackendContribution,
	completeness CompletenessInfo,
) *Provenance {
	var warnings []string
	var timeouts []string

	return &Provenance{
		RepoStateId:     repoState.RepoStateId,
		RepoStateDirty:  repoState.Dirty,
		RepoStateMode:   mode,
		Backends:        contributions,
		Completeness:    completeness,
		QueryDurationMs: time.Since(startTime).Milliseconds(),
		Warnings:        warnings,
		Timeouts:        timeouts,
	}
}

// sortAndEncode applies deterministic sorting and encoding to response data.
func (e *Engine) sortAndEncode(data interface{}) ([]byte, error) {
	return output.DeterministicEncode(data)
}

// generateDrilldowns creates contextual drilldowns based on truncation and completeness.
func (e *Engine) generateDrilldowns(
	truncation *compression.TruncationInfo,
	completeness CompletenessInfo,
	symbolId string,
	topModule *output.Module,
) []output.Drilldown {
	ctx := &compression.DrilldownContext{
		Budget: e.compressor.GetBudget(),
	}

	if truncation != nil {
		ctx.TruncationReason = truncation.Reason
	}

	ctx.Completeness = compression.CompletenessInfo{
		Score:            completeness.Score,
		IsBestEffort:     completeness.Reason == "best-effort-lsp",
		IsWorkspaceReady: completeness.Reason != "workspace-not-ready",
	}

	ctx.SymbolId = symbolId
	ctx.TopModule = topModule

	return compression.GenerateDrilldowns(ctx)
}

// wrapError converts an error to a CKB error with suggestions.
func (e *Engine) wrapError(err error, code errors.ErrorCode) *errors.CkbError {
	if ckbErr, ok := err.(*errors.CkbError); ok {
		return ckbErr
	}
	return errors.NewCkbError(code, err.Error(), nil, nil, nil)
}

// Close shuts down the query engine.
func (e *Engine) Close() error {
	var lastErr error

	if e.orchestrator != nil {
		if err := e.orchestrator.Shutdown(); err != nil {
			lastErr = err
		}
	}

	if e.lspSupervisor != nil {
		if err := e.lspSupervisor.Shutdown(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// GetScipBackend returns the SCIP adapter (may be nil).
func (e *Engine) GetScipBackend() *scip.SCIPAdapter {
	return e.scipAdapter
}

// GetGitBackend returns the Git adapter (may be nil).
func (e *Engine) GetGitBackend() *git.GitAdapter {
	return e.gitAdapter
}

// GetLspSupervisor returns the LSP supervisor (may be nil).
func (e *Engine) GetLspSupervisor() *lsp.LspSupervisor {
	return e.lspSupervisor
}
