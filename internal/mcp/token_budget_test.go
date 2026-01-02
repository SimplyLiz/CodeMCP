package mcp

import (
	"encoding/json"
	"testing"

	"ckb/internal/logging"
)

// Token budget thresholds for CI regression detection.
// These values are baselines - if a change causes them to be exceeded,
// the test fails and forces review of the token impact.
const (
	// tools/list budgets (bytes)
	// v8.0: Increased budgets for compound tools (explore, understand, prepareChange, batchGet, batchSearch)
	maxCorePresetBytes   = 60000  // ~15k tokens - v8.0: core now includes 5 compound tools
	maxReviewPresetBytes = 80000  // ~20k tokens - review adds a few tools
	maxFullPresetBytes   = 270000 // ~67k tokens - all 86 tools (v8.0: 81 + 5 compound)

	// Per-tool schema budget (bytes) - catches bloated schemas
	maxToolSchemaBytes = 6000 // ~1500 tokens per tool
)

// TestToolsListTokenBudget validates that preset token usage stays within budget.
// This test fails CI if someone accidentally bloats tool schemas or adds too many tools.
func TestToolsListTokenBudget(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	server := NewMCPServer("test", nil, logger)

	tests := []struct {
		preset   string
		maxBytes int
		minTools int // Ensure we don't accidentally drop tools
		maxTools int
	}{
		{PresetCore, maxCorePresetBytes, 17, 21},     // v8.0: 19 tools (14 + 5 compound)
		{PresetReview, maxReviewPresetBytes, 22, 27}, // v8.0: 24 tools (19 + 5 review-specific)
		{PresetFull, maxFullPresetBytes, 80, 90},     // v8.0: 86 tools (81 + 5 compound)
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			if err := server.SetPreset(tt.preset); err != nil {
				t.Fatalf("SetPreset(%s) failed: %v", tt.preset, err)
			}

			tools := server.GetFilteredTools()
			data, err := json.Marshal(map[string]interface{}{"tools": tools})
			if err != nil {
				t.Fatalf("Failed to marshal tools: %v", err)
			}

			// Check byte budget
			if len(data) > tt.maxBytes {
				t.Errorf("preset %s exceeds token budget: %d bytes (max %d, ~%d tokens over)",
					tt.preset, len(data), tt.maxBytes, (len(data)-tt.maxBytes)/4)
			}

			// Check tool count bounds (functionality preservation)
			if len(tools) < tt.minTools {
				t.Errorf("preset %s has too few tools: %d (min %d) - possible regression",
					tt.preset, len(tools), tt.minTools)
			}
			if len(tools) > tt.maxTools {
				t.Errorf("preset %s has too many tools: %d (max %d) - review if intentional",
					tt.preset, len(tools), tt.maxTools)
			}
		})
	}
}

// TestToolSchemaSize validates individual tool schemas don't bloat.
// Catches cases where a single tool's schema grows excessively.
func TestToolSchemaSize(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	server := NewMCPServer("test", nil, logger)

	tools := server.GetToolDefinitions()

	var violations []string
	for _, tool := range tools {
		data, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("Failed to marshal tool %s: %v", tool.Name, err)
		}

		if len(data) > maxToolSchemaBytes {
			violations = append(violations, tool.Name)
			t.Errorf("tool %s schema too large: %d bytes (max %d, ~%d tokens)",
				tool.Name, len(data), maxToolSchemaBytes, len(data)/4)
		}
	}

	if len(violations) > 0 {
		t.Logf("Tools exceeding schema budget: %v", violations)
	}
}

// TestPresetToolCoverage validates that core tools are included in all presets.
// Ensures expanding from core to other presets doesn't lose essential tools.
func TestPresetToolCoverage(t *testing.T) {
	coreTools := GetPresetTools(PresetCore)
	coreSet := make(map[string]bool)
	for _, name := range coreTools {
		coreSet[name] = true
	}

	// All non-core presets should include core tools
	presets := []string{PresetReview, PresetRefactor, PresetFederation, PresetDocs, PresetOps}

	for _, preset := range presets {
		t.Run(preset, func(t *testing.T) {
			tools := GetPresetTools(preset)
			toolSet := make(map[string]bool)
			for _, name := range tools {
				toolSet[name] = true
			}

			for coreTool := range coreSet {
				if !toolSet[coreTool] {
					t.Errorf("preset %s missing core tool: %s", preset, coreTool)
				}
			}
		})
	}
}

// TestTokenMetrics outputs current token metrics for manual review.
// Run with: go test -v -run TestTokenMetrics ./internal/mcp/...
func TestTokenMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping metrics output in short mode")
	}

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	server := NewMCPServer("test", nil, logger)

	t.Log("")
	t.Log("=== TOKEN BUDGET METRICS ===")

	for _, preset := range ValidPresets() {
		if err := server.SetPreset(preset); err != nil {
			continue
		}

		tools := server.GetFilteredTools()
		data, _ := json.Marshal(map[string]interface{}{"tools": tools})

		t.Logf("%-12s: %3d tools, %6d bytes, ~%5d tokens",
			preset, len(tools), len(data), len(data)/4)
	}

	t.Log("============================")
	t.Log("")
}
