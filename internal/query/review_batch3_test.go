package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/backends/git"
)

func TestClassifyChanges_NewFile(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	diffStats := []git.DiffStats{
		{FilePath: "pkg/new.go", Additions: 100, IsNew: true},
	}

	breakdown := engine.classifyChanges(ctx, diffStats, map[string]bool{}, ReviewPROptions{})
	if len(breakdown.Classifications) != 1 {
		t.Fatalf("expected 1 classification, got %d", len(breakdown.Classifications))
	}

	c := breakdown.Classifications[0]
	if c.Category != CategoryNew {
		t.Errorf("expected category %q, got %q", CategoryNew, c.Category)
	}
	if c.ReviewPriority != "high" {
		t.Errorf("expected priority 'high', got %q", c.ReviewPriority)
	}
}

func TestClassifyChanges_RenamedFile(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	diffStats := []git.DiffStats{
		{FilePath: "pkg/new_name.go", IsRenamed: true, OldPath: "pkg/old_name.go", Additions: 1, Deletions: 1},
	}

	breakdown := engine.classifyChanges(ctx, diffStats, map[string]bool{}, ReviewPROptions{})
	c := breakdown.Classifications[0]
	if c.Category != CategoryMoved {
		t.Errorf("expected category %q, got %q", CategoryMoved, c.Category)
	}
	if c.ReviewPriority != "low" {
		t.Errorf("expected priority 'low' for pure rename, got %q", c.ReviewPriority)
	}
}

func TestClassifyChanges_TestFile(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	diffStats := []git.DiffStats{
		{FilePath: "pkg/handler_test.go", Additions: 20, Deletions: 5},
	}

	breakdown := engine.classifyChanges(ctx, diffStats, map[string]bool{}, ReviewPROptions{})
	c := breakdown.Classifications[0]
	if c.Category != CategoryTest {
		t.Errorf("expected category %q, got %q", CategoryTest, c.Category)
	}
}

func TestClassifyChanges_ConfigFile(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	diffStats := []git.DiffStats{
		{FilePath: "go.mod", Additions: 3, Deletions: 1},
		{FilePath: "Dockerfile", Additions: 5, Deletions: 2},
	}

	breakdown := engine.classifyChanges(ctx, diffStats, map[string]bool{}, ReviewPROptions{})
	for _, c := range breakdown.Classifications {
		if c.Category != CategoryConfig {
			t.Errorf("expected %q to be classified as config, got %q", c.File, c.Category)
		}
	}
}

func TestClassifyChanges_GeneratedFile(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	diffStats := []git.DiffStats{
		{FilePath: "types.pb.go", Additions: 500, Deletions: 300},
	}
	generatedSet := map[string]bool{"types.pb.go": true}

	breakdown := engine.classifyChanges(ctx, diffStats, generatedSet, ReviewPROptions{})
	c := breakdown.Classifications[0]
	if c.Category != CategoryGenerated {
		t.Errorf("expected category %q, got %q", CategoryGenerated, c.Category)
	}
	if c.ReviewPriority != "skip" {
		t.Errorf("expected priority 'skip', got %q", c.ReviewPriority)
	}
}

func TestClassifyChanges_Summary(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	diffStats := []git.DiffStats{
		{FilePath: "new.go", Additions: 100, IsNew: true},
		{FilePath: "test_util.go", Additions: 20, IsNew: true},  // new, not test (no _test.go)
		{FilePath: "handler_test.go", Additions: 50, Deletions: 10},
		{FilePath: "go.mod", Additions: 2, Deletions: 1},
	}

	breakdown := engine.classifyChanges(ctx, diffStats, map[string]bool{}, ReviewPROptions{})
	if breakdown.Summary[CategoryNew] < 1 {
		t.Errorf("expected at least 1 new file in summary")
	}
	if breakdown.Summary[CategoryTest] < 1 {
		t.Errorf("expected at least 1 test file in summary")
	}
}

func TestEstimateReviewEffort_Empty(t *testing.T) {
	t.Parallel()

	effort := estimateReviewEffort(nil, nil, 0, 0)
	if effort.EstimatedMinutes != 0 {
		t.Errorf("expected 0 minutes for empty PR, got %d", effort.EstimatedMinutes)
	}
	if effort.Complexity != "trivial" {
		t.Errorf("expected complexity 'trivial', got %q", effort.Complexity)
	}
}

func TestEstimateReviewEffort_SmallPR(t *testing.T) {
	t.Parallel()

	diffStats := []git.DiffStats{
		{FilePath: "main.go", Additions: 10, Deletions: 5},
	}

	effort := estimateReviewEffort(diffStats, nil, 0, 1)
	if effort.EstimatedMinutes < 5 {
		t.Errorf("expected at least 5 minutes, got %d", effort.EstimatedMinutes)
	}
	if effort.Complexity == "very-complex" {
		t.Error("small PR should not be very-complex")
	}
}

func TestEstimateReviewEffort_LargePR(t *testing.T) {
	t.Parallel()

	// 50 files, ~2000 LOC, 5 modules, 3 critical
	diffStats := make([]git.DiffStats, 50)
	for i := range diffStats {
		diffStats[i] = git.DiffStats{
			FilePath:  fmt.Sprintf("mod%d/file%d.go", i%5, i),
			Additions: 30,
			Deletions: 10,
		}
	}

	effort := estimateReviewEffort(diffStats, nil, 3, 5)
	if effort.EstimatedMinutes < 60 {
		t.Errorf("expected large PR to take > 60 min, got %d", effort.EstimatedMinutes)
	}
	if effort.Complexity != "complex" && effort.Complexity != "very-complex" {
		t.Errorf("expected complexity 'complex' or 'very-complex', got %q", effort.Complexity)
	}
	if len(effort.Factors) == 0 {
		t.Error("expected factors to be populated")
	}
}

func TestEstimateReviewEffort_WithClassification(t *testing.T) {
	t.Parallel()

	diffStats := []git.DiffStats{
		{FilePath: "new.go", Additions: 200, IsNew: true},
		{FilePath: "types.pb.go", Additions: 1000},
	}
	breakdown := &ChangeBreakdown{
		Classifications: []ChangeClassification{
			{File: "new.go", Category: CategoryNew},
			{File: "types.pb.go", Category: CategoryGenerated},
		},
	}

	effort := estimateReviewEffort(diffStats, breakdown, 0, 1)
	// Generated files should be excluded from LOC calculation
	// So the effort should be driven mainly by 200 LOC of new code
	if effort.EstimatedMinutes > 120 {
		t.Errorf("generated files inflating estimate too much: %d min", effort.EstimatedMinutes)
	}
}

func TestSuggestPRSplit_BelowThreshold(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	policy := DefaultReviewPolicy()
	policy.SplitThreshold = 50

	// Only 5 files — below threshold
	diffStats := make([]git.DiffStats, 5)
	for i := range diffStats {
		diffStats[i] = git.DiffStats{FilePath: fmt.Sprintf("pkg/file%d.go", i)}
	}

	result := engine.suggestPRSplit(ctx, diffStats, policy)
	if result != nil {
		t.Error("expected nil split suggestion below threshold")
	}
}

func TestSuggestPRSplit_MultiModule(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	policy := DefaultReviewPolicy()
	policy.SplitThreshold = 3 // Low threshold for testing

	// Files in two distinct modules with no coupling
	diffStats := []git.DiffStats{
		{FilePath: "frontend/components/app.tsx", Additions: 50},
		{FilePath: "frontend/components/nav.tsx", Additions: 30},
		{FilePath: "backend/api/handler.go", Additions: 40},
		{FilePath: "backend/api/routes.go", Additions: 20},
	}

	result := engine.suggestPRSplit(ctx, diffStats, policy)
	if result == nil {
		t.Fatal("expected split suggestion for multi-module PR")
	}
	if !result.ShouldSplit {
		t.Error("expected ShouldSplit=true for files in different modules")
	}
	if len(result.Clusters) < 2 {
		t.Errorf("expected at least 2 clusters, got %d", len(result.Clusters))
	}
}

func TestSuggestPRSplit_SingleModule(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	policy := DefaultReviewPolicy()
	policy.SplitThreshold = 3

	// All files in the same module
	diffStats := []git.DiffStats{
		{FilePath: "pkg/api/handler.go", Additions: 50},
		{FilePath: "pkg/api/routes.go", Additions: 30},
		{FilePath: "pkg/api/middleware.go", Additions: 40},
	}

	result := engine.suggestPRSplit(ctx, diffStats, policy)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ShouldSplit {
		t.Error("expected ShouldSplit=false for single-module PR")
	}
}

func TestBFS(t *testing.T) {
	t.Parallel()

	adj := map[string]map[string]bool{
		"a": {"b": true},
		"b": {"a": true, "c": true},
		"c": {"b": true},
		"d": {}, // isolated
	}
	visited := make(map[string]bool)

	component := bfs("a", adj, visited)
	if len(component) != 3 {
		t.Errorf("expected component of 3, got %d: %v", len(component), component)
	}

	// d should not be visited
	if visited["d"] {
		t.Error("d should not be visited from a")
	}

	// d forms its own component
	component2 := bfs("d", adj, visited)
	if len(component2) != 1 {
		t.Errorf("expected isolated component of 1, got %d", len(component2))
	}
}

func TestIsConfigFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{"go.mod", true},
		{"go.sum", true},
		{"Dockerfile", true},
		{"Makefile", true},
		{"package.json", true},
		{".github/workflows/ci.yml", true},
		{"main.go", false},
		{"src/app.ts", false},
		{"README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isConfigFile(tt.path)
			if got != tt.expected {
				t.Errorf("isConfigFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestReviewPR_IncludesEffort(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/main.go": "package main\n\nfunc main() {}\n",
		"pkg/util.go": "package main\n\nfunc helper() {}\n",
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	if resp.ReviewEffort == nil {
		t.Fatal("expected ReviewEffort to be populated")
	}
	if resp.ReviewEffort.EstimatedMinutes < 5 {
		t.Errorf("expected at least 5 minutes, got %d", resp.ReviewEffort.EstimatedMinutes)
	}
	if resp.ReviewEffort.Complexity == "" {
		t.Error("expected complexity to be set")
	}
}
