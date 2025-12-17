package scip

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
)

// findRepoRoot finds the repository root by looking for go.mod
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod")
		}
		dir = parent
	}
}

func TestFindCallers(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.RepoRoot = repoRoot

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.DebugLevel,
	})

	adapter, err := NewSCIPAdapter(cfg, logger)
	if err != nil {
		t.Skipf("SCIP adapter not available: %v", err)
	}

	if !adapter.IsAvailable() {
		t.Skip("SCIP index not available")
	}

	// Check index info
	indexInfo := adapter.GetIndexInfo()
	t.Logf("Index available: %v, docs: %d, symbols: %d",
		indexInfo.Available, indexInfo.DocumentCount, indexInfo.SymbolCount)

	// Find NewEngine via internal search
	var symbolId string
	results, err := adapter.index.SearchSymbols("NewEngine", SearchOptions{MaxResults: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	t.Logf("Found %d results for 'NewEngine'", len(results))
	for _, s := range results {
		t.Logf("  - %s: %s (kind=%s)", s.Name, s.StableId, s.Kind)
		if s.Name == "NewEngine" && s.Kind == KindFunction {
			symbolId = s.StableId
		}
	}

	if symbolId == "" {
		t.Skip("NewEngine function not found")
	}

	t.Logf("Testing FindCallers for: %s", symbolId)

	// Test callers
	callers, err := adapter.index.FindCallers(symbolId)
	if err != nil {
		t.Errorf("FindCallers error: %v", err)
	} else {
		t.Logf("Found %d callers:", len(callers))
		for _, c := range callers {
			t.Logf("  - %s (%s)", c.Name, c.SymbolID)
		}
	}

	// Test callees
	callees, err := adapter.index.FindCallees(symbolId)
	if err != nil {
		t.Errorf("FindCallees error: %v", err)
	} else {
		t.Logf("Found %d callees:", len(callees))
		for _, c := range callees {
			t.Logf("  - %s (%s)", c.Name, c.SymbolID)
		}
	}

	// Debug: Check raw occurrences for this symbol
	t.Log("\n=== Debug: Checking raw occurrences ===")
	for _, doc := range adapter.index.Documents {
		for _, occ := range doc.Occurrences {
			if occ.Symbol == symbolId {
				t.Logf("Occurrence in %s at line %d, roles=%d, enclosing=%v",
					doc.RelativePath, occ.Range[0], occ.SymbolRoles, occ.EnclosingRange)
			}
		}
	}

	// Debug: Check what EnclosingRange looks like for function defs
	t.Log("\n=== Debug: Sample function definitions with enclosing ranges ===")
	count := 0
	for _, doc := range adapter.index.Documents {
		if count >= 5 {
			break
		}
		for _, sym := range doc.Symbols {
			if mapSCIPKind(sym.Kind) == KindFunction {
				for _, occ := range doc.Occurrences {
					if occ.Symbol == sym.Symbol && occ.SymbolRoles&SymbolRoleDefinition != 0 {
						t.Logf("Function %s: range=%v, enclosing=%v",
							extractSymbolName(sym.Symbol), occ.Range, occ.EnclosingRange)
						count++
						break
					}
				}
			}
		}
	}

	// Debug: Check function ranges in engine_helper.go where NewEngine is called
	t.Log("\n=== Debug: Functions in cmd/ckb/engine_helper.go ===")
	for _, doc := range adapter.index.Documents {
		if doc.RelativePath == "cmd/ckb/engine_helper.go" {
			t.Logf("Document: %s, symbols: %d", doc.RelativePath, len(doc.Symbols))
			funcRanges := buildFunctionRanges(doc)
			t.Logf("Found %d function ranges", len(funcRanges))
			for sym, r := range funcRanges {
				t.Logf("  Function %s: lines %d-%d", extractSymbolName(sym), r.start, r.end)
			}
			// Also show symbol kinds and raw data
			for _, sym := range doc.Symbols {
				kind := mapSCIPKind(sym.Kind)
				t.Logf("  Symbol %s: rawKind=%d mapped=%s displayName=%q",
					sym.Symbol, sym.Kind, kind, sym.DisplayName)
			}
		}
	}
}

func TestCallGraph(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.RepoRoot = repoRoot

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.DebugLevel,
	})

	adapter, err := NewSCIPAdapter(cfg, logger)
	if err != nil {
		t.Skipf("SCIP adapter not available: %v", err)
	}

	if !adapter.IsAvailable() {
		t.Skip("SCIP index not available")
	}

	// Find a good function to test with
	results, err := adapter.index.SearchSymbols("NewEngine", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	var symbolId string
	for _, s := range results {
		if s.Name == "NewEngine" && s.Kind == KindFunction {
			symbolId = s.StableId
			break
		}
	}

	if symbolId == "" {
		t.Skip("NewEngine function not found")
	}

	t.Logf("Testing call graph for: %s", symbolId)

	graph, err := adapter.BuildCallGraph(symbolId, CallGraphOptions{
		Direction: DirectionBoth,
		MaxDepth:  2,
		MaxNodes:  50,
	})

	if err != nil {
		t.Fatalf("BuildCallGraph error: %v", err)
	}

	t.Logf("Call graph: %d nodes, %d edges", len(graph.Nodes), len(graph.Edges))
	t.Logf("Direct callers: %d", len(graph.Callers))
	t.Logf("Direct callees: %d", len(graph.Callees))

	for _, caller := range graph.Callers {
		t.Logf("  Caller: %s", caller.Name)
	}
	for _, callee := range graph.Callees {
		t.Logf("  Callee: %s", callee.Name)
	}
}

func TestCountSymbolsByPath(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.RepoRoot = repoRoot

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.DebugLevel,
	})

	adapter, err := NewSCIPAdapter(cfg, logger)
	if err != nil {
		t.Skipf("SCIP adapter not available: %v", err)
	}

	if !adapter.IsAvailable() {
		t.Skip("SCIP index not available")
	}

	// Test symbol counting
	paths := []string{
		"internal/query",
		"internal/backends",
		"internal/mcp",
		"cmd/ckb",
		".",
	}
	for _, p := range paths {
		count := adapter.CountSymbolsByPath(p)
		t.Logf("Path %q: %d symbols", p, count)
	}

	// Also log some sample document paths
	t.Log("\n=== Sample document paths ===")
	indexInfo := adapter.GetIndexInfo()
	t.Logf("Total documents: %d", indexInfo.DocumentCount)

	// List a few document paths from the index
	for i, doc := range adapter.index.Documents {
		if i >= 10 {
			break
		}
		t.Logf("  Doc %d: %s (symbols: %d)", i, doc.RelativePath, len(doc.Symbols))
	}
}

func TestIsFunctionSymbol(t *testing.T) {
	tests := []struct {
		name     string
		symbolId string
		want     bool
	}{
		// Functions should return true
		{
			name:     "function",
			symbolId: "scip-go go ckb/internal/query NewEngine().",
			want:     true,
		},
		{
			name:     "method",
			symbolId: "scip-go go ckb/internal/query Engine#Close().",
			want:     true,
		},
		{
			name:     "function with params",
			symbolId: "scip-go go fmt Printf().",
			want:     true,
		},
		// Non-functions should return false
		{
			name:     "type/struct",
			symbolId: "scip-go go ckb/internal/query Engine#",
			want:     false,
		},
		{
			name:     "field",
			symbolId: "scip-go go ckb/internal/query Engine#logger.",
			want:     false,
		},
		{
			name:     "package",
			symbolId: "scip-go go ckb/internal/query/",
			want:     false,
		},
		{
			name:     "variable",
			symbolId: "scip-go go ckb/internal/config DefaultConfig.",
			want:     false,
		},
		{
			name:     "empty string",
			symbolId: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFunctionSymbol(tt.symbolId)
			if got != tt.want {
				t.Errorf("isFunctionSymbol(%q) = %v, want %v", tt.symbolId, got, tt.want)
			}
		})
	}
}
