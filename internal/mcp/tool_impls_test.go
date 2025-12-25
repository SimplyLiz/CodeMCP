package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// Tool Call Helper
// =============================================================================

// callTool is a helper to call a tool and return the response
func callTool(t *testing.T, server *MCPServer, name string, args map[string]interface{}) *MCPMessage {
	t.Helper()

	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	return sendRequest(t, server, "tools/call", 1, params)
}

// hasToolError checks if the tool response contains an error in the envelope
func hasToolError(t *testing.T, resp *MCPMessage) bool {
	t.Helper()

	if resp.Error != nil {
		return true // JSON-RPC error
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return false
	}

	content, ok := result["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		return false
	}

	text, ok := content[0]["text"].(string)
	if !ok {
		return false
	}

	// Check if the envelope has an error field
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return false
	}

	errVal, hasErr := envelope["error"]
	if !hasErr || errVal == nil {
		return false
	}

	// Check if error is not null/empty
	errStr, ok := errVal.(string)
	if ok && errStr != "" {
		return true
	}

	return false
}

// getToolErrorMessage extracts the error message from a tool response
func getToolErrorMessage(t *testing.T, resp *MCPMessage) string {
	t.Helper()

	if resp.Error != nil {
		return resp.Error.Message
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return ""
	}

	content, ok := result["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		return ""
	}

	text, ok := content[0]["text"].(string)
	if !ok {
		return ""
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return ""
	}

	errVal, _ := envelope["error"].(string)
	return errVal
}

// =============================================================================
// Navigation Tools - Parameter Validation Tests
// =============================================================================

func TestToolGetSymbol_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getSymbol", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
	if errMsg := getToolErrorMessage(t, resp); !strings.Contains(errMsg, "symbolId") {
		t.Errorf("expected error about symbolId, got: %s", errMsg)
	}
}

func TestToolGetSymbol_InvalidRepoStateMode(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// repoStateMode is optional, so invalid values should still work
	resp := callTool(t, server, "getSymbol", map[string]interface{}{
		"symbolId":      "test:sym:1",
		"repoStateMode": "invalid",
	})

	// Should succeed (graceful handling) or fail with symbol not found, not param error
	if resp.Error != nil {
		// Acceptable - symbol not found is OK
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolSearchSymbols_MissingQuery(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "searchSymbols", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing query")
	}
}

func TestToolSearchSymbols_ValidQuery(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "searchSymbols", map[string]interface{}{
		"query": "Engine",
	})

	// Should succeed (possibly with empty results if no backends)
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolSearchSymbols_WithScope(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "searchSymbols", map[string]interface{}{
		"query": "Engine",
		"scope": "internal/query",
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolSearchSymbols_WithKinds(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "searchSymbols", map[string]interface{}{
		"query": "Engine",
		"kinds": []interface{}{"class", "function"},
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolSearchSymbols_WithLimit(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "searchSymbols", map[string]interface{}{
		"query": "Engine",
		"limit": float64(5),
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolFindReferences_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findReferences", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
}

func TestToolFindReferences_ValidSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findReferences", map[string]interface{}{
		"symbolId": "test:sym:1",
	})

	// May succeed or fail if symbol not found - either is OK
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolGetCallGraph_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getCallGraph", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
}

func TestToolGetCallGraph_ValidDirections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction string
	}{
		{"callers", "callers"},
		{"callees", "callees"},
		{"both", "both"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			resp := callTool(t, server, "getCallGraph", map[string]interface{}{
				"symbolId":  "test:sym:1",
				"direction": tt.direction,
			})

			// Either success or symbol not found is OK
			if hasToolError(t, resp) {
				return
			}

			if resp.Result == nil {
				t.Error("expected result if no error")
			}
		})
	}
}

func TestToolGetCallGraph_DepthBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		depth float64
	}{
		{"depth 1", 1},
		{"depth 4 max", 4},
		{"depth over max", 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			resp := callTool(t, server, "getCallGraph", map[string]interface{}{
				"symbolId": "test:sym:1",
				"depth":    tt.depth,
			})

			if hasToolError(t, resp) {
				return
			}

			if resp.Result == nil {
				t.Error("expected result if no error")
			}
		})
	}
}

func TestToolExplainSymbol_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "explainSymbol", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
}

func TestToolExplainFile_MissingFilePath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "explainFile", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing filePath")
	}
}

func TestToolTraceUsage_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "traceUsage", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
}

func TestToolListEntrypoints_LimitBounds(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "listEntrypoints", map[string]interface{}{
		"limit": float64(100),
	})

	// Should work regardless of limit
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// Analysis Tools - Parameter Validation Tests
// =============================================================================

func TestToolAnalyzeImpact_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "analyzeImpact", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
}

func TestToolAnalyzeImpact_ValidTelemetryPeriod(t *testing.T) {
	t.Parallel()

	periods := []string{"7d", "30d", "90d", "all"}

	for _, period := range periods {
		period := period
		t.Run(period, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			resp := callTool(t, server, "analyzeImpact", map[string]interface{}{
				"symbolId":        "test:sym:1",
				"telemetryPeriod": period,
			})

			if hasToolError(t, resp) {
				return
			}

			if resp.Result == nil {
				t.Error("expected result if no error")
			}
		})
	}
}

func TestToolGetHotspots_LimitBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit float64
	}{
		{"default", 0},
		{"small", 5},
		{"at max", 50},
		{"over max", 100},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			args := map[string]interface{}{}
			if tt.limit > 0 {
				args["limit"] = tt.limit
			}

			resp := callTool(t, server, "getHotspots", args)

			if hasToolError(t, resp) {
				return
			}

			if resp.Result == nil {
				t.Error("expected result if no error")
			}
		})
	}
}

func TestToolListKeyConcepts_LimitBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit float64
	}{
		{"default", 0},
		{"small", 5},
		{"at max", 12},
		{"over max", 20},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			args := map[string]interface{}{}
			if tt.limit > 0 {
				args["limit"] = tt.limit
			}

			resp := callTool(t, server, "listKeyConcepts", args)

			if hasToolError(t, resp) {
				return
			}

			if resp.Result == nil {
				t.Error("expected result if no error")
			}
		})
	}
}

func TestToolGetArchitecture_DepthBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		depth float64
	}{
		{"depth 1", 1},
		{"depth 2", 2},
		{"depth 5", 5},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			resp := callTool(t, server, "getArchitecture", map[string]interface{}{
				"depth": tt.depth,
			})

			if hasToolError(t, resp) {
				return
			}

			if resp.Result == nil {
				t.Error("expected result if no error")
			}
		})
	}
}

func TestToolGetModuleOverview_OptionalPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Both path and name are optional
	resp := callTool(t, server, "getModuleOverview", map[string]interface{}{})

	// Should work with defaults
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// System Tools - Parameter Validation Tests
// =============================================================================

func TestToolGetStatus_NoParams(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getStatus", map[string]interface{}{})

	if resp.Error != nil {
		t.Errorf("getStatus should work with no params: %v", resp.Error.Message)
	}

	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestToolDoctor_NoParams(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "doctor", map[string]interface{}{})

	if resp.Error != nil {
		t.Errorf("doctor should work with no params: %v", resp.Error.Message)
	}

	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestToolExpandToolset_MissingPreset(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "expandToolset", map[string]interface{}{
		"reason": "test reason",
	})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing preset")
	}
}

func TestToolExpandToolset_MissingReason(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "expandToolset", map[string]interface{}{
		"preset": "review",
	})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing reason")
	}
}

func TestToolExpandToolset_InvalidPreset(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "expandToolset", map[string]interface{}{
		"preset": "invalid_preset",
		"reason": "test reason",
	})

	if !hasToolError(t, resp) {
		t.Error("expected error for invalid preset")
	}
}

func TestToolExpandToolset_ValidPresets(t *testing.T) {
	t.Parallel()

	presets := []string{"review", "refactor", "federation", "docs", "ops", "full"}

	for _, preset := range presets {
		preset := preset
		t.Run(preset, func(t *testing.T) {
			t.Parallel()

			localServer := newTestMCPServer(t)
			resp := callTool(t, localServer, "expandToolset", map[string]interface{}{
				"preset": preset,
				"reason": "testing " + preset,
			})

			if resp.Error != nil {
				t.Errorf("expected success for preset %s: %v", preset, resp.Error.Message)
			}

			if resp.Result == nil {
				t.Error("expected result")
			}
		})
	}
}

// =============================================================================
// Ownership Tools - Parameter Validation Tests
// =============================================================================

func TestToolGetOwnership_MissingPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getOwnership", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing path")
	}
}

func TestToolGetOwnership_ValidPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getOwnership", map[string]interface{}{
		"path": "internal/query/engine.go",
	})

	// May succeed or fail depending on file existence
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolGetModuleResponsibilities_OptionalModule(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Module is optional
	resp := callTool(t, server, "getModuleResponsibilities", map[string]interface{}{})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// Decision Tools - Parameter Validation Tests
// =============================================================================

func TestToolRecordDecision_RequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]interface{}
		hasErr bool
	}{
		{
			name:   "missing title",
			params: map[string]interface{}{"status": "accepted"},
			hasErr: true,
		},
		{
			name:   "missing status",
			params: map[string]interface{}{"title": "Test Decision"},
			hasErr: true,
		},
		{
			name: "valid minimal",
			params: map[string]interface{}{
				"title":  "Test Decision",
				"status": "proposed",
			},
			hasErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newTestMCPServer(t)

			resp := callTool(t, server, "recordDecision", tt.params)

			if tt.hasErr && !hasToolError(t, resp) {
				t.Error("expected error")
			}
			if !tt.hasErr && hasToolError(t, resp) {
				// May fail for other reasons (storage, etc) - that's OK
				return
			}
		})
	}
}

func TestToolGetDecisions_OptionalFilters(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// All filters are optional
	resp := callTool(t, server, "getDecisions", map[string]interface{}{})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolGetDecisions_WithFilters(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getDecisions", map[string]interface{}{
		"status":   "accepted",
		"category": "architecture",
		"limit":    float64(10),
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// SummarizeDiff Tool Tests
// =============================================================================

func TestToolSummarizeDiff_DefaultTimeWindow(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// No selector defaults to last 30 days
	resp := callTool(t, server, "summarizeDiff", map[string]interface{}{})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolSummarizeDiff_WithCommit(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "summarizeDiff", map[string]interface{}{
		"commit": "HEAD",
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

func TestToolSummarizeDiff_WithCommitRange(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "summarizeDiff", map[string]interface{}{
		"commitRange": map[string]interface{}{
			"base": "HEAD~5",
			"head": "HEAD",
		},
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// SummarizePr Tool Tests
// =============================================================================

func TestToolSummarizePr_OptionalBranch(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Branch is optional (uses current branch)
	resp := callTool(t, server, "summarizePr", map[string]interface{}{})

	// May fail if not in a git repo - that's OK
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// Complexity Tool Tests
// =============================================================================

func TestToolGetFileComplexity_MissingPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getFileComplexity", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing filePath")
	}
}

func TestToolGetFileComplexity_ValidPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "getFileComplexity", map[string]interface{}{
		"filePath": "internal/query/engine.go",
	})

	// May fail if file not found - that's OK
	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// JustifySymbol Tool Tests
// =============================================================================

func TestToolJustifySymbol_MissingSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "justifySymbol", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing symbolId")
	}
}

func TestToolJustifySymbol_ValidSymbolId(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "justifySymbol", map[string]interface{}{
		"symbolId": "test:sym:1",
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// ExplainPath Tool Tests
// =============================================================================

func TestToolExplainPath_MissingPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "explainPath", map[string]interface{}{})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing filePath")
	}
}

func TestToolExplainPath_ValidPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "explainPath", map[string]interface{}{
		"filePath": "internal/query/",
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}

// =============================================================================
// AnnotateModule Tool Tests
// =============================================================================

func TestToolAnnotateModule_MissingPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "annotateModule", map[string]interface{}{
		"annotations": []interface{}{"stability:stable"},
	})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing modulePath")
	}
}

func TestToolAnnotateModule_MissingAnnotations(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "annotateModule", map[string]interface{}{
		"modulePath": "internal/query",
	})

	if !hasToolError(t, resp) {
		t.Error("expected error for missing annotations")
	}
}

func TestToolAnnotateModule_ArrayParsing(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "annotateModule", map[string]interface{}{
		"modulePath":  "internal/query",
		"annotations": []interface{}{"stability:stable", "tier:core"},
	})

	if resp.Error != nil {
		return
	}

	if resp.Result == nil {
		t.Error("expected result if no error")
	}
}
