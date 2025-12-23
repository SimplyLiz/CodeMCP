package mcp

import (
	"context"
	"fmt"

	"ckb/internal/audit"
	"ckb/internal/coupling"
	"ckb/internal/explain"
	"ckb/internal/export"
)

// v6.5 Developer Intelligence tool implementations

// toolExplainOrigin explains why code exists with full context
func (s *MCPServer) toolExplainOrigin(params map[string]interface{}) (interface{}, error) {
	symbol, ok := params["symbol"].(string)
	if !ok || symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	includeUsage := true
	if v, ok := params["includeUsage"].(bool); ok {
		includeUsage = v
	}

	includeCoChange := true
	if v, ok := params["includeCoChange"].(bool); ok {
		includeCoChange = v
	}

	historyLimit := 10
	if v, ok := params["historyLimit"].(float64); ok {
		historyLimit = int(v)
	}

	repoRoot := s.engine().GetRepoRoot()
	explainer := explain.NewExplainer(repoRoot, s.logger)

	ctx := context.Background()
	result, err := explainer.Explain(ctx, explain.ExplainOptions{
		Symbol:          symbol,
		IncludeUsage:    includeUsage,
		IncludeCoChange: includeCoChange,
		HistoryLimit:    historyLimit,
		RepoRoot:        repoRoot,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to explain symbol: %w", err)
	}

	return result, nil
}

// toolAnalyzeCoupling finds files/symbols that historically change together
func (s *MCPServer) toolAnalyzeCoupling(params map[string]interface{}) (interface{}, error) {
	target, ok := params["target"].(string)
	if !ok || target == "" {
		return nil, fmt.Errorf("target is required")
	}

	minCorrelation := 0.3
	if v, ok := params["minCorrelation"].(float64); ok {
		minCorrelation = v
	}

	windowDays := 365
	if v, ok := params["windowDays"].(float64); ok {
		windowDays = int(v)
	}

	limit := 20
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	repoRoot := s.engine().GetRepoRoot()
	analyzer := coupling.NewAnalyzer(repoRoot, s.logger)

	ctx := context.Background()
	result, err := analyzer.Analyze(ctx, coupling.AnalyzeOptions{
		Target:         target,
		MinCorrelation: minCorrelation,
		WindowDays:     windowDays,
		Limit:          limit,
		RepoRoot:       repoRoot,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to analyze coupling: %w", err)
	}

	return result, nil
}

// toolExportForLLM exports codebase structure in LLM-friendly format
func (s *MCPServer) toolExportForLLM(params map[string]interface{}) (interface{}, error) {
	includeUsage := true
	if v, ok := params["includeUsage"].(bool); ok {
		includeUsage = v
	}

	includeOwnership := true
	if v, ok := params["includeOwnership"].(bool); ok {
		includeOwnership = v
	}

	includeContracts := true
	if v, ok := params["includeContracts"].(bool); ok {
		includeContracts = v
	}

	includeComplexity := true
	if v, ok := params["includeComplexity"].(bool); ok {
		includeComplexity = v
	}

	var minComplexity int
	if v, ok := params["minComplexity"].(float64); ok {
		minComplexity = int(v)
	}

	var minCalls int
	if v, ok := params["minCalls"].(float64); ok {
		minCalls = int(v)
	}

	var maxSymbols int
	if v, ok := params["maxSymbols"].(float64); ok {
		maxSymbols = int(v)
	}

	repoRoot := s.engine().GetRepoRoot()
	exporter := export.NewExporter(repoRoot, s.logger)

	ctx := context.Background()
	result, err := exporter.Export(ctx, export.ExportOptions{
		RepoRoot:          repoRoot,
		IncludeUsage:      includeUsage,
		IncludeOwnership:  includeOwnership,
		IncludeContracts:  includeContracts,
		IncludeComplexity: includeComplexity,
		MinComplexity:     minComplexity,
		MinCalls:          minCalls,
		MaxSymbols:        maxSymbols,
		Format:            "text",
	})

	if err != nil {
		return nil, fmt.Errorf("failed to export for LLM: %w", err)
	}

	// Format output as text for LLM consumption
	formatted := exporter.FormatText(result, export.ExportOptions{
		IncludeComplexity: includeComplexity,
		IncludeUsage:      includeUsage,
		IncludeContracts:  includeContracts,
	})

	return map[string]interface{}{
		"text":     formatted,
		"metadata": result.Metadata,
	}, nil
}

// toolAuditRisk finds risky code based on multiple signals
func (s *MCPServer) toolAuditRisk(params map[string]interface{}) (interface{}, error) {
	minScore := 40.0
	if v, ok := params["minScore"].(float64); ok {
		minScore = v
	}

	limit := 50
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	factor := ""
	if v, ok := params["factor"].(string); ok {
		factor = v
	}

	quickWins := false
	if v, ok := params["quickWins"].(bool); ok {
		quickWins = v
	}

	repoRoot := s.engine().GetRepoRoot()
	analyzer := audit.NewAnalyzer(repoRoot, s.logger)

	ctx := context.Background()
	result, err := analyzer.Analyze(ctx, audit.AuditOptions{
		RepoRoot:  repoRoot,
		MinScore:  minScore,
		Limit:     limit,
		Factor:    factor,
		QuickWins: quickWins,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to audit risk: %w", err)
	}

	// If quickWins mode, return only quick wins
	if quickWins {
		return map[string]interface{}{
			"quickWins": result.QuickWins,
			"summary":   result.Summary,
		}, nil
	}

	return result, nil
}
