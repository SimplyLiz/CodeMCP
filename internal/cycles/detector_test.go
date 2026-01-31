package cycles

import (
	"testing"
)

func TestNewDetector(t *testing.T) {
	d := NewDetector()
	if d == nil {
		t.Fatal("NewDetector returned nil")
	}
}

func TestDetect_EmptyGraph(t *testing.T) {
	d := NewDetector()
	result := d.Detect(nil, nil, nil, DetectOptions{})
	if result == nil {
		t.Fatal("expected non-nil result for empty graph")
	}
	if len(result.Cycles) != 0 {
		t.Errorf("expected 0 cycles, got %d", len(result.Cycles))
	}
}

func TestDetect_NoCycles(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b", "c"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if len(result.Cycles) != 0 {
		t.Errorf("expected 0 cycles in DAG, got %d", len(result.Cycles))
	}
}

func TestDetect_SimpleCycle(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"a"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if result.TotalCycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", result.TotalCycles)
	}

	cycle := result.Cycles[0]
	if cycle.Size != 2 {
		t.Errorf("expected cycle size 2, got %d", cycle.Size)
	}
	if cycle.Severity != "low" {
		t.Errorf("expected severity low for size 2, got %s", cycle.Severity)
	}
	if len(cycle.Edges) == 0 {
		t.Error("expected edges in cycle")
	}
}

func TestDetect_TriangleCycle(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b", "c"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if result.TotalCycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", result.TotalCycles)
	}
	if result.Cycles[0].Size != 3 {
		t.Errorf("expected cycle size 3, got %d", result.Cycles[0].Size)
	}
	if result.Cycles[0].Severity != "medium" {
		t.Errorf("expected severity medium for size 3, got %s", result.Cycles[0].Severity)
	}
}

func TestDetect_LargeCycle(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b", "c", "d", "e"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"d"},
		"d": {"e"},
		"e": {"a"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if result.TotalCycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", result.TotalCycles)
	}
	if result.Cycles[0].Severity != "high" {
		t.Errorf("expected severity high for size 5, got %s", result.Cycles[0].Severity)
	}
}

func TestDetect_MultipleCycles(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b", "c", "d"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"a"},
		"c": {"d"},
		"d": {"c"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if result.TotalCycles != 2 {
		t.Fatalf("expected 2 cycles, got %d", result.TotalCycles)
	}
}

func TestDetect_MaxCyclesLimit(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b", "c", "d"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"a"},
		"c": {"d"},
		"d": {"c"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{MaxCycles: 1})
	if result.TotalCycles != 2 {
		t.Errorf("TotalCycles should still report 2, got %d", result.TotalCycles)
	}
	if len(result.Cycles) != 1 {
		t.Errorf("expected 1 cycle returned (limit), got %d", len(result.Cycles))
	}
}

func TestDetect_EdgeMetaAndBreakCost(t *testing.T) {
	d := NewDetector()
	nodes := []string{"a", "b", "c"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	edgeMeta := map[[2]string]EdgeMeta{
		{"a", "b"}: {Strength: 10},
		{"b", "c"}: {Strength: 2},
		{"c", "a"}: {Strength: 5},
	}
	result := d.Detect(nodes, adj, edgeMeta, DetectOptions{})
	if result.TotalCycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", result.TotalCycles)
	}

	cycle := result.Cycles[0]

	// The weakest edge (strength=2, b→c) should be recommended
	var recommended *CycleEdge
	for i := range cycle.Edges {
		if cycle.Edges[i].Recommended {
			recommended = &cycle.Edges[i]
		}
	}
	if recommended == nil {
		t.Fatal("expected a recommended edge")
	}
	if recommended.Strength != 2 {
		t.Errorf("expected recommended edge to have strength 2 (weakest), got %d", recommended.Strength)
	}

	// BreakCost should be normalized: 2/10 = 0.2
	if recommended.BreakCost < 0.19 || recommended.BreakCost > 0.21 {
		t.Errorf("expected break cost ~0.2, got %f", recommended.BreakCost)
	}

	// Strongest edge should have break cost 1.0
	for _, e := range cycle.Edges {
		if e.Strength == 10 && (e.BreakCost < 0.99 || e.BreakCost > 1.01) {
			t.Errorf("strongest edge should have break cost 1.0, got %f", e.BreakCost)
		}
	}
}

func TestDetect_SelfLoop(t *testing.T) {
	d := NewDetector()
	// A self-loop is a node pointing to itself — Tarjan treats it as SCC of size 1,
	// which we filter out (only size > 1 kept).
	nodes := []string{"a"}
	adj := map[string][]string{
		"a": {"a"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if len(result.Cycles) != 0 {
		t.Errorf("self-loops should not be reported as cycles, got %d", len(result.Cycles))
	}
}

func TestCycleSeverity(t *testing.T) {
	tests := []struct {
		size     int
		expected string
	}{
		{2, "low"},
		{3, "medium"},
		{4, "medium"},
		{5, "high"},
		{10, "high"},
	}

	for _, tc := range tests {
		got := cycleSeverity(tc.size)
		if got != tc.expected {
			t.Errorf("cycleSeverity(%d) = %q, want %q", tc.size, got, tc.expected)
		}
	}
}

func TestDetect_DisconnectedComponents(t *testing.T) {
	d := NewDetector()
	// Two disconnected subgraphs, one with cycle, one without
	nodes := []string{"a", "b", "x", "y", "z"}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"a"},
		"x": {"y"},
		"y": {"z"},
	}
	result := d.Detect(nodes, adj, nil, DetectOptions{})
	if result.TotalCycles != 1 {
		t.Errorf("expected 1 cycle (only in a↔b), got %d", result.TotalCycles)
	}
}
