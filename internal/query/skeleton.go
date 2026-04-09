package query

import "github.com/SimplyLiz/CodeMCP/internal/cartographer"

// GetSkeleton returns a token-optimised skeleton of the project using Cartographer.
// detailLevel: "minimal", "standard" (default), or "extended".
// Returns nil with no error when Cartographer is not compiled in.
func (e *Engine) GetSkeleton(detailLevel string) (*cartographer.SkeletonResult, error) {
	if !cartographer.Available() {
		return nil, nil //nolint:nilnil // intentional: callers check nil to detect unavailability
	}
	return cartographer.SkeletonMap(e.repoRoot, detailLevel)
}

// GetRankedSkeleton returns project files ranked by PageRank relevance to a set of
// focus files, pruned to a token budget. Returns nil when Cartographer is not compiled in.
func (e *Engine) GetRankedSkeleton(focus []string, tokenBudget uint32) (*cartographer.RankedSkeletonResult, error) {
	if !cartographer.Available() {
		return nil, nil //nolint:nilnil // intentional: callers check nil to detect unavailability
	}
	return cartographer.RankedSkeleton(e.repoRoot, focus, tokenBudget)
}
