package mcp

import (
	"context"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/errors"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// v8.2 Test Gap Analysis tool implementation

// toolAnalyzeTestGaps identifies functions that lack test coverage.
func (s *MCPServer) toolAnalyzeTestGaps(params map[string]interface{}) (*envelope.Response, error) {
	target, ok := params["target"].(string)
	if !ok || target == "" {
		return nil, errors.NewInvalidParameterError("target", "required")
	}

	minLines := 3
	if v, ok := params["minLines"].(float64); ok {
		minLines = int(v)
	}

	limit := 50
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	engine, err := s.GetEngine()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	result, err := engine.AnalyzeTestGaps(ctx, query.AnalyzeTestGapsOptions{
		Target:   target,
		MinLines: minLines,
		Limit:    limit,
	})
	if err != nil {
		return nil, errors.NewOperationError("analyze test gaps", err)
	}

	resp := NewToolResponse().Data(result)
	for _, dw := range engine.GetDegradationWarnings() {
		resp.Warning(dw.Message)
	}
	return resp.Build(), nil
}
