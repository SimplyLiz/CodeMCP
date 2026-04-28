package query

import "github.com/SimplyLiz/CodeMCP/internal/navigator"

// GetSkeleton returns a token-optimised skeleton of the project using Cartographer.
// detailLevel: "minimal", "standard" (default), or "extended".
// Returns nil with no error when Cartographer is not compiled in.
func (e *Engine) GetSkeleton(detailLevel string) (*navigator.SkeletonResult, error) {
	if !navigator.Available() {
		return nil, nil //nolint:nilnil // intentional: callers check nil to detect unavailability
	}
	return navigator.SkeletonMap(e.repoRoot, detailLevel)
}

// GetRankedSkeleton returns project files ranked by PageRank relevance to a set of
// focus files, pruned to a token budget. Returns nil when Cartographer is not compiled in.
func (e *Engine) GetRankedSkeleton(focus []string, tokenBudget uint32) (*navigator.RankedSkeletonResult, error) {
	if !navigator.Available() {
		return nil, nil //nolint:nilnil // intentional: callers check nil to detect unavailability
	}
	return navigator.RankedSkeleton(e.repoRoot, focus, tokenBudget)
}
