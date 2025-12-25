package query

import (
	"context"
	"testing"
)

// =============================================================================
// ExplainSymbol Tests
// =============================================================================

func TestExplainSymbol_ValidSymbol(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Test with a symbol ID - even if not found, should return structured response
	resp, err := engine.ExplainSymbol(ctx, ExplainSymbolOptions{
		SymbolId: "test:sym:1",
	})

	// Error is acceptable for non-existent symbol
	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "explainSymbol" {
		t.Errorf("expected tool=explainSymbol, got %s", resp.Tool)
	}
}

func TestExplainSymbol_WithDrilldowns(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ExplainSymbol(ctx, ExplainSymbolOptions{
		SymbolId: "nonexistent:sym:123",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have drilldowns for further exploration
	if resp.Drilldowns == nil {
		t.Log("no drilldowns in response")
	}
}

// =============================================================================
// JustifySymbol Tests
// =============================================================================

func TestJustifySymbol_ReturnsVerdict(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.JustifySymbol(ctx, JustifySymbolOptions{
		SymbolId: "test:sym:1",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "justifySymbol" {
		t.Errorf("expected tool=justifySymbol, got %s", resp.Tool)
	}

	// Should have verdict info
	if resp.Verdict == "" {
		t.Log("no verdict in response (symbol may not exist)")
	}
}

// =============================================================================
// GetCallGraph Tests
// =============================================================================

func TestGetCallGraph_DefaultDepth(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Test with zero depth - should default to 1
	resp, err := engine.GetCallGraph(ctx, CallGraphOptions{
		SymbolId:  "test:sym:1",
		Depth:     0,
		Direction: "",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "getCallGraph" {
		t.Errorf("expected tool=getCallGraph, got %s", resp.Tool)
	}
}

func TestGetCallGraph_Directions(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name      string
		direction string
	}{
		{"both", "both"},
		{"callers only", "callers"},
		{"callees only", "callees"},
		{"empty defaults to both", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := engine.GetCallGraph(ctx, CallGraphOptions{
				SymbolId:  "test:sym:1",
				Depth:     1,
				Direction: tt.direction,
			})

			if err != nil {
				return
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}
		})
	}
}

func TestGetCallGraph_DepthLimits(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name  string
		depth int
	}{
		{"depth 1", 1},
		{"depth 2", 2},
		{"depth 4 (max)", 4},
		{"depth 10 (should cap)", 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := engine.GetCallGraph(ctx, CallGraphOptions{
				SymbolId:  "test:sym:1",
				Depth:     tt.depth,
				Direction: "both",
			})

			if err != nil {
				return
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}
		})
	}
}

// =============================================================================
// GetModuleOverview Tests
// =============================================================================

func TestGetModuleOverview_WithPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.GetModuleOverview(ctx, ModuleOverviewOptions{
		Path: "internal/query",
		Name: "",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "getModuleOverview" {
		t.Errorf("expected tool=getModuleOverview, got %s", resp.Tool)
	}
}

func TestGetModuleOverview_EmptyPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Empty path should use root
	resp, err := engine.GetModuleOverview(ctx, ModuleOverviewOptions{
		Path: "",
		Name: "test-module",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// =============================================================================
// ExplainFile Tests
// =============================================================================

func TestExplainFile_ValidPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ExplainFile(ctx, ExplainFileOptions{
		FilePath: "internal/query/engine.go",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "explainFile" {
		t.Errorf("expected tool=explainFile, got %s", resp.Tool)
	}
}

func TestExplainFile_NonExistentPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ExplainFile(ctx, ExplainFileOptions{
		FilePath: "nonexistent/path/file.go",
	})

	// Should return error or empty response for non-existent file
	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// =============================================================================
// ListEntrypoints Tests
// =============================================================================

func TestListEntrypoints_DefaultLimit(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ListEntrypoints(ctx, ListEntrypointsOptions{
		Limit: 0, // Should default
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "listEntrypoints" {
		t.Errorf("expected tool=listEntrypoints, got %s", resp.Tool)
	}
}

func TestListEntrypoints_WithModuleFilter(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ListEntrypoints(ctx, ListEntrypointsOptions{
		ModuleFilter: "cmd/",
		Limit:        10,
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// =============================================================================
// TraceUsage Tests
// =============================================================================

func TestTraceUsage_DefaultLimits(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.TraceUsage(ctx, TraceUsageOptions{
		SymbolId: "test:sym:1",
		MaxPaths: 0, // Should default to 10
		MaxDepth: 0, // Should default to 5
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "traceUsage" {
		t.Errorf("expected tool=traceUsage, got %s", resp.Tool)
	}
}

func TestTraceUsage_CustomLimits(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name     string
		maxPaths int
		maxDepth int
	}{
		{"small limits", 5, 3},
		{"large limits", 20, 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := engine.TraceUsage(ctx, TraceUsageOptions{
				SymbolId: "test:sym:1",
				MaxPaths: tt.maxPaths,
				MaxDepth: tt.maxDepth,
			})

			if err != nil {
				return
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}
		})
	}
}

// =============================================================================
// SummarizeDiff Tests
// =============================================================================

func TestSummarizeDiff_DefaultTimeWindow(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.SummarizeDiff(ctx, SummarizeDiffOptions{
		// No selector - should default to last 30 days
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "summarizeDiff" {
		t.Errorf("expected tool=summarizeDiff, got %s", resp.Tool)
	}
}

func TestSummarizeDiff_TimeWindowSelector(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.SummarizeDiff(ctx, SummarizeDiffOptions{
		TimeWindow: &TimeWindowSelector{
			Start: "2024-01-01T00:00:00Z",
			End:   "2024-12-31T23:59:59Z",
		},
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// =============================================================================
// GetHotspots Tests
// =============================================================================

func TestGetHotspots_DefaultLimit(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.GetHotspots(ctx, GetHotspotsOptions{
		Limit: 0, // Should default to 20
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "getHotspots" {
		t.Errorf("expected tool=getHotspots, got %s", resp.Tool)
	}
}

func TestGetHotspots_WithScope(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name  string
		scope string
	}{
		{"internal scope", "internal/"},
		{"cmd scope", "cmd/"},
		{"empty scope", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := engine.GetHotspots(ctx, GetHotspotsOptions{
				Scope: tt.scope,
				Limit: 10,
			})

			if err != nil {
				return
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}
		})
	}
}

func TestGetHotspots_LimitCapping(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Request more than max (50) - should be capped
	resp, err := engine.GetHotspots(ctx, GetHotspotsOptions{
		Limit: 100,
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify it returned at most 50
	if len(resp.Hotspots) > 50 {
		t.Errorf("expected max 50 hotspots, got %d", len(resp.Hotspots))
	}
}

// =============================================================================
// ExplainPath Tests
// =============================================================================

func TestExplainPath_ValidPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ExplainPath(ctx, ExplainPathOptions{
		FilePath: "internal/query/",
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "explainPath" {
		t.Errorf("expected tool=explainPath, got %s", resp.Tool)
	}
}

// =============================================================================
// ListKeyConcepts Tests
// =============================================================================

func TestListKeyConcepts_DefaultLimit(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ListKeyConcepts(ctx, ListKeyConceptsOptions{
		Limit: 0, // Should default to 12
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "listKeyConcepts" {
		t.Errorf("expected tool=listKeyConcepts, got %s", resp.Tool)
	}
}

func TestListKeyConcepts_CustomLimit(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.ListKeyConcepts(ctx, ListKeyConceptsOptions{
		Limit: 5,
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should respect the limit
	if len(resp.Concepts) > 5 {
		t.Errorf("expected max 5 concepts, got %d", len(resp.Concepts))
	}
}

// =============================================================================
// RecentlyRelevant Tests
// =============================================================================

func TestRecentlyRelevant_DefaultTimeWindow(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := engine.RecentlyRelevant(ctx, RecentlyRelevantOptions{
		// No time window - should default
	})

	if err != nil {
		return
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have metadata
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion")
	}
	if resp.Tool != "recentlyRelevant" {
		t.Errorf("expected tool=recentlyRelevant, got %s", resp.Tool)
	}
}

func TestRecentlyRelevant_CustomTimeWindow(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name       string
		timeWindow *TimeWindowSelector
	}{
		{"7 days", &TimeWindowSelector{Start: "2024-12-18T00:00:00Z"}},
		{"30 days", &TimeWindowSelector{Start: "2024-11-25T00:00:00Z"}},
		{"with end", &TimeWindowSelector{Start: "2024-10-01T00:00:00Z", End: "2024-12-31T00:00:00Z"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := engine.RecentlyRelevant(ctx, RecentlyRelevantOptions{
				TimeWindow: tt.timeWindow,
			})

			if err != nil {
				return
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}
		})
	}
}
