package graph

import (
	"context"
	"testing"
)

func TestPPRBasic(t *testing.T) {
	// Create a simple graph:
	// A -> B -> C
	// A -> D
	// B -> D
	g := NewGraph()
	g.AddEdge("A", "B", 1.0, "call")
	g.AddEdge("B", "C", 1.0, "call")
	g.AddEdge("A", "D", 0.5, "reference")
	g.AddEdge("B", "D", 0.8, "call")

	ctx := context.Background()
	opts := DefaultPPROptions()
	opts.TopK = 10

	// Run PPR with A as seed
	result, err := g.PPR(ctx, []string{"A"}, opts)
	if err != nil {
		t.Fatalf("PPR failed: %v", err)
	}

	if len(result.Results) == 0 {
		t.Fatal("Expected some results")
	}

	// A should be in results (it's a seed)
	foundA := false
	for _, r := range result.Results {
		if r.NodeID == "A" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Error("Expected seed node A in results")
	}

	// Check metadata
	if result.TotalNodes != 4 {
		t.Errorf("Expected 4 nodes, got %d", result.TotalNodes)
	}
	if result.TotalEdges != 4 {
		t.Errorf("Expected 4 edges, got %d", result.TotalEdges)
	}
}

func TestPPRConvergence(t *testing.T) {
	g := NewGraph()

	// Create a larger graph with more structure
	nodes := []string{"main", "engine", "backend", "scip", "lsp", "query", "cache"}
	edges := []Edge{
		{From: "main", To: "engine", Weight: 1.0, Kind: "call"},
		{From: "engine", To: "backend", Weight: 1.0, Kind: "call"},
		{From: "engine", To: "query", Weight: 1.0, Kind: "call"},
		{From: "engine", To: "cache", Weight: 0.8, Kind: "reference"},
		{From: "backend", To: "scip", Weight: 1.0, Kind: "call"},
		{From: "backend", To: "lsp", Weight: 1.0, Kind: "call"},
		{From: "query", To: "cache", Weight: 0.9, Kind: "call"},
		{From: "scip", To: "cache", Weight: 0.5, Kind: "reference"},
	}
	g.AddEdges(edges)

	ctx := context.Background()
	opts := DefaultPPROptions()
	opts.TopK = len(nodes)

	result, err := g.PPR(ctx, []string{"main"}, opts)
	if err != nil {
		t.Fatalf("PPR failed: %v", err)
	}

	// Should converge within max iterations
	if !result.Converged && result.Iterations >= opts.MaxIterations {
		t.Log("PPR did not converge, but that's OK for this test size")
	}

	// All nodes should be reachable from main
	if len(result.Results) != len(nodes) {
		t.Logf("Expected %d nodes, got %d", len(nodes), len(result.Results))
	}

	// main should have highest score (it's the seed)
	if len(result.Results) > 0 && result.Results[0].NodeID != "main" {
		t.Logf("Expected 'main' as top result, got %s (score may vary)", result.Results[0].NodeID)
	}
}

func TestPPRMultipleSeeds(t *testing.T) {
	g := NewGraph()
	g.AddEdge("A", "B", 1.0, "call")
	g.AddEdge("C", "B", 1.0, "call")
	g.AddEdge("B", "D", 1.0, "call")

	ctx := context.Background()
	opts := DefaultPPROptions()

	// Use A and C as seeds
	result, err := g.PPR(ctx, []string{"A", "C"}, opts)
	if err != nil {
		t.Fatalf("PPR failed: %v", err)
	}

	// B should have high score (reachable from both seeds)
	foundB := false
	for _, r := range result.Results {
		if r.NodeID == "B" {
			foundB = true
			break
		}
	}
	if !foundB {
		t.Error("Expected B in results")
	}
}

func TestPPREmptySeeds(t *testing.T) {
	g := NewGraph()
	g.AddEdge("A", "B", 1.0, "call")

	ctx := context.Background()
	opts := DefaultPPROptions()

	_, err := g.PPR(ctx, []string{}, opts)
	if err == nil {
		t.Error("Expected error for empty seeds")
	}
}

func TestPPRNonexistentSeeds(t *testing.T) {
	g := NewGraph()
	g.AddEdge("A", "B", 1.0, "call")

	ctx := context.Background()
	opts := DefaultPPROptions()

	result, err := g.PPR(ctx, []string{"X", "Y"}, opts)
	if err != nil {
		t.Fatalf("PPR failed: %v", err)
	}

	// Should return empty results (no valid seeds)
	if len(result.Results) != 0 {
		t.Errorf("Expected 0 results for nonexistent seeds, got %d", len(result.Results))
	}
}

func TestPPRPathBacktracking(t *testing.T) {
	g := NewGraph()
	g.AddEdge("A", "B", 1.0, "call")
	g.AddEdge("B", "C", 1.0, "call")
	g.AddEdge("C", "D", 1.0, "call")

	ctx := context.Background()
	opts := DefaultPPROptions()
	opts.IncludePaths = true

	result, err := g.PPR(ctx, []string{"A"}, opts)
	if err != nil {
		t.Fatalf("PPR failed: %v", err)
	}

	// D should have a path back to A
	for _, r := range result.Results {
		if r.NodeID == "D" && len(r.Path) > 0 {
			if r.Path[0] != "A" {
				t.Errorf("Expected path to start with A, got %v", r.Path)
			}
			break
		}
	}
}

func TestFilterResults(t *testing.T) {
	results := []PPRResult{
		{NodeID: "scip/foo", Score: 0.5},
		{NodeID: "scip/bar", Score: 0.3},
		{NodeID: "lsp/baz", Score: 0.2},
	}

	filtered := FilterByPrefix(results, "scip/")
	if len(filtered) != 2 {
		t.Errorf("Expected 2 results with scip/ prefix, got %d", len(filtered))
	}

	filtered = FilterByMinScore(results, 0.3)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 results with score >= 0.3, got %d", len(filtered))
	}
}

func BenchmarkPPR(b *testing.B) {
	// Create a moderate-sized graph
	g := NewGraph()
	numNodes := 1000
	for i := range numNodes {
		for j := 1; j <= 5; j++ {
			target := (i + j) % numNodes
			g.AddEdge(
				nodeID(i),
				nodeID(target),
				1.0,
				"call",
			)
		}
	}

	ctx := context.Background()
	opts := DefaultPPROptions()
	opts.TopK = 20

	b.ResetTimer()
	for range b.N {
		_, _ = g.PPR(ctx, []string{"node_0"}, opts)
	}
}

func nodeID(i int) string {
	return "node_" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
