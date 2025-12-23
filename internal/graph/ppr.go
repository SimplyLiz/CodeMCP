// Package graph provides graph algorithms for code navigation.
package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Edge represents a directed edge in the symbol graph.
type Edge struct {
	From   string  // Source symbol ID
	To     string  // Target symbol ID
	Weight float64 // Edge weight (0.0-1.0)
	Kind   string  // Edge kind: "call", "reference", "type", "module"
}

// PPROptions configures Personalized PageRank computation.
type PPROptions struct {
	// Damping is the probability of following an edge vs teleporting (default: 0.85)
	Damping float64

	// MaxIterations is the maximum number of power iterations (default: 20)
	MaxIterations int

	// Tolerance for convergence detection (default: 1e-6)
	Tolerance float64

	// TopK is the number of top results to return (default: 20)
	TopK int

	// IncludePaths enables backtracking to explain why nodes were reached
	IncludePaths bool
}

// DefaultPPROptions returns sensible defaults for PPR.
func DefaultPPROptions() PPROptions {
	return PPROptions{
		Damping:       0.85,
		MaxIterations: 20,
		Tolerance:     1e-6,
		TopK:          20,
		IncludePaths:  true,
	}
}

// PPRResult represents a ranked node from PPR computation.
type PPRResult struct {
	NodeID string   `json:"nodeId"`
	Score  float64  `json:"score"`
	Path   []string `json:"path,omitempty"` // Path from seed to this node
}

// PPROutput contains the full PPR computation result.
type PPROutput struct {
	Results       []PPRResult `json:"results"`
	Iterations    int         `json:"iterations"`
	Converged     bool        `json:"converged"`
	SeedNodes     []string    `json:"seedNodes"`
	TotalNodes    int         `json:"totalNodes"`
	TotalEdges    int         `json:"totalEdges"`
	ComputationMs int64       `json:"computationMs"`
}

// Graph represents a sparse directed graph for PPR computation.
type Graph struct {
	// Node IDs (for index lookup)
	nodes    []string
	nodeIdx  map[string]int
	numNodes int

	// Adjacency list: outEdges[i] = list of (neighbor_idx, weight)
	outEdges [][]edgeEntry
	inEdges  [][]edgeEntry // Reverse edges for path backtracking

	// Edge metadata
	edgeKinds map[string]map[string]string // from -> to -> kind
}

type edgeEntry struct {
	target int
	weight float64
}

// NewGraph creates an empty graph.
func NewGraph() *Graph {
	return &Graph{
		nodes:     make([]string, 0),
		nodeIdx:   make(map[string]int),
		outEdges:  make([][]edgeEntry, 0),
		inEdges:   make([][]edgeEntry, 0),
		edgeKinds: make(map[string]map[string]string),
	}
}

// AddNode adds a node if it doesn't exist, returns its index.
func (g *Graph) AddNode(id string) int {
	if idx, ok := g.nodeIdx[id]; ok {
		return idx
	}
	idx := len(g.nodes)
	g.nodes = append(g.nodes, id)
	g.nodeIdx[id] = idx
	g.outEdges = append(g.outEdges, nil)
	g.inEdges = append(g.inEdges, nil)
	g.numNodes++
	return idx
}

// AddEdge adds a directed edge from src to dst.
func (g *Graph) AddEdge(src, dst string, weight float64, kind string) {
	srcIdx := g.AddNode(src)
	dstIdx := g.AddNode(dst)

	// Add forward edge
	g.outEdges[srcIdx] = append(g.outEdges[srcIdx], edgeEntry{target: dstIdx, weight: weight})

	// Add reverse edge for backtracking
	g.inEdges[dstIdx] = append(g.inEdges[dstIdx], edgeEntry{target: srcIdx, weight: weight})

	// Store edge kind
	if g.edgeKinds[src] == nil {
		g.edgeKinds[src] = make(map[string]string)
	}
	g.edgeKinds[src][dst] = kind
}

// AddEdges adds multiple edges at once.
func (g *Graph) AddEdges(edges []Edge) {
	for _, e := range edges {
		g.AddEdge(e.From, e.To, e.Weight, e.Kind)
	}
}

// NumNodes returns the number of nodes in the graph.
func (g *Graph) NumNodes() int {
	return g.numNodes
}

// NumEdges returns the total number of edges.
func (g *Graph) NumEdges() int {
	total := 0
	for _, edges := range g.outEdges {
		total += len(edges)
	}
	return total
}

// AllNodes returns all node IDs in the graph.
func (g *Graph) AllNodes() []string {
	return g.nodes
}

// PPR computes Personalized PageRank with the given seed nodes.
// Seeds should be symbol IDs that exist in the graph.
func (g *Graph) PPR(_ context.Context, seeds []string, opts PPROptions) (*PPROutput, error) {
	if len(seeds) == 0 {
		return nil, fmt.Errorf("no seed nodes provided")
	}

	if g.numNodes == 0 {
		return &PPROutput{
			Results:    []PPRResult{},
			SeedNodes:  seeds,
			TotalNodes: 0,
			TotalEdges: 0,
		}, nil
	}

	// Apply defaults
	if opts.Damping <= 0 || opts.Damping >= 1 {
		opts.Damping = 0.85
	}
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 20
	}
	if opts.Tolerance <= 0 {
		opts.Tolerance = 1e-6
	}
	if opts.TopK <= 0 {
		opts.TopK = 20
	}

	// Map seeds to indices
	seedIndices := make([]int, 0, len(seeds))
	validSeeds := make([]string, 0, len(seeds))
	for _, s := range seeds {
		if idx, ok := g.nodeIdx[s]; ok {
			seedIndices = append(seedIndices, idx)
			validSeeds = append(validSeeds, s)
		}
	}

	if len(seedIndices) == 0 {
		return &PPROutput{
			Results:    []PPRResult{},
			SeedNodes:  seeds,
			TotalNodes: g.numNodes,
			TotalEdges: g.NumEdges(),
		}, nil
	}

	// Initialize teleport vector (uniform over seeds)
	teleport := make([]float64, g.numNodes)
	teleportWeight := 1.0 / float64(len(seedIndices))
	for _, idx := range seedIndices {
		teleport[idx] = teleportWeight
	}

	// Initialize scores
	scores := make([]float64, g.numNodes)
	copy(scores, teleport)

	// Pre-compute out-degree normalization
	outDegree := make([]float64, g.numNodes)
	for i, edges := range g.outEdges {
		for _, e := range edges {
			outDegree[i] += e.weight
		}
	}

	// Power iteration
	newScores := make([]float64, g.numNodes)
	var iterations int
	var converged bool

	for iter := range opts.MaxIterations {
		iterations = iter + 1

		// Reset new scores
		for i := range newScores {
			newScores[i] = 0
		}

		// Propagate scores along edges
		for i, edges := range g.outEdges {
			if len(edges) == 0 || outDegree[i] == 0 {
				continue
			}
			contrib := scores[i] / outDegree[i]
			for _, e := range edges {
				newScores[e.target] += contrib * e.weight
			}
		}

		// Apply damping and teleport
		maxDiff := 0.0
		for i := range newScores {
			newScores[i] = opts.Damping*newScores[i] + (1-opts.Damping)*teleport[i]
			diff := abs(newScores[i] - scores[i])
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		// Swap score vectors
		scores, newScores = newScores, scores

		// Check convergence
		if maxDiff < opts.Tolerance {
			converged = true
			break
		}
	}

	// Collect top-K results
	type scoredNode struct {
		idx   int
		score float64
	}
	ranked := make([]scoredNode, 0, g.numNodes)
	for i, s := range scores {
		if s > 0 {
			ranked = append(ranked, scoredNode{idx: i, score: s})
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	// Limit to top-K
	if len(ranked) > opts.TopK {
		ranked = ranked[:opts.TopK]
	}

	// Build results with optional path backtracking
	results := make([]PPRResult, len(ranked))
	seedSet := make(map[int]bool)
	for _, idx := range seedIndices {
		seedSet[idx] = true
	}

	for i, sn := range ranked {
		result := PPRResult{
			NodeID: g.nodes[sn.idx],
			Score:  sn.score,
		}

		// Backtrack to find path from seed
		if opts.IncludePaths && !seedSet[sn.idx] {
			result.Path = g.backtrackPath(sn.idx, seedSet, 5)
		}

		results[i] = result
	}

	return &PPROutput{
		Results:    results,
		Iterations: iterations,
		Converged:  converged,
		SeedNodes:  validSeeds,
		TotalNodes: g.numNodes,
		TotalEdges: g.NumEdges(),
	}, nil
}

// backtrackPath finds a path from the target back to any seed node.
// Uses greedy backtracking following incoming edges with highest weight.
func (g *Graph) backtrackPath(target int, seedSet map[int]bool, maxDepth int) []string {
	path := []string{g.nodes[target]}
	current := target
	visited := make(map[int]bool)
	visited[target] = true

	for depth := 0; depth < maxDepth; depth++ {
		// Find best incoming edge
		bestPrev := -1
		bestWeight := 0.0

		for _, e := range g.inEdges[current] {
			if !visited[e.target] && e.weight > bestWeight {
				bestWeight = e.weight
				bestPrev = e.target
			}
		}

		if bestPrev < 0 {
			break
		}

		path = append(path, g.nodes[bestPrev])
		visited[bestPrev] = true

		if seedSet[bestPrev] {
			// Reached a seed node
			break
		}

		current = bestPrev
	}

	// Reverse path to go from seed to target
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// GetEdgeKind returns the kind of edge between two nodes.
func (g *Graph) GetEdgeKind(from, to string) string {
	if m, ok := g.edgeKinds[from]; ok {
		return m[to]
	}
	return ""
}

// HasNode checks if a node exists in the graph.
func (g *Graph) HasNode(id string) bool {
	_, ok := g.nodeIdx[id]
	return ok
}

// Neighbors returns the outgoing neighbors of a node.
func (g *Graph) Neighbors(id string) []string {
	idx, ok := g.nodeIdx[id]
	if !ok {
		return nil
	}

	neighbors := make([]string, len(g.outEdges[idx]))
	for i, e := range g.outEdges[idx] {
		neighbors[i] = g.nodes[e.target]
	}
	return neighbors
}

// FilterResults filters PPR results by a predicate.
func FilterResults(results []PPRResult, predicate func(PPRResult) bool) []PPRResult {
	filtered := make([]PPRResult, 0, len(results))
	for _, r := range results {
		if predicate(r) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FilterByPrefix returns results where NodeID has the given prefix.
func FilterByPrefix(results []PPRResult, prefix string) []PPRResult {
	return FilterResults(results, func(r PPRResult) bool {
		return strings.HasPrefix(r.NodeID, prefix)
	})
}

// FilterByMinScore returns results with score >= minScore.
func FilterByMinScore(results []PPRResult, minScore float64) []PPRResult {
	return FilterResults(results, func(r PPRResult) bool {
		return r.Score >= minScore
	})
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
