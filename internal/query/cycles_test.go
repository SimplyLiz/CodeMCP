package query

import (
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/cycles"
)

func TestExtractGraph_Module(t *testing.T) {
	arch := &GetArchitectureResponse{
		Modules: []ModuleSummary{
			{ModuleId: "mod-a"},
			{ModuleId: "mod-b"},
		},
		DependencyGraph: []DependencyEdge{
			{From: "mod-a", To: "mod-b", Strength: 5},
		},
	}

	nodes, adj, meta := extractGraph(arch, "module")

	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
	if len(adj["mod-a"]) != 1 || adj["mod-a"][0] != "mod-b" {
		t.Error("expected mod-a → mod-b edge")
	}
	if meta[[2]string{"mod-a", "mod-b"}].Strength != 5 {
		t.Error("expected strength 5")
	}
}

func TestExtractGraph_Directory(t *testing.T) {
	arch := &GetArchitectureResponse{
		Directories: []DirectorySummary{
			{Path: "internal/query"},
			{Path: "internal/mcp"},
		},
		DirectoryDependencies: []DirectoryDependencyEdge{
			{From: "internal/query", To: "internal/mcp", ImportCount: 3},
		},
	}

	nodes, adj, meta := extractGraph(arch, "directory")

	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
	if len(adj["internal/query"]) != 1 {
		t.Error("expected internal/query → internal/mcp edge")
	}
	if meta[[2]string{"internal/query", "internal/mcp"}].Strength != 3 {
		t.Error("expected strength 3 (ImportCount)")
	}
}

func TestExtractGraph_File(t *testing.T) {
	arch := &GetArchitectureResponse{
		Files: []FileSummary{
			{Path: "a.go"},
			{Path: "b.go"},
		},
		FileDependencies: []FileDependencyEdge{
			{From: "a.go", To: "b.go", Resolved: true},
		},
	}

	nodes, adj, meta := extractGraph(arch, "file")

	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
	if len(adj["a.go"]) != 1 {
		t.Error("expected a.go → b.go edge")
	}
	// Resolved edges get strength 2
	if meta[[2]string{"a.go", "b.go"}].Strength != 2 {
		t.Errorf("expected strength 2 for resolved edge, got %d", meta[[2]string{"a.go", "b.go"}].Strength)
	}
}

func TestExtractGraph_FileUnresolved(t *testing.T) {
	arch := &GetArchitectureResponse{
		Files: []FileSummary{{Path: "a.go"}, {Path: "b.go"}},
		FileDependencies: []FileDependencyEdge{
			{From: "a.go", To: "b.go", Resolved: false},
		},
	}

	_, _, meta := extractGraph(arch, "file")
	if meta[[2]string{"a.go", "b.go"}].Strength != 1 {
		t.Errorf("expected strength 1 for unresolved edge, got %d", meta[[2]string{"a.go", "b.go"}].Strength)
	}
}

func TestExtractGraph_Empty(t *testing.T) {
	arch := &GetArchitectureResponse{}

	nodes, adj, meta := extractGraph(arch, "directory")

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(adj) != 0 {
		t.Errorf("expected empty adjacency, got %d entries", len(adj))
	}
	if len(meta) != 0 {
		t.Errorf("expected empty meta, got %d entries", len(meta))
	}
}

func TestBuildCycleSummary_Empty(t *testing.T) {
	summary := buildCycleSummary(nil)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.HighSeverity != 0 || summary.MediumSeverity != 0 || summary.LowSeverity != 0 {
		t.Error("expected all zeroes for empty input")
	}
	if summary.LargestCycle != 0 || summary.AvgCycleSize != 0 {
		t.Error("expected zero largest/avg for empty input")
	}
}

func TestBuildCycleSummary_Mixed(t *testing.T) {
	detected := []cycles.Cycle{
		{Size: 2, Severity: "low"},
		{Size: 3, Severity: "medium"},
		{Size: 5, Severity: "high"},
		{Size: 6, Severity: "high"},
	}

	summary := buildCycleSummary(detected)

	if summary.LowSeverity != 1 {
		t.Errorf("expected 1 low, got %d", summary.LowSeverity)
	}
	if summary.MediumSeverity != 1 {
		t.Errorf("expected 1 medium, got %d", summary.MediumSeverity)
	}
	if summary.HighSeverity != 2 {
		t.Errorf("expected 2 high, got %d", summary.HighSeverity)
	}
	if summary.LargestCycle != 6 {
		t.Errorf("expected largest cycle 6, got %d", summary.LargestCycle)
	}
	expectedAvg := float64(2+3+5+6) / 4.0
	if summary.AvgCycleSize != expectedAvg {
		t.Errorf("expected avg %.2f, got %.2f", expectedAvg, summary.AvgCycleSize)
	}
}
