package mcp

import (
	"context"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// v8.1 Cycle Detection MCP Tool Implementation

// toolFindCycles detects circular dependencies in the dependency graph.
func (s *MCPServer) toolFindCycles(params map[string]interface{}) (*envelope.Response, error) {
	opts := query.FindCyclesOptions{
		Granularity: "directory",
		MaxCycles:   20,
	}

	if v, ok := params["granularity"].(string); ok && v != "" {
		switch v {
		case "module", "directory", "file":
			opts.Granularity = v
		}
	}

	if v, ok := params["targetPath"].(string); ok && v != "" {
		opts.TargetPath = v
	}

	if v, ok := params["maxCycles"].(float64); ok {
		opts.MaxCycles = int(v)
	}

	engine, err := s.GetEngine()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	result, err := engine.FindCycles(ctx, opts)
	if err != nil {
		return nil, err
	}

	resp := NewToolResponse().Data(result)
	for _, dw := range engine.GetDegradationWarnings() {
		resp.Warning(dw.Message)
	}
	return resp.Build(), nil
}
