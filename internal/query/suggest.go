package query

import (
	"context"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/suggest"
	"github.com/SimplyLiz/CodeMCP/internal/version"
)

// SuggestRefactoringsOptions configures suggestion detection.
type SuggestRefactoringsOptions struct {
	Scope       string   // directory or file path
	MinSeverity string   // minimum severity (default: "low")
	Types       []string // filter by suggestion type
	Limit       int      // max results (default: 50)
}

// SuggestRefactoringsResponse is the response for suggestRefactorings.
type SuggestRefactoringsResponse struct {
	AINavigationMeta
	Suggestions []suggest.Suggestion    `json:"suggestions"`
	Summary     *suggest.SuggestSummary `json:"summary"`
	TotalFound  int                     `json:"totalFound"`
}

// SuggestRefactorings detects refactoring opportunities in the codebase.
func (e *Engine) SuggestRefactorings(ctx context.Context, opts SuggestRefactoringsOptions) (*SuggestRefactoringsResponse, error) {
	startTime := time.Now()

	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, err
	}

	// Create analyzer
	analyzer := suggest.NewAnalyzer(e.repoRoot, e.logger, e.scipAdapter)

	result, err := analyzer.Analyze(ctx, suggest.AnalyzeOptions{
		Scope:       opts.Scope,
		MinSeverity: opts.MinSeverity,
		Types:       opts.Types,
		Limit:       opts.Limit,
	})
	if err != nil {
		return nil, err
	}

	// Build provenance
	var backendContribs []BackendContribution
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "scip", Available: true, Used: true,
		})
	}
	if e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "git", Available: true, Used: true,
		})
	}

	resp := &SuggestRefactoringsResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    version.Version,
			SchemaVersion: 1,
			Tool:          "suggestRefactorings",
			Provenance: e.buildProvenance(repoState, "full", startTime, backendContribs, CompletenessInfo{
				Score:  0.75,
				Reason: "multi-analyzer-suggestions",
			}),
		},
		Suggestions: result.Suggestions,
		Summary:     result.Summary,
		TotalFound:  result.TotalFound,
	}

	return resp, nil
}
