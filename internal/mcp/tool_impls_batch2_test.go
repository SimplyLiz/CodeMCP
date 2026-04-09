package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// FindCycles Tool Tests
// =============================================================================

func TestToolFindCycles_DefaultParams(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findCycles", map[string]interface{}{})

	// Should succeed with defaults (granularity=directory, maxCycles=20)
	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestToolFindCycles_GranularityOptions(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	granularities := []string{"module", "directory", "file"}
	for _, g := range granularities {
		t.Run(g, func(t *testing.T) {
			resp := callTool(t, server, "findCycles", map[string]interface{}{
				"granularity": g,
			})

			if resp.Error != nil {
				t.Errorf("unexpected error for granularity=%s: %v", g, resp.Error)
			}
		})
	}
}

func TestToolFindCycles_InvalidGranularity(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Invalid granularity should fall back to default "directory"
	resp := callTool(t, server, "findCycles", map[string]interface{}{
		"granularity": "invalid",
	})

	// Should still succeed — invalid values are ignored, default used
	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolFindCycles_WithTargetPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findCycles", map[string]interface{}{
		"targetPath": "internal/query",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolFindCycles_WithMaxCycles(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findCycles", map[string]interface{}{
		"maxCycles": float64(5),
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolFindCycles_AllParams(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findCycles", map[string]interface{}{
		"granularity": "file",
		"targetPath":  "internal/mcp",
		"maxCycles":   float64(10),
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestToolFindCycles_ResponseEnvelope(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "findCycles", map[string]interface{}{
		"granularity": "directory",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Verify the response has a valid envelope structure
	env := extractToolEnvelope(t, resp)
	if env == nil {
		t.Fatal("expected non-nil envelope in response")
	}

	// Envelope must have schemaVersion
	if _, ok := env["schemaVersion"]; !ok {
		t.Error("expected schemaVersion in envelope")
	}

	// Data may be null in test env (no git repo), but envelope error should explain why
	if env["data"] == nil {
		errStr, _ := env["error"].(string)
		if errStr == "" {
			t.Error("expected either data or error in envelope")
		}
	} else {
		data, ok := env["data"].(map[string]interface{})
		if !ok {
			t.Fatal("expected data to be a map")
		}
		if _, ok := data["granularity"]; !ok {
			t.Error("expected granularity field in response data")
		}
	}
}

// =============================================================================
// PrepareChange with Move Tests
// =============================================================================

func TestToolPrepareChange_MoveType(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "prepareChange", map[string]interface{}{
		"target":     "internal/mcp/handler.go",
		"changeType": "move",
		"targetPath": "pkg/handler.go",
	})

	// Should succeed — may have limited data without SCIP
	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolPrepareChange_MoveWithoutTargetPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Move without targetPath — should still succeed (targetPath is optional at MCP level)
	resp := callTool(t, server, "prepareChange", map[string]interface{}{
		"target":     "internal/mcp/handler.go",
		"changeType": "move",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolPrepareChange_MoveInChangeTypeOptions(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Verify "move" is accepted alongside existing change types
	changeTypes := []string{"modify", "rename", "delete", "extract", "move"}
	for _, ct := range changeTypes {
		t.Run(ct, func(t *testing.T) {
			args := map[string]interface{}{
				"target":     "internal/mcp/handler.go",
				"changeType": ct,
			}
			if ct == "move" {
				args["targetPath"] = "pkg/handler.go"
			}

			resp := callTool(t, server, "prepareChange", args)
			if resp.Error != nil {
				t.Errorf("unexpected error for changeType=%s: %v", ct, resp.Error)
			}
		})
	}
}

func TestToolPrepareChange_TargetPathIgnoredForNonMove(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// targetPath should be silently ignored for non-move change types
	resp := callTool(t, server, "prepareChange", map[string]interface{}{
		"target":     "internal/mcp/handler.go",
		"changeType": "modify",
		"targetPath": "pkg/handler.go",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

// =============================================================================
// PlanRefactor with Move Tests
// =============================================================================

func TestToolPlanRefactor_MoveType(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "planRefactor", map[string]interface{}{
		"target":     "internal/mcp/handler.go",
		"changeType": "move",
		"targetPath": "pkg/handler.go",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolPlanRefactor_MoveWithoutTargetPath(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "planRefactor", map[string]interface{}{
		"target":     "internal/mcp/handler.go",
		"changeType": "move",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

// =============================================================================
// SuggestRefactorings Tool Tests
// =============================================================================

func TestToolSuggestRefactorings_DefaultParams(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{})

	// Should succeed with defaults (limit=50, no scope filter)
	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestToolSuggestRefactorings_WithScope(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{
		"scope": "internal/query",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolSuggestRefactorings_MinSeverityOptions(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	severities := []string{"low", "medium", "high", "critical"}
	for _, sev := range severities {
		t.Run(sev, func(t *testing.T) {
			resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{
				"minSeverity": sev,
			})

			if resp.Error != nil {
				t.Errorf("unexpected error for minSeverity=%s: %v", sev, resp.Error)
			}
		})
	}
}

func TestToolSuggestRefactorings_InvalidMinSeverity(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Invalid severity should be ignored (default to no filter)
	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{
		"minSeverity": "invalid",
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolSuggestRefactorings_WithTypes(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{
		"types": []interface{}{"extract_function", "simplify_function"},
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolSuggestRefactorings_WithLimit(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{
		"limit": float64(10),
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

func TestToolSuggestRefactorings_AllParams(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{
		"scope":       "internal/mcp",
		"minSeverity": "medium",
		"types":       []interface{}{"extract_function", "reduce_coupling"},
		"limit":       float64(25),
	})

	if resp.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("expected result")
	}
}

func TestToolSuggestRefactorings_ResponseEnvelope(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	resp := callTool(t, server, "suggestRefactorings", map[string]interface{}{})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	env := extractToolEnvelope(t, resp)
	if env == nil {
		t.Fatal("expected non-nil envelope in response")
	}

	if _, ok := env["schemaVersion"]; !ok {
		t.Error("expected schemaVersion in envelope")
	}

	if env["data"] != nil {
		data, ok := env["data"].(map[string]interface{})
		if !ok {
			t.Fatal("expected data to be a map")
		}
		if _, ok := data["suggestions"]; !ok {
			t.Error("expected suggestions field in response data")
		}
		if _, ok := data["summary"]; !ok {
			t.Error("expected summary field in response data")
		}
	}
}

// =============================================================================
// Tool Registration Tests
// =============================================================================

func TestToolFindCycles_Registered(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Expand refactor preset to get findCycles
	callTool(t, server, "expandToolset", map[string]interface{}{
		"preset": "refactor",
		"reason": "testing tool registration",
	})

	// Use GetFilteredTools directly to avoid pagination truncation — the refactor
	// preset may exceed DefaultPageSize so tools/list only returns the first page.
	toolsList := server.GetFilteredTools()

	found := false
	for _, tool := range toolsList {
		if tool.Name == "findCycles" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("findCycles not found in tools list after expanding refactor preset (total tools: %d)", len(toolsList))
	}
}

func TestToolSuggestRefactorings_Registered(t *testing.T) {
	t.Parallel()
	server := newTestMCPServer(t)

	// Expand refactor preset to get suggestRefactorings
	callTool(t, server, "expandToolset", map[string]interface{}{
		"preset": "refactor",
		"reason": "testing tool registration",
	})

	// Use GetFilteredTools directly to avoid pagination truncation — the refactor
	// preset may exceed DefaultPageSize so tools/list only returns the first page.
	toolsList := server.GetFilteredTools()

	found := false
	for _, tool := range toolsList {
		if tool.Name == "suggestRefactorings" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("suggestRefactorings not found in tools list after expanding refactor preset (total tools: %d)", len(toolsList))
	}
}

// =============================================================================
// Helpers
// =============================================================================

// extractToolEnvelope parses the response and returns the full envelope map.
func extractToolEnvelope(t *testing.T, resp *MCPMessage) map[string]interface{} {
	t.Helper()

	if resp.Result == nil {
		return nil
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil
	}

	content, ok := result["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		return nil
	}

	text, ok := content[0]["text"].(string)
	if !ok {
		return nil
	}

	var env map[string]interface{}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		return nil
	}

	return env
}

// Ensure imports are used
var _ = strings.Contains
