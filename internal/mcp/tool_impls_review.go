package mcp

import (
	"context"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/errors"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// toolReviewPR runs a comprehensive PR review with quality gates.
func (s *MCPServer) toolReviewPR(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse baseBranch
	baseBranch := "main"
	if v, ok := params["baseBranch"].(string); ok && v != "" {
		baseBranch = v
	}

	// Parse headBranch
	headBranch := ""
	if v, ok := params["headBranch"].(string); ok {
		headBranch = v
	}

	// Parse checks filter
	var checks []string
	if v, ok := params["checks"].([]interface{}); ok {
		for _, c := range v {
			if cs, ok := c.(string); ok {
				checks = append(checks, cs)
			}
		}
	}

	// Parse failOnLevel
	failOnLevel := ""
	if v, ok := params["failOnLevel"].(string); ok {
		failOnLevel = v
	}

	// Parse critical paths
	var criticalPaths []string
	if v, ok := params["criticalPaths"].([]interface{}); ok {
		for _, p := range v {
			if ps, ok := p.(string); ok {
				criticalPaths = append(criticalPaths, ps)
			}
		}
	}

	policy := query.DefaultReviewPolicy()
	if failOnLevel != "" {
		policy.FailOnLevel = failOnLevel
	}
	if len(criticalPaths) > 0 {
		policy.CriticalPaths = criticalPaths
	}

	s.logger.Debug("Executing reviewPR",
		"baseBranch", baseBranch,
		"headBranch", headBranch,
		"checks", checks,
	)

	result, err := s.engine().ReviewPR(ctx, query.ReviewPROptions{
		BaseBranch: baseBranch,
		HeadBranch: headBranch,
		Policy:     policy,
		Checks:     checks,
	})
	if err != nil {
		return nil, errors.NewOperationError("review PR", err)
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}
