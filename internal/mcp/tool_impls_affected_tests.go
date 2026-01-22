package mcp

import (
	"context"

	"ckb/internal/envelope"
	"ckb/internal/query"
)

// v7.6 Affected Tests MCP Tool Implementation

// toolGetAffectedTests finds tests affected by current code changes.
func (s *MCPServer) toolGetAffectedTests(params map[string]interface{}) (*envelope.Response, error) {
	opts := query.GetAffectedTestsOptions{}

	// Staged
	if v, ok := params["staged"].(bool); ok {
		opts.Staged = v
	}

	// Base branch
	opts.BaseBranch = "HEAD"
	if v, ok := params["baseBranch"].(string); ok && v != "" {
		opts.BaseBranch = v
	}

	// Depth
	opts.TransitiveDepth = 1
	if v, ok := params["depth"].(float64); ok {
		opts.TransitiveDepth = int(v)
	}

	// Use coverage
	if v, ok := params["useCoverage"].(bool); ok {
		opts.UseCoverage = v
	}

	// Execute analysis
	ctx := context.Background()
	result, err := s.engine().GetAffectedTests(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build response
	return NewToolResponse().
		Data(result).
		Build(), nil
}
