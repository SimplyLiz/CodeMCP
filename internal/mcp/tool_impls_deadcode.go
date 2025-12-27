package mcp

import (
	"context"

	"ckb/internal/envelope"
	"ckb/internal/query"
)

// v7.6 Static Dead Code Detection tool implementation

// toolFindDeadCode finds dead code using static analysis of the SCIP index.
func (s *MCPServer) toolFindDeadCode(params map[string]interface{}) (*envelope.Response, error) {
	// Parse parameters
	opts := query.FindDeadCodeOptions{
		IncludeExported: true,
	}

	// Scope
	if scopeRaw, ok := params["scope"].([]interface{}); ok {
		for _, s := range scopeRaw {
			if str, ok := s.(string); ok {
				opts.Scope = append(opts.Scope, str)
			}
		}
	}

	// Include unexported
	if v, ok := params["includeUnexported"].(bool); ok {
		opts.IncludeUnexported = v
	}

	// Min confidence
	opts.MinConfidence = 0.7
	if v, ok := params["minConfidence"].(float64); ok {
		opts.MinConfidence = v
	}

	// Exclude patterns
	if patternsRaw, ok := params["excludePatterns"].([]interface{}); ok {
		for _, p := range patternsRaw {
			if str, ok := p.(string); ok {
				opts.ExcludePatterns = append(opts.ExcludePatterns, str)
			}
		}
	}

	// Include test-only (default is to exclude them)
	opts.ExcludeTestOnly = true
	if v, ok := params["includeTestOnly"].(bool); ok {
		opts.ExcludeTestOnly = !v
	}

	// Limit
	opts.Limit = 100
	if v, ok := params["limit"].(float64); ok {
		opts.Limit = int(v)
	}

	// Execute analysis
	ctx := context.Background()
	result, err := s.engine().FindDeadCode(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build response
	return NewToolResponse().
		Data(result).
		Build(), nil
}
