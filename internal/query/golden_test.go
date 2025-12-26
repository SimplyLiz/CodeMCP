package query

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/backends/scip"
	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/storage"
	"ckb/internal/testutil"
)

// setupGoldenEngine creates a query engine using a fixture's SCIP index.
func setupGoldenEngine(t *testing.T, fixture *testutil.FixtureContext) (*Engine, func()) {
	t.Helper()

	// Create temp directory for CKB storage
	tmpDir, err := os.MkdirTemp("", "ckb-golden-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create .ckb directory
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create .ckb dir: %v", err)
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.JSONFormat,
		Level:  logging.ErrorLevel,
	})

	// Create storage in temp dir
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open storage: %v", err)
	}

	// Create config pointing to fixture
	cfg := config.DefaultConfig()
	cfg.RepoRoot = fixture.Root
	cfg.Backends.Scip.Enabled = true
	cfg.Backends.Scip.IndexPath = fixture.SCIPPath // Use absolute path to fixture's index

	// Create engine
	engine, err := NewEngine(fixture.Root, db, logger, cfg)
	if err != nil {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Ensure SCIP backend is loaded with fixture index
	scipBackend := engine.GetScipBackend()
	if scipBackend != nil {
		if loadErr := scipBackend.LoadIndex(); loadErr != nil {
			t.Logf("Warning: Failed to load SCIP index: %v", loadErr)
		}
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return engine, cleanup
}

// TestGolden_SearchSymbols tests SearchSymbols against golden files.
func TestGolden_SearchSymbols(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		engine, cleanup := setupGoldenEngine(t, fixture)
		defer cleanup()

		ctx := context.Background()

		testCases := []struct {
			name  string
			query string
			limit int
		}{
			{"search_handler", "Handler", 50},
			{"search_service", "Service", 50},
			{"search_model", "Model", 50},
			{"search_main", "main", 50},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp, err := engine.SearchSymbols(ctx, SearchSymbolsOptions{
					Query: tc.query,
					Limit: tc.limit,
				})
				if err != nil {
					t.Fatalf("SearchSymbols failed: %v", err)
				}

				// Normalize the response for golden comparison
				result := normalizeSearchResults(resp)
				testutil.CompareGolden(t, fixture, tc.name, result)
			})
		}
	})
}

// TestGolden_GetCallGraph tests GetCallGraph against golden files.
func TestGolden_GetCallGraph(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		engine, cleanup := setupGoldenEngine(t, fixture)
		defer cleanup()

		ctx := context.Background()

		// First, find a symbol to get call graph for
		searchResp, err := engine.SearchSymbols(ctx, SearchSymbolsOptions{
			Query: "main",
			Limit: 5,
		})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		if len(searchResp.Symbols) == 0 {
			t.Skip("No symbols found for call graph test")
		}

		// Find the main function specifically
		var mainSymbolID string
		for _, sym := range searchResp.Symbols {
			if sym.Name == "main" && sym.Kind == "function" {
				mainSymbolID = sym.StableId
				break
			}
		}

		if mainSymbolID == "" {
			t.Skip("main function not found")
		}

		testCases := []struct {
			name      string
			symbolID  string
			depth     int
			direction string
		}{
			{"callgraph_main_depth1", mainSymbolID, 1, "both"},
			{"callgraph_main_depth2", mainSymbolID, 2, "both"},
			{"callgraph_main_callees", mainSymbolID, 2, "callees"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp, err := engine.GetCallGraph(ctx, CallGraphOptions{
					SymbolId:  tc.symbolID,
					Depth:     tc.depth,
					Direction: tc.direction,
				})
				if err != nil {
					t.Fatalf("GetCallGraph failed: %v", err)
				}

				// Normalize for golden comparison
				result := normalizeCallGraph(resp)
				testutil.CompareGolden(t, fixture, tc.name, result)
			})
		}
	})
}

// TestGolden_FindReferences tests FindReferences against golden files.
func TestGolden_FindReferences(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		engine, cleanup := setupGoldenEngine(t, fixture)
		defer cleanup()

		ctx := context.Background()

		// Find a symbol with references
		searchResp, err := engine.SearchSymbols(ctx, SearchSymbolsOptions{
			Query: "FormatOutput",
			Limit: 5,
		})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		if len(searchResp.Symbols) == 0 {
			t.Skip("No FormatOutput symbol found")
		}

		symbolID := searchResp.Symbols[0].StableId

		t.Run("refs_FormatOutput", func(t *testing.T) {
			resp, err := engine.FindReferences(ctx, FindReferencesOptions{
				SymbolId: symbolID,
				Limit:    100,
			})
			if err != nil {
				t.Fatalf("FindReferences failed: %v", err)
			}

			result := normalizeReferences(resp)
			testutil.CompareGolden(t, fixture, "refs_FormatOutput", result)
		})
	})
}

// TestGolden_ExplainSymbol tests ExplainSymbol against golden files.
func TestGolden_ExplainSymbol(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		engine, cleanup := setupGoldenEngine(t, fixture)
		defer cleanup()

		ctx := context.Background()

		// Find symbols to explain
		// Use specific names to avoid non-deterministic results
		testCases := []struct {
			searchQuery string
			goldenName  string
		}{
			{"NewHandler", "explain_NewHandler"},
			{"DefaultService", "explain_DefaultService"},
			{"FormatOutput", "explain_FormatOutput"},
		}

		for _, tc := range testCases {
			t.Run(tc.goldenName, func(t *testing.T) {
				// Find the symbol first
				searchResp, err := engine.SearchSymbols(ctx, SearchSymbolsOptions{
					Query: tc.searchQuery,
					Limit: 1,
				})
				if err != nil {
					t.Fatalf("SearchSymbols failed: %v", err)
				}
				if len(searchResp.Symbols) == 0 {
					t.Skipf("No %s symbol found", tc.searchQuery)
				}

				symbolID := searchResp.Symbols[0].StableId

				resp, err := engine.ExplainSymbol(ctx, ExplainSymbolOptions{
					SymbolId: symbolID,
				})
				if err != nil {
					t.Fatalf("ExplainSymbol failed: %v", err)
				}

				result := normalizeExplainSymbol(resp)
				testutil.CompareGolden(t, fixture, tc.goldenName, result)
			})
		}
	})
}

// TestGolden_GetArchitecture tests GetArchitecture against golden files.
func TestGolden_GetArchitecture(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		engine, cleanup := setupGoldenEngine(t, fixture)
		defer cleanup()

		ctx := context.Background()

		t.Run("arch_default", func(t *testing.T) {
			resp, err := engine.GetArchitecture(ctx, GetArchitectureOptions{
				Depth:               2,
				IncludeExternalDeps: false,
			})
			if err != nil {
				t.Fatalf("GetArchitecture failed: %v", err)
			}

			result := normalizeArchitecture(resp)
			testutil.CompareGolden(t, fixture, "arch_default", result)
		})
	})
}

// TestGolden_TraceUsage tests TraceUsage against golden files.
func TestGolden_TraceUsage(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		engine, cleanup := setupGoldenEngine(t, fixture)
		defer cleanup()

		ctx := context.Background()

		// Find an internal symbol to trace
		searchResp, err := engine.SearchSymbols(ctx, SearchSymbolsOptions{
			Query: "FormatOutput",
			Limit: 1,
		})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}
		if len(searchResp.Symbols) == 0 {
			t.Skip("No FormatOutput symbol found")
		}

		symbolID := searchResp.Symbols[0].StableId

		t.Run("trace_FormatOutput", func(t *testing.T) {
			resp, err := engine.TraceUsage(ctx, TraceUsageOptions{
				SymbolId: symbolID,
				MaxPaths: 10,
				MaxDepth: 5,
			})
			if err != nil {
				t.Fatalf("TraceUsage failed: %v", err)
			}

			result := normalizeTraceUsage(resp)
			testutil.CompareGolden(t, fixture, "trace_FormatOutput", result)
		})
	})
}

// normalizeSearchResults normalizes SearchSymbolsResponse for golden comparison.
func normalizeSearchResults(resp *SearchSymbolsResponse) map[string]any {
	results := make([]map[string]any, 0, len(resp.Symbols))
	for _, r := range resp.Symbols {
		file := ""
		line := 0
		if r.Location != nil {
			file = r.Location.FileId
			line = r.Location.StartLine
		}
		results = append(results, map[string]any{
			"name":     r.Name,
			"kind":     r.Kind,
			"moduleId": r.ModuleId,
			"file":     normalizeFilePath(file),
			"line":     line,
		})
	}

	return map[string]any{
		"symbols": results,
		"total":   resp.TotalCount,
	}
}

// normalizeCallGraph normalizes CallGraphResponse for golden comparison.
func normalizeCallGraph(resp *CallGraphResponse) map[string]any {
	nodes := make([]map[string]any, 0, len(resp.Nodes))
	for _, n := range resp.Nodes {
		file := ""
		if n.Location != nil {
			file = n.Location.FileId
		}
		nodes = append(nodes, map[string]any{
			"id":   n.ID,
			"name": n.Name,
			"file": normalizeFilePath(file),
			"role": n.Role,
		})
	}

	edges := make([]map[string]any, 0, len(resp.Edges))
	for _, e := range resp.Edges {
		edges = append(edges, map[string]any{
			"from": e.From,
			"to":   e.To,
		})
	}

	return map[string]any{
		"root":  resp.Root,
		"nodes": nodes,
		"edges": edges,
	}
}

// normalizeReferences normalizes FindReferencesResponse for golden comparison.
func normalizeReferences(resp *FindReferencesResponse) map[string]any {
	refs := make([]map[string]any, 0, len(resp.References))
	for _, r := range resp.References {
		file := ""
		line := 0
		column := 0
		if r.Location != nil {
			file = r.Location.FileId
			line = r.Location.StartLine
			column = r.Location.StartColumn
		}
		refs = append(refs, map[string]any{
			"file":    normalizeFilePath(file),
			"line":    line,
			"column":  column,
			"context": r.Context,
			"kind":    r.Kind,
		})
	}

	return map[string]any{
		"references": refs,
		"total":      resp.TotalCount,
	}
}

// normalizeFilePath strips absolute path prefixes for stable comparison.
func normalizeFilePath(path string) string {
	// Get just the relative part after the fixture root
	// This handles paths like /tmp/.../testdata/fixtures/go/pkg/handler.go
	parts := []string{"pkg/", "internal/", "main.go"}
	for _, p := range parts {
		if idx := indexLast(path, p); idx != -1 {
			return path[idx:]
		}
	}
	// Fallback: return just the filename
	return filepath.Base(path)
}

func indexLast(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// normalizeExplainSymbol normalizes ExplainSymbolResponse for golden comparison.
func normalizeExplainSymbol(resp *ExplainSymbolResponse) map[string]any {
	facts := map[string]any{
		"module": resp.Facts.Module,
	}

	if resp.Facts.Symbol != nil {
		file := ""
		if resp.Facts.Symbol.Location != nil {
			file = resp.Facts.Symbol.Location.FileId
		}
		facts["symbol"] = map[string]any{
			"name": resp.Facts.Symbol.Name,
			"kind": resp.Facts.Symbol.Kind,
			"file": normalizeFilePath(file),
		}
	}

	if resp.Facts.Usage != nil {
		facts["usage"] = map[string]any{
			"callerCount":    resp.Facts.Usage.CallerCount,
			"calleeCount":    resp.Facts.Usage.CalleeCount,
			"referenceCount": resp.Facts.Usage.ReferenceCount,
		}
	}

	return map[string]any{
		"facts": facts,
		"summary": map[string]any{
			"tldr":     resp.Summary.Tldr,
			"identity": resp.Summary.Identity,
			"usage":    resp.Summary.Usage,
		},
	}
}

// normalizeArchitecture normalizes GetArchitectureResponse for golden comparison.
func normalizeArchitecture(resp *GetArchitectureResponse) map[string]any {
	modules := make([]map[string]any, 0, len(resp.Modules))
	for _, m := range resp.Modules {
		modules = append(modules, map[string]any{
			"moduleId":    m.ModuleId,
			"name":        m.Name,
			"path":        m.Path,
			"symbolCount": m.SymbolCount,
			"fileCount":   m.FileCount,
		})
	}

	deps := make([]map[string]any, 0, len(resp.DependencyGraph))
	for _, d := range resp.DependencyGraph {
		deps = append(deps, map[string]any{
			"from": d.From,
			"to":   d.To,
		})
	}

	return map[string]any{
		"modules":         modules,
		"dependencyGraph": deps,
	}
}

// normalizeTraceUsage normalizes TraceUsageResponse for golden comparison.
func normalizeTraceUsage(resp *TraceUsageResponse) map[string]any {
	paths := make([]map[string]any, 0, len(resp.Paths))
	for _, p := range resp.Paths {
		nodes := make([]map[string]any, 0, len(p.Nodes))
		for _, n := range p.Nodes {
			file := ""
			if n.Location != nil {
				file = n.Location.FileId
			}
			nodes = append(nodes, map[string]any{
				"name": n.Name,
				"kind": n.Kind,
				"role": n.Role,
				"file": normalizeFilePath(file),
			})
		}
		paths = append(paths, map[string]any{
			"pathType": p.PathType,
			"nodes":    nodes,
		})
	}

	return map[string]any{
		"targetSymbol":    resp.TargetSymbol,
		"paths":           paths,
		"totalPathsFound": resp.TotalPathsFound,
	}
}

// TestGolden_SCIPBackendDirect tests SCIP backend methods directly.
func TestGolden_SCIPBackendDirect(t *testing.T) {
	testutil.ForEachLanguage(t, func(t *testing.T, fixture *testutil.FixtureContext) {
		logger := logging.NewLogger(logging.Config{
			Format: logging.JSONFormat,
			Level:  logging.ErrorLevel,
		})

		// Create config for SCIP adapter
		cfg := config.DefaultConfig()
		cfg.RepoRoot = fixture.Root
		cfg.Backends.Scip.Enabled = true
		cfg.Backends.Scip.IndexPath = fixture.SCIPPath

		// Load SCIP index directly
		adapter, err := scip.NewSCIPAdapter(cfg, logger)
		if err != nil {
			t.Fatalf("Failed to create SCIP adapter: %v", err)
		}

		t.Run("scip_all_symbols", func(t *testing.T) {
			symbols := adapter.AllSymbols()

			// Normalize for golden comparison
			result := make([]map[string]any, 0, len(symbols))
			for _, s := range symbols {
				result = append(result, map[string]any{
					"symbol": s.Symbol,
					"kind":   s.Kind,
				})
			}

			testutil.CompareGolden(t, fixture, "scip_all_symbols", result)
		})
	})
}
