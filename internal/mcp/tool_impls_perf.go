package mcp

import (
	"context"

	"github.com/SimplyLiz/CodeMCP/internal/errors"
	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/perf"
)

// toolScanPerformance detects hidden coupling: file pairs that co-change
// frequently in git but have no static import edge between them.
func (s *MCPServer) toolScanPerformance(params map[string]interface{}) (*envelope.Response, error) {
	minCorrelation := 0.3
	if v, ok := params["minCorrelation"].(float64); ok {
		minCorrelation = v
	}

	minCoChanges := 3
	if v, ok := params["minCoChanges"].(float64); ok {
		minCoChanges = int(v)
	}

	windowDays := 365
	if v, ok := params["windowDays"].(float64); ok {
		windowDays = int(v)
	}

	limit := 50
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	var scope []string
	if v, ok := params["scope"].([]interface{}); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				scope = append(scope, s)
			}
		}
	}

	repoRoot := s.engine().GetRepoRoot()
	analyzer := perf.NewAnalyzer(repoRoot, s.logger)

	ctx := context.Background()
	result, err := analyzer.Scan(ctx, perf.ScanOptions{
		RepoRoot:       repoRoot,
		Scope:          scope,
		MinCorrelation: minCorrelation,
		MinCoChanges:   minCoChanges,
		WindowDays:     windowDays,
		Limit:          limit,
	})
	if err != nil {
		return nil, errors.NewOperationError("scan performance", err)
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}
