package mcp

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/query"
	"ckb/internal/storage"
	"ckb/internal/version"
)

// Wide-result token budgets for CI regression detection.
// These are per-response limits to catch token bloat.
const (
	// Wide-result tool output budgets (bytes)
	// These should decrease with frontierMode implementation
	maxCallGraphBytes     = 15000 // ~3750 tokens
	maxFindReferencesBytes = 12000 // ~3000 tokens
	maxAnalyzeImpactBytes  = 16000 // ~4000 tokens
	maxGetHotspotsBytes    = 10000 // ~2500 tokens
	maxSummarizePrBytes    = 12000 // ~3000 tokens
)

// testResponseMetrics captures token-related metrics for a tool response (test-local).
type testResponseMetrics struct {
	ToolName        string
	JSONBytes       int
	EstimatedTokens int
}

// measureToolResponse measures the size of a tool response.
func measureToolResponse(toolName string, response interface{}) testResponseMetrics {
	data, _ := json.Marshal(response)
	return testResponseMetrics{
		ToolName:        toolName,
		JSONBytes:       len(data),
		EstimatedTokens: len(data) / 4,
	}
}

// findProjectRoot walks up from current directory to find go.mod
func findProjectRoot() (string, error) {
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
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// newTestMCPServerWithIndex creates an MCP server with the project's SCIP index.
// Returns nil if no index is available (allows tests to skip gracefully).
func newTestMCPServerWithIndex(t testing.TB) *MCPServer {
	t.Helper()

	// Find project root
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil
	}

	// Check for SCIP index
	scipPath := filepath.Join(projectRoot, ".scip", "index.scip")
	if _, err := os.Stat(scipPath); os.IsNotExist(err) {
		return nil
	}

	cfg := &config.Config{
		Version:  5,
		RepoRoot: projectRoot,
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: false,
			},
			Scip: config.ScipConfig{
				Enabled:   true,
				IndexPath: scipPath,
			},
			Lsp: config.LspConfig{
				Enabled: false,
			},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.JSONFormat,
		Output: io.Discard,
	})

	db, err := storage.Open(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	engine, err := query.NewEngine(".", db, logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}

	return NewMCPServer(version.Version, engine, logger)
}

// TestWideResultTokenBudgets validates that wide-result tools stay within token budgets.
// These tests require a SCIP index and are skipped if not available.
func TestWideResultTokenBudgets(t *testing.T) {
	server := newTestMCPServerWithIndex(t)
	if server == nil {
		t.Skip("Skipping: no SCIP index available (run 'ckb index' first)")
	}

	// Test getCallGraph
	t.Run("getCallGraph", func(t *testing.T) {
		// Find a symbol with callers
		resp := sendRequest(t, server, "tools/call", 1, map[string]interface{}{
			"name": "searchSymbols",
			"arguments": map[string]interface{}{
				"query": "Engine",
				"limit": 5,
			},
		})

		if resp.Error != nil {
			t.Skipf("searchSymbols failed: %v", resp.Error)
		}

		// Parse to get a symbol ID
		var result map[string]interface{}
		resultBytes, _ := json.Marshal(resp.Result)
		json.Unmarshal(resultBytes, &result)

		content, ok := result["content"].([]interface{})
		if !ok || len(content) == 0 {
			t.Skip("No symbols found for testing")
		}

		// Get first content item's text and parse it
		firstContent := content[0].(map[string]interface{})
		text := firstContent["text"].(string)
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			t.Skipf("Failed to parse response: %v", err)
		}

		// The envelope has {"schemaVersion": "1.0", "data": {"symbols": [...]}}
		dataField, ok := data["data"].(map[string]interface{})
		if !ok {
			t.Skip("No data field in response")
		}

		symbols, ok := dataField["symbols"].([]interface{})
		if !ok || len(symbols) == 0 {
			t.Skip("No symbols in response")
		}

		firstSymbol := symbols[0].(map[string]interface{})
		symbolID, ok := firstSymbol["stableId"].(string)
		if !ok {
			t.Skip("Symbol has no stableId")
		}

		// Now test getCallGraph with this symbol
		callGraphResp := sendRequest(t, server, "tools/call", 2, map[string]interface{}{
			"name": "getCallGraph",
			"arguments": map[string]interface{}{
				"symbolId":  symbolID,
				"direction": "both",
				"depth":     2,
			},
		})

		if callGraphResp.Error != nil {
			t.Logf("getCallGraph returned error (may be expected): %v", callGraphResp.Error)
			return
		}

		metrics := measureToolResponse("getCallGraph", callGraphResp.Result)
		t.Logf("getCallGraph: %d bytes (~%d tokens)", metrics.JSONBytes, metrics.EstimatedTokens)

		if metrics.JSONBytes > maxCallGraphBytes {
			t.Errorf("getCallGraph exceeds token budget: %d bytes (max %d)",
				metrics.JSONBytes, maxCallGraphBytes)
		}
	})

	// Test getHotspots (doesn't need symbol lookup)
	t.Run("getHotspots", func(t *testing.T) {
		resp := sendRequest(t, server, "tools/call", 3, map[string]interface{}{
			"name": "getHotspots",
			"arguments": map[string]interface{}{
				"limit": 20,
			},
		})

		if resp.Error != nil {
			t.Logf("getHotspots returned error (may be expected): %v", resp.Error)
			return
		}

		metrics := measureToolResponse("getHotspots", resp.Result)
		t.Logf("getHotspots: %d bytes (~%d tokens)", metrics.JSONBytes, metrics.EstimatedTokens)

		if metrics.JSONBytes > maxGetHotspotsBytes {
			t.Errorf("getHotspots exceeds token budget: %d bytes (max %d)",
				metrics.JSONBytes, maxGetHotspotsBytes)
		}
	})

	// Test analyzeImpact
	t.Run("analyzeImpact", func(t *testing.T) {
		// Search for a symbol first
		resp := sendRequest(t, server, "tools/call", 4, map[string]interface{}{
			"name": "searchSymbols",
			"arguments": map[string]interface{}{
				"query": "Search",
				"limit": 1,
			},
		})

		if resp.Error != nil {
			t.Skip("searchSymbols failed")
		}

		var result map[string]interface{}
		resultBytes, _ := json.Marshal(resp.Result)
		json.Unmarshal(resultBytes, &result)

		content, ok := result["content"].([]interface{})
		if !ok || len(content) == 0 {
			t.Skip("No symbols found")
		}

		firstContent := content[0].(map[string]interface{})
		text := firstContent["text"].(string)
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			t.Skip("Failed to parse response")
		}

		// The envelope has {"schemaVersion": "1.0", "data": {"symbols": [...]}}
		dataField, ok := data["data"].(map[string]interface{})
		if !ok {
			t.Skip("No data field in response")
		}

		symbols, ok := dataField["symbols"].([]interface{})
		if !ok || len(symbols) == 0 {
			t.Skip("No symbols in response")
		}

		firstSymbol := symbols[0].(map[string]interface{})
		symbolID, ok := firstSymbol["stableId"].(string)
		if !ok {
			t.Skip("Symbol has no stableId")
		}

		// Test analyzeImpact
		impactResp := sendRequest(t, server, "tools/call", 5, map[string]interface{}{
			"name": "analyzeImpact",
			"arguments": map[string]interface{}{
				"symbolId": symbolID,
				"depth":    2,
			},
		})

		if impactResp.Error != nil {
			t.Logf("analyzeImpact returned error (may be expected): %v", impactResp.Error)
			return
		}

		metrics := measureToolResponse("analyzeImpact", impactResp.Result)
		t.Logf("analyzeImpact: %d bytes (~%d tokens)", metrics.JSONBytes, metrics.EstimatedTokens)

		if metrics.JSONBytes > maxAnalyzeImpactBytes {
			t.Errorf("analyzeImpact exceeds token budget: %d bytes (max %d)",
				metrics.JSONBytes, maxAnalyzeImpactBytes)
		}
	})
}

// TestWideResultMetricsOutput outputs current wide-result metrics for manual review.
// Run with: go test -v -run TestWideResultMetricsOutput ./internal/mcp/...
func TestWideResultMetricsOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping metrics output in short mode")
	}

	server := newTestMCPServerWithIndex(t)
	if server == nil {
		t.Skip("Skipping: no SCIP index available")
	}

	t.Log("")
	t.Log("=== WIDE-RESULT TOKEN METRICS ===")

	// getStatus (baseline - should be small)
	resp := sendRequest(t, server, "tools/call", 1, map[string]interface{}{
		"name":      "getStatus",
		"arguments": map[string]interface{}{},
	})
	if resp.Error == nil {
		metrics := measureToolResponse("getStatus", resp.Result)
		t.Logf("%-20s: %6d bytes, ~%5d tokens", "getStatus", metrics.JSONBytes, metrics.EstimatedTokens)
	}

	// getHotspots
	resp = sendRequest(t, server, "tools/call", 2, map[string]interface{}{
		"name": "getHotspots",
		"arguments": map[string]interface{}{
			"limit": 20,
		},
	})
	if resp.Error == nil {
		metrics := measureToolResponse("getHotspots", resp.Result)
		t.Logf("%-20s: %6d bytes, ~%5d tokens", "getHotspots", metrics.JSONBytes, metrics.EstimatedTokens)
	}

	// searchSymbols
	resp = sendRequest(t, server, "tools/call", 3, map[string]interface{}{
		"name": "searchSymbols",
		"arguments": map[string]interface{}{
			"query": "Engine",
			"limit": 50,
		},
	})
	if resp.Error == nil {
		metrics := measureToolResponse("searchSymbols", resp.Result)
		t.Logf("%-20s: %6d bytes, ~%5d tokens", "searchSymbols", metrics.JSONBytes, metrics.EstimatedTokens)
	}

	// getArchitecture
	resp = sendRequest(t, server, "tools/call", 4, map[string]interface{}{
		"name": "getArchitecture",
		"arguments": map[string]interface{}{
			"depth": 2,
		},
	})
	if resp.Error == nil {
		metrics := measureToolResponse("getArchitecture", resp.Result)
		t.Logf("%-20s: %6d bytes, ~%5d tokens", "getArchitecture", metrics.JSONBytes, metrics.EstimatedTokens)
	}

	t.Log("==================================")
	t.Log("")
}
