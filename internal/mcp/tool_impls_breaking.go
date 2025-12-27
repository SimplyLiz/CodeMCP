package mcp

import (
	"context"

	"ckb/internal/envelope"
	"ckb/internal/query"
)

// v7.6 Breaking Change Detection MCP Tool Implementation

// toolCompareAPI compares API surfaces between two git refs
func (s *MCPServer) toolCompareAPI(params map[string]interface{}) (*envelope.Response, error) {
	opts := query.CompareAPIOptions{
		IgnorePrivate: true,
	}

	// Base ref
	opts.BaseRef = "HEAD~1"
	if v, ok := params["baseRef"].(string); ok && v != "" {
		opts.BaseRef = v
	}

	// Target ref
	opts.TargetRef = "HEAD"
	if v, ok := params["targetRef"].(string); ok && v != "" {
		opts.TargetRef = v
	}

	// Scope
	if scopeRaw, ok := params["scope"].([]interface{}); ok {
		for _, s := range scopeRaw {
			if str, ok := s.(string); ok {
				opts.Scope = append(opts.Scope, str)
			}
		}
	}

	// Include minor changes
	if v, ok := params["includeMinor"].(bool); ok {
		opts.IncludeMinor = v
	}

	// Execute analysis
	ctx := context.Background()
	result, err := s.engine().CompareAPI(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build response
	return NewToolResponse().
		Data(result).
		Build(), nil
}
