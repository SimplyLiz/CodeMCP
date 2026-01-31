package mcp

import (
	"context"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// v8.1 Suggested Refactorings MCP Tool Implementation

// toolSuggestRefactorings detects refactoring opportunities in the codebase.
func (s *MCPServer) toolSuggestRefactorings(params map[string]interface{}) (*envelope.Response, error) {
	opts := query.SuggestRefactoringsOptions{
		Limit: 50,
	}

	if v, ok := params["scope"].(string); ok && v != "" {
		opts.Scope = v
	}

	if v, ok := params["minSeverity"].(string); ok && v != "" {
		switch v {
		case "critical", "high", "medium", "low":
			opts.MinSeverity = v
		}
	}

	if typesRaw, ok := params["types"].([]interface{}); ok {
		for _, t := range typesRaw {
			if str, ok := t.(string); ok {
				opts.Types = append(opts.Types, str)
			}
		}
	}

	if v, ok := params["limit"].(float64); ok {
		opts.Limit = int(v)
	}

	engine, err := s.GetEngine()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	result, err := engine.SuggestRefactorings(ctx, opts)
	if err != nil {
		return nil, err
	}

	resp := NewToolResponse().Data(result)
	for _, dw := range engine.GetDegradationWarnings() {
		resp.Warning(dw.Message)
	}
	return resp.Build(), nil
}
