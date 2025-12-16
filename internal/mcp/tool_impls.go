package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"ckb/internal/query"
)

// toolGetStatus implements the getStatus tool
func (s *MCPServer) toolGetStatus(params map[string]interface{}) (interface{}, error) {
	s.logger.Debug("Executing getStatus", map[string]interface{}{
		"params": params,
	})

	ctx := context.Background()
	statusResp, err := s.engine.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	backends := make([]map[string]interface{}, 0, len(statusResp.Backends))
	for _, b := range statusResp.Backends {
		backends = append(backends, map[string]interface{}{
			"id":           b.Id,
			"available":    b.Available,
			"healthy":      b.Healthy,
			"capabilities": b.Capabilities,
			"details":      b.Details,
		})
	}

	status := "unhealthy"
	if statusResp.Healthy {
		status = "healthy"
	}

	result := map[string]interface{}{
		"status":   status,
		"healthy":  statusResp.Healthy,
		"backends": backends,
		"cache": map[string]interface{}{
			"sizeBytes":     statusResp.Cache.SizeBytes,
			"queriesCached": statusResp.Cache.QueriesCached,
			"viewsCached":   statusResp.Cache.ViewsCached,
			"hitRate":       statusResp.Cache.HitRate,
		},
		"repoState": map[string]interface{}{
			"dirty":       statusResp.RepoState.Dirty,
			"repoStateId": statusResp.RepoState.RepoStateId,
			"headCommit":  statusResp.RepoState.HeadCommit,
		},
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolDoctor implements the doctor tool
func (s *MCPServer) toolDoctor(params map[string]interface{}) (interface{}, error) {
	s.logger.Debug("Executing doctor", map[string]interface{}{
		"params": params,
	})

	ctx := context.Background()
	doctorResp, err := s.engine.Doctor(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to run diagnostics: %w", err)
	}

	checks := make([]map[string]interface{}, 0, len(doctorResp.Checks))
	for _, c := range doctorResp.Checks {
		fixes := make([]string, 0, len(c.SuggestedFixes))
		for _, f := range c.SuggestedFixes {
			if f.Command != "" {
				fixes = append(fixes, f.Command)
			}
		}

		checks = append(checks, map[string]interface{}{
			"name":    c.Name,
			"status":  c.Status,
			"message": c.Message,
			"fixes":   fixes,
		})
	}

	result := map[string]interface{}{
		"healthy": doctorResp.Healthy,
		"checks":  checks,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetSymbol implements the getSymbol tool
func (s *MCPServer) toolGetSymbol(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	repoStateMode, _ := params["repoStateMode"].(string)
	if repoStateMode == "" {
		repoStateMode = "head"
	}

	s.logger.Debug("Executing getSymbol", map[string]interface{}{
		"symbolId":      symbolId,
		"repoStateMode": repoStateMode,
	})

	ctx := context.Background()
	opts := query.GetSymbolOptions{
		SymbolId:      symbolId,
		RepoStateMode: repoStateMode,
	}

	symbolResp, err := s.engine.GetSymbol(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol: %w", err)
	}

	result := map[string]interface{}{}

	if symbolResp.Symbol != nil {
		sym := symbolResp.Symbol
		symbolInfo := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
		}
		if sym.Signature != "" {
			symbolInfo["signature"] = sym.Signature
		}
		if sym.SignatureNormalized != "" {
			symbolInfo["signatureNormalized"] = sym.SignatureNormalized
		}
		if sym.ContainerName != "" {
			symbolInfo["containerName"] = sym.ContainerName
		}
		if sym.Documentation != "" {
			symbolInfo["documentation"] = sym.Documentation
		}
		if sym.ModuleId != "" {
			symbolInfo["moduleId"] = sym.ModuleId
		}

		if sym.Visibility != nil {
			symbolInfo["visibility"] = map[string]interface{}{
				"visibility": sym.Visibility.Visibility,
				"confidence": sym.Visibility.Confidence,
			}
		}

		if sym.Location != nil {
			symbolInfo["location"] = map[string]interface{}{
				"fileId":      sym.Location.FileId,
				"startLine":   sym.Location.StartLine,
				"startColumn": sym.Location.StartColumn,
				"endLine":     sym.Location.EndLine,
				"endColumn":   sym.Location.EndColumn,
			}
		}

		result["symbol"] = symbolInfo
	}

	if symbolResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     symbolResp.Provenance.RepoStateId,
			"repoStateDirty":  symbolResp.Provenance.RepoStateDirty,
			"queryDurationMs": symbolResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolSearchSymbols implements the searchSymbols tool
func (s *MCPServer) toolSearchSymbols(params map[string]interface{}) (interface{}, error) {
	queryStr, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'query' parameter")
	}

	scope, _ := params["scope"].(string)

	var kinds []string
	if kindsVal, ok := params["kinds"].([]interface{}); ok {
		for _, k := range kindsVal {
			if kStr, ok := k.(string); ok {
				kinds = append(kinds, kStr)
			}
		}
	}

	limit := 20
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	s.logger.Debug("Executing searchSymbols", map[string]interface{}{
		"query": queryStr,
		"scope": scope,
		"kinds": kinds,
		"limit": limit,
	})

	ctx := context.Background()
	opts := query.SearchSymbolsOptions{
		Query: queryStr,
		Scope: scope,
		Kinds: kinds,
		Limit: limit,
	}

	searchResp, err := s.engine.SearchSymbols(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	symbols := make([]map[string]interface{}, 0, len(searchResp.Symbols))
	for _, sym := range searchResp.Symbols {
		symbolInfo := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
			"score":    sym.Score,
		}

		if sym.ModuleId != "" {
			symbolInfo["moduleId"] = sym.ModuleId
		}

		if sym.Visibility != nil {
			symbolInfo["visibility"] = map[string]interface{}{
				"visibility": sym.Visibility.Visibility,
				"confidence": sym.Visibility.Confidence,
			}
		}

		if sym.Location != nil {
			symbolInfo["location"] = map[string]interface{}{
				"fileId":      sym.Location.FileId,
				"startLine":   sym.Location.StartLine,
				"startColumn": sym.Location.StartColumn,
			}
		}

		symbols = append(symbols, symbolInfo)
	}

	result := map[string]interface{}{
		"symbols":    symbols,
		"totalCount": searchResp.TotalCount,
		"truncated":  searchResp.Truncated,
	}

	if searchResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     searchResp.Provenance.RepoStateId,
			"repoStateDirty":  searchResp.Provenance.RepoStateDirty,
			"queryDurationMs": searchResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFindReferences implements the findReferences tool
func (s *MCPServer) toolFindReferences(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	scope, _ := params["scope"].(string)

	limit := 100
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	includeTests := false
	if includeVal, ok := params["includeTests"].(bool); ok {
		includeTests = includeVal
	}

	s.logger.Debug("Executing findReferences", map[string]interface{}{
		"symbolId":     symbolId,
		"scope":        scope,
		"limit":        limit,
		"includeTests": includeTests,
	})

	ctx := context.Background()
	opts := query.FindReferencesOptions{
		SymbolId:     symbolId,
		Scope:        scope,
		IncludeTests: includeTests,
		Limit:        limit,
	}

	refsResp, err := s.engine.FindReferences(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("find references failed: %w", err)
	}

	refs := make([]map[string]interface{}, 0, len(refsResp.References))
	for _, r := range refsResp.References {
		ref := map[string]interface{}{
			"kind":   r.Kind,
			"isTest": r.IsTest,
		}

		if r.Context != "" {
			ref["context"] = r.Context
		}

		if r.Location != nil {
			ref["location"] = map[string]interface{}{
				"fileId":      r.Location.FileId,
				"startLine":   r.Location.StartLine,
				"startColumn": r.Location.StartColumn,
				"endLine":     r.Location.EndLine,
				"endColumn":   r.Location.EndColumn,
			}
		}

		refs = append(refs, ref)
	}

	result := map[string]interface{}{
		"references": refs,
		"totalCount": refsResp.TotalCount,
		"truncated":  refsResp.Truncated,
	}

	if refsResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     refsResp.Provenance.RepoStateId,
			"repoStateDirty":  refsResp.Provenance.RepoStateDirty,
			"queryDurationMs": refsResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetArchitecture implements the getArchitecture tool
func (s *MCPServer) toolGetArchitecture(params map[string]interface{}) (interface{}, error) {
	depth := 2
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	includeExternalDeps := false
	if includeVal, ok := params["includeExternalDeps"].(bool); ok {
		includeExternalDeps = includeVal
	}

	refresh := false
	if refreshVal, ok := params["refresh"].(bool); ok {
		refresh = refreshVal
	}

	s.logger.Debug("Executing getArchitecture", map[string]interface{}{
		"depth":               depth,
		"includeExternalDeps": includeExternalDeps,
		"refresh":             refresh,
	})

	ctx := context.Background()
	opts := query.GetArchitectureOptions{
		Depth:               depth,
		IncludeExternalDeps: includeExternalDeps,
		Refresh:             refresh,
	}

	archResp, err := s.engine.GetArchitecture(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("architecture analysis failed: %w", err)
	}

	modules := make([]map[string]interface{}, 0, len(archResp.Modules))
	for _, m := range archResp.Modules {
		moduleInfo := map[string]interface{}{
			"moduleId":    m.ModuleId,
			"name":        m.Name,
			"symbolCount": m.SymbolCount,
			"fileCount":   m.FileCount,
		}

		if m.Path != "" {
			moduleInfo["path"] = m.Path
		}
		if m.Language != "" {
			moduleInfo["language"] = m.Language
		}

		modules = append(modules, moduleInfo)
	}

	// Convert dependency graph edges
	depEdges := make([]map[string]interface{}, 0, len(archResp.DependencyGraph))
	for _, edge := range archResp.DependencyGraph {
		depEdges = append(depEdges, map[string]interface{}{
			"from":     edge.From,
			"to":       edge.To,
			"kind":     edge.Kind,
			"strength": edge.Strength,
		})
	}

	result := map[string]interface{}{
		"modules":         modules,
		"dependencyGraph": depEdges,
		"truncated":       archResp.Truncated,
	}

	if archResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     archResp.Provenance.RepoStateId,
			"repoStateDirty":  archResp.Provenance.RepoStateDirty,
			"queryDurationMs": archResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolAnalyzeImpact implements the analyzeImpact tool
func (s *MCPServer) toolAnalyzeImpact(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	depth := 2
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	s.logger.Debug("Executing analyzeImpact", map[string]interface{}{
		"symbolId": symbolId,
		"depth":    depth,
	})

	ctx := context.Background()
	opts := query.AnalyzeImpactOptions{
		SymbolId: symbolId,
		Depth:    depth,
	}

	impactResp, err := s.engine.AnalyzeImpact(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("impact analysis failed: %w", err)
	}

	directImpact := make([]map[string]interface{}, 0, len(impactResp.DirectImpact))
	for _, item := range impactResp.DirectImpact {
		itemInfo := map[string]interface{}{
			"stableId":   item.StableId,
			"name":       item.Name,
			"kind":       item.Kind,
			"distance":   item.Distance,
			"moduleId":   item.ModuleId,
			"confidence": item.Confidence,
		}
		if item.Location != nil {
			itemInfo["location"] = map[string]interface{}{
				"fileId":    item.Location.FileId,
				"startLine": item.Location.StartLine,
			}
		}
		directImpact = append(directImpact, itemInfo)
	}

	transitiveImpact := make([]map[string]interface{}, 0, len(impactResp.TransitiveImpact))
	for _, item := range impactResp.TransitiveImpact {
		itemInfo := map[string]interface{}{
			"stableId":   item.StableId,
			"name":       item.Name,
			"kind":       item.Kind,
			"distance":   item.Distance,
			"moduleId":   item.ModuleId,
			"confidence": item.Confidence,
		}
		if item.Location != nil {
			itemInfo["location"] = map[string]interface{}{
				"fileId":    item.Location.FileId,
				"startLine": item.Location.StartLine,
			}
		}
		transitiveImpact = append(transitiveImpact, itemInfo)
	}

	result := map[string]interface{}{
		"directImpact":     directImpact,
		"transitiveImpact": transitiveImpact,
	}

	if impactResp.RiskScore != nil {
		factors := make([]map[string]interface{}, 0, len(impactResp.RiskScore.Factors))
		for _, f := range impactResp.RiskScore.Factors {
			factors = append(factors, map[string]interface{}{
				"name":  f.Name,
				"value": f.Value,
			})
		}
		result["riskScore"] = map[string]interface{}{
			"score":       impactResp.RiskScore.Score,
			"level":       impactResp.RiskScore.Level,
			"explanation": impactResp.RiskScore.Explanation,
			"factors":     factors,
		}
	}

	if impactResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     impactResp.Provenance.RepoStateId,
			"repoStateDirty":  impactResp.Provenance.RepoStateDirty,
			"queryDurationMs": impactResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}
