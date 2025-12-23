package query

import (
	"context"
	"sort"
	"strings"

	"ckb/internal/backends/scip"
	"ckb/internal/graph"
	"ckb/internal/logging"
)

// FusionWeights controls how different signals are combined in ranking.
type FusionWeights struct {
	FTS     float64 `json:"fts"`     // Full-text search score
	PPR     float64 `json:"ppr"`     // Personalized PageRank score
	Hotspot float64 `json:"hotspot"` // Hotspot/churn score
	Recency float64 `json:"recency"` // Recent activity score
	Exact   float64 `json:"exact"`   // Exact match bonus
}

// DefaultFusionWeights returns default weights based on research.
func DefaultFusionWeights() FusionWeights {
	return FusionWeights{
		FTS:     0.40, // Lexical match is still primary
		PPR:     0.30, // Graph proximity adds context
		Hotspot: 0.15, // Recent activity matters
		Recency: 0.10, // Fresh code gets slight boost
		Exact:   0.05, // Small bonus for exact matches
	}
}

// FusionConfig controls the fusion ranking process.
type FusionConfig struct {
	Weights       FusionWeights
	PPREnabled    bool
	PPROptions    graph.PPROptions
	NormalizeMode string // "minmax" or "softmax"
}

// DefaultFusionConfig returns sensible defaults.
func DefaultFusionConfig() FusionConfig {
	return FusionConfig{
		Weights:       DefaultFusionWeights(),
		PPREnabled:    true,
		PPROptions:    graph.DefaultPPROptions(),
		NormalizeMode: "minmax",
	}
}

// FusionRanker combines multiple signals to rank search results.
type FusionRanker struct {
	config FusionConfig
	graph  *graph.Graph
	logger *logging.Logger
}

// NewFusionRanker creates a new fusion ranker.
func NewFusionRanker(g *graph.Graph, logger *logging.Logger, config FusionConfig) *FusionRanker {
	return &FusionRanker{
		config: config,
		graph:  g,
		logger: logger,
	}
}

// RankedResult extends SearchResultItem with fusion scores.
type RankedResult struct {
	SearchResultItem
	FusionScore    float64            `json:"fusionScore"`
	ScoreBreakdown map[string]float64 `json:"scoreBreakdown,omitempty"`
}

// Rank takes search results and re-ranks them using fusion scoring.
func (fr *FusionRanker) Rank(ctx context.Context, results []SearchResultItem, opts RankOptions) ([]RankedResult, error) {
	if len(results) == 0 {
		return []RankedResult{}, nil
	}

	// Extract seed symbols from top FTS results
	seeds := make([]string, 0, min(10, len(results)))
	for i := 0; i < len(results) && i < 10; i++ {
		seeds = append(seeds, results[i].StableId)
	}

	// Run PPR if graph is available
	var pprScores map[string]float64
	if fr.config.PPREnabled && fr.graph != nil && fr.graph.NumNodes() > 0 {
		pprOutput, err := fr.graph.PPR(ctx, seeds, fr.config.PPROptions)
		if err == nil && pprOutput != nil {
			pprScores = make(map[string]float64, len(pprOutput.Results))
			for _, r := range pprOutput.Results {
				pprScores[r.NodeID] = r.Score
			}
		}
	}

	// Compute raw scores for each signal
	ftsScores := make([]float64, len(results))
	hotspotScores := make([]float64, len(results))
	recencyScores := make([]float64, len(results))
	exactScores := make([]float64, len(results))
	pprScoresNorm := make([]float64, len(results))

	for i, r := range results {
		// FTS score (from search ranking)
		ftsScores[i] = r.Score

		// PPR score
		if pprScores != nil {
			pprScoresNorm[i] = pprScores[r.StableId]
		}

		// Hotspot score (placeholder - will integrate with hotspots package)
		hotspotScores[i] = 0.0
		if opts.HotspotScores != nil {
			if score, ok := opts.HotspotScores[r.Location.FileId]; ok {
				hotspotScores[i] = score
			}
		}

		// Recency score (placeholder - based on file modification)
		recencyScores[i] = 0.0
		if opts.RecencyScores != nil {
			if score, ok := opts.RecencyScores[r.Location.FileId]; ok {
				recencyScores[i] = score
			}
		}

		// Exact match bonus
		exactScores[i] = 0.0
		if r.Ranking != nil && r.Ranking.Signals != nil {
			if matchType, ok := r.Ranking.Signals["matchType"].(string); ok && matchType == "exact" {
				exactScores[i] = 1.0
			}
		}
	}

	// Normalize each signal to [0, 1]
	normalizeSlice(ftsScores)
	normalizeSlice(pprScoresNorm)
	normalizeSlice(hotspotScores)
	normalizeSlice(recencyScores)
	// exactScores is already 0 or 1

	// Compute fusion scores
	ranked := make([]RankedResult, len(results))
	weights := fr.config.Weights

	for i, r := range results {
		fusionScore := weights.FTS*ftsScores[i] +
			weights.PPR*pprScoresNorm[i] +
			weights.Hotspot*hotspotScores[i] +
			weights.Recency*recencyScores[i] +
			weights.Exact*exactScores[i]

		ranked[i] = RankedResult{
			SearchResultItem: r,
			FusionScore:      fusionScore,
			ScoreBreakdown: map[string]float64{
				"fts":     ftsScores[i],
				"ppr":     pprScoresNorm[i],
				"hotspot": hotspotScores[i],
				"recency": recencyScores[i],
				"exact":   exactScores[i],
			},
		}
	}

	// Sort by fusion score descending
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].FusionScore > ranked[j].FusionScore
	})

	return ranked, nil
}

// RankOptions provides additional context for ranking.
type RankOptions struct {
	Query         string
	HotspotScores map[string]float64 // file path -> hotspot score
	RecencyScores map[string]float64 // file path -> recency score
}

// normalizeSlice normalizes values to [0, 1] using min-max normalization.
func normalizeSlice(values []float64) {
	if len(values) == 0 {
		return
	}

	minVal := values[0]
	maxVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal == minVal {
		// All values are the same
		for i := range values {
			values[i] = 0.5
		}
		return
	}

	for i := range values {
		values[i] = (values[i] - minVal) / (maxVal - minVal)
	}
}

// BuildGraphFromSCIP creates a symbol graph from SCIP data.
// This is a convenience function for building graphs for ranking.
func BuildGraphFromSCIP(ctx context.Context, adapter *scip.SCIPAdapter, logger *logging.Logger) (*graph.Graph, error) {
	if adapter == nil || !adapter.IsAvailable() {
		return graph.NewGraph(), nil
	}

	idx := adapter.GetIndex()
	if idx == nil {
		return graph.NewGraph(), nil
	}

	weights := graph.DefaultEdgeWeights()
	return graph.BuildFromSCIP(ctx, idx, weights)
}

// RerankWithPPR re-ranks results using PPR only (simpler API).
func RerankWithPPR(ctx context.Context, g *graph.Graph, results []SearchResultItem, topK int) ([]SearchResultItem, error) {
	if g == nil || g.NumNodes() == 0 || len(results) == 0 {
		return results, nil
	}

	// Extract seeds from results
	seeds := make([]string, 0, min(10, len(results)))
	for i := 0; i < len(results) && i < 10; i++ {
		seeds = append(seeds, results[i].StableId)
	}

	// Expand seeds to include struct methods when we have struct fields
	// This helps PPR find cross-module dependencies through method calls
	seeds = expandSeedsWithMethods(seeds, g)

	// Run PPR
	opts := graph.DefaultPPROptions()
	opts.TopK = topK * 2 // Get extra for merging
	pprOutput, err := g.PPR(ctx, seeds, opts)
	if err != nil || pprOutput == nil {
		// PPR failed, fall back to original results (graceful degradation)
		return results, nil //nolint:nilerr // intentional fallback
	}

	// Build PPR score map
	pprScores := make(map[string]float64)
	for _, r := range pprOutput.Results {
		pprScores[r.NodeID] = r.Score
	}

	// Combine scores: original position + PPR
	type scoredResult struct {
		result SearchResultItem
		score  float64
	}

	scored := make([]scoredResult, len(results))
	for i, r := range results {
		// Original rank bonus (higher rank = higher bonus)
		positionScore := 1.0 / (float64(i) + 1.0)

		// PPR score
		pprScore := pprScores[r.StableId]

		// Combined score (weighted)
		combined := 0.6*positionScore + 0.4*pprScore

		scored[i] = scoredResult{result: r, score: combined}
	}

	// Sort by combined score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract results
	reranked := make([]SearchResultItem, len(scored))
	for i, s := range scored {
		reranked[i] = s.result
	}

	return reranked, nil
}

// expandSeedsWithMethods expands seeds to include struct methods.
// When seeds contain struct fields (e.g., "Engine#field."), this adds
// methods from the same struct (e.g., "Engine#Initialize()") to help
// PPR traverse cross-module call relationships.
func expandSeedsWithMethods(seeds []string, g *graph.Graph) []string {
	if g == nil || len(seeds) == 0 {
		return seeds
	}

	// Extract struct prefixes from seeds
	prefixes := make(map[string]bool)
	for _, seed := range seeds {
		prefix := extractStructPrefix(seed)
		if prefix != "" {
			prefixes[prefix] = true
		}
	}

	if len(prefixes) == 0 {
		return seeds
	}

	// Find methods in the graph that match these prefixes
	seedSet := make(map[string]bool, len(seeds))
	for _, s := range seeds {
		seedSet[s] = true
	}

	// Check all nodes in graph for matching methods
	for _, nodeID := range g.AllNodes() {
		// Skip if already in seeds
		if seedSet[nodeID] {
			continue
		}

		// Check if this is a method on one of our structs
		if isMethodOf(nodeID, prefixes) {
			seeds = append(seeds, nodeID)
			// Limit expansion to avoid explosion
			if len(seeds) >= 30 {
				break
			}
		}
	}

	return seeds
}

// extractStructPrefix extracts the struct prefix from a symbol ID.
// e.g., "scip-go ... Engine#cacheMisses." -> finds "Engine#" pattern
func extractStructPrefix(symbolID string) string {
	// Look for Type#member pattern
	hashIdx := -1
	for i := len(symbolID) - 1; i >= 0; i-- {
		if symbolID[i] == '#' {
			hashIdx = i
			break
		}
		// Stop at module boundary
		if symbolID[i] == '/' || symbolID[i] == '`' {
			break
		}
	}

	if hashIdx < 0 {
		return ""
	}

	// Find the start of the type name (after / or `)
	startIdx := hashIdx
	for i := hashIdx - 1; i >= 0; i-- {
		if symbolID[i] == '/' || symbolID[i] == '`' {
			startIdx = i + 1
			break
		}
	}

	if startIdx >= hashIdx {
		return ""
	}

	// Return the struct prefix including the #
	return symbolID[startIdx : hashIdx+1]
}

// isMethodOf checks if a symbol ID is a method of one of the given struct prefixes.
func isMethodOf(symbolID string, prefixes map[string]bool) bool {
	// Must end with () to be a method
	if len(symbolID) < 3 || symbolID[len(symbolID)-2:] != "()" {
		return false
	}

	// Check if it matches any prefix
	for prefix := range prefixes {
		// Find this prefix in the symbol
		idx := strings.Index(symbolID, prefix)
		if idx >= 0 {
			// Verify it's followed by method name and ()
			remainder := symbolID[idx+len(prefix):]
			if len(remainder) > 2 && remainder[len(remainder)-2:] == "()" {
				return true
			}
		}
	}

	return false
}
